package main

// Ventana de TurboMON (Wails). Solo existe mientras está abierta: los datos en
// directo se los pide al agente; la revisión del equipo y la lista de
// programas los hace ella misma (sin permiso de administrador).

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	xwin "golang.org/x/sys/windows"

	"turbomon/internal/checks"
)

const windowClass = "TurboMONWindow"

type App struct {
	ctx   context.Context
	procs *procSampler
}

func userTag() string {
	if u, err := xwin.GetCurrentProcessToken().GetTokenUser(); err == nil {
		return strings.ReplaceAll(u.User.Sid.String(), "-", "")
	}
	return "u"
}

func runUI() {
	app := &App{procs: newProcSampler()}
	err := wails.Run(&options.App{
		Title:            "TurboMON",
		Width:            1180,
		Height:           780,
		MinWidth:         960,
		MinHeight:        640,
		Frameless:        true,
		BackgroundColour: &options.RGBA{R: 10, G: 11, B: 18, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		Bind:             []interface{}{app},
		DragAndDrop:      &options.DragAndDrop{DisableWebViewDrop: true},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "turbomon-ui-" + userTag(),
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				if app.ctx != nil {
					runtime.WindowUnminimise(app.ctx)
					runtime.Show(app.ctx)
				}
			},
		},
		Windows: &windows.Options{
			Theme:               windows.Dark,
			WebviewUserDataPath: filepath.Join(dataDir(), "webview"),
			DisablePinchZoom:    true,
			WindowClassName:     windowClass,
		},
	})
	if err != nil {
		messageBox("TurboMON", "No se pudo abrir TurboMON:\n\n"+err.Error(), true)
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	go ensureAgent()
}

func (a *App) raw(method, path string, body any, timeout time.Duration) (json.RawMessage, error) {
	var out json.RawMessage
	err := agentCall(method, path, body, &out, timeout)
	if err != nil {
		// El agente se ha cerrado (o aún no ha arrancado): se arranca y se reintenta una vez.
		if ensureAgent() == nil {
			err = agentCall(method, path, body, &out, timeout)
		}
	}
	return out, err
}

// Live: sensores, FPS y partida en curso.
func (a *App) Live() (json.RawMessage, error) { return a.raw("GET", "/live", nil, 3*time.Second) }

// Settings devuelve los ajustes.
func (a *App) Settings() (json.RawMessage, error) {
	return a.raw("GET", "/settings", nil, 3*time.Second)
}

// SaveSettings guarda los ajustes (el agente los aplica al momento).
func (a *App) SaveSettings(s json.RawMessage) (json.RawMessage, error) {
	return a.raw("POST", "/settings", s, 3*time.Second)
}

// SetOverlay pone o quita el overlay.
func (a *App) SetOverlay(on bool) (json.RawMessage, error) {
	return a.raw("POST", "/overlay", map[string]bool{"on": on}, 3*time.Second)
}

// Purge suelta la memoria en espera.
func (a *App) Purge() (json.RawMessage, error) { return a.raw("POST", "/purge", nil, 15*time.Second) }

// Sessions: las partidas guardadas (sin gráfica).
func (a *App) Sessions() (json.RawMessage, error) {
	return a.raw("GET", "/sessions", nil, 5*time.Second)
}

// Session: una partida con su gráfica.
func (a *App) Session(id string) (json.RawMessage, error) {
	return a.raw("GET", "/session?id="+urlEscape(id), nil, 5*time.Second)
}

// DeleteSession borra una partida.
func (a *App) DeleteSession(id string) error {
	_, err := a.raw("POST", "/session?delete=1&id="+urlEscape(id), nil, 5*time.Second)
	return err
}

// ClearSessions borra todas las partidas.
func (a *App) ClearSessions() error {
	_, err := a.raw("POST", "/sessions?clear=1", nil, 5*time.Second)
	return err
}

// Checks hace la revisión del equipo.
func (a *App) Checks() checks.Report { return checks.Run() }

// Fix arregla una comprobación y vuelve a revisar.
func (a *App) Fix(id string) (checks.Report, error) {
	if err := checks.Fix(id); err != nil {
		return checks.Run(), err
	}
	time.Sleep(300 * time.Millisecond) // la pantalla tarda un poco en dar los hercios nuevos
	return checks.Run(), nil
}

// Processes: lo que más gasta ahora.
func (a *App) Processes() []ProcGroup { return a.procs.sample() }

// KillGroup cierra todos los procesos de un programa.
func (a *App) KillGroup(exe string) (int, error) { return killGroup(exe) }

// EnableFPS pide permiso de administrador (una vez) para medir los FPS.
func (a *App) EnableFPS() error {
	if err := runElevated("setup-admin"); err != nil {
		if errors.Is(err, errUACCancelled) {
			return errors.New("no se ha dado el permiso")
		}
		return err
	}
	setRunKey(false)
	stopAgent()
	if err := startAgentTask(); err != nil {
		return err
	}
	return ensureAgent()
}

// QuitAgent cierra TurboMON del todo (el overlay y la bandeja también).
func (a *App) QuitAgent() error {
	stopAgent()
	runtime.Quit(a.ctx)
	return nil
}

// Version de TurboMON.
func (a *App) Version() string { return version }

func urlEscape(s string) string {
	return strings.NewReplacer("%", "%25", "&", "%26", "?", "%3F", "#", "%23", " ", "%20", "+", "%2B").Replace(s)
}
