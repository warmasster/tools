package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ------------------------------------------------------------ servidor de pruebas

type testServer struct {
	size      int
	rate      int          // bytes/s por conexión (0 = sin límite)
	noRange   bool         // ignorar Range
	failRate  float64      // probabilidad de cortar en cada bloque
	maxConns  int          // >0: responder 403 por encima de N conexiones simultáneas
	limitCode int          // código para el límite (403 por defecto)
	busyUntil atomic.Int64 // hasta este instante (UnixNano) responde 429 a todo
	version   atomic.Int32
	active    atomic.Int32
	maxActive atomic.Int32
	failures  atomic.Int32
	newConns  atomic.Int32 // conexiones TCP abiertas
	perAddr   sync.Map     // IP local que atendió → nº de peticiones
	mu        sync.Mutex
	data      map[int32][]byte
}

func newTestServer(size int) *testServer {
	s := &testServer{size: size, data: map[int32][]byte{}}
	s.version.Store(1)
	return s
}

func (s *testServer) content(v int32) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.data[v]; ok {
		return b
	}
	b := make([]byte, s.size)
	rand.New(rand.NewSource(int64(v))).Read(b)
	s.data[v] = b
	return b
}

func (s *testServer) hash(v int32) [32]byte { return sha256.Sum256(s.content(v)) }

