package main

// El agente: lo único de TurboMON que está siempre abierto. Vive en la bandeja,
// escucha los fotogramas (ETW), lee los sensores una vez por segundo solo si
// alguien los mira (overlay, ventana o una partida) y si no, uno ligero cada 5 s,
// y apunta las partidas. No tiene ventana web: unos 15 MB de memoria.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"turbomon/internal/fps"
	"turbomon/internal/procstat"
	"turbomon/internal/sensors"
)

const (
	agentWindowClass = "TurboMONAgent"
	agentMutexName   = `Local\TurboMON.Agent`
	histLen          = 90
)

// FPSView es la medida de FPS de lo que hay delante.
type FPSView struct {
	Available bool   `json:"available"` // se pueden medir (hay permiso)
	Reason    string `json:"reason,omitempty"`
	PID       uint32 `json:"pid"`
	Exe       string `json:"exe"`
	Name      string `json:"name"`
	IsGame    bool   `json:"isGame"`
	fps.Stats
}

// AgentView: cómo está el propio TurboMON.
type AgentView struct {
	Version    string  `json:"version"`
	Elevated   bool    `json:"elevated"`
	CPU        float64 `json:"cpu"`   // % de CPU que gasta el agente
	MemMB      float64 `json:"memMB"` // memoria del agente
	Overlay    bool    `json:"overlay"`
	HistBytes  int64   `json:"histBytes"`
	Sessions   int     `json:"sessions"`
	Task       bool    `json:"task"` // arranca con Windows como administrador
	Hotkey     bool    `json:"hotkey"`
	StartedAgo int64   `json:"startedAgo"`
}

// Live es todo lo que ve la ventana.
type Live struct {
	sensors.Snapshot
	FPS     FPSView              `json:"fps"`
	Session *SessionView         `json:"session"`
	Hist    map[string][]float32 `json:"hist"`
	Agent   AgentView            `json:"agent"`
}

// Agent es el proceso de la bandeja.
type Agent struct {
	exe      string
	elevated bool
	started  time.Time

	smu     sync.Mutex
	sampler *sensors.Sampler
	tracker *fps.Tracker
	etw     *fps.Session
	fpsErr  string

	mu       sync.Mutex
	live     Live
	hist     map[string][]float32
	lastPoll time.Time
	procs    map[uint32]procInfo

	ui    *ui
	st    *sessionTracker
	token string
	ln    net.Listener
	kickC chan struct{}
	quit  chan struct{}
	once  sync.Once

	selfPrev  time.Duration
	selfAt    time.Time
	selfCPU   float64
	selfMemMB float64
	lastOv    bool
	win       windowCtl
	net       *netState
	pstat     *procstat.Sampler // qué gasta cada proceso (solo en partida, desde el bucle)
}

