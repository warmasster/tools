package main

import (
	"embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed all:web
var webFS embed.FS

const maxChunk = 16 << 20

var copyBufPool = sync.Pool{New: func() any { b := make([]byte, 1<<20); return &b }}

type PublicServer struct {
	app   *App
	srv   *http.Server
	ln    net.Listener
	nonce string
	web   fs.FS
}

func newPublicServer(app *App) *PublicServer {
	sub, _ := fs.Sub(webFS, "web")
	return &PublicServer{app: app, nonce: newID(), web: sub}
}

// Start escucha en todas las interfaces (necesario para modo directo y LAN).
func (p *PublicServer) Start(port int) (int, error) {
	var ln net.Listener
	var err error
	for i := 0; i < 20; i++ {
		ln, err = net.Listen("tcp", fmt.Sprintf(":%d", port+i))
		if err == nil {
			port = port + i
			break
		}
	}
	if err != nil {
		return 0, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", p.handleRoot)
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		io.WriteString(w, p.nonce)
	})
	mux.HandleFunc("GET /assets/{file}", p.handleAsset)
	mux.HandleFunc("GET /s/{id}", p.handlePage)
	mux.HandleFunc("GET /api/s/{id}", p.handleInfo)
	mux.HandleFunc("GET /d/{id}/{i}", p.handleFile)
	mux.HandleFunc("GET /c/{id}/{i}", p.handleChunk)
	mux.HandleFunc("GET /m/{id}", p.handleBatch)
	mux.HandleFunc("GET /z/{id}", p.handleZip)
	mux.HandleFunc("POST /fin/{id}", p.handleFin)
	mux.HandleFunc("/", p.handleNotFound)

	p.ln = ln
	p.srv = &http.Server{
		Handler:           secHeaders(mux),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go p.srv.Serve(ln)
	return port, nil
}

func (p *PublicServer) Close() {
	if p.srv != nil {
		p.srv.Close()
	}
}

func secHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("Referrer-Policy", "no-referrer")
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("X-Robots-Tag", "noindex, nofollow")
		h.ServeHTTP(w, r)
	})
}

// clientInfo distingue Cloudflare (llega por loopback con Cf-Connecting-IP), LAN y directo.
func clientInfo(r *http.Request) (ip, via string) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	rip := net.ParseIP(host)
	if rip != nil && rip.IsLoopback() {
		if cf := r.Header.Get("Cf-Connecting-Ip"); cf != "" {
			return cf, "cloudflare"
		}
		return host, "local"
	}
	if rip != nil && (rip.IsPrivate() || rip.IsLinkLocalUnicast()) {
		return host, "lan"
	}
	return host, "direct"
}

func (p *PublicServer) serveWeb(w http.ResponseWriter, name string, status int) {
	b, err := fs.ReadFile(p.web, name)
	if err != nil {
		http.Error(w, "no encontrado", 404)
		return
	}
	ct := "text/html; charset=utf-8"
	switch {
	case strings.HasSuffix(name, ".css"):
		ct = "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".js"):
		ct = "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".svg"):
		ct = "image/svg+xml"
	case strings.HasSuffix(name, ".png"):
		ct = "image/png"
	}
	w.Header().Set("Content-Type", ct)
	if strings.HasSuffix(name, ".html") {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	w.WriteHeader(status)
	w.Write(b)
}

func (p *PublicServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	p.serveWeb(w, "index.html", 200)
}

func (p *PublicServer) handleNotFound(w http.ResponseWriter, r *http.Request) {
	p.serveWeb(w, "404.html", 404)
}

func (p *PublicServer) handleAsset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if strings.Contains(name, "..") || strings.HasSuffix(name, ".html") {
		p.handleNotFound(w, r)
		return
	}
	p.serveWeb(w, name, 200)
}

func (p *PublicServer) handlePage(w http.ResponseWriter, r *http.Request) {
	if p.app.shares.Get(r.PathValue("id")) == nil {
		p.handleNotFound(w, r)
		return
	}
	p.serveWeb(w, "share.html", 200)
}

type infoFile struct {
	Rel  string `json:"rel"`
	Size int64  `json:"size"`
}