func (s *testServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	local := r.Context().Value(http.LocalAddrContextKey).(net.Addr).String()
	host, _, _ := net.SplitHostPort(local)
	n, _ := s.perAddr.LoadOrStore(host, new(atomic.Int32))
	n.(*atomic.Int32).Add(1)

	cur := s.active.Add(1)
	defer s.active.Add(-1)
	if time.Now().UnixNano() < s.busyUntil.Load() {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "espera", http.StatusTooManyRequests)
		return
	}
	if s.maxConns > 0 && int(cur) > s.maxConns {
		code := s.limitCode
		if code == 0 {
			code = http.StatusForbidden
		}
		w.Header().Set("Retry-After", "1")
		http.Error(w, "demasiadas conexiones", code)
		return
	}
	for {
		m := s.maxActive.Load()
		if cur <= m || s.maxActive.CompareAndSwap(m, cur) {
			break
		}
	}
	v := s.version.Load()
	data := s.content(v)
	etag := fmt.Sprintf(`"v%d"`, v)
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="prueba.bin"`)
	start, end, status := 0, len(data)-1, http.StatusOK
	if !s.noRange {
		w.Header().Set("Accept-Ranges", "bytes")
		ir := r.Header.Get("If-Range")
		if rg := r.Header.Get("Range"); rg != "" && (ir == "" || ir == etag) {
			a, b, _ := strings.Cut(strings.TrimPrefix(rg, "bytes="), "-")
			start, _ = strconv.Atoi(a)
			if b != "" {
				end, _ = strconv.Atoi(b)
				end = min(end, len(data)-1)
			}
			status = http.StatusPartialContent
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
		}
	}
	w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
	w.WriteHeader(status)
	const chunk = 32 << 10
	t0 := time.Now()
	sent := 0
	for p := start; p <= end; p += chunk {
		if s.failRate > 0 && rand.Float64() < s.failRate {
			s.failures.Add(1)
			panic(http.ErrAbortHandler) // corta la conexión de golpe
		}
		q := min(p+chunk, end+1)
		if _, err := w.Write(data[p:q]); err != nil {
			return
		}
		sent += q - p
		if s.rate > 0 {
			if ahead := time.Duration(float64(sent)/float64(s.rate)*float64(time.Second)) - time.Since(t0); ahead > 0 {
				time.Sleep(ahead)
			}
		}
	}
}

// listen arranca el servidor en 127.0.0.1 (y en otras IPs de loopback con el mismo puerto).
func (s *testServer) listen(t *testing.T, extraIPs ...string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	srv := &http.Server{Handler: s, ConnState: func(_ net.Conn, st http.ConnState) {
		if st == http.StateNew {
			s.newConns.Add(1)
		}
	}}
	go srv.Serve(ln)
	for _, ip := range extraIPs {
		l2, err := net.Listen("tcp", fmt.Sprintf("%s:%d", ip, port))
		if err != nil {
			t.Skipf("no se puede escuchar en %s: %v", ip, err)
		}
		go srv.Serve(l2)
	}
	t.Cleanup(func() { srv.Close() })
	return fmt.Sprintf("http://127.0.0.1:%d/prueba.bin", port)
}

// ------------------------------------------------------------ utilidades

func fileHash(t *testing.T, p string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

func run(t *testing.T, req Request, setup func(*Download)) (string, error, *Download) {
	t.Helper()
	d := NewDownload(req)
	if setup != nil {
		setup(d)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	p, err := d.Run(ctx)
	return p, err, d
}

func timed(t *testing.T, srv *testServer, url string, conns int) time.Duration {
	t.Helper()
	dir := t.TempDir()
	t0 := time.Now()
	p, err, _ := run(t, Request{URL: url, Dir: dir, Connections: conns}, nil)
	if err != nil {
		t.Fatal(err)
	}
	el := time.Since(t0)
	if fileHash(t, p) != srv.hash(1) {
		t.Fatalf("hash distinto con %d conexiones", conns)
	}
	return el
}

// ------------------------------------------------------------ pruebas

func TestSpeedup(t *testing.T) {
	srv := newTestServer(48 << 20)
	srv.rate = 2 << 20
	url := srv.listen(t)
	one := timed(t, srv, url, 1)
	many := timed(t, srv, url, 16)
	ratio := one.Seconds() / many.Seconds()
	t.Logf("1 conexión %.1fs · 16 conexiones %.1fs · %.1fx (máx. simultáneas %d)", one.Seconds(), many.Seconds(), ratio, srv.maxActive.Load())
	if ratio < 6 { // el arranque gradual (4 → 8 → 16) cuesta un par de segundos
		t.Fatalf("mejora insuficiente: %.1fx", ratio)
	}
}

func TestPauseResume(t *testing.T) {
	srv := newTestServer(32 << 20)
	srv.rate = 1 << 20
	url := srv.listen(t)
	dir := t.TempDir()
	var part string
	_, err, _ := run(t, Request{URL: url, Dir: dir, Connections: 8}, func(d *Download) {
		d.OnInfo = func(i Info) { part = i.PartPath }
		d.OnProgress = func(p Progress) {
			if p.BytesDone > 12<<20 {
				d.Pause()
			}
		}
	})
	if !errors.Is(err, ErrPaused) {
		t.Fatalf("se esperaba pausa, error %v", err)
	}
	if _, err := os.Stat(statePath(part)); err != nil {
		t.Fatal("no se guardó el estado")
	}
	srv.rate = 0
	// Sin PartPath: debe encontrar sola la descarga a medias por la URL.
	var resumed bool
	p, err, _ := run(t, Request{URL: url, Dir: dir, Connections: 8}, func(d *Download) {
		d.OnInfo = func(i Info) { resumed = i.Resumed }
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resumed {
		t.Fatal("no reanudó: empezó de cero")
	}
	if fileHash(t, p) != srv.hash(1) {
		t.Fatal("hash distinto tras reanudar")
	}
	if _, err := os.Stat(statePath(part)); !os.IsNotExist(err) {
		t.Fatal("quedó el archivo de estado")
	}
}

// Con tramos acotados las conexiones se reutilizan: no se abre una nueva con cada robo.
func TestConnectionReuse(t *testing.T) {
	srv := newTestServer(96 << 20)
	srv.rate = 8 << 20
	url := srv.listen(t)
	p, err, _ := run(t, Request{URL: url, Dir: t.TempDir(), Connections: 8}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fileHash(t, p) != srv.hash(1) {
		t.Fatal("hash distinto")
	}
	t.Logf("conexiones TCP abiertas: %d (máx. simultáneas %d)", srv.newConns.Load(), srv.maxActive.Load())
	if srv.newConns.Load() > srv.maxActive.Load()+4 {
		t.Fatalf("se abrieron %d conexiones TCP para %d simultáneas", srv.newConns.Load(), srv.maxActive.Load())
	}
}

// Si una sola conexión ya llena la línea, no tiene sentido abrir 16.
func TestRampPlateau(t *testing.T) {
	srv := newTestServer(64 << 20)
	url := srv.listen(t) // sin límite por conexión: 1 ya va a tope
	p, err, _ := run(t, Request{URL: url, Dir: t.TempDir(), Connections: 16}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fileHash(t, p) != srv.hash(1) {
		t.Fatal("hash distinto")
	}
	t.Logf("máximo de conexiones simultáneas: %d", srv.maxActive.Load())
	if srv.maxActive.Load() > 8 {
		t.Fatalf("abrió %d conexiones sin ganar velocidad", srv.maxActive.Load())
	}
}

func TestNoRange(t *testing.T) {
	srv := newTestServer(8 << 20)
	srv.noRange = true
	url := srv.listen(t)
	p, err, _ := run(t, Request{URL: url, Dir: t.TempDir(), Connections: 16}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fileHash(t, p) != srv.hash(1) {
		t.Fatal("hash distinto")
	}
	if srv.maxActive.Load() != 1 {
		t.Fatalf("sin Range debería usar 1 conexión, usó %d", srv.maxActive.Load())
	}
}

func TestFailures(t *testing.T) {
	srv := newTestServer(32 << 20)
	srv.failRate = 0.01
	url := srv.listen(t)
	p, err, _ := run(t, Request{URL: url, Dir: t.TempDir(), Connections: 16}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fileHash(t, p) != srv.hash(1) {
		t.Fatal("hash distinto")
	}
	t.Logf("%d cortes simulados", srv.failures.Load())
	if srv.failures.Load() == 0 {
		t.Fatal("no hubo cortes")
	}
}

func TestETagChange(t *testing.T) {
	srv := newTestServer(16 << 20)
	srv.rate = 1 << 20
	url := srv.listen(t)
	dir := t.TempDir()
	_, err, _ := run(t, Request{URL: url, Dir: dir, Connections: 4}, func(d *Download) {
		d.OnProgress = func(p Progress) {
			if p.BytesDone > 4<<20 {
				d.Pause()
			}
		}
	})
	if !errors.Is(err, ErrPaused) {
		t.Fatal(err)
	}
	srv.version.Store(2) // el archivo cambia en el servidor
	srv.rate = 0
	var notices []string
	p, err, _ := run(t, Request{URL: url, Dir: dir, Connections: 4}, func(d *Download) {
		d.OnNotice = func(s string) { notices = append(notices, s) }
	})
	if err != nil {
		t.Fatal(err)
	}
	if fileHash(t, p) != srv.hash(2) {
		t.Fatal("no es la versión nueva")
	}
	if len(notices) == 0 || !strings.Contains(notices[0], "cambiado") {
		t.Fatalf("faltó el aviso de cambio: %v", notices)
	}
}

func TestServerConnectionLimit(t *testing.T) {
	srv := newTestServer(24 << 20)
	srv.rate = 4 << 20
	srv.maxConns = 3
	url := srv.listen(t)
	var notices []string
	p, err, _ := run(t, Request{URL: url, Dir: t.TempDir(), Connections: 16}, func(d *Download) {
		d.OnNotice = func(s string) { notices = append(notices, s) }
	})
	if err != nil {
		t.Fatal(err)
	}
	if fileHash(t, p) != srv.hash(1) {
		t.Fatal("hash distinto")
	}
	t.Logf("avisos: %v", notices)
	var n int
	if len(notices) == 0 || !strings.Contains(notices[0], "limita") {
		t.Fatalf("faltó el aviso de límite: %v", notices)
	}
	fmt.Sscanf(notices[0][strings.LastIndex(notices[0], "usan ")+5:], "%d", &n)
	if n < 1 || n > 3 {
		t.Fatalf("se esperaba bajar a 3 o menos: %v", notices)
	}
}

func TestSpreadIPs(t *testing.T) {
	srv := newTestServer(16 << 20)
	srv.rate = 2 << 20
	base := srv.listen(t, "127.0.0.2", "127.0.0.3")
	url := strings.Replace(base, "127.0.0.1", "turbodl.test", 1)
	p, err, _ := run(t, Request{URL: url, Dir: t.TempDir(), Connections: 9}, func(d *Download) {
		d.lookupIP = func(context.Context, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("127.0.0.2"), net.ParseIP("127.0.0.3")}, nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if fileHash(t, p) != srv.hash(1) {
		t.Fatal("hash distinto")
	}
	used := 0
	srv.perAddr.Range(func(k, v any) bool {
		t.Logf("%s: %d peticiones", k, v.(*atomic.Int32).Load())
		used++
		return true
	})
	if used < 3 {
		t.Fatalf("solo se usaron %d IPs", used)
	}
}

func TestUniqueName(t *testing.T) {
	srv := newTestServer(2 << 20)
	url := srv.listen(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "prueba.bin"), []byte("ya existía"), 0o644)
	p, err, _ := run(t, Request{URL: url, Dir: dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != "prueba (1).bin" {
		t.Fatalf("nombre %q", filepath.Base(p))
	}
}

func TestCancelRemovesFiles(t *testing.T) {
	srv := newTestServer(16 << 20)
	srv.rate = 1 << 20
	url := srv.listen(t)
	dir := t.TempDir()
	_, err, _ := run(t, Request{URL: url, Dir: dir, Connections: 4}, func(d *Download) {
		d.OnProgress = func(p Progress) {
			if p.BytesDone > 2<<20 {
				d.Cancel()
			}
		}
	})
	if !errors.Is(err, ErrCanceled) {
		t.Fatal(err)
	}
	left, _ := os.ReadDir(dir)
	if len(left) != 0 {
		t.Fatalf("quedan archivos: %v", left)
	}
}

// Dos descargas del mismo archivo a la vez no pueden compartir nombre ni temporal.
func TestConcurrentSameName(t *testing.T) {
	srv := newTestServer(16 << 20)
	srv.rate = 4 << 20
	url := srv.listen(t)
	dir := t.TempDir()
	var wg sync.WaitGroup
	paths := make([]string, 3)
	errs := make([]error, 3)
	for i := range paths {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Cada una con su propio "sitio" en la URL para que no se reanuden entre sí.
			paths[i], errs[i], _ = run(t, Request{URL: fmt.Sprintf("%s?n=%d", url, i), Dir: dir, Connections: 4}, nil)
		}()
	}
	wg.Wait()
	seen := map[string]bool{}
	for i, p := range paths {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		if seen[p] {
			t.Fatalf("nombre repetido: %s", p)
		}
		seen[p] = true
		if fileHash(t, p) != srv.hash(1) {
			t.Fatalf("hash distinto en %s", p)
		}
	}
}

// El servidor responde 429 a todo durante unos segundos: no hay que rendirse.
func TestServerBusy(t *testing.T) {
	srv := newTestServer(16 << 20)
	srv.rate = 4 << 20
	url := srv.listen(t)
	var notices []string
	p, err, _ := run(t, Request{URL: url, Dir: t.TempDir(), Connections: 8}, func(d *Download) {
		d.OnNotice = func(s string) { notices = append(notices, s) }
		d.OnProgress = func(p Progress) {
			// A mitad de la descarga el servidor se pone a rechazar todo durante 4 s.
			if p.BytesDone > 4<<20 && srv.busyUntil.Load() == 0 {
				srv.busyUntil.Store(time.Now().Add(4 * time.Second).UnixNano())
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if fileHash(t, p) != srv.hash(1) {
		t.Fatal("hash distinto")
	}
	t.Logf("avisos: %v", notices)
}

// Límite por 429 en vez de 403: se adapta igual y termina.
func TestLimit429(t *testing.T) {
	srv := newTestServer(24 << 20)
	srv.rate = 4 << 20
	srv.maxConns = 2
	srv.limitCode = http.StatusTooManyRequests
	url := srv.listen(t)
	p, err, _ := run(t, Request{URL: url, Dir: t.TempDir(), Connections: 16}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fileHash(t, p) != srv.hash(1) {
		t.Fatal("hash distinto")
	}
}
