package main

// Servidor del móvil. Un solo puerto para todo:
//   - HTTPS con el certificado propio: por wifi y por la conexión directa (UPnP).
//   - HTTP sin cifrar, solo desde este PC: es por donde entra el túnel de Cloudflare
//     (que ya pone su HTTPS de verdad). Desde fuera, HTTP se redirige a HTTPS.
// Todo cuelga de /c/<secreto>/: sin el secreto, el servidor no enseña nada.
//
// El móvil manda los fotogramas por WebSocket. Si no puede abrirlo (Safari no lo
// abre contra un certificado propio aunque se haya aceptado), usa HTTP: los
// fotogramas van por POST y lo que manda el PC por un EventSource.

import (
	"bufio"
	"crypto/subtle"
	"crypto/tls"
	"embed"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed web
var webFS embed.FS

const (
	frameKindVideo = 1
	frameKindPhoto = 2
	maxMessage     = 12 << 20 // una foto a toda resolución
)

type Server struct {
	e      *Engine
	nonce  string
	port   int
	ln     *sniffListener
	srv    *http.Server
	tlsCfg *tls.Config

	mu    sync.Mutex
	conns map[string]*httpTransport // conexiones por HTTP (plan B)
}

func startServer(e *Engine, nonce string) (*Server, error) {
	cert, err := loadOrCreateCert()
	if err != nil {
		return nil, fmt.Errorf("no se pudo crear el certificado: %w", err)
	}
	s := &Server{e: e, nonce: nonce, conns: map[string]*httpTransport{}}
	s.tlsCfg = &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		NextProtos:   []string{"http/1.1"}, // el WebSocket necesita HTTP/1.1
	}

	// El mismo puerto siempre que se pueda: el enlace del móvil (y la app instalada
	// en su pantalla de inicio) siguen valiendo de una vez para otra.
	var raw net.Listener
	pref := getSettings().Port
	for _, p := range append([]int{pref}, portRange()...) {
		if p <= 0 {
			continue
		}
		if raw, err = net.Listen("tcp", fmt.Sprintf(":%d", p)); err == nil {
			break
		}
	}
	if raw == nil {
		if raw, err = net.Listen("tcp", ":0"); err != nil {
			return nil, errors.New("no se pudo abrir ningún puerto en este PC")
		}
	}
	s.port = raw.Addr().(*net.TCPAddr).Port
	if s.port != pref {
		updateSettings(func(st *Settings) { st.Port = s.port })
	}
	s.ln = newSniffListener(raw, s.tlsCfg)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, s.nonce) })
	mux.HandleFunc("GET /c/{tok}", s.guard(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path+"/", http.StatusFound)
	}))
	mux.HandleFunc("GET /c/{tok}/{$}", s.guard(s.handlePage))
	mux.HandleFunc("GET /c/{tok}/{file}", s.guard(s.handleAsset))
	mux.HandleFunc("GET /c/{tok}/ws", s.guard(s.handleWS))
	// alive: la página pregunta si el enlace sigue valiendo (404 si no).
	mux.HandleFunc("GET /c/{tok}/alive", s.guard(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("GET /c/{tok}/ev", s.guard(s.handleEvents))
	mux.HandleFunc("POST /c/{tok}/x", s.guard(s.handlePost))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { notFound(w) })

	s.srv = &http.Server{
		Handler:           s.secure(mux),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       2 * time.Minute,
		TLSNextProto:      map[string]func(*http.Server, *tls.Conn, http.Handler){}, // sin HTTP/2
		ErrorLog:          nullLog,
	}
	go s.srv.Serve(s.ln)
	return s, nil
}

func portRange() []int {
	var out []int
	for p := 47851; p <= 47860; p++ {
		out = append(out, p)
	}
	return out
}

func (s *Server) Close() {
	s.srv.Close()
	s.mu.Lock()
	for _, t := range s.conns {
		t.close()
	}
	s.mu.Unlock()
}

const expiredPage = `<!doctype html><meta charset=utf-8><meta name=viewport content="width=device-width,initial-scale=1"><title>TurboCAM</title><body style="margin:0;min-height:100vh;display:grid;place-items:center;background:#0a0b12;color:#eceef8;font:16px system-ui;text-align:center;padding:24px;box-sizing:border-box"><div><div style="font-size:44px">📷</div><h2>Este enlace ya no funciona</h2><p style="color:#9196ae">Abre TurboCAM en el PC y escanea el código QR otra vez.</p></div>`

func notFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNotFound)
	io.WriteString(w, expiredPage)
}

func isLoopback(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// secure: desde fuera solo HTTPS, y cabeceras de seguridad para todo.
func (s *Server) secure(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil && !isLoopback(r.RemoteAddr) {
			http.Redirect(w, r, "https://"+r.Host+r.URL.RequestURI(), http.StatusFound)
			return
		}
		hd := w.Header()
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("Referrer-Policy", "no-referrer")
		hd.Set("X-Robots-Tag", "noindex")
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("Permissions-Policy", "camera=(self), microphone=()")
		h.ServeHTTP(w, r)
	})
}

func (s *Server) guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := getSettings().Token
		if subtle.ConstantTimeCompare([]byte(r.PathValue("tok")), []byte(tok)) != 1 {
			notFound(w)
			return
		}
		h(w, r)
	}
}

// via dice por dónde ha llegado el móvil (se enseña en la ventana).
func via(r *http.Request) string {
	if r.TLS == nil {
		if r.Header.Get("Cf-Connecting-Ip") != "" {
			return "internet"
		}
		return "local"
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip := net.ParseIP(host); ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()) {
		return "lan"
	}
	return "direct"
}

// ---------- página y archivos ----------

// phoneFiles: los archivos de la página y su tipo. Se fija a mano: en algunos Windows
// el registro dice que .js es texto, y el navegador no ejecuta así un worker.
var phoneFiles = map[string]string{
	"phone.js":     "text/javascript; charset=utf-8",
	"paint.js":     "text/javascript; charset=utf-8",
	"encoder.js":   "text/javascript; charset=utf-8",
	"sw.js":        "text/javascript; charset=utf-8",
	"phone.css":    "text/css; charset=utf-8",
	"logo.svg":     "image/svg+xml",
	"icon-192.png": "image/png",
	"icon-512.png": "image/png",
}

func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	b, _ := webFS.ReadFile("web/phone.html")
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; media-src 'self' blob: mediastream:; "+
		"connect-src 'self'; style-src 'self'; script-src 'self'; worker-src 'self'; manifest-src 'self'; "+
		"object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	w.Write(b)
}

func (s *Server) handleAsset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if name == "manifest.webmanifest" {
		s.handleManifest(w, r)
		return
	}
	ct, ok := phoneFiles[name]
	if !ok {
		notFound(w)
		return
	}
	b, err := webFS.ReadFile("web/" + name)
	if err != nil {
		notFound(w)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(b)
}

// handleManifest: la app para la pantalla de inicio apunta a este mismo enlace.
func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	base := "/c/" + r.PathValue("tok") + "/"
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "no-cache")
	json.NewEncoder(w).Encode(map[string]any{
		"name":             "TurboCAM",
		"short_name":       "TurboCAM",
		"description":      "El móvil como cámara del PC",
		"start_url":        base,
		"scope":            base,
		"id":               base,
		"display":          "fullscreen",
		"orientation":      "any",
		"background_color": "#0a0b12",
		"theme_color":      "#0a0b12",
		"icons": []map[string]string{
			{"src": base + "icon-192.png", "sizes": "192x192", "type": "image/png", "purpose": "any maskable"},
			{"src": base + "icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "any maskable"},
		},
	})
}

// ---------- lo que manda el móvil ----------

