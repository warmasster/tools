package procstat

import (
	"os"
	"runtime"
	"testing"
	"time"
)

func TestSample(t *testing.T) {
	s := New(runtime.NumCPU())
	s.Sample()
	// Gasta algo de procesador para salir en la lista.
	end := time.Now().Add(300 * time.Millisecond)
	x := 1
	for time.Now().Before(end) {
		x = x*31 + 7
	}
	_ = x
	start := time.Now()
	list := s.Sample()
	t.Logf("lectura en %v, %d procesos con gasto", time.Since(start), len(list))
	found := false
	for _, u := range list {
		if u.PID == uint32(os.Getpid()) {
			found = true
			t.Logf("este proceso: %+v", u)
			if u.CPU <= 0 {
				t.Fatal("sin procesador")
			}
		}
	}
	if !found {
		t.Fatal("no sale este proceso")
	}
	for i, u := range list {
		if i < 5 {
			t.Logf("%+v", u)
		}
	}
}
