package main

// El laboratorio en el agente: mientras se juega, enciende y apaga un ajuste
// del juego (en qué núcleos va, su prioridad) en bloques de 20 s al azar y
// mide cada bloque. Quien juega no sabe qué bloque es cuál: nada de placebo.
// Al acabar (o si se cierra el juego) lo deja todo como estaba.

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"turbomon/internal/lab"
	"turbomon/internal/parts"
)

const (
	labBlockSecs  = 20
	labSettleSecs = 3
	labPairs      = 14
)

var (
	procGetProcAffinity = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetProcessAffinityMask")
	procSetProcAffinity = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetProcessAffinityMask")
	procGetPrioClass    = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetPriorityClass")
	procD3DSetSched     = windows.NewLazySystemDLL("gdi32.dll").NewProc("D3DKMTSetProcessSchedulingPriorityClass")
	procD3DGetSched     = windows.NewLazySystemDLL("gdi32.dll").NewProc("D3DKMTGetProcessSchedulingPriorityClass")
)

// LabKnob es un ajuste que se puede probar.
type LabKnob struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Desc  string `json:"desc"`
}

// labKnobs son los ajustes que tienen sentido en este PC.
func labKnobs() []LabKnob {
	sets := parts.CPUSets()
	classes, caches, smt := map[int]bool{}, map[int]bool{}, false
	perCore := map[int]int{}
	for _, c := range sets {
		classes[c.Efficiency] = true
		caches[c.Cache] = true
		perCore[c.Core]++
		if perCore[c.Core] > 1 {
			smt = true
		}
	}
	var out []LabKnob
	if len(classes) > 1 {
		out = append(out, LabKnob{"pcores", "Solo núcleos de rendimiento", "Saca el juego de los núcleos de eficiencia (E), que son más lentos."})
	}
	if smt {
		out = append(out, LabKnob{"nosmt", "Un hilo por núcleo", "Como apagar el Hyper-Threading o el SMT, pero solo para el juego."})
	}
	if len(caches) > 1 {
		out = append(out, LabKnob{"ccd", "Solo el primer chip", "El juego en un solo bloque de núcleos con su caché (en los Ryzen con dos chips, el de la caché 3D)."})
	}
	if len(sets) >= 6 {
		out = append(out, LabKnob{"nocore0", "Lejos del núcleo 0", "Windows atiende casi todas las interrupciones en el primer núcleo: el juego deja de usarlo."})
	}
	out = append(out,
		LabKnob{"high", "Prioridad alta", "Windows le da el procesador al juego antes que a todo lo demás."},
		LabKnob{"gpuhigh", "Prioridad alta en la gráfica", "La gráfica atiende antes al juego que al navegador, Discord o el escritorio."},
	)
	return out
}

func knobTitle(id string) string {
	for _, k := range labKnobs() {
		if k.ID == id {
			return k.Title
		}
	}
	return id
}

// coreMask es la máscara de procesadores lógicos de un ajuste de núcleos.
func coreMask(id string) uint64 {
	sets := parts.CPUSets()
	if len(sets) == 0 {
		return 0
	}
	maxEff, cache0, core0 := 0, -1, -1
	for _, c := range sets {
		if c.Efficiency > maxEff {
			maxEff = c.Efficiency
		}
		if c.Logical == 0 {
			cache0, core0 = c.Cache, c.Core
		}
	}
	seen := map[int]bool{}
	var m uint64
	sorted := append([]parts.CPUSet(nil), sets...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Logical < sorted[j].Logical })
	for _, c := range sorted {
		if c.Logical >= 64 {
			continue
		}
		ok := true
		switch id {
		case "pcores":
			ok = c.Efficiency == maxEff
		case "nosmt":
			ok = !seen[c.Core]
			seen[c.Core] = true
		case "ccd":
			ok = c.Cache == cache0
		case "nocore0":
			ok = c.Core != core0
		}
		if ok {
			m |= 1 << c.Logical
		}
	}
	return m
}

// procCfg es cómo está el juego (para dejarlo igual).
type procCfg struct {
	mask  uint64
	prio  uint32
	gpu   int32
	hasGP bool
}

func openGame(pid uint32) (windows.Handle, error) {
	return windows.OpenProcess(windows.PROCESS_SET_INFORMATION|windows.PROCESS_QUERY_INFORMATION, false, pid)
}