func runAgent() int {
	mutexName, _ := windows.UTF16PtrFromString(agentMutexName + testEnv("TURBOMON_DIR"))
	mtx, err := windows.CreateMutex(nil, false, mutexName)
	if err == windows.ERROR_ALREADY_EXISTS || err == windows.ERROR_ACCESS_DENIED {
		// Ya hay uno. Si este es administrador y el otro no, lo sustituye (al
		// dar permiso para los FPS); si no, se va.
		if !isElevated() {
			return 0
		}
		info, ok := readAgentInfo()
		if ok && info.Elevated {
			return 0
		}
		agentCall("POST", "/quit", nil, nil, 2*time.Second)
		if ok {
			if h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(info.PID)); err == nil {
				windows.WaitForSingleObject(h, 5000)
				windows.CloseHandle(h)
			}
		}
	} else if err != nil {
		return 1
	}
	if mtx != 0 {
		defer windows.CloseHandle(mtx)
	}

	procSetDpiContext.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
	exe, _ := os.Executable()
	a := &Agent{
		exe: exe, elevated: isElevated(), started: time.Now(),
		hist: map[string][]float32{}, procs: map[uint32]procInfo{},
		kickC: make(chan struct{}, 1), quit: make(chan struct{}),
	}
	a.st = &sessionTracker{a: a}
	a.net = newNetState()
	os.MkdirAll(dataDir(), 0o755)
	logf("agente %s arrancado (administrador: %v)", version, a.elevated)
	restoreGovernor() // primero: su plan «de antes» puede ser el del modo Turbo
	restorePlan()
	restoreWifi()
	a.sampler = sensors.NewSampler()
	a.tracker = fps.NewTracker(fps.Freq())
	if a.elevated {
		if s, err := fps.Start("TurboMON-FPS"+testEnv("TURBOMON_DIR"), a.tracker.Present); err != nil {
			a.fpsErr = err.Error()
			logf("FPS: %v", err)
		} else {
			a.etw = s
		}
	} else {
		a.fpsErr = fps.ErrNeedAdmin.Error()
	}
	if err := a.serve(); err != nil {
		logf("no se pudo abrir el servidor local: %v", err)
		return 1
	}
	a.ui = startUI(a)
	if s := getSettings(); !s.Welcomed {
		updateSettings(func(s *Settings) { s.Welcomed = true })
		a.ui.notify("TurboMON está aquí abajo", "Un clic en este icono pone o quita el overlay de FPS (o Ctrl+Alt+O). Doble clic abre TurboMON.")
	}
	a.startProcessWatch()
	go a.loop()
	go a.watchFocus()
	go a.pingLoop()
	<-a.quit
	a.ui.close()
	a.st.mu.Lock()
	a.st.finish() // si había una partida, se guarda
	a.st.mu.Unlock()
	a.etw.Stop()
	a.sampler.Close()
	if a.ln != nil {
		a.ln.Close()
	}
	os.Remove(agentInfoPath())
	logf("agente cerrado")
	return 0
}

func (a *Agent) shutdown() { a.once.Do(func() { close(a.quit) }) }

// kick hace que el bucle dé una vuelta ya (tras un cambio de ajustes).
func (a *Agent) kick() {
	select {
	case a.kickC <- struct{}{}:
	default:
	}
}

func (a *Agent) loop() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	tick := 0
	var last sensors.Snapshot
	for {
		select {
		case <-a.quit:
			return
		case <-t.C:
		case <-a.kickC:
		}
		tick++
		func() {
			defer func() {
				if r := recover(); r != nil {
					logf("fallo en el bucle: %v", r)
				}
			}()
			last = a.step(tick, last)
		}()
	}
}

func (a *Agent) procInfo(pid uint32) procInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	if pi, ok := a.procs[pid]; ok {
		return pi
	}
	if len(a.procs) > 200 {
		a.procs = map[uint32]procInfo{}
	}
	pi := newProcInfo(pid)
	a.procs[pid] = pi
	return pi
}

func (a *Agent) step(tick int, last sensors.Snapshot) sensors.Snapshot {
	now := fps.Now()
	hwnd, pid := foreground()
	var info procInfo
	if pid != 0 && pid != uint32(os.Getpid()) {
		info = a.procInfo(pid)
	}
	var st fps.Stats
	if a.etw != nil && pid != 0 {
		st = a.tracker.Stats(pid, now)
	}
	s := getSettings()
	a.mu.Lock()
	watching := time.Since(a.lastPoll) < 4*time.Second
	a.mu.Unlock()
	inSession := a.st.active()
	gameFg := isGame(info, hwnd, st)
	full := s.Overlay.On || watching || inSession || gameFg
	snap := last
	if full || tick%5 == 0 || last.Time == 0 {
		a.smu.Lock()
		snap = a.sampler.Sample(full)
		a.smu.Unlock()
	}
	a.st.update(pid, info, hwnd, st, &snap)
	a.applyWindowRules(hwnd, pid)
	a.netTick()
	if tick%30 == 0 {
		a.tracker.Prune(now)
	}
	if tick%5 == 1 {
		a.measureSelf()
	}

	fv := FPSView{Available: a.etw != nil, Reason: a.fpsErr, PID: pid, Exe: info.exe, Name: info.game, IsGame: gameFg, Stats: st}
	a.mu.Lock()
	if snap.Full && snap.Time != a.live.Time {
		push := func(k string, v float64) {
			h := append(a.hist[k], float32(math.Round(v*10)/10))
			if len(h) > histLen {
				h = h[len(h)-histLen:]
			}
			a.hist[k] = h
		}
		push("cpu", snap.CPU.Usage)
		push("ram", float64(snap.Mem.Used)/(1<<30))
		if g := snap.MainGPU(); g != nil {
			push("gpu", g.Usage)
			push("gpuTemp", g.Temp)
			push("vram", float64(g.VRAMUsed)/(1<<30))
		}
		if snap.CPU.Temp > 0 {
			push("cpuTemp", snap.CPU.Temp)
		}
		push("fps", st.FPS)
		push("disk", snap.DiskBusy)
		push("net", (snap.NetDown+snap.NetUp)/1024)
	}
	a.live = Live{Snapshot: snap, FPS: fv, Hist: a.hist, Agent: AgentView{
		Version: version, Elevated: a.elevated, CPU: a.selfCPU, MemMB: a.selfMemMB, Overlay: s.Overlay.On,
		Task: taskInstalled(), Hotkey: s.Overlay.Hotkey, StartedAgo: int64(time.Since(a.started).Seconds()),
	}}
	a.mu.Unlock()

	if s.Overlay.On || a.lastOv {
		ping, srv := a.overlayPing()
		a.ui.ov.set(buildOverlay(&snap, fv, s.Overlay, hwnd, gameFg, ping, srv))
		a.lastOv = s.Overlay.On
	}
	return snap
}

