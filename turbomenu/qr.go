package main

// «Pasar al móvil» y «Recibir del móvil»: un servidor que solo vive mientras la ventana
// está abierta, accesible desde internet por Cloudflare (HTTPS, funciona siempre) y,
// si el router deja, directo por la IP pública con el puerto abierto por UPnP. Al cerrar
// la ventana se apaga todo y se libera el puerto del router.

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"rsc.io/qr"
)

//go:embed web
var webFS embed.FS

type ShareFile struct {
	Path    string `json:"-"`
	Rel     string `json:"name"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"-"`
}

type qrDownload struct {
	Name  string  `json:"name"`
	Bytes int64   `json:"bytes"`
	Total int64   `json:"total"`
	Done  bool    `json:"done"`
	Speed float64 `json:"speed"`
	start time.Time
}

type qrUpload struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Final string `json:"final"`
	Size  int64  `json:"size"`
	Got   int64  `json:"got"`
	Done  bool   `json:"done"`
	part  string
	mu    sync.Mutex
}

type qrSession struct {
	mode   string // send | receive
	token  string
	nonce  string
	name   string
	files  []ShareFile
	total  int64
	dest   string
	ln     net.Listener
	srv    *http.Server
	port   int
	tunnel *Tunnel
	direct *Direct

	mu        sync.Mutex
	downloads []*qrDownload
	uploads   map[string]*qrUpload
	visits    int
	lastPush  time.Time
	onChange  func()
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// collectFiles recorre archivos y carpetas (las carpetas conservan su estructura).
func collectFiles(paths []string) ([]ShareFile, int64) {
	var files []ShareFile
	var total int64
	used := map[string]int{}
	uniq := func(rel string) string {
		k := strings.ToLower(rel)
		n := used[k]
		used[k] = n + 1
		if n == 0 {
			return rel
		}
		ext := filepath.Ext(rel)
		return fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(rel, ext), n+1, ext)
	}
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if !st.IsDir() {
			files = append(files, ShareFile{Path: p, Rel: uniq(st.Name()), Size: st.Size(), ModTime: st.ModTime().Unix()})
			total += st.Size()
			continue
		}
		root := uniq(filepath.Base(p))
		filepath.WalkDir(p, func(fp string, d fs.DirEntry, err error) error {
			if err != nil {
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(p, fp)
			files = append(files, ShareFile{Path: fp, Rel: root + "/" + filepath.ToSlash(rel), Size: info.Size(), ModTime: info.ModTime().Unix()})
			total += info.Size()
			return nil
		})
	}
	return files, total
}

func newQRSession(mode string, paths []string, dest string, onChange func()) (*qrSession, error) {
	s := &qrSession{mode: mode, token: randHex(12), nonce: randHex(16), dest: dest, uploads: map[string]*qrUpload{}, onChange: onChange}
	if mode == "send" {
		s.files, s.total = collectFiles(paths)
		if len(s.files) == 0 {
			return nil, errors.New("No hay nada que enviar (¿carpeta vacía?)")
		}
		s.name = filepath.Base(paths[0])
		if len(paths) > 1 {
			s.name = fmt.Sprintf("%d archivos", len(s.files))
		}
	} else {
		s.name = filepath.Base(dest)
	}
	// Puerto fijo si se puede (el router lo recuerda mejor), si no, cualquiera.
	var err error
	host := ""
	if testMode {
		host = "127.0.0.1" // en pruebas, sin aviso del firewall
	}
	for port := 47832; port < 47842; port++ {
		s.ln, err = net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
		if err == nil {
			break
		}
	}
	if s.ln == nil {
		s.ln, err = net.Listen("tcp", ":0")
		if err != nil {
			return nil, errors.New("No se pudo abrir ningún puerto en este PC")
		}
	}
	s.port = s.ln.Addr().(*net.TCPAddr).Port
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, s.nonce) })
	mux.HandleFunc("GET /q/{tok}", s.guard(s.handlePage))
	mux.HandleFunc("GET /q/{tok}/", s.guard(s.handlePage))
	mux.HandleFunc("GET /q/{tok}/info", s.guard(s.handleInfo))
	mux.HandleFunc("GET /q/{tok}/f/{i}", s.guard(s.handleFile))
	mux.HandleFunc("HEAD /q/{tok}/f/{i}", s.guard(s.handleFile))
	mux.HandleFunc("GET /q/{tok}/zip", s.guard(s.handleZip))
	mux.HandleFunc("POST /q/{tok}/up", s.guard(s.handleUpStart))
	mux.HandleFunc("PUT /q/{tok}/up/{id}", s.guard(s.handleUpChunk))
	mux.HandleFunc("GET /q/{tok}/a/{file}", s.guard(s.handleAsset))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(404)
		io.WriteString(w, expiredPage)
	})
	s.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 20 * time.Second}
	go s.srv.Serve(s.ln)

	s.tunnel = newTunnel(cacheDir(), s.nonce, s.changed)
	s.direct = newDirect(s.nonce, s.changed)
	go s.tunnel.Start(s.port, "", "")
	go s.direct.Start(s.port, true, false)
	return s, nil
}

