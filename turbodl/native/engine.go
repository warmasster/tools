package main

// Motor de descarga multiconexión.
//
// Diferencias con el motor del navegador, que son las que lo hacen más rápido:
//   - Sin el límite de 6 conexiones por servidor de Chrome (por defecto 16, hasta 64).
//   - Cada conexión tiene su propio Transport: una conexión TCP por trabajador aunque
//     el servidor hable HTTP/2 (Chrome lo metería todo en una sola).
//   - Reparte las conexiones entre todas las IPs del servidor (CDN con varias IPs).
//   - Escribe directamente en la carpeta de destino: sin copia final ni 2x de disco.
//   - Arranque gradual: empieza con 4 conexiones y las duplica mientras la velocidad
//     suba. Si una sola ya llena la línea no abre más (y no despierta los sistemas
//     antiabuso de algunos servidores, que bloquean la IP si llegan 16 de golpe).
//   - Pide el archivo en tramos acotados (8-256 MB, ~30 s cada uno) y solo reparte
//     lo que nadie tiene pedido: ninguna respuesta se corta a medias, así que cada
//     conexión TCP se reutiliza (keep-alive) en vez de abrir otra con cada robo. Abrir
//     muchas conexiones nuevas por minuto es justo lo que vigilan los antiabuso.
//   - Sustituye conexiones lentas y se adapta si el servidor limita las conexiones.
//
// Modelo de segmentos: cada segmento [start, end) tiene `pos` (bytes ya leídos y
// reservados por su trabajador) y `done` (bytes ya escritos en disco). El robo de
// trabajo parte por `pos`; el estado que se guarda usa `done`, así nunca promete
// datos que no estén en disco.

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

const (
	DefaultConnections = 16
	MaxConnections     = 64
	minSegment         = 1 << 20   // al robar, cada mitad tiene al menos 1 MiB
	firstChunk         = 4 << 20   // la primera petición (sirve también de sondeo)
	minChunk           = 8 << 20   // tramo mínimo por petición
	maxChunk           = 256 << 20 // tramo máximo por petición
	chunkSeconds       = 30        // cada tramo dura ~30 s: pocas peticiones, conexión reutilizada
	bufSize            = 1 << 20   // búfer de escritura por conexión
	flushEvery         = 250 * time.Millisecond
	stallTimeout       = 20 * time.Second
	saveEvery          = 2 * time.Second
	tickEvery          = 500 * time.Millisecond
	evictEvery         = 3 * time.Second
	maxRetries         = 5
	maxRetriesAlone    = 10 // la última conexión aguanta más cortes de red (~2,5 min)
	rampStart          = 4
	rampEvery          = 500 * time.Millisecond
	rampGain           = 1.10 // duplicar solo si la velocidad subió al menos un 10 %
	probeEvery         = 30 * time.Second
	maxCooldown        = 2 * time.Minute
	drainLimit         = 256 << 10 // si al cortar un rango quedan menos bytes, se leen para reutilizar la conexión
	partExt            = ".turbodl"
	stateExt           = ".turbodl.json"
)

var (
	ErrPaused   = errors.New("pausada")
	ErrCanceled = errors.New("cancelada")
	errChanged  = errors.New("el archivo ha cambiado en el servidor")
	errNoRange  = errors.New("el servidor ha dejado de aceptar rangos")
	errSlow     = errors.New("conexión lenta")
	errYield    = errors.New("conexión sobrante")
	errStall    = errors.New("conexión parada")
	errRestart  = errors.New("reinicio")
)

// Request describe una descarga. Los campos JSON son los que manda la extensión.
type Request struct {
	URL         string            `json:"url"`
	Dir         string            `json:"dir,omitempty"`
	Filename    string            `json:"filename,omitempty"`
	PartPath    string            `json:"partPath,omitempty"` // reanudar este .turbodl
	Headers     map[string]string `json:"headers,omitempty"`  // Cookie, Referer, User-Agent...
	Connections int               `json:"connections,omitempty"`
	HTTP2       bool              `json:"http2,omitempty"`    // permitir HTTP/2 (desactivado: 1 TCP por conexión)
	NoSpread    bool              `json:"noSpread,omitempty"` // no repartir entre IPs
}

type Progress struct {
	BytesDone   int64      `json:"bytesDone"`
	Size        int64      `json:"size"` // -1 si no se conoce
	Speed       float64    `json:"speed"`
	ETA         float64    `json:"eta"` // -1 si no se puede calcular
	Connections int        `json:"connections"`
	Waiting     bool       `json:"waiting"`  // ninguna conexión recibe datos: esperando al servidor
	Segments    [][4]int64 `json:"segments"` // start, end, done, activo
}

type Info struct {
	FinalPath string `json:"finalPath"`
	PartPath  string `json:"partPath"`
	Size      int64  `json:"size"`
	RangeOK   bool   `json:"rangeOK"`
	Resumed   bool   `json:"resumed"`
}

type HTTPError struct {
	Status     int
	RetryAfter time.Duration // de la cabecera Retry-After (429/503)
}

func httpError(resp *http.Response) *HTTPError {
	e := &HTTPError{Status: resp.StatusCode}
	if v := strings.TrimSpace(resp.Header.Get("Retry-After")); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			e.RetryAfter = time.Duration(secs) * time.Second
		} else if t, err := http.ParseTime(v); err == nil {
			e.RetryAfter = time.Until(t)
		}
	}
	return e
}

func (e *HTTPError) Error() string { return fmt.Sprintf("el servidor respondió %d", e.Status) }

// fatal: reintentar no lo arregla (enlace caducado, no existe...).
func (e *HTTPError) fatal() bool {
	return e.Status == 401 || e.Status == 404 || e.Status == 410
}

// limitLike: respuestas típicas de "demasiadas conexiones desde tu IP".
func limitLike(err error) bool {
	var he *HTTPError
	if errors.As(err, &he) {
		return he.Status == 403 || he.Status == 429 || he.Status == 503 || he.Status == 509
	}
	return false
}

type segment struct {
	start, end int64 // end exclusivo; -1 = tamaño desconocido (modo single)
	pos, done  int64
	reqEnd     int64 // fin del tramo pedido en curso: hasta ahí no se puede robar
	w          *worker
}

func (s *segment) complete() bool { return s.end >= 0 && s.done >= s.end }

type worker struct {
	id        int
	client    *http.Client
	tr        *http.Transport
	ip        net.IP
	seg       *segment
	buf       []byte
	cancel    context.CancelCauseFunc // cancela la petición en curso
	chunk     int64                   // tamaño de tramo por petición (se adapta a su velocidad)
	bytes     atomic.Int64            // recibidos desde la última medición
	speed     float64
	retired   bool
	streaming bool      // recibiendo datos de una respuesta válida
	since     time.Time // desde cuándo recibe datos (para no sustituir conexiones recién abiertas)
	limitHits int       // rechazos seguidos de esta conexión
}

