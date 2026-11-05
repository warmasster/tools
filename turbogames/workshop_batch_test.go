package main

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"turbogames/internal/steamworks"
)

// Steam de mentira para el proceso del Workshop: no habla con Steam. Cada
// suscripción o baja tarda 20 ms; los múltiplos de 37 fallan la primera vez
// (como un corte pasajero) y el 250 siempre.
type fakeWS struct {
	mu    sync.Mutex
	subs  map[uint64]bool
	tries map[uint64]int
}

func openFakeWS(string, uint32) (wsClient, error) {
	f := &fakeWS{subs: map[uint64]bool{}, tries: map[uint64]int{}}
	for id := uint64(200); id < 400; id++ {
		f.subs[id] = true
	}
	return f, nil
}

func (f *fakeWS) Close() {}
func (f *fakeWS) Subscribed() []uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []uint64
	for id := range f.subs {
		out = append(out, id)
	}
	return out
}
func (f *fakeWS) State(id uint64) steamworks.ItemState { return steamworks.ItemState{ID: id, Flags: 1} }
func (f *fakeWS) Details(ids []uint64) ([]steamworks.Item, error) {
	return nil, errors.New("sin datos")
}
func (f *fakeWS) Subscribe(id uint64) error   { return nil }
func (f *fakeWS) Unsubscribe(id uint64) error { return nil }
func (f *fakeWS) Search(string, int, string, []string) (steamworks.Page, error) {
	return steamworks.Page{}, nil
}
func (f *fakeWS) Batch(sub bool, ids []uint64, window int, done func(uint64, error), stop func() bool) {
	for i := 0; i < len(ids); i += window {
		if stop() {
			return
		}
		end := min(i+window, len(ids))
		time.Sleep(20 * time.Millisecond) // una tanda de «window» a la vez
		for _, id := range ids[i:end] {
			f.mu.Lock()
			f.tries[id]++
			try := f.tries[id]
			f.mu.Unlock()
			switch {
			case id == 250:
				done(id, errors.New("Steam ha dicho que no (código 2)"))
			case id%37 == 0 && try == 1:
				done(id, errors.New("Steam no ha respondido a tiempo"))
			default:
				f.mu.Lock()
				f.subs[id] = sub
				f.mu.Unlock()
				done(id, nil)
			}
		}
	}
}

// Prueba el camino entero (proceso aparte, tandas, progreso, reintentos y
// parar) con el Steam de mentira. Solo lee el estado del Steam real (tiene que
// estar abierto con una cuenta y sin juegos).
func TestModsChangeBatch(t *testing.T) {
	dir := t.TempDir()
	a := &App{settings: defaultSettings(), uaCache: map[uint32]uaEntry{}, wcache: loadWsCache(filepath.Join(dir, "w.json")), presets: loadPresetStore(filepath.Join(dir, "p.json")), gmeta: loadGameMetaStore(filepath.Join(dir, "j.json")), metas: loadMetaStore(filepath.Join(dir, "c.json")), mstore: loadModStore(filepath.Join(dir, "m.json"))}
	a.locate()
	in := a.steam()
	if in == nil {
		t.Skip("sin Steam")
	}
	if st := in.ReadStatus(); !st.Running || st.ActiveUser == 0 || st.RunningApp != 0 {
		t.Skip("hace falta Steam abierto con una cuenta y sin juegos")
	}
	t.Setenv("TURBOGAMES_FAKE_WS", "1")
	old := webDetailsFn
	webDetailsFn = func(ids []uint64) ([]steamworks.Item, error) { return nil, errors.New("sin red") }
	defer func() { webDetailsFn = old }()
	defer a.stopWorkshop()

	var sub, unsub []uint64
	for id := uint64(100); id < 150; id++ {
		sub = append(sub, id)
	}
	for id := uint64(200); id < 400; id++ {
		unsub = append(unsub, id)
	}
	start := time.Now()
	r, err := a.ModsChange(1, sub, unsub)
	took := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("250 mods en %v: %+v", took, r)
	if r.Subscribed != 50 || r.Unsubscribed != 199 || len(r.Failed) != 1 || r.Failed[0].ID != 250 || r.Canceled || r.Backup == "" {
		t.Fatalf("%+v", r)
	}
	if took > 5*time.Second {
		t.Errorf("demasiado lento: %v", took)
	}

	// parar a medias
	go func() { time.Sleep(150 * time.Millisecond); a.ModsCancel() }()
	r, err = a.ModsChange(1, nil, unsub)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("parado: %+v", r)
	if !r.Canceled || r.Unsubscribed+len(r.Failed) >= len(unsub) {
		t.Fatalf("no ha parado: %+v", r)
	}
	_ = os.Getenv
}