const expiredPage = `<!doctype html><meta name=viewport content="width=device-width,initial-scale=1"><body style="margin:0;min-height:100vh;display:grid;place-items:center;background:#0a0b12;color:#eceef8;font:16px system-ui;text-align:center"><div><div style="font-size:44px">⏳</div><h2>Este enlace ya no funciona</h2><p style="color:#9196ae">Se cerró la ventana en el PC. Pide un QR nuevo.</p></div>`

func (s *qrSession) changed() {
	if s.onChange != nil {
		s.onChange()
	}
}

func (s *qrSession) guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("tok") != s.token {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(404)
			io.WriteString(w, expiredPage)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Robots-Tag", "noindex")
		h(w, r)
	}
}

func (s *qrSession) stop() {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); s.direct.Stop() }() // quita el puerto del router
	go func() { defer wg.Done(); s.tunnel.Stop() }()
	s.srv.Close()
	wg.Wait()
	s.mu.Lock()
	for _, u := range s.uploads {
		if !u.Done && u.part != "" {
			os.Remove(u.part)
		}
	}
	s.mu.Unlock()
}

// ---------- páginas ----------

func (s *qrSession) handlePage(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/") {
		http.Redirect(w, r, r.URL.Path+"/", http.StatusFound)
		return
	}
	b, _ := webFS.ReadFile("web/phone.html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(b)
	s.mu.Lock()
	s.visits++
	s.mu.Unlock()
	s.changed()
}

func (s *qrSession) handleAsset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	b, err := webFS.ReadFile("web/" + filepath.Base(name))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if ct := mime.TypeByExtension(filepath.Ext(name)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Write(b)
}

func (s *qrSession) handleInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	host, _ := os.Hostname()
	json.NewEncoder(w).Encode(map[string]any{
		"mode": s.mode, "name": s.name, "files": s.files, "total": s.total, "pc": host,
		"zip": len(s.files) > 1,
	})
}

func contentDisposition(name string) string {
	ascii := strings.Map(func(r rune) rune {
		if r < 32 || r > 126 || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii, url.PathEscape(name))
}

func (s *qrSession) track(name string, total int64) *qrDownload {
	d := &qrDownload{Name: name, Total: total, start: time.Now()}
	s.mu.Lock()
	s.downloads = append(s.downloads, d)
	if len(s.downloads) > 40 {
		s.downloads = s.downloads[len(s.downloads)-40:]
	}
	s.mu.Unlock()
	s.changed()
	return d
}

type progressWriter struct {
	w io.Writer
	d *qrDownload
	s *qrSession
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.s.mu.Lock()
	p.d.Bytes += int64(n)
	if el := time.Since(p.d.start).Seconds(); el > 0.3 {
		p.d.Speed = float64(p.d.Bytes) / el
	}
	push := time.Since(p.s.lastPush) > 300*time.Millisecond
	if push {
		p.s.lastPush = time.Now()
	}
	p.s.mu.Unlock()
	if push {
		p.s.changed()
	}
	return n, err
}

func (s *qrSession) handleFile(w http.ResponseWriter, r *http.Request) {
	i, err := strconv.Atoi(r.PathValue("i"))
	if err != nil || i < 0 || i >= len(s.files) {
		http.NotFound(w, r)
		return
	}
	f := s.files[i]
	fh, err := os.Open(f.Path)
	if err != nil {
		http.Error(w, "El archivo ya no está en el PC", http.StatusGone)
		return
	}
	defer fh.Close()
	w.Header().Set("Content-Disposition", contentDisposition(filepath.Base(f.Rel)))
	if ct := mime.TypeByExtension(filepath.Ext(f.Rel)); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	if r.Method == http.MethodHead {
		http.ServeContent(w, r, "", time.Unix(f.ModTime, 0), fh)
		return
	}
	d := s.track(filepath.Base(f.Rel), f.Size)
	pw := &rangeTracker{ResponseWriter: w, pw: &progressWriter{w: w, d: d, s: s}}
	http.ServeContent(pw, r, "", time.Unix(f.ModTime, 0), fh)
	s.mu.Lock()
	d.Done = d.Bytes >= d.Total
	s.mu.Unlock()
	s.changed()
}

// rangeTracker cuenta los bytes que realmente salen hacia el móvil.
type rangeTracker struct {
	http.ResponseWriter
	pw *progressWriter
}

func (t *rangeTracker) Write(b []byte) (int, error) { return t.pw.Write(b) }

func (s *qrSession) handleZip(w http.ResponseWriter, r *http.Request) {
	entries, size := zipLayout(s.files)
	name := strings.TrimSuffix(s.name, filepath.Ext(s.name)) + ".zip"
	h := w.Header()
	h.Set("Content-Type", "application/zip")
	h.Set("Content-Length", strconv.FormatInt(size, 10))
	h.Set("Content-Disposition", contentDisposition(name))
	w.WriteHeader(200)
	d := s.track(name, size)
	buf := make([]byte, 1<<20)
	writeZip(&progressWriter{w: w, d: d, s: s}, entries, buf)
	s.mu.Lock()
	d.Done = d.Bytes >= d.Total
	s.mu.Unlock()
	s.changed()
}

// ---------- subidas desde el móvil ----------

func cleanFileName(n string) string {
	n = filepath.Base(strings.ReplaceAll(n, "\\", "/"))
	n = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, n)
	n = strings.Trim(n, " .")
	if n == "" {
		n = "archivo"
	}
	if len([]rune(n)) > 180 {
		n = string([]rune(n)[:180])
	}
	return n
}

