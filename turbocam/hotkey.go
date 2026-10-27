package main

// Atajos de teclado globales (funcionan aunque TurboCAM no tenga el foco). Se
// registran en un hilo propio con su cola de mensajes de Windows.

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procRegisterHotKey     = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey   = user32.NewProc("UnregisterHotKey")
	procGetMessageW        = user32.NewProc("GetMessageW")
	procPostThreadMessageW = user32.NewProc("PostThreadMessageW")
)

const (
	wmHotkey    = 0x0312
	wmQuit      = 0x0012
	wmReload    = 0x8001 // WM_APP + 1
	modAlt      = 0x1
	modCtrl     = 0x2
	modShift    = 0x4
	modWin      = 0x8
	modNoRepeat = 0x4000
)

var hotkeyActions = []string{"photo", "freeze", "cover"}

type winMsg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

// parseHotkey entiende «Ctrl+Alt+P», «Ctrl+Shift+F9», «Win+Alt+1»…
func parseHotkey(s string) (mods, vk uint32, err error) {
	parts := strings.Split(strings.ReplaceAll(s, " ", ""), "+")
	if len(parts) < 2 {
		return 0, 0, errors.New("el atajo necesita una tecla y al menos Ctrl, Alt, Mayús o Win")
	}
	for _, p := range parts[:len(parts)-1] {
		switch strings.ToLower(p) {
		case "ctrl", "control":
			mods |= modCtrl
		case "alt":
			mods |= modAlt
		case "shift", "mayús", "mayus":
			mods |= modShift
		case "win":
			mods |= modWin
		default:
			return 0, 0, fmt.Errorf("modificador desconocido: %s", p)
		}
	}
	key := strings.ToUpper(parts[len(parts)-1])
	switch {
	case len(key) == 1 && (key[0] >= 'A' && key[0] <= 'Z' || key[0] >= '0' && key[0] <= '9'):
		vk = uint32(key[0])
	case len(key) >= 2 && len(key) <= 3 && key[0] == 'F':
		var n int
		if _, e := fmt.Sscanf(key[1:], "%d", &n); e == nil && n >= 1 && n <= 24 {
			vk = uint32(0x70 + n - 1)
		}
	case key == "ESPACIO" || key == "SPACE":
		vk = 0x20
	case key == "PAUSA" || key == "PAUSE":
		vk = 0x13
	}
	if vk == 0 {
		return 0, 0, fmt.Errorf("tecla no admitida: %s", parts[len(parts)-1])
	}
	if mods == 0 {
		return 0, 0, errors.New("el atajo necesita Ctrl, Alt, Mayús o Win")
	}
	return mods, vk, nil
}

type hotkeyManager struct {
	tid     uint32
	onPress func(action string)
	mu      sync.Mutex
	errs    map[string]string
	ready   chan struct{}
	applied chan struct{} // avisa cuando se han vuelto a registrar
}

func startHotkeys(onPress func(action string)) *hotkeyManager {
	h := &hotkeyManager{onPress: onPress, errs: map[string]string{}, ready: make(chan struct{}), applied: make(chan struct{}, 1)}
	go h.run()
	<-h.ready
	return h
}

func (h *hotkeyManager) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	h.tid = windows.GetCurrentThreadId()
	h.register()
	close(h.ready)
	var m winMsg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		switch m.Message {
		case wmHotkey:
			if i := int(m.WParam) - 1; i >= 0 && i < len(hotkeyActions) {
				go h.onPress(hotkeyActions[i])
			}
		case wmReload:
			h.register()
		}
	}
	h.unregister()
}

func (h *hotkeyManager) unregister() {
	for i := range hotkeyActions {
		procUnregisterHotKey.Call(0, uintptr(i+1))
	}
}

// register (en el hilo de los atajos) pone los de los ajustes.
func (h *hotkeyManager) register() {
	h.unregister()
	hk := getSettings().Hotkeys
	combos := map[string]string{"photo": hk.Photo, "freeze": hk.Freeze, "cover": hk.Cover}
	errs := map[string]string{}
	for i, a := range hotkeyActions {
		c := combos[a]
		if c == "" {
			continue
		}
		mods, vk, err := parseHotkey(c)
		if err != nil {
			errs[a] = err.Error()
			continue
		}
		if r, _, _ := procRegisterHotKey.Call(0, uintptr(i+1), uintptr(mods|modNoRepeat), uintptr(vk)); r == 0 {
			errs[a] = c + " ya lo usa otro programa"
		}
	}
	h.mu.Lock()
	h.errs = errs
	h.mu.Unlock()
	select {
	case h.applied <- struct{}{}:
	default:
	}
}

// Reload vuelve a registrar los atajos de los ajustes y espera a que esté hecho.
func (h *hotkeyManager) Reload() {
	select {
	case <-h.applied:
	default:
	}
	procPostThreadMessageW.Call(uintptr(h.tid), wmReload, 0, 0)
	select {
	case <-h.applied:
	case <-time.After(time.Second):
	}
}
func (h *hotkeyManager) Stop() { procPostThreadMessageW.Call(uintptr(h.tid), wmQuit, 0, 0) }

func (h *hotkeyManager) Errors() map[string]string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make(map[string]string, len(h.errs))
	for k, v := range h.errs {
		out[k] = v
	}
	return out
}