type savedState struct {
	Version      int        `json:"version"`
	URL          string     `json:"url"`      // la que pidió el usuario
	FinalURL     string     `json:"finalUrl"` // tras redirecciones
	FinalPath    string     `json:"finalPath"`
	Size         int64      `json:"size"`
	ETag         string     `json:"etag,omitempty"`
	LastModified string     `json:"lastModified,omitempty"`
	RangeOK      bool       `json:"rangeOK"`
	Segments     [][3]int64 `json:"segments"` // start, end, done
}

type Download struct {
	Req        Request
	OnProgress func(Progress)
	OnInfo     func(Info)
	OnNotice   func(string)
	Logf       func(format string, args ...any)                         // opcional: diagnóstico (el host lo manda a native.log)
	lookupIP   func(ctx context.Context, host string) ([]net.IP, error) // sustituible en pruebas

	mu        sync.Mutex
	url       string
	host      string
	size      int64
	etag      string
	lastMod   string
	rangeOK   bool
	segs      []*segment
	target    int // máximo pedido por el usuario
	cur       int // conexiones permitidas ahora (arranque gradual / límite del servidor)
	plateau   bool
	lastLimit time.Time // último rechazo por límite del servidor
	coolUntil time.Time // nadie hace peticiones nuevas hasta entonces (Retry-After)
	rateLimit bool      // el servidor limita peticiones por ventana de tiempo
	rateNoted bool
	waitNoted bool
	flat      int // mediciones seguidas sin ganar velocidad
	probes    int
	workers   map[int]*worker
	nextID    int
	ips       []net.IP
	ipNext    int
	file      *os.File
	finalPath string
	partPath  string
	failErr   error
	restartOf error // errChanged / errNoRange pendiente
	limited   bool

	received atomic.Int64
	ctx      context.Context
	cancel   context.CancelCauseFunc
	wg       sync.WaitGroup
	wake     chan struct{}
}

func NewDownload(req Request) *Download {
	if req.Connections <= 0 {
		req.Connections = DefaultConnections
	}
	req.Connections = min(req.Connections, MaxConnections)
	return &Download{Req: req, target: req.Connections, cur: min(req.Connections, rampStart), wake: make(chan struct{}, 1)}
}

// SetConnections cambia el número de conexiones en caliente.
func (d *Download) SetConnections(n int) {
	n = max(1, min(n, MaxConnections))
	d.mu.Lock()
	if n > d.target {
		d.plateau = false // dejar que el arranque gradual vuelva a subir
	}
	d.target = n
	d.cur = min(max(d.cur, 1), n)
	d.trimLocked()
	d.mu.Unlock()
	d.poke()
}

// Pause y Cancel paran la descarga; Run devuelve ErrPaused / ErrCanceled.
func (d *Download) Pause() {
	if d.cancel != nil {
		d.cancel(ErrPaused)
	}
}

func (d *Download) Cancel() {
	if d.cancel != nil {
		d.cancel(ErrCanceled)
	}
}

func (d *Download) poke() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

func (d *Download) logf(format string, args ...any) {
	if d.Logf != nil {
		d.Logf(format, args...)
	}
}

func (d *Download) notice(msg string) {
	if d.OnNotice != nil {
		d.OnNotice(msg)
	}
}

// ================================================================ arranque

// Run descarga hasta terminar, pausar, cancelar o fallar. Devuelve la ruta final.
func (d *Download) Run(parent context.Context) (string, error) {
	d.ctx, d.cancel = context.WithCancelCause(parent)
	defer d.cancel(nil)
	if d.lookupIP == nil {
		d.lookupIP = func(ctx context.Context, host string) ([]net.IP, error) {
			return net.DefaultResolver.LookupIP(ctx, "ip", host)
		}
	}
	for {
		first, w, err := d.prepare()
		if err != nil {
			d.closeFile()
			return "", d.stopErr(err)
		}
		result, err := d.loop(first, w)
		if errors.Is(err, errRestart) {
			continue
		}
		return result, err
	}
}

func (d *Download) stopErr(err error) error {
	if c := context.Cause(d.ctx); c != nil && (errors.Is(c, ErrPaused) || errors.Is(c, ErrCanceled)) {
		if errors.Is(c, ErrCanceled) {
			d.removeFiles()
		}
		return c
	}
	return err
}

// prepare abre el estado guardado o hace el primer contacto con el servidor. Si
// hace el primer contacto, devuelve esa respuesta para que el trabajador 0 la siga
// leyendo (no se desperdicia un viaje de ida y vuelta en "sondear").
func (d *Download) prepare() (*http.Response, *worker, error) {
	d.workers = map[int]*worker{}
	if d.Req.PartPath == "" && d.Req.Dir != "" {
		d.Req.PartPath = findResumable(d.Req.Dir, d.Req.URL)
	}
	if d.Req.PartPath != "" && d.restartOf == nil {
		if ok, err := d.loadState(); err != nil {
			return nil, nil, err
		} else if ok {
			d.resolveIPs()
			return nil, nil, nil
		}
	}
	restarted := d.restartOf != nil
	d.restartOf = nil

	if d.url == "" {
		d.url = d.Req.URL
	}
	u, err := url.Parse(d.url)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, nil, fmt.Errorf("URL no válida: %s", d.Req.URL)
	}
	d.host = u.Hostname()
	d.resolveIPs()

	w := d.newWorker()
	firstEnd := int64(-1) // fin del tramo pedido (-1: hasta el final)
	var resp *http.Response
	for attempt := 0; ; attempt++ {
		resp, err = d.do(w, d.Req.URL, 0, firstChunk, false)
		if err == nil || attempt >= 3 || d.ctx.Err() != nil {
			break
		}
		// Fallo de red en la primera petición: otra IP y otra vez.
		w.tr.CloseIdleConnections()
		if len(d.ips) > 0 {
			w.ip = d.ips[d.ipNext%len(d.ips)]
			d.ipNext++
		}
		select {
		case <-time.After(time.Duration(500<<attempt) * time.Millisecond):
		case <-d.ctx.Done():
		}
	}
	if err != nil {
		return nil, nil, err
	}
	if resp.Request != nil && resp.Request.URL != nil {
		d.url = resp.Request.URL.String()
		if h := resp.Request.URL.Hostname(); h != d.host {
			d.host = h
			d.resolveIPs()
		}
	}
	d.etag = resp.Header.Get("ETag")
	d.lastMod = resp.Header.Get("Last-Modified")
	enc := strings.ToLower(resp.Header.Get("Content-Encoding"))
	switch {
	case resp.StatusCode == http.StatusPartialContent && (enc == "" || enc == "identity"):
		cr, ok := parseContentRange(resp.Header.Get("Content-Range"))
		if !ok || cr.start != 0 || cr.total < 0 {
			resp.Body.Close()
			return nil, nil, errors.New("respuesta de rango no válida")
		}
		d.rangeOK, d.size = true, cr.total
		firstEnd = cr.end + 1
	case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent:
		d.rangeOK, d.size = false, -1
		if resp.StatusCode == http.StatusOK && resp.ContentLength >= 0 && (enc == "" || enc == "identity") {
			d.size = resp.ContentLength
		}
	default:
		resp.Body.Close()
		return nil, nil, httpError(resp)
	}

	if !restarted || d.finalPath == "" {
		name := d.Req.Filename
		if name == "" {
			name = filenameFrom(resp, d.url)
		}
		dir := d.Req.Dir
		if dir == "" {
			dir = DownloadsDir()
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			resp.Body.Close()
			return nil, nil, err
		}
		final, f, err := reservePath(filepath.Join(dir, sanitizeFilename(name)))
		if err != nil {
			resp.Body.Close()
			return nil, nil, err
		}
		f.Close()
		d.finalPath, d.partPath = final, final+partExt
	}
	if d.size > 0 {
		if free, ok := freeSpace(filepath.Dir(d.partPath)); ok && free < uint64(d.size)+64<<20 {
			resp.Body.Close()
			return nil, nil, fmt.Errorf("no hay espacio suficiente: hacen falta %s y quedan %s", fmtBytes(d.size), fmtBytes(int64(free)))
		}
	}
	if err := d.openFile(true); err != nil {
		resp.Body.Close()
		return nil, nil, err
	}
	end := d.size
	d.segs = []*segment{{start: 0, end: end}}
	if !d.rangeOK && d.target > 1 {
		d.notice("El servidor no permite descargar por partes: una sola conexión.")
	}
	d.segs[0].w = w
	d.segs[0].reqEnd = firstEnd
	w.seg = d.segs[0]
	if d.OnInfo != nil {
		d.OnInfo(Info{FinalPath: d.finalPath, PartPath: d.partPath, Size: d.size, RangeOK: d.rangeOK})
	}
	d.saveState()
	return resp, w, nil
}

