package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                    = windows.NewLazySystemDLL("user32.dll")
	shell32                   = windows.NewLazySystemDLL("shell32.dll")
	shcore                    = windows.NewLazySystemDLL("shcore.dll")
	procGetCursorPos          = user32.NewProc("GetCursorPos")
	procEnumWindows           = user32.NewProc("EnumWindows")
	procGetWindowThreadPID    = user32.NewProc("GetWindowThreadProcessId")
	procGetClassName          = user32.NewProc("GetClassNameW")
	procSetWindowPos          = user32.NewProc("SetWindowPos")
	procGetWindowRect         = user32.NewProc("GetWindowRect")
	procMonitorFromPoint      = user32.NewProc("MonitorFromPoint")
	procMonitorFromWindow     = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfo        = user32.NewProc("GetMonitorInfoW")
	procShowWindow            = user32.NewProc("ShowWindow")
	procSetForegroundWindow   = user32.NewProc("SetForegroundWindow")
	procAllowSetForeground    = user32.NewProc("AllowSetForegroundWindow")
	procBringWindowToTop      = user32.NewProc("BringWindowToTop")
	procIsWindowVisible       = user32.NewProc("IsWindowVisible")
	procIsIconic              = user32.NewProc("IsIconic")
	procGetForegroundWindow   = user32.NewProc("GetForegroundWindow")
	procAttachThreadInput     = user32.NewProc("AttachThreadInput")
	procKeybdEvent            = user32.NewProc("keybd_event")
	procGetDpiForMonitor      = shcore.NewProc("GetDpiForMonitor")
	procSHOpenFolderAndSelect = shell32.NewProc("SHOpenFolderAndSelectItems")
	procILCreateFromPath      = shell32.NewProc("ILCreateFromPathW")
	procILFree                = shell32.NewProc("ILFree")
	procSHChangeNotify        = shell32.NewProc("SHChangeNotify")
	procGetCurrentThreadID    = kernel32.NewProc("GetCurrentThreadId")
	procFlashWindowEx         = user32.NewProc("FlashWindowEx")
	procSetWindowLongPtr      = user32.NewProc("SetWindowLongPtrW")
	procGetWindowLongPtr      = user32.NewProc("GetWindowLongPtrW")
	procDwmSetWindowAttribute = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
)

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

func cursorPos() (int32, int32) {
	var p point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return p.X, p.Y
}