func readCfg(h windows.Handle) procCfg {
	var c procCfg
	var pm, sm uintptr
	procGetProcAffinity.Call(uintptr(h), uintptr(unsafe.Pointer(&pm)), uintptr(unsafe.Pointer(&sm)))
	c.mask = uint64(pm)
	p, _, _ := procGetPrioClass.Call(uintptr(h))
	c.prio = uint32(p)
	var g int32
	if r, _, _ := procD3DGetSched.Call(uintptr(h), uintptr(unsafe.Pointer(&g))); r == 0 {
		c.gpu, c.hasGP = g, true
	}
	return c
}

func setMask(h windows.Handle, m uint64) error {
	if m == 0 {
		return nil
	}
	if r, _, err := procSetProcAffinity.Call(uintptr(h), uintptr(m)); r == 0 {
		return err
	}
	return nil
}

func setGPUPrio(h windows.Handle, class int32) error {
	if r, _, _ := procD3DSetSched.Call(uintptr(h), uintptr(class)); r != 0 {
		return fmt.Errorf("la gráfica no ha aceptado el cambio (0x%x)", r)
	}
	return nil
}

// applyKnob pone el ajuste (on) o lo de antes (off).
func applyKnob(h windows.Handle, knob string, base procCfg, on bool) error {
	switch knob {
	case "pcores", "nosmt", "ccd", "nocore0":
		if !on {
			return setMask(h, base.mask)
		}
		m := coreMask(knob) & base.mask
		if m == 0 {
			m = coreMask(knob)
		}
		return setMask(h, m)
	case "high":
		if !on {
			return windows.SetPriorityClass(h, base.prio)
		}
		return windows.SetPriorityClass(h, windows.HIGH_PRIORITY_CLASS)
	case "gpuhigh":
		if !on {
			if base.hasGP {
				return setGPUPrio(h, base.gpu)
			}
			return setGPUPrio(h, 2) // normal
		}
		return setGPUPrio(h, 4) // D3DKMT_SCHEDULINGPRIORITYCLASS_HIGH
	}
	return fmt.Errorf("ajuste desconocido")
}

// applyGameProfile pone al empezar la partida lo que el juego tiene guardado del laboratorio.
func applyGameProfile(pid uint32, p GameProfile) {
	if p.Cores == "" && p.Priority == "" && !p.GPUHigh {
		return
	}
	h, err := openGame(pid)
	if err != nil {
		logf("perfil: no se puede tocar el juego: %v", err)
		return
	}
	defer windows.CloseHandle(h)
	base := readCfg(h)
	if p.Cores != "" && p.Cores != "all" {
		applyKnob(h, p.Cores, base, true)
	}
	switch p.Priority {
	case "high":
		windows.SetPriorityClass(h, windows.HIGH_PRIORITY_CLASS)
	case "above":
		windows.SetPriorityClass(h, windows.ABOVE_NORMAL_PRIORITY_CLASS)
	}
	if p.GPUHigh {
		setGPUPrio(h, 4)
	}
	logf("perfil aplicado: núcleos %q, prioridad %q, gráfica alta %v", p.Cores, p.Priority, p.GPUHigh)
}

// ---------- el experimento ----------

// LabRecord es un experimento guardado.
type LabRecord struct {
	ID      string      `json:"id"`
	Exe     string      `json:"exe"`
	Game    string      `json:"game"`
	Knob    string      `json:"knob"`
	Title   string      `json:"title"`
	Start   int64       `json:"start"`
	End     int64       `json:"end"`
	Planned int         `json:"planned"` // parejas previstas
	Partial bool        `json:"partial,omitempty"`
	Blocks  []lab.Block `json:"blocks"`
	Result  lab.Result  `json:"result"`
}

type labRun struct {
	rec    LabRecord
	pid    uint32
	h      windows.Handle
	base   procCfg
	order  []int
	idx    int
	settle int
	secs   int
	frames []float32
	err    string
}

// LabView es cómo va el experimento (sin decir qué bloque es cuál).
type LabView struct {
	Knob   string  `json:"knob"`
	Title  string  `json:"title"`
	Game   string  `json:"game"`
	Block  int     `json:"block"`
	Blocks int     `json:"blocks"`
	Pct    float64 `json:"pct"`
	Left   int     `json:"left"` // segundos que faltan (jugando)
	Paused bool    `json:"paused"`
	Error  string  `json:"error,omitempty"`
}