func (d *Download) loadState() (bool, error) {
	if !isPartPath(d.Req.PartPath) {
		return false, nil // no es un parcial de TurboDL: no se toca
	}
	raw, err := os.ReadFile(statePath(d.Req.PartPath))
	if err != nil {
		return false, nil // sin estado: empezar de cero
	}
	var st savedState
	fi, ferr := os.Stat(d.Req.PartPath)
	if json.Unmarshal(raw, &st) != nil || !st.RangeOK || ferr != nil || fi.Size() != st.Size ||
		!strings.EqualFold(st.FinalPath+partExt, d.Req.PartPath) || !validSegments(st.Segments, st.Size) {
		// Estado roto o que no cuadra: lo que hay no sirve para seguir. Se borra para
		// no dejar un parcial de gigas huérfano (y que el nombre quede libre).
		os.Remove(d.Req.PartPath)
		os.Remove(statePath(d.Req.PartPath))
		return false, nil
	}
	d.url, d.size, d.etag, d.lastMod, d.rangeOK = st.FinalURL, st.Size, st.ETag, st.LastModified, true
	d.finalPath, d.partPath = st.FinalPath, d.Req.PartPath
	if u, err := url.Parse(d.url); err == nil {
		d.host = u.Hostname()
	}
	d.segs = nil
	for _, s := range st.Segments {
		d.segs = append(d.segs, &segment{start: s[0], end: s[1], pos: s[2], done: s[2]})
	}
	if err := d.openFile(false); err != nil {
		return false, err
	}
	if d.OnInfo != nil {
		d.OnInfo(Info{FinalPath: d.finalPath, PartPath: d.partPath, Size: d.size, RangeOK: true, Resumed: true})
	}
	return true, nil
}

func (d *Download) openFile(create bool) error {
	flags := os.O_RDWR
	if create {
		flags |= os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(d.partPath, flags, 0o644)
	if err != nil {
		return err
	}
	if create && d.size > 0 {
		// Disperso: si no, NTFS rellenaría con ceros hasta el punto de cada escritura
		// lejana (gigas escritos dos veces al empezar).
		setSparse(f, true)
		if err := f.Truncate(d.size); err != nil {
			f.Close()
			return err
		}
	}
	d.file = f
	return nil
}

func (d *Download) closeFile() {
	if d.file != nil {
		d.file.Close()
		d.file = nil
	}
}

func (d *Download) removeFiles() {
	d.closeFile()
	if d.partPath != "" {
		os.Remove(d.partPath)
		os.Remove(statePath(d.partPath))
	}
}

// resolveIPs busca todas las IPs del servidor para repartir las conexiones.
func (d *Download) resolveIPs() {
	d.ips = nil
	if d.Req.NoSpread || d.host == "" || net.ParseIP(d.host) != nil {
		return
	}
	ctx, cancel := context.WithTimeout(d.ctx, 3*time.Second)
	defer cancel()
	ips, err := d.lookupIP(ctx, d.host)
	if err != nil {
		return
	}
	var v4, v6 []net.IP
	for _, ip := range ips {
		if ip.To4() != nil {
			v4 = append(v4, ip)
		} else {
			v6 = append(v6, ip)
		}
	}
	// IPv4 si lo hay, aunque sea una sola IP: hay muchas redes con IPv6 roto que
	// conectan pero luego cortan (EOF), y Go probaría IPv6 primero.
	if len(v4) > 0 {
		d.ips = v4
	} else {
		d.ips = v6
	}
}

// ================================================================ trabajadores

func (d *Download) newWorker() *worker {
	// Tramo inicial grande (luego se ajusta a la velocidad): muchos servidores limitan
	// las peticiones por minuto, y con tramos pequeños se gastan enseguida.
	w := &worker{id: d.nextID, buf: make([]byte, bufSize), chunk: maxChunk}
	d.nextID++
	if len(d.ips) > 0 {
		w.ip = d.ips[d.ipNext%len(d.ips)]
		d.ipNext++
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	w.tr = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, _ := net.SplitHostPort(addr)
			d.mu.Lock()
			ip, target := w.ip, d.host
			d.mu.Unlock()
			if ip != nil && strings.EqualFold(host, target) {
				if c, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port)); err == nil {
					return c, nil
				}
			}
			return dialer.DialContext(ctx, network, addr)
		},
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		MaxIdleConns:          2,
		MaxIdleConnsPerHost:   1,
		IdleConnTimeout:       60 * time.Second,
		DisableCompression:    true,
		ReadBufferSize:        256 << 10,
		ForceAttemptHTTP2:     d.Req.HTTP2,
	}
	if !d.Req.HTTP2 {
		// Mapa vacío = nada de HTTP/2: cada trabajador es una conexión TCP de verdad.
		w.tr.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	}
	w.client = &http.Client{Transport: w.tr}
	d.workers[w.id] = w
	return w
}

