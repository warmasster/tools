package main

// Partidas: cuando un juego está delante presentando fotogramas un rato,
// TurboMON empieza a apuntar cómo va (FPS, 1 % bajo, CPU, GPU, RAM y
// temperaturas) y al cerrarlo guarda un resumen de unos pocos KB en
// partidas.jsonl, con una gráfica de un punto cada 5 s.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"turbomon/internal/fps"
	"turbomon/internal/parts"
	"turbomon/internal/sensors"
	"turbomon/internal/stutter"
)

// Stat es una media y un máximo.
type Stat struct {
	Avg float64 `json:"avg"`
	Max float64 `json:"max"`
}

// Series es la gráfica de una partida.
type Series struct {
	Step int       `json:"step"` // segundos entre puntos
	FPS  []float32 `json:"fps"`
	CPU  []float32 `json:"cpu"`
	GPU  []float32 `json:"gpu"`
	RAM  []float32 `json:"ram"` // GB
}

// SessionRecord es una partida guardada.
type SessionRecord struct {
	ID       string          `json:"id"`
	Game     string          `json:"game"`
	Exe      string          `json:"exe"`
	Path     string          `json:"path,omitempty"` // dónde está el juego
	Start    int64           `json:"start"`          // unix
	Secs     int             `json:"secs"`           // tiempo con el juego delante
	FPSAvg   float64         `json:"fpsAvg"`
	Low1     float64         `json:"low1"`
	Low01    float64         `json:"low01"`
	CPU      Stat            `json:"cpu"`
	MaxCore  float64         `json:"maxCore"` // media del hilo más ocupado
	GPU      Stat            `json:"gpu"`
	RAM      Stat            `json:"ram"`  // GB
	VRAM     Stat            `json:"vram"` // GB
	RAMTot   float64         `json:"ramTotal"`
	VRAMTot  float64         `json:"vramTotal"`
	CPUTemp  Stat            `json:"cpuTemp"`
	GPUTemp  Stat            `json:"gpuTemp"`
	GPUName  string          `json:"gpuName"`
	Limit    string          `json:"limit"` // gpu, cpu, cap o ""
	Cap      int             `json:"cap,omitempty"`
	Purges   int             `json:"purges,omitempty"`
	Turbo    bool            `json:"turbo,omitempty"`
	Ping     *PingStat       `json:"ping,omitempty"`
	Stutter  *stutter.Report `json:"stutter,omitempty"`
	Monitors int             `json:"monitors,omitempty"`
	GameGPU  string          `json:"gameGPU,omitempty"`   // la gráfica en la que dibujaba el juego
	OnIGPU   bool            `json:"onIGPU,omitempty"`    // dibujaba en la integrada habiendo dedicada
	Throttle float64         `json:"throttle,omitempty"`  // % del tiempo con el procesador frenado
	Gov      *GovStat        `json:"gov,omitempty"`       // lo que hizo el ahorro inteligente
	GPUDrv   string          `json:"gpuDriver,omitempty"` // versión del driver de la gráfica
	WinBuild string          `json:"winBuild,omitempty"`  // versión de Windows
	NetHogs  []NetHog        `json:"netHogs,omitempty"`
	Series   *Series         `json:"series,omitempty"`
}

// PingStat es el ping durante la partida.
type PingStat struct {
	Server string  `json:"server,omitempty"`
	NoICMP bool    `json:"noICMP,omitempty"` // el servidor no contesta al ping
	Avg    float64 `json:"avg"`
	P95    float64 `json:"p95"`
	Jitter float64 `json:"jitter"`
	Loss   float64 `json:"loss"`
	Inet   float64 `json:"inet"` // ping medio a internet (8.8.8.8)
}

// NetHog es un programa que movió mucho por la red durante la partida.
type NetHog struct {
	Name string `json:"name"`
	Down uint64 `json:"down"`
	Up   uint64 `json:"up"`
}

type acc struct {
	sum, max float64
	n        int
}

func (a *acc) add(v float64) {
	a.sum += v
	a.n++
	if v > a.max {
		a.max = v
	}
}

