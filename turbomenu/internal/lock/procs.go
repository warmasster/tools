//go:build windows

package lock

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// procEntry son los datos básicos de un proceso (de la instantánea del sistema).
type procEntry struct {
	pid     uint32
	parent  uint32
	exeName string
}

// processSnapshot lista todos los procesos (PID → nombre del .exe).
func processSnapshot() map[uint32]procEntry {
	out := map[uint32]procEntry{}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return out
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		out[e.ProcessID] = procEntry{e.ProcessID, e.ParentProcessID, windows.UTF16ToString(e.ExeFile[:])}
	}
	return out
}

// imageInfo devuelve la ruta del .exe y la hora de inicio de un proceso.
func imageInfo(pid uint32) (exe string, started int64, ok bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", 0, false
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) == nil {
		exe = windows.UTF16ToString(buf[:n])
	}
	var c, x, k, u windows.Filetime
	if windows.GetProcessTimes(h, &c, &x, &k, &u) == nil {
		started = c.Nanoseconds()
	}
	return exe, started, true
}

// canOpen indica si este proceso nos deja pedir ciertos permisos sobre él.
func canOpen(pid uint32, access uint32) bool {
	h, err := windows.OpenProcess(access, false, pid)
	if err != nil {
		return false
	}
	windows.CloseHandle(h)
	return true
}

func sessionOf(pid uint32) (uint32, bool) {
	var s uint32
	if windows.ProcessIdToSessionId(pid, &s) != nil {
		return 0, false
	}
	return s, true
}

var (
	descMu    sync.Mutex
	descCache = map[string]string{}
)

// fileDescription lee la descripción del ejecutable (p. ej. «Microsoft Word»).
func fileDescription(exe string) string {
	if exe == "" {
		return ""
	}
	key := strings.ToLower(exe)
	descMu.Lock()
	d, ok := descCache[key]
	descMu.Unlock()
	if ok {
		return d
	}
	d = readDescription(exe)
	descMu.Lock()
	descCache[key] = d
	descMu.Unlock()
	return d
}