func (p *PublicServer) handleInfo(w http.ResponseWriter, r *http.Request) {
	sh := p.app.shares.Get(r.PathValue("id"))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if sh == nil {
		w.WriteHeader(404)
		io.WriteString(w, `{"error":"not_found"}`)
		return
	}
	_, via := clientInfo(r)
	files := make([]infoFile, len(sh.Files))
	for i, f := range sh.Files {
		files[i] = infoFile{Rel: f.Rel, Size: f.Size}
	}
	set := p.app.getSettings()
	out := map[string]any{
		"id":      sh.ID,
		"name":    sh.Name,
		"kind":    sh.Kind,
		"total":   sh.Total,
		"files":   files,
		"created": sh.Created,
		"via":     via,
		"conns":   set.Connections,
		"chunk":   8 << 20,
	}
	if sh.Kind == "multi" {
		_, zsize := zipLayout(sh.Files)
		out["zipSize"] = zsize
	}
	if via == "cloudflare" && set.ShowDirectOnTunnel {
		if base := p.app.directBase(); base != "" {
			out["direct"] = base + "/s/" + sh.ID
		}
	}
	json.NewEncoder(w).Encode(out)
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

func (p *PublicServer) fileFor(w http.ResponseWriter, r *http.Request) (*Share, int, bool) {
	sh := p.app.shares.Get(r.PathValue("id"))
	if sh == nil {
		http.Error(w, "este envío ya no existe", http.StatusGone)
		return nil, 0, false
	}
	i, err := strconv.Atoi(r.PathValue("i"))
	if err != nil || i < 0 || i >= len(sh.Files) {
		http.Error(w, "archivo no válido", 404)
		return nil, 0, false
	}
	return sh, i, true
}

func baseName(rel string) string {
	if k := strings.LastIndex(rel, "/"); k >= 0 {
		return rel[k+1:]
	}
	return rel
}

// countingRW cuenta los bytes del cuerpo que salen hacia el cliente.
type countingRW struct {
	http.ResponseWriter
	t  *Transfer
	tr *Tracker
}

func (c *countingRW) Write(b []byte) (int, error) {
	n, err := c.ResponseWriter.Write(b)
	c.t.Add(int64(n))
	c.tr.Out(int64(n))
	return n, err
}

// handleFile sirve un archivo completo con soporte de Range (reanudable).
func (p *PublicServer) handleFile(w http.ResponseWriter, r *http.Request) {
	sh, i, ok := p.fileFor(w, r)
	if !ok {
		return
	}
	sf := sh.Files[i]
	f, err := os.Open(sf.Path)
	if err != nil {
		http.Error(w, "el archivo ya no está disponible en el PC de origen", http.StatusGone)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.Error(w, "error leyendo el archivo", 500)
		return
	}
	ip, via := clientInfo(r)
	key := fmt.Sprintf("d|%s|%s|%d", ip, sh.ID, i)
	t := p.app.tracker.Get(key, func() *Transfer {
		return &Transfer{ShareID: sh.ID, Label: baseName(sf.Rel), Via: via, IP: ip, Total: st.Size()}
	})
	if start, ok := rangeStart(r.Header.Get("Range")); ok {
		p.app.tracker.SetBytes(t, start)
	} else {
		p.app.tracker.SetBytes(t, 0)
	}
	t.Begin()
	defer t.End()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", contentDisposition(baseName(sf.Rel)))
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(&countingRW{ResponseWriter: w, t: t, tr: p.app.tracker}, r, "", st.ModTime(), f)
}

func rangeStart(h string) (int64, bool) {
	if !strings.HasPrefix(h, "bytes=") {
		return 0, false
	}
	spec := strings.TrimPrefix(h, "bytes=")
	if strings.Contains(spec, ",") {
		return 0, false
	}
	a, _, found := strings.Cut(spec, "-")
	if !found || a == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(a, 10, 64)
	return n, err == nil
}

func blockSize(q url.Values) int64 {
	bs, _ := strconv.ParseInt(q.Get("bs"), 10, 64)
	if bs < 1<<20 || bs > maxChunk {
		bs = 8 << 20
	}
	return bs
}

func (p *PublicServer) turboTransfer(r *http.Request, sh *Share, single *ShareFile) *Transfer {
	ip, via := clientInfo(r)
	tid := r.URL.Query().Get("t")
	if len(tid) > 64 {
		tid = tid[:64]
	}
	return p.app.tracker.Get("c|"+ip+"|"+tid, func() *Transfer {
		label, total := sh.Name, sh.Total
		if sh.Kind == "file" && single != nil {
			label, total = baseName(single.Rel), single.Size
		}
		return &Transfer{ShareID: sh.ID, Label: label, Via: via, IP: ip, Total: total, Turbo: true}
	})
}

// streamBlocks envía [off, off+ln) del archivo en bloques de bs bytes, cada uno
// seguido de 4 bytes con su CRC32 (big endian). Los datos salen según se leen del disco.
func (p *PublicServer) streamBlocks(w http.ResponseWriter, f *os.File, off, ln, bs int64, t *Transfer, buf []byte) bool {
	crc := crc32.NewIEEE()
	var tail [4]byte
	for done := int64(0); done < ln; {
		blk := bs
		if ln-done < blk {
			blk = ln - done
		}
		crc.Reset()
		sec := io.NewSectionReader(f, off+done, blk)
		var sent int64
		for sent < blk {
			n, rerr := sec.Read(buf)
			if n > 0 {
				crc.Write(buf[:n])
				wn, werr := w.Write(buf[:n])
				sent += int64(wn)
				t.Add(int64(wn))
				p.app.tracker.Out(int64(wn))
				if werr != nil {
					return false
				}
			}
			if rerr != nil && sent < blk {
				return false // archivo truncado: cortamos para que el cliente reintente
			}
		}
		binary.BigEndian.PutUint32(tail[:], crc.Sum32())
		if _, err := w.Write(tail[:]); err != nil {
			return false
		}
		done += blk
	}
	return true
}

// handleChunk sirve un tramo de un archivo para la descarga turbo del navegador.
func (p *PublicServer) handleChunk(w http.ResponseWriter, r *http.Request) {
	sh, i, ok := p.fileFor(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	off, err1 := strconv.ParseInt(q.Get("off"), 10, 64)
	ln, err2 := strconv.ParseInt(q.Get("len"), 10, 64)
	bs := blockSize(q)
	sf := sh.Files[i]
	if err1 != nil || err2 != nil || off < 0 || ln <= 0 || off+ln > sf.Size {
		http.Error(w, "tramo no válido", 400)
		return
	}
	f, err := os.Open(sf.Path)
	if err != nil {
		http.Error(w, "el archivo ya no está disponible en el PC de origen", http.StatusGone)
		return
	}
	defer f.Close()
	t := p.turboTransfer(r, sh, &sf)
	t.Begin()
	defer t.End()

	nblocks := (ln + bs - 1) / bs
	h := w.Header()
	h.Set("Content-Type", "application/octet-stream")
	h.Set("Content-Length", strconv.FormatInt(ln+4*nblocks, 10))
	h.Set("Cache-Control", "no-store, no-transform")
	w.WriteHeader(200)

	bp := copyBufPool.Get().(*[]byte)
	defer copyBufPool.Put(bp)
	if !p.streamBlocks(w, f, off, ln, bs, t, (*bp)[:256<<10]) {
		panic(http.ErrAbortHandler)
	}
}

// handleBatch envía varios archivos pequeños seguidos en una sola respuesta
// (cada uno como un bloque con su CRC), para no pagar una petición por archivo.
func (p *PublicServer) handleBatch(w http.ResponseWriter, r *http.Request) {
	sh := p.app.shares.Get(r.PathValue("id"))
	if sh == nil {
		http.Error(w, "este envío ya no existe", http.StatusGone)
		return
	}
	parts := strings.Split(r.URL.Query().Get("f"), ",")
	if len(parts) == 0 || len(parts) > 1000 {
		http.Error(w, "lista no válida", 400)
		return
	}
	var idx []int
	var total int64
	for _, s := range parts {
		i, err := strconv.Atoi(s)
		if err != nil || i < 0 || i >= len(sh.Files) || sh.Files[i].Size > maxChunk {
			http.Error(w, "lista no válida", 400)
			return
		}
		idx = append(idx, i)
		total += sh.Files[i].Size + 4
	}
	t := p.turboTransfer(r, sh, nil)
	t.Begin()
	defer t.End()

	h := w.Header()
	h.Set("Content-Type", "application/octet-stream")
	h.Set("Content-Length", strconv.FormatInt(total, 10))
	h.Set("Cache-Control", "no-store, no-transform")
	w.WriteHeader(200)

	bp := copyBufPool.Get().(*[]byte)
	defer copyBufPool.Put(bp)
	var tail [4]byte
	for _, i := range idx {
		sf := sh.Files[i]
		if sf.Size == 0 {
			binary.BigEndian.PutUint32(tail[:], 0)
			if _, err := w.Write(tail[:]); err != nil {
				return
			}
			continue
		}
		f, err := os.Open(sf.Path)
		if err != nil {
			panic(http.ErrAbortHandler)
		}
		ok := p.streamBlocks(w, f, 0, sf.Size, maxChunk, t, (*bp)[:256<<10])
		f.Close()
		if !ok {
			panic(http.ErrAbortHandler)
		}
	}
}

func (p *PublicServer) handleFin(w http.ResponseWriter, r *http.Request) {
	sh := p.app.shares.Get(r.PathValue("id"))
	tid := r.URL.Query().Get("t")
	if sh == nil || tid == "" {
		w.WriteHeader(204)
		return
	}
	ip, _ := clientInfo(r)
	p.app.tracker.Finish("c|" + ip + "|" + tid)
	w.WriteHeader(204)
}

// handleZip genera un ZIP sin compresión al vuelo con Content-Length exacto.
func (p *PublicServer) handleZip(w http.ResponseWriter, r *http.Request) {
	sh := p.app.shares.Get(r.PathValue("id"))
	if sh == nil {
		http.Error(w, "este envío ya no existe", http.StatusGone)
		return
	}
	entries, size := zipLayout(sh.Files)
	ip, via := clientInfo(r)
	key := fmt.Sprintf("z|%s|%s|%d", ip, sh.ID, time.Now().UnixNano())
	t := p.app.tracker.Get(key, func() *Transfer {
		return &Transfer{ShareID: sh.ID, Label: sh.Name + ".zip", Via: via, IP: ip, Total: size}
	})
	t.Begin()
	defer t.End()

	h := w.Header()
	h.Set("Content-Type", "application/zip")
	h.Set("Content-Length", strconv.FormatInt(size, 10))
	h.Set("Content-Disposition", contentDisposition(sh.Name+".zip"))
	h.Set("Cache-Control", "no-store, no-transform")
	w.WriteHeader(200)
	if r.Method == http.MethodHead {
		return
	}
	bp := copyBufPool.Get().(*[]byte)
	defer copyBufPool.Put(bp)
	writeZip(&countingRW{ResponseWriter: w, t: t, tr: p.app.tracker}, entries, *bp)
}