func (a acc) stat(round float64) Stat {
	if a.n == 0 {
		return Stat{}
	}
	return Stat{Avg: math.Round(a.sum/float64(a.n)*round) / round, Max: math.Round(a.max*round) / round}
}

type liveSession struct {
	pid      uint32
	game     string
	exe      string
	path     string
	start    time.Time
	lastFg   time.Time
	secs     int
	hist     fps.Hist
	buf      []float32
	cpu, gpu acc
	core     acc
	ram      acc
	vram     acc
	cpuT     acc
	gpuT     acc
	fpsSec   acc
	fpsSq    float64
	ramTot   float64
	vramTot  float64
	gpuName  string
	series   Series
	pt       [4]acc // punto de la gráfica en curso
	turbo    *turboState
	netStart map[uint32][2]uint64
	purges   int
	lastPurg time.Time
	stut     *stutter.Analyzer
	ts       []int64
	away     bool
	monitors int
	limited  int
	gpuSeen  map[string]int // gráfica en la que dibuja el juego (cada 10 s)
	gov      *govRun
	gpuDrv   string
	winBuild string
}

type sessionTracker struct {
	a         *Agent
	mu        sync.Mutex
	cur       *liveSession
	candPid   uint32
	candSince time.Time
}

// SessionView es lo que se enseña de la partida en curso.
type SessionView struct {
	Game   string  `json:"game"`
	Secs   int     `json:"secs"`
	FPSAvg float64 `json:"fpsAvg"`
	Low1   float64 `json:"low1"`
	Turbo  bool    `json:"turbo"`
}

func (t *sessionTracker) view() *SessionView {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cur == nil {
		return nil
	}
	return &SessionView{Game: t.cur.game, Secs: t.cur.secs, FPSAvg: t.cur.hist.AvgFPS(), Low1: t.cur.hist.Low(0.01), Turbo: t.cur.turbo != nil}
}

func (t *sessionTracker) active() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cur != nil
}

// update se llama una vez por segundo.
func (t *sessionTracker) update(fgPid uint32, info procInfo, fgHwnd uintptr, st fps.Stats, snap *sensors.Snapshot) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if c := t.cur; c != nil {
		if !processAlive(c.pid) || now.Sub(c.lastFg) > 5*time.Minute {
			t.finish()
			return
		}
		if fgPid != c.pid {
			c.away = true
			t.a.labAway()
			return // en el escritorio, en Discord…: la partida sigue, pero no cuenta
		}
		c.lastFg = now
		c.secs++
		tnow := fps.Now()
		c.buf, c.ts = t.a.tracker.CollectTS(c.pid, c.buf[:0], c.ts[:0])
		if c.away {
			// Lo de mientras estaba fuera no cuenta (en segundo plano los juegos van a saltos).
			c.away = false
			c.stut.Skip(tnow)
		} else {
			c.hist.Add(c.buf)
			c.stut.Frames(c.buf, c.ts)
			c.stut.Second(tnow, t.a.stutterCtx(c, snap))
			t.a.labTick(c, c.buf)
			t.a.govTick(c.gov, c.buf, st.FPS, snap)
		}
		if snap != nil && snap.Full && snap.CPU.PerfCap > 0 && snap.CPU.PerfCap < 90 {
			c.limited++
		}
		if c.secs%10 == 0 && snap != nil && len(snap.GPUs) > 1 {
			t.a.smu.Lock()
			u, ok := t.a.sampler.ProcGPU(c.pid)
			t.a.smu.Unlock()
			if ok && u.Usage >= 5 {
				c.gpuSeen[u.Luid]++
			}
		}
		if st.Presenting && st.FPS > 0 {
			c.fpsSec.add(st.FPS)
			c.fpsSq += st.FPS * st.FPS
		}
		t.sample(c, st, snap)
		t.a.turboTick(c, snap)
		return
	}
	if fgPid == 0 || !isGame(info, fgHwnd, st) {
		t.candPid = 0
		return
	}
	if t.candPid != fgPid {
		t.candPid, t.candSince = fgPid, now
		return
	}
	if now.Sub(t.candSince) < 10*time.Second {
		return
	}
	// Empieza la partida.
	c := &liveSession{pid: fgPid, game: info.game, exe: info.exe, path: info.path, start: t.candSince, lastFg: now, series: Series{Step: 5},
		stut: stutter.New(fps.Freq(), fps.Now()), ts: make([]int64, 0, 1024), monitors: monitorCount(), gpuSeen: map[string]int{}}
	t.a.tracker.Collect(fgPid, nil) // lo de antes no cuenta
	if c.monitors > 1 {
		t.a.smu.Lock()
		t.a.sampler.WatchDecode(true) // ¿hay un vídeo en la otra pantalla?
		t.a.smu.Unlock()
	}
	if p, ok := getProfile(c.exe); ok {
		applyGameProfile(c.pid, p)
	}
	t.a.labSessionStart(c)
	t.a.prefSessionStart(c, info.path)
	t.cur = c
	t.candPid = 0
	logf("partida: %s (%s, pid %d)", c.game, c.exe, c.pid)
	if s := getSettings(); s.TurboMode {
		c.turbo = t.a.turboStart(c.pid)
	}
	c.gov = t.a.govStart() // después del modo Turbo: copia el plan que haya puesto
	go func() {            // versiones del driver y de Windows (para ver si una actualización cambia los FPS)
		comp := parts.Read(0)
		drv := ""
		for _, g := range comp.GPUs {
			if g.Dedicated || drv == "" {
				drv = g.DriverVersion
			}
		}
		t.mu.Lock()
		c.gpuDrv, c.winBuild = drv, comp.OS.Build
		t.mu.Unlock()
	}()
	c.netStart = t.a.net.mon.Totals()
	t.a.net.gamePing.Reset()
	t.a.net.inetPing.Reset()
	go t.a.wifiSessionStart()
}

