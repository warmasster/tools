package main

// Icono de la bandeja (junto a la flechita de la barra de tareas) y una ventana
// oculta que se entera de cuándo se bloquea Windows, se suspende el equipo o se
// cierra la sesión, para cerrar la bóveda en ese momento.

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"turbovault/internal/gen"
)

//go:embed assets/tray-open.ico
var icoOpen []byte

//go:embed assets/tray-locked.ico
var icoLocked []byte

var (
	shell32                   = windows.NewLazySystemDLL("shell32.dll")
	wtsapi32                  = windows.NewLazySystemDLL("wtsapi32.dll")
	procShellNotifyIcon       = shell32.NewProc("Shell_NotifyIconW")
	procRegisterClassEx       = user32.NewProc("RegisterClassExW")
	procCreateWindowEx        = user32.NewProc("CreateWindowExW")
	procDefWindowProc         = user32.NewProc("DefWindowProcW")
	procGetMessage            = user32.NewProc("GetMessageW")
	procTranslateMessage      = user32.NewProc("TranslateMessage")
	procDispatchMessage       = user32.NewProc("DispatchMessageW")
	procPostMessage           = user32.NewProc("PostMessageW")
	procPostQuitMessage       = user32.NewProc("PostQuitMessage")
	procDestroyWindow         = user32.NewProc("DestroyWindow")
	procRegisterWindowMessage = user32.NewProc("RegisterWindowMessageW")
	procCreatePopupMenu       = user32.NewProc("CreatePopupMenu")
	procAppendMenu            = user32.NewProc("AppendMenuW")
	procTrackPopupMenu        = user32.NewProc("TrackPopupMenu")
	procDestroyMenu           = user32.NewProc("DestroyMenu")
	procSetForegroundWindow   = user32.NewProc("SetForegroundWindow")
	procGetCursorPos          = user32.NewProc("GetCursorPos")
	procCreateIconFromRes     = user32.NewProc("CreateIconFromResourceEx")
	procDestroyIcon           = user32.NewProc("DestroyIcon")
	procGetSystemMetrics      = user32.NewProc("GetSystemMetrics")
	procSetTimer              = user32.NewProc("SetTimer")
	procWTSRegister           = wtsapi32.NewProc("WTSRegisterSessionNotification")
	procWTSUnregister         = wtsapi32.NewProc("WTSUnRegisterSessionNotification")
)

const (
	wmDestroy          = 0x0002
	wmClose            = 0x0010
	wmQueryEndSession  = 0x0011
	wmEndSession       = 0x0016
	wmTimer            = 0x0113
	wmPowerBroadcast   = 0x0218
	wmWTSSessionChange = 0x02B1
	wmContextMenu      = 0x007B
	wmLButtonUp        = 0x0202
	wmLButtonDblClk    = 0x0203
	wmApp              = 0x8000
	msgTray            = wmApp + 1
	msgRefresh         = wmApp + 2
	msgNotify          = wmApp + 3
	ninSelect          = 0x0400
	ninKeySelect       = 0x0401
	ninBalloonClick    = 0x0405
	pbtAPMSuspend      = 0x0004
	wtsConsoleDisc     = 0x2
	wtsRemoteDisc      = 0x4
	wtsLogoff          = 0x6
	wtsLock            = 0x7
	nimAdd             = 0
	nimModify          = 1
	nimDelete          = 2
	nimSetVersion      = 4
	nifMessage         = 0x01
	nifIcon            = 0x02
	nifTip             = 0x04
	nifInfo            = 0x10
	nifShowTip         = 0x80
	niifInfo           = 0x01
	niifWarning        = 0x02
	notifyIconVersion4 = 4
	mfString           = 0x0000
	mfGrayed           = 0x0001
	mfSeparator        = 0x0800
	tpmReturnCmd       = 0x0100
	tpmRightButton     = 0x0002
	tpmNoNotify        = 0x0080
	smCxSmIcon         = 49
)

type notifyIconData struct {
	Size            uint32
	Wnd             uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GuidItem        windows.GUID
	BalloonIcon     uintptr
}

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

type tray struct {
	a          *Agent
	hwnd       uintptr
	iconOpen   uintptr
	iconLocked uintptr
	taskbarMsg uint32
	done       chan struct{}
	mu         sync.Mutex
	notes      []string // título\x00texto
	added      bool
	closing    atomic.Bool // solo el agente puede cerrar esta ventana
}

// iconFromICO elige del .ico la imagen más cercana al tamaño pedido.
func iconFromICO(ico []byte, size int) uintptr {
	if len(ico) < 6 {
		return 0
	}
	n := int(binary.LittleEndian.Uint16(ico[4:6]))
	best, bestDiff := -1, 1<<30
	for i := 0; i < n; i++ {
		e := ico[6+16*i:]
		w := int(e[0])
		if w == 0 {
			w = 256
		}
		d := w - size
		if d < 0 {
			d = -d*4 + 1 // mejor uno más grande que uno más pequeño
		}
		if d < bestDiff {
			best, bestDiff = i, d
		}
	}
	if best < 0 {
		return 0
	}
	e := ico[6+16*best:]
	ln := binary.LittleEndian.Uint32(e[8:12])
	off := binary.LittleEndian.Uint32(e[12:16])
	if int(off+ln) > len(ico) {
		return 0
	}
	data := ico[off : off+ln]
	h, _, _ := procCreateIconFromRes.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(ln), 1, 0x00030000, uintptr(size), uintptr(size), 0)
	return h
}

