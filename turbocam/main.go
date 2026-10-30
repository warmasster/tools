package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	xwin "golang.org/x/sys/windows"
)

//go:embed all:frontend
var assets embed.FS

//go:embed wails.json
var wailsJSON []byte

// version sale de wails.json (info.productVersion), que es lo que publica el workflow.
var version = func() string {
	var w struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	if json.Unmarshal(wailsJSON, &w) == nil && w.Info.ProductVersion != "" {
		return w.Info.ProductVersion
	}
	return "0.0.0"
}()

const (
	windowClass = "TurboCAMWindow"
	minW, minH  = 980, 640
)

func main() {
	if len(os.Args) > 1 {
		switch strings.ToLower(os.Args[1]) {
		case "setup":
			os.Exit(cliSetup())
		case "uninstall":
			os.Exit(cliUninstall())
		case "nativecam": // nativecam install|remove [archivo de resultado]: lo lanza TurboCAM elevado
			os.Exit(cliNativeCam(os.Args[2:]))
		}
	}
	app := NewApp()
	err := wails.Run(&options.App{
		Title:            "TurboCAM",
		Width:            1240,
		Height:           780,
		MinWidth:         minW,
		MinHeight:        minH,
		Frameless:        true,
		BackgroundColour: &options.RGBA{R: 10, G: 11, B: 18, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		OnBeforeClose:    app.beforeClose,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "turbocam-" + userTag(),
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				if app.ctx != nil {
					runtime.WindowUnminimise(app.ctx)
					runtime.Show(app.ctx)
				}
			},
		},
		Windows: &windows.Options{
			Theme:               windows.Dark,
			WebviewUserDataPath: filepath.Join(cacheDir(), "webview"),
			DisablePinchZoom:    true,
			WindowClassName:     windowClass,
		},
	})
	if err != nil {
		fatal("No se pudo abrir TurboCAM:\n\n" + err.Error())
	}
}

func fatal(msg string) {
	t, _ := xwin.UTF16PtrFromString("TurboCAM")
	m, _ := xwin.UTF16PtrFromString(msg)
	xwin.MessageBox(0, m, t, xwin.MB_ICONERROR)
	os.Exit(1)
}

// userTag distingue a cada usuario de Windows (para el candado de instancia única).
func userTag() string {
	if u, err := xwin.GetCurrentProcessToken().GetTokenUser(); err == nil {
		return strings.ReplaceAll(u.User.Sid.String(), "-", "")
	}
	return "u"
}

// ---------- órdenes de TurboSETUP ----------

// cliSetup registra la cámara y crea el acceso directo del menú Inicio.
func cliSetup() int {
	if err := installCamera(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if exe, err := os.Executable(); err == nil {
		createShortcut(startMenuLink(), exe, "El móvil como cámara del PC")
	}
	// En Windows 11, la cámara nativa (la única que ven algunas apps, como Discord).
	// Pide permiso de administrador; solo se pregunta una vez: si se dice que no, se
	// puede activar luego en Ajustes.
	if nativeCamSupported() && !nativeCamInstalled() && !getSettings().NativeAsked {
		updateSettings(func(s *Settings) { s.NativeAsked = true })
		if err := enableNativeCam(); err != nil && !errors.Is(err, errUACCancelled) {
			fmt.Fprintln(os.Stderr, "cámara nativa:", err)
		}
	}
	fmt.Println("TurboCAM instalado: ya aparece como cámara en Discord, Meet, OBS…")
	return 0
}

// cliUninstall cierra TurboCAM si está abierto y quita la cámara y el acceso directo.
// Los ajustes (%APPDATA%\TurboCAM) se quedan, como en el resto de herramientas.
func cliUninstall() int {
	signalQuit()
	if nativeCamInstalled() {
		if err := nativeCamElevated("remove"); err != nil {
			fmt.Fprintln(os.Stderr, "cámara nativa:", err)
		}
	}
	uninstallCamera()
	os.Remove(startMenuLink())
	os.RemoveAll(cacheDir())
	fmt.Println("TurboCAM desinstalado.")
	return 0
}