func (t *sessionTracker) sample(c *liveSession, st fps.Stats, snap *sensors.Snapshot) {
	if snap == nil || !snap.Full {
		return
	}
	const g = 1 << 30
	c.cpu.add(snap.CPU.Usage)
	c.core.add(snap.CPU.MaxCore)
	c.ram.add(float64(snap.Mem.Used) / g)
	c.ramTot = float64(snap.Mem.Total) / g
	if snap.CPU.Temp > 0 {
		c.cpuT.add(snap.CPU.Temp)
	}
	gu := 0.0
	if mg := snap.MainGPU(); mg != nil {
		gu = mg.Usage
		c.gpu.add(mg.Usage)
		c.gpuName = mg.Name
		if mg.VRAMUsed > 0 {
			c.vram.add(float64(mg.VRAMUsed) / g)
		}
		c.vramTot = float64(mg.VRAM) / g
		if mg.Temp > 0 {
			c.gpuT.add(mg.Temp)
		}
	}
	c.pt[0].add(st.FPS)
	c.pt[1].add(snap.CPU.Usage)
	c.pt[2].add(gu)
	c.pt[3].add(float64(snap.Mem.Used) / g)
	if c.pt[0].n >= c.series.Step {
		r1 := func(v float64) float32 { return float32(math.Round(v*10) / 10) }
		c.series.FPS = append(c.series.FPS, r1(c.pt[0].sum/float64(c.pt[0].n)))
		c.series.CPU = append(c.series.CPU, r1(c.pt[1].sum/float64(c.pt[1].n)))
		c.series.GPU = append(c.series.GPU, r1(c.pt[2].sum/float64(c.pt[2].n)))
		c.series.RAM = append(c.series.RAM, float32(math.Round(c.pt[3].sum/float64(c.pt[3].n)*100)/100))
		c.pt = [4]acc{}
		// Partidas muy largas: se juntan los puntos de dos en dos (como mucho 1440 puntos).
		if len(c.series.FPS) >= 1440 {
			c.series.Step *= 2
			c.series.FPS, c.series.CPU, c.series.GPU, c.series.RAM = halve(c.series.FPS), halve(c.series.CPU), halve(c.series.GPU), halve(c.series.RAM)
		}
	}
}

func halve(v []float32) []float32 {
	out := v[:0]
	for i := 0; i+1 < len(v); i += 2 {
		out = append(out, (v[i]+v[i+1])/2)
	}
	return out
}

