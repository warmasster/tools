package main

// Escritor de la memoria compartida que lee la cámara virtual. El formato está
// descrito en camera/protocol.h; las constantes de aquí deben coincidir con las de
// allí (lo comprueba TestProtocolMatchesHeader).

import (
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

// shmName es el de protocol.h; las pruebas usan otro para no tocar la cámara de verdad.
var shmName = `Local\TurboCAM.Video.1`

const (
	tcMapSDDL = "D:(A;;GA;;;WD)(A;;GA;;;AC)(A;;GA;;;S-1-15-2-2)S:(ML;;NW;;;LW)"

	tcMagic   = 0x4D414354
	tcVersion = 1

	tcMaxW         = 1920
	tcMaxH         = 1080
	tcSlots        = 3
	tcMaxConsumers = 16

	tcStateOff     = 0
	tcStateWaiting = 1
	tcStateLive    = 2
	tcStatePaused  = 3
	tcStateCover   = 4

	tcConsumerTimeoutMs = 5000

	tcOffMagic     = 0
	tcOffVersion   = 4
	tcOffState     = 8
	tcOffLatest    = 12
	tcOffHeartbeat = 16
	tcOffFps       = 20
	tcOffWidth     = 24
	tcOffHeight    = 28

	tcConsBase      = 64
	tcConsSize      = 64
	tcConsPid       = 0
	tcConsSeen      = 4
	tcConsWidth     = 8
	tcConsHeight    = 12
	tcConsFps       = 16
	tcConsExe       = 20
	tcConsExeChars  = 22
	tcSlotBase      = 4096
	tcSlotSeq       = 0
	tcSlotWidth     = 4
	tcSlotHeight    = 8
	tcSlotData      = 64
	tcFrameBytes    = tcMaxW * tcMaxH * 3 / 2
	tcSlotStride    = 3112960
	tcMapSize       = tcSlotBase + tcSlots*tcSlotStride
	tcHeartbeatMsec = 3000
)

// Consumer es una app que está usando la cámara ahora mismo.
type Consumer struct {
	PID    uint32 `json:"pid"`
	Exe    string `json:"exe"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	FPS    int    `json:"fps"`
}

type camShm struct {
	h   windows.Handle
	mem []byte
	mu  sync.Mutex // un fotograma a la vez
	seq uint32
}

func openShm() (*camShm, error) {
	sd, err := windows.SecurityDescriptorFromString(tcMapSDDL)
	if err != nil {
		return nil, err
	}
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	name, _ := windows.UTF16PtrFromString(shmName)
	h, err := windows.CreateFileMapping(windows.InvalidHandle, sa, windows.PAGE_READWRITE, 0, tcMapSize, name)
	if h == 0 {
		return nil, err
	}
	addr, err := windows.MapViewOfFile(h, windows.FILE_MAP_READ|windows.FILE_MAP_WRITE, 0, 0, tcMapSize)
	if err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	// La vista vive fuera del montón de Go y no se mueve: se puede ver como []byte.
	view := *(*unsafe.Pointer)(unsafe.Pointer(&addr))
	s := &camShm{h: h, mem: unsafe.Slice((*byte)(view), tcMapSize)}
	// Si la cámara ya la había creado (una app la abrió antes que TurboCAM), se
	// respeta lo que hay: los consumidores y el número de fotograma.
	s.seq = atomic.LoadUint32(s.u32(tcOffLatest))
	atomic.StoreUint32(s.u32(tcOffVersion), tcVersion)
	atomic.StoreUint32(s.u32(tcOffState), tcStateWaiting)
	s.Beat()
	atomic.StoreUint32(s.u32(tcOffMagic), tcMagic)
	return s, nil
}

func (s *camShm) u32(off int) *uint32 { return (*uint32)(unsafe.Pointer(&s.mem[off])) }

var procGetTickCount = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetTickCount")

// tick32 es el mismo GetTickCount que usa la cámara: milisegundos desde el arranque.
func tick32() uint32 {
	r, _, _ := procGetTickCount.Call()
	return uint32(r)
}

// Beat renueva el latido: sin él la cámara da TurboCAM por cerrado a los 3 s.
func (s *camShm) Beat() { atomic.StoreUint32(s.u32(tcOffHeartbeat), tick32()) }

func (s *camShm) SetState(st uint32) { atomic.StoreUint32(s.u32(tcOffState), st) }

func (s *camShm) SetFormat(w, h, fps int) {
	atomic.StoreUint32(s.u32(tcOffWidth), uint32(w))
	atomic.StoreUint32(s.u32(tcOffHeight), uint32(h))
	atomic.StoreUint32(s.u32(tcOffFps), uint32(fps))
}

// WriteFrame publica un fotograma NV12 compacto de w x h. fill escribe los
// w*h*3/2 bytes en dst.
func (s *camShm) WriteFrame(w, h int, fill func(dst []byte)) {
	if w <= 0 || h <= 0 || w > tcMaxW || h > tcMaxH || w%2 != 0 || h%2 != 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	if s.seq == 0 {
		s.seq = 1
	}
	slot := tcSlotBase + int(s.seq%tcSlots)*tcSlotStride
	atomic.StoreUint32(s.u32(slot+tcSlotSeq), 0)
	fill(s.mem[slot+tcSlotData : slot+tcSlotData+w*h*3/2])
	atomic.StoreUint32(s.u32(slot+tcSlotWidth), uint32(w))
	atomic.StoreUint32(s.u32(slot+tcSlotHeight), uint32(h))
	atomic.StoreUint32(s.u32(slot+tcSlotSeq), s.seq)
	atomic.StoreUint32(s.u32(tcOffLatest), s.seq)
}

// Consumers lista las apps que tienen la cámara abierta y emitiendo.
func (s *camShm) Consumers() []Consumer {
	now := tick32()
	var out []Consumer
	for i := 0; i < tcMaxConsumers; i++ {
		c := tcConsBase + i*tcConsSize
		pid := atomic.LoadUint32(s.u32(c + tcConsPid))
		if pid == 0 || now-atomic.LoadUint32(s.u32(c+tcConsSeen)) > tcConsumerTimeoutMs {
			continue
		}
		exe := make([]uint16, tcConsExeChars)
		for k := range exe {
			exe[k] = *(*uint16)(unsafe.Pointer(&s.mem[c+tcConsExe+2*k]))
		}
		out = append(out, Consumer{
			PID:    pid,
			Exe:    strings.TrimSpace(windows.UTF16ToString(exe)),
			Width:  int(atomic.LoadUint32(s.u32(c + tcConsWidth))),
			Height: int(atomic.LoadUint32(s.u32(c + tcConsHeight))),
			FPS:    int(atomic.LoadUint32(s.u32(c + tcConsFps))),
		})
	}
	return out
}

// Close deja la cámara en «TurboCAM está cerrado» y suelta la memoria.
func (s *camShm) Close() {
	if s == nil || s.mem == nil {
		return
	}
	s.SetState(tcStateOff)
	atomic.StoreUint32(s.u32(tcOffHeartbeat), 0)
	windows.UnmapViewOfFile(uintptr(unsafe.Pointer(&s.mem[0])))
	windows.CloseHandle(s.h)
	s.mem = nil
}