type hello struct {
	T     string `json:"t"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Model string `json:"model"`
}

// cleanID deja solo letras y números (el id lo inventa la página del móvil).
func cleanID(id string) string {
	if len(id) < 8 || len(id) > 40 {
		return ""
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return ""
		}
	}
	return id
}

func cleanName(s string, n int) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if r := []rune(s); len(r) > n {
		s = string(r[:n])
	}
	return s
}

type ackMsg struct {
	T string `json:"t"`
	N uint32 `json:"n"`
}

// onBinary procesa un mensaje binario: [tipo][número u32][JPEG].
func (s *Server) onBinary(p *phone, t transport, data []byte) {
	if len(data) < 6 {
		return
	}
	n := binary.LittleEndian.Uint32(data[1:5])
	switch data[0] {
	case frameKindVideo:
		// Se confirma nada más llegar: la decodificación va aparte y no hace esperar al
		// móvil (si no, la latencia de internet limita los fotogramas por segundo).
		s.e.queueFrame(p, n, data[5:])
		t.send(ackMsg{T: "ack", N: n})
	case frameKindPhoto:
		s.e.onPhoto(p, data[5:])
	}
}

func (s *Server) onText(p *phone, t transport, data []byte) {
	var m phoneMsg
	if json.Unmarshal(data, &m) != nil {
		return
	}
	if m.T == "ping" {
		var ts struct {
			TS float64 `json:"ts"`
		}
		json.Unmarshal(data, &ts)
		t.send(map[string]any{"t": "pong", "ts": ts.TS})
		return
	}
	if m.T == "forget" {
		s.e.Forget(p)
		return
	}
	s.e.onMessage(p, m)
}

// ---------- WebSocket ----------

var upgrader = websocket.Upgrader{
	ReadBufferSize:  64 << 10,
	WriteBufferSize: 16 << 10,
	// Solo desde la propia página (mismo origen).
	CheckOrigin: func(r *http.Request) bool {
		o := r.Header.Get("Origin")
		return o == "" || strings.EqualFold(strings.TrimPrefix(strings.TrimPrefix(o, "https://"), "http://"), r.Host)
	},
}

type wsTransport struct {
	c    *websocket.Conn
	mu   sync.Mutex
	once sync.Once
}

func (t *wsTransport) send(msg any) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.c.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return t.c.WriteMessage(websocket.TextMessage, b)
}

func (t *wsTransport) close() { t.once.Do(func() { t.c.Close() }) }

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	t := &wsTransport{c: c}
	defer t.close()
	c.SetReadLimit(maxMessage)

	c.SetReadDeadline(time.Now().Add(15 * time.Second))
	_, data, err := c.ReadMessage()
	if err != nil {
		return
	}
	var h hello
	if json.Unmarshal(data, &h) != nil || h.T != "hello" || cleanID(h.ID) == "" {
		t.send(map[string]any{"t": "bye", "reason": "bad"})
		return
	}
	p, err := s.e.addPhone(h.ID, cleanName(h.Name, 60), cleanName(h.Model, 80), via(r), t)
	if err != nil {
		t.send(map[string]any{"t": "bye", "reason": "full"})
		return
	}
	defer s.e.removePhone(p)
	for {
		// El móvil manda algo al menos cada 5 s; si se calla 30, se da por perdido.
		c.SetReadDeadline(time.Now().Add(30 * time.Second))
		kind, data, err := c.ReadMessage()
		if err != nil {
			return
		}
		if kind == websocket.BinaryMessage {
			s.onBinary(p, t, data)
		} else {
			s.onText(p, t, data)
		}
	}
}

// ---------- plan B por HTTP ----------

type httpTransport struct {
	id   string
	p    *phone
	q    chan []byte
	done chan struct{}
	once sync.Once
	seen atomic.Int64 // último POST del móvil (UnixNano)
}

func (t *httpTransport) touch() { t.seen.Store(time.Now().UnixNano()) }

func (t *httpTransport) send(msg any) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	select {
	case t.q <- b:
		return nil
	case <-t.done:
		return errors.New("cerrado")
	default:
		return errors.New("cola llena")
	}
}

func (t *httpTransport) close() { t.once.Do(func() { close(t.done) }) }

// handleEvents abre la conexión: lo que manda el PC va por aquí (EventSource).
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id := cleanID(q.Get("id"))
	fl, ok := w.(http.Flusher)
	if id == "" || !ok {
		http.Error(w, "petición no válida", http.StatusBadRequest)
		return
	}
	t := &httpTransport{id: randHex(12), q: make(chan []byte, 256), done: make(chan struct{})}
	p, err := s.e.addPhone(id, cleanName(q.Get("name"), 60), cleanName(q.Get("model"), 80), via(r), t)
	if err != nil {
		http.Error(w, "demasiados móviles", http.StatusServiceUnavailable)
		return
	}
	t.p = p
	s.mu.Lock()
	s.conns[t.id] = t
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.conns, t.id)
		s.mu.Unlock()
		t.close()
		s.e.removePhone(p)
	}()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	fmt.Fprintf(w, "data: {\"t\":\"conn\",\"c\":%q}\n\n", t.id)
	fl.Flush()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	t.touch()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-t.done:
			return
		case b := <-t.q:
			if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
				return
			}
			fl.Flush()
		case <-ping.C:
			// Si el móvil deja de mandar cosas por POST, la conexión se da por perdida.
			if time.Since(time.Unix(0, t.seen.Load())) > 30*time.Second {
				return
			}
			io.WriteString(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

// handlePost recibe del móvil fotogramas (binario) o mensajes (JSON).
func (s *Server) handlePost(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	t := s.conns[r.URL.Query().Get("c")]
	s.mu.Unlock()
	if t == nil {
		http.Error(w, "conexión cerrada", http.StatusGone)
		return
	}
	t.touch()
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxMessage))
	if err != nil {
		http.Error(w, "demasiado grande", http.StatusRequestEntityTooLarge)
		return
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		s.onText(t.p, t, data)
	} else {
		s.onBinary(t.p, t, data)
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- un puerto, HTTP y HTTPS ----------

// sniffListener mira el primer byte de cada conexión: 0x16 es el saludo de TLS.
type sniffListener struct {
	net.Listener
	cfg    *tls.Config
	conns  chan net.Conn
	closed chan struct{}
	once   sync.Once
}

func newSniffListener(raw net.Listener, cfg *tls.Config) *sniffListener {
	l := &sniffListener{Listener: raw, cfg: cfg, conns: make(chan net.Conn), closed: make(chan struct{})}
	go l.run()
	return l
}

func (l *sniffListener) run() {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			l.Close()
			return
		}
		go l.classify(c)
	}
}

type peekConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *peekConn) Read(b []byte) (int, error) { return c.r.Read(b) }

func (l *sniffListener) classify(c net.Conn) {
	c.SetReadDeadline(time.Now().Add(10 * time.Second))
	br := bufio.NewReader(c)
	first, err := br.Peek(1)
	c.SetReadDeadline(time.Time{})
	if err != nil {
		c.Close()
		return
	}
	var out net.Conn = &peekConn{Conn: c, r: br}
	if first[0] == 0x16 {
		out = tls.Server(out, l.cfg)
	}
	select {
	case l.conns <- out:
	case <-l.closed:
		c.Close()
	}
}

func (l *sniffListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *sniffListener) Close() error {
	var err error
	l.once.Do(func() {
		close(l.closed)
		err = l.Listener.Close()
	})
	return err
}

// ---------- vista previa para la ventana (solo este PC) ----------

type Preview struct {
	e     *Engine
	token string
	ln    net.Listener
	srv   *http.Server
}

func startPreview(e *Engine) (*Preview, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	pv := &Preview{e: e, token: randHex(16), ln: ln}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /out", pv.guard(func(w http.ResponseWriter, r *http.Request) {
		mjpeg(w, r, e.outHub, 0)
	}))
	mux.HandleFunc("GET /src/{id}", pv.guard(func(w http.ResponseWriter, r *http.Request) {
		h := e.phoneHub(r.PathValue("id"))
		if h == nil {
			http.NotFound(w, r)
			return
		}
		mjpeg(w, r, h, time.Second/8)
	}))
	pv.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second, ErrorLog: nullLog}
	go pv.srv.Serve(ln)
	return pv, nil
}

func (pv *Preview) Base() string {
	return fmt.Sprintf("http://%s", pv.ln.Addr().String())
}

func (pv *Preview) guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("k")), []byte(pv.token)) != 1 {
			http.NotFound(w, r)
			return
		}
		h(w, r)
	}
}

func (pv *Preview) Close() { pv.srv.Close() }

// mjpeg manda JPEG seguidos (multipart/x-mixed-replace), que un <img> pinta solo.
func mjpeg(w http.ResponseWriter, r *http.Request, hub *jpegHub, minGap time.Duration) {
	fl, ok := w.(http.Flusher)
	if !ok {
		return
	}
	hub.subs.Add(1)
	defer hub.subs.Add(-1)
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=turbocam")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	ctx := r.Context()
	var last uint64
	for {
		data, seq, ok := hub.next(ctx, last)
		if !ok {
			return
		}
		last = seq
		if _, err := fmt.Fprintf(w, "--turbocam\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(data)); err != nil {
			return
		}
		if _, err := w.Write(data); err != nil {
			return
		}
		io.WriteString(w, "\r\n")
		fl.Flush()
		if minGap > 0 {
			select {
			case <-time.After(minGap):
			case <-ctx.Done():
				return
			}
		}
	}
}

// nullLog calla los avisos de net/http (saludos TLS a medias de quien escanea puertos…).
var nullLog = log.New(io.Discard, "", 0)
