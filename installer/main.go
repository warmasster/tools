package main

import (
	"embed"
	"encoding/json"
	"os"
	"strconv"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend
var assets embed.FS

//go:embed wails.json
var wailsJSON []byte

// version sale de wails.json, así solo hay un sitio donde subirla.
var version = func() string {
	var w struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	json.Unmarshal(wailsJSON, &w)
	return w.Info.ProductVersion
}()

func main() {
	args := os.Args[1:]

	// Tras actualizarse, la copia nueva espera a que la vieja se cierre.
	if len(args) >= 2 && args[0] == "--wait-pid" {
		if pid, err := strconv.Atoi(args[1]); err == nil {
			waitForProcess(pid)
		}
		args = args[2:]
	}
	cleanupOldFiles()

	// "Desinstalar" desde Configuración de Windows: sin ventana, solo un aviso.
	if len(args) >= 2 && args[0] == "--uninstall" {
		os.Exit(cliUninstall(args[1]))
	}

	app := NewApp()
	err := wails.Run(&options.App{
		Title:            "TurboSETUP",
		Width:            860,
		Height:           640,
		MinWidth:         700,
		MinHeight:        540,
		Frameless:        true,
		BackgroundColour: &options.RGBA{R: 10, G: 11, B: 18, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "turbosetup-3c9a71",
			OnSecondInstanceLaunch: app.onSecondInstance,
		},
		Windows: &windows.Options{
			Theme:               windows.Dark,
			WebviewUserDataPath: webviewDir(),
			DisablePinchZoom:    true,
		},
	})
	if err != nil {
		messageBox("TurboSETUP", "No se pudo abrir TurboSETUP:\n\n"+err.Error(), mbError)
	}
}
