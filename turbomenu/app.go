package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"turbomenu/internal/lock"
	"turbomenu/internal/shellx"
)

type App struct {
	ctx  context.Context
	hwnd uintptr

	mu        sync.Mutex
	job       *Job
	busy      bool // hay una ventana con trabajo (no acepta otro)
	collector *pipeServer
	idleSrv   *pipeServer
	idleTimer *time.Timer
	startIdle bool
	gen       int // cambia con cada trabajo nuevo, para ignorar respuestas viejas

	tasks sync.Map // id → context.CancelFunc
	qr    *qrSession
}

var testMode = os.Getenv("TURBOMENU_TEST") == "1"

// appendTestLog apunta tiempos en %TEMP%\TurboMENU-test.log (solo en modo prueba).
func appendTestLog(line string) {
	f, err := os.OpenFile(filepath.Join(os.TempDir(), "TurboMENU-test.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		fmt.Fprintf(f, "%d %s\n", os.Getpid(), line)
		f.Close()
	}
}

func NewApp(job *Job, idle bool) *App {
	return &App{job: job, startIdle: idle, busy: job != nil}
}

func (a *App) emit(name string, data ...any) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, name, data...)
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.mu.Lock()
	job := a.job
	a.mu.Unlock()
	if job != nil && job.Action != "home" {
		a.startCollector(job.Action)
	}
	releaseLead()
	go shellx.Warmup()
	if job != nil && job.Action == "lock" {
		go lock.Warmup()
	}
	go syncMenu()
	cleanupLater()
}

func (a *App) domReady(ctx context.Context) {
	a.hwnd = findOwnWindow()
	roundCorners(a.hwnd)
	if a.startIdle {
		a.goIdle()
	}
}

func (a *App) shutdown(ctx context.Context) {
	a.tasks.Range(func(_, v any) bool { v.(context.CancelFunc)(); return true })
	a.stopQR()
	if a.collector != nil {
		a.collector.stop()
	}
	if a.idleSrv != nil {
		a.idleSrv.stop()
	}
}

// beforeClose: Alt+F4 y similares pasan por Close (que puede dejar el proceso preparado).
func (a *App) beforeClose(ctx context.Context) bool {
	if a.isIdle() {
		return false
	}
	go a.Close()
	return true
}

func (a *App) isIdle() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return !a.busy
}

// startCollector recoge durante unos segundos las rutas que lleguen de otros procesos
// lanzados para la misma selección.
func (a *App) startCollector(action string) {
	srv := servePipe(pipeName("collect."+action), func(j *Job) bool {
		if j.Action != action {
			return false
		}
		a.addPaths(j.Paths)
		return true
	})
	a.mu.Lock()
	a.collector = srv
	gen := a.gen
	a.mu.Unlock()
	time.AfterFunc(collectWindow, func() {
		srv.stop()
		a.mu.Lock()
		if a.collector == srv && a.gen == gen {
			a.collector = nil
		}
		a.mu.Unlock()
	})
}

func (a *App) addPaths(paths []string) {
	a.mu.Lock()
	if a.job == nil {
		a.mu.Unlock()
		return
	}
	seen := map[string]bool{}
	for _, p := range a.job.Paths {
		seen[strings.ToLower(p)] = true
	}
	added := false
	for _, p := range paths {
		if !seen[strings.ToLower(p)] {
			seen[strings.ToLower(p)] = true
			a.job.Paths = append(a.job.Paths, p)
			added = true
		}
	}
	a.mu.Unlock()
	if added {
		if testMode {
			appendTestLog(fmt.Sprintf("paths +%d", len(paths)))
		}
		a.emit("paths", a.GetJob())
	}
}

// ---------- ventana ----------

