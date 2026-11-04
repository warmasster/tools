package main

// «¿Quién me ha sacado del juego?»: durante una partida se apunta qué programa
// se pone delante del juego sin que hayas tocado el teclado ni el ratón (si
// acabas de pulsar Alt+Tab o la tecla Windows, has sido tú).

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procSetWinEventHook = user32.NewProc("SetWinEventHook")
	procUnhookWinEvent  = user32.NewProc("UnhookWinEvent")
	procGetLastInput    = user32.NewProc("GetLastInputInfo")
	procGetTickCount    = kernel32.NewProc("GetTickCount")
	fgEvents            = make(chan uintptr, 16)
	winEventCallback    = windows.NewCallback(func(hook, event, hwnd, idObject, idChild, thread, ms uintptr) uintptr {
		if event == 0x0003 && idObject == 0 { // EVENT_SYSTEM_FOREGROUND, OBJID_WINDOW
			select {
			case fgEvents <- hwnd:
			default:
			}
		}
		return 0
	})
)

// hookForeground se llama desde el hilo de las ventanas (que tiene bucle de mensajes).
func hookForeground() uintptr {
	h, _, _ := procSetWinEventHook.Call(0x0003, 0x0003, 0, winEventCallback, 0, 0, 0x0000|0x0002) // OUTOFCONTEXT | SKIPOWNPROCESS
	return h
}

// msSinceInput: cuánto hace de la última pulsación o movimiento del usuario.
func msSinceInput() uint32 {
	var li struct{ Size, Time uint32 }
	li.Size = uint32(unsafe.Sizeof(li))
	if r, _, _ := procGetLastInput.Call(uintptr(unsafe.Pointer(&li))); r == 0 {
		return 1 << 30
	}
	now, _, _ := procGetTickCount.Call()
	return uint32(now) - li.Time
}

// Theft es una vez que otro programa sacó el juego de delante.
type Theft struct {
	Time  int64  `json:"time"`
	Game  string `json:"game"`
	Exe   string `json:"exe"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	Idle  int    `json:"idleMs"` // ms sin tocar nada cuando pasó
	Class string `json:"class"`
}

var theftMu sync.Mutex

func theftsPath() string { return filepath.Join(dataDir(), "robos-de-foco.jsonl") }

// ignoredThief: ventanas que salen delante por algo que has hecho tú (o del sistema).
func ignoredThief(exe, class string) bool {
	switch exe {
	case "explorer.exe", "searchhost.exe", "startmenuexperiencehost.exe", "shellexperiencehost.exe", "lockapp.exe", "logonui.exe",
		"textinputhost.exe", "turbomon.exe", "taskmgr.exe", "dwm.exe", "":
		return true
	}
	return class == "Shell_TrayWnd" || class == "TaskSwitcherWnd" || class == "XamlExplorerHostIslandWindow" || class == "ForegroundStaging"
}

// watchFocus atiende los cambios de ventana de delante (goroutine del agente).
func (a *Agent) watchFocus() {
	var lastGame uint32
	for {
		select {
		case <-a.quit:
			return
		case hwnd := <-fgEvents:
			var pid uint32
			procGetWindowThreadPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
			a.st.mu.Lock()
			cur := a.st.cur
			var gamePid uint32
			game := ""
			if cur != nil {
				gamePid, game = cur.pid, cur.game
			}
			a.st.mu.Unlock()
			if gamePid == 0 {
				lastGame = 0
				continue
			}
			if pid == gamePid {
				lastGame = pid
				continue
			}
			if lastGame != gamePid {
				continue // el juego no estaba delante: no es un robo
			}
			lastGame = 0
			idle := msSinceInput()
			if idle < 1200 {
				continue // acabas de pulsar algo: lo has cambiado tú
			}
			info := a.procInfo(pid)
			cls := className(hwnd)
			if ignoredThief(info.exe, cls) || pid == uint32(os.Getpid()) {
				continue
			}
			t := Theft{Time: time.Now().Unix(), Game: game, Exe: info.exe, Name: fileDescription(info.path), Path: info.path, Idle: int(idle), Class: cls}
			if t.Name == "" {
				t.Name = strings.TrimSuffix(info.exe, ".exe")
			}
			logf("robo de foco: %s sacó a %s (%d ms sin tocar nada)", t.Exe, game, idle)
			theftMu.Lock()
			if f, err := os.OpenFile(theftsPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
				b, _ := json.Marshal(t)
				f.Write(append(b, '\n'))
				f.Close()
			}
			theftMu.Unlock()
		}
	}
}

func readThefts(days int) []Theft {
	theftMu.Lock()
	defer theftMu.Unlock()
	f, err := os.Open(theftsPath())
	if err != nil {
		return []Theft{}
	}
	defer f.Close()
	since := time.Now().AddDate(0, 0, -days).Unix()
	out := []Theft{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var t Theft
		if json.Unmarshal(sc.Bytes(), &t) == nil && t.Time >= since {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time > out[j].Time })
	return out
}

func (a *Agent) hThefts(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" && r.URL.Query().Get("clear") == "1" {
		theftMu.Lock()
		os.Remove(theftsPath())
		theftMu.Unlock()
	}
	writeJSON(w, readThefts(60))
}
