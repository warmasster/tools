// Package etw abre sesiones de ETW (los eventos que Windows emite por dentro)
// en tiempo real y reparte cada evento a quien lo haya pedido, por proveedor.
//
// Una sesión puede tener varios proveedores, y se pueden añadir y quitar sin
// pararla: así TurboMON solo escucha la red o el disco mientras hace falta.
// Necesita permiso de administrador.
package etw

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
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

	modKernel = windows.NewLazySystemDLL("kernel32.dll")
	pQPC      = modKernel.NewProc("QueryPerformanceCounter")
	pQPF      = modKernel.NewProc("QueryPerformanceFrequency")
)

const (
	wnodeTracedGUID  = 0x00020000
	realTimeMode     = 0x00000100
	systemLoggerMode = 0x02000000
	ptmRealTime      = 0x00000100
	ptmEventRecord   = 0x10000000
	ptmRawTimestamp  = 0x00001000
	controlQuery     = 0
	controlStop      = 1
	errAlreadyExists = 183
	invalidHandle    = ^uint64(0)
	propsHeaderSize  = 120
	logfileSize      = 448
	logfileNameOff   = 8
	logfileModeOff   = 28
	logfileRecCbOff  = 424
	logfileCtxOff    = 440
	controlEnable    = 1
	controlDisable   = 0
	filterTypePID    = 0x80000004
)

// Indicadores del registrador del núcleo (para Options.KernelFlags).
const (
	KernelDPC       = 0x00000020
	KernelInterrupt = 0x00000040
)

// ErrNeedAdmin: Windows solo deja escuchar estos eventos a un administrador.
var ErrNeedAdmin = errors.New("hace falta permiso de administrador")

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

// Header es EVENT_HEADER.
type Header struct {
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

// Record es EVENT_RECORD.
type Record struct {
	Header            Header
	BufferContext     uint32
	ExtendedDataCount uint16
	UserDataLength    uint16
	ExtendedData      uintptr
	UserData          unsafe.Pointer
	UserContext       uintptr
}

// Data devuelve los datos del evento (válidos solo mientras dura el aviso).
func (r *Record) Data() []byte {
	if r.UserData == nil || r.UserDataLength == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(r.UserData), r.UserDataLength)
}

// Handler recibe los eventos de un proveedor. Se llama desde el hilo de la
// sesión: tiene que volver enseguida.
type Handler func(r *Record)

// Options de una sesión.
type Options struct {
	// KernelFlags > 0 abre un registrador del núcleo (DPC, interrupciones…)
	// en vez de una sesión normal de proveedores.
	KernelFlags uint32
	BufferKB    uint32
}

// Session es una sesión de ETW en tiempo real.
type Session struct {
	name     string
	id       uintptr
	handle   uint64
	trace    uint64
	props    []byte
	lf       []byte // EVENT_TRACE_LOGFILEW: vive lo que la sesión
	namePtr  *uint16
	done     chan struct{}
	once     sync.Once
	mu       sync.Mutex
	handlers atomic.Pointer[map[uint32]Handler] // por GUID.Data1 del proveedor
	fallback atomic.Pointer[Handler]            // registrador del núcleo: todos los eventos
	opts     Options
}

var (
	sessions   [16]atomic.Pointer[Session]
	nextID     atomic.Uintptr
	callbackFn = windows.NewCallback(onEvent)
)

func onEvent(r *Record) uintptr {
	if r.UserContext == 0 || r.UserContext >= uintptr(len(sessions)) {
		return 0
	}
	s := sessions[r.UserContext].Load()
	if s == nil {
		return 0
	}
	if m := s.handlers.Load(); m != nil {
		if h, ok := (*m)[r.Header.Provider.Data1]; ok {
			h(r)
			return 0
		}
	}
	if f := s.fallback.Load(); f != nil {
		(*f)(r)
	}
	return 0
}

func newProps(name string, o Options) []byte {
	n, _ := windows.UTF16FromString(name)
	b := make([]byte, propsHeaderSize+len(n)*2+16)
	put := func(off int, v uint32) { *(*uint32)(unsafe.Pointer(&b[off])) = v }
	kb := o.BufferKB
	if kb == 0 {
		kb = 64
	}
	put(0, uint32(len(b)))   // Wnode.BufferSize
	put(40, 1)               // Wnode.ClientContext: QueryPerformanceCounter
	put(44, wnodeTracedGUID) // Wnode.Flags
	put(48, kb)              // BufferSize (KB)
	put(52, 4)               // MinimumBuffers
	put(56, 64)              // MaximumBuffers
	mode := uint32(realTimeMode)
	if o.KernelFlags != 0 {
		mode |= systemLoggerMode
		put(72, o.KernelFlags) // EnableFlags
	}
	put(64, mode)             // LogFileMode
	put(68, 1)                // FlushTimer (s)
	put(116, propsHeaderSize) // LoggerNameOffset
	return b
}

