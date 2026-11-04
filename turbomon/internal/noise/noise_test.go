package noise

import (
	"bytes"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 3 && os.Args[1] == "--noise-probe" {
		os.Exit(RunProbe(os.Args[2:], os.Stdout))
	}
	os.Exit(m.Run())
}

func TestProbeInProcess(t *testing.T) {
	var b bytes.Buffer
	if rc := RunProbe([]string{"200", "0,1"}, &b); rc != 0 {
		t.Fatalf("rc %d", rc)
	}
	t.Log(b.String())
	if bytes.Count(b.Bytes(), []byte("\n")) != 2 {
		t.Fatal("faltan líneas")
	}
}

func TestStart(t *testing.T) {
	if testing.Short() {
		t.Skip("tarda unos segundos")
	}
	exe, _ := os.Executable()
	var cores []LP
	for i := 0; i < runtime.NumCPU(); i++ {
		cores = append(cores, LP{ID: i, Phys: i / 2})
	}
	tt := Start(Options{Exe: exe, PerCore: 300 * time.Millisecond, Cores: cores, Deep: true})
	for tt.Get().Running {
		time.Sleep(200 * time.Millisecond)
	}
	s := tt.Get()
	if s.Error != "" {
		t.Fatal(s.Error)
	}
	t.Logf("deep=%v (%s) en %v s", s.Deep, s.DeepErr, s.Secs)
	for _, c := range s.Cores {
		t.Logf("núcleo %2d: robado %.3f %%, peor %6.0f µs, >100µs %.1f/s, >1ms %.1f/s, DPC %.2f %%, ISR %.2f %% [%s] %s %.0f", c.ID, c.Stolen, c.MaxUs, c.Per100, c.Per1000, c.DPC, c.ISR, c.Level, c.WorstDrv, c.WorstDrUs)
		if !c.Done {
			t.Errorf("núcleo %d sin medir", c.ID)
		}
	}
	for _, d := range s.Drivers {
		t.Logf("%-16s %-40s n=%d total %.2f ms, peor %.0f µs (>0,5 ms: %d) isr=%v", d.File, d.Desc, d.Count, d.TotalMs, d.MaxUs, d.Over500, d.ISR)
	}
}

// Con los drivers (como administrador):
//
//	$env:TURBOMON_REAL="1"; go test -run RealDeep -v ./internal/noise/
func TestRealDeep(t *testing.T) {
	if os.Getenv("TURBOMON_REAL") == "" {
		t.Skip("TURBOMON_REAL no está puesto")
	}
	exe, _ := os.Executable()
	var cores []LP
	for i := 0; i < runtime.NumCPU(); i++ {
		cores = append(cores, LP{ID: i, Phys: i / 2})
	}
	tt := Start(Options{Exe: exe, PerCore: 400 * time.Millisecond, Cores: cores, Deep: true})
	for tt.Get().Running {
		time.Sleep(200 * time.Millisecond)
	}
	s := tt.Get()
	if !s.Deep {
		t.Fatalf("sin drivers: %s", s.DeepErr)
	}
	known := 0
	for _, d := range s.Drivers {
		t.Logf("%-16s %-40s n=%d total %.2f ms, peor %.0f µs isr=%v", d.File, d.Desc, d.Count, d.TotalMs, d.MaxUs, d.ISR)
		if d.File != "desconocido" {
			known++
		}
	}
	for _, c := range s.Cores {
		t.Logf("hilo %2d: robado %.3f %%, peor %5.0f µs, DPC %.2f %% ISR %.2f %%, peor driver %s (%.0f µs)", c.ID, c.Stolen, c.MaxUs, c.DPC, c.ISR, c.WorstDrv, c.WorstDrUs)
	}
	if known == 0 {
		t.Fatal("no se ha sabido de qué driver es ninguna rutina")
	}
}
