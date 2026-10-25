package main

// Host de Native Messaging ("com.turbodl.native"). Chrome lo arranca cuando la
// extensión llama a chrome.runtime.connectNative y le habla por stdin/stdout con
// mensajes JSON precedidos de su longitud (uint32, little endian).
//
// Órdenes (de la extensión):  hello · start · pause · cancel · setConnections ·
//                             open · show · stat
// Eventos (a la extensión):   info · progress · notice · status
// Cada orden con reqId recibe {type:"reply", reqId, ok, result | error}.
//
// stdout es el canal del protocolo: nada de prints. El log va a AppDir()/native.log.

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type hostMsg struct {
	ReqID int    `json:"reqId,omitempty"`
	Cmd   string `json:"cmd"`
	ID    int64  `json:"id,omitempty"`
	N     int    `json:"n,omitempty"`
	Path  string `json:"path,omitempty"`
	Request
}

type job struct {
	d    *Download
	done chan struct{}
}

type host struct {
	out  *bufio.Writer
	wmu  sync.Mutex
	mu   sync.Mutex
	jobs map[int64]*job
}

func runHost() {
	setupLog()
	log.Printf("host iniciado (%s)", version)
	h := &host{out: bufio.NewWriterSize(os.Stdout, 64<<10), jobs: map[int64]*job{}}
	in := bufio.NewReaderSize(os.Stdin, 64<<10)
	for {
		var size uint32
		if err := binary.Read(in, binary.LittleEndian, &size); err != nil {
			break // Chrome cerró el puerto
		}
		if size > 64<<20 {
			log.Printf("mensaje demasiado grande: %d", size)
			break
		}
		raw := make([]byte, size)
		if _, err := io.ReadFull(in, raw); err != nil {
			break
		}
		var m hostMsg
		if err := json.Unmarshal(raw, &m); err != nil {
			log.Printf("JSON no válido: %v", err)
			continue
		}
		go h.handle(m)
	}
	// Sin extensión al otro lado: pausar todo guardando el estado para reanudar luego.
	log.Printf("puerto cerrado: pausando %d descarga(s)", len(h.jobs))
	h.mu.Lock()
	jobs := make([]*job, 0, len(h.jobs))
	for _, j := range h.jobs {
		j.d.Pause()
		jobs = append(jobs, j)
	}
	h.mu.Unlock()
	for _, j := range jobs {
		select {
		case <-j.done:
		case <-time.After(10 * time.Second):
		}
	}
}

func (h *host) send(v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.wmu.Lock()
	defer h.wmu.Unlock()
	binary.Write(h.out, binary.LittleEndian, uint32(len(raw)))
	h.out.Write(raw)
	h.out.Flush()
}

func (h *host) reply(m hostMsg, result any, err error) {
	if m.ReqID == 0 {
		return
	}
	if err != nil {
		h.send(map[string]any{"type": "reply", "reqId": m.ReqID, "ok": false, "error": err.Error()})
		return
	}
	h.send(map[string]any{"type": "reply", "reqId": m.ReqID, "ok": true, "result": result})
}

func (h *host) handle(m hostMsg) {
	// Un mensaje raro no debe tirar el proceso (y con él todas las descargas).
	defer func() {
		if r := recover(); r != nil {
			log.Printf("error interno con %q: %v", m.Cmd, r)
			h.reply(m, nil, fmt.Errorf("error interno"))
		}
	}()
	switch m.Cmd {
	case "hello":
		h.reply(m, map[string]any{
			"version": version, "downloadsDir": DownloadsDir(),
			"maxConnections": MaxConnections, "defaultConnections": DefaultConnections,
		}, nil)
	case "start":
		h.reply(m, nil, h.start(m))
	case "pause":
		h.stop(m.ID, false)
		h.reply(m, nil, nil)
	case "cancel":
		if !h.stop(m.ID, true) && isPartPath(m.PartPath) {
			os.Remove(m.PartPath)
			os.Remove(statePath(m.PartPath))
		}
		h.reply(m, nil, nil)
	case "setConnections":
		h.mu.Lock()
		for id, j := range h.jobs {
			if m.ID == 0 || id == m.ID {
				j.d.SetConnections(m.N)
			}
		}
		h.mu.Unlock()
		h.reply(m, nil, nil)
	case "open":
		h.reply(m, nil, openPath(m.Path))
	case "show":
		h.reply(m, nil, showInFolder(m.Path))
	case "stat":
		fi, err := os.Stat(m.Path)
		h.reply(m, map[string]any{"exists": err == nil, "size": sizeOf(fi)}, nil)
	case "ping":
		h.reply(m, "pong", nil)
	default:
		h.reply(m, nil, fmt.Errorf("orden desconocida: %s", m.Cmd))
	}
}

