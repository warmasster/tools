package main

// «Me va a tirones»: el análisis de tirones de cada partida (quién estaba
// haciendo algo justo en cada tirón y si se repiten con un ritmo) y el mapa de
// ruido de los núcleos (qué núcleos y qué drivers le quitan tiempo al juego).

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"turbomon/internal/noise"
	"turbomon/internal/parts"
	"turbomon/internal/procstat"
	"turbomon/internal/sensors"
	"turbomon/internal/stutter"
)

var (
	pmGetSysMetrics = user32.NewProc("GetSystemMetrics")
	noiseMu         sync.Mutex
	noiseTest       *noise.Test
)

func monitorCount() int {
	n, _, _ := pmGetSysMetrics.Call(80) // SM_CMONITORS
	return int(n)
}

// stutterCtx es lo que pasaba en el equipo este segundo (para el análisis de tirones).
func (a *Agent) stutterCtx(c *liveSession, snap *sensors.Snapshot) stutter.Context {
	ctx := stutter.Context{}
	if snap != nil {
		ctx.DPC = snap.CPU.DPC
		ctx.Limited = snap.CPU.PerfCap > 0 && snap.CPU.PerfCap < 90
		for _, d := range snap.Decode {
			if d.PID != c.pid {
				ctx.Decode = true
			}
		}
	}
	if a.pstat == nil {
		a.pstat = procstat.New(runtime.NumCPU())
	}
	for _, u := range a.pstat.Sample() {
		if u.PID == c.pid {
			ctx.GameHard, ctx.GameRead = u.HardFault, u.ReadMBps
			continue
		}
		name := u.Name
		if u.PID == 4 {
			name = "system"
		}
		ctx.Procs = append(ctx.Procs, stutter.Proc{Name: name, CPU: u.CPU, ReadMB: u.ReadMBps})
	}
	return ctx
}

// Nombres claros de los programas de fondo que más salen.
var knownProcs = map[string]string{
	"system": "Windows (núcleo y drivers)", "msmpeng.exe": "Antivirus de Windows", "mpdefendercoreservice.exe": "Antivirus de Windows",
	"searchindexer.exe": "Indexador de búsqueda de Windows", "searchprotocolhost.exe": "Indexador de búsqueda de Windows",
	"tiworker.exe": "Windows Update", "trustedinstaller.exe": "Windows Update", "musnotification.exe": "Windows Update", "usoclient.exe": "Windows Update",
	"compattelrunner.exe": "Telemetría de Windows", "svchost.exe": "Servicios de Windows", "audiodg.exe": "Audio de Windows",
	"steamwebhelper.exe": "Steam (navegador interno)", "steam.exe": "Steam", "steamservice.exe": "Steam", "epicgameslauncher.exe": "Epic Games",
	"discord.exe": "Discord", "chrome.exe": "Google Chrome", "msedge.exe": "Microsoft Edge", "firefox.exe": "Firefox", "opera.exe": "Opera", "opera_gx.exe": "Opera GX", "brave.exe": "Brave",
	"spotify.exe": "Spotify", "obs64.exe": "OBS", "wallpaper64.exe": "Wallpaper Engine", "wallpaper32.exe": "Wallpaper Engine",
	"icue.exe": "iCUE (Corsair)", "lghub.exe": "G HUB (Logitech)", "lghub_agent.exe": "G HUB (Logitech)", "nzxt cam.exe": "NZXT CAM",
	"armourycrate.service.exe": "Armoury Crate (ASUS)", "armourycrateservice.exe": "Armoury Crate (ASUS)", "lightingservice.exe": "Luces RGB de ASUS",
	"razer synapse service.exe": "Razer Synapse", "rzsdkservice.exe": "Razer Synapse", "signalrgb.exe": "SignalRGB", "openrgb.exe": "OpenRGB",
	"nvcontainer.exe": "NVIDIA (servicios)", "nvidia app.exe": "NVIDIA App", "nvidia share.exe": "Superposición de NVIDIA", "amdrsserv.exe": "AMD Software",
	"onedrive.exe": "OneDrive", "dropbox.exe": "Dropbox", "googledrivefs.exe": "Google Drive", "msiafterburner.exe": "MSI Afterburner", "rtss.exe": "RivaTuner",
	"vmmem": "Máquina virtual (WSL o Hyper-V)", "vmmemwsl": "WSL (Linux en Windows)", "memcompression": "Compresión de memoria de Windows",
	"explorer.exe": "Explorador de Windows", "searchhost.exe": "Buscador de Windows", "widgets.exe": "Widgets de Windows", "phoneexperiencehost.exe": "Enlace Móvil",
}