func uniquePath(dir, name string) string {
	p := filepath.Join(dir, name)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return p
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 2; ; i++ {
		p = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return p
		}
	}
}

func (s *qrSession) handleUpStart(w http.ResponseWriter, r *http.Request) {
	if s.mode != "receive" {
		http.Error(w, "no", http.StatusForbidden)
		return
	}
	var req struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&req) != nil || req.Size < 0 {
		http.Error(w, "petición incorrecta", 400)
		return
	}
	u := &qrUpload{ID: randHex(8), Name: cleanFileName(req.Name), Size: req.Size}
	u.part = filepath.Join(s.dest, "."+u.ID+".turbomenu-part")
	f, err := os.Create(u.part)
	if err != nil {
		http.Error(w, "No se puede guardar en esa carpeta", 500)
		return
	}
	f.Close()
	s.mu.Lock()
	s.uploads[u.ID] = u
	s.mu.Unlock()
	if req.Size == 0 {
		s.finishUpload(u)
	}
	s.changed()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": u.ID})
}

const maxChunk = 32 << 20

func (s *qrSession) handleUpChunk(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	u := s.uploads[r.PathValue("id")]
	s.mu.Unlock()
	if u == nil {
		http.Error(w, "subida desconocida", 404)
		return
	}
	off, err := strconv.ParseInt(r.URL.Query().Get("off"), 10, 64)
	if err != nil || off < 0 {
		http.Error(w, "posición incorrecta", 400)
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.Done {
		json.NewEncoder(w).Encode(map[string]any{"got": u.Got, "done": true})
		return
	}
	if off != u.Got {
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(map[string]any{"got": u.Got})
		return
	}
	f, err := os.OpenFile(u.part, os.O_WRONLY, 0)
	if err != nil {
		http.Error(w, "No se puede escribir", 500)
		return
	}
	f.Seek(off, io.SeekStart)
	n, err := io.Copy(f, io.LimitReader(r.Body, min(maxChunk, u.Size-off)))
	f.Close()
	u.Got += n
	s.changed()
	if err != nil {
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(map[string]any{"got": u.Got})
		return
	}
	if u.Got >= u.Size {
		s.finishUpload(u)
	}
	json.NewEncoder(w).Encode(map[string]any{"got": u.Got, "done": u.Done})
}

func (s *qrSession) finishUpload(u *qrUpload) {
	final := uniquePath(s.dest, u.Name)
	if err := os.Rename(u.part, final); err == nil {
		u.Final = final
		u.Done = true
		notifyChanged(s.dest)
	}
	s.changed()
}

// ---------- estado para la ventana ----------

type QRState struct {
	Mode       string        `json:"mode"`
	Name       string        `json:"name"`
	Count      int           `json:"count"`
	Total      int64         `json:"total"`
	Dest       string        `json:"dest"`
	Cloudflare string        `json:"cloudflare"`
	CFStatus   string        `json:"cfStatus"`
	CFMessage  string        `json:"cfMessage"`
	CFProgress float64       `json:"cfProgress"`
	Direct     string        `json:"direct"`
	DirStatus  string        `json:"dirStatus"`
	DirMessage string        `json:"dirMessage"`
	QRCF       string        `json:"qrCF"`     // SVG
	QRDirect   string        `json:"qrDirect"` // SVG
	Visits     int           `json:"visits"`
	Downloads  []qrDownload  `json:"downloads"`
	Uploads    []qrUploadVew `json:"uploads"`
}

type qrUploadVew struct {
	Name  string `json:"name"`
	Final string `json:"final"`
	Size  int64  `json:"size"`
	Got   int64  `json:"got"`
	Done  bool   `json:"done"`
}

var qrCache sync.Map

func qrSVG(text string) string {
	if text == "" {
		return ""
	}
	if v, ok := qrCache.Load(text); ok {
		return v.(string)
	}
	c, err := qr.Encode(text, qr.M)
	if err != nil {
		return ""
	}
	n := c.Size
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="-2 -2 %d %d" shape-rendering="crispEdges"><path fill="currentColor" d="`, n+4, n+4)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if c.Black(x, y) {
				run := 1
				for x+run < n && c.Black(x+run, y) {
					run++
				}
				fmt.Fprintf(&b, "M%d %dh%dv1h-%dz", x, y, run, run)
				x += run - 1
			}
		}
	}
	b.WriteString(`"/></svg>`)
	qrCache.Store(text, b.String())
	return b.String()
}

