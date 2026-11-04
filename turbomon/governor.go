package main

// Ahorro inteligente: durante la partida, una copia del plan de energía en la
// que TurboMON va bajando la frecuencia máxima del procesador mientras los
// fotogramas sigan saliendo a tiempo. Al acabar se vuelve al plan de antes y la
// copia se borra (también si TurboMON se cerró a mitad: lo arregla al arrancar).

import (
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"turbomon/internal/checks"
	"turbomon/internal/governor"
	"turbomon/internal/sensors"
)

var (
	modPowr        = windows.NewLazySystemDLL("powrprof.dll")
	pDupScheme     = modPowr.NewProc("PowerDuplicateScheme")
	pDelScheme     = modPowr.NewProc("PowerDeleteScheme")
	pWriteName     = modPowr.NewProc("PowerWriteFriendlyName")
	pWriteACIdx    = modPowr.NewProc("PowerWriteACValueIndex")
	pWriteDCIdx    = modPowr.NewProc("PowerWriteDCValueIndex")
	pSetActiveSch  = modPowr.NewProc("PowerSetActiveScheme")
	guidSubProc, _ = windows.GUIDFromString("{54533251-82be-4824-96c1-47b60b740d00}")
	guidFreqMax, _ = windows.GUIDFromString("{75b0ae3f-bce0-45a7-8c89-c9611c25e100}") // PROCFREQMAX (MHz, 0 = sin tope)
	guidFreqMx1, _ = windows.GUIDFromString("{75b0ae3f-bce0-45a7-8c89-c9611c25e101}") // lo mismo para los núcleos de la clase 1 (P)
)

// GovStat es lo que hizo el ahorro en una partida.
type GovStat struct {
	Secs        int     `json:"secs"`
	CapSecs     int     `json:"capSecs"`               // segundos con tope
	AvgCap      int     `json:"avgCap"`                // tope medio (MHz)
	MaxMHz      int     `json:"maxMHz"`                // la frecuencia sin tope
	MHzOn       int     `json:"mhzOn"`                 // frecuencia media con tope
	MHzOff      int     `json:"mhzOff"`                // y sin tope
	WattsOn     float64 `json:"wattsOn,omitempty"`     // consumo del procesador con tope (HWiNFO)
	WattsOff    float64 `json:"wattsOff,omitempty"`    // y sin
	TempOn      float64 `json:"tempOn,omitempty"`      // temperatura con tope
	TempOff     float64 `json:"tempOff,omitempty"`     // y sin
	Unsupported bool    `json:"unsupported,omitempty"` // este procesador no hace caso al tope
}

type govRun struct {
	prev, dup windows.GUID
	ctl       *governor.Controller
	frames    [][]float32 // los últimos 3 segundos
	fpsWin    []float64
	cap       int
	since     int // segundos con el tope actual
	st        GovStat
	on, off   [3]acc // MHz, vatios, temperatura
	capSum    float64
	dead      bool
}

func govMarker() string { return filepath.Join(dataDir(), "ahorro-plan.txt") }

func writeProcFreq(scheme windows.GUID, mhz int) error {
	for _, g := range []*windows.GUID{&guidFreqMax, &guidFreqMx1} {
		r, _, _ := pWriteACIdx.Call(0, uintptr(unsafe.Pointer(&scheme)), uintptr(unsafe.Pointer(&guidSubProc)), uintptr(unsafe.Pointer(g)), uintptr(mhz))
		if r != 0 && g == &guidFreqMax {
			return syscall.Errno(r)
		}
		pWriteDCIdx.Call(0, uintptr(unsafe.Pointer(&scheme)), uintptr(unsafe.Pointer(&guidSubProc)), uintptr(unsafe.Pointer(g)), uintptr(mhz))
	}
	if r, _, _ := pSetActiveSch.Call(0, uintptr(unsafe.Pointer(&scheme))); r != 0 { // aplica el cambio
		return syscall.Errno(r)
	}
	return nil
}

func (a *Agent) govStart() *govRun {
	if !getSettings().Governor || !a.elevated {
		return nil
	}
	cur, err := checks.ActivePlan()
	if err != nil {
		return nil
	}
	var dup *windows.GUID
	if r, _, _ := pDupScheme.Call(0, uintptr(unsafe.Pointer(&cur.GUID)), uintptr(unsafe.Pointer(&dup))); r != 0 || dup == nil {
		logf("ahorro: no se pudo copiar el plan (%d)", r)
		return nil
	}
	g := *dup
	windows.LocalFree(windows.Handle(unsafe.Pointer(dup)))
	name, _ := windows.UTF16FromString("TurboMON (ahorro en partida)")
	pWriteName.Call(0, uintptr(unsafe.Pointer(&g)), 0, 0, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)*2))
	os.WriteFile(govMarker(), []byte(cur.GUID.String()+" "+g.String()), 0o644)
	if err := writeProcFreq(g, 0); err != nil {
		checks.SetPlan(cur.GUID)
		pDelScheme.Call(0, uintptr(unsafe.Pointer(&g)))
		os.Remove(govMarker())
		logf("ahorro: no se pudo usar la copia del plan: %v", err)
		return nil
	}
	logf("ahorro: en marcha (copia de «%s»)", cur.Name)
	return &govRun{prev: cur.GUID, dup: g, ctl: governor.New()}
}