// do hace una petición de rango [from, to) (to = -1 hasta el final).
func (d *Download) do(w *worker, rawURL string, from, to int64, ifRange bool) (*http.Response, error) {
	ctx, cancel := context.WithCancelCause(d.ctx)
	d.mu.Lock()
	w.cancel = cancel
	d.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		cancel(nil)
		return nil, err
	}
	req.Header.Set("User-Agent", "turbodl-native/"+version) // la extensión manda el del navegador
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "identity") // bytes tal cual: los rangos tienen que cuadrar
	sameSite := sameSiteHost(hostOnly(rawURL), hostOnly(d.Req.URL))
	for k, v := range d.Req.Headers {
		if v == "" {
			continue
		}
		// Las cookies se calcularon para la URL que mandó la extensión: si el servidor
		// redirige a otro dominio, no se le mandan (como hace Go al seguir redirecciones).
		if !sameSite && (strings.EqualFold(k, "Cookie") || strings.EqualFold(k, "Authorization")) {
			continue
		}
		req.Header.Set(k, v)
	}
	if to >= 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", from, to-1))
	} else {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", from))
	}
	if ifRange {
		if d.etag != "" && !strings.HasPrefix(d.etag, "W/") {
			req.Header.Set("If-Range", d.etag)
		} else if d.lastMod != "" {
			req.Header.Set("If-Range", d.lastMod)
		}
	}
	resp, err := w.client.Do(req)
	if err != nil {
		if c := context.Cause(ctx); c != nil && !errors.Is(c, context.Canceled) {
			return nil, c
		}
		return nil, err
	}
	return resp, nil
}

func (d *Download) runWorker(w *worker, first *http.Response) {
	defer d.wg.Done()
	defer d.workerExit(w)
	retries := 0
	if first != nil {
		d.mu.Lock()
		reqEnd := w.seg.reqEnd
		d.mu.Unlock()
		if !d.handle(w, w.seg, d.stream(w, w.seg, first, reqEnd), &retries) {
			return
		}
	}
	for {
		d.mu.Lock()
		seg := w.seg
		if seg == nil || seg.w != w || seg.complete() {
			seg = d.pickLocked(w, true)
			retries = 0
		}
		d.mu.Unlock()
		if seg == nil {
			return
		}
		d.mu.Lock()
		before := seg.done
		d.mu.Unlock()
		err := d.fetch(w, seg)
		d.mu.Lock()
		if seg.done > before {
			retries = 0 // hubo progreso: el fallo es puntual
		}
		d.mu.Unlock()
		if !d.handle(w, seg, err, &retries) {
			return
		}
	}
}

// handle decide qué hacer tras un intento. Devuelve false si el trabajador termina.
func (d *Download) handle(w *worker, seg *segment, err error, retries *int) bool {
	if err == nil {
		*retries = 0
		d.mu.Lock()
		if seg.end < 0 { // single sin tamaño: terminado al acabar el cuerpo
			seg.end = seg.done
		}
		if seg.complete() && seg.w == w {
			seg.w, w.seg = nil, nil
		}
		d.mu.Unlock()
		return d.ctx.Err() == nil
	}
	// Si no se pudo escribir lo último que llegó (disco lleno, antivirus…), pos va por
	// delante de done: se vuelve a pedir desde lo que está de verdad en disco. Si no,
	// el siguiente tramo empezaría más adelante y quedaría un hueco de ceros.
	d.mu.Lock()
	if seg != nil && seg.w == w && seg.pos > seg.done {
		seg.pos = seg.done
	}
	d.mu.Unlock()
	if d.ctx.Err() != nil {
		return false
	}
	switch {
	case errors.Is(err, errYield), errors.Is(err, errRestart):
		d.release(w, seg)
		return false
	case errors.Is(err, errSlow), errors.Is(err, errStall):
		// Conexión nueva (quizá a otra IP). Una lenta no cuenta como fallo.
		w.tr.CloseIdleConnections()
		d.mu.Lock()
		if len(d.ips) > 0 {
			w.ip = d.ips[d.ipNext%len(d.ips)]
			d.ipNext++
		}
		d.mu.Unlock()
		if errors.Is(err, errSlow) {
			return true
		}
	case errors.Is(err, errChanged), errors.Is(err, errNoRange):
		d.requestRestart(err)
		return false
	}
	var he *HTTPError
	if errors.As(err, &he) && he.fatal() {
		d.fail(err)
		return false
	}
	others := d.othersActive(w)
	if limitLike(err) {
		var le *HTTPError
		errors.As(err, &le)
		d.mu.Lock()
		d.lastLimit = time.Now()
		w.limitHits++
		d.logf("límite del servidor: %d (Retry-After %s), otras %d, permitidas %d, rechazos seguidos %d", le.Status, le.RetryAfter, others, d.cur, w.limitHits)
		if (le.Status == 429 || le.Status == 503) && le.RetryAfter > 0 && w.limitHits <= 3 {
			// Límite de PETICIONES por ventana de tiempo (p. ej. megadl: el Retry-After
			// es la cuenta atrás hasta la ventana siguiente). No sobran conexiones: las
			// que están descargando siguen, y nadie pide tramos nuevos hasta entonces.
			// Si la misma conexión se rechaza 3 veces seguidas, sí es límite de conexiones.
			d.rateLimit = true
			if until := time.Now().Add(min(le.RetryAfter, maxCooldown)); until.After(d.coolUntil) {
				d.coolUntil = until
			}
			first := !d.rateNoted
			d.rateNoted = true
			d.mu.Unlock()
			if first {
				go d.notice("El servidor limita cuántas peticiones se le hacen: se espera lo que pide y se sigue con todas las conexiones.")
			}
			d.release(w, seg)
			return true // fetch espera a que pase el Retry-After
		}
		d.limited = true
		if others > 0 {
			// Hay otras conexiones funcionando: esta sobra y se retira. Su Retry-After
			// no se aplica a las demás (si no, al final de la descarga, cuando todas
			// piden tramo nuevo a menudo, se quedarían todas esperando).
			if d.cur > others {
				d.cur = others
				go d.notice(fmt.Sprintf("El servidor limita las conexiones simultáneas: se usan %d.", others))
			}
			d.mu.Unlock()
			d.release(w, seg)
			return false
		}
		if le.RetryAfter > 0 {
			// Somos la única: el servidor dice cuánto esperar y nadie pide nada antes.
			if until := time.Now().Add(min(le.RetryAfter, maxCooldown)); until.After(d.coolUntil) {
				d.coolUntil = until
			}
		}
		noted := d.waitNoted
		d.waitNoted = true
		d.mu.Unlock()
		if le.Status != 403 {
			// Somos la única conexión y el servidor pide esperar (429/503): es temporal,
			// así que esperar y reintentar sin rendirse (hasta que el usuario pause).
			if !noted {
				go d.notice(fmt.Sprintf("El servidor pide esperar (%d): se reintenta solo.", le.Status))
			}
			*retries = min(*retries+1, 8)
			pause := max(le.RetryAfter, min(time.Duration(1<<*retries)*time.Second, time.Minute))
			select {
			case <-time.After(min(pause, maxCooldown)):
			case <-d.ctx.Done():
				return false
			}
			return true
		}
	}
	*retries++
	limit := maxRetries
	var he2 *HTTPError
	if others == 0 && !errors.As(err, &he2) {
		limit = maxRetriesAlone // cortes de red con una sola conexión: aguantar más
	}
	if *retries > limit {
		if others > 0 {
			d.release(w, seg)
			return false
		}
		d.fail(fmt.Errorf("demasiados fallos seguidos: %w", err))
		return false
	}
	if !d.rangeOK {
		// Sin rangos no se puede seguir donde iba: vuelta a empezar.
		d.mu.Lock()
		seg.pos, seg.done = 0, 0
		if d.size < 0 {
			seg.end = -1
		}
		d.mu.Unlock()
	}
	select {
	case <-time.After(min(time.Duration(500<<(*retries-1))*time.Millisecond, 30*time.Second)):
	case <-d.ctx.Done():
		return false
	}
	return true
}