// Show coloca y enseña la ventana (tamaño en píxeles CSS). La interfaz lo llama cuando
// ya ha pintado el trabajo, para que no se vea nada a medio cargar.
func (a *App) Show(w, h int) {
	if a.hwnd == 0 {
		a.hwnd = findOwnWindow()
	}
	a.mu.Lock()
	job := a.job
	a.mu.Unlock()
	if job == nil {
		return
	}
	center := job.Action == "home"
	x, y := job.X, job.Y
	if center {
		x, y = cursorPos()
	}
	if testMode {
		// Pruebas: se pinta detrás de todo y sin quitar el foco a nadie.
		placeWindow(a.hwnd, w, h, 40, 40, false)
		procSetWindowPos.Call(a.hwnd, 1, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate) // HWND_BOTTOM
		procShowWindow.Call(a.hwnd, 4)                                                  // SW_SHOWNOACTIVATE
		appendTestLog(fmt.Sprintf("show %s %dms paths=%d", job.Action, time.Now().UnixMilli()-job.Time, len(job.Paths)))
		if ms, _ := strconv.Atoi(os.Getenv("TURBOMENU_AUTOCLOSE")); ms > 0 {
			time.AfterFunc(time.Duration(ms)*time.Millisecond, a.Close)
		}
		return
	}
	placeWindow(a.hwnd, w, h, x, y, center)
	runtime.WindowShow(a.ctx)
	bringToFront(a.hwnd)
}

// Resize cambia el tamaño de la ventana (p. ej. al pasar a la pantalla de resultado).
func (a *App) Resize(w, h int) { resizeKeep(a.hwnd, w, h) }

func (a *App) Minimise() { runtime.WindowMinimise(a.ctx) }

// Close cierra la ventana. El proceso se queda preparado (oculto) un rato para que la
// siguiente acción se abra al instante.
func (a *App) Close() {
	a.tasks.Range(func(k, v any) bool { v.(context.CancelFunc)(); a.tasks.Delete(k); return true })
	a.stopQR()
	a.goIdle()
}

func (a *App) goIdle() {
	hideWindow(a.hwnd)
	a.mu.Lock()
	a.job = nil
	a.busy = false
	a.gen++
	if a.collector != nil {
		a.collector.stop()
		a.collector = nil
	}
	a.mu.Unlock()
	a.emit("reset")

	// Solo un TurboMENU preparado a la vez: si ya hay otro, este sobra.
	if h, err := dialPipe(pipeName("idle"), 0); err == nil {
		closeHandle(h)
		runtime.Quit(a.ctx)
		return
	}
	srv := servePipe(pipeName("idle"), a.acceptJob)
	mins := getSettings().IdleMins
	a.mu.Lock()
	a.idleSrv = srv
	if a.idleTimer != nil {
		a.idleTimer.Stop()
	}
	if !a.startIdle && !getSettings().Prewarm {
		a.idleTimer = time.AfterFunc(time.Duration(mins)*time.Minute, func() {
			if a.isIdle() {
				runtime.Quit(a.ctx)
			}
		})
	}
	a.mu.Unlock()
}

// acceptJob recibe un trabajo nuevo estando preparado.
func (a *App) acceptJob(j *Job) bool {
	if j.Action == "quit" {
		go func() { time.Sleep(100 * time.Millisecond); runtime.Quit(a.ctx) }()
		return true
	}
	a.mu.Lock()
	if a.busy {
		a.mu.Unlock()
		return false
	}
	a.busy = true
	a.job = j
	a.gen++
	srv := a.idleSrv
	a.idleSrv = nil
	if a.idleTimer != nil {
		a.idleTimer.Stop()
		a.idleTimer = nil
	}
	a.mu.Unlock()
	if srv != nil {
		go srv.stop()
	}
	if j.Action != "home" {
		a.startCollector(j.Action)
	}
	a.emit("job", a.GetJob())
	return true
}

// ---------- trabajo actual ----------

type FileInfo struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Ext   string `json:"ext"`
	Dir   string `json:"dir"`
	Kind  string `json:"kind"` // video | audio | image | pdf | doc | exe | archive | text | folder | drive | other
	Size  int64  `json:"size"`
	IsDir bool   `json:"isDir"`
	Mod   int64  `json:"mod"`
	Err   string `json:"err"`
}

type JobView struct {
	Action  string     `json:"action"`
	Files   []FileInfo `json:"files"`
	Total   int64      `json:"total"`
	Kinds   []string   `json:"kinds"` // tipos presentes, de más a menos
	Version string     `json:"version"`
	Gen     int        `json:"gen"`
	Deps    Deps       `json:"deps"`
}

func (a *App) GetJob() JobView {
	a.mu.Lock()
	job := a.job
	gen := a.gen
	a.mu.Unlock()
	v := JobView{Version: version, Gen: gen, Deps: detectDeps(), Files: []FileInfo{}}
	if job == nil {
		return v
	}
	v.Action = job.Action
	count := map[string]int{}
	for _, p := range job.Paths {
		fi := statFile(p)
		v.Files = append(v.Files, fi)
		v.Total += fi.Size
		count[fi.Kind]++
	}
	for k := range count {
		v.Kinds = append(v.Kinds, k)
	}
	sort.Slice(v.Kinds, func(i, j int) bool { return count[v.Kinds[i]] > count[v.Kinds[j]] })
	return v
}