// StopByName para una sesión que haya quedado abierta (de una vez anterior).
func StopByName(name string) {
	p := newProps(name, Options{})
	n, _ := windows.UTF16PtrFromString(name)
	pControlTrace.Call(0, uintptr(unsafe.Pointer(n)), uintptr(unsafe.Pointer(&p[0])), controlStop)
}

// Start abre una sesión en tiempo real con ese nombre.
func Start(name string, o Options) (*Session, error) {
	if pStartTrace.Find() != nil {
		return nil, errors.New("este Windows no tiene ETW")
	}
	id := nextID.Add(1)
	if id >= uintptr(len(sessions)) {
		return nil, errors.New("demasiadas sesiones de eventos")
	}
	s := &Session{name: name, id: id, props: newProps(name, o), done: make(chan struct{}), opts: o}
	empty := map[uint32]Handler{}
	s.handlers.Store(&empty)
	n, _ := windows.UTF16PtrFromString(name)
	r, _, _ := pStartTrace.Call(uintptr(unsafe.Pointer(&s.handle)), uintptr(unsafe.Pointer(n)), uintptr(unsafe.Pointer(&s.props[0])))
	if r == errAlreadyExists {
		StopByName(name)
		s.props = newProps(name, o)
		r, _, _ = pStartTrace.Call(uintptr(unsafe.Pointer(&s.handle)), uintptr(unsafe.Pointer(n)), uintptr(unsafe.Pointer(&s.props[0])))
	}
	if r != 0 {
		if r == uintptr(windows.ERROR_ACCESS_DENIED) {
			return nil, ErrNeedAdmin
		}
		return nil, syscall.Errno(r)
	}
	sessions[id].Store(s)
	lf := make([]byte, logfileSize)
	*(**uint16)(unsafe.Pointer(&lf[logfileNameOff])) = n
	*(*uint32)(unsafe.Pointer(&lf[logfileModeOff])) = ptmRealTime | ptmEventRecord | ptmRawTimestamp
	*(*uintptr)(unsafe.Pointer(&lf[logfileRecCbOff])) = callbackFn
	*(*uintptr)(unsafe.Pointer(&lf[logfileCtxOff])) = id
	th, _, _ := pOpenTrace.Call(uintptr(unsafe.Pointer(&lf[0])))
	if uint64(th) == invalidHandle {
		s.stopSession()
		sessions[id].Store(nil)
		return nil, errors.New("no se pudo abrir la sesión de eventos")
	}
	s.trace = uint64(th)
	s.lf, s.namePtr = lf, n
	go func() {
		runtime.LockOSThread()
		defer close(s.done)
		pProcessTrace.Call(uintptr(unsafe.Pointer(&s.trace)), 1, 0, 0) // se queda aquí hasta Stop
	}()
	return s, nil
}

// OnAll recibe todos los eventos que no tengan un proveedor propio (para el
// registrador del núcleo, cuyos eventos vienen de varias «clases»).
func (s *Session) OnAll(h Handler) {
	if s != nil {
		s.fallback.Store(&h)
	}
}

func (s *Session) setHandler(data1 uint32, h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := *s.handlers.Load()
	m := make(map[uint32]Handler, len(old)+1)
	for k, v := range old {
		m[k] = v
	}
	if h == nil {
		delete(m, data1)
	} else {
		m[data1] = h
	}
	s.handlers.Store(&m)
}

// Enable añade un proveedor. pids (como mucho 8) limita los eventos a esos
// procesos (solo vale para proveedores que escriben en el contexto del proceso).
func (s *Session) Enable(g windows.GUID, level uint8, keywords uint64, pids []uint32, h Handler) error {
	if s == nil {
		return errors.New("sesión cerrada")
	}
	s.setHandler(g.Data1, h)
	var params []byte
	var pidList []uint32
	if len(pids) > 0 {
		if len(pids) > 8 {
			pids = pids[:8]
		}
		pidList = append([]uint32(nil), pids...)
		filter := make([]byte, 16) // EVENT_FILTER_DESCRIPTOR
		*(*uintptr)(unsafe.Pointer(&filter[0])) = uintptr(unsafe.Pointer(&pidList[0]))
		*(*uint32)(unsafe.Pointer(&filter[8])) = uint32(4 * len(pidList))
		*(*uint32)(unsafe.Pointer(&filter[12])) = filterTypePID
		params = make([]byte, 48)                  // ENABLE_TRACE_PARAMETERS
		*(*uint32)(unsafe.Pointer(&params[0])) = 2 // ENABLE_TRACE_PARAMETERS_VERSION_2
		*(*uintptr)(unsafe.Pointer(&params[32])) = uintptr(unsafe.Pointer(&filter[0]))
		*(*uint32)(unsafe.Pointer(&params[40])) = 1
		defer runtime.KeepAlive(filter)
	}
	var pp uintptr
	if params != nil {
		pp = uintptr(unsafe.Pointer(&params[0]))
	}
	r, _, _ := pEnableEx2.Call(uintptr(s.handle), uintptr(unsafe.Pointer(&g)), controlEnable, uintptr(level), uintptr(keywords), 0, 0, pp)
	runtime.KeepAlive(pidList)
	runtime.KeepAlive(params)
	if r != 0 {
		s.setHandler(g.Data1, nil)
		return syscall.Errno(r)
	}
	return nil
}

