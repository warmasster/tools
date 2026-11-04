package main

// Piezas del equipo y prueba de velocidad de discos (en el agente, que con
// permiso de administrador también ve la salud de los discos).

import (
	"net/http"
	"strconv"
	"sync"

	"turbomon/internal/parts"
	"turbomon/internal/sensors"
)

type diskTestState struct {
	mu      sync.Mutex
	Running bool                      `json:"running"`
	Disk    int                       `json:"disk"`
	Phase   string                    `json:"phase"`
	Pct     float64                   `json:"pct"`
	Error   string                    `json:"error,omitempty"`
	Results map[int]parts.SpeedResult `json:"results"`
}

var diskTest = &diskTestState{Results: map[int]parts.SpeedResult{}}

func (d *diskTestState) snapshot() map[string]any {
	d.mu.Lock()
	defer d.mu.Unlock()
	res := map[string]parts.SpeedResult{}
	for k, v := range d.Results {
		res[strconv.Itoa(k)] = v
	}
	return map[string]any{"running": d.Running, "disk": d.Disk, "phase": d.Phase, "pct": d.Pct, "error": d.Error, "results": res}
}

func (a *Agent) hComponents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, parts.Read(sensors.TotalRAM()))
}

func (a *Agent) hDiskTest(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		n, err := strconv.Atoi(r.URL.Query().Get("disk"))
		if err != nil {
			http.Error(w, "disco", 400)
			return
		}
		letter := ""
		for _, d := range parts.Read(sensors.TotalRAM()).Disks {
			if d.Number == n && len(d.Letters) > 0 {
				letter = d.Letters[0]
			}
		}
		diskTest.mu.Lock()
		switch {
		case diskTest.Running:
			diskTest.mu.Unlock()
			http.Error(w, "ya se está midiendo un disco", 409)
			return
		case letter == "":
			diskTest.mu.Unlock()
			http.Error(w, "ese disco no tiene ninguna unidad (letra) donde probar", 400)
			return
		}
		diskTest.Running, diskTest.Disk, diskTest.Phase, diskTest.Pct, diskTest.Error = true, n, "write", 0, ""
		diskTest.mu.Unlock()
		go func() {
			res, err := parts.SpeedTest(letter, 1024, func(phase string, pct float64) {
				diskTest.mu.Lock()
				diskTest.Phase, diskTest.Pct = phase, pct
				diskTest.mu.Unlock()
			})
			diskTest.mu.Lock()
			diskTest.Running = false
			if err != nil {
				diskTest.Error = err.Error()
			} else {
				diskTest.Results[n] = res
			}
			diskTest.mu.Unlock()
			logf("velocidad del disco %d (%s): lee %.0f MB/s, escribe %.0f MB/s (%v)", n, letter, res.Read, res.Write, err)
		}()
	}
	writeJSON(w, diskTest.snapshot())
}
