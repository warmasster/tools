package main

// Icono de la bandeja (junto a la flechita de la barra de tareas): un clic pone
// o quita el overlay, doble clic abre la ventana y el botón derecho enseña el
// menú. También atiende el atajo Ctrl+Alt+O. Comparte hilo con el overlay.

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

//go:embed assets/tray-on.ico
var icoOn []byte

//go:embed assets/tray-off.ico
var icoOff []byte

const (
	nimAdd             = 0
	nimModify          = 1
	nimDelete          = 2
	nimSetVersion      = 4
	nifMessage         = 0x01
	nifIcon            = 0x02
	nifTip             = 0x04
	nifInfo            = 0x10
	nifShowTip         = 0x80
	notifyIconVersion4 = 4
	mfString           = 0x0000
	mfGrayed           = 0x0001
	mfChecked          = 0x0008
	mfPopup            = 0x0010
	mfSeparator        = 0x0800
	tpmReturnCmd       = 0x0100
	tpmRightButton     = 0x0002
	tpmNoNotify        = 0x0080
	hotkeyID           = 1
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

// ui es el hilo de las ventanas del agente: la oculta de la bandeja y el overlay.
type ui struct {
	a          *Agent
	hwnd       uintptr
	ov         *overlay
	iconOn     uintptr
	iconOff    uintptr
	taskbarMsg uint32
	hotkey     bool
	done       chan struct{}
	mu         sync.Mutex
	notes      [][2]string
	lastClick  time.Time
}

var trayCallback = windows.NewCallback(trayWndProc)
var uiInstance *ui

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
			d = -d*4 + 1
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

func startUI(a *Agent) *ui {
	u := &ui{a: a, done: make(chan struct{})}
	ready := make(chan struct{})
	go u.run(ready)
	<-ready
	return u
}

func (u *ui) run(ready chan struct{}) {
	runtime.LockOSThread()
	defer close(u.done)
	uiInstance = u
	size, _, _ := procGetSystemMetrics.Call(49) // SM_CXSMICON
	if size == 0 {
		size = 16
	}
	u.iconOn = iconFromICO(icoOn, int(size))
	u.iconOff = iconFromICO(icoOff, int(size))
	tb, _ := windows.UTF16PtrFromString("TaskbarCreated")
	m, _, _ := procRegisterWindowMsg.Call(uintptr(unsafe.Pointer(tb)))
	u.taskbarMsg = uint32(m)

	var inst windows.Handle
	windows.GetModuleHandleEx(0, nil, &inst)
	cls, _ := windows.UTF16PtrFromString(agentWindowClass)
	wc := wndClassEx{WndProc: trayCallback, Instance: inst, ClassName: cls}
	wc.Size = uint32(unsafe.Sizeof(wc))
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	title, _ := windows.UTF16PtrFromString("TurboMON")
	hwnd, _, _ := procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(title)), 0, 0, 0, 0, 0, 0, 0, uintptr(inst), 0)
	u.hwnd = hwnd
	if hwnd == 0 {
		close(ready)
		return
	}
	// Siendo administrador, el Explorador (que no lo es) no podría avisar al icono sin esto.
	for _, m := range []uint32{msgTray, u.taskbarMsg} {
		procChangeMsgFilter.Call(hwnd, uintptr(m), 1, 0) // MSGFLT_ALLOW
	}
	u.ov = newOverlay(inst)
	u.addIcon()
	u.applyHotkey()
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
	w, _ := windows.UTF16FromString(s)
	if len(w) > len(dst) {
		w = append(w[:len(dst)-1], 0)
	}
	copy(dst, w)
}

func (u *ui) baseData() notifyIconData {
	var d notifyIconData
	d.Size = uint32(unsafe.Sizeof(d))
	d.Wnd = u.hwnd
	d.ID = 1
	return d
}

func (u *ui) iconAndTip() (uintptr, string) {
	s := getSettings()
	if s.Overlay.On {
		tip := "TurboMON · overlay puesto"
		if s.Overlay.Hotkey {
			tip += " (Ctrl+Alt+O)"
		}
		return u.iconOn, tip + "\nClic: quitarlo · Doble clic: abrir"
	}
	return u.iconOff, "TurboMON · overlay quitado\nClic: ponerlo · Doble clic: abrir"
}

