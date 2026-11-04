package netmon

import (
	"errors"
	"math"
	"net/netip"
	"sort"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modIphlp     = windows.NewLazySystemDLL("iphlpapi.dll")
	pIcmpCreate  = modIphlp.NewProc("IcmpCreateFile")
	pIcmpClose   = modIphlp.NewProc("IcmpCloseHandle")
	pIcmpSend    = modIphlp.NewProc("IcmpSendEcho")
	pIcmp6Create = modIphlp.NewProc("Icmp6CreateFile")
	pIcmp6Send   = modIphlp.NewProc("Icmp6SendEcho2")
)

// ErrTimeout: no contesta.
var ErrTimeout = errors.New("sin respuesta")

// Ping manda un eco ICMP y devuelve el tiempo de ida y vuelta.
func Ping(addr netip.Addr, timeout time.Duration) (time.Duration, error) {
	if addr.Is4() || addr.Is4In6() {
		return ping4(addr.Unmap(), timeout)
	}
	return ping6(addr, timeout)
}

func ping4(addr netip.Addr, timeout time.Duration) (time.Duration, error) {
	h, _, _ := pIcmpCreate.Call()
	if h == 0 || h == ^uintptr(0) {
		return 0, errors.New("no se pudo abrir ICMP")
	}
	defer pIcmpClose.Call(h)
	ip := addr.As4()
	dst := uint32(ip[0]) | uint32(ip[1])<<8 | uint32(ip[2])<<16 | uint32(ip[3])<<24
	data := []byte("TurboMON-ping-32b")
	reply := make([]byte, 28+len(data)+8+64)
	start := time.Now()
	n, _, _ := pIcmpSend.Call(h, uintptr(dst), uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 0,
		uintptr(unsafe.Pointer(&reply[0])), uintptr(len(reply)), uintptr(timeout/time.Millisecond))
	el := time.Since(start)
	if n == 0 {
		return 0, ErrTimeout
	}
	status := *(*uint32)(unsafe.Pointer(&reply[4]))
	rtt := *(*uint32)(unsafe.Pointer(&reply[8]))
	if status != 0 {
		return 0, ErrTimeout
	}
	d := time.Duration(rtt) * time.Millisecond
	if rtt == 0 {
		d = el // menos de 1 ms: el reloj de alta resolución lo dice mejor
	}
	return d, nil
}

func ping6(addr netip.Addr, timeout time.Duration) (time.Duration, error) {
	h, _, _ := pIcmp6Create.Call()
	if h == 0 || h == ^uintptr(0) {
		return 0, errors.New("no se pudo abrir ICMPv6")
	}
	defer pIcmpClose.Call(h)
	var src, dst windows.RawSockaddrInet6
	src.Family = windows.AF_INET6
	dst.Family = windows.AF_INET6
	dst.Addr = addr.As16()
	data := []byte("TurboMON-ping-32b")
	reply := make([]byte, 64+len(data)+64)
	start := time.Now()
	n, _, _ := pIcmp6Send.Call(h, 0, 0, 0, uintptr(unsafe.Pointer(&src)), uintptr(unsafe.Pointer(&dst)), uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 0,
		uintptr(unsafe.Pointer(&reply[0])), uintptr(len(reply)), uintptr(timeout/time.Millisecond))
	if n == 0 {
		return 0, ErrTimeout
	}
	return time.Since(start), nil
}

// Series guarda pings para calcular media, saltos (jitter) y pérdidas.
type Series struct {
	mu    sync.Mutex
	vals  []float64 // ms; -1 = perdido
	limit int
}

// NewSeries guarda como mucho n medidas.
func NewSeries(n int) *Series { return &Series{limit: n} }

// Add apunta una medida (ms) o una pérdida (ok=false).
func (s *Series) Add(ms float64, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !ok {
		ms = -1
	}
	s.vals = append(s.vals, ms)
	if len(s.vals) > s.limit {
		s.vals = s.vals[len(s.vals)-s.limit:]
	}
}

// Stats es el resumen.
type Stats struct {
	Last   float64   `json:"last"`
	Avg    float64   `json:"avg"`
	P95    float64   `json:"p95"`
	Jitter float64   `json:"jitter"`
	Loss   float64   `json:"loss"` // %
	Count  int       `json:"count"`
	Hist   []float64 `json:"hist"`
}

// Stats resume las medidas.
func (s *Series) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Stats{Count: len(s.vals), Hist: append([]float64(nil), s.vals...)}
	var ok []float64
	lost := 0
	var jit, prev float64
	njit := 0
	for i, v := range s.vals {
		if v < 0 {
			lost++
			continue
		}
		ok = append(ok, v)
		if i > 0 && prev >= 0 && njit < len(s.vals) {
			jit += math.Abs(v - prev)
			njit++
		}
		prev = v
	}
	if len(s.vals) > 0 {
		st.Loss = math.Round(float64(lost)*1000/float64(len(s.vals))) / 10
		if s.vals[len(s.vals)-1] >= 0 {
			st.Last = s.vals[len(s.vals)-1]
		}
	}
	if len(ok) == 0 {
		return st
	}
	sum := 0.0
	for _, v := range ok {
		sum += v
	}
	st.Avg = math.Round(sum/float64(len(ok))*10) / 10
	sorted := append([]float64(nil), ok...)
	sort.Float64s(sorted)
	st.P95 = sorted[int(math.Min(float64(len(sorted)-1), math.Ceil(float64(len(sorted))*0.95)-1))]
	if njit > 0 {
		st.Jitter = math.Round(jit/float64(njit)*10) / 10
	}
	return st
}

// Reset borra las medidas.
func (s *Series) Reset() {
	s.mu.Lock()
	s.vals = nil
	s.mu.Unlock()
}