func statFile(p string) FileInfo {
	fi := FileInfo{Path: p, Name: filepath.Base(p), Dir: filepath.Dir(p)}
	if len(p) <= 3 && strings.HasSuffix(p, `:\`) {
		fi.Kind, fi.Name, fi.IsDir = "drive", strings.TrimSuffix(p, `\`), true
		return fi
	}
	st, err := os.Stat(p)
	if err != nil {
		fi.Err = "No se encuentra"
		fi.Kind = kindOf(p)
		return fi
	}
	fi.Mod = st.ModTime().Unix()
	if st.IsDir() {
		fi.IsDir, fi.Kind = true, "folder"
		return fi
	}
	fi.Size = st.Size()
	fi.Ext = strings.TrimPrefix(strings.ToLower(filepath.Ext(p)), ".")
	fi.Kind = kindOf(p)
	return fi
}

var kindByExt = map[string]string{}

func init() {
	add := func(kind string, exts ...string) {
		for _, e := range exts {
			kindByExt[e] = kind
		}
	}
	add("video", "mp4", "mkv", "mov", "avi", "webm", "wmv", "flv", "m4v", "mpg", "mpeg", "3gp", "ts", "mts", "m2ts", "ogv", "vob")
	add("audio", "mp3", "wav", "flac", "m4a", "aac", "ogg", "opus", "wma", "aiff", "aif", "alac", "amr", "mka")
	add("image", "jpg", "jpeg", "png", "gif", "bmp", "tif", "tiff", "webp", "heic", "heif", "avif", "ico", "jfif", "jxr", "dng", "cr2", "nef", "arw")
	add("pdf", "pdf")
	add("doc", "doc", "docx", "odt", "rtf", "xls", "xlsx", "ods", "ppt", "pptx", "odp")
	add("exe", "exe", "msi", "bat", "cmd", "ps1", "scr", "dll", "com", "vbs", "js", "jar", "appx", "msix", "lnk")
	add("archive", "zip", "rar", "7z", "tar", "gz", "bz2", "xz", "iso", "cab")
	add("text", "txt", "md", "csv", "json", "xml", "html", "htm", "log", "ini", "yaml", "yml")
}

func kindOf(p string) string {
	if k, ok := kindByExt[strings.TrimPrefix(strings.ToLower(filepath.Ext(p)), ".")]; ok {
		return k
	}
	return "other"
}

// ---------- utilidades para la interfaz ----------

func (a *App) Open(path string)   { explorer(`"` + path + `"`) }
func (a *App) Reveal(path string) { selectInExplorer(path, false) }
func (a *App) OpenURL(u string)   { runtime.BrowserOpenURL(a.ctx, u) }
func (a *App) CopyText(t string) error {
	return runtime.ClipboardSetText(a.ctx, t)
}

// PickFolder abre el diálogo de elegir carpeta.
func (a *App) PickFolder(title, start string) string {
	p, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: title, DefaultDirectory: start})
	if err != nil {
		return ""
	}
	return p
}

// newTask registra una tarea cancelable y devuelve su contexto.
func (a *App) newTask(id string) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	if old, ok := a.tasks.Swap(id, context.CancelFunc(cancel)); ok {
		old.(context.CancelFunc)()
	}
	return ctx
}

func (a *App) endTask(id string) {
	if v, ok := a.tasks.LoadAndDelete(id); ok {
		v.(context.CancelFunc)()
	}
}

// Cancel detiene una tarea en curso.
func (a *App) Cancel(id string) { a.endTask(id) }

// cleanupLater borra temporales viejos de otras sesiones (listas de rutas, previas…).
func cleanupLater() {
	go func() {
		time.Sleep(5 * time.Second)
		matches, _ := filepath.Glob(filepath.Join(os.TempDir(), "TurboMENU-*"))
		for _, m := range matches {
			if st, err := os.Stat(m); err == nil && time.Since(st.ModTime()) > 24*time.Hour {
				os.RemoveAll(m)
			}
		}
	}()
}
