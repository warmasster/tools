package fps

import (
	"errors"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func elevated() bool { return windows.GetCurrentProcessToken().IsElevated() }

// Sin permiso de administrador, Start tiene que decirlo claro (y no dejar nada abierto).
func TestStartWithoutAdmin(t *testing.T) {
	if elevated() {
		t.Skip("este proceso es administrador")
	}
	_, err := Start(func(uint32, int64, int) {})
	if !errors.Is(err, ErrNeedAdmin) {
		t.Fatalf("esperaba ErrNeedAdmin, llega %v", err)
	}
}

// Prueba de verdad (como administrador): escucha 4 s y enseña quién presenta.
//
//	$env:TURBOMON_REAL="1"; go test -run RealETW -v ./internal/fps/
func TestRealETW(t *testing.T) {
	if os.Getenv("TURBOMON_REAL") == "" {
		t.Skip("TURBOMON_REAL no está puesto")
	}
	if !elevated() {
		t.Skip("hace falta ser administrador")
	}
	tr := NewTracker(Freq())
	var mu sync.Mutex
	counts := map[[2]uint32]int{}
	s, err := Start(func(pid uint32, ts int64, src int) {
		tr.Present(pid, ts, src)
		mu.Lock()
		counts[[2]uint32{pid, uint32(src)}]++
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(4 * time.Second)
	now := Now()
	type row struct {
		pid uint32
		st  Stats
	}
	var rows []row
	for _, pid := range tr.Active(now) {
		rows = append(rows, row{pid, tr.Stats(pid, now)})
	}
	lost := s.EventsLost()
	s.Stop()
	sort.Slice(rows, func(i, j int) bool { return rows[i].st.FPS > rows[j].st.FPS })
	mu.Lock()
	t.Logf("eventos por proceso y fuente: %v", counts)
	mu.Unlock()
	for _, r := range rows {
		t.Logf("pid %d: %+v", r.pid, r.st)
	}
	t.Logf("eventos perdidos: %d", lost)
	if len(counts) == 0 {
		t.Fatal("no ha llegado ningún fotograma (¿no hay nada moviéndose en pantalla?)")
	}
}
