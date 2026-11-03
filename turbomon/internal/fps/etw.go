// Package fps mide los fotogramas por segundo de cualquier juego escuchando los
// eventos que Windows ya emite cada vez que un programa presenta un fotograma
// (ETW: DXGI para DirectX 10/11/12, D3D9 y, para Vulkan y OpenGL, el nÃºcleo
// grÃ¡fico). Es lo mismo que hace PresentMon: no se inyecta nada en el juego, asÃ­
// que no hay problemas con los antitrampas.
//
// Escuchar estos eventos necesita permiso de administrador.
package fps

import (
	"errors"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modAdvapi     = windows.NewLazySystemDLL("advapi32.dll")
	pStartTrace   = modAdvapi.NewProc("StartTraceW")
	pControlTrace = modAdvapi.NewProc("ControlTraceW")
	pEnableEx2    = modAdvapi.NewProc("EnableTraceEx2")
	pOpenTrace    = modAdvapi.NewProc("OpenTraceW")
	pProcessTrace = modAdvapi.NewProc("ProcessTrace")
	pCloseTrace   = modAdvapi.NewProc("CloseTrace")
)

// Fuentes de un fotograma.
const (
	SrcDXGI   = 1
	SrcD3D9   = 2
	SrcKernel = 3
)

var (
	guidDXGI    = windows.GUID{Data1: 0xCA11C036, Data2: 0x0102, Data3: 0x4A2D, Data4: [8]byte{0xA6, 0xAD, 0xF0, 0x3C, 0xFE, 0xD5, 0xD3, 0xC9}}
	guidD3D9    = windows.GUID{Data1: 0x783ACA0A, Data2: 0x790E, Data3: 0x4D7F, Data4: [8]byte{0x84, 0x51, 0xAA, 0x85, 0x05, 0x11, 0xC6, 0xB9}}
	guidDxgKrnl = windows.GUID{Data1: 0x802EC45A, Data2: 0x1E99, Data3: 0x4B83, Data4: [8]byte{0x99, 0x20, 0x87, 0xC9, 0x82, 0x77, 0xBA, 0x9D}}
)

const (
	sessionName       = "TurboMON-FPS"
	wnodeTracedGUID   = 0x00020000
	realTimeMode      = 0x00000100
	ptmRealTime       = 0x00000100
	ptmEventRecord    = 0x10000000
	ptmRawTimestamp   = 0x00001000
	controlStop       = 1
	errAlreadyExists  = 183
	invalidHandle     = ^uint64(0)
	propsHeaderSize   = 120
	logfileSize       = 448
	logfileNameOff    = 8
	logfileModeOff    = 28
	logfileRecCbOff   = 424
	traceLevelInfo    = 4
	kwDxgKrnlBase     = 0x1
	kwDxgKrnlPresent  = 0x8000000
	eventControlEnable = 1
)

var (
	modKernel = windows.NewLazySystemDLL("kernel32.dll")
	pQPC      = modKernel.NewProc("QueryPerformanceCounter")
	pQPF      = modKernel.NewProc("QueryPerformanceFrequency")
)

// Now es la marca de tiempo actual, en el mismo reloj que los eventos.
func Now() int64 {
	var v int64
	pQPC.Call(uintptr(unsafe.Pointer(&v)))
	return v
}

// Freq es cuántas marcas hay en un segundo.
func Freq() int64 {
	var v int64
	pQPF.Call(uintptr(unsafe.Pointer(&v)))
	return v
}

// ErrNeedAdmin: Windows solo deja escuchar estos eventos a un administrador.
var ErrNeedAdmin = errors.New("medir los FPS necesita permiso de administrador")

type eventHeader struct {
	Size, HeaderType, Flags, EventProperty uint16
	ThreadID, ProcessID                    uint32
	TimeStamp                              int64
	Provider                               windows.GUID
	ID                                     uint16
	Version, Channel, Level, Opcode        uint8
	Task                                   uint16
	Keyword                                uint64
	ProcessorTime                          uint64
	Activity                               windows.GUID
}

// Session es una sesiÃ³n de ETW en tiempo real.
type Session struct {
	handle uint64
	trace  uint64
	props  []byte
	lf     []byte // EVENT_TRACE_LOGFILEW: vive lo que la sesión
	name   *uint16
	done   chan struct{}
	once   sync.Once
}

var (
	cbMu      sync.Mutex
	cbPresent func(pid uint32, ts int64, src int)
	callback  = windows.NewCallback(onEvent)
)

func onEvent(rec *eventHeader) uintptr {
	src := 0
	switch rec.Provider.Data1 {
	case guidDXGI.Data1:
		if rec.ID == 42 || rec.ID == 55 { // Present_Start, PresentMultiplaneOverlay_Start
			src = SrcDXGI
		}
	case guidD3D9.Data1:
		if rec.ID == 1 { // Present_Start
			src = SrcD3D9
		}
	case guidDxgKrnl.Data1:
		if rec.ID == 184 || rec.ID == 168 || rec.ID == 252 { // Present_Info, Flip_Info, FlipMultiPlaneOverlay_Info
			src = SrcKernel
		}
	}
	if src != 0 {
		f := cbPresent
		if f != nil {
			f(rec.ProcessID, rec.TimeStamp, src)
		}
	}
	return 0
}