func (d *Download) fetch(w *worker, seg *segment) error {
	d.mu.Lock()
	from, to := seg.pos, seg.end
	if d.rangeOK && to >= 0 {
		// Un tramo acotado: al acabarlo la conexión queda libre para el siguiente. En
		// archivos pequeños el tramo también se limita para que haya trabajo para todas.
		div := 2 * max(d.target, 1)
		if d.rateLimit {
			div = max(d.cur, 1) // con límite de peticiones, menos tramos y más grandes
		}
		chunk := min(w.chunk, max(2*minSegment, d.size/int64(div)))
		to = min(to, from+chunk)
		seg.reqEnd = to
	}
	d.mu.Unlock()
	reqEnd := to
	if !d.rangeOK {
		from, reqEnd = 0, -1
	} else if to >= 0 && from >= to {
		return nil
	}
	d.mu.Lock()
	wait := time.Until(d.coolUntil)
	d.mu.Unlock()
	if wait > 0 {
		select {
		case <-time.After(wait):
		case <-d.ctx.Done():
			return context.Cause(d.ctx)
		}
	}
	resp, err := d.do(w, d.url, from, to, d.rangeOK)
	if err != nil {
		return err
	}
	if d.rangeOK {
		switch resp.StatusCode {
		case http.StatusPartialContent:
			cr, ok := parseContentRange(resp.Header.Get("Content-Range"))
			if !ok || cr.start != from {
				resp.Body.Close()
				return errors.New("Content-Range inesperado")
			}
			if cr.total != d.size {
				resp.Body.Close()
				return errChanged
			}
		case http.StatusOK:
			resp.Body.Close()
			etag, lm := resp.Header.Get("ETag"), resp.Header.Get("Last-Modified")
			if (d.etag != "" && etag != "" && etag != d.etag) || (d.etag == "" && d.lastMod != "" && lm != "" && lm != d.lastMod) {
				return errChanged
			}
			return errNoRange
		default:
			resp.Body.Close()
			return httpError(resp)
		}
	} else if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return httpError(resp)
	} else if d.file != nil {
		d.file.Truncate(0)
		if d.size > 0 {
			d.file.Truncate(d.size)
		}
	}
	return d.stream(w, seg, resp, reqEnd)
}

// stream lee la respuesta y la escribe en su sitio. requestedEnd es el fin del tramo
// pedido (-1: hasta el final del cuerpo).
func (d *Download) stream(w *worker, seg *segment, resp *http.Response, requestedEnd int64) (err error) {
	defer resp.Body.Close()
	d.mu.Lock()
	w.streaming = true
	w.since = time.Now()
	w.limitHits = 0
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		w.streaming = false
		d.mu.Unlock()
	}()
	buf := w.buf
	n := 0
	var bufStart int64
	lastFlush := time.Now()
	flush := func() error {
		if n == 0 {
			return nil
		}
		if _, err := d.file.WriteAt(buf[:n], bufStart); err != nil {
			return err
		}
		d.mu.Lock()
		seg.done = bufStart + int64(n)
		d.mu.Unlock()
		n = 0
		lastFlush = time.Now()
		return nil
	}
	defer func() {
		if ferr := flush(); err == nil && ferr != nil {
			err = ferr
		}
	}()

	stall := time.AfterFunc(stallTimeout, func() {
		d.mu.Lock()
		c := w.cancel
		d.mu.Unlock()
		if c != nil {
			c(errStall)
		}
	})
	defer stall.Stop()

	for {
		if n > 0 && (n >= len(buf)-64<<10 || time.Since(lastFlush) > flushEvery) {
			if err := flush(); err != nil {
				return err
			}
		}
		m, rerr := resp.Body.Read(buf[n:])
		if m > 0 {
			stall.Reset(stallTimeout)
			d.mu.Lock()
			if seg.w != w {
				d.mu.Unlock()
				return errYield
			}
			if seg.end >= 0 && int64(m) > seg.end-seg.pos {
				m = int(seg.end - seg.pos)
			}
			if n == 0 {
				bufStart = seg.pos
			}
			seg.pos += int64(m)
			ended := seg.end >= 0 && seg.pos >= seg.end
			end := seg.end
			d.mu.Unlock()
			n += m
			d.received.Add(int64(m))
			w.bytes.Add(int64(m))
			if ended {
				if err := flush(); err != nil {
					return err
				}
				// Recortado por un robo: si queda poco, leerlo y tirarlo para que la
				// conexión se pueda reutilizar en vez de abrir otra.
				if requestedEnd > end && requestedEnd-end <= drainLimit {
					io.CopyN(io.Discard, resp.Body, requestedEnd-end)
				}
				return nil
			}
		}
		if rerr == io.EOF {
			if err := flush(); err != nil {
				return err
			}
			d.mu.Lock()
			target := seg.end
			if requestedEnd >= 0 && (target < 0 || requestedEnd < target) {
				target = requestedEnd
			}
			short := target >= 0 && seg.pos < target
			d.mu.Unlock()
			if short {
				return errors.New("la conexión se cerró antes de tiempo")
			}
			return nil
		}
		if rerr != nil {
			if c := context.Cause(resp.Request.Context()); c != nil && !errors.Is(c, context.Canceled) {
				return c
			}
			return rerr
		}
	}
}

