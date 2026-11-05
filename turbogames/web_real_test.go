package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// Prueba con la web de Steam de verdad (solo lee páginas públicas):
//
//	$env:TURBOGAMES_WEB = "1"; go test -run WebReal -v .
func TestWebReal(t *testing.T) {
	if os.Getenv("TURBOGAMES_WEB") == "" {
		t.Skip("pon TURBOGAMES_WEB=1")
	}
	dir := t.TempDir()
	a := &App{wcache: loadWsCache(filepath.Join(dir, "w.json")), presets: loadPresetStore(filepath.Join(dir, "p.json")), gmeta: loadGameMetaStore(filepath.Join(dir, "j.json"))}
	items, err := webDetails([]uint64{1299328280, 450814997, 1})
	if err != nil || len(items) != 3 {
		t.Fatalf("%v %d", err, len(items))
	}
	for _, it := range items {
		t.Logf("%d app=%d %q tam=%d subs=%d img=%v falta=%v", it.ID, it.AppID, it.Title, it.Size, it.Subscriptions, it.Preview != "", it.Missing)
	}
	if items[0].Title == "" || items[1].AppID != 107410 || !items[2].Missing {
		t.Error("datos raros")
	}
	// una colección pública de Arma 3 (la oficial de Antistasi Community)
	cols, err := webCollections([]uint64{2867537125, 450814997})
	t.Logf("colecciones: %v %d", err, len(cols))
	for id, kids := range cols {
		t.Logf("  %d: %d mods", id, len(kids))
	}
	if _, err := a.ModInfo(108600, 1299328280, false); err != nil {
		t.Fatal(err)
	}
	d, err := a.ModInfo(108600, 3807334881, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("estrellas=%d valoraciones=%d comentarios=%d (%d leídos) requiere=%d galería=%d error=%q", d.Stars, d.Ratings, d.CommentsN, len(d.CommentList), len(d.Required), len(d.Gallery), d.PageError)
	for i, c := range d.CommentList {
		if i < 2 {
			t.Logf("  %s (%d): %.60q", c.Author, c.Time, c.Text)
		}
	}
	for _, r := range d.Required {
		t.Logf("  requiere %d %q", r.ID, r.Title)
	}
}

// TestMain deja que el propio binario de las pruebas haga de proceso del
// Workshop (lo lanza wsCall con --workshop, como hace TurboGAMES.exe).
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--workshop" {
		os.Exit(workshopChild(os.Args[2:]))
	}
	os.Exit(m.Run())
}

// Prueba de principio a fin con el Steam abierto, solo leyendo:
//
//	$env:TURBOGAMES_WORKSHOP = "<appid>"; go test -run WorkshopReal -v .
func TestWorkshopReal(t *testing.T) {
	v := os.Getenv("TURBOGAMES_WORKSHOP")
	if v == "" {
		t.Skip("pon TURBOGAMES_WORKSHOP con el appid de un juego")
	}
	appid64, _ := strconv.ParseUint(v, 10, 32)
	appid := uint32(appid64)
	dir := t.TempDir()
	a := &App{settings: defaultSettings(), uaCache: map[uint32]uaEntry{}, wcache: loadWsCache(filepath.Join(dir, "w.json")), presets: loadPresetStore(filepath.Join(dir, "p.json")), gmeta: loadGameMetaStore(filepath.Join(dir, "j.json")), metas: loadMetaStore(filepath.Join(dir, "c.json")), mstore: loadModStore(filepath.Join(dir, "m.json"))}
	a.locate()
	defer a.stopWorkshop()
	l, err := a.Mods(appid, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("live=%v error=%q cuenta=%s mods=%d disco=%d", l.Live, l.LiveError, l.Account, len(l.Mods), l.DiskSize)
	for i, m := range l.Mods {
		if i < 4 {
			t.Logf("  %q de %q suscrito=%v instalado=%v votos=%v %d/%d", m.Title, m.Author, m.Subscribed, m.Installed, m.HasVotes, m.VotesUp, m.VotesDown)
		}
	}
	if !l.Live {
		t.Fatal("no ha conectado con Steam")
	}
	if st := a.Status(); st.RunningApp != 0 {
		t.Errorf("el juego de mentira se ve como abierto: %d", st.RunningApp)
	}
	p, err := a.ModsSearch(appid, "", "votes", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("búsqueda: %d de %d", len(p.Mods), p.Total)
	if len(p.Mods) == 0 {
		t.Error("búsqueda vacía")
	}
	g, err := a.Games()
	if err != nil {
		t.Fatal(err)
	}
	nw, nc, n2 := 0, 0, 0
	for _, x := range g.Games {
		if x.Workshop {
			nw++
		}
		if len(x.Collections) > 0 {
			nc++
		}
		if x.Minutes2w > 0 {
			n2++
		}
	}
	t.Logf("juegos=%d con Workshop=%d en colecciones=%d jugados en 2 semanas=%d", len(g.Games), nw, nc, n2)
}

// La ficha con Steam conectado trae la galería de Steam.
func TestWorkshopRealInfo(t *testing.T) {
	v := os.Getenv("TURBOGAMES_WORKSHOP")
	if v == "" {
		t.Skip("pon TURBOGAMES_WORKSHOP con el appid de un juego")
	}
	appid64, _ := strconv.ParseUint(v, 10, 32)
	dir := t.TempDir()
	a := &App{settings: defaultSettings(), uaCache: map[uint32]uaEntry{}, wcache: loadWsCache(filepath.Join(dir, "w.json")), presets: loadPresetStore(filepath.Join(dir, "p.json")), gmeta: loadGameMetaStore(filepath.Join(dir, "j.json")), metas: loadMetaStore(filepath.Join(dir, "c.json")), mstore: loadModStore(filepath.Join(dir, "m.json"))}
	a.locate()
	// la prueba anterior acaba de cerrar su proceso: Steam tarda en soltar el juego
	for i := 0; i < 40 && a.steam() != nil && a.steam().ReadStatus().RunningApp != 0; i++ {
		time.Sleep(500 * time.Millisecond)
	}
	defer a.stopWorkshop()
	l, err := a.Mods(uint32(appid64), true)
	if err != nil || !l.Live || len(l.Mods) == 0 {
		t.Fatalf("%v %+v", err, l.LiveError)
	}
	d, err := a.ModInfo(uint32(appid64), l.Mods[0].ID, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%q: galería=%d estrellas=%d comentarios=%d", d.Mod.Title, len(d.Gallery), d.Stars, len(d.CommentList))
	p, err := a.ModsSearch(uint32(appid64), "", "votes", "", []string{"Build 42"})
	t.Logf("búsqueda con etiqueta: %v %d de %d", err, len(p.Mods), p.Total)
}
