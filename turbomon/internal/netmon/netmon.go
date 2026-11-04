// Package netmon dice cuánto descarga y sube cada programa (con los eventos de
// red de Windows, sin capturar paquetes) y a qué servidor habla un juego, y
// mide el ping con ICMP.
package netmon

import (
	"net/netip"
	"sort"
	"sync"
	"time"

	"golang.org/x/sys/windows"

	"turbomon/internal/etw"
)

// Provider es Microsoft-Windows-Kernel-Network.
var Provider = windows.GUID{Data1: 0x7DD42A49, Data2: 0x5329, Data3: 0x4832, Data4: [8]byte{0x8D, 0xFD, 0x43, 0xD9, 0x79, 0x15, 0x3A, 0x88}}

// Keywords: IPv4 e IPv6.
const Keywords = 0x10 | 0x20

type flowKey struct {
	pid    uint32
	remote netip.AddrPort
	udp    bool
}

type flow struct {
	packets uint64
	bytes   uint64
	last    time.Time
}

type counter struct{ rx, tx uint64 }

// Monitor junta lo que llega de los eventos.
type Monitor struct {
	mu    sync.Mutex
	procs map[uint32]*counter
	prev  map[uint32]counter
	prevT time.Time
	flows map[flowKey]*flow
}

// New crea un monitor vacío.
func New() *Monitor {
	return &Monitor{procs: map[uint32]*counter{}, prev: map[uint32]counter{}, flows: map[flowKey]*flow{}, prevT: time.Now()}
}

// Handle procesa un evento de Kernel-Network (se llama desde el hilo de ETW).
func (m *Monitor) Handle(r *etw.Record) {
	id := r.Header.ID
	var v6, udp, send bool
	switch id {
	case 10:
		send = true
	case 11:
	case 26:
		v6, send = true, true
	case 27:
		v6 = true
	case 42:
		udp, send = true, true
	case 43:
		udp = true
	case 58:
		v6, udp, send = true, true, true
	case 59:
		v6, udp = true, true
	default:
		return
	}
	rd := etw.NewReader(r.Data())
	pid := rd.U32()
	size := uint64(rd.U32())
	var daddr, saddr netip.Addr
	if v6 {
		daddr = netip.AddrFrom16([16]byte(rd.Bytes(16)))
		saddr = netip.AddrFrom16([16]byte(rd.Bytes(16)))
	} else {
		daddr = netip.AddrFrom4([4]byte(rd.Bytes(4)))
		saddr = netip.AddrFrom4([4]byte(rd.Bytes(4)))
	}
	dport := rd.U16BE()
	sport := rd.U16BE()
	if !rd.OK() || pid == 0 {
		return
	}
	// Al enviar, el otro lado es el destino; al recibir, el origen.
	remote := netip.AddrPortFrom(daddr, dport)
	if !send {
		remote = netip.AddrPortFrom(saddr, sport)
	}
	m.mu.Lock()
	c := m.procs[pid]
	if c == nil {
		c = &counter{}
		m.procs[pid] = c
	}
	if send {
		c.tx += size
	} else {
		c.rx += size
	}
	k := flowKey{pid: pid, remote: remote, udp: udp}
	f := m.flows[k]
	if f == nil {
		if len(m.flows) > 4096 {
			m.flows = map[flowKey]*flow{}
		}
		f = &flow{}
		m.flows[k] = f
	}
	f.packets++
	f.bytes += size
	f.last = time.Now()
	m.mu.Unlock()
}

// Rate es lo que mueve un proceso por segundo.
type Rate struct {
	PID  uint32  `json:"pid"`
	Down float64 `json:"down"` // bytes/s
	Up   float64 `json:"up"`
}

// Rates devuelve lo que ha movido cada proceso desde la última llamada.
func (m *Monitor) Rates() []Rate {
	m.mu.Lock()
	defer m.mu.Unlock()
	dt := time.Since(m.prevT).Seconds()
	m.prevT = time.Now()
	if dt <= 0 {
		dt = 1
	}
	var out []Rate
	for pid, c := range m.procs {
		p := m.prev[pid]
		if c.rx > p.rx || c.tx > p.tx {
			out = append(out, Rate{PID: pid, Down: float64(c.rx-p.rx) / dt, Up: float64(c.tx-p.tx) / dt})
		}
		m.prev[pid] = *c
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Down+out[i].Up > out[j].Down+out[j].Up })
	return out
}

// Totals devuelve lo descargado y subido por cada proceso desde que se empezó.
func (m *Monitor) Totals() map[uint32][2]uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[uint32][2]uint64, len(m.procs))
	for pid, c := range m.procs {
		out[pid] = [2]uint64{c.rx, c.tx}
	}
	return out
}

// Server es el servidor con el que habla un juego.
type Server struct {
	Addr netip.AddrPort
	UDP  bool
}

// GameServer adivina el servidor del juego: el destino con más paquetes en los
// últimos 10 s (mejor UDP, que es lo que usan los juegos para jugar), fuera de
// la red de casa y de los puertos web.
func (m *Monitor) GameServer(pid uint32) (Server, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var best Server
	bestScore := uint64(0)
	cut := time.Now().Add(-10 * time.Second)
	for k, f := range m.flows {
		if k.pid != pid || f.last.Before(cut) {
			continue
		}
		a := k.remote.Addr()
		if !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() {
			continue
		}
		port := k.remote.Port()
		if port == 53 || port == 80 || port == 443 && !k.udp || port == 5353 || port == 1900 {
			continue
		}
		score := f.packets
		if k.udp {
			score *= 4
		}
		if score > bestScore {
			best, bestScore = Server{Addr: k.remote, UDP: k.udp}, score
		}
	}
	return best, bestScore >= 20
}

// Forget borra lo de procesos que ya no existen (o todo, con pid 0).
func (m *Monitor) Forget(alive func(uint32) bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for pid := range m.procs {
		if !alive(pid) {
			delete(m.procs, pid)
			delete(m.prev, pid)
		}
	}
	cut := time.Now().Add(-2 * time.Minute)
	for k, f := range m.flows {
		if f.last.Before(cut) {
			delete(m.flows, k)
		}
	}
}
