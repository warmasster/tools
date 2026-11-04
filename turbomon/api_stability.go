package main

// «¿Es estable mi PC?»: la prueba de núcleos y RAM corre en el agente (sigue
// aunque se cierre la ventana) y el último resultado se guarda.

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"turbomon/internal/sensors"
	"turbomon/internal/stability"
)

var (
	stabMu   sync.Mutex
	stabTest *stability.Test
)

func stabilityPath() string { return filepath.Join(dataDir(), "estabilidad.json") }

func lastStability() *stability.State {
	b, err := os.ReadFile(stabilityPath())
	if err != nil {
		return nil
	}
	var s stability.State
	if json.Unmarshal(b, &s) != nil {
		return nil
	}
	return &s
}

func (a *Agent) hStability(w http.ResponseWriter, r *http.Request) {
	stabMu.Lock()
	defer stabMu.Unlock()
	if r.Method == "POST" {
		if r.URL.Query().Get("stop") == "1" {
			if stabTest != nil {
				stabTest.Stop()
			}
		} else if stabTest == nil || !stabTest.Get().Running {
			// Hasta 2 GB de RAM, sin pasar de un tercio de la que hay libre.
			mb := 2048
			if m := sensors.AvailableRAM(); m > 0 && int(m/3>>20) < mb {
				mb = int(m / 3 >> 20)
			}
			stabTest = stability.Start(stability.Options{PerThread: 3 * time.Second, AllCores: 15 * time.Second, RAMMB: mb, Threads: runtime.NumCPU()})
			logf("prueba de estabilidad: empieza (%d MB de RAM)", mb)
			go func(t *stability.Test) {
				for t.Get().Running {
					time.Sleep(time.Second)
				}
				s := t.Get()
				b, _ := json.Marshal(s)
				os.WriteFile(stabilityPath(), b, 0o644)
				bad := 0
				for _, c := range s.Cores {
					if c.Errors > 0 {
						bad++
					}
				}
				logf("prueba de estabilidad: acaba (%d hilos con errores, %d errores de RAM, parada: %v)", bad, s.RAMErrors, s.Stopped)
			}(stabTest)
		}
	}
	if stabTest != nil {
		if s := stabTest.Get(); s.Running {
			writeJSON(w, map[string]any{"state": s})
			return
		}
	}
	writeJSON(w, map[string]any{"state": lastStability()})
}
