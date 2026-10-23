package win11menu

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ServerArg es el argumento con el que el sistema lanza el .exe como servidor COM
// (lo declara el manifiesto del paquete).
const ServerArg = "-TurboMenuCom"

// CLSID de los comandos: uno para archivos/carpetas y otro para el fondo de carpeta.
var (
	CLSIDItems      = guid("{321E46AB-2041-4428-9D9F-CF84AEA715F0}")
	CLSIDBackground = guid("{2454DDF6-7D73-4DCE-8D17-0E01611E9110}")
)

// IsServerLaunch dice si el proceso se ha lanzado como servidor COM del menú.
func IsServerLaunch(args []string) bool {
	for _, a := range args {
		if strings.EqualFold(a, ServerArg) {
			return true
		}
	}
	return false
}

// ServeOptions ajusta el servidor COM.
type ServeOptions struct {
	// Idle: cuánto sigue vivo el proceso sin uso (para que el siguiente clic
	// derecho sea instantáneo). Por defecto 60 s.
	Idle time.Duration
	// Log, si no es nil, recibe una línea por cada llamada COM.
	Log func(string)
}

type server struct {
	menu  Menu
	run   func(string, Selection)
	opt   ServeOptions
	locks atomic.Int32
	last  atomic.Int64
	busy  sync.WaitGroup
	nbusy atomic.Int32

	mu    sync.Mutex
	cache map[bool]cached // por bg
}

type cached struct {
	arr uintptr
	n   uint32
	at  time.Time
	sel Selection
}

var srv = &server{}

// Serve registra las class factories y atiende al Explorador hasta que deja de
// usarlas (más opt.Idle). run se llama en otra goroutine al pulsar un comando,
// con todas las rutas de la selección; Serve espera a que termine antes de volver.
func Serve(m Menu, opt ServeOptions, run func(action string, sel Selection)) error {
	if opt.Idle <= 0 {
		opt.Idle = 60 * time.Second
	}
	srv.menu, srv.run, srv.opt = m, run, opt
	srv.cache = map[bool]cached{}
	srv.touch()
	initVtables()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_MULTITHREADED); err != nil {
		return fmt.Errorf("CoInitializeEx: %w", err)
	}
	defer windows.CoUninitialize()

	facs := []*factory{{vtbl: &factoryVtbl}, {vtbl: &factoryVtbl, bg: true}, {vtbl: &factoryVtbl, exec: true}}
	clsids := []windows.GUID{CLSIDItems, CLSIDBackground, CLSIDExecute}
	var cookies []uint32
	for i, f := range facs {
		keep(f)
		var cookie uint32
		r, _, _ := procCoRegisterClassObj.Call(uintptr(unsafe.Pointer(&clsids[i])),
			uintptr(unsafe.Pointer(f)), clsctxLocalServer, regclsMultipleUse,
			uintptr(unsafe.Pointer(&cookie)))
		if r != 0 {
			return fmt.Errorf("CoRegisterClassObject: 0x%08X", uint32(r))
		}
		cookies = append(cookies, cookie)
	}
	srv.logf("servidor listo pid=%d en %v desde que arrancó el proceso, args=%q", os.Getpid(), sinceStart(), os.Args)

	for {
		time.Sleep(250 * time.Millisecond)
		objs := liveCount.Load() - int32(len(facs))
		idle := time.Since(time.Unix(0, srv.last.Load()))
		if objs <= 0 && srv.locks.Load() <= 0 && srv.nbusy.Load() == 0 && idle > opt.Idle {
			break
		}
	}
	for _, c := range cookies {
		procCoRevokeClassObj.Call(uintptr(c))
	}
	srv.busy.Wait()
	srv.logf("servidor termina")
	return nil
}

func (s *server) touch() { s.last.Store(time.Now().UnixNano()) }

func (s *server) logf(format string, a ...any) {
	if s.opt.Log != nil {
		s.opt.Log(fmt.Sprintf("%s [tid %d] ", time.Now().Format("15:04:05.000"), windows.GetCurrentThreadId()) + fmt.Sprintf(format, a...))
	}
}