func (u *ui) addIcon() {
	d := u.baseData()
	d.Flags = nifMessage | nifIcon | nifTip | nifShowTip
	d.CallbackMessage = msgTray
	icon, tip := u.iconAndTip()
	d.Icon = icon
	setText(d.Tip[:], tip)
	procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&d)))
	d.Version = notifyIconVersion4
	procShellNotifyIcon.Call(nimSetVersion, uintptr(unsafe.Pointer(&d)))
}

func (u *ui) updateIcon() {
	d := u.baseData()
	d.Flags = nifIcon | nifTip | nifShowTip
	icon, tip := u.iconAndTip()
	d.Icon = icon
	setText(d.Tip[:], tip)
	procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&d)))
}

func (u *ui) applyHotkey() {
	want := getSettings().Overlay.Hotkey
	if want == u.hotkey {
		return
	}
	if want {
		r, _, _ := procRegisterHotKey.Call(u.hwnd, hotkeyID, 0x0001|0x0002|0x4000, 'O') // ALT CONTROL NOREPEAT
		u.hotkey = r != 0
		if r == 0 {
			logf("no se pudo registrar Ctrl+Alt+O (lo usa otro programa)")
		}
	} else {
		procUnregisterHotKey.Call(u.hwnd, hotkeyID)
		u.hotkey = false
	}
}

// refresh: icono, atajo y overlay al día con los ajustes (desde cualquier goroutine).
func (u *ui) refresh() {
	if u != nil && u.hwnd != 0 {
		procPostMessage.Call(u.hwnd, msgRefreshTray, 0, 0)
	}
}

func (u *ui) notify(title, text string) {
	if u == nil || u.hwnd == 0 {
		return
	}
	u.mu.Lock()
	u.notes = append(u.notes, [2]string{title, text})
	u.mu.Unlock()
	procPostMessage.Call(u.hwnd, msgNotify, 0, 0)
}

func (u *ui) showNotes() {
	u.mu.Lock()
	notes := u.notes
	u.notes = nil
	u.mu.Unlock()
	for _, n := range notes {
		d := u.baseData()
		d.Flags = nifInfo
		setText(d.InfoTitle[:], n[0])
		setText(d.Info[:], n[1])
		d.InfoFlags = 0x01 | 0x20 // NIIF_INFO | NIIF_LARGE_ICON
		procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&d)))
	}
}

func (u *ui) close() {
	if u == nil || u.hwnd == 0 {
		return
	}
	procPostMessage.Call(u.hwnd, msgQuit, 0, 0)
	select {
	case <-u.done:
	case <-time.After(3 * time.Second):
	}
}

const (
	cmdOverlay = iota + 100
	cmdFPSOnly
	cmdGameOnly
	cmdOpen
	cmdPurge
	cmdEnableFPS
	cmdQuit
	cmdCornerTI
	cmdCornerTD
	cmdCornerBI
	cmdCornerBD
)

var cornerCmds = map[uintptr]string{cmdCornerTI: "ti", cmdCornerTD: "td", cmdCornerBI: "bi", cmdCornerBD: "bd"}

