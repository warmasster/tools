//go:build windows

// Package lock averigua qué programas bloquean archivos, carpetas o unidades
// («el archivo está en uso») y los libera: cerrando el programa, forzándolo
// o soltando solo los archivos concretos. También expulsa unidades USB y
// manda archivos a la papelera.
package lock

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

// Kind es el tipo de proceso, que decide qué se puede hacer con él.
type Kind string

const (
	KindApp      Kind = "app"      // programa normal: se puede cerrar
	KindExplorer Kind = "explorer" // Explorador de Windows: nunca se cierra, solo se sueltan sus archivos
	KindService  Kind = "service"  // servicio de Windows
	KindSystem   Kind = "system"   // proceso crítico de Windows: nunca se cierra
)

// Process es un programa que bloquea alguna de las rutas.
type Process struct {
	PID        int      `json:"pid"`
	Name       string   `json:"name"`               // nombre amigable («Microsoft Word»)
	Exe        string   `json:"exe"`                // ruta del .exe (vacía si Windows no la da)
	ExeName    string   `json:"exeName"`            // «WINWORD.EXE»
	Window     string   `json:"window,omitempty"`   // título de su ventana principal
	Kind       Kind     `json:"kind"`               // tipo de proceso
	Services   []string `json:"services,omitempty"` // servicios que corren en él
	Files      []string `json:"files"`              // primeros archivos que bloquea
	FileCount  int      `json:"fileCount"`          // total de archivos que bloquea
	Hint       string   `json:"hint,omitempty"`     // explicación corta para el usuario
	HasWindows bool     `json:"hasWindows"`         // tiene ventanas visibles
	CanClose   bool     `json:"canClose"`           // se puede cerrar o forzar desde aquí
	CanUnlock  bool     `json:"canUnlock"`          // se pueden soltar sus archivos sin cerrarlo
	Admin      bool     `json:"admin"`              // de administrador o del sistema: sin permiso para tocarlo
}

// Report es el resultado de Find.
type Report struct {
	Paths     []string  `json:"paths"`            // rutas revisadas
	Processes []Process `json:"processes"`        // quién las bloquea (vacío: nadie)
	Hidden    []string  `json:"hidden,omitempty"` // programas de administrador que no se pudieron revisar por dentro
	Partial   bool      `json:"partial"`          // se agotó el tiempo y puede faltar algo
	Note      string    `json:"note,omitempty"`   // aviso para el usuario
	Millis    int64     `json:"ms"`               // lo que tardó
}

// Topes para que la ventana responda al instante.
const (
	maxFilesShown   = 8
	walkMaxEntries  = 4000
	walkMaxTime     = 150 * time.Millisecond
	directPriority  = 32 // .exe/.dll dentro de carpetas
	directDirs      = 8
	directOthers    = 8
	directWorkers   = 16
	handleScanLimit = 1500 * time.Millisecond
)

// target es una ruta buscada.
type target struct {
	dos  string // ruta normal (C:\...)
	nt   string // ruta NT normalizada (\Device\HarddiskVolumeN\...)
	dir  bool   // carpeta o unidad
	root bool   // unidad entera
}

// Origen de cada coincidencia.
const (
	srcHandle = 1 << iota // archivo abierto
	srcDirect             // Windows dice que lo usa (incluye admin)
	srcImage              // su .exe está ahí
	srcModule             // tiene cargada una DLL de ahí
	srcDir                // tiene abierta una carpeta
)

type hitSet struct {
	mu      sync.Mutex
	files   map[uint32]map[string]string // pid → clave en minúsculas → ruta
	src     map[uint32]int
	handles map[uint32]int // handles encontrados por proceso
	rm      map[uint32]rmProcessInfo
}

func (h *hitSet) add(pid uint32, path string, src int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	m := h.files[pid]
	if m == nil {
		m = map[string]string{}
		h.files[pid] = m
	}
	if path != "" {
		m[strings.ToLower(path)] = path
	}
	h.src[pid] |= src
	if src&srcHandle != 0 {
		h.handles[pid]++
	}
}

var (
	startMu    sync.Mutex
	startTimes = map[uint32]int64{} // hora de inicio vista en Find, contra PID reutilizados
)