// finish cierra la partida en curso y la guarda (con t.mu cogido).
func (t *sessionTracker) finish() {
	c := t.cur
	t.cur = nil
	if c == nil {
		return
	}
	t.a.labSessionEnd(c)
	gov := t.a.govEnd(c.gov) // antes que el modo Turbo: vuelve a su plan
	if c.turbo != nil {
		t.a.turboEnd(c.turbo)
	}
	go t.a.wifiSessionEnd()
	if c.monitors > 1 {
		t.a.smu.Lock()
		t.a.sampler.WatchDecode(false)
		t.a.smu.Unlock()
	}
	if c.secs < 60 || c.hist.Count < 100 || !getSettings().Sessions {
		logf("partida de %s descartada (%d s)", c.game, c.secs)
		return
	}
	rec := SessionRecord{
		ID: fmt.Sprintf("%d-%d", c.start.Unix(), c.pid), Game: c.game, Exe: c.exe, Path: c.path, Start: c.start.Unix(), Secs: c.secs,
		FPSAvg: c.hist.AvgFPS(), Low1: c.hist.Low(0.01), Low01: c.hist.Low(0.001),
		CPU: c.cpu.stat(10), GPU: c.gpu.stat(10), RAM: c.ram.stat(100), VRAM: c.vram.stat(100),
		RAMTot: math.Round(c.ramTot*10) / 10, VRAMTot: math.Round(c.vramTot*10) / 10,
		CPUTemp: c.cpuT.stat(1), GPUTemp: c.gpuT.stat(1), GPUName: c.gpuName,
		Purges: c.purges, Turbo: c.turbo != nil,
	}
	if c.core.n > 0 {
		rec.MaxCore = math.Round(c.core.sum / float64(c.core.n))
	}
	rec.Limit, rec.Cap = limitOf(rec, c)
	rec.Ping, rec.NetHogs = t.a.sessionNet(c)
	if c.stut != nil {
		rep := c.stut.Report()
		rec.Stutter = &rep
	}
	rec.Monitors = c.monitors
	rec.Gov = gov
	rec.GPUDrv, rec.WinBuild = c.gpuDrv, c.winBuild
	if c.secs > 0 {
		rec.Throttle = math.Round(float64(c.limited) / float64(c.secs) * 100)
	}
	rec.GameGPU, rec.OnIGPU = t.a.gameGPU(c)
	s := c.series
	rec.Series = &s
	if err := appendSession(rec); err != nil {
		logf("no se pudo guardar la partida: %v", err)
	}
	logf("partida guardada: %s, %d s, %.0f FPS (1 %% %.0f)", rec.Game, rec.Secs, rec.FPSAvg, rec.Low1)
}

var commonCaps = []int{30, 60, 72, 75, 90, 100, 120, 144, 165, 170, 180, 200, 240, 280, 300, 360}

// limitOf dice qué frena los FPS: la gráfica (va al máximo), el procesador
// (algún hilo al máximo y la gráfica sobrada) o un límite (FPS clavados en una
// cifra redonda: sincronización vertical o un tope del juego).
func limitOf(r SessionRecord, c *liveSession) (string, int) {
	if c.fpsSec.n > 30 {
		mean := c.fpsSec.sum / float64(c.fpsSec.n)
		sd := math.Sqrt(math.Max(0, c.fpsSq/float64(c.fpsSec.n)-mean*mean))
		for _, cp := range commonCaps {
			if math.Abs(mean-float64(cp)) <= float64(cp)*0.03 && sd <= float64(cp)*0.04 && r.GPU.Avg < 95 {
				return "cap", cp
			}
		}
	}
	switch {
	case r.GPU.Avg >= 90:
		return "gpu", 0
	case r.MaxCore >= 90 || r.CPU.Avg >= 85:
		return "cpu", 0
	}
	return "", 0
}

// ---------- qué es un juego ----------

var notGames = map[string]bool{}

