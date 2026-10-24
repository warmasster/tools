//go:build e2e

package main

// Solo en la compilación de pruebas (dev/run-tests.js): un servidor local para
// que las pruebas manejen la ventana como si se pulsaran sus botones. No existe
// en TurboVAULT.exe normal.

import (
	"encoding/json"
	"net"
	"net/http"
	"os"

	"time"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var e2eReport = make(chan string, 1)

// E2EReport recibe lo que la interfaz cuenta de sí misma (solo en pruebas).
func (a *App) E2EReport(s string) {
	select {
	case e2eReport <- s:
	default:
	}
}

func guardWebView() {}

func webviewOptions(*options.App) {}

func e2eHooks(a *App) {
	port := os.Getenv("TURBOVAULT_E2E_PORT")
	token := os.Getenv("TURBOVAULT_E2E_TOKEN")
	if port == "" || len(token) < 16 {
		return
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		return
	}
	go http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Token") != token {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		if r.URL.Path == "/ui" {
			// Ejecutar JS en la ventana y devolver lo que conteste.
			js := r.URL.Query().Get("js")
			select {
			case <-e2eReport:
			default:
			}
			runtime.WindowExecJS(a.ctx, "Promise.resolve().then(async () => { try { const v = await ("+js+"); window.go.main.App.E2EReport(JSON.stringify({ ok: true, v })); } catch (e) { window.go.main.App.E2EReport(JSON.stringify({ ok: false, e: String(e) })); } })")
			select {
			case s := <-e2eReport:
				w.Write([]byte(s))
			case <-time.After(5 * time.Second):
				http.Error(w, "sin respuesta", http.StatusGatewayTimeout)
			}
			return
		}
		var req struct {
			Cmd  string          `json:"cmd"`
			Args json.RawMessage `json:"args"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		json.NewEncoder(w).Encode(a.Call(req.Cmd, req.Args))
	}))
}

func testEnv(name string) string { return os.Getenv(name) }