func (s *server) invoke(action string, sel Selection) {
	if s.run == nil {
		return
	}
	s.busy.Add(1)
	s.nbusy.Add(1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logf("pánico en %s: %v", action, r)
			}
			s.nbusy.Add(-1)
			s.touch()
			s.busy.Done()
		}()
		s.run(action, sel)
	}()
}

// selection lee la selección con caché: el Explorador pasa el mismo array a
// GetTitle/GetState de cada comando y cada lectura son varias llamadas entre procesos.
func (s *server) selection(arr *iface, bg bool) Selection {
	if arr == nil {
		// Sin array (algunas llamadas no lo pasan): la última selección vista.
		if sel, ok := s.lastSelection(bg); ok {
			return sel
		}
		if bg {
			return s.siteSelection()
		}
		return Selection{}
	}
	key := uintptr(unsafe.Pointer(arr))
	n := arrCount(arr)
	if bg && n == 0 {
		return s.siteSelection()
	}
	s.mu.Lock()
	c, ok := s.cache[bg]
	s.mu.Unlock()
	if ok && c.arr == key && c.n == n && time.Since(c.at) < 5*time.Second {
		return c.sel
	}
	sel := s.readSelection(arr, bg)
	s.mu.Lock()
	s.cache[bg] = cached{arr: key, n: n, at: time.Now(), sel: sel}
	s.mu.Unlock()
	return sel
}

// lastSelection: la última selección vista (EnumSubCommands no la recibe).
func (s *server) lastSelection(bg bool) (Selection, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.cache[bg]
	if !ok || time.Since(c.at) > 30*time.Second {
		return Selection{}, false
	}
	return c.sel, true
}

func arrCount(arr *iface) uint32 {
	if arr == nil {
		return 0
	}
	var n uint32
	if arr.call(7, uintptr(unsafe.Pointer(&n))) != 0 {
		return 0
	}
	return n
}

// readSelection: IShellItemArray -> rutas (GetCount, GetItemAt, GetDisplayName).
func (s *server) readSelection(arr *iface, bg bool) Selection {
	sel := Selection{Background: bg}
	n := arrCount(arr)
	if bg && n == 0 {
		return s.siteSelection()
	}
	for i := uint32(0); i < n; i++ {
		var it *iface
		if arr.call(8, uintptr(i), uintptr(unsafe.Pointer(&it))) != 0 || it == nil {
			sel.NonFS = true
			continue
		}
		var p *uint16
		if it.call(5, sigdnFileSysPath, uintptr(unsafe.Pointer(&p))) == 0 && p != nil {
			path := windows.UTF16PtrToString(p)
			windows.CoTaskMemFree(unsafe.Pointer(p))
			sel.Paths = append(sel.Paths, path)
			sel.Dirs = append(sel.Dirs, isDir(path))
		} else {
			sel.NonFS = true
		}
		it.release()
	}
	return sel
}

// procName: nombre del ejecutable de un PID (para el log).
func procName(pid uint32) string {
	if pid == 0 {
		return "?"
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return fmt.Sprintf("%d", pid)
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 512)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return fmt.Sprintf("%d", pid)
	}
	return fmt.Sprintf("%s(%d)", filepath.Base(windows.UTF16ToString(buf[:n])), pid)
}

// FileLog devuelve un Log que añade líneas a un archivo.
func FileLog(path string) func(string) {
	var mu sync.Mutex
	return func(line string) {
		mu.Lock()
		defer mu.Unlock()
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		f.WriteString(line + "\n")
		f.Close()
	}
}

// sinceStart: tiempo desde la creación del proceso (para medir el arranque en frío).
func sinceStart() time.Duration {
	var c, e, k, u windows.Filetime
	if windows.GetProcessTimes(windows.CurrentProcess(), &c, &e, &k, &u) != nil {
		return 0
	}
	return time.Since(time.Unix(0, c.Nanoseconds())).Round(time.Millisecond)
}
