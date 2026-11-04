package main

// Ventana de TurboMON (Wails). Solo existe mientras está abierta: los datos en
// directo se los pide al agente; la revisión del equipo y la lista de
// programas los hace ella misma (sin permiso de administrador).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
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
	"turbomon/internal/events"
	"turbomon/internal/netmon"
	"turbomon/internal/parts"
)

const windowClass = "TurboMONWindow"

type App struct {
	ctx   context.Context
	procs *procSampler
	bloat *netmon.Bloat
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

// Checks hace la revisión del equipo (con las piezas que lee el agente, que ve también la salud de los discos).
func (a *App) Checks() checks.Report { return checks.Run(a.comps()) }

func (a *App) comps() *parts.Components {
	var c parts.Components
	if agentCall("GET", "/components", nil, &c, 10*time.Second) == nil && c.CPU.Name != "" {
		return &c
	}
	return nil
}

// Components: las piezas del equipo.
func (a *App) Components() (json.RawMessage, error) {
	return a.raw("GET", "/components", nil, 15*time.Second)
}

// StartDiskTest empieza a medir la velocidad de un disco.
func (a *App) StartDiskTest(disk int) error {
	_, err := a.raw("POST", fmt.Sprintf("/disktest?disk=%d", disk), nil, 5*time.Second)
	return err
}

// DiskTest: cómo va la prueba de velocidad.
func (a *App) DiskTest() (json.RawMessage, error) {
	return a.raw("GET", "/disktest", nil, 3*time.Second)
}

// Fix arregla una comprobación y vuelve a revisar.
func (a *App) Fix(id string) (checks.Report, error) {
	if err := checks.Fix(id); err != nil {
		return checks.Run(a.comps()), err
	}
	time.Sleep(300 * time.Millisecond) // la pantalla tarda un poco en dar los hercios nuevos
	return checks.Run(a.comps()), nil
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

// Crashes: cierres de juegos, cuelgues de la gráfica, reinicios y errores de hardware.
func (a *App) Crashes() events.Report {
	games := map[string]bool{}
	var list []SessionRecord
	if agentCall("GET", "/sessions", nil, &list, 5*time.Second) == nil {
		for _, s := range list {
			games[strings.ToLower(s.Exe)] = true
		}
	}
	raptor := false
	if c := a.comps(); c != nil {
		raptor = c.CPU.Vendor == "intel" && c.CPU.Family == 6 && c.CPU.Model == 0xB7 && c.CPU.Microcode != 0 && c.CPU.Microcode < 0x12B && !c.Board.Laptop
	}
	return events.Collect(events.Options{Days: 60, GameExes: games, IntelRaptor: raptor})
}

// Stability: la prueba de estabilidad (en marcha o la última).
func (a *App) Stability() (json.RawMessage, error) {
	return a.raw("GET", "/stability", nil, 3*time.Second)
}

// StartStability empieza la prueba de estabilidad.
func (a *App) StartStability() (json.RawMessage, error) {
	return a.raw("POST", "/stability", nil, 5*time.Second)
}

// StopStability para la prueba.
func (a *App) StopStability() (json.RawMessage, error) {
	return a.raw("POST", "/stability?stop=1", nil, 5*time.Second)
}

// Thefts: quién ha sacado el juego de delante (últimos 60 días).
func (a *App) Thefts() (json.RawMessage, error) { return a.raw("GET", "/thefts", nil, 3*time.Second) }

// ClearThefts borra la lista.
func (a *App) ClearThefts() error {
	_, err := a.raw("POST", "/thefts?clear=1", nil, 3*time.Second)
	return err
}

// StartupInfo dice qué programas de la lista arrancan con Windows.
func (a *App) StartupInfo(exes []string) map[string]bool {
	out := map[string]bool{}
	for _, e := range exes {
		out[e] = startupEnabled(e)
	}
	return out
}

// DisableStartup hace que un programa no arranque con Windows.
func (a *App) DisableStartup(exe string) error { return disableStartup(exe) }

// Profiles: los ajustes de cada juego.
func (a *App) Profiles() (json.RawMessage, error) {
	return a.raw("GET", "/profiles", nil, 3*time.Second)
}

// SaveProfile guarda los ajustes de un juego.
func (a *App) SaveProfile(exe string, p json.RawMessage) (json.RawMessage, error) {
	return a.raw("POST", "/profiles", map[string]any{"exe": exe, "profile": p}, 3*time.Second)
}

// Net: qué programa usa la red y el ping (lo mide el agente).
func (a *App) Net() (json.RawMessage, error) { return a.raw("GET", "/net", nil, 3*time.Second) }

// StartBloat empieza la prueba de ping con la conexión cargada (usa unos cientos de MB).
func (a *App) StartBloat() {
	if a.bloat == nil || !a.bloat.Get().Running {
		a.bloat = netmon.StartBloat(netip.MustParseAddr("8.8.8.8"))
	}
}

// Bloat: cómo va la prueba.
func (a *App) Bloat() *netmon.BloatState {
	if a.bloat == nil {
		return nil
	}
	s := a.bloat.Get()
	return &s
}

// Stutter: el análisis de tirones de la partida en curso y de las últimas.
func (a *App) Stutter() (json.RawMessage, error) { return a.raw("GET", "/stutter", nil, 4*time.Second) }

// Noise: el mapa de ruido de los núcleos (el último o el que está en marcha).
func (a *App) Noise() (json.RawMessage, error) { return a.raw("GET", "/noise", nil, 3*time.Second) }

// StartNoise empieza el mapa de ruido (un segundo por núcleo).
func (a *App) StartNoise() (json.RawMessage, error) {
	return a.raw("POST", "/noise", nil, 5*time.Second)
}

// Lab: los ajustes que se pueden probar, el experimento en marcha y los resultados.
func (a *App) Lab() (json.RawMessage, error) { return a.raw("GET", "/lab", nil, 3*time.Second) }

// LabAction: start (knob, pairs), stop, apply (id, on), delete (id).
func (a *App) LabAction(b json.RawMessage) (json.RawMessage, error) {
	return a.raw("POST", "/lab", b, 5*time.Second)
}

// Prefetch: lo aprendido para la precarga de cada juego.
func (a *App) Prefetch() (json.RawMessage, error) {
	return a.raw("GET", "/prefetch", nil, 3*time.Second)
}

// Version de TurboMON.
func (a *App) Version() string { return version }

func urlEscape(s string) string {
	return strings.NewReplacer("%", "%25", "&", "%26", "?", "%3F", "#", "%23", " ", "%20", "+", "%2B").Replace(s)
}