// Find averigua qué procesos bloquean esos archivos, carpetas o unidades.
// Funciona sin administrador; lo que no puede revisar lo indica en Hidden.
func Find(ctx context.Context, paths []string) (*Report, error) {
	t0 := time.Now()
	targets, devs, err := resolveTargets(paths)
	if err != nil {
		return nil, err
	}
	rep := &Report{Paths: make([]string, 0, len(targets)), Processes: []Process{}}
	hasDir, hasRoot := false, false
	for _, t := range targets {
		rep.Paths = append(rep.Paths, t.dos)
		hasDir = hasDir || t.dir
		hasRoot = hasRoot || t.root
	}
	hits := &hitSet{files: map[uint32]map[string]string{}, src: map[uint32]int{},
		handles: map[uint32]int{}, rm: map[uint32]rmProcessInfo{}}

	var (
		wg     sync.WaitGroup
		snap   map[uint32]procEntry
		denied map[uint32]bool
		wins   map[uint32][]topWindow
	)
	wg.Add(4)
	go func() { defer wg.Done(); directQueries(ctx, targets, hits) }()
	go func() {
		defer wg.Done()
		nts := make([]ntTarget, 0, len(targets))
		for _, t := range targets {
			if t.nt != "" {
				nts = append(nts, ntTarget{t.nt, t.dir})
			}
		}
		if len(nts) == 0 {
			return
		}
		match := ntMatcher(nts)
		res, err := scanHandles(ctx, scanOpts{match: match, budget: handleScanLimit})
		if err != nil {
			return
		}
		denied = res.denied
		rep.Partial = res.partial
		dirs := map[string]bool{}
		for _, t := range targets {
			if t.dir {
				dirs[strings.ToLower(t.nt)] = true
			}
		}
		for _, h := range res.handles {
			src := srcHandle
			if dirs[strings.ToLower(h.nt)] {
				src |= srcDir
			}
			hits.add(h.pid, devs.toDOS(h.nt), src)
		}
	}()
	go func() {
		defer wg.Done()
		snap = processSnapshot()
		if hasDir {
			imageAndModuleHits(snap, targets, hasRoot, hits)
		}
	}()
	go func() { defer wg.Done(); wins = topWindows() }()
	wg.Wait()

	rep.Processes = buildProcesses(snap, wins, hits)
	if hasDir {
		rep.Hidden = hiddenProcesses(snap, denied, hits)
		if len(rep.Hidden) > 0 {
			rep.Note = "Algunos programas de administrador no se pueden revisar del todo."
		}
	}
	if rep.Partial && rep.Note == "" {
		rep.Note = "No ha dado tiempo a revisarlo todo; puede faltar algún programa."
	}
	rep.Millis = time.Since(t0).Milliseconds()
	return rep, nil
}

// Warmup carga de antemano lo que Find necesita, para que la primera
// búsqueda sea tan rápida como las siguientes. Se puede llamar en segundo plano.
func Warmup() {
	fileTypeOnce.Do(detectFileType)
	for _, p := range []*windows.LazyProc{procNtQueryInformationFile, procGetFileVersionInfo, procVerQueryValue, procDwmGetWindowAttribute} {
		p.Find()
	}
	topWindows()
}