// findOwnWindow busca la ventana principal de este proceso.
func findOwnWindow() uintptr {
	pid := uint32(os.Getpid())
	var found uintptr
	cb := syscall.NewCallback(func(h uintptr, _ uintptr) uintptr {
		var wp uint32
		procGetWindowThreadPID.Call(h, uintptr(unsafe.Pointer(&wp)))
		if wp != pid {
			return 1
		}
		buf := make([]uint16, 64)
		procGetClassName.Call(h, uintptr(unsafe.Pointer(&buf[0])), 64)
		if windows.UTF16ToString(buf) == windowClass {
			found = h
			return 0
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
	return found
}

func monitorAt(x, y int32) (work rect, dpi uint32) {
	pt := uintptr(uint32(x)) | uintptr(uint32(y))<<32
	hm, _, _ := procMonitorFromPoint.Call(pt, 2) // MONITOR_DEFAULTTONEAREST
	mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	procGetMonitorInfo.Call(hm, uintptr(unsafe.Pointer(&mi)))
	var dx, dy uint32 = 96, 96
	if procGetDpiForMonitor.Find() == nil {
		procGetDpiForMonitor.Call(hm, 0, uintptr(unsafe.Pointer(&dx)), uintptr(unsafe.Pointer(&dy)))
	}
	if dx == 0 {
		dx = 96
	}
	return mi.Work, dx
}

const (
	swpNoSize     = 0x0001
	swpNoMove     = 0x0002
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010
	swpShowWindow = 0x0040
)

// placeWindow coloca la ventana (tamaño en píxeles CSS) junto al punto (x, y) de la
// pantalla, dentro del área útil de ese monitor. center la centra en el monitor.
func placeWindow(hwnd uintptr, cssW, cssH int, x, y int32, center bool) {
	if hwnd == 0 {
		return
	}
	work, dpi := monitorAt(x, y)
	// Primero la movemos a ese monitor, para que Windows aplique su escala antes de medir.
	procSetWindowPos.Call(hwnd, 0, uintptr(work.Left+8), uintptr(work.Top+8), 0, 0, swpNoSize|swpNoZOrder|swpNoActivate)
	w := int32(cssW * int(dpi) / 96)
	h := int32(cssH * int(dpi) / 96)
	ww, wh := work.Right-work.Left, work.Bottom-work.Top
	if w > ww-16 {
		w = ww - 16
	}
	if h > wh-16 {
		h = wh - 16
	}
	var px, py int32
	if center {
		px = work.Left + (ww-w)/2
		py = work.Top + (wh-h)/2
	} else {
		// A la derecha y un poco por encima del cursor, como si saliera del propio menú.
		off := int32(14 * int(dpi) / 96)
		px = x + off
		py = y - h/3
		if px+w > work.Right-8 {
			px = x - w - off
		}
	}
	if px < work.Left+8 {
		px = work.Left + 8
	}
	if px+w > work.Right-8 {
		px = work.Right - 8 - w
	}
	if py < work.Top+8 {
		py = work.Top + 8
	}
	if py+h > work.Bottom-8 {
		py = work.Bottom - 8 - h
	}
	procSetWindowPos.Call(hwnd, 0, uintptr(px), uintptr(py), uintptr(w), uintptr(h), swpNoZOrder|swpNoActivate)
}

// resizeKeep cambia el tamaño manteniendo la ventana dentro de su monitor.
func resizeKeep(hwnd uintptr, cssW, cssH int) {
	if hwnd == 0 {
		return
	}
	var r rect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	work, dpi := monitorAt((r.Left+r.Right)/2, (r.Top+r.Bottom)/2)
	w := int32(cssW * int(dpi) / 96)
	h := int32(cssH * int(dpi) / 96)
	if h > work.Bottom-work.Top-16 {
		h = work.Bottom - work.Top - 16
	}
	// Crece desde el centro vertical actual para que no "salte".
	cy := (r.Top + r.Bottom) / 2
	px, py := r.Left, cy-h/2
	if px+w > work.Right-8 {
		px = work.Right - 8 - w
	}
	if px < work.Left+8 {
		px = work.Left + 8
	}
	if py+h > work.Bottom-8 {
		py = work.Bottom - 8 - h
	}
	if py < work.Top+8 {
		py = work.Top + 8
	}
	procSetWindowPos.Call(hwnd, 0, uintptr(px), uintptr(py), uintptr(w), uintptr(h), swpNoZOrder|swpNoActivate)
}

// bringToFront muestra la ventana y le da el foco aunque el proceso estuviera en segundo plano.
func bringToFront(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	if ic, _, _ := procIsIconic.Call(hwnd); ic != 0 {
		procShowWindow.Call(hwnd, 9) // SW_RESTORE
	} else {
		procShowWindow.Call(hwnd, 5) // SW_SHOW
	}
	fg, _, _ := procGetForegroundWindow.Call()
	if fg == hwnd {
		return
	}
	// Truco habitual: una pulsación de Alt "desbloquea" SetForegroundWindow.
	procKeybdEvent.Call(0x12, 0, 0, 0)
	procKeybdEvent.Call(0x12, 0, 2, 0)
	var fgPid uint32
	fgThread, _, _ := procGetWindowThreadPID.Call(fg, uintptr(unsafe.Pointer(&fgPid)))
	me, _, _ := procGetCurrentThreadID.Call()
	if fgThread != 0 && fgThread != me {
		procAttachThreadInput.Call(me, fgThread, 1)
		defer procAttachThreadInput.Call(me, fgThread, 0)
	}
	procBringWindowToTop.Call(hwnd)
	procSetForegroundWindow.Call(hwnd)
}

func hideWindow(hwnd uintptr) {
	if hwnd != 0 {
		procShowWindow.Call(hwnd, 0)
	}
}

func allowForeground(pid int) { procAllowSetForeground.Call(uintptr(pid)) }

// selectInExplorer selecciona path en su carpeta del Explorador (abriéndola si hace
// falta). Con rename deja el nombre listo para escribir.
func selectInExplorer(path string, rename bool) {
	dir := filepath.Dir(path)
	pd, _ := windows.UTF16PtrFromString(dir)
	pf, _ := windows.UTF16PtrFromString(path)
	windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
	folder, _, _ := procILCreateFromPath.Call(uintptr(unsafe.Pointer(pd)))
	item, _, _ := procILCreateFromPath.Call(uintptr(unsafe.Pointer(pf)))
	if folder == 0 || item == 0 {
		explorer(`/select,"` + path + `"`)
		return
	}
	defer procILFree.Call(folder)
	defer procILFree.Call(item)
	var flags uintptr
	if rename {
		flags = 1 // OFASI_EDIT
	}
	items := []uintptr{item}
	r, _, _ := procSHOpenFolderAndSelect.Call(folder, 1, uintptr(unsafe.Pointer(&items[0])), flags)
	if r != 0 {
		explorer(`/select,"` + path + `"`)
	}
}

// notifyChanged avisa al Explorador de que algo ha cambiado en una carpeta.
func notifyChanged(dir string) {
	p, _ := windows.UTF16PtrFromString(dir)
	procSHChangeNotify.Call(0x00001000, 0x0005|0x1000, uintptr(unsafe.Pointer(p)), 0) // SHCNE_UPDATEDIR, SHCNF_PATHW|FLUSH
}

func explorer(args string) {
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: "explorer.exe " + args}
	_ = cmd.Start()
	go cmd.Wait()
}

const createNoWindow = 0x08000000

func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

// darkTitle no hace falta en una ventana sin marco; esto quita además la sombra rara de
// algunos temas y pide las esquinas redondeadas de Windows 11.
func roundCorners(hwnd uintptr) {
	if hwnd == 0 || procDwmSetWindowAttribute.Find() != nil {
		return
	}
	pref := uint32(2) // DWMWCP_ROUND
	procDwmSetWindowAttribute.Call(hwnd, 33, uintptr(unsafe.Pointer(&pref)), 4)
}