// pickLocked da al trabajador un segmento pendiente o roba la mitad del mayor.
// endgame permite, si ya no queda nada libre, partir el tramo en curso de otro.
func (d *Download) pickLocked(w *worker, endgame bool) *segment {
	if w.retired || d.ctx.Err() != nil || d.restartOf != nil {
		return nil
	}
	for _, s := range d.segs {
		if s.w == nil && !s.complete() {
			s.pos = s.done
			s.w, w.seg = w, s
			return s
		}
	}
	if !d.rangeOK {
		return nil
	}
	// Robo: la mitad de lo que nadie ha pedido todavía en el segmento con más trabajo
	// libre. Lo que está en un tramo en curso no se toca, así no se corta ninguna
	// respuesta y la conexión de la víctima sigue sirviendo.
	var best *segment
	var bestFree int64
	for _, s := range d.segs {
		if s.w != nil && s.end >= 0 {
			if free := s.end - max(s.pos, s.reqEnd); free > bestFree {
				best, bestFree = s, free
			}
		}
	}
	if best == nil || bestFree < 2*minSegment {
		if !endgame {
			return nil
		}
		// Remate: ya no queda nada sin pedir. Partir el tramo en curso más grande; su
		// conexión cortará al llegar al nuevo final (solo pasa al terminar).
		best, bestFree = nil, 0
		for _, s := range d.segs {
			if s.w != nil && s.end >= 0 && s.end-s.pos > bestFree {
				best, bestFree = s, s.end-s.pos
			}
		}
		if best == nil || bestFree < 2*minSegment {
			return nil
		}
	}
	lo := best.pos
	if best.reqEnd > lo && best.end-best.reqEnd >= 2*minSegment {
		lo = best.reqEnd
	}
	bestFree = best.end - lo
	mid := lo + bestFree/2
	if a := mid &^ 4095; a > lo { // alineado a 4 KiB, mejor para el disco
		mid = a
	}
	ns := &segment{start: mid, end: best.end, pos: mid, done: mid, w: w}
	best.end = mid
	i := 0
	for i < len(d.segs) && d.segs[i] != best {
		i++
	}
	d.segs = append(d.segs[:i+1], append([]*segment{ns}, d.segs[i+1:]...)...)
	w.seg = ns
	return ns
}

func (d *Download) hasWorkLocked() bool {
	for _, s := range d.segs {
		if s.w == nil && !s.complete() {
			return true
		}
		if d.rangeOK && s.w != nil && s.end >= 0 && s.end-max(s.pos, s.reqEnd) >= 2*minSegment {
			return true
		}
	}
	return false
}

// release suelta el segmento si sigue siendo de w (otro puede habérselo quedado ya).
func (d *Download) release(w *worker, seg *segment) {
	if seg == nil {
		return
	}
	d.mu.Lock()
	if seg.w == w {
		seg.pos = seg.done
		seg.w = nil
	}
	d.mu.Unlock()
}

func (d *Download) othersActive(w *worker) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, o := range d.workers {
		if o != w && !o.retired && o.streaming {
			n++
		}
	}
	return n
}

func (d *Download) workerExit(w *worker) {
	d.mu.Lock()
	if w.seg != nil && w.seg.w == w {
		w.seg.pos = w.seg.done
		w.seg.w = nil
	}
	delete(d.workers, w.id)
	d.mu.Unlock()
	w.tr.CloseIdleConnections()
	d.poke()
}

// trimLocked retira trabajadores si sobran respecto al objetivo.
func (d *Download) trimLocked() {
	alive := 0
	ids := make([]int, 0, len(d.workers))
	for id, w := range d.workers {
		if !w.retired {
			alive++
			ids = append(ids, id)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ids)))
	for _, id := range ids {
		if alive <= d.cur {
			break
		}
		w := d.workers[id]
		w.retired = true
		if w.seg != nil && w.seg.w == w {
			w.seg.w = nil // el bucle de lectura lo ve y suelta (errYield)
			w.seg.pos = w.seg.done
		}
		if w.cancel != nil {
			w.cancel(errYield)
		}
		alive--
	}
}

func (d *Download) fail(err error) {
	d.mu.Lock()
	if d.failErr == nil {
		d.failErr = err
	}
	d.mu.Unlock()
	d.cancel(err)
}

func (d *Download) requestRestart(err error) {
	d.mu.Lock()
	if d.restartOf == nil {
		d.restartOf = err
		for _, w := range d.workers {
			if w.cancel != nil {
				w.cancel(errRestart)
			}
		}
	}
	d.mu.Unlock()
	d.poke()
}

// ================================================================ bucle principal

func (d *Download) loop(first *http.Response, w0 *worker) (string, error) {
	start := func(w *worker, resp *http.Response) {
		d.wg.Add(1)
		go d.runWorker(w, resp)
	}
	if w0 != nil {
		start(w0, first)
	}
	d.poke() // repartir el resto de conexiones ya, sin esperar al primer tic
	tick := time.NewTicker(tickEvery)
	defer tick.Stop()
	lastSave, lastEvict, lastProbe := time.Now(), time.Now(), time.Now()
	rampAt, rampBytes, rampPrev := time.Now(), d.received.Load(), 0.0
	type sample struct {
		t time.Time
		b int64
	}
	samples := []sample{{time.Now(), d.received.Load()}}

	for {
		select {
		case <-d.ctx.Done():
			return d.shutdown()
		case <-d.wake:
		case now := <-tick.C:
			samples = append(samples, sample{now, d.received.Load()})
			for len(samples) > 2 && now.Sub(samples[0].t) > 3*time.Second {
				samples = samples[1:]
			}
			speed := float64(samples[len(samples)-1].b-samples[0].b) / now.Sub(samples[0].t).Seconds()
			d.emitProgress(speed)
			if now.Sub(lastSave) >= saveEvery {
				d.saveState()
				lastSave = now
			}
			if now.Sub(lastEvict) >= evictEvery {
				d.evictSlow(now.Sub(lastEvict))
				lastEvict = now
			}
			if now.Sub(rampAt) >= rampEvery {
				b := d.received.Load()
				speed := float64(b-rampBytes) / now.Sub(rampAt).Seconds()
				d.mu.Lock()
				if d.rangeOK && !d.limited && !d.plateau && d.cur < d.target {
					if rampPrev == 0 || speed > rampPrev*rampGain {
						d.cur = min(d.target, d.cur*2)
						d.flat = 0
					} else if d.flat++; d.flat >= 2 {
						// Dos mediciones seguidas sin mejorar (las conexiones nuevas tardan un
						// momento en coger velocidad): más conexiones ya no dan más.
						d.plateau = true
					}
				}
				d.mu.Unlock()
				rampAt, rampBytes, rampPrev = now, b, speed
			}
			d.mu.Lock()
			if d.limited && d.cur < d.target && now.Sub(lastProbe) >= probeEvery && now.Sub(d.lastLimit) >= probeEvery {
				// El servidor nos limitó, pero lleva un rato sin quejarse: probar con una
				// conexión más. Si sobra, el 403/429 la retira y todo sigue igual.
				d.cur++
				lastProbe = now
			}
			d.mu.Unlock()
		}

		d.mu.Lock()
		if d.restartOf != nil {
			reason := d.restartOf
			d.mu.Unlock()
			d.wg.Wait()
			return "", d.restart(reason)
		}
		allDone := len(d.segs) > 0
		for _, s := range d.segs {
			if !s.complete() {
				allDone = false
				break
			}
		}
		if allDone && len(d.workers) == 0 {
			d.mu.Unlock()
			return d.finish()
		}
		alive := 0
		for _, w := range d.workers {
			if !w.retired {
				alive++
			}
		}
		limit := d.cur
		if !d.rangeOK {
			limit = 1
		}
		var spawn []*worker
		for alive < limit && d.hasWorkLocked() {
			w := d.newWorker()
			if d.pickLocked(w, false) == nil {
				delete(d.workers, w.id)
				break
			}
			spawn = append(spawn, w)
			alive++
		}
		d.mu.Unlock()
		for _, w := range spawn {
			start(w, nil)
		}
	}
}