// measureSelf mide lo que gasta el propio agente (para enseñarlo).
func (a *Agent) measureSelf() {
	var c, e, k, u windows.Filetime
	if windows.GetProcessTimes(windows.CurrentProcess(), &c, &e, &k, &u) == nil {
		used := time.Duration((uint64(k.HighDateTime)<<32|uint64(k.LowDateTime))+(uint64(u.HighDateTime)<<32|uint64(u.LowDateTime))) * 100
		if !a.selfAt.IsZero() {
			wall := time.Since(a.selfAt)
			ncpu := float64(a.sampler.Info().Threads)
			if ncpu == 0 {
				ncpu = 1
			}
			a.selfCPU = math.Round(float64(used-a.selfPrev)/float64(wall)/ncpu*100*100) / 100
		}
		a.selfPrev, a.selfAt = used, time.Now()
	}
	var pmc [80]byte
	*(*uint32)(unsafe.Pointer(&pmc[0])) = 80
	if r, _, _ := procGetProcessMemoryInfo.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&pmc[0])), 80); r != 0 {
		// La memoria física que ocupa (lo que enseña el Administrador de tareas).
		ws := *(*uintptr)(unsafe.Pointer(&pmc[16]))
		a.selfMemMB = math.Round(float64(ws)/(1<<20)*10) / 10
	}
}

var procGetProcessMemoryInfo = kernel32.NewProc("K32GetProcessMemoryInfo")

func (a *Agent) menuHeader() string {
	a.mu.Lock()
	l := a.live
	a.mu.Unlock()
	parts := []string{fmt.Sprintf("CPU %.0f %%", l.CPU.Usage)}
	if g := l.MainGPU(); g != nil {
		parts = append(parts, fmt.Sprintf("GPU %.0f %%", g.Usage))
	}
	parts = append(parts, fmt.Sprintf("RAM %s/%s", gbShort(l.Mem.Used), gbShort(l.Mem.Total)))
	if l.FPS.Presenting && l.FPS.IsGame {
		parts = append([]string{fmt.Sprintf("%.0f FPS", l.FPS.FPS)}, parts...)
	}
	return "TurboMON · " + strings.Join(parts, " · ")
}

func (a *Agent) toggleOverlay() {
	s := a.changeSettings(func(s *Settings) { s.Overlay.On = !s.Overlay.On })
	logf("overlay %v", s.Overlay.On)
}

func (a *Agent) changeSettings(f func(*Settings)) Settings {
	s := updateSettings(f)
	a.ui.refresh()
	a.kick()
	return s
}

