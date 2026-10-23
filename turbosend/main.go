package main

import (
	"embed"
	"path/filepath"

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
	state := options.Normal
	if startMinimised() {
		state = options.Minimised
	}
	err := wails.Run(&options.App{
		WindowStartState: state,
		Title:            "TurboSEND",
		Width:            1320,
		Height:           840,
		MinWidth:         980,
		MinHeight:        640,
		Frameless:        true,
		BackgroundColour: &options.RGBA{R: 10, G: 11, B: 18, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		OnBeforeClose:    app.beforeClose,
		Bind:             []interface{}{app},
		DragAndDrop:      &options.DragAndDrop{EnableFileDrop: true},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "enviador-v2-chats-5d1e8b",
			OnSecondInstanceLaunch: app.onSecondInstance,
		},
		Windows: &windows.Options{
			Theme:               windows.Dark,
			WebviewUserDataPath: filepath.Join(dataDir(), "webview"),
			DisablePinchZoom:    true,
			WindowClassName:     windowClass,
		},
	})
	if err != nil {
		t, _ := xwin.UTF16PtrFromString("TurboSEND")
		m, _ := xwin.UTF16PtrFromString("No se pudo iniciar TurboSEND:\n\n" + err.Error())
		xwin.MessageBox(0, m, t, xwin.MB_ICONERROR)
	}
}
