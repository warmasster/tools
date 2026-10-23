package main

// Host de Native Messaging: mensajes JSON por stdin/stdout precedidos de su
// longitud (4 bytes, little-endian). La extensión abre el puerto, pide "update" y
// lo cierra al recibir la respuesta.

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type hostRequest struct {
	ID    int    `json:"id"`
	Cmd   string `json:"cmd"`
	Build string `json:"build"`
	Force bool   `json:"force"`
}

var outMu sync.Mutex

func send(v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	outMu.Lock()
	defer outMu.Unlock()
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(raw)))
	os.Stdout.Write(n[:])
	os.Stdout.Write(raw)
}

func runHost() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	in := bufio.NewReader(os.Stdin)
	var wg sync.WaitGroup
	for {
		var n [4]byte
		if _, err := io.ReadFull(in, n[:]); err != nil {
			break // Chrome cerró el puerto
		}
		size := binary.LittleEndian.Uint32(n[:])
		if size > 1<<20 {
			break
		}
		buf := make([]byte, size)
		if _, err := io.ReadFull(in, buf); err != nil {
			break
		}
		var req hostRequest
		if json.Unmarshal(buf, &req) != nil {
			continue
		}
		switch req.Cmd {
		case "hello":
			send(map[string]any{"id": req.ID, "ok": true, "version": version})
		case "update":
			wg.Add(1)
			go func() {
				defer wg.Done()
				res := updateAll(ctx, req.Force, req.Build)
				send(map[string]any{"id": req.ID, "ok": res.Error == "", "result": res})
			}()
		default:
			send(map[string]any{"id": req.ID, "ok": false, "error": "orden desconocida"})
		}
	}
	cancel()
	wg.Wait()
}

// logf escribe en %LOCALAPPDATA%\TurboBLOCK\native.log (se recorta al pasar de 1 MB).
func logf(format string, a ...any) {
	p := filepath.Join(AppDir(), "native.log")
	if st, err := os.Stat(p); err == nil && st.Size() > 1<<20 {
		os.Rename(p, p+".1")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, a...))
}