// resolveTargets comprueba las rutas y calcula su forma NT.
func resolveTargets(paths []string) ([]target, devMap, error) {
	devs := buildDevMap()
	var out []target
	var missing string
	seen := map[string]bool{}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if len(p) == 2 && p[1] == ':' {
			p += `\`
		}
		p = filepath.Clean(p)
		if seen[strings.ToLower(p)] {
			continue
		}
		seen[strings.ToLower(p)] = true
		t := target{dos: p}
		t.root = len(p) == 3 && p[1] == ':' && p[2] == '\\'
		u, err := windows.UTF16PtrFromString(longPath(p))
		if err != nil {
			continue
		}
		attr, err := windows.GetFileAttributes(u)
		if err != nil {
			if missing == "" {
				missing = p
			}
			continue
		}
		t.dir = attr&windows.FILE_ATTRIBUTE_DIRECTORY != 0 || t.root
		if h, err := openForQuery(p); err == nil {
			t.nt, _ = finalPathNT(h, nil)
			windows.CloseHandle(h)
		}
		if t.nt == "" && t.root {
			t.nt = devs.device(p[:2])
			if t.nt != "" {
				t.nt += `\`
			}
		}
		if t.root && t.nt != "" && !strings.HasSuffix(t.nt, `\`) {
			t.nt += `\`
		}
		out = append(out, t)
	}
	if len(out) == 0 {
		if missing != "" {
			return nil, devs, errors.New("No se encuentra «" + filepath.Base(missing) + "».")
		}
		return nil, devs, errors.New("No hay nada que revisar.")
	}
	return out, devs, nil
}

// directQueries pregunta a Windows quién usa cada ruta (y, en carpetas, una
// muestra de lo que hay dentro, empezando por programas y bibliotecas).
func directQueries(ctx context.Context, targets []target, hits *hitSet) {
	var cands []cand
	for _, t := range targets {
		cands = append(cands, cand{t.dos, t.dir, true})
	}
	for _, t := range targets {
		if t.dir {
			cands = append(cands, sampleDir(t.dos)...)
		}
	}
	var failed []string
	var fmu sync.Mutex
	ch := make(chan cand)
	var wg sync.WaitGroup
	n := directWorkers
	if len(cands) < n {
		n = len(cands)
	}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range ch {
				h, err := openForQuery(c.path)
				if err != nil {
					continue
				}
				pids, ok := pidsUsingFile(h)
				windows.CloseHandle(h)
				if !ok {
					if c.top && !c.dir {
						fmu.Lock()
						failed = append(failed, c.path)
						fmu.Unlock()
					}
					continue
				}
				src := srcDirect
				if c.dir {
					src |= srcDir
				}
				for _, pid := range pids {
					if pid != windows.GetCurrentProcessId() {
						hits.add(pid, c.path, src)
					}
				}
			}
		}()
	}
	for _, c := range cands {
		if ctx.Err() != nil {
			break
		}
		ch <- c
	}
	close(ch)
	wg.Wait()
	// Respaldo (unidades de red…): el Restart Manager.
	if len(failed) > 0 {
		for _, info := range rmList(failed) {
			hits.mu.Lock()
			hits.rm[info.PID] = info
			hits.mu.Unlock()
			for _, f := range failed {
				hits.add(info.PID, f, srcDirect)
			}
		}
	}
}

// cand es una ruta que se consulta directamente.
type cand struct {
	path string
	dir  bool
	top  bool // la ruta que pidió el usuario
}

// sampleDir elige qué mirar dentro de una carpeta: primero .exe/.dll (que un
// programa puede tener cargados), luego subcarpetas y algunos archivos.
func sampleDir(root string) []cand {
	deadline := time.Now().Add(walkMaxTime)
	var pri, dirs, others []string
	queue := []string{root}
	seen := 0
	for len(queue) > 0 && seen < walkMaxEntries && time.Now().Before(deadline) {
		d := queue[0]
		queue = queue[1:]
		f, err := os.Open(longPath(d))
		if err != nil {
			continue
		}
		for seen < walkMaxEntries {
			ents, err := f.ReadDir(256)
			for _, e := range ents {
				seen++
				p := filepath.Join(d, e.Name())
				if e.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
					continue // enlaces y uniones: no seguirlos
				}
				if e.IsDir() {
					queue = append(queue, p)
					if len(dirs) < directDirs {
						dirs = append(dirs, p)
					}
					continue
				}
				switch strings.ToLower(filepath.Ext(e.Name())) {
				case ".exe", ".dll", ".sys", ".ocx", ".cpl", ".scr", ".node", ".pyd", ".ax":
					if len(pri) < directPriority {
						pri = append(pri, p)
					}
				default:
					if len(others) < directOthers {
						others = append(others, p)
					}
				}
			}
			if err != nil || time.Now().After(deadline) {
				break
			}
		}
		f.Close()
	}
	out := make([]cand, 0, len(pri)+len(dirs)+len(others))
	for _, p := range pri {
		out = append(out, cand{p, false, false})
	}
	for _, p := range dirs {
		out = append(out, cand{p, true, false})
	}
	for _, p := range others {
		out = append(out, cand{p, false, false})
	}
	return out
}

// imageAndModuleHits busca procesos cuyo .exe (o, en unidades, alguna DLL)
// está dentro de las carpetas buscadas.
func imageAndModuleHits(snap map[uint32]procEntry, targets []target, modules bool, hits *hitSet) {
	var dirs []string
	for _, t := range targets {
		if t.dir {
			dirs = append(dirs, t.dos)
		}
	}
	inside := func(p string) bool {
		for _, d := range dirs {
			if dosInside(p, d) {
				return true
			}
		}
		return false
	}
	me := windows.GetCurrentProcessId()
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for pid := range snap {
		if pid <= 4 || pid == me {
			continue
		}
		exe, _, ok := imageInfo(pid)
		if ok && exe != "" && inside(exe) {
			hits.add(pid, exe, srcImage)
		}
		if !modules {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(pid uint32) {
			defer wg.Done()
			defer func() { <-sem }()
			for _, m := range processModules(pid) {
				if inside(m) {
					hits.add(pid, m, srcModule)
				}
			}
		}(pid)
	}
	wg.Wait()
}

// processModules lista las DLL cargadas por un proceso (si deja leerlo).
func processModules(pid uint32) []string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, pid)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(h)
	mods := make([]windows.Handle, 512)
	var need uint32
	const listAll = 0x03
	if windows.EnumProcessModulesEx(h, &mods[0], uint32(len(mods))*8, &need, listAll) != nil {
		return nil
	}
	n := int(need / 8)
	if n > len(mods) {
		n = len(mods)
	}
	out := make([]string, 0, n)
	buf := make([]uint16, windows.MAX_PATH*2)
	for _, m := range mods[:n] {
		if windows.GetModuleFileNameEx(h, m, &buf[0], uint32(len(buf))) == nil {
			out = append(out, windows.UTF16ToString(buf))
		}
	}
	return out
}

// dosInside dice si p es d o está dentro de d (sin distinguir mayúsculas).
func dosInside(p, d string) bool {
	if len(p) < len(d) || !strings.EqualFold(p[:len(d)], d) {
		return false
	}
	return len(p) == len(d) || strings.HasSuffix(d, `\`) || p[len(d)] == '\\'
}

// Programas de ayuda de Windows que corren elevados y no bloquean nada útil.
var quietHelpers = map[string]bool{
	"ctfmon.exe": true, "tabtip.exe": true, "defendersessionhelper.exe": true,
	"securityhealthsystray.exe": true, "conhost.exe": true,
}

// buildProcesses junta las coincidencias en la lista que ve el usuario.
func buildProcesses(snap map[uint32]procEntry, wins map[uint32][]topWindow, hits *hitSet) []Process {
	hits.mu.Lock()
	defer hits.mu.Unlock()
	var svcs map[uint32][]string
	out := make([]Process, 0, len(hits.files))
	for pid, files := range hits.files {
		e, ok := snap[pid]
		if !ok {
			continue // ya no existe
		}
		if svcs == nil {
			svcs = servicesByPID()
		}
		p := describe(pid, e, svcs[pid], wins[pid])
		if info, ok := hits.rm[pid]; ok && p.Kind == KindApp {
			if n := windows.UTF16ToString(info.AppName[:]); n != "" {
				p.Name = n
			}
		}
		list := make([]string, 0, len(files))
		for _, f := range files {
			list = append(list, f)
		}
		sort.Slice(list, func(i, j int) bool { return strings.ToLower(list[i]) < strings.ToLower(list[j]) })
		p.FileCount = len(list)
		if len(list) > maxFilesShown {
			list = list[:maxFilesShown]
		}
		p.Files = list
		src := hits.src[pid]
		p.CanUnlock = hits.handles[pid] > 0 && p.Kind != KindSystem
		p.Hint = hintFor(p, src)
		out = append(out, p)
	}
	rank := func(p Process) int {
		switch {
		case p.Kind == KindApp && p.HasWindows:
			return 0
		case p.Kind == KindApp:
			return 1
		case p.Kind == KindExplorer:
			return 2
		case p.Kind == KindService:
			return 3
		}
		return 4
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := rank(out[i]), rank(out[j])
		if ri != rj {
			return ri < rj
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// describe rellena los datos de un proceso.
func describe(pid uint32, e procEntry, svcs []string, wins []topWindow) Process {
	exe, started, _ := imageInfo(pid)
	if started != 0 {
		startMu.Lock()
		if len(startTimes) > 4096 {
			startTimes = map[uint32]int64{}
		}
		startTimes[pid] = started
		startMu.Unlock()
	}
	p := Process{PID: int(pid), Exe: exe, ExeName: e.exeName, Services: svcs, Kind: kindOf(pid, e.exeName, svcs)}
	for _, w := range wins {
		p.HasWindows = true
		if p.Window == "" && strings.TrimSpace(w.title) != "" {
			p.Window = w.title
		}
	}
	switch p.Kind {
	case KindService:
		if len(svcs) > 0 {
			p.Name = svcs[0]
		}
	case KindExplorer:
		p.Name = "Explorador de Windows"
	}
	if p.Name == "" {
		p.Name = fileDescription(exe)
	}
	if p.Name == "" {
		p.Name = prettyExeName(e.exeName)
	}
	switch p.Kind {
	case KindApp:
		p.Admin = !canOpen(pid, windows.PROCESS_TERMINATE)
		p.CanClose = !p.Admin
	default:
		p.Admin = !canOpen(pid, windows.PROCESS_DUP_HANDLE)
	}
	return p
}

// kindOf clasifica un proceso.
func kindOf(pid uint32, exeName string, svcs []string) Kind {
	name := strings.ToLower(exeName)
	sess, _ := sessionOf(pid)
	switch {
	case name == "explorer.exe" && sess != 0:
		return KindExplorer
	case len(svcs) > 0:
		return KindService
	case pid <= 4 || criticalNames[name] || sess == 0:
		return KindSystem
	}
	return KindApp
}

var consoles = map[string]bool{
	"cmd.exe": true, "powershell.exe": true, "pwsh.exe": true, "bash.exe": true,
	"wsl.exe": true, "windowsterminal.exe": true, "openconsole.exe": true, "conhost.exe": true,
}

// hintFor explica en una frase por qué bloquea.
func hintFor(p Process, src int) string {
	switch {
	case p.Kind == KindSystem:
		return "Es parte de Windows y no se puede cerrar."
	case p.Kind == KindService:
		return "Es un servicio de Windows."
	case p.Kind == KindExplorer:
		return "Lo está usando una ventana del Explorador o su vista previa."
	case src&srcImage != 0:
		return "Es un programa que se está ejecutando desde aquí."
	case src&srcModule != 0:
		return "Tiene cargado un archivo de programa (DLL) de aquí."
	case src&srcDir != 0 && consoles[strings.ToLower(p.ExeName)]:
		return "Es una consola abierta en esta carpeta."
	case p.Admin:
		return "Es un programa de administrador."
	}
	return ""
}

// hiddenProcesses lista los programas del usuario que no se pudieron revisar
// por dentro (normalmente, abiertos como administrador).
func hiddenProcesses(snap map[uint32]procEntry, denied map[uint32]bool, hits *hitSet) []string {
	hits.mu.Lock()
	defer hits.mu.Unlock()
	seen := map[string]bool{}
	var out []string
	for pid := range denied {
		e, ok := snap[pid]
		if !ok || hits.files[pid] != nil {
			continue
		}
		name := strings.ToLower(e.exeName)
		if criticalNames[name] || quietHelpers[name] || seen[name] {
			continue
		}
		if s, ok := sessionOf(pid); !ok || s == 0 {
			continue
		}
		seen[name] = true
		exe, _, _ := imageInfo(pid)
		n := fileDescription(exe)
		if n == "" {
			n = prettyExeName(e.exeName)
		}
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// devMap traduce rutas NT a letras de unidad.
type devMap map[string]string // \Device\HarddiskVolume3 → C:

func buildDevMap() devMap {
	m := devMap{}
	mask, _ := windows.GetLogicalDrives()
	buf := make([]uint16, 1024)
	for i := 0; i < 26; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		letter := string(rune('A'+i)) + ":"
		l, _ := windows.UTF16PtrFromString(letter)
		n, err := windows.QueryDosDevice(l, &buf[0], uint32(len(buf)))
		if err != nil || n == 0 {
			continue
		}
		dev := windows.UTF16ToString(buf[:n])
		if strings.HasPrefix(dev, `\??\`) {
			continue // unidad SUBST
		}
		if _, dup := m[dev]; !dup {
			m[dev] = letter
		}
	}
	return m
}

func (m devMap) device(letter string) string {
	for d, l := range m {
		if strings.EqualFold(l, letter) {
			return d
		}
	}
	return ""
}

// toDOS convierte \Device\HarddiskVolume3\x en C:\x.
func (m devMap) toDOS(nt string) string {
	if strings.HasPrefix(nt, `\Device\Mup\`) {
		return `\\` + nt[len(`\Device\Mup\`):]
	}
	for d, l := range m {
		if len(nt) >= len(d) && strings.EqualFold(nt[:len(d)], d) && (len(nt) == len(d) || nt[len(d)] == '\\') {
			rest := nt[len(d):]
			if rest == "" {
				rest = `\`
			}
			return l + rest
		}
	}
	return nt
}
