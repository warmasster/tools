package fps

import (
	"math"
	"sort"
	"sync"
)

// ringSize: fotogramas que se recuerdan por proceso (10 s a 400 FPS).
const ringSize = 4096

// Tracker cuenta los fotogramas de cada proceso. Es seguro usarlo desde varios hilos.
type Tracker struct {
	mu    sync.Mutex
	freq  float64 // ticks por segundo de las marcas de tiempo
	procs map[uint32]*proc
	tmp   []float64
}

type proc struct {
	ring      [ringSize]int64
	n         int // fotogramas guardados (hasta ringSize)
	head      int // siguiente posición
	last      int64
	api       bool  // presenta con DXGI o D3D9 (más fiable que el núcleo)
	lastAPI   int64 // último fotograma de DXGI/D3D9
	collected int64 // hasta dónde se han entregado los tiempos a Collect
}

// NewTracker: freq es la frecuencia de las marcas de tiempo (QueryPerformanceFrequency).
func NewTracker(freq int64) *Tracker {
	if freq <= 0 {
		freq = 10_000_000
	}
	return &Tracker{freq: float64(freq), procs: map[uint32]*proc{}}
}

// Present apunta un fotograma.
func (t *Tracker) Present(pid uint32, ts int64, src int) {
	t.mu.Lock()
	p := t.procs[pid]
	if p == nil {
		p = &proc{}
		t.procs[pid] = p
	}
	if src == SrcKernel {
		// Los juegos de DirectX generan los dos tipos de evento: si ya hay de
		// DXGI/D3D9 hace poco, los del núcleo sobran (contarían doble).
		if p.api && float64(ts-p.lastAPI) < 2*t.freq {
			t.mu.Unlock()
			return
		}
		p.api = false
	} else {
		if !p.api {
			// Pasa a contar por DXGI: se olvida lo del núcleo para no mezclar.
			p.n, p.head = 0, 0
		}
		p.api = true
		p.lastAPI = ts
	}
	if ts <= p.last { // duplicado o desordenado
		t.mu.Unlock()
		return
	}
	p.ring[p.head] = ts
	p.head = (p.head + 1) % ringSize
	if p.n < ringSize {
		p.n++
	}
	p.last = ts
	t.mu.Unlock()
}

// Stats es la medida de un proceso.
type Stats struct {
	FPS        float64 `json:"fps"`        // último segundo
	Low1       float64 `json:"low1"`       // FPS del 1 % de fotogramas más lentos (últimos 10 s)
	FrameMs    float64 `json:"frameMs"`    // tiempo medio por fotograma (último segundo)
	MaxMs      float64 `json:"maxMs"`      // el fotograma más lento de los últimos 10 s
	Presenting bool    `json:"presenting"` // presenta fotogramas ahora mismo
}

// at devuelve el fotograma i-ésimo empezando por el más reciente (0).
func (p *proc) at(i int) int64 {
	k := p.head - 1 - i
	for k < 0 {
		k += ringSize
	}
	return p.ring[k]
}

// Stats mide un proceso. now es la marca de tiempo actual (mismo reloj que los eventos).
func (t *Tracker) Stats(pid uint32, now int64) Stats {
	t.mu.Lock()
	defer t.mu.Unlock()
	var st Stats
	p := t.procs[pid]
	if p == nil || p.n < 2 {
		return st
	}
	// Los eventos llegan con hasta ~1 s de retraso (Windows los manda por tandas):
	// se mide el último segundo que se ha recibido entero, y si el proceso lleva
	// más de 2,5 s sin fotogramas, es que ha dejado de presentar.
	if float64(now-p.last) > 2.5*t.freq {
		return st
	}
	st.Presenting = true
	end := p.last
	sec := int64(t.freq)
	count := 0
	for i := 0; i < p.n; i++ {
		if end-p.at(i) >= sec {
			break
		}
		count++
	}
	if count >= 2 {
		span := float64(p.at(0)-p.at(count-1)) / t.freq
		if span > 0 {
			st.FPS = float64(count-1) / span
			st.FrameMs = span * 1000 / float64(count-1)
		}
	}
	// 1 % más lento de los últimos 10 s.
	t.tmp = t.tmp[:0]
	for i := 0; i+1 < p.n; i++ {
		if end-p.at(i+1) > 10*sec {
			break
		}
		t.tmp = append(t.tmp, float64(p.at(i)-p.at(i+1))*1000/t.freq)
	}
	if len(t.tmp) >= 10 {
		st.Low1, st.MaxMs = lowFPS(t.tmp, 0.01)
	}
	st.FPS = round1(st.FPS)
	st.FrameMs = round2(st.FrameMs)
	return st
}

