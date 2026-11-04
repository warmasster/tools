package main

// Precarga aprendida en el agente: cuando arranca un juego que ya conocemos
// (lo avisa Windows con sus eventos de procesos), se apunta qué lee durante
// los primeros minutos y, si el juego tiene la precarga puesta, se leen por
// delante los trozos que pidió las otras veces. Solo en discos duros y SSD
// SATA: en un NVMe no se nota.

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"turbomon/internal/etw"
	"turbomon/internal/parts"
	"turbomon/internal/prefetch"
)

const prefetchMax = 2 << 30 // como mucho 2 GB por juego

type prefState struct {
	mu       sync.Mutex
	known    map[string]bool // exes de juegos ya vistos
	knownAt  time.Time
	rec      *prefetch.Recorder
	recPid   uint32
	recExe   string
	lastWarm map[string]WarmInfo
}

// WarmInfo es la última precarga de un juego.
type WarmInfo struct {
	At    int64   `json:"at"`
	Bytes uint64  `json:"bytes"`
	Secs  float64 `json:"secs"`
}

var pref = &prefState{lastWarm: map[string]WarmInfo{}}

func prefetchDir() string { return filepath.Join(dataDir(), "precarga") }

func prefetchPath(exe string) string {
	return filepath.Join(prefetchDir(), strings.TrimSuffix(strings.ToLower(exe), ".exe")+".json")
}

func (p *prefState) isKnown(exe string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.known == nil || time.Since(p.knownAt) > 10*time.Minute {
		p.known = map[string]bool{}
		for _, s := range readSessions(false) {
			p.known[strings.ToLower(s.Exe)] = true
		}
		for k := range loadProfiles() {
			p.known[k] = true
		}
		p.knownAt = time.Now()
	}
	return p.known[strings.ToLower(exe)]
}

// diskKindOf dice en qué tipo de disco está un archivo (nvme, ssd, hdd, usb o "").
func diskKindOf(path string) string {
	if len(path) < 2 || path[1] != ':' {
		return ""
	}
	l := strings.ToUpper(path[:1])
	for _, d := range parts.Read(0).Disks {
		for _, x := range d.Letters {
			if strings.EqualFold(strings.TrimSuffix(x, ":"), l) {
				return d.Kind
			}
		}
	}
	return ""
}

// startProcessWatch escucha los arranques de procesos (cuesta casi nada: un evento por programa abierto).
func (a *Agent) startProcessWatch() {
	if a.etw == nil {
		return
	}
	dev := prefetch.NewRecorder()
	err := a.etw.Events().Enable(prefetch.ProcessProvider, 4, 0x10, nil, func(r *etw.Record) {
		pid, img, ok := prefetch.ParseProcessStart(r)
		if !ok {
			return
		}
		path := dev.DOSPath(img)
		exe := strings.ToLower(filepath.Base(img))
		go a.onProcessStart(pid, exe, path)
	})
	if err != nil {
		logf("precarga: sin eventos de procesos: %v", err)
	}
}

func (a *Agent) onProcessStart(pid uint32, exe, path string) {
	if !pref.isKnown(exe) {
		return
	}
	kind := diskKindOf(path)
	if kind == "nvme" || kind == "" {
		return
	}
	if p, ok := getProfile(exe); ok && p.Prefetch {
		if plan, ok := prefetch.Load(prefetchPath(exe)); ok && plan.Bytes > 0 {
			go a.warm(pid, exe, plan)
		}
	}
	a.prefRecord(pid, exe, 150*time.Second)
}

func (a *Agent) warm(pid uint32, exe string, plan prefetch.Plan) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	go func() { // si el juego se cierra, se para
		for ctx.Err() == nil {
			time.Sleep(time.Second)
			if !processAlive(pid) {
				cancel()
			}
		}
	}()
	t0 := time.Now()
	n := prefetch.Warm(ctx, plan)
	info := WarmInfo{At: time.Now().Unix(), Bytes: n, Secs: time.Since(t0).Seconds()}
	pref.mu.Lock()
	pref.lastWarm[exe] = info
	pref.mu.Unlock()
	logf("precarga: %s, %d MB en %.1f s", exe, n>>20, info.Secs)
}

// prefRecord apunta qué lee el juego durante un rato (uno a la vez).
func (a *Agent) prefRecord(pid uint32, exe string, d time.Duration) {
	if a.etw == nil {
		return
	}
	pref.mu.Lock()
	if pref.rec != nil {
		pref.mu.Unlock()
		return
	}
	rec := prefetch.NewRecorder()
	rec.Pid = pid
	pref.rec, pref.recPid, pref.recExe = rec, pid, exe
	pref.mu.Unlock()
	if err := a.etw.Events().Enable(prefetch.Provider, 4, prefetch.Keywords, []uint32{pid}, rec.Handle); err != nil {
		logf("precarga: no se pueden ver los archivos: %v", err)
		pref.mu.Lock()
		pref.rec = nil
		pref.mu.Unlock()
		return
	}
	end := time.Now().Add(d)
	for time.Now().Before(end) && processAlive(pid) {
		time.Sleep(time.Second)
	}
	a.etw.Events().Disable(prefetch.Provider)
	plan := rec.Plan(prefetchMax)
	pref.mu.Lock()
	pref.rec = nil
	pref.mu.Unlock()
	if plan.Bytes == 0 {
		return
	}
	os.MkdirAll(prefetchDir(), 0o755)
	old, _ := prefetch.Load(prefetchPath(exe))
	merged := prefetch.Merge(old, plan, prefetchMax)
	merged.Save(prefetchPath(exe))
	logf("precarga: aprendido %s, %d archivos, %d MB (%d veces)", exe, len(merged.Files), merged.Bytes>>20, merged.Runs)
}

// prefSessionStart: la primera vez que se juega a algo no se vio arrancar; se aprende desde aquí.
func (a *Agent) prefSessionStart(c *liveSession, path string) {
	if k := diskKindOf(path); k == "nvme" || k == "" {
		return
	}
	if _, ok := prefetch.Load(prefetchPath(c.exe)); ok {
		return
	}
	go a.prefRecord(c.pid, c.exe, 120*time.Second)
}

// PrefetchInfo es lo que se sabe de la precarga de un juego.
type PrefetchInfo struct {
	Disk     string    `json:"disk"` // nvme, ssd, hdd…
	Learned  uint64    `json:"learned"`
	Files    int       `json:"files"`
	Runs     int       `json:"runs"`
	LastWarm *WarmInfo `json:"lastWarm,omitempty"`
	Learning bool      `json:"learning"`
}

func (a *Agent) hPrefetch(w http.ResponseWriter, r *http.Request) {
	out := map[string]PrefetchInfo{}
	paths := map[string]string{}
	for _, s := range readSessions(false) {
		k := strings.ToLower(s.Exe)
		if _, ok := paths[k]; !ok {
			paths[k] = s.Path
		}
	}
	pref.mu.Lock()
	learning := pref.recExe
	if pref.rec == nil {
		learning = ""
	}
	warm := map[string]WarmInfo{}
	for k, v := range pref.lastWarm {
		warm[k] = v
	}
	pref.mu.Unlock()
	for k, p := range paths {
		info := PrefetchInfo{Disk: diskKindOf(p), Learning: learning == k}
		if plan, ok := prefetch.Load(prefetchPath(k)); ok {
			info.Learned, info.Files, info.Runs = plan.Bytes, len(plan.Files), plan.Runs
		}
		if wi, ok := warm[k]; ok {
			info.LastWarm = &wi
		}
		out[k] = info
	}
	writeJSON(w, out)
}