// redact quita la query de una URL para el log: suele llevar tokens de descarga.
func redact(raw string) string {
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		return raw[:i] + "?…"
	}
	return raw
}

func sizeOf(fi os.FileInfo) int64 {
	if fi == nil {
		return 0
	}
	return fi.Size()
}

func (h *host) start(m hostMsg) error {
	h.mu.Lock()
	if _, ok := h.jobs[m.ID]; ok {
		h.mu.Unlock()
		return nil // ya está en marcha: la orden es idempotente
	}
	if m.Dir == "" {
		m.Dir = DownloadsDir()
	}
	d := NewDownload(m.Request)
	id := m.ID
	d.Logf = func(format string, args ...any) { log.Printf("[%d] "+format, append([]any{id}, args...)...) }
	d.OnInfo = func(i Info) { h.send(map[string]any{"type": "info", "id": id, "info": i}) }
	d.OnNotice = func(s string) {
		log.Printf("[%d] aviso: %s", id, s)
		h.send(map[string]any{"type": "notice", "id": id, "message": s})
	}
	d.OnProgress = func(p Progress) {
		h.send(map[string]any{"type": "progress", "id": id, "progress": p})
	}
	j := &job{d: d, done: make(chan struct{})}
	h.jobs[id] = j
	h.mu.Unlock()

	log.Printf("[%d] inicio %s (%d conexiones)", id, redact(m.URL), d.Req.Connections)
	h.send(map[string]any{"type": "status", "id": id, "status": "downloading"})
	go func() {
		defer close(j.done)
		path, err := d.Run(context.Background())
		h.mu.Lock()
		delete(h.jobs, id)
		h.mu.Unlock()
		switch {
		case err == nil:
			log.Printf("[%d] terminada: %s", id, path)
			fi, _ := os.Stat(path)
			h.send(map[string]any{"type": "status", "id": id, "status": "done", "path": path, "size": sizeOf(fi)})
		case errors.Is(err, ErrPaused):
			log.Printf("[%d] pausada", id)
			h.send(map[string]any{"type": "status", "id": id, "status": "paused"})
		case errors.Is(err, ErrCanceled):
			log.Printf("[%d] cancelada", id)
			h.send(map[string]any{"type": "status", "id": id, "status": "cancelled"})
		default:
			log.Printf("[%d] error: %v", id, err)
			h.send(map[string]any{"type": "status", "id": id, "status": "error", "error": errorText(err)})
		}
	}()
	return nil
}

// stop pausa o cancela y espera a que termine (con tope). false si no existía.
func (h *host) stop(id int64, cancel bool) bool {
	h.mu.Lock()
	j, ok := h.jobs[id]
	h.mu.Unlock()
	if !ok {
		return false
	}
	if cancel {
		j.d.Cancel()
	} else {
		j.d.Pause()
	}
	select {
	case <-j.done:
	case <-time.After(10 * time.Second):
	}
	return true
}

// errorText traduce los errores a algo entendible en el gestor.
func errorText(err error) string {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "El servidor no responde. Puede que haya bloqueado tu IP un rato por abrir demasiadas conexiones: el progreso está guardado, reanuda en unos minutos (o baja las conexiones)."
	}
	var he *HTTPError
	if errors.As(err, &he) {
		switch he.Status {
		case 401, 403:
			return fmt.Sprintf("El servidor denegó el acceso (%d). Puede que el enlace haya caducado o haga falta iniciar sesión.", he.Status)
		case 404, 410:
			return fmt.Sprintf("El archivo ya no existe en el servidor (%d).", he.Status)
		}
	}
	return err.Error()
}

func setupLog() {
	dir := AppDir()
	os.MkdirAll(dir, 0o755)
	p := filepath.Join(dir, "native.log")
	if fi, err := os.Stat(p); err == nil && fi.Size() > 2<<20 {
		os.Rename(p, p+".old")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.SetOutput(io.Discard)
		return
	}
	log.SetOutput(f)
}
