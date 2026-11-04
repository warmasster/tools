package netmon

// Prueba de «bufferbloat»: el ping con la conexión en reposo, descargando a
// tope y subiendo a tope. Si sube mucho, el router (o el del operador) guarda
// demasiados paquetes en cola y cualquier descarga te arruina la partida.

import (
	"context"
	"io"
	"math"
	"net/http"
	"net/netip"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// BloatState es cómo va la prueba.
type BloatState struct {
	Running  bool    `json:"running"`
	Phase    string  `json:"phase"` // idle, down, up, done
	Pct      float64 `json:"pct"`
	Idle     float64 `json:"idle"` // ping en reposo (ms, mediana)
	Down     float64 `json:"down"` // ping descargando
	Up       float64 `json:"up"`   // ping subiendo
	DownMbps float64 `json:"downMbps"`
	UpMbps   float64 `json:"upMbps"`
	Grade    string  `json:"grade"` // A+ … F
	Error    string  `json:"error,omitempty"`
	Live     float64 `json:"live"` // último ping
	At       int64   `json:"at"`
}

// Bloat es una prueba.
type Bloat struct {
	mu sync.Mutex
	st BloatState
}

// Get devuelve el estado.
func (b *Bloat) Get() BloatState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.st
}

func (b *Bloat) set(f func(*BloatState)) {
	b.mu.Lock()
	f(&b.st)
	b.mu.Unlock()
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return math.Round(s[len(s)/2]*10) / 10
}

func grade(added float64) string {
	switch {
	case added < 5:
		return "A+"
	case added < 30:
		return "A"
	case added < 60:
		return "B"
	case added < 200:
		return "C"
	case added < 400:
		return "D"
	}
	return "F"
}

// StartBloat lanza la prueba (unos 25 s). target es a quién se hace ping.
func StartBloat(target netip.Addr) *Bloat {
	b := &Bloat{st: BloatState{Running: true, Phase: "idle"}}
	go b.run(target)
	return b
}

const (
	downURL = "https://speed.cloudflare.com/__down?bytes=100000000"
	upURL   = "https://speed.cloudflare.com/__up"
)

func (b *Bloat) pings(target netip.Addr, d time.Duration, every time.Duration, base float64, span float64) []float64 {
	var out []float64
	end := time.Now().Add(d)
	start := time.Now()
	for time.Now().Before(end) {
		t0 := time.Now()
		if rtt, err := Ping(target, time.Second); err == nil {
			ms := float64(rtt.Microseconds()) / 1000
			out = append(out, ms)
			b.set(func(s *BloatState) { s.Live = math.Round(ms*10) / 10 })
		}
		p := base + span*time.Since(start).Seconds()/d.Seconds()
		b.set(func(s *BloatState) { s.Pct = math.Min(p, 100) })
		if w := every - time.Since(t0); w > 0 {
			time.Sleep(w)
		}
	}
	return out
}

// load mete carga (descarga o subida) con 4 conexiones mientras dura ctx.
func load(ctx context.Context, upload bool, moved *atomic.Int64) {
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				var req *http.Request
				if upload {
					req, _ = http.NewRequestWithContext(ctx, "POST", upURL, &countingReader{ctx: ctx, moved: moved, left: 50 << 20})
					req.ContentLength = 50 << 20
				} else {
					req, _ = http.NewRequestWithContext(ctx, "GET", downURL, nil)
				}
				req.Header.Set("User-Agent", "TurboMON")
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					time.Sleep(200 * time.Millisecond)
					continue
				}
				buf := make([]byte, 64<<10)
				for {
					n, err := resp.Body.Read(buf)
					if !upload {
						moved.Add(int64(n))
					}
					if err != nil {
						break
					}
				}
				resp.Body.Close()
			}
		}()
	}
	wg.Wait()
}

type countingReader struct {
	ctx   context.Context
	moved *atomic.Int64
	left  int
}

var zeros = make([]byte, 64<<10)

func (c *countingReader) Read(p []byte) (int, error) {
	if c.ctx.Err() != nil || c.left <= 0 {
		return 0, io.EOF
	}
	n := len(p)
	if n > c.left {
		n = c.left
	}
	if n > len(zeros) {
		n = len(zeros)
	}
	copy(p, zeros[:n])
	c.left -= n
	c.moved.Add(int64(n))
	return n, nil
}

func (b *Bloat) run(target netip.Addr) {
	defer b.set(func(s *BloatState) { s.Running = false; s.Phase = "done"; s.Pct = 100; s.At = time.Now().Unix() })
	idle := b.pings(target, 5*time.Second, 250*time.Millisecond, 0, 20)
	if len(idle) < 5 {
		b.set(func(s *BloatState) { s.Error = "no hay respuesta al ping: ¿hay internet?" })
		return
	}
	b.set(func(s *BloatState) { s.Idle = median(idle); s.Phase = "down" })
	for _, up := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		var moved atomic.Int64
		go load(ctx, up, &moved)
		time.Sleep(1500 * time.Millisecond) // que arranque la carga
		start := moved.Load()
		t0 := time.Now()
		base := 20.0
		if up {
			base = 60
		}
		p := b.pings(target, 8*time.Second, 250*time.Millisecond, base, 40)
		mbps := float64(moved.Load()-start) * 8 / 1e6 / time.Since(t0).Seconds()
		cancel()
		if up {
			b.set(func(s *BloatState) { s.Up = median(p); s.UpMbps = math.Round(mbps) })
		} else {
			b.set(func(s *BloatState) { s.Down = median(p); s.DownMbps = math.Round(mbps); s.Phase = "up" })
		}
		time.Sleep(500 * time.Millisecond)
	}
	b.set(func(s *BloatState) {
		worst := math.Max(s.Down, s.Up) - s.Idle
		s.Grade = grade(math.Max(0, worst))
	})
}