var trayCallback = windows.NewCallback(trayWndProc)
var trayInstance *tray

func startTray(a *Agent) *tray {
	t := &tray{a: a, done: make(chan struct{})}
	ready := make(chan struct{})
	go t.run(ready)
	<-ready
	return t
}

func (t *tray) run(ready chan struct{}) {
	runtime.LockOSThread()
	defer close(t.done)
	trayInstance = t
	size, _, _ := procGetSystemMetrics.Call(smCxSmIcon)
	if size == 0 {
		size = 16
	}
	t.iconOpen = iconFromICO(icoOpen, int(size))
	t.iconLocked = iconFromICO(icoLocked, int(size))
	tbName, _ := windows.UTF16PtrFromString("TaskbarCreated")
	m, _, _ := procRegisterWindowMessage.Call(uintptr(unsafe.Pointer(tbName)))
	t.taskbarMsg = uint32(m)

	cls, _ := windows.UTF16PtrFromString("TurboVAULTAgent")
	var inst windows.Handle
	windows.GetModuleHandleEx(0, nil, &inst)
	wc := wndClassEx{WndProc: trayCallback, Instance: inst, ClassName: cls}
	wc.Size = uint32(unsafe.Sizeof(wc))
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	title, _ := windows.UTF16PtrFromString("TurboVAULT")
	hwnd, _, _ := procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(title)), 0, 0, 0, 0, 0, 0, 0, uintptr(inst), 0)
	t.hwnd = hwnd
	if hwnd == 0 {
		close(ready)
		return
	}
	procWTSRegister.Call(hwnd, 0) // NOTIFY_FOR_THIS_SESSION
	t.addIcon()
	procSetTimer.Call(hwnd, 1, 15000, 0)
	close(ready)

	var msg msgT
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func setText(dst []uint16, s string) {
	u, _ := windows.UTF16FromString(s)
	if len(u) > len(dst) {
		u = u[:len(dst)-1]
		u = append(u, 0)
	}
	copy(dst, u)
}

func (t *tray) baseData() notifyIconData {
	var d notifyIconData
	d.Size = uint32(unsafe.Sizeof(d))
	d.Wnd = t.hwnd
	d.ID = 1
	return d
}

func (t *tray) addIcon() {
	d := t.baseData()
	d.Flags = nifMessage | nifIcon | nifTip | nifShowTip
	d.CallbackMessage = msgTray
	d.Icon, _ = t.iconAndTip(&d)
	procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&d)))
	d.Version = notifyIconVersion4
	procShellNotifyIcon.Call(nimSetVersion, uintptr(unsafe.Pointer(&d)))
	t.added = true
}

func (t *tray) iconAndTip(d *notifyIconData) (uintptr, string) {
	tip := "TurboVAULT · cerrada"
	icon := t.iconLocked
	if t.a.v.Unlocked() {
		icon = t.iconOpen
		t.a.mu.Lock()
		exp := t.a.expires
		t.a.mu.Unlock()
		left := int(time.Until(exp).Minutes() + 0.99)
		if left < 1 {
			left = 1
		}
		if left >= 60 {
			tip = fmt.Sprintf("TurboVAULT · abierta (se cierra en %d h %d min)", left/60, left%60)
		} else {
			tip = fmt.Sprintf("TurboVAULT · abierta (se cierra en %d min)", left)
		}
	} else if !t.a.v.Exists() {
		tip = "TurboVAULT · sin bóveda todavía"
	}
	setText(d.Tip[:], tip)
	return icon, tip
}

func (t *tray) update() {
	d := t.baseData()
	d.Flags = nifIcon | nifTip | nifShowTip
	d.Icon, _ = t.iconAndTip(&d)
	procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&d)))
}

func (t *tray) showNotes() {
	t.mu.Lock()
	notes := t.notes
	t.notes = nil
	t.mu.Unlock()
	for _, n := range notes {
		var title, text string
		for i := 0; i < len(n); i++ {
			if n[i] == 0 {
				title, text = n[:i], n[i+1:]
				break
			}
		}
		d := t.baseData()
		d.Flags = nifInfo
		setText(d.InfoTitle[:], title)
		setText(d.Info[:], text)
		d.InfoFlags = niifInfo
		procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&d)))
	}
}

// refresh actualiza icono y texto (se puede llamar desde cualquier goroutine).
func (t *tray) refresh() {
	if t != nil && t.hwnd != 0 {
		procPostMessage.Call(t.hwnd, msgRefresh, 0, 0)
	}
}

