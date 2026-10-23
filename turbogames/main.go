package main

import (
	"context"
	"embed"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	xwin "golang.org/x/sys/windows"
)

//go:embed all:frontend
var assets embed.FS

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:            "TurboGAMES",
		Width:            1280,
		Height:           820,
		MinWidth:         1000,
		MinHeight:        660,
		Frameless:        true,
		BackgroundColour: &options.RGBA{R: 10, G: 11, B: 18, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets, Handler: app.images},
		OnStartup:        app.startup,
		OnShutdown:       func(context.Context) { app.shutdownClip() },
		Bind:             []interface{}{app},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "turbogames-7c1f3e52-cuentas",
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
		},
	})
	if err != nil {
		t, _ := xwin.UTF16PtrFromString("TurboGAMES")
		m, _ := xwin.UTF16PtrFromString("No se pudo iniciar TurboGAMES:\n\n" + err.Error())
		xwin.MessageBox(0, m, t, xwin.MB_ICONERROR)
	}
}