// openWindow abre la ventana. Siendo administrador, se abre a través del
// Explorador para que la ventana no lo sea (no le hace falta).
func (a *Agent) openWindow() {
	var cmd *exec.Cmd
	if a.elevated {
		cmd = exec.Command(filepath.Join(os.Getenv("SystemRoot"), "explorer.exe"), a.exe)
	} else {
		cmd = exec.Command(a.exe)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS}
	if err := cmd.Start(); err != nil {
		logf("no se pudo abrir la ventana: %v", err)
		return
	}
	go cmd.Wait()
}

// enableFPS pide permiso de administrador para medir los FPS.
func (a *Agent) enableFPS() {
	if err := runElevated("setup-admin"); err != nil {
		if !errors.Is(err, errUACCancelled) {
			a.ui.notify("No se pudo activar", err.Error())
		}
		return
	}
	setRunKey(false)
	// El agente con permiso arranca y sustituye a este.
	startAgentTask()
}

// overlayPing: el último ping al servidor del juego (si contesta) o a internet.
func (a *Agent) overlayPing() (float64, bool) {
	n := a.net
	n.mu.Lock()
	on, has, no := n.on, n.hasSrv, n.noICMP
	n.mu.Unlock()
	if !on {
		return 0, false
	}
	if has && !no {
		if s := n.gamePing.Stats(); s.Last > 0 {
			return s.Last, true
		}
	}
	return n.inetPing.Stats().Last, false
}

// ---------- overlay ----------

func dec(v float64, d int) string {
	return strings.Replace(fmt.Sprintf("%.*f", d, v), ".", ",", 1)
}

func gbShort(b uint64) string {
	v := float64(b) / (1 << 30)
	if v >= 100 {
		return dec(v, 0)
	}
	return dec(v, 1)
}

func humanBytes(b uint64) string {
	switch {
	case b >= 1<<30:
		return dec(float64(b)/(1<<30), 1) + " GB"
	case b >= 1<<20:
		return dec(float64(b)/(1<<20), 0) + " MB"
	}
	return dec(float64(b)/1024, 0) + " KB"
}

func fpsColor(v float64) uint32 {
	switch {
	case v >= 60:
		return colGood
	case v >= 30:
		return colMid
	}
	return colBad
}

func tempColor(v float64) uint32 {
	switch {
	case v >= 85:
		return colBad
	case v >= 75:
		return colMid
	}
	return colValue
}

