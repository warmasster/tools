// Package stability busca núcleos que «se equivocan» (como los que describió
// Google en «Cores that don't count») y errores de la RAM: cada hilo hace los
// mismos cálculos con resultado conocido, primero uno a uno (a la frecuencia
// máxima de un núcleo, donde fallan los undervolt) y luego todos a la vez (con
// calor y la tensión más baja), y se comparan. Un núcleo estable da siempre lo
// mismo que los demás.
package stability

import (
	"crypto/aes"
	"crypto/sha256"
	"hash/crc32"
	"math"
	"math/bits"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modKernel         = windows.NewLazySystemDLL("kernel32.dll")
	pSetThreadAffMask = modKernel.NewProc("SetThreadAffinityMask")
	pGetCurrentThread = modKernel.NewProc("GetCurrentThread")
)

// CoreResult es lo que dio un hilo.
type CoreResult struct {
	Logical int `json:"logical"`
	Errors  int `json:"errors"`
	Items   int `json:"items"`
}

// State es cómo va (o cómo acabó) la prueba.
type State struct {
	Running   bool         `json:"running"`
	Phase     string       `json:"phase"` // single, all, ram, done
	Pct       float64      `json:"pct"`
	Current   int          `json:"current"` // hilo que se está probando
	Cores     []CoreResult `json:"cores"`
	RAMMB     int          `json:"ramMB"`
	RAMErrors int          `json:"ramErrors"`
	Started   int64        `json:"started"`
	Finished  int64        `json:"finished"`
	Stopped   bool         `json:"stopped"`
	Error     string       `json:"error,omitempty"`
}

// Test es una prueba en marcha.
type Test struct {
	mu    sync.Mutex
	st    State
	stop  atomic.Bool
	ref   []uint64 // resultado correcto de cada cálculo (por votación)
	items int
}

// Get devuelve una copia del estado.
func (t *Test) Get() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.st
	s.Cores = append([]CoreResult(nil), t.st.Cores...)
	return s
}

// Stop pide que pare.
func (t *Test) Stop() { t.stop.Store(true) }

func pin(logical int) {
	runtime.LockOSThread()
	if logical < 64 {
		h, _, _ := pGetCurrentThread.Call()
		pSetThreadAffMask.Call(h, uintptr(1)<<uint(logical))
	}
}

// work es un cálculo determinista que toca la ALU entera, la de coma flotante
// (FMA), y las unidades de vector (AES, SHA y CRC con instrucciones propias).
func work(seed uint64, scratch []byte) uint64 {
	x := seed*0x9E3779B97F4A7C15 + 1
	var acc uint64
	f := float64(seed%1000) + 0.5
	for i := 0; i < 2000; i++ {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		hi, lo := bits.Mul64(x, 0xD1B54A32D192ED03)
		acc += hi ^ bits.RotateLeft64(lo, int(i&63))
		if x%97 == 0 {
			acc ^= x / (uint64(i) | 1)
		}
		f = math.FMA(f, 1.0000001, float64(i&7)*0.25)
		if f > 1e12 {
			f = math.Sqrt(f)
		}
	}
	acc ^= math.Float64bits(f)
	for i := range scratch {
		scratch[i] = byte(x >> (uint(i) & 56))
		x = x*6364136223846793005 + 1442695040888963407
	}
	sum := sha256.Sum256(scratch)
	acc ^= *(*uint64)(unsafe.Pointer(&sum[0]))
	acc += uint64(crc32.Checksum(scratch, crc32.MakeTable(crc32.Castagnoli)))
	blk, _ := aes.NewCipher(sum[:16])
	var out [16]byte
	for i := 0; i+16 <= len(scratch) && i < 1024; i += 16 {
		blk.Encrypt(out[:], scratch[i:i+16])
		acc ^= *(*uint64)(unsafe.Pointer(&out[0]))
	}
	return acc
}

// Options de la prueba.
type Options struct {
	PerThread time.Duration // tiempo por hilo en la fase de uno en uno
	AllCores  time.Duration // fase de todos a la vez
	RAMMB     int           // RAM a probar (0 = nada)
	Threads   int
}

// Start lanza la prueba en segundo plano.
func Start(o Options) *Test {
	if o.Threads <= 0 {
		o.Threads = runtime.NumCPU()
	}
	if o.Threads > 64 {
		o.Threads = 64
	}
	t := &Test{}
	t.st = State{Running: true, Phase: "single", Started: time.Now().Unix(), RAMMB: o.RAMMB}
	for i := 0; i < o.Threads; i++ {
		t.st.Cores = append(t.st.Cores, CoreResult{Logical: i})
	}
	go t.run(o)
	return t
}

func (t *Test) set(f func(s *State)) {
	t.mu.Lock()
	f(&t.st)
	t.mu.Unlock()
}