func init() {
	for _, n := range strings.Fields(`explorer.exe dwm.exe chrome.exe msedge.exe firefox.exe opera.exe opera_gx.exe brave.exe vivaldi.exe
		discord.exe steam.exe steamwebhelper.exe epicgameslauncher.exe epicwebhelper.exe spotify.exe code.exe devenv.exe obs64.exe
		vlc.exe mpc-hc64.exe mpc-be64.exe potplayermini64.exe wmplayer.exe teams.exe ms-teams.exe slack.exe whatsapp.exe telegram.exe
		applicationframehost.exe searchhost.exe startmenuexperiencehost.exe shellexperiencehost.exe textinputhost.exe lockapp.exe
		winword.exe excel.exe powerpnt.exe outlook.exe olk.exe onenote.exe notepad.exe mspaint.exe photoshop.exe afterfx.exe
		resolve.exe blender.exe claude.exe windowsterminal.exe cmd.exe powershell.exe pwsh.exe conhost.exe
		taskmgr.exe hwinfo64.exe msiafterburner.exe rtss.exe battle.net.exe eadesktop.exe galaxyclient.exe ubisoftconnect.exe
		upc.exe riotclientux.exe riotclientservices.exe leagueclientux.exe leagueclient.exe xboxpcapp.exe gamebar.exe
		nvcontainer.exe rundll32.exe mmc.exe systemsettings.exe photos.exe microsoft.photos.exe video.ui.exe
		zoom.exe msedgewebview2.exe electron.exe figma.exe notion.exe obsidian.exe cursor.exe idea64.exe pycharm64.exe
		turbomon.exe turbosend.exe turbocut.exe turbogames.exe turbovault.exe turbocam.exe turbomenu.exe turbosetup.exe`) {
		notGames[n] = true
	}
	// Los que llevan espacios en el nombre.
	notGames["adobe premiere pro.exe"] = true
	notGames["nvidia app.exe"] = true
}

var gameDirs = []string{`\steamapps\common\`, `\epic games\`, `\xboxgames\`, `\riot games\`, `\gog galaxy\games\`, `\gog games\`,
	`\ea games\`, `\ubisoft game launcher\games\`, `\battle.net\`, `\games\`, `\juegos\`, `\windowsapps\`}

type procInfo struct {
	path     string
	exe      string
	game     string
	excluded bool
	inGames  bool
}

func newProcInfo(pid uint32) procInfo {
	path := processPath(pid)
	pi := procInfo{path: path, exe: exeName(path)}
	lp := strings.ToLower(path)
	pi.excluded = path == "" || notGames[pi.exe] || strings.HasPrefix(lp, strings.ToLower(os.Getenv("SystemRoot"))+`\`)
	for _, d := range gameDirs {
		if strings.Contains(lp, d) {
			pi.inGames = true
			break
		}
	}
	pi.game = gameName(path)
	return pi
}

// isGame: está delante, presenta fotogramas de verdad y o bien ocupa toda la
// pantalla o bien está instalado donde se instalan los juegos.
func isGame(pi procInfo, hwnd uintptr, st fps.Stats) bool {
	if pi.excluded || !st.Presenting || st.FPS < 15 {
		return false
	}
	return pi.inGames || coversMonitor(hwnd)
}

// gameName saca un nombre legible: la carpeta del juego en Steam/Epic/Xbox, o la
// descripción del ejecutable, o el nombre del archivo.
func gameName(path string) string {
	if path == "" {
		return ""
	}
	lp := strings.ToLower(path)
	for _, d := range []string{`\steamapps\common\`, `\epic games\`, `\xboxgames\`, `\riot games\`, `\gog galaxy\games\`, `\gog games\`, `\ea games\`} {
		if i := strings.Index(lp, d); i >= 0 {
			rest := path[i+len(d):]
			if j := strings.IndexAny(rest, `\/`); j > 0 {
				return rest[:j]
			}
		}
	}
	if d := fileDescription(path); d != "" {
		l := strings.ToLower(d)
		if !strings.Contains(l, "unreal") && !strings.Contains(l, "unity") && !strings.Contains(l, "launcher") && len(d) < 60 {
			return d
		}
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	for _, suf := range []string{"-Win64-Shipping", "-WinGDK-Shipping", "_x64", "64", "-Shipping"} {
		base = strings.TrimSuffix(base, suf)
	}
	if base == "" {
		return filepath.Base(path)
	}
	return strings.ToUpper(base[:1]) + base[1:]
}

func fileDescription(path string) string {
	size, err := windows.GetFileVersionInfoSize(path, nil)
	if err != nil || size == 0 {
		return ""
	}
	buf := make([]byte, size)
	if windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])) != nil {
		return ""
	}
	var tr unsafe.Pointer
	var n uint32
	if windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\VarFileInfo\Translation`, unsafe.Pointer(&tr), &n) != nil || n < 4 {
		return ""
	}
	langs := unsafe.Slice((*uint16)(tr), n/2)
	for i := 0; i+1 < len(langs); i += 2 {
		var p unsafe.Pointer
		var ln uint32
		key := fmt.Sprintf(`\StringFileInfo\%04x%04x\FileDescription`, langs[i], langs[i+1])
		if windows.VerQueryValue(unsafe.Pointer(&buf[0]), key, unsafe.Pointer(&p), &ln) == nil && ln > 1 {
			return strings.TrimSpace(windows.UTF16ToString(unsafe.Slice((*uint16)(p), ln)))
		}
	}
	return ""
}