func (u *ui) menu() {
	h, _, _ := procCreatePopupMenu.Call()
	if h == 0 {
		return
	}
	defer procDestroyMenu.Call(h)
	add := func(menu, flags, id uintptr, text string) {
		p, _ := windows.UTF16PtrFromString(text)
		procAppendMenu.Call(menu, flags, id, uintptr(unsafe.Pointer(p)))
	}
	check := func(b bool) uintptr {
		if b {
			return mfChecked
		}
		return 0
	}
	s := getSettings()
	add(h, mfString|mfGrayed, 1, u.a.menuHeader())
	add(h, mfSeparator, 0, "")
	hk := ""
	if s.Overlay.Hotkey {
		hk = "\tCtrl+Alt+O"
	}
	add(h, mfString|check(s.Overlay.On), cmdOverlay, "Overlay en pantalla"+hk)
	add(h, mfString|check(s.Overlay.FPSOnly), cmdFPSOnly, "Solo el contador de FPS")
	add(h, mfString|check(s.Overlay.GameOnly), cmdGameOnly, "Solo con un juego delante")
	sub, _, _ := procCreatePopupMenu.Call()
	for _, it := range []struct {
		id    uintptr
		label string
	}{{cmdCornerTI, "Arriba a la izquierda"}, {cmdCornerTD, "Arriba a la derecha"}, {cmdCornerBI, "Abajo a la izquierda"}, {cmdCornerBD, "Abajo a la derecha"}} {
		add(sub, mfString|check(cornerCmds[it.id] == s.Overlay.Corner), it.id, it.label)
	}
	add(h, mfPopup, sub, "Posición del overlay")
	add(h, mfSeparator, 0, "")
	add(h, mfString, cmdOpen, "Abrir TurboMON")
	if u.a.elevated {
		add(h, mfString, cmdPurge, "Liberar la memoria en espera")
	} else {
		add(h, mfString, cmdEnableFPS, "Activar la medida de FPS…")
	}
	add(h, mfSeparator, 0, "")
	add(h, mfString, cmdQuit, "Salir de TurboMON")
	var pt struct{ X, Y int32 }
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(u.hwnd)
	cmd, _, _ := procTrackPopupMenu.Call(h, tpmReturnCmd|tpmRightButton|tpmNoNotify, uintptr(pt.X), uintptr(pt.Y), 0, u.hwnd, 0)
	procPostMessage.Call(u.hwnd, 0, 0, 0)
	switch cmd {
	case cmdOverlay:
		u.a.toggleOverlay()
	case cmdFPSOnly:
		u.a.changeSettings(func(s *Settings) { s.Overlay.FPSOnly = !s.Overlay.FPSOnly; s.Overlay.On = true })
	case cmdGameOnly:
		u.a.changeSettings(func(s *Settings) { s.Overlay.GameOnly = !s.Overlay.GameOnly })
	case cmdCornerTI, cmdCornerTD, cmdCornerBI, cmdCornerBD:
		c := cornerCmds[cmd]
		u.a.changeSettings(func(s *Settings) { s.Overlay.Corner = c; s.Overlay.On = true })
	case cmdOpen:
		go u.a.openWindow()
	case cmdPurge:
		go func() {
			freed, err := u.a.purgeStandby()
			if err != nil {
				u.notify("No se pudo liberar", err.Error())
				return
			}
			u.notify("Memoria en espera liberada", fmt.Sprintf("%s que Windows tenía guardados por si acaso ahora están libres.", humanBytes(freed)))
		}()
	case cmdEnableFPS:
		go u.a.enableFPS()
	case cmdQuit:
		go u.a.shutdown()
	}
}

func trayWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	u := uiInstance
	if u == nil {
		r, _, _ := procDefWindowProc.Call(hwnd, msg, wparam, lparam)
		return r
	}
	switch uint32(msg) {
	case msgTray:
		switch uint32(lparam & 0xffff) {
		case wmContextMenu:
			u.menu()
		case ninSelect, ninKeySelect:
			// El primer clic de un doble clic también llega aquí: se aplaza un
			// poco para no poner y quitar el overlay al abrir la ventana.
			u.lastClick = time.Now()
			procSetTimer.Call(hwnd, 2, 260, 0)
		case wmLButtonDblClk:
			u.lastClick = time.Time{}
			go u.a.openWindow()
		}
		return 0
	case wmTimer:
		if wparam == 2 {
			procKillTimer.Call(hwnd, 2)
			if !u.lastClick.IsZero() {
				u.lastClick = time.Time{}
				u.a.toggleOverlay()
			}
		}
		return 0
	case wmHotkey:
		if wparam == hotkeyID {
			u.a.toggleOverlay()
		}
		return 0
	case msgRefreshTray:
		u.updateIcon()
		u.applyHotkey()
		return 0
	case msgNotify:
		u.showNotes()
		return 0
	case msgQuit:
		d := u.baseData()
		procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&d)))
		if u.hotkey {
			procUnregisterHotKey.Call(hwnd, hotkeyID)
		}
		if u.ov != nil && u.ov.hwnd != 0 {
			procDestroyWindow.Call(u.ov.hwnd)
		}
		procDestroyWindow.Call(hwnd)
		procPostQuitMessage.Call(0)
		return 0
	case wmClose:
		return 0 // solo se cierra con «Salir»
	}
	if u.taskbarMsg != 0 && uint32(msg) == u.taskbarMsg {
		u.addIcon() // el Explorador se ha reiniciado
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wparam, lparam)
	return r
}

var procKillTimer = user32.NewProc("KillTimer")
