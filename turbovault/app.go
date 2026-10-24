package main

// Ventana de TurboVAULT (Wails). No guarda la bóveda: todo se lo pide al agente
// por la tubería y lo enseña. Al cerrar la ventana el proceso termina y no
// queda nada en memoria.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	xwin "golang.org/x/sys/windows"

	"turbovault/internal/ipc"
	"turbovault/internal/winsec"
)

const windowClass = "TurboVAULTWindow"

type App struct {
	ctx   context.Context
	mu    sync.Mutex
	mode  string
	cl    *ipc.Client
	other *otherAgent
	state json.RawMessage
}

// Resp es la respuesta de Call para la interfaz.
type Resp struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
	Code  string          `json:"code,omitempty"`
}

func runUI(mode string) {
	guardWebView()
	_ = winsec.HardenProcess()
	_ = winsec.HardenExistingThreads()
	_ = winsec.BlockInjection(false)
	app := &App{mode: mode}
	w, h := 1180, 760
	if mode != "" {
		w, h = 440, 600
	}
	opts := &options.App{
		Title:            "TurboVAULT",
		Width:            w,
		Height:           h,
		MinWidth:         400,
		MinHeight:        520,
		Frameless:        true,
		BackgroundColour: &options.RGBA{R: 10, G: 11, B: 18, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		Bind:             []interface{}{app},
		// Soltar un archivo o un enlace en la ventana no la lleva a otra página.
		DragAndDrop: &options.DragAndDrop{DisableWebViewDrop: true},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "turbovault-ui-3f9c2a71" + testEnv("TURBOVAULT_PIPE"),
			OnSecondInstanceLaunch: func(d options.SecondInstanceData) {
				m := ""
				for _, a := range d.Args {
					switch a {
					case "--unlock":
						m = "unlock"
					case "--pair":
						m = "pair"
					}
				}
				app.prompt(m)
			},
		},
		Windows: &windows.Options{
			Theme:               windows.Dark,
			WebviewUserDataPath: filepath.Join(localDir(), "webview"),
			DisablePinchZoom:    true,
			WindowClassName:     windowClass,
		},
	}
	webviewOptions(opts)
	err := wails.Run(opts)
	if err != nil {
		t, _ := xwin.UTF16PtrFromString("TurboVAULT")
		m, _ := xwin.UTF16PtrFromString("No se pudo abrir TurboVAULT:\n\n" + err.Error())
		xwin.MessageBox(0, m, t, xwin.MB_ICONERROR)
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	go a.connectLoop()
	e2eHooks(a)
}

func (a *App) domReady(ctx context.Context) {
	go func() {
		for {
			_ = winsec.HardenExistingThreads()
			time.Sleep(3 * time.Second)
		}
	}()
	logf("ventana lista (modo %q, ventana %#x)", a.Mode(), a.hwnd())
	a.protect(getSettings().HideCapture)
	a.front()
}

// connectLoop mantiene la conexión con el agente (lo arranca si hace falta).
func (a *App) connectLoop() {
	for {
		conn, other, err := connectAgent(6 * time.Second)
		if err != nil {
			a.mu.Lock()
			a.other = other
			a.mu.Unlock()
			msg := "No se pudo arrancar el agente de TurboVAULT."
			if other != nil {
				msg = "Hay otro TurboVAULT en marcha desde otra carpeta: " + other.Path
			}
			runtime.EventsEmit(a.ctx, "agent", map[string]any{"ok": false, "error": msg, "other": other != nil})
			time.Sleep(3 * time.Second)
			continue
		}
		cl := ipc.NewClient(conn, func(m ipc.Message) {
			if m.Event == "state" {
				a.mu.Lock()
				a.state = m.Data
				a.mu.Unlock()
				runtime.EventsEmit(a.ctx, "state", m.Data)
			}
		})
		a.mu.Lock()
		a.cl = cl
		a.other = nil
		a.mu.Unlock()
		runtime.EventsEmit(a.ctx, "agent", map[string]any{"ok": true})
		<-cl.Done()
		a.mu.Lock()
		a.cl = nil
		a.mu.Unlock()
		if cl.Rejected {
			// El agente no reconoce esta ventana (por ejemplo, se reinició): se
			// le pide una ventana nueva y esta se cierra.
			logf("ventana no reconocida por el agente: se abre otra")
			spawnUI(modeArg(a.Mode())...)
			runtime.Quit(a.ctx)
			return
		}
		runtime.EventsEmit(a.ctx, "agent", map[string]any{"ok": false, "error": "Se perdió la conexión con el agente. Reconectando…"})
		time.Sleep(500 * time.Millisecond)
	}
}

// Call pasa una orden al agente.
func (a *App) Call(cmd string, args json.RawMessage) Resp {
	a.mu.Lock()
	cl := a.cl
	a.mu.Unlock()
	if cl == nil {
		return Resp{Error: "TurboVAULT se está conectando…", Code: "disconnected"}
	}
	if string(args) == "null" {
		args = nil
	}
	var out json.RawMessage
	var payload any
	if len(args) > 0 {
		payload = args
	}
	err := cl.Call(cmd, payload, &out, 3*time.Minute)
	if err != nil {
		var ce *ipc.CallError
		if errors.As(err, &ce) {
			return Resp{Error: ce.Msg, Code: ce.Code}
		}
		return Resp{Error: err.Error(), Code: "error"}
	}
	return Resp{OK: true, Data: out}
}

// Mode dice si la ventana se abrió solo para desbloquear o vincular.
func (a *App) Mode() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.mode
}