// ---------- archivo de partidas ----------

var sessFileMu sync.Mutex

func sessionsPath() string { return filepath.Join(dataDir(), "partidas.jsonl") }

func appendSession(r SessionRecord) error {
	sessFileMu.Lock()
	defer sessFileMu.Unlock()
	os.MkdirAll(dataDir(), 0o755)
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(sessionsPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

// readSessions lee las partidas (las más nuevas primero). Sin series si withSeries=false.
func readSessions(withSeries bool) []SessionRecord {
	sessFileMu.Lock()
	defer sessFileMu.Unlock()
	f, err := os.Open(sessionsPath())
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []SessionRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for sc.Scan() {
		var r SessionRecord
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.ID == "" {
			continue
		}
		if !withSeries {
			r.Series = nil
			if r.Stutter != nil {
				st := *r.Stutter
				st.Times = nil
				r.Stutter = &st
			}
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start > out[j].Start })
	return out
}

func findSession(id string) (SessionRecord, bool) {
	for _, r := range readSessions(true) {
		if r.ID == id {
			return r, true
		}
	}
	return SessionRecord{}, false
}

// rewriteSessions deja en el archivo solo las partidas que keep acepte, y a las de
// más de 90 días les quita la gráfica (el resumen se queda para siempre).
func rewriteSessions(keep func(SessionRecord) bool) error {
	all := readSessions(true)
	sessFileMu.Lock()
	defer sessFileMu.Unlock()
	var b bytes.Buffer
	old := time.Now().AddDate(0, 0, -90).Unix()
	for i := len(all) - 1; i >= 0; i-- {
		r := all[i]
		if keep != nil && !keep(r) {
			continue
		}
		if r.Start < old {
			r.Series = nil
		}
		j, _ := json.Marshal(r)
		b.Write(j)
		b.WriteByte('\n')
	}
	tmp := sessionsPath() + ".tmp"
	if err := os.WriteFile(tmp, b.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, sessionsPath())
}

func sessionsSize() int64 {
	st, err := os.Stat(sessionsPath())
	if err != nil {
		return 0
	}
	return st.Size()
}

// gameGPU dice en qué gráfica dibujaba el juego y si era la integrada habiendo una dedicada (portátiles).
func (a *Agent) gameGPU(c *liveSession) (string, bool) {
	best, n := "", 0
	for k, v := range c.gpuSeen {
		if v > n {
			best, n = k, v
		}
	}
	if best == "" {
		return "", false
	}
	a.smu.Lock()
	ads := a.sampler.GPUAdapters()
	a.smu.Unlock()
	name, integrated, hasDedicated := "", false, false
	for _, ad := range ads {
		if ad.Dedicated {
			hasDedicated = true
		}
		if ad.LuidKey() == best {
			name, integrated = ad.Name, !ad.Dedicated
		}
	}
	return name, integrated && hasDedicated
}
