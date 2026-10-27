package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testApp(t *testing.T) (*App, string, *Member) {
	t.Helper()
	dir := t.TempDir()
	a := &App{dir: dir, settings: defaultSettings(), hub: newHub(), tracker: newTracker(func(*Transfer) {})}
	a.settings.RecvDir = filepath.Join(dir, "recibido")
	a.settings.AutoAccept = false
	a.rooms = newRoomStore(dir)
	r := a.rooms.Create("Prueba")
	m, _, err := a.rooms.Join(r.ID, "0123456789abcdef0123", "Ana")
	if err != nil {
		t.Fatal(err)
	}
	return a, r.ID, m
}

func partFiles(t *testing.T, root string) int {
	n := 0
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Ext(p) == ".turbosend-part" {
			n++
		}
		return nil
	})
	return n
}

// Aceptar no crea nada en el disco (eso congelaba la app con miles de archivos):
// cada archivo se crea con su primer bloque, y los vacíos al terminar.
func TestAcceptInLazy(t *testing.T) {
	a, room, m := testApp(t)
	var files []offerFile
	for i := 0; i < 3000; i++ {
		files = append(files, offerFile{Rel: fmt.Sprintf("fotos/%04d.txt", i), Size: 5})
	}
	files = append(files, offerFile{Rel: "fotos/vacio.txt", Size: 0})
	b, err := a.Offer(room, m, files)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	if err := a.AcceptIn(room, b.ID); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(t0); d > 2*time.Second {
		t.Errorf("aceptar ha tardado %v", d)
	}
	if n := partFiles(t, a.settings.RecvDir); n != 0 {
		t.Errorf("al aceptar ya hay %d archivos a medias", n)
	}
	// Otro envío con los mismos nombres, aceptado antes de que llegue nada del
	// primero: no puede caer en la misma carpeta.
	b2, err := a.Offer(room, m, []offerFile{{Rel: "fotos/0000.txt", Size: 5}})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.AcceptIn(room, b2.ID); err != nil {
		t.Fatal(err)
	}
	var f1, f2 string
	a.rooms.With(room, func(r *Room) bool {
		f1, f2 = r.inBundle(b.ID).Files[0].Final, r.inBundle(b2.ID).Files[0].Final
		return false
	})
	if f1 == f2 || filepath.Dir(f1) == filepath.Dir(f2) {
		t.Fatalf("los dos envíos van al mismo sitio: %s y %s", f1, f2)
	}

	for i := 0; i < 3000; i++ {
		if err := a.receiveChunk(room, b.ID, i, 0, []byte("hola!")); err != nil {
			t.Fatalf("bloque %d: %v", i, err)
		}
	}
	var status, root string
	a.rooms.With(room, func(r *Room) bool {
		status, root = r.inBundle(b.ID).Status, r.inBundle(b.ID).Root
		return false
	})
	if status != "done" {
		t.Fatalf("estado %q, quería done", status)
	}
	got, err := os.ReadFile(filepath.Join(root, "0042.txt"))
	if err != nil || !bytes.Equal(got, []byte("hola!")) {
		t.Errorf("0042.txt = %q, %v", got, err)
	}
	if st, err := os.Stat(filepath.Join(root, "vacio.txt")); err != nil || st.Size() != 0 {
		t.Errorf("el archivo vacío no se ha creado: %v", err)
	}
	if n := partFiles(t, root); n != 0 {
		t.Errorf("quedan %d archivos a medias", n)
	}
}

// Un bloque que llega justo después de cancelar no deja el archivo a medias.
func TestChunkAfterCancel(t *testing.T) {
	a, room, m := testApp(t)
	b, err := a.Offer(room, m, []offerFile{{Rel: "a.bin", Size: 10}})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.AcceptIn(room, b.ID); err != nil {
		t.Fatal(err)
	}
	a.CancelIn(room, b.ID)
	if err := a.receiveChunk(room, b.ID, 0, 0, make([]byte, 10)); err == nil {
		t.Error("un envío cancelado no debería aceptar bloques")
	}
	if n := partFiles(t, a.settings.RecvDir); n != 0 {
		t.Errorf("quedan %d archivos a medias", n)
	}
}

// Los ajustes de antes de la 2.1.2 dejan de ofrecer el enlace directo (enseña la IP).
func TestSettingsMigrate(t *testing.T) {
	s := defaultSettings()
	s.ShowDirectOnTunnel = true // como lo guardaba la 2.1.1
	s.migrate()
	if s.ShowDirectOnTunnel || s.Rev != settingsRev {
		t.Errorf("sin migrar: %+v", s)
	}
	s.ShowDirectOnTunnel = true // ya migrado: lo que elijas se respeta
	s.migrate()
	if !s.ShowDirectOnTunnel {
		t.Error("una vez migrado, el ajuste es tuyo")
	}
	if !defaultSettings().LANLink {
		t.Error("el enlace de red local sigue activado por defecto")
	}
}