// SetMode cambia entre la ventana completa ("") y la pequeña.
func (a *App) SetMode(m string) {
	a.mu.Lock()
	a.mode = m
	a.mu.Unlock()
	if m == "" {
		runtime.WindowSetSize(a.ctx, 1180, 760)
	} else {
		runtime.WindowSetSize(a.ctx, 440, 600)
	}
	runtime.WindowCenter(a.ctx)
}

func modeArg(m string) []string {
	if m == "unlock" || m == "pair" {
		return []string{"--" + m}
	}
	return nil
}

func (a *App) prompt(m string) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "prompt", m)
	a.front()
}

// Version de TurboVAULT.
func (a *App) Version() string { return version }

// KillOtherAgent cierra un agente de otra carpeta (otra versión).
func (a *App) KillOtherAgent() error {
	a.mu.Lock()
	o := a.other
	a.mu.Unlock()
	if o == nil {
		return nil
	}
	if !strings.EqualFold(filepath.Base(o.Path), filepath.Base(selfExe())) && !strings.HasPrefix(strings.ToLower(filepath.Base(o.Path)), "turbovault") {
		return errors.New("no es TurboVAULT")
	}
	return killProcess(o.PID)
}

// ---------- archivos ----------

// PickFile pide un archivo para abrir.
func (a *App) PickFile(title, filterName, pattern string) string {
	f, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   title,
		Filters: []runtime.FileFilter{{DisplayName: filterName, Pattern: pattern}},
	})
	if err != nil {
		return ""
	}
	return f
}

// PickSaveFile pide dónde guardar un archivo.
func (a *App) PickSaveFile(title, name, filterName, pattern string) string {
	f, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           title,
		DefaultFilename: name,
		Filters:         []runtime.FileFilter{{DisplayName: filterName, Pattern: pattern}},
	})
	if err != nil {
		return ""
	}
	return f
}

// PickFolder pide una carpeta.
func (a *App) PickFolder(title string) string {
	f, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: title, CanCreateDirectories: true})
	if err != nil {
		return ""
	}
	return f
}

// ShowInFolder abre el Explorador en esa carpeta o archivo.
func (a *App) ShowInFolder(path string) {
	// Ruta completa: que nadie cuele otro explorer.exe por el PATH.
	win, err := xwin.GetSystemWindowsDirectory()
	if err != nil {
		return
	}
	explorer := filepath.Join(win, "explorer.exe")
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		exec.Command(explorer, "/select,", path).Start()
		return
	}
	exec.Command(explorer, path).Start()
}

// DataFolder es donde está la bóveda.
func (a *App) DataFolder() string { return dataDir() }

// OpenURL abre una web en el navegador.
func (a *App) OpenURL(u string) {
	if strings.HasPrefix(u, "https://") {
		runtime.BrowserOpenURL(a.ctx, u)
	}
}

// ---------- extensión ----------

type ExtInfo struct {
	Dir       string   `json:"dir"`
	Installed bool     `json:"installed"`
	Loaded    []string `json:"loaded"`
	Host      bool     `json:"host"`
	ID        string   `json:"id"`
}

// ExtensionInfo dice dónde está la extensión y si Chrome la tiene cargada.
func (a *App) ExtensionInfo() ExtInfo {
	dir := extensionDir()
	_, err := os.Stat(filepath.Join(dir, "manifest.json"))
	return ExtInfo{Dir: dir, Installed: err == nil, Loaded: chromeExtensionPaths(), Host: hostRegistered(), ID: extensionID}
}

// SetupExtension deja la extensión lista y abre chrome://extensions con la
// ruta copiada (Chrome no deja instalarla sola).
func (a *App) SetupExtension() (ExtInfo, error) {
	if err := registerHost(); err != nil {
		return a.ExtensionInfo(), err
	}
	if _, err := installExtension(); err != nil {
		return a.ExtensionInfo(), err
	}
	setClipboard(extensionDir(), false)
	openInChrome("chrome://extensions/")
	return a.ExtensionInfo(), nil
}

// ---------- ventana ----------

func (a *App) hwnd() uintptr { return findOwnWindow(windowClass) }

// protect quita (o vuelve a poner) la ventana de capturas y grabaciones.
func (a *App) protect(on bool) {
	h := a.hwnd()
	if h == 0 {
		return
	}
	if on {
		winsec.ExcludeFromCapture(h)
	} else {
		procSetWindowDisplayAffinity.Call(h, 0)
	}
}

// Protect cambia la protección contra capturas desde Ajustes.
func (a *App) Protect(on bool) { a.protect(on) }

func (a *App) front() {
	if a.ctx == nil {
		return
	}
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowShow(a.ctx)
	bringToFront(a.hwnd())
}
