package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Recibir del móvil: simula el navegador del móvil contra el servidor real (solo en
// local). La ventana pide el estado en cada aviso, como hace la app de verdad.
func TestQRReceive(t *testing.T) {
	qrNoNetwork = true
	defer func() { qrNoNetwork = false }()
	dest := t.TempDir()
	var s *qrSession
	var pushes sync.WaitGroup
	s, err := newQRSession("receive", nil, dest, func() {
		pushes.Add(1)
		defer pushes.Done()
		if s != nil {
			s.state() // antes esto se quedaba bloqueado para siempre
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.stop()
	base := fmt.Sprintf("http://127.0.0.1:%d/q/%s", s.port, s.token)
	c := &http.Client{Timeout: 10 * time.Second}

	start := func(name string, size int, key, client string) (id string, got int64) {
		t.Helper()
		b, _ := json.Marshal(map[string]any{"name": name, "size": size, "key": key, "client": client})
		r, err := c.Post(base+"/up", "application/json", bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		var j struct {
			ID  string `json:"id"`
			Got int64  `json:"got"`
		}
		json.NewDecoder(r.Body).Decode(&j)
		if r.StatusCode != 200 || j.ID == "" {
			t.Fatalf("inicio %s: %d", name, r.StatusCode)
		}
		return j.ID, j.Got
	}
	put := func(id string, off int64, data []byte) (int64, bool) {
		t.Helper()
		req, _ := http.NewRequest("PUT", fmt.Sprintf("%s/up/%s?off=%d", base, id, off), bytes.NewReader(data))
		r, err := c.Do(req)
		if err != nil {
			t.Fatalf("trozo: %v", err)
		}
		defer r.Body.Close()
		var j struct {
			Got  int64 `json:"got"`
			Done bool  `json:"done"`
		}
		json.NewDecoder(r.Body).Decode(&j)
		if r.StatusCode != 200 {
			t.Fatalf("trozo %d: estado %d", off, r.StatusCode)
		}
		return j.Got, j.Done
	}
	upload := func(name string, data []byte, client string) {
		id, off := start(name, len(data), name+"|k", client)
		for off < int64(len(data)) {
			end := min(off+8<<20, int64(len(data)))
			got, done := put(id, off, data[off:end])
			off = got
			if done {
				break
			}
		}
	}
	rnd := func(n int) []byte { b := make([]byte, n); rand.Read(b); return b }

	// 1) Una foto de 1,7 MB, como la del fallo.
	foto := rnd(1_700_000)
	upload("foto.jpg", foto, "a")
	// 2) Dos a la vez, una de varios trozos.
	big, small := rnd(19_000_000), rnd(300_000)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); upload("video.mp4", big, "a") }()
	go func() { defer wg.Done(); upload("otra.jpg", small, "a") }()
	wg.Wait()
	// 3) Mismo nombre: no se pisa.
	upload("foto.jpg", small, "a")

	// 4) Se corta a mitad (el móvil recarga) y otra pestaña sigue por donde iba.
	half := rnd(3_000_000)
	id, _ := start("recarga.jpg", len(half), "recarga|k", "vieja")
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", s.port))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(conn, "PUT /q/%s/up/%s?off=0 HTTP/1.1\r\nHost: x\r\nContent-Length: %d\r\n\r\n", s.token, id, len(half))
	conn.Write(half[:1_000_000])
	time.Sleep(300 * time.Millisecond)
	conn.Close()
	time.Sleep(2300 * time.Millisecond) // se considera abandonada a los 2 s
	id2, got := start("recarga.jpg", len(half), "recarga|k", "nueva")
	if id2 != id || got != 1_000_000 {
		t.Fatalf("reanudar: id=%v (antes %v) got=%d", id2, id, got)
	}
	if g, done := put(id2, got, half[got:]); !done || g != int64(len(half)) {
		t.Fatalf("terminar la reanudada: %d %v", g, done)
	}

	// Que la página siga respondiendo (antes una recarga se quedaba colgada).
	if r, err := c.Get(base + "/"); err != nil || r.StatusCode != 200 {
		t.Fatalf("página: %v", err)
	}

	check := func(name string, want []byte) {
		t.Helper()
		got, err := os.ReadFile(filepath.Join(dest, name))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s no coincide (%v)", name, err)
		}
	}
	check("foto.jpg", foto)
	check("video.mp4", big)
	check("otra.jpg", small)
	check("foto (2).jpg", small)
	check("recarga.jpg", half)
	ents, _ := os.ReadDir(dest)
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".turbomenu-part") {
			t.Fatalf("queda un archivo a medias: %s", e.Name())
		}
	}
	st := s.state()
	if len(st.Uploads) != 5 {
		t.Fatalf("la ventana ve %d subidas", len(st.Uploads))
	}
	for _, u := range st.Uploads {
		if !u.Done {
			t.Fatalf("sin terminar en la ventana: %+v", u)
		}
	}
	io.Discard.Write(nil)
}

// Para probar la página del móvil a mano: TURBOMENU_QRDEMO=<carpeta> go test -run QRDemo -v .
func TestQRDemo(t *testing.T) {
	dest := os.Getenv("TURBOMENU_QRDEMO")
	if dest == "" {
		t.Skip()
	}
	qrNoNetwork = true
	s, err := newQRSession("receive", nil, dest, func() {})
	if err != nil {
		t.Fatal(err)
	}
	defer s.stop()
	os.WriteFile(filepath.Join(dest, "url.txt"), []byte(fmt.Sprintf("http://127.0.0.1:%d/q/%s/", s.port, s.token)), 0o644)
	time.Sleep(3 * time.Minute)
}

// Pasar al móvil: descargas de un archivo (también por partes) y de todo en ZIP.
func TestQRSend(t *testing.T) {
	qrNoNetwork = true
	defer func() { qrNoNetwork = false }()
	dir := t.TempDir()
	a := bytes.Repeat([]byte("turbo"), 400_000)
	os.WriteFile(filepath.Join(dir, "a.bin"), a, 0o644)
	os.MkdirAll(filepath.Join(dir, "fotos"), 0o755)
	os.WriteFile(filepath.Join(dir, "fotos", "b.jpg"), []byte("jpg"), 0o644)
	var s *qrSession
	s, err := newQRSession("send", []string{filepath.Join(dir, "a.bin"), filepath.Join(dir, "fotos")}, "", func() {
		if s != nil {
			s.state()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.stop()
	base := fmt.Sprintf("http://127.0.0.1:%d/q/%s", s.port, s.token)
	c := &http.Client{Timeout: 10 * time.Second}
	r, err := c.Get(base + "/f/0")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if !bytes.Equal(got, a) {
		t.Fatal("la descarga no coincide")
	}
	req, _ := http.NewRequest("GET", base+"/f/0", nil)
	req.Header.Set("Range", "bytes=10-19")
	r, err = c.Do(req)
	if err != nil || r.StatusCode != 206 {
		t.Fatalf("por partes: %v %v", err, r)
	}
	part, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if !bytes.Equal(part, a[10:20]) {
		t.Fatal("el trozo no coincide")
	}
	r, err = c.Get(base + "/zip")
	if err != nil {
		t.Fatal(err)
	}
	z, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if int64(len(z)) != r.ContentLength || len(z) < len(a) {
		t.Fatalf("zip: %d bytes de %d", len(z), r.ContentLength)
	}
	time.Sleep(300 * time.Millisecond)
	if st := s.state(); len(st.Downloads) != 3 {
		t.Fatalf("la ventana ve %d descargas", len(st.Downloads))
	}
}