// Disable quita un proveedor.
func (s *Session) Disable(g windows.GUID) {
	if s == nil {
		return
	}
	pEnableEx2.Call(uintptr(s.handle), uintptr(unsafe.Pointer(&g)), controlDisable, 0, 0, 0, 0, 0)
	s.setHandler(g.Data1, nil)
}

func (s *Session) stopSession() {
	pControlTrace.Call(uintptr(s.handle), 0, uintptr(unsafe.Pointer(&s.props[0])), controlStop)
}

// Stop para la sesión (Windows deja de mandar eventos al momento).
func (s *Session) Stop() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		s.stopSession()
		pCloseTrace.Call(uintptr(s.trace))
		<-s.done
		sessions[s.id].Store(nil)
	})
}

// EventsLost dice cuántos eventos se han perdido (si Windows no da abasto).
func (s *Session) EventsLost() uint32 {
	if s == nil {
		return 0
	}
	p := newProps(s.name, s.opts)
	n, _ := windows.UTF16PtrFromString(s.name)
	pControlTrace.Call(0, uintptr(unsafe.Pointer(n)), uintptr(unsafe.Pointer(&p[0])), controlQuery)
	return *(*uint32)(unsafe.Pointer(&p[88]))
}

// ---------- lectura de datos ----------

// Reader lee campos de los datos de un evento sin salirse del búfer.
type Reader struct {
	b   []byte
	off int
	bad bool
}

// NewReader lee los datos del evento.
func NewReader(b []byte) *Reader { return &Reader{b: b} }

func (r *Reader) need(n int) bool {
	if r.bad || r.off+n > len(r.b) {
		r.bad = true
		return false
	}
	return true
}

// U8 lee un byte.
func (r *Reader) U8() uint8 {
	if !r.need(1) {
		return 0
	}
	v := r.b[r.off]
	r.off++
	return v
}

// U16 lee un entero de 16 bits (en el orden del procesador).
func (r *Reader) U16() uint16 {
	if !r.need(2) {
		return 0
	}
	v := uint16(r.b[r.off]) | uint16(r.b[r.off+1])<<8
	r.off += 2
	return v
}

// U16BE lee un entero de 16 bits en el orden de la red (puertos).
func (r *Reader) U16BE() uint16 {
	if !r.need(2) {
		return 0
	}
	v := uint16(r.b[r.off])<<8 | uint16(r.b[r.off+1])
	r.off += 2
	return v
}

// U32 lee un entero de 32 bits.
func (r *Reader) U32() uint32 {
	if !r.need(4) {
		return 0
	}
	b := r.b[r.off:]
	v := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
	r.off += 4
	return v
}

// U64 lee un entero de 64 bits.
func (r *Reader) U64() uint64 {
	lo := uint64(r.U32())
	hi := uint64(r.U32())
	return lo | hi<<32
}

// Ptr lee un puntero (8 bytes en 64 bits).
func (r *Reader) Ptr() uint64 { return r.U64() }

// Bytes lee n bytes.
func (r *Reader) Bytes(n int) []byte {
	if !r.need(n) {
		return nil
	}
	v := r.b[r.off : r.off+n]
	r.off += n
	return v
}

// UTF16Z lee una cadena UTF-16 acabada en cero.
func (r *Reader) UTF16Z() string {
	start := r.off
	for r.off+1 < len(r.b) {
		if r.b[r.off] == 0 && r.b[r.off+1] == 0 {
			u := make([]uint16, (r.off-start)/2)
			for i := range u {
				u[i] = uint16(r.b[start+2*i]) | uint16(r.b[start+2*i+1])<<8
			}
			r.off += 2
			return windows.UTF16ToString(u)
		}
		r.off += 2
	}
	r.bad = true
	return ""
}

// Skip se salta n bytes.
func (r *Reader) Skip(n int) { r.need(n); r.off += n }

// OK dice si todo lo leído estaba dentro del búfer.
func (r *Reader) OK() bool { return !r.bad }