// notify enseña un aviso junto al icono.
func (t *tray) notify(title, text string) {
	if t == nil || t.hwnd == 0 {
		return
	}
	t.mu.Lock()
	t.notes = append(t.notes, title+"\x00"+text)
	t.mu.Unlock()
	procPostMessage.Call(t.hwnd, msgNotify, 0, 0)
}

func (t *tray) close() {
	if t == nil || t.hwnd == 0 {
		return
	}
	t.closing.Store(true)
	procPostMessage.Call(t.hwnd, wmClose, 0, 0)
	select {
	case <-t.done:
	case <-time.After(3 * time.Second):
	}
}

const (
	cmdOpen = iota + 100
	cmdLockToggle
	cmdGenerate
	cmdQuit
)

func (t *tray) menu() {
	h, _, _ := procCreatePopupMenu.Call()
	if h == 0 {
		return
	}
	defer procDestroyMenu.Call(h)
	add := func(flags uintptr, id uintptr, text string) {
		p, _ := windows.UTF16PtrFromString(text)
		procAppendMenu.Call(h, flags, id, uintptr(unsafe.Pointer(p)))
	}
	open := t.a.v.Unlocked()
	head := "TurboVAULT · cerrada"
	if open {
		t.a.mu.Lock()
		exp := t.a.expires
		t.a.mu.Unlock()
		head = "TurboVAULT · abierta hasta las " + exp.Format("15:04")
	}
	add(mfString|mfGrayed, 1, head)
	add(mfSeparator, 0, "")
	add(mfString, cmdOpen, "Abrir TurboVAULT")
	if open {
		add(mfString, cmdLockToggle, "Cerrar la bóveda ahora")
	} else if t.a.v.Exists() {
		add(mfString, cmdLockToggle, "Abrir la bóveda…")
	}
	add(mfString, cmdGenerate, "Copiar una contraseña nueva")
	add(mfSeparator, 0, "")
	add(mfString, cmdQuit, "Salir (la extensión dejará de funcionar)")
	var pt struct{ X, Y int32 }
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(t.hwnd)
	cmd, _, _ := procTrackPopupMenu.Call(h, tpmReturnCmd|tpmRightButton|tpmNoNotify, uintptr(pt.X), uintptr(pt.Y), 0, t.hwnd, 0)
	procPostMessage.Call(t.hwnd, 0, 0, 0)
	switch cmd {
	case cmdOpen:
		go t.a.openWindow("")
	case cmdLockToggle:
		if open {
			go t.a.lockNow("manual")
		} else {
			go t.a.openWindow("unlock")
		}
	case cmdGenerate:
		go func() {
			r, err := gen.Generate(getSettings().Gen)
			if err != nil {
				return
			}
			if secs, err := t.a.copySecret(r.Password); err == nil {
				t.notify("Contraseña nueva copiada", fmt.Sprintf("Pégala donde la necesites. Se borra del portapapeles en %d s.", secs))
			}
		}()
	case cmdQuit:
		go t.a.shutdown()
	}
}

func trayWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	t := trayInstance
	if t == nil {
		r, _, _ := procDefWindowProc.Call(hwnd, msg, wparam, lparam)
		return r
	}
	switch uint32(msg) {
	case msgTray:
		switch uint32(lparam & 0xffff) {
		case wmContextMenu:
			t.menu()
		case ninSelect, ninKeySelect, wmLButtonDblClk, ninBalloonClick:
			go t.a.openWindow("")
		}
		return 0
	case msgRefresh, wmTimer:
		if t.a.v.Unlocked() {
			t.a.expired()
		}
		t.update()
		return 0
	case msgNotify:
		t.showNotes()
		return 0
	case wmWTSSessionChange:
		switch wparam {
		case wtsLock, wtsLogoff, wtsConsoleDisc, wtsRemoteDisc:
			go t.a.lockNow("windows")
		}
		return 0
	case wmPowerBroadcast:
		if wparam == pbtAPMSuspend {
			t.a.lockNow("suspender") // antes de dormir, no en otra goroutine
		}
		return 1
	case wmQueryEndSession:
		return 1
	case wmEndSession:
		if wparam != 0 {
			t.a.lockNow("windows")
		}
		return 0
	case wmClose:
		// Otro programa puede mandar WM_CLOSE a cualquier ventana: si se
		// hiciera caso, la bóveda dejaría de cerrarse al bloquear Windows.
		if t.closing.Load() {
			procDestroyWindow.Call(hwnd)
		}
		return 0
	case wmDestroy:
		if !t.closing.Load() {
			return 0
		}
		d := t.baseData()
		procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&d)))
		procWTSUnregister.Call(hwnd)
		procDestroyIcon.Call(t.iconOpen)
		procDestroyIcon.Call(t.iconLocked)
		procPostQuitMessage.Call(0)
		return 0
	}
	if t.taskbarMsg != 0 && uint32(msg) == t.taskbarMsg {
		t.addIcon() // el Explorador se ha reiniciado
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wparam, lparam)
	return r
}