type labPlan struct {
	Knob  string `json:"knob"`
	Title string `json:"title"`
	Pairs int    `json:"pairs"`
}

var (
	labMu      sync.Mutex
	labCur     *labRun
	labPending *labPlan
	labPaused  bool
)

func labPath() string { return filepath.Join(dataDir(), "laboratorio.json") }

func readLab() []LabRecord {
	var out []LabRecord
	if b, err := os.ReadFile(labPath()); err == nil {
		json.Unmarshal(b, &out)
	}
	return out
}

func writeLab(list []LabRecord) {
	if len(list) > 40 {
		list = list[len(list)-40:]
	}
	b, _ := json.MarshalIndent(list, "", " ")
	os.WriteFile(labPath(), b, 0o644)
}

// labStart empieza el experimento con el juego en curso. Con labMu cogido.
func (a *Agent) labStart(c *liveSession, plan labPlan) error {
	h, err := openGame(c.pid)
	if err != nil {
		return fmt.Errorf("este juego no deja cambiar sus ajustes (suele ser por el antitrampas)")
	}
	base := readCfg(h)
	if err := applyKnob(h, plan.Knob, base, false); err != nil {
		windows.CloseHandle(h)
		return fmt.Errorf("este juego no deja cambiar sus ajustes: %v", err)
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	run := &labRun{pid: c.pid, h: h, base: base, order: lab.Order(plan.Pairs, r), settle: labSettleSecs,
		rec: LabRecord{ID: fmt.Sprintf("%d", time.Now().Unix()), Exe: c.exe, Game: c.game, Knob: plan.Knob, Title: plan.Title, Start: time.Now().Unix(), Planned: plan.Pairs}}
	if err := applyKnob(h, plan.Knob, base, run.order[0] == 1); err != nil {
		windows.CloseHandle(h)
		return fmt.Errorf("no se ha podido poner el ajuste: %v", err)
	}
	labCur = run
	logf("laboratorio: «%s» en %s (%d parejas)", plan.Title, c.game, plan.Pairs)
	return nil
}

// labSessionStart: al empezar una partida, si había un experimento esperando.
func (a *Agent) labSessionStart(c *liveSession) {
	labMu.Lock()
	defer labMu.Unlock()
	if labPending == nil || labCur != nil {
		return
	}
	plan := *labPending
	labPending = nil
	if err := a.labStart(c, plan); err != nil {
		logf("laboratorio: %v", err)
		a.ui.notify("Laboratorio", err.Error())
	}
}

// labTick: un segundo de partida con el juego delante (frames son sus fotogramas nuevos).
func (a *Agent) labTick(c *liveSession, frames []float32) {
	labMu.Lock()
	defer labMu.Unlock()
	run := labCur
	if run == nil || run.pid != c.pid {
		return
	}
	labPaused = false
	if run.settle > 0 {
		run.settle--
		return
	}
	run.frames = append(run.frames, frames...)
	run.secs++
	if run.secs < labBlockSecs {
		return
	}
	run.rec.Blocks = append(run.rec.Blocks, lab.Measure(run.frames, run.secs, run.order[run.idx]))
	run.frames, run.secs = run.frames[:0], 0
	run.idx++
	if run.idx >= len(run.order) {
		a.labFinish(false)
		return
	}
	if run.order[run.idx] != run.order[run.idx-1] {
		if err := applyKnob(run.h, run.rec.Knob, run.base, run.order[run.idx] == 1); err != nil {
			run.err = err.Error()
			a.labFinish(true)
			return
		}
	}
	run.settle = labSettleSecs
}

// labAway: el juego no está delante; el bloque en curso no vale y se repite.
func (a *Agent) labAway() {
	labMu.Lock()
	defer labMu.Unlock()
	if labCur != nil && !labPaused {
		labPaused = true
		labCur.frames, labCur.secs, labCur.settle = labCur.frames[:0], 0, labSettleSecs
	}
}

// labSessionEnd: se acaba la partida.
func (a *Agent) labSessionEnd(c *liveSession) {
	labMu.Lock()
	defer labMu.Unlock()
	if labCur != nil && labCur.pid == c.pid {
		a.labFinish(true)
	}
}

// labFinish deja el juego como estaba y guarda el resultado. Con labMu cogido.
func (a *Agent) labFinish(early bool) {
	run := labCur
	labCur = nil
	if run == nil {
		return
	}
	applyKnob(run.h, run.rec.Knob, run.base, false)
	windows.CloseHandle(run.h)
	run.rec.End = time.Now().Unix()
	run.rec.Partial = early
	run.rec.Result = lab.Analyze(run.rec.Blocks)
	if run.rec.Result.Pairs < 3 {
		logf("laboratorio: parado con %d parejas, no se guarda", run.rec.Result.Pairs)
		return
	}
	writeLab(append(readLab(), run.rec))
	logf("laboratorio: «%s» en %s → %s (FPS %+.1f %%, p=%.3f)", run.rec.Title, run.rec.Game, run.rec.Result.Verdict, run.rec.Result.FPS.Delta, run.rec.Result.FPS.P)
	if !early {
		a.ui.notify("Experimento acabado", fmt.Sprintf("«%s» en %s: mira el resultado en TurboMON.", run.rec.Title, run.rec.Game))
	}
}

func (a *Agent) hLab(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var b struct {
			Action string `json:"action"`
			Knob   string `json:"knob"`
			Pairs  int    `json:"pairs"`
			ID     string `json:"id"`
			On     bool   `json:"on"`
		}
		json.NewDecoder(r.Body).Decode(&b)
		switch b.Action {
		case "start":
			ok := false
			for _, k := range labKnobs() {
				ok = ok || k.ID == b.Knob
			}
			if !ok {
				http.Error(w, "ese ajuste no vale en este PC", 400)
				return
			}
			if b.Pairs < 4 || b.Pairs > 30 {
				b.Pairs = labPairs
			}
			plan := labPlan{Knob: b.Knob, Title: knobTitle(b.Knob), Pairs: b.Pairs}
			a.st.mu.Lock()
			c := a.st.cur
			labMu.Lock()
			var err error
			if labCur != nil {
				err = fmt.Errorf("ya hay un experimento en marcha")
			} else if c != nil {
				err = a.labStart(c, plan)
			} else {
				labPending = &plan
			}
			labMu.Unlock()
			a.st.mu.Unlock()
			if err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
		case "stop":
			labMu.Lock()
			labPending = nil
			if labCur != nil {
				a.labFinish(true)
			}
			labMu.Unlock()
		case "apply":
			for _, rec := range readLab() {
				if rec.ID != b.ID {
					continue
				}
				p, _ := getProfile(rec.Exe)
				if p.Name == "" {
					p.Name = rec.Game
				}
				switch rec.Knob {
				case "high":
					p.Priority = map[bool]string{true: "high", false: ""}[b.On]
				case "gpuhigh":
					p.GPUHigh = b.On
				default:
					if b.On {
						p.Cores = rec.Knob
					} else if p.Cores == rec.Knob {
						p.Cores = ""
					}
				}
				setProfile(rec.Exe, p)
			}
		case "delete":
			var keep []LabRecord
			for _, rec := range readLab() {
				if rec.ID != b.ID {
					keep = append(keep, rec)
				}
			}
			writeLab(keep)
		}
	}
	out := map[string]any{"knobs": labKnobs(), "elevated": a.elevated}
	labMu.Lock()
	if labCur != nil {
		run := labCur
		total := len(run.order)
		left := (total-run.idx)*(labBlockSecs+labSettleSecs) - run.secs
		out["run"] = LabView{Knob: run.rec.Knob, Title: run.rec.Title, Game: run.rec.Game, Block: run.idx + 1, Blocks: total,
			Pct: float64(run.idx*labBlockSecs+run.secs) / float64(total*labBlockSecs) * 100, Left: left, Paused: labPaused, Error: run.err}
	}
	if labPending != nil {
		out["pending"] = labPending
	}
	labMu.Unlock()
	hist := readLab()
	// Lo más nuevo primero, y si ya está puesto en el perfil del juego.
	type recView struct {
		LabRecord
		Applied bool `json:"applied"`
	}
	views := make([]recView, 0, len(hist))
	for i := len(hist) - 1; i >= 0; i-- {
		rec := hist[i]
		p, _ := getProfile(rec.Exe)
		applied := false
		switch rec.Knob {
		case "high":
			applied = p.Priority == "high"
		case "gpuhigh":
			applied = p.GPUHigh
		default:
			applied = strings.EqualFold(p.Cores, rec.Knob)
		}
		views = append(views, recView{rec, applied})
	}
	out["history"] = views
	writeJSON(w, out)
}
