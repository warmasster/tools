package main

import (
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	shcore   = windows.NewLazySystemDLL("shcore.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassEx      = user32.NewProc("RegisterClassExW")
	procCreateWindowEx       = user32.NewProc("CreateWindowExW")
	procDefWindowProc        = user32.NewProc("DefWindowProcW")
	procGetMessage           = user32.NewProc("GetMessageW")
	procTranslateMessage     = user32.NewProc("TranslateMessage")
	procDispatchMessage      = user32.NewProc("DispatchMessageW")
	procPostMessage          = user32.NewProc("PostMessageW")
	procPostQuitMessage      = user32.NewProc("PostQuitMessage")
	procDestroyWindow        = user32.NewProc("DestroyWindow")
	procRegisterWindowMsg    = user32.NewProc("RegisterWindowMessageW")
	procCreatePopupMenu      = user32.NewProc("CreatePopupMenu")
	procAppendMenu           = user32.NewProc("AppendMenuW")
	procTrackPopupMenu       = user32.NewProc("TrackPopupMenu")
	procDestroyMenu          = user32.NewProc("DestroyMenu")
	procSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	procGetForegroundWindow  = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadPID   = user32.NewProc("GetWindowThreadProcessId")
	procGetCursorPos         = user32.NewProc("GetCursorPos")
	procCreateIconFromRes    = user32.NewProc("CreateIconFromResourceEx")
	procDestroyIcon          = user32.NewProc("DestroyIcon")
	procGetSystemMetrics     = user32.NewProc("GetSystemMetrics")
	procSetTimer             = user32.NewProc("SetTimer")
	procRegisterHotKey       = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey     = user32.NewProc("UnregisterHotKey")
	procSetWindowPos         = user32.NewProc("SetWindowPos")
	procShowWindow           = user32.NewProc("ShowWindow")
	procInvalidateRect       = user32.NewProc("InvalidateRect")
	procBeginPaint           = user32.NewProc("BeginPaint")
	procEndPaint             = user32.NewProc("EndPaint")
	procFillRect             = user32.NewProc("FillRect")
	procGetDC                = user32.NewProc("GetDC")
	procReleaseDC            = user32.NewProc("ReleaseDC")
	procSetLayeredAttrs      = user32.NewProc("SetLayeredWindowAttributes")
	procSetWindowRgn         = user32.NewProc("SetWindowRgn")
	procMonitorFromWindow    = user32.NewProc("MonitorFromWindow")
	procMonitorFromPoint     = user32.NewProc("MonitorFromPoint")
	procGetMonitorInfo       = user32.NewProc("GetMonitorInfoW")
	procGetWindowRect        = user32.NewProc("GetWindowRect")
	procGetClassName         = user32.NewProc("GetClassNameW")
	procIsWindowVisible      = user32.NewProc("IsWindowVisible")
	procMessageBox           = user32.NewProc("MessageBoxW")
	procSetDpiContext        = user32.NewProc("SetProcessDpiAwarenessContext")
	procChangeMsgFilter      = user32.NewProc("ChangeWindowMessageFilterEx")
	procGetDpiForMonitor     = shcore.NewProc("GetDpiForMonitor")
	procCreateFont           = gdi32.NewProc("CreateFontW")
	procSelectObject         = gdi32.NewProc("SelectObject")
	procDeleteObject         = gdi32.NewProc("DeleteObject")
	procCreateSolidBrush     = gdi32.NewProc("CreateSolidBrush")
	procSetTextColor         = gdi32.NewProc("SetTextColor")
	procSetBkMode            = gdi32.NewProc("SetBkMode")
	procTextOut              = gdi32.NewProc("TextOutW")
	procGetTextExtent        = gdi32.NewProc("GetTextExtentPoint32W")
	procCreateCompatibleDC   = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBmp  = gdi32.NewProc("CreateCompatibleBitmap")
	procBitBlt               = gdi32.NewProc("BitBlt")
	procDeleteDC             = gdi32.NewProc("DeleteDC")
	procCreateRoundRectRgn   = gdi32.NewProc("CreateRoundRectRgn")
	procRoundRect            = gdi32.NewProc("RoundRect")
	procCreatePen            = gdi32.NewProc("CreatePen")
	procShellNotifyIcon      = shell32.NewProc("Shell_NotifyIconW")
	procShellExecuteEx       = shell32.NewProc("ShellExecuteExW")
)

const (
	wmDestroy       = 0x0002
	wmPaint         = 0x000F
	wmClose         = 0x0010
	wmEraseBkgnd    = 0x0014
	wmNCHitTest     = 0x0084
	wmContextMenu   = 0x007B
	wmTimer         = 0x0113
	wmHotkey        = 0x0312
	wmLButtonUp     = 0x0202
	wmLButtonDblClk = 0x0203
	wmDisplayChange = 0x007E
	wmDpiChanged    = 0x02E0
	wmApp           = 0x8000
	msgTray         = wmApp + 1
	msgOverlay      = wmApp + 2
	msgRefreshTray  = wmApp + 3
	msgNotify       = wmApp + 4
	msgQuit         = wmApp + 5
	ninSelect       = 0x0400
	ninKeySelect    = 0x0401
	htTransparent   = ^uintptr(0) // -1
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type msgT struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
	Private uint32
}

type rect struct{ Left, Top, Right, Bottom int32 }

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

type paintStruct struct {
	Hdc       uintptr
	Erase     int32
	Paint     rect
	Restore   int32
	IncUpdate int32
	Reserved  [32]byte
}

func messageBox(title, text string, isErr bool) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	flags := uintptr(0x40) // MB_ICONINFORMATION
	if isErr {
		flags = 0x10
	}
	procMessageBox.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), flags|0x00010000) // MB_SETFOREGROUND
}

