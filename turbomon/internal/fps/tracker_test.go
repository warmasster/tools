package fps

import (
	"math"
	"testing"
)

const freq = 10_000_000 // marcas de 100 ns

// Un juego a 100 FPS con un tirón de 50 ms cada segundo.
func feed(t *Tracker, pid uint32, secs int, src int) int64 {
	ts := int64(1000)
	for s := 0; s < secs; s++ {
		for f := 0; f < 100; f++ {
			step := int64(freq / 100)
			if f == 50 {
				step = freq / 20
			}
			ts += step
			t.Present(pid, ts, src)
		}
	}
	return ts
}

func TestStats(t *testing.T) {
	tr := NewTracker(freq)
	end := feed(tr, 7, 12, SrcDXGI)
	st := tr.Stats(7, end+freq/2)
	if !st.Presenting || st.FPS < 80 || st.FPS > 100 {
		t.Fatalf("FPS raros: %+v", st)
	}
	// El 1 % más lento son los tirones de 50 ms → 20 FPS.
	if math.Abs(st.Low1-20) > 0.5 || math.Abs(st.MaxMs-50) > 0.1 {
		t.Fatalf("1 %% bajo: %+v", st)
	}
	if st := tr.Stats(7, end+3*freq); st.Presenting {
		t.Fatal("debería haber dejado de presentar")
	}
}

func TestKernelIgnoredWithDXGI(t *testing.T) {
	tr := NewTracker(freq)
	end := feed(tr, 9, 3, SrcDXGI)
	// Los mismos fotogramas vistos por el núcleo, un poco desplazados: no cuentan.
	for i := int64(0); i < 100; i++ {
		tr.Present(9, end-i*freq/100+freq/1000, SrcKernel)
	}
	st := tr.Stats(9, end)
	if st.FPS > 100 {
		t.Fatalf("cuenta doble: %+v", st)
	}
	// Un programa de Vulkan solo tiene eventos del núcleo.
	feed(tr, 11, 2, SrcKernel)
	if st := tr.Stats(11, end); st.FPS < 80 {
		t.Fatalf("Vulkan sin FPS: %+v", st)
	}
}

func TestCollectAndHist(t *testing.T) {
	tr := NewTracker(freq)
	feed(tr, 3, 1, SrcDXGI)
	a := tr.Collect(3, nil)
	feed2 := int64(0)
	// Segunda tanda, siguiendo donde acabó la primera.
	last := tr.procs[3].last
	for f := 0; f < 50; f++ {
		last += freq / 100
		tr.Present(3, last, SrcDXGI)
		feed2++
	}
	b := tr.Collect(3, nil)
	if len(a) != 99 || len(b) != 50 {
		t.Fatalf("collect: %d y %d tiempos", len(a), len(b))
	}
	var h Hist
	h.Add(a)
	h.Add(b)
	if avg := h.AvgFPS(); avg < 90 || avg > 100 {
		t.Fatalf("media %v", avg)
	}
	// 149 fotogramas: el 1 % son 2, el tirón de 50 ms y uno de 10 ms → 30 ms → 33,3 FPS.
	if low := h.Low(0.01); math.Abs(low-33.3) > 0.5 {
		t.Fatalf("1 %% bajo del histograma: %v", low)
	}
	if len(tr.Collect(3, nil)) != 0 {
		t.Fatal("sin fotogramas nuevos debería devolver nada")
	}
}