func buildOverlay(snap *sensors.Snapshot, fv FPSView, o OverlaySettings, anchor uintptr, isGame bool, pingMs float64, pingSrv bool) ovContent {
	c := ovContent{show: o.On && (!o.GameOnly || isGame), anchor: anchor, corner: o.Corner, scale: o.Scale, opacity: o.Opacity}
	if !c.show {
		return c
	}
	// FPS
	var segs []seg
	if fv.Available && fv.Presenting && fv.FPS > 0 {
		segs = append(segs, seg{text: dec(fv.FPS, 0), color: fpsColor(fv.FPS), font: fontBig}, seg{text: " FPS", color: colDim, font: fontSmall})
		if !o.FPSOnly && o.Low1 && fv.Low1 > 0 {
			segs = append(segs, seg{text: "   1 % ", color: colDim, font: fontSmall}, seg{text: dec(fv.Low1, 0), color: fpsColor(fv.Low1), font: fontNormal})
		}
		if !o.FPSOnly && o.FrameMs && fv.FrameMs > 0 {
			segs = append(segs, seg{text: "   " + dec(fv.FrameMs, 1) + " ms", color: colDim, font: fontSmall})
		}
	} else {
		segs = append(segs, seg{text: "—", color: colDim, font: fontBig}, seg{text: " FPS", color: colDim, font: fontSmall})
		if !fv.Available {
			segs = append(segs, seg{text: "   sin permiso", color: colDim, font: fontSmall})
		}
	}
	c.lines = append(c.lines, ovLine{segs: segs})
	if o.FPSOnly {
		return c
	}
	if o.CPU || o.CPUTemp || o.Clock {
		var s []seg
		if o.CPU {
			s = append(s, seg{text: dec(snap.CPU.Usage, 0) + " %", color: colValue, font: fontNormal})
		}
		if o.CPUTemp && snap.CPU.Temp > 0 {
			s = append(s, seg{text: "  " + dec(snap.CPU.Temp, 0) + "°", color: tempColor(snap.CPU.Temp), font: fontNormal})
		}
		if o.Clock && snap.CPU.MHz > 0 {
			s = append(s, seg{text: "  " + dec(float64(snap.CPU.MHz)/1000, 1) + " GHz", color: colDim, font: fontSmall})
		}
		if o.Power && snap.CPU.PowerW > 0 {
			s = append(s, seg{text: "  " + dec(snap.CPU.PowerW, 0) + " W", color: colDim, font: fontSmall})
		}
		if len(s) > 0 {
			c.lines = append(c.lines, ovLine{label: "CPU", segs: s})
		}
	}
	if g := snap.MainGPU(); g != nil && (o.GPU || o.GPUTemp || o.Clock) {
		var s []seg
		if o.GPU {
			s = append(s, seg{text: dec(g.Usage, 0) + " %", color: colValue, font: fontNormal})
		}
		if o.GPUTemp && g.Temp > 0 {
			s = append(s, seg{text: "  " + dec(g.Temp, 0) + "°", color: tempColor(g.Temp), font: fontNormal})
		}
		if o.Clock && g.ClockMHz > 0 {
			s = append(s, seg{text: "  " + fmt.Sprint(g.ClockMHz) + " MHz", color: colDim, font: fontSmall})
		}
		if len(s) > 0 {
			c.lines = append(c.lines, ovLine{label: "GPU", segs: s})
		}
	}
	if o.RAM && snap.Mem.Total > 0 {
		c.lines = append(c.lines, ovLine{label: "RAM", segs: []seg{
			{text: gbShort(snap.Mem.Used), color: colValue, font: fontNormal},
			{text: " / " + dec(math.Round(float64(snap.Mem.Total)/(1<<30)), 0) + " GB", color: colDim, font: fontSmall},
		}})
	}
	if o.Ping && pingMs > 0 {
		col := uint32(colValue)
		switch {
		case pingMs >= 100:
			col = colBad
		case pingMs >= 60:
			col = colMid
		}
		lbl := " ms (internet)"
		if pingSrv {
			lbl = " ms"
		}
		c.lines = append(c.lines, ovLine{label: "PING", segs: []seg{{text: dec(pingMs, 0), color: col, font: fontNormal}, {text: lbl, color: colDim, font: fontSmall}}})
	}
	if g := snap.MainGPU(); o.VRAM && g != nil && g.VRAM > 0 && g.VRAMUsed > 0 {
		c.lines = append(c.lines, ovLine{label: "VRAM", segs: []seg{
			{text: gbShort(g.VRAMUsed), color: colValue, font: fontNormal},
			{text: " / " + dec(math.Round(float64(g.VRAM)/(1<<30)), 0) + " GB", color: colDim, font: fontSmall},
		}})
	}
	return c
}

// ---------- servidor local para la ventana ----------

// agentInfo se guarda en agente.json para que la ventana sepa dónde llamar.
type agentInfo struct {
	Port     int    `json:"port"`
	Token    string `json:"token"`
	PID      int    `json:"pid"`
	Elevated bool   `json:"elevated"`
	Version  string `json:"version"`
}

func agentInfoPath() string { return filepath.Join(dataDir(), "agente.json") }

func readAgentInfo() (agentInfo, bool) {
	var ai agentInfo
	b, err := os.ReadFile(agentInfoPath())
	if err != nil || json.Unmarshal(b, &ai) != nil || ai.Port == 0 {
		return ai, false
	}
	return ai, true
}

