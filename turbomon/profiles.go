package main

// Ajustes por juego (perfiles): ratón encerrado en la pantalla del juego,
// forzar «ventana sin bordes» y lo que haya ganado en el laboratorio.

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"
)

// GameProfile son los ajustes de un juego.
type GameProfile struct {
	Name       string `json:"name"`
	LockCursor bool   `json:"lockCursor"`
	Borderless bool   `json:"borderless"`
	Cores      string `json:"cores,omitempty"`    // núcleos para el juego (del laboratorio): all, pcores, nosmt, nocore0, ccd
	Priority   string `json:"priority,omitempty"` // normal, above, high
	Prefetch   bool   `json:"prefetch"`
	GPUHigh    bool   `json:"gpuHigh,omitempty"` // prioridad alta en la gráfica (del laboratorio)
}

var (
	profMu   sync.Mutex
	profiles map[string]GameProfile // por exe en minúsculas
)

func profilesPath() string { return filepath.Join(dataDir(), "juegos.json") }

func loadProfiles() map[string]GameProfile {
	profMu.Lock()
	defer profMu.Unlock()
	if profiles == nil {
		profiles = map[string]GameProfile{}
		if b, err := os.ReadFile(profilesPath()); err == nil {
			json.Unmarshal(b, &profiles)
		}
	}
	out := make(map[string]GameProfile, len(profiles))
	for k, v := range profiles {
		out[k] = v
	}
	return out
}

func getProfile(exe string) (GameProfile, bool) {
	p, ok := loadProfiles()[strings.ToLower(exe)]
	return p, ok
}

func setProfile(exe string, p GameProfile) {
	loadProfiles()
	profMu.Lock()
	profiles[strings.ToLower(exe)] = p
	b, _ := json.MarshalIndent(profiles, "", "  ")
	profMu.Unlock()
	os.MkdirAll(dataDir(), 0o755)
	os.WriteFile(profilesPath(), b, 0o644)
}

func (a *Agent) hProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var b struct {
			Exe     string      `json:"exe"`
			Profile GameProfile `json:"profile"`
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil || b.Exe == "" {
			http.Error(w, "perfil", 400)
			return
		}
		setProfile(b.Exe, b.Profile)
		a.kick()
	}
	writeJSON(w, loadProfiles())
}

// ---------- ratón y ventana ----------

var (
	procClipCursor     = user32.NewProc("ClipCursor")
	procGetClientRect  = user32.NewProc("GetClientRect")
	procClientToScreen = user32.NewProc("ClientToScreen")
	procGetWindowLong  = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLong  = user32.NewProc("SetWindowLongPtrW")
	procIsIconicW      = user32.NewProc("IsIconic")
)

type windowCtl struct {
	clipped    bool
	borderless map[uintptr]bool
}

// applyWindowRules se llama cada segundo con lo que hay delante.
func (a *Agent) applyWindowRules(hwnd uintptr, pid uint32) {
	w := &a.win
	if w.borderless == nil {
		w.borderless = map[uintptr]bool{}
	}
	a.st.mu.Lock()
	cur := a.st.cur
	gamePid, exe := uint32(0), ""
	if cur != nil {
		gamePid, exe = cur.pid, cur.exe
	}
	a.st.mu.Unlock()
	if gamePid == 0 || pid != gamePid || hwnd == 0 {
		if w.clipped {
			procClipCursor.Call(0)
			w.clipped = false
		}
		return
	}
	p, ok := getProfile(exe)
	if !ok {
		return
	}
	if ic, _, _ := procIsIconicW.Call(hwnd); ic != 0 {
		return
	}
	if p.Borderless && !w.borderless[hwnd] {
		if makeBorderless(hwnd) {
			logf("ventana sin bordes para %s", exe)
		}
		w.borderless[hwnd] = true
	}
	if p.LockCursor {
		var cr rect
		procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&cr)))
		var tl, br struct{ X, Y int32 }
		tl.X, tl.Y = cr.Left, cr.Top
		br.X, br.Y = cr.Right, cr.Bottom
		procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&tl)))
		procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&br)))
		if br.X-tl.X > 200 && br.Y-tl.Y > 200 {
			r := rect{tl.X, tl.Y, br.X, br.Y}
			procClipCursor.Call(uintptr(unsafe.Pointer(&r)))
			w.clipped = true
		}
	} else if w.clipped {
		procClipCursor.Call(0)
		w.clipped = false
	}
}

// makeBorderless quita el marco a la ventana y la pone a pantalla completa en su pantalla.
func makeBorderless(hwnd uintptr) bool {
	const (
		gwlStyle   = ^uintptr(15) // -16
		gwlExStyle = ^uintptr(19) // -20
		wsCaption  = 0x00C00000
		wsFrame    = 0x00040000
		wsMin      = 0x00020000
		wsMax      = 0x00010000
		wsSysMenu  = 0x00080000
	)
	st, _, _ := procGetWindowLong.Call(hwnd, gwlStyle)
	if st&(wsCaption|wsFrame) == 0 && coversMonitor(hwnd) {
		return false // ya está sin bordes
	}
	procSetWindowLong.Call(hwnd, gwlStyle, st&^(wsCaption|wsFrame|wsMin|wsMax|wsSysMenu))
	ex, _, _ := procGetWindowLong.Call(hwnd, gwlExStyle)
	procSetWindowLong.Call(hwnd, gwlExStyle, ex&^(0x1|0x200|0x20000|0x100))
	m, _ := monitorOf(hwnd)
	procSetWindowPos.Call(hwnd, 0, uintptr(m.Left), uintptr(m.Top), uintptr(m.Right-m.Left), uintptr(m.Bottom-m.Top), 0x20|0x200|0x40) // FRAMECHANGED NOOWNERZORDER SHOWWINDOW
	return true
}