func (s *qrSession) state() QRState {
	ts := s.tunnel.State()
	ds := s.direct.State()
	st := QRState{Mode: s.mode, Name: s.name, Count: len(s.files), Total: s.total, Dest: s.dest,
		CFStatus: ts.Status, CFMessage: ts.Message, CFProgress: ts.Progress,
		DirStatus: ds.Status, DirMessage: ds.Message}
	path := "/q/" + s.token + "/"
	if ts.Status == "online" && ts.URL != "" {
		st.Cloudflare = ts.URL + path
		st.QRCF = qrSVG(st.Cloudflare)
	}
	if b := s.direct.Base(); b != "" {
		st.Direct = b + path
		st.QRDirect = qrSVG(st.Direct)
	}
	s.mu.Lock()
	st.Visits = len(s.downloads) + s.visits
	for _, d := range s.downloads {
		st.Downloads = append(st.Downloads, *d)
	}
	for _, u := range s.uploads {
		u.mu.Lock()
		st.Uploads = append(st.Uploads, qrUploadVew{Name: u.Name, Final: u.Final, Size: u.Size, Got: u.Got, Done: u.Done})
		u.mu.Unlock()
	}
	s.mu.Unlock()
	return st
}

// ---------- métodos para la interfaz ----------

// StartQR arranca el servidor para pasar los archivos del trabajo actual al móvil
// (mode "send") o para recibir en la carpeta del trabajo (mode "receive").
func (a *App) StartQR(mode string) error {
	a.stopQR()
	job := a.currentJob()
	if job == nil || len(job.Paths) == 0 {
		return errors.New("No hay nada seleccionado")
	}
	var s *qrSession
	var err error
	gen := a.gen
	push := func() {
		if a.qr != nil && a.gen == gen {
			a.emit("qr", a.qr.state())
		}
	}
	if mode == "receive" {
		dest := job.Paths[0]
		if st, e := os.Stat(dest); e != nil || !st.IsDir() {
			dest = filepath.Dir(dest)
		}
		s, err = newQRSession("receive", nil, dest, push)
	} else {
		s, err = newQRSession("send", job.Paths, "", push)
	}
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.qr = s
	a.mu.Unlock()
	push()
	return nil
}

func (a *App) QRState() *QRState {
	a.mu.Lock()
	s := a.qr
	a.mu.Unlock()
	if s == nil {
		return nil
	}
	st := s.state()
	return &st
}

func (a *App) stopQR() {
	a.mu.Lock()
	s := a.qr
	a.qr = nil
	a.mu.Unlock()
	if s != nil {
		s.stop()
	}
}

func (a *App) StopQR() { a.stopQR() }

func (a *App) currentJob() *Job {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.job == nil {
		return nil
	}
	j := *a.job
	j.Paths = append([]string(nil), a.job.Paths...)
	return &j
}
