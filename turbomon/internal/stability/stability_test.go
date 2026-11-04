package stability

import (
	"testing"
	"time"
)

func TestWorkDeterministic(t *testing.T) {
	buf := make([]byte, 64<<10)
	a := work(42, buf)
	b := work(42, make([]byte, 64<<10))
	if a != b || a == work(43, buf) {
		t.Fatal("el cálculo no es determinista o no depende de la semilla")
	}
}

// Una prueba cortita de verdad (2 hilos, sin RAM).
func TestShortRun(t *testing.T) {
	if testing.Short() {
		t.Skip("prueba corta")
	}
	tt := Start(Options{PerThread: 200 * time.Millisecond, AllCores: 300 * time.Millisecond, RAMMB: 32, Threads: 2})
	for tt.Get().Running {
		time.Sleep(50 * time.Millisecond)
	}
	s := tt.Get()
	t.Logf("%+v", s)
	for _, c := range s.Cores {
		if c.Items == 0 || c.Errors != 0 {
			t.Fatalf("núcleo %d: %+v", c.Logical, c)
		}
	}
	if s.RAMErrors != 0 {
		t.Fatal("errores de RAM")
	}
}