func readDescription(exe string) string {
	p, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return ""
	}
	size, _, _ := syscall.SyscallN(procGetFileVersionInfoSize.Addr(), uintptr(unsafe.Pointer(p)), 0)
	if size == 0 {
		return ""
	}
	data := make([]byte, size)
	if r, _, _ := syscall.SyscallN(procGetFileVersionInfo.Addr(), uintptr(unsafe.Pointer(p)), 0, size,
		uintptr(unsafe.Pointer(&data[0]))); r == 0 {
		return ""
	}
	query := func(sub string) (unsafe.Pointer, uint32) {
		s, _ := windows.UTF16PtrFromString(sub)
		var ptr unsafe.Pointer
		var n uint32
		r, _, _ := syscall.SyscallN(procVerQueryValue.Addr(), uintptr(unsafe.Pointer(&data[0])),
			uintptr(unsafe.Pointer(s)), uintptr(unsafe.Pointer(&ptr)), uintptr(unsafe.Pointer(&n)))
		if r == 0 || ptr == nil || n == 0 {
			return nil, 0
		}
		return ptr, n
	}
	var langs []string
	if ptr, n := query(`\VarFileInfo\Translation`); ptr != nil {
		pairs := unsafe.Slice((*uint16)(ptr), n/2)
		for i := 0; i+1 < len(pairs); i += 2 {
			langs = append(langs, fmt.Sprintf("%04x%04x", pairs[i], pairs[i+1]))
		}
	}
	langs = append(langs, "040904b0", "040904e4", "0c0a04b0", "000004b0")
	for _, l := range langs {
		if ptr, n := query(`\StringFileInfo\` + l + `\FileDescription`); ptr != nil {
			s := strings.TrimSpace(windows.UTF16ToString(unsafe.Slice((*uint16)(ptr), n)))
			if s != "" {
				return s
			}
		}
	}
	return ""
}

// Ventanas de nivel superior visibles, por proceso.
type topWindow struct {
	hwnd  windows.HWND
	title string
}

var (
	enumMu    sync.Mutex
	enumOut   map[uint32][]topWindow
	enumCB    uintptr
	enumCBSet sync.Once

	dwmapi                    = windows.NewLazySystemDLL("dwmapi.dll")
	procDwmGetWindowAttribute = dwmapi.NewProc("DwmGetWindowAttribute")
)

// topWindows devuelve, por PID, sus ventanas principales visibles.
func topWindows() map[uint32][]topWindow {
	enumCBSet.Do(func() {
		enumCB = windows.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
			if !windows.IsWindowVisible(h) {
				return 1
			}
			// Ventanas principales: sin dueño o con un dueño oculto (WinForms…).
			if owner, _, _ := syscall.SyscallN(procGetWindow.Addr(), uintptr(h), gwOwner); owner != 0 && windows.IsWindowVisible(windows.HWND(owner)) {
				return 1
			}
			var cloaked uint32
			if procDwmGetWindowAttribute.Find() == nil {
				syscall.SyscallN(procDwmGetWindowAttribute.Addr(), uintptr(h), 14, // DWMWA_CLOAKED
					uintptr(unsafe.Pointer(&cloaked)), 4)
			}
			if cloaked != 0 {
				return 1
			}
			var pid uint32
			windows.GetWindowThreadProcessId(h, &pid)
			buf := make([]uint16, 256)
			n, _, _ := syscall.SyscallN(procGetWindowTextW.Addr(), uintptr(h), uintptr(unsafe.Pointer(&buf[0])), 256)
			enumOut[pid] = append(enumOut[pid], topWindow{h, windows.UTF16ToString(buf[:n])})
			return 1
		})
	})
	enumMu.Lock()
	defer enumMu.Unlock()
	enumOut = map[uint32][]topWindow{}
	windows.EnumWindows(enumCB, nil)
	out := enumOut
	enumOut = nil
	return out
}

// servicesByPID devuelve los nombres visibles de los servicios de cada proceso.
func servicesByPID() map[uint32][]string {
	out := map[uint32][]string{}
	m, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_ENUMERATE_SERVICE)
	if err != nil {
		return out
	}
	defer windows.CloseServiceHandle(m)
	var need, count, resume uint32
	buf := make([]byte, 256<<10)
	for {
		err = windows.EnumServicesStatusEx(m, windows.SC_ENUM_PROCESS_INFO, windows.SERVICE_WIN32,
			windows.SERVICE_ACTIVE, &buf[0], uint32(len(buf)), &need, &count, &resume, nil)
		if err == windows.ERROR_MORE_DATA && need > uint32(len(buf)) && count == 0 {
			buf = make([]byte, need)
			continue
		}
		if err != nil && err != windows.ERROR_MORE_DATA {
			return out
		}
		list := unsafe.Slice((*windows.ENUM_SERVICE_STATUS_PROCESS)(unsafe.Pointer(&buf[0])), count)
		for _, s := range list {
			if s.ServiceStatusProcess.ProcessId != 0 {
				name := windows.UTF16PtrToString(s.DisplayName)
				pid := s.ServiceStatusProcess.ProcessId
				out[pid] = append(out[pid], name)
			}
		}
		if err == nil {
			return out
		}
	}
}

// Procesos que Windows necesita para funcionar: nunca se cierran.
var criticalNames = map[string]bool{
	"system": true, "registry": true, "secure system": true, "memory compression": true,
	"smss.exe": true, "csrss.exe": true, "wininit.exe": true, "winlogon.exe": true,
	"services.exe": true, "lsass.exe": true, "lsaiso.exe": true, "dwm.exe": true,
	"fontdrvhost.exe": true, "svchost.exe": true, "sihost.exe": true, "audiodg.exe": true,
	"spoolsv.exe": true, "msmpeng.exe": true, "nissrv.exe": true, "searchindexer.exe": true,
}

// prettyExeName convierte «notepad.exe» en «Notepad».
func prettyExeName(exe string) string {
	b := strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
	if b == "" {
		return exe
	}
	return strings.ToUpper(b[:1]) + b[1:]
}