func newProps() []byte {
	name, _ := windows.UTF16FromString(sessionName)
	b := make([]byte, propsHeaderSize+len(name)*2+16)
	put32 := func(off int, v uint32) { *(*uint32)(unsafe.Pointer(&b[off])) = v }
	put32(0, uint32(len(b)))       // Wnode.BufferSize
	put32(40, 1)                   // Wnode.ClientContext: QueryPerformanceCounter
	put32(44, wnodeTracedGUID)     // Wnode.Flags
	put32(48, 64)                  // BufferSize (KB)
	put32(52, 4)                   // MinimumBuffers
	put32(56, 32)                  // MaximumBuffers
	put32(64, realTimeMode)        // LogFileMode
	put32(68, 1)                   // FlushTimer (s)
	put32(116, propsHeaderSize)    // LoggerNameOffset
	return b
}

func stopByName() {
	p := newProps()
	name, _ := windows.UTF16PtrFromString(sessionName)
	pControlTrace.Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&p[0])), controlStop)
}

// Start empieza a escuchar. onPresent se llama (desde otro hilo) con cada
// fotograma: proceso, marca de tiempo (contador de rendimiento) y fuente.
func Start(onPresent func(pid uint32, ts int64, src int)) (*Session, error) {
	if pStartTrace.Find() != nil {
		return nil, errors.New("este Windows no tiene ETW")
	}
	s := &Session{props: newProps(), done: make(chan struct{})}
	name, _ := windows.UTF16PtrFromString(sessionName)
	r, _, _ := pStartTrace.Call(uintptr(unsafe.Pointer(&s.handle)), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&s.props[0])))
	if r == errAlreadyExists {
		// Una sesiÃ³n que quedÃ³ de una vez anterior (se cerrÃ³ sin pararla).
		stopByName()
		s.props = newProps()
		r, _, _ = pStartTrace.Call(uintptr(unsafe.Pointer(&s.handle)), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&s.props[0])))
	}
	if r != 0 {
		if r == uintptr(windows.ERROR_ACCESS_DENIED) {
			return nil, ErrNeedAdmin
		}
		return nil, syscall.Errno(r)
	}
	enable := func(g *windows.GUID, kw uint64) uintptr {
		r, _, _ := pEnableEx2.Call(uintptr(s.handle), uintptr(unsafe.Pointer(g)), eventControlEnable, traceLevelInfo, uintptr(kw), 0, 0, 0)
		return r
	}
	if r := enable(&guidDXGI, ^uint64(0)); r != 0 {
		s.stopSession()
		return nil, syscall.Errno(r)
	}
	enable(&guidD3D9, ^uint64(0))
	enable(&guidDxgKrnl, kwDxgKrnlBase|kwDxgKrnlPresent)

	lf := make([]byte, logfileSize)
	*(**uint16)(unsafe.Pointer(&lf[logfileNameOff])) = name
	*(*uint32)(unsafe.Pointer(&lf[logfileModeOff])) = ptmRealTime | ptmEventRecord | ptmRawTimestamp
	*(*uintptr)(unsafe.Pointer(&lf[logfileRecCbOff])) = callback
	th, _, _ := pOpenTrace.Call(uintptr(unsafe.Pointer(&lf[0])))
	if uint64(th) == invalidHandle {
		s.stopSession()
		return nil, errors.New("no se pudo abrir la sesiÃ³n de eventos")
	}
	s.trace = uint64(th)
	cbMu.Lock()
	cbPresent = onPresent
	cbMu.Unlock()
	go func() {
		runtime.LockOSThread()
		defer close(s.done)
		pProcessTrace.Call(uintptr(unsafe.Pointer(&s.trace)), 1, 0, 0) // se queda aquÃ­ hasta Stop
	}()
	s.lf, s.name = lf, name
	return s, nil
}

func (s *Session) stopSession() {
	pControlTrace.Call(uintptr(s.handle), 0, uintptr(unsafe.Pointer(&s.props[0])), controlStop)
}

// Stop para la sesiÃ³n (Windows deja de mandar eventos al momento).
func (s *Session) Stop() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		s.stopSession()
		pCloseTrace.Call(uintptr(s.trace))
		<-s.done
		cbMu.Lock()
		cbPresent = nil
		cbMu.Unlock()
	})
}

// EventsLost dice cuÃ¡ntos eventos se han perdido (si Windows no da abasto).
func (s *Session) EventsLost() uint32 {
	if s == nil {
		return 0
	}
	p := newProps()
	name, _ := windows.UTF16PtrFromString(sessionName)
	pControlTrace.Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&p[0])), 0) // EVENT_TRACE_CONTROL_QUERY
	return *(*uint32)(unsafe.Pointer(&p[88]))
}
