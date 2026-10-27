package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func bundleState(a *App, room, bid string) (status string, received int64, files []*InFile) {
	a.rooms.With(room, func(r *Room) bool {
		b := r.inBundle(bid)
		status, received, files = b.Status, b.Received, b.Files
		return false
	})
	return
}

// Si el archivo a medias desaparece (un antivirus) cuando ya habían llegado bloques,
// no se vuelve a crear a ceros: se olvida lo recibido y se pide otra vez.
func TestPartLostIsRequestedAgain(t *testing.T) {
	a, room, m := testApp(t)
	b, err := a.Offer(room, m, []offerFile{{Rel: "dos.bin", Size: 2 * inChunk}})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.AcceptIn(room, b.ID); err != nil {
		t.Fatal(err)
	}
	one, two := bytes.Repeat([]byte{1}, inChunk), bytes.Repeat([]byte{2}, inChunk)
	if err := a.receiveChunk(room, b.ID, 0, 0, one); err != nil {
		t.Fatal(err)
	}
	_, _, files := bundleState(a, room, b.ID)
	os.Remove(files[0].Final + ".turbosend-part")
	if err := a.receiveChunk(room, b.ID, 0, inChunk, two); !errors.Is(err, errPartLost) {
		t.Fatalf("con el archivo a medias borrado: %v", err)
	}
	if _, err := os.Stat(files[0].Final + ".turbosend-part"); err == nil {
		t.Fatal("no debe volver a crear el archivo a medias")
	}
	if _, rec, _ := bundleState(a, room, b.ID); rec != 0 {
		t.Fatalf("recibido %d, debería volver a 0", rec)
	}
	// El invitado lo vuelve a mandar entero.
	for i, d := range [][]byte{one, two} {
		if err := a.receiveChunk(room, b.ID, 0, int64(i)*inChunk, d); err != nil {
			t.Fatal(err)
		}
	}
	st, _, files := bundleState(a, room, b.ID)
	got, _ := os.ReadFile(files[0].Final)
	if st != "done" || !bytes.Equal(got, append(append([]byte{}, one...), two...)) {
		t.Fatalf("estado %s, %d bytes", st, len(got))
	}
}

// Dos chats con el mismo nombre comparten carpeta: dos envíos aceptados a la vez no
// pueden escribir en el mismo archivo.
func TestSameNameRoomsDontCollide(t *testing.T) {
	a, room1, m1 := testApp(t)
	r2 := a.rooms.Create("Prueba")
	m2, _, err := a.rooms.Join(r2.ID, "abcdef0123456789abcd", "Luis")
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := a.Offer(room1, m1, []offerFile{{Rel: "foto.jpg", Size: 5}})
	b2, _ := a.Offer(r2.ID, m2, []offerFile{{Rel: "foto.jpg", Size: 5}})
	if err := a.AcceptIn(room1, b1.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.AcceptIn(r2.ID, b2.ID); err != nil {
		t.Fatal(err)
	}
	_, _, f1 := bundleState(a, room1, b1.ID)
	_, _, f2 := bundleState(a, r2.ID, b2.ID)
	if f1[0].Final == f2[0].Final {
		t.Fatalf("los dos van a %s", f1[0].Final)
	}
}

// La app se cerró justo al guardar: al arrancar se termina, y lo ya renombrado cuenta
// como guardado (no se crea una copia a ceros).
func TestFinishPendingAfterCrash(t *testing.T) {
	a, room, m := testApp(t)
	b, _ := a.Offer(room, m, []offerFile{{Rel: "a.txt", Size: 5}, {Rel: "b.txt", Size: 5}})
	if err := a.AcceptIn(room, b.ID); err != nil {
		t.Fatal(err)
	}
	a.receiveChunk(room, b.ID, 0, 0, []byte("hola!"))
	// Simula el cierre a mitad: el primero ya renombrado, el segundo con su parte.
	_, _, files := bundleState(a, room, b.ID)
	os.Rename(files[0].Final+".turbosend-part", files[0].Final)
	os.MkdirAll(filepath.Dir(files[1].Final), 0o755)
	os.WriteFile(files[1].Final+".turbosend-part", []byte("adios"), 0o644)
	a.rooms.With(room, func(r *Room) bool {
		f := r.inBundle(b.ID).Files[1]
		f.mark(0)
		r.inBundle(b.ID).Status = "accepted"
		return true
	})
	a.finishPending()
	st, _, files := bundleState(a, room, b.ID)
	if st != "done" {
		t.Fatalf("estado %s", st)
	}
	for i, want := range []string{"hola!", "adios"} {
		if got, _ := os.ReadFile(files[i].Final); string(got) != want {
			t.Errorf("%s = %q", files[i].Final, got)
		}
	}
	if m, _ := filepath.Glob(filepath.Join(filepath.Dir(files[0].Final), "*(2)*")); len(m) > 0 {
		t.Errorf("ha creado copias: %v", m)
	}
}