func prettyProc(exe string) string {
	if v, ok := knownProcs[strings.ToLower(exe)]; ok {
		return v
	}
	return strings.TrimSuffix(exe, ".exe")
}

// stutterReport añade nombres claros a un análisis.
func stutterReport(r stutter.Report) stutter.Report {
	for i := range r.Suspects {
		if r.Suspects[i].Kind == "proc" {
			r.Suspects[i].Name = prettyProc(r.Suspects[i].Name)
		}
	}
	return r
}

// StutterSummary es una partida en la lista de tirones.
type StutterSummary struct {
	ID       string          `json:"id"`
	Game     string          `json:"game"`
	Start    int64           `json:"start"`
	Secs     int             `json:"secs"`
	FPSAvg   float64         `json:"fpsAvg"`
	Low1     float64         `json:"low1"`
	Monitors int             `json:"monitors,omitempty"`
	Stutter  *stutter.Report `json:"stutter"`
}

func (a *Agent) hStutter(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"monitors": monitorCount(), "elevated": a.elevated}
	a.st.mu.Lock()
	if c := a.st.cur; c != nil && c.stut != nil {
		rep := stutterReport(c.stut.Report())
		out["live"] = StutterSummary{Game: c.game, Start: c.start.Unix(), Secs: c.secs, FPSAvg: c.hist.AvgFPS(), Low1: c.hist.Low(0.01), Monitors: c.monitors, Stutter: &rep}
	}
	a.st.mu.Unlock()
	var list []StutterSummary
	for _, s := range readSessionsFull(12) {
		if s.Stutter == nil {
			continue
		}
		rep := stutterReport(*s.Stutter)
		list = append(list, StutterSummary{ID: s.ID, Game: s.Game, Start: s.Start, Secs: s.Secs, FPSAvg: s.FPSAvg, Low1: s.Low1, Monitors: s.Monitors, Stutter: &rep})
	}
	if list == nil {
		list = []StutterSummary{}
	}
	out["sessions"] = list
	writeJSON(w, out)
}

// readSessionsFull lee las últimas n partidas con todo (series y tirones).
func readSessionsFull(n int) []SessionRecord {
	all := readSessions(true)
	if len(all) > n {
		all = all[:n]
	}
	return all
}

// ---------- mapa de ruido ----------

func noisePath() string { return filepath.Join(dataDir(), "ruido.json") }

func lastNoise() *noise.State {
	b, err := os.ReadFile(noisePath())
	if err != nil {
		return nil
	}
	var s noise.State
	if json.Unmarshal(b, &s) != nil {
		return nil
	}
	return &s
}

func noiseCores() []noise.LP {
	var out []noise.LP
	for _, c := range parts.CPUSets() {
		out = append(out, noise.LP{ID: c.Logical, Phys: c.Core, Class: c.Efficiency})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (a *Agent) hNoise(w http.ResponseWriter, r *http.Request) {
	noiseMu.Lock()
	defer noiseMu.Unlock()
	if r.Method == "POST" && (noiseTest == nil || !noiseTest.Get().Running) {
		if a.st.active() {
			http.Error(w, "cierra el juego antes: la prueba usa los núcleos uno a uno", http.StatusConflict)
			return
		}
		noiseTest = noise.Start(noise.Options{Exe: a.exe, PerCore: time.Second, Cores: noiseCores(), Deep: a.elevated, Describe: fileDescription})
		logf("mapa de ruido: empieza")
		go func(t *noise.Test) {
			for t.Get().Running {
				time.Sleep(500 * time.Millisecond)
			}
			s := t.Get()
			b, _ := json.Marshal(s)
			os.WriteFile(noisePath(), b, 0o644)
			logf("mapa de ruido: acaba (%d núcleos, %d drivers, %s)", len(s.Cores), len(s.Drivers), s.Error)
		}(noiseTest)
	}
	if noiseTest != nil {
		if s := noiseTest.Get(); s.Running {
			writeJSON(w, map[string]any{"state": s})
			return
		}
	}
	writeJSON(w, map[string]any{"state": lastNoise()})
}
