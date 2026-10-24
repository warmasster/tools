package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procFindWindowEx             = user32.NewProc("FindWindowExW")
	procGetWindowThreadPID       = user32.NewProc("GetWindowThreadProcessId")
	procSetWindowDisplayAffinity = user32.NewProc("SetWindowDisplayAffinity")
	procShowWindow               = user32.NewProc("ShowWindow")
	procIsIconic                 = user32.NewProc("IsIconic")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procKeybdEvent               = user32.NewProc("keybd_event")
	procAttachThreadInput        = user32.NewProc("AttachThreadInput")
	procBringWindowToTop         = user32.NewProc("BringWindowToTop")
	procGetCurrentThreadID       = kernel32.NewProc("GetCurrentThreadId")
)

// findOwnWindow busca la ventana de esta clase que es de este proceso.
func findOwnWindow(class string) uintptr {
	cls, _ := windows.UTF16PtrFromString(class)
	pid := windows.GetCurrentProcessId()
	var prev uintptr
	for i := 0; i < 64; i++ {
		h, _, _ := procFindWindowEx.Call(0, prev, uintptr(unsafe.Pointer(cls)), 0)
		if h == 0 {
			return 0
		}
		var p uint32
		procGetWindowThreadPID.Call(h, uintptr(unsafe.Pointer(&p)))
		if p == pid {
			return h
		}
		prev = h
	}
	return 0
}

// bringToFront pone la ventana delante aunque otro programa tenga el foco.
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
