// Package procstat lee de una vez lo que gasta cada proceso (procesador,
// lecturas de disco y fallos de página «duros», los que van al disco) con una
// sola llamada a Windows, para saber quién estaba haciendo qué en cada tirón.
package procstat

import (
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modNtdll = windows.NewLazySystemDLL("ntdll.dll")
	pNtQSI   = modNtdll.NewProc("NtQuerySystemInformation")
)

type raw struct {
	cpu  int64 // 100 ns
	read uint64
	hard uint32
	name string
}

// Use es lo que gastó un proceso en el último intervalo.
type Use struct {
	PID       uint32  `json:"pid"`
	Name      string  `json:"name"`
	CPU       float64 `json:"cpu"`       // % del equipo entero
	ReadMBps  float64 `json:"readMBps"`  // lecturas (de disco o caché)
	HardFault uint32  `json:"hardFault"` // páginas que tuvieron que leerse del disco
}

// Sampler compara dos lecturas seguidas.
type Sampler struct {
	mu    sync.Mutex
	buf   []byte
	prev  map[uint32]raw
	prevT time.Time
	ncpu  float64
}

// New prepara el lector.
func New(ncpu int) *Sampler {
	if ncpu < 1 {
		ncpu = 1
	}
	return &Sampler{buf: make([]byte, 1<<20), prev: map[uint32]raw{}, ncpu: float64(ncpu)}
}

func (s *Sampler) read() map[uint32]raw {
	for {
		var ret uint32
		r, _, _ := pNtQSI.Call(5, uintptr(unsafe.Pointer(&s.buf[0])), uintptr(len(s.buf)), uintptr(unsafe.Pointer(&ret))) // SystemProcessInformation
		if r == 0xC0000004 {                                                                                              // STATUS_INFO_LENGTH_MISMATCH
			s.buf = make([]byte, int(ret)+64<<10)
			continue
		}
		if r != 0 {
			return nil
		}
		break
	}
	out := make(map[uint32]raw, 256)
	for off := 0; off+256 <= len(s.buf); {
		b := s.buf[off:]
		next := *(*uint32)(unsafe.Pointer(&b[0]))
		pid := uint32(*(*uintptr)(unsafe.Pointer(&b[80])))
		user := *(*int64)(unsafe.Pointer(&b[40]))
		kern := *(*int64)(unsafe.Pointer(&b[48]))
		hard := *(*uint32)(unsafe.Pointer(&b[16]))
		read := *(*uint64)(unsafe.Pointer(&b[232]))
		nameLen := int(*(*uint16)(unsafe.Pointer(&b[56])))
		namePtr := *(**uint16)(unsafe.Pointer(&b[64]))
		name := ""
		if namePtr != nil && nameLen > 0 {
			name = windows.UTF16ToString(unsafe.Slice(namePtr, nameLen/2))
		}
		out[pid] = raw{cpu: user + kern, read: read, hard: hard, name: strings.ToLower(name)}
		if next == 0 {
			break
		}
		off += int(next)
	}
	return out
}

// Sample devuelve lo que gastó cada proceso desde la última vez (los que gastaron algo).
func (s *Sampler) Sample() []Use {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.read()
	now := time.Now()
	dt := now.Sub(s.prevT).Seconds()
	var out []Use
	if !s.prevT.IsZero() && dt > 0 {
		for pid, c := range cur {
			if pid == 0 {
				continue // el proceso de reposo
			}
			p, ok := s.prev[pid]
			if !ok || c.cpu < p.cpu {
				continue
			}
			u := Use{PID: pid, Name: c.name,
				CPU:       float64(c.cpu-p.cpu) / 1e7 / dt / s.ncpu * 100,
				ReadMBps:  float64(c.read-p.read) / dt / (1 << 20),
				HardFault: c.hard - p.hard,
			}
			if u.CPU >= 0.5 || u.ReadMBps >= 1 || u.HardFault > 0 {
				out = append(out, u)
			}
		}
	}
	s.prev, s.prevT = cur, now
	sort.Slice(out, func(i, j int) bool { return out[i].CPU > out[j].CPU })
	return out
}
