package main

// Cierra la bóveda en cuanto se bloquea Windows (Win+L), se cierra la sesión o se
// suspende el equipo, no solo cuando pasa el tiempo sin usarla. Una ventana oculta
// (que nunca se enseña) recibe esos avisos de Windows.

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	wtsapi32             = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSRegister      = wtsapi32.NewProc("WTSRegisterSessionNotification")
	procRegisterClassEx  = user32.NewProc("RegisterClassExW")
	procCreateWindowEx   = user32.NewProc("CreateWindowExW")
	procDefWindowProc    = user32.NewProc("DefWindowProcW")
	procGetMessage       = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessage  = user32.NewProc("DispatchMessageW")
)

const (
	wmEndSession       = 0x0016
	wmPowerBroadcast   = 0x0218
	wmWTSSessionChange = 0x02B1
	pbtAPMSuspend      = 0x0004
	wtsConsoleDisc     = 0x2
	wtsRemoteDisc      = 0x4
	wtsLogoff          = 0x6
	wtsLock            = 0x7
	wsPopup            = 0x80000000
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

type winMsg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
	Private uint32
}

var sessionApp *App

var sessionProc = windows.NewCallback(func(hwnd, msg, wparam, lparam uintptr) uintptr {
	a := sessionApp
	switch uint32(msg) {
	case wmWTSSessionChange:
		switch wparam {
		case wtsLock, wtsLogoff, wtsConsoleDisc, wtsRemoteDisc:
			if a != nil {
				go a.lockVaultNow()
			}
		}
		return 0
	case wmPowerBroadcast:
		if wparam == pbtAPMSuspend && a != nil {
			a.lockVaultNow() // antes de dormir, no en otra goroutine
		}
		return 1
	case wmEndSession:
		if wparam != 0 && a != nil {
			a.lockVaultNow()
		}
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wparam, lparam)
	return r
})

// watchSession arranca la ventana oculta (en su propio hilo, con su bucle de mensajes).
func (a *App) watchSession() {
	sessionApp = a
	go func() {
		runtime.LockOSThread()
		cls, _ := windows.UTF16PtrFromString("TurboGAMESSession")
		var inst windows.Handle
		windows.GetModuleHandleEx(0, nil, &inst)
		wc := wndClassEx{WndProc: sessionProc, Instance: inst, ClassName: cls}
		wc.Size = uint32(unsafe.Sizeof(wc))
		procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
		title, _ := windows.UTF16PtrFromString("TurboGAMES")
		// Ventana normal pero oculta (no de solo mensajes): esas no reciben el aviso
		// de suspender.
		hwnd, _, _ := procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(title)), wsPopup, 0, 0, 0, 0, 0, 0, uintptr(inst), 0)
		if hwnd == 0 {
			return
		}
		procWTSRegister.Call(hwnd, 0) // NOTIFY_FOR_THIS_SESSION
		var m winMsg
		for {
			r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				return
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
		}
	}()
}

// lockVaultNow cierra la bóveda (si estaba abierta) y se lo dice a la ventana.
func (a *App) lockVaultNow() {
	if a.vault.Unlocked() {
		a.vault.Lock()
		a.emit("vault", a.VaultState())
	}
}