// p99 de los fotogramas de los últimos segundos.
func p99(frames [][]float32) float64 {
	var all []float64
	for _, f := range frames {
		for _, v := range f {
			all = append(all, float64(v))
		}
	}
	if len(all) < 20 {
		return 0
	}
	sort.Float64s(all)
	return all[int(float64(len(all))*0.99)]
}

// liveCap: si los FPS de los últimos 10 s están clavados en una cifra redonda.
func liveCap(fps []float64) int {
	if len(fps) < 8 {
		return 0
	}
	s, q := 0.0, 0.0
	for _, v := range fps {
		s += v
		q += v * v
	}
	mean := s / float64(len(fps))
	sd := math.Sqrt(math.Max(0, q/float64(len(fps))-mean*mean))
	for _, cp := range commonCaps {
		if math.Abs(mean-float64(cp)) <= float64(cp)*0.03 && sd <= float64(cp)*0.04 {
			return cp
		}
	}
	return 0
}

// govTick: un segundo de partida con el juego delante.
func (a *Agent) govTick(g *govRun, frames []float32, fps float64, snap *sensors.Snapshot) {
	if g == nil || g.dead || snap == nil || !snap.Full {
		return
	}
	g.frames = append(g.frames, append([]float32(nil), frames...))
	if len(g.frames) > 3 {
		g.frames = g.frames[1:]
	}
	g.fpsWin = append(g.fpsWin, fps)
	if len(g.fpsWin) > 10 {
		g.fpsWin = g.fpsWin[1:]
	}
	gpu := 0.0
	if mg := snap.MainGPU(); mg != nil {
		gpu = mg.Usage
	}
	g.st.Secs++
	side := &g.off
	if g.cap > 0 {
		side = &g.on
		g.st.CapSecs++
		g.capSum += float64(g.cap)
	}
	side[0].add(float64(snap.CPU.MHz))
	if snap.CPU.PowerW > 0 {
		side[1].add(snap.CPU.PowerW)
	}
	if snap.CPU.Temp > 0 {
		side[2].add(snap.CPU.Temp)
	}
	g.since++
	// ¿Hace caso Windows al tope? Si tras 5 s sigue muy por encima, se deja.
	if g.cap > 0 && g.since == 5 && snap.CPU.MHz > g.cap+400 {
		logf("ahorro: el procesador no hace caso al tope (%d MHz con tope %d): se deja", snap.CPU.MHz, g.cap)
		g.dead, g.st.Unsupported = true, true
		writeProcFreq(g.dup, 0)
		g.cap = 0
		return
	}
	cp, changed := g.ctl.Step(governor.Input{FPS: fps, P99: p99(g.frames), Cap: liveCap(g.fpsWin), GPU: gpu, MHz: snap.CPU.MHz})
	if changed {
		if err := writeProcFreq(g.dup, cp); err != nil {
			g.dead = true
			logf("ahorro: %v", err)
			return
		}
		g.cap, g.since = cp, 0
	}
}

// govEnd vuelve al plan de antes y borra la copia.
func (a *Agent) govEnd(g *govRun) *GovStat {
	if g == nil {
		return nil
	}
	checks.SetPlan(g.prev)
	pDelScheme.Call(0, uintptr(unsafe.Pointer(&g.dup)))
	os.Remove(govMarker())
	st := g.st
	st.MaxMHz = g.ctl.MaxMHz()
	if st.CapSecs > 0 {
		st.AvgCap = int(g.capSum / float64(st.CapSecs))
	}
	avg := func(x acc) float64 {
		if x.n == 0 {
			return 0
		}
		return math.Round(x.sum/float64(x.n)*10) / 10
	}
	st.MHzOn, st.MHzOff = int(avg(g.on[0])), int(avg(g.off[0]))
	st.WattsOn, st.WattsOff = avg(g.on[1]), avg(g.off[1])
	st.TempOn, st.TempOff = avg(g.on[2]), avg(g.off[2])
	logf("ahorro: fin (%d s con tope de %d, medio %d MHz)", st.CapSecs, st.Secs, st.AvgCap)
	if st.Secs < 30 {
		return nil
	}
	return &st
}

// restoreGovernor deja el plan de antes si TurboMON se cerró en mitad de una partida.
func restoreGovernor() {
	b, err := os.ReadFile(govMarker())
	if err != nil {
		return
	}
	f := strings.Fields(string(b))
	if len(f) == 2 {
		if prev, err := windows.GUIDFromString(f[0]); err == nil {
			checks.SetPlan(prev)
		}
		if dup, err := windows.GUIDFromString(f[1]); err == nil {
			pDelScheme.Call(0, uintptr(unsafe.Pointer(&dup)))
		}
	}
	os.Remove(govMarker())
}