func (d *Download) restart(reason error) error {
	d.closeFile()
	os.Remove(d.partPath)
	os.Remove(statePath(d.partPath))
	if errors.Is(reason, errChanged) {
		d.notice("El archivo ha cambiado en el servidor: la descarga empieza de cero.")
	} else {
		d.notice("El servidor dejó de aceptar descargas por partes: se sigue con una sola conexión desde el principio.")
	}
	d.Req.PartPath = ""
	d.restartOf = reason
	d.url, d.etag, d.lastMod = "", "", ""
	return errRestart
}

func (d *Download) shutdown() (string, error) {
	d.wg.Wait()
	cause := context.Cause(d.ctx)
	if errors.Is(cause, ErrCanceled) {
		d.removeFiles()
		return "", ErrCanceled
	}
	d.saveState()
	d.closeFile()
	d.mu.Lock()
	failErr := d.failErr
	d.mu.Unlock()
	if failErr != nil {
		return "", failErr
	}
	if errors.Is(cause, ErrPaused) {
		return "", ErrPaused
	}
	return "", cause
}

func (d *Download) finish() (string, error) {
	d.emitProgress(0)
	if err := d.file.Sync(); err != nil {
		return "", err
	}
	if d.size < 0 { // tamaño desconocido: recortar al real
		var size int64
		for _, s := range d.segs {
			size = max(size, s.end)
		}
		d.file.Truncate(size)
	}
	setSparse(d.file, false)
	d.closeFile()
	final := d.finalPath
	if _, err := os.Stat(final); err == nil {
		final = uniquePath(final)
	}
	var err error
	for i := 0; i < 20; i++ { // el antivirus a veces tiene el archivo abierto un momento
		if err = os.Rename(d.partPath, final); err == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil {
		return "", fmt.Errorf("no se pudo renombrar el archivo terminado: %w", err)
	}
	os.Remove(statePath(d.partPath))
	d.markFromInternet(final)
	return final, nil
}

// markFromInternet pone la marca «descargado de Internet» (Zone.Identifier), como
// hace Chrome: así SmartScreen avisa antes de ejecutarlo y Office lo abre en Vista
// protegida.
func (d *Download) markFromInternet(path string) {
	zone := "[ZoneTransfer]\r\nZoneId=3\r\n"
	if ref := d.Req.Headers["Referer"]; ref != "" && !strings.ContainsAny(ref, "\r\n") {
		zone += "ReferrerUrl=" + ref + "\r\n"
	}
	if u := d.url; u != "" && !strings.ContainsAny(u, "\r\n") {
		zone += "HostUrl=" + u + "\r\n"
	}
	os.WriteFile(path+":Zone.Identifier", []byte(zone), 0o644)
}

// ================================================================ progreso y estado

func (d *Download) emitProgress(speed float64) {
	if d.OnProgress == nil {
		return
	}
	d.mu.Lock()
	p := Progress{Size: d.size, Speed: speed, ETA: -1}
	streaming := false
	for _, w := range d.workers {
		streaming = streaming || w.streaming
	}
	p.Waiting = time.Now().Before(d.coolUntil) || (len(d.workers) > 0 && !streaming)
	for _, s := range d.segs {
		active := int64(0)
		if s.w != nil {
			active = 1
			p.Connections++
		}
		p.BytesDone += s.pos - s.start
		p.Segments = append(p.Segments, [4]int64{s.start, s.end, s.pos, active})
	}
	d.mu.Unlock()
	if d.size > 0 && speed > 0 {
		p.ETA = float64(d.size-p.BytesDone) / speed
	}
	d.OnProgress(p)
}

// saveState: primero Sync() y luego el JSON (escrito aparte y renombrado), así lo
// guardado nunca va por delante de los datos que están de verdad en disco.
func (d *Download) saveState() {
	if d.file == nil || !d.rangeOK || d.partPath == "" {
		return
	}
	d.mu.Lock()
	st := savedState{
		Version: 1, URL: d.Req.URL, FinalURL: d.url, FinalPath: d.finalPath,
		Size: d.size, ETag: d.etag, LastModified: d.lastMod, RangeOK: true,
	}
	for _, s := range d.segs {
		st.Segments = append(st.Segments, [3]int64{s.start, s.end, s.done})
	}
	d.mu.Unlock()
	if err := d.file.Sync(); err != nil {
		return
	}
	raw, _ := json.Marshal(st)
	tmp := statePath(d.partPath) + ".tmp"
	if os.WriteFile(tmp, raw, 0o644) == nil {
		os.Rename(tmp, statePath(d.partPath))
	}
}

// evictSlow sustituye la conexión más lenta si va muy por detrás de las demás.
func (d *Download) evictSlow(window time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var speeds []float64
	var slowest *worker
	for _, w := range d.workers {
		w.speed = float64(w.bytes.Swap(0)) / window.Seconds()
		if w.speed > 0 {
			w.chunk = min(max(int64(w.speed*chunkSeconds), minChunk), maxChunk)
		}
		if w.retired || w.seg == nil || w.seg.w != w || w.seg.end < 0 || time.Since(w.since) < 10*time.Second {
			continue // las recién abiertas aún están arrancando (TCP slow start)
		}
		speeds = append(speeds, w.speed)
		if slowest == nil || w.speed < slowest.speed {
			slowest = w
		}
	}
	// Con pocas conexiones, o si el servidor está limitando (reconectar sería otro
	// rechazo), no se sustituye ninguna.
	if len(speeds) < 6 || slowest == nil || d.limited || d.rateLimit {
		return
	}
	sort.Float64s(speeds)
	median := speeds[len(speeds)/2]
	if slowest.speed < median*0.2 && slowest.seg.end-slowest.seg.pos > 8*minSegment && slowest.cancel != nil {
		d.logf("conexión lenta sustituida: %.0f KB/s frente a una mediana de %.0f KB/s", slowest.speed/1024, median/1024)
		slowest.cancel(errSlow)
	}
}

// ================================================================ utilidades

func statePath(part string) string { return strings.TrimSuffix(part, partExt) + stateExt }

// isPartPath: ruta de un parcial de TurboDL («….turbodl»). Puede ser relativa
// (TurboDL.exe get -o carpeta, desde la terminal).
func isPartPath(p string) bool {
	return len(p) > len(partExt) && strings.EqualFold(filepath.Ext(p), partExt)
}

// validSegments comprueba que los tramos guardados tienen sentido: dentro del
// archivo, ordenados, sin solaparse y con lo hecho dentro de cada uno.
func validSegments(segs [][3]int64, size int64) bool {
	if len(segs) == 0 || size <= 0 {
		return false
	}
	prevEnd := int64(0)
	for _, s := range segs {
		start, end, done := s[0], s[1], s[2]
		if start < prevEnd || start > end || end > size || done < start || done > end {
			return false
		}
		prevEnd = end
	}
	return true
}

func hostOnly(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return strings.ToLower(u.Hostname())
	}
	return ""
}