func (a *Agent) serve() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	a.ln = ln
	tok := make([]byte, 24)
	rand.Read(tok)
	a.token = hex.EncodeToString(tok)
	info := agentInfo{Port: ln.Addr().(*net.TCPAddr).Port, Token: a.token, PID: os.Getpid(), Elevated: a.elevated, Version: version}
	b, _ := json.Marshal(info)
	tmp := agentInfoPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, agentInfoPath()); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/live", a.hLive)
	mux.HandleFunc("/settings", a.hSettings)
	mux.HandleFunc("/overlay", a.hOverlay)
	mux.HandleFunc("/purge", a.hPurge)
	mux.HandleFunc("/sessions", a.hSessions)
	mux.HandleFunc("/session", a.hSession)
	mux.HandleFunc("/components", a.hComponents)
	mux.HandleFunc("/disktest", a.hDiskTest)
	mux.HandleFunc("/stability", a.hStability)
	mux.HandleFunc("/thefts", a.hThefts)
	mux.HandleFunc("/profiles", a.hProfiles)
	mux.HandleFunc("/net", a.hNet)
	mux.HandleFunc("/stutter", a.hStutter)
	mux.HandleFunc("/noise", a.hNoise)
	mux.HandleFunc("/lab", a.hLab)
	mux.HandleFunc("/prefetch", a.hPrefetch)
	mux.HandleFunc("/quit", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"ok": true})
		go a.shutdown()
	})
	srv := &http.Server{Handler: a.auth(mux), ReadHeaderTimeout: 5 * time.Second}
	go srv.Serve(ln)
	return nil
}

// auth: solo la ventana de TurboMON (que lee el token de agente.json) puede llamar.
func (a *Agent) auth(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-TurboMON") != a.token || !strings.HasPrefix(r.Host, "127.0.0.1:") {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (a *Agent) hLive(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	first := time.Since(a.lastPoll) > 4*time.Second
	a.lastPoll = time.Now()
	a.mu.Unlock()
	if first {
		a.kick() // la ventana acaba de abrirse: lectura completa ya
		time.Sleep(60 * time.Millisecond)
	}
	a.mu.Lock()
	l := a.live
	l.Session = a.st.view()
	l.Agent.HistBytes = sessionsSize()
	hist := map[string][]float32{}
	for k, v := range a.hist {
		hist[k] = append([]float32(nil), v...)
	}
	l.Hist = hist
	a.mu.Unlock()
	writeJSON(w, l)
}

func (a *Agent) hSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var s Settings
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		s.Welcomed = true
		saveSettings(s)
		a.ui.refresh()
		a.kick()
	}
	writeJSON(w, getSettings())
}

func (a *Agent) hOverlay(w http.ResponseWriter, r *http.Request) {
	var b struct{ On bool }
	json.NewDecoder(r.Body).Decode(&b)
	s := a.changeSettings(func(s *Settings) { s.Overlay.On = b.On })
	writeJSON(w, s)
}

func (a *Agent) hPurge(w http.ResponseWriter, r *http.Request) {
	freed, err := a.purgeStandby()
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "freed": freed})
}

func (a *Agent) hSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" && r.URL.Query().Get("clear") == "1" {
		rewriteSessions(func(SessionRecord) bool { return false })
	}
	list := readSessions(false)
	if list == nil {
		list = []SessionRecord{}
	}
	writeJSON(w, list)
}

func (a *Agent) hSession(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if r.Method == "POST" && r.URL.Query().Get("delete") == "1" {
		rewriteSessions(func(s SessionRecord) bool { return s.ID != id })
		writeJSON(w, map[string]bool{"ok": true})
		return
	}
	s, ok := findSession(id)
	if !ok {
		http.Error(w, "no existe", 404)
		return
	}
	writeJSON(w, s)
}

// ---------- registro ----------

var logMu sync.Mutex

// logf escribe en el registro técnico (turbomon.log), que se recorta solo.
func logf(format string, args ...any) {
	logMu.Lock()
	defer logMu.Unlock()
	dir := dataDir()
	os.MkdirAll(dir, 0o755)
	p := filepath.Join(dir, "turbomon.log")
	if st, err := os.Stat(p); err == nil && st.Size() > 512<<10 {
		os.Rename(p, p+".1")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s [%d] %s\n", time.Now().Format("2006-01-02 15:04:05"), os.Getpid(), fmt.Sprintf(format, args...))
}

func testEnv(name string) string {
	if v := os.Getenv(name); v != "" {
		return "." + strings.NewReplacer(`\`, "_", ":", "_", "/", "_").Replace(v)
	}
	return ""
}