// foreground devuelve la ventana de delante y su proceso.
func foreground() (hwnd uintptr, pid uint32) {
	hwnd, _, _ = procGetForegroundWindow.Call()
	if hwnd != 0 {
		procGetWindowThreadPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	}
	return
}

func monitorOf(hwnd uintptr) (rect, uintptr) {
	var h uintptr
	if hwnd != 0 {
		h, _, _ = procMonitorFromWindow.Call(hwnd, 1) // MONITOR_DEFAULTTOPRIMARY
	} else {
		h, _, _ = procMonitorFromPoint.Call(0, 1)
	}
	mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	procGetMonitorInfo.Call(h, uintptr(unsafe.Pointer(&mi)))
	return mi.Monitor, h
}

func monitorDPI(hmon uintptr) int {
	if procGetDpiForMonitor.Find() != nil {
		return 96
	}
	var x, y uint32
	if r, _, _ := procGetDpiForMonitor.Call(hmon, 0, uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y))); r != 0 || x == 0 {
		return 96
	}
	return int(x)
}

// coversMonitor: la ventana ocupa toda su pantalla (juego a pantalla completa o sin bordes).
func coversMonitor(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	var r rect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	m, _ := monitorOf(hwnd)
	return r.Left <= m.Left && r.Top <= m.Top && r.Right >= m.Right && r.Bottom >= m.Bottom
}

func className(hwnd uintptr) string {
	var b [128]uint16
	procGetClassName.Call(hwnd, uintptr(unsafe.Pointer(&b[0])), 128)
	return windows.UTF16ToString(b[:])
}

// processPath devuelve la ruta del ejecutable de un proceso.
func processPath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	var buf [windows.MAX_PATH * 2]uint16
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

func processAlive(pid uint32) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	r, _ := windows.WaitForSingleObject(h, 0)
	return r == uint32(windows.WAIT_TIMEOUT)
}

func exeName(path string) string { return strings.ToLower(filepath.Base(path)) }

func isElevated() bool { return windows.GetCurrentProcessToken().IsElevated() }

// shellExecuteInfo es SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     uintptr
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    uintptr
	dwHotKey     uint32
	hIcon        uintptr
	hProcess     windows.Handle
}