// sameSiteHost: los dos hosts son del mismo sitio (www.web.com y descargas.web.com
// sí; web.com y otra.com no). Sin lista de sufijos públicos: las dos últimas
// partes, o tres si la penúltima es corta (web.co.uk, web.com.es).
func sameSiteHost(host, base string) bool {
	return host != "" && base != "" && siteOf(host) == siteOf(base)
}

func siteOf(host string) string {
	if net.ParseIP(host) != nil {
		return host
	}
	p := strings.Split(strings.TrimSuffix(host, "."), ".")
	if len(p) <= 2 {
		return strings.Join(p, ".")
	}
	n := 2
	switch p[len(p)-2] {
	case "co", "com", "net", "org", "gov", "gob", "edu", "ac", "or", "ne", "go":
		n = 3
	}
	return strings.Join(p[len(p)-n:], ".")
}

// findResumable busca en dir una descarga a medias de la misma URL.
func findResumable(dir, rawURL string) string {
	matches, _ := filepath.Glob(filepath.Join(dir, "*"+stateExt))
	for _, m := range matches {
		raw, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		var st savedState
		if json.Unmarshal(raw, &st) == nil && st.URL == rawURL {
			part := strings.TrimSuffix(m, stateExt) + partExt
			if _, err := os.Stat(part); err == nil {
				return part
			}
		}
	}
	return ""
}

type contentRange struct{ start, end, total int64 }

func parseContentRange(h string) (contentRange, bool) {
	h = strings.TrimSpace(h)
	if !strings.HasPrefix(strings.ToLower(h), "bytes ") {
		return contentRange{}, false
	}
	rng, total, ok := strings.Cut(h[6:], "/")
	if !ok {
		return contentRange{}, false
	}
	a, b, ok := strings.Cut(rng, "-")
	if !ok {
		return contentRange{}, false
	}
	s, err1 := strconv.ParseInt(strings.TrimSpace(a), 10, 64)
	e, err2 := strconv.ParseInt(strings.TrimSpace(b), 10, 64)
	if err1 != nil || err2 != nil {
		return contentRange{}, false
	}
	t := int64(-1)
	if total != "*" {
		v, err := strconv.ParseInt(strings.TrimSpace(total), 10, 64)
		if err != nil {
			return contentRange{}, false
		}
		t = v
	}
	return contentRange{s, e, t}, true
}

func filenameFrom(resp *http.Response, rawURL string) string {
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil && params["filename"] != "" {
			return params["filename"]
		}
	}
	if u, err := url.Parse(rawURL); err == nil {
		if name, err := url.PathUnescape(path.Base(u.Path)); err == nil && name != "/" && name != "." && name != "" {
			return name
		}
	}
	return "descarga"
}

func sanitizeFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		// Marcas de dirección y de anchura cero: disfrazan nombres («factura\u202Etxt.exe»).
		if r == 0x200B || r == 0x200E || r == 0x200F || r >= 0x202A && r <= 0x202E || r >= 0x2066 && r <= 0x2069 || r == 0xFEFF {
			return -1
		}
		if r < 32 || r == 127 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimRight(strings.TrimSpace(name), ". ")
	if reservedName(name) {
		name = "_" + name
	}
	if len(name) > 200 {
		ext := filepath.Ext(name)
		if len(ext) > 12 {
			ext = ""
		}
		cut := 200 - len(ext)
		for cut > 0 && !utf8.RuneStart(name[cut]) { // no partir una letra de varios bytes
			cut--
		}
		name = strings.TrimRight(name[:cut], ". ") + ext
	}
	if name == "" {
		return "descarga"
	}
	return name
}

// reservedName: nombres que Windows reserva para dispositivos (CON, NUL, COM1…),
// también con extensión («nul.txt»).
func reservedName(name string) bool {
	base := strings.ToLower(strings.TrimRight(strings.SplitN(name, ".", 2)[0], " "))
	switch base {
	case "con", "prn", "aux", "nul", "conin$", "conout$":
		return true
	}
	if len(base) >= 4 && (strings.HasPrefix(base, "com") || strings.HasPrefix(base, "lpt")) {
		switch base[3:] {
		case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "¹", "²", "³":
			return true
		}
	}
	return false
}

// reservePath elige un nombre libre y crea su .turbodl con O_EXCL: si dos descargas
// del mismo archivo empiezan a la vez, cada una se queda con un nombre distinto.
func reservePath(p string) (string, *os.File, error) {
	for i := 0; i < 1000; i++ {
		c := uniquePath(p)
		f, err := os.OpenFile(c+partExt, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			return c, f, nil
		}
		if !os.IsExist(err) {
			return "", nil, err
		}
	}
	return "", nil, fmt.Errorf("no hay un nombre libre para %s", p)
}

// uniquePath añade " (1)", " (2)"... si ya existe el archivo o una descarga a medias.
func uniquePath(p string) string {
	taken := func(c string) bool {
		_, e1 := os.Stat(c)
		_, e2 := os.Stat(c + partExt)
		return e1 == nil || e2 == nil
	}
	if !taken(p) {
		return p
	}
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	for i := 1; ; i++ {
		c := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if !taken(c) {
			return c
		}
	}
}

func fmtBytes(n int64) string {
	f := float64(n)
	for _, u := range []string{"B", "KB", "MB", "GB"} {
		if f < 1024 {
			if u == "B" {
				return fmt.Sprintf("%.0f %s", f, u)
			}
			return fmt.Sprintf("%.1f %s", f, u)
		}
		f /= 1024
	}
	return fmt.Sprintf("%.2f TB", f)
}