// lowFPS: FPS medios del peor «frac» de los fotogramas, y el peor tiempo en ms.
// Ordena ms (se modifica).
func lowFPS(ms []float64, frac float64) (float64, float64) {
	sort.Float64s(ms)
	k := int(math.Ceil(float64(len(ms)) * frac))
	if k < 1 {
		k = 1
	}
	sum := 0.0
	for _, v := range ms[len(ms)-k:] {
		sum += v
	}
	avg := sum / float64(k)
	if avg <= 0 {
		return 0, 0
	}
	return round1(1000 / avg), round2(ms[len(ms)-1])
}

// Collect devuelve los tiempos de fotograma (ms) nuevos de un proceso desde la
// última llamada. Lo usa el registro de partidas.
func (t *Tracker) Collect(pid uint32, dst []float32) []float32 {
	dst, _ = t.CollectTS(pid, dst, nil)
	return dst
}

// CollectTS es Collect y además devuelve cuándo acabó cada fotograma (si ts no es nil).
func (t *Tracker) CollectTS(pid uint32, dst []float32, ts []int64) ([]float32, []int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	p := t.procs[pid]
	if p == nil || p.n < 2 {
		return dst, ts
	}
	// Desde el más antiguo pendiente hasta el último.
	start := p.n - 1
	for i := 0; i < p.n; i++ {
		if p.at(i) <= p.collected {
			start = i // el último ya entregado: el siguiente intervalo empieza en él
			break
		}
	}
	for i := start; i >= 1; i-- {
		d := float64(p.at(i-1)-p.at(i)) * 1000 / t.freq
		if d > 0 && d < 5000 {
			dst = append(dst, float32(d))
			if ts != nil {
				ts = append(ts, p.at(i-1))
			}
		}
	}
	p.collected = p.last
	return dst, ts
}

// Active devuelve los procesos que han presentado algo en los últimos 2,5 s.
func (t *Tracker) Active(now int64) []uint32 {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []uint32
	for pid, p := range t.procs {
		if float64(now-p.last) <= 2.5*t.freq {
			out = append(out, pid)
		}
	}
	return out
}

// Prune olvida los procesos que llevan más de 30 s sin presentar.
func (t *Tracker) Prune(now int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for pid, p := range t.procs {
		if float64(now-p.last) > 30*t.freq {
			delete(t.procs, pid)
		}
	}
}

// ---------- histograma de una partida ----------

// Hist acumula los tiempos de fotograma de una partida en casillas de 0,1 ms
// (hasta 250 ms): ocupa 10 KB sea cual sea la duración.
type Hist struct {
	Bins  [2500]uint32
	Over  uint32 // más de 250 ms
	Count uint64
	SumMs float64
}

// Add suma tiempos de fotograma (ms).
func (h *Hist) Add(ms []float32) {
	for _, v := range ms {
		i := int(v * 10)
		if i < 0 {
			continue
		}
		if i >= len(h.Bins) {
			h.Over++
		} else {
			h.Bins[i]++
		}
		h.Count++
		h.SumMs += float64(v)
	}
}

// AvgFPS: fotogramas totales entre tiempo total.
func (h *Hist) AvgFPS() float64 {
	if h.SumMs <= 0 {
		return 0
	}
	return round1(float64(h.Count) * 1000 / h.SumMs)
}

// Low devuelve los FPS medios del peor «frac» de los fotogramas (0,01 = 1 % bajo).
func (h *Hist) Low(frac float64) float64 {
	if h.Count == 0 {
		return 0
	}
	k := uint64(math.Ceil(float64(h.Count) * frac))
	if k < 1 {
		k = 1
	}
	var taken uint64
	sum := 0.0
	if h.Over > 0 {
		n := uint64(h.Over)
		if n > k {
			n = k
		}
		taken += n
		sum += float64(n) * 250
	}
	for i := len(h.Bins) - 1; i >= 0 && taken < k; i-- {
		n := uint64(h.Bins[i])
		if n == 0 {
			continue
		}
		if taken+n > k {
			n = k - taken
		}
		taken += n
		sum += float64(n) * (float64(i) + 0.5) / 10
	}
	if sum <= 0 {
		return 0
	}
	return round1(1000 / (sum / float64(taken)))
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round2(v float64) float64 { return math.Round(v*100) / 100 }