func (t *Test) run(o Options) {
	defer func() {
		if r := recover(); r != nil {
			t.set(func(s *State) { s.Error = "la prueba falló por dentro" })
		}
		t.set(func(s *State) {
			s.Running = false
			s.Phase = "done"
			s.Pct = 100
			s.Finished = time.Now().Unix()
			s.Stopped = t.stop.Load()
		})
	}()
	// Referencia: el mismo cálculo en tres hilos distintos, y se queda el que coincida en dos.
	const items = 400
	t.items = items
	t.ref = make([]uint64, items)
	refs := make([][]uint64, 3)
	var wg sync.WaitGroup
	for r := 0; r < 3; r++ {
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			pin(r * o.Threads / 3)
			defer runtime.UnlockOSThread()
			buf := make([]byte, 64<<10)
			refs[r] = make([]uint64, items)
			for i := 0; i < items; i++ {
				refs[r][i] = work(uint64(i), buf)
			}
		}(r)
	}
	wg.Wait()
	for i := 0; i < items; i++ {
		a, b, c := refs[0][i], refs[1][i], refs[2][i]
		switch {
		case a == b || a == c:
			t.ref[i] = a
		case b == c:
			t.ref[i] = b
		default:
			t.ref[i] = a // ninguno coincide: algo va muy mal
		}
	}
	// Fase 1: hilo a hilo (el resto del procesador en reposo, así ese núcleo sube al máximo).
	total := float64(o.Threads)
	for c := 0; c < o.Threads && !t.stop.Load(); c++ {
		t.set(func(s *State) { s.Current = c })
		errs, n := t.runOn(c, o.PerThread)
		t.set(func(s *State) {
			s.Cores[c].Errors += errs
			s.Cores[c].Items += n
			s.Pct = float64(c+1) / total * 60
		})
	}
	// Fase 2: todos a la vez.
	if !t.stop.Load() && o.AllCores > 0 {
		t.set(func(s *State) { s.Phase = "all"; s.Current = -1 })
		var wg sync.WaitGroup
		for c := 0; c < o.Threads; c++ {
			wg.Add(1)
			go func(c int) {
				defer wg.Done()
				errs, n := t.runOn(c, o.AllCores)
				t.set(func(s *State) { s.Cores[c].Errors += errs; s.Cores[c].Items += n })
			}(c)
		}
		start := time.Now()
		for !waitTimeout(&wg, 500*time.Millisecond) {
			p := time.Since(start).Seconds() / o.AllCores.Seconds()
			t.set(func(s *State) { s.Pct = 60 + math.Min(p, 1)*25 })
		}
	}
	// Fase 3: RAM.
	if !t.stop.Load() && o.RAMMB > 0 {
		t.set(func(s *State) { s.Phase = "ram"; s.Pct = 85 })
		errs := t.ramTest(o.RAMMB, o.Threads, func(p float64) { t.set(func(s *State) { s.Pct = 85 + p*15 }) })
		t.set(func(s *State) { s.RAMErrors = errs })
	}
}

func waitTimeout(wg *sync.WaitGroup, d time.Duration) bool {
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}

// runOn hace cálculos en un hilo concreto durante d y cuenta los que no cuadran.
func (t *Test) runOn(logical int, d time.Duration) (errs, n int) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		pin(logical)
		defer runtime.UnlockOSThread()
		buf := make([]byte, 64<<10)
		end := time.Now().Add(d)
		for i := 0; time.Now().Before(end) && !t.stop.Load(); i++ {
			k := i % t.items
			if work(uint64(k), buf) != t.ref[k] {
				errs++
			}
			n++
		}
	}()
	<-done
	return
}

// ramTest escribe un patrón distinto en cada pasada y lo comprueba.
func (t *Test) ramTest(mb, threads int, progress func(float64)) int {
	size := uintptr(mb) << 20
	p, err := windows.VirtualAlloc(0, size, windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_READWRITE)
	if err != nil {
		return 0
	}
	defer windows.VirtualFree(p, 0, windows.MEM_RELEASE)
	words := unsafe.Slice((*uint64)(*(*unsafe.Pointer)(unsafe.Pointer(&p))), size/8)
	var errs atomic.Int64
	const passes = 4
	for pass := 0; pass < passes && !t.stop.Load(); pass++ {
		key := uint64(pass)*0xA5A5A5A5A5A5A5A5 + 0x0123456789ABCDEF
		for phase := 0; phase < 2; phase++ { // escribir y luego comprobar
			var wg sync.WaitGroup
			chunk := len(words) / threads
			for w := 0; w < threads; w++ {
				wg.Add(1)
				go func(w int) {
					defer wg.Done()
					lo, hi := w*chunk, (w+1)*chunk
					if w == threads-1 {
						hi = len(words)
					}
					for i := lo; i < hi; i++ {
						v := uint64(i)*0x9E3779B97F4A7C15 ^ key
						if pass&1 == 1 {
							v = ^v
						}
						if phase == 0 {
							words[i] = v
						} else if words[i] != v {
							errs.Add(1)
						}
					}
				}(w)
			}
			wg.Wait()
			progress(float64(pass*2+phase+1) / float64(passes*2))
		}
	}
	return int(errs.Load())
}
