package main

import (
	"errors"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
)

// Una descarga parada cuyo enlace firmado caducó sigue con el enlace nuevo que manda
// la extensión, sin empezar de cero.
func TestResumeWithNewURL(t *testing.T) {
	srv := newTestServer(24 << 20)
	srv.rate = 1 << 20
	var expired atomic.Bool
	var newHits atomic.Int32
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") == "viejo" && expired.Load() {
			http.Error(w, "enlace caducado", http.StatusForbidden)
			return
		}
		if r.URL.Query().Get("t") == "nuevo" {
			newHits.Add(1)
		}
		srv.ServeHTTP(w, r)
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go (&http.Server{Handler: h}).Serve(ln)
	base := "http://" + ln.Addr().String() + "/archivo.bin"
	dir := t.TempDir()

	var part string
	_, err, _ = run(t, Request{URL: base + "?t=viejo", Dir: dir, Connections: 4}, func(d *Download) {
		d.OnInfo = func(i Info) { part = i.PartPath }
		d.OnProgress = func(p Progress) {
			if p.BytesDone > 6<<20 {
				d.Pause()
			}
		}
	})
	if !errors.Is(err, ErrPaused) {
		t.Fatalf("se esperaba pausa, error %v", err)
	}
	expired.Store(true)
	srv.rate = 0

	var resumed bool
	p, err, _ := run(t, Request{URL: base + "?t=nuevo", Dir: dir, PartPath: part, Connections: 4}, func(d *Download) {
		d.OnInfo = func(i Info) { resumed = i.Resumed }
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resumed {
		t.Fatal("ha empezado de cero en vez de seguir")
	}
	if newHits.Load() == 0 {
		t.Fatal("no ha usado el enlace nuevo")
	}
	if fileHash(t, p) != srv.hash(1) {
		t.Fatal("el archivo no es el original")
	}
	if _, err := os.Stat(statePath(part)); !os.IsNotExist(err) {
		t.Fatal("quedó el archivo de estado")
	}
}
