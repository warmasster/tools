package main

import (
	"embed"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	xwin "golang.org/x/sys/windows"
)

//go:embed all:frontend
var assets embed.FS

func main() {
	app := NewApp()

	// "Abrir siempre en modo turbo": se vuelve a lanzar como administrador.
	// (TURBOCUT_NO_ELEVATE=1 lo evita, para las pruebas con wails dev)
	if s := app.getSettings(); s.AlwaysAdmin && !isAdmin() && os.Getenv("TURBOCUT_NO_ELEVATE") == "" {
		if err := relaunchAsAdmin(os.Args[1:]); err == nil {
			return
		}
	}
	if isAdmin() {
		// Elevado, el navegador interno no hace caso de las variables WEBVIEW2_* que
		// pueda haber dejado un programa sin permisos (abrir un puerto de depuración,
		// usar otro WebView2…): con ellas manejaría una ventana de administrador.
		for _, kv := range os.Environ() {
			k, _, _ := strings.Cut(kv, "=")
			if strings.HasPrefix(strings.ToUpper(k), "WEBVIEW2_") {
				os.Unsetenv(k)
			}
		}
	}

	err := wails.Run(&options.App{
		Title:            "TurboCUT",
		Width:            1320,
		Height:           840,
		MinWidth:         1020,
		MinHeight:        660,
		Frameless:        true,
		BackgroundColour: &options.RGBA{R: 10, G: 11, B: 18, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		Bind:             []interface{}{app},
		DragAndDrop:      &options.DragAndDrop{EnableFileDrop: true},
		Windows: &windows.Options{
			Theme:               windows.Dark,
			WebviewUserDataPath: filepath.Join(dataDir(), "webview"),
			DisablePinchZoom:    true,
		},
	})
	if err != nil {
		t, _ := xwin.UTF16PtrFromString("TurboCUT")
		m, _ := xwin.UTF16PtrFromString("No se pudo iniciar TurboCUT:\n\n" + err.Error())
		xwin.MessageBox(0, m, t, xwin.MB_ICONERROR)
	}
}
