package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	xwin "golang.org/x/sys/windows"

	"turbomenu/internal/win11menu"
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

const windowClass = "TurboMENUWindow"

func main() {
	args := os.Args[1:]
	if win11menu.IsServerLaunch(args) {
		serveModern() // lo lanza el Explorador para el menú nuevo de Windows 11
		return
	}
	if len(args) == 0 {
		runWindow(&Job{Action: "home"}, false)
		return
	}
	switch strings.ToLower(args[0]) {
	case "setup":
		os.Exit(cliSetup())
	case "uninstall":
		os.Exit(cliUninstall())
	case "--idle":
		// Arranque al iniciar sesión: queda preparado y oculto para abrir al instante.
		runWindow(nil, true)
		return
	case "refresh":
		// TurboSETUP lo llama al instalar o quitar otras herramientas.
		if exe, err := os.Executable(); err == nil && menuInstalled(exe) && !getSettings().Modern {
			registerMenu(exe)
		}
		return
	case "quit":
		quitIdle()
		return
	case "paste":
		// Sin ventana: crea el archivo y lo deja seleccionado para renombrar.
		if len(args) > 1 {
			pasteAsFile(args[1])
		}
		return
	}

	job := &Job{Action: strings.ToLower(args[0]), Paths: expandArgs(args[1:]), Time: time.Now().UnixMilli()}
	if !validAction(job.Action) {
		job = &Job{Action: "home"}
	}
	job.X, job.Y = cursorPos()
	if handoff(job) {
		return
	}
	runWindow(job, false)
}

// selectionList: solo se leen (y luego se borran) las listas que deja el menú nuevo
// en %TEMP% (TurboMENU-sel-*.txt), nunca un archivo cualquiera que llegue con @.
func selectionList(p string) bool {
	dir, name := filepath.Dir(filepath.Clean(p)), filepath.Base(p)
	ok, _ := filepath.Match("TurboMENU-sel-*.txt", name)
	return ok && strings.EqualFold(dir, filepath.Clean(os.TempDir()))
}

// expandArgs admite rutas sueltas y @lista (un archivo de texto con una ruta por línea,
// para selecciones enormes que no caben en la línea de órdenes).
func expandArgs(in []string) []string {
	var out []string
	for _, a := range in {
		a = strings.TrimSpace(strings.Trim(a, `"`))
		if a == "" {
			continue
		}
		if strings.HasPrefix(a, "@") && selectionList(a[1:]) {
			if b, err := os.ReadFile(a[1:]); err == nil {
				for _, l := range strings.Split(string(b), "\n") {
					if l = strings.TrimSpace(l); l != "" {
						out = append(out, l)
					}
				}
				os.Remove(a[1:])
				continue
			}
		}
		out = append(out, a)
	}
	for i, p := range out {
		// «E:\» llega como «E:» (el \" final se lee como comilla): Abs("E:") sería la
		// carpeta actual de esa unidad, no su raíz.
		if len(p) == 2 && p[1] == ':' {
			p += `\`
		}
		if abs, err := filepath.Abs(p); err == nil {
			out[i] = abs
		}
	}
	return out
}

// webviewSlot da a cada proceso abierto a la vez su propia carpeta del navegador interno
// (compartirla entre dos procesos a la vez falla). Los procesos que se suceden reutilizan
// la primera, que ya tiene la caché caliente.
func webviewSlot() string {
	for i := 0; i < 16; i++ {
		name, _ := xwin.UTF16PtrFromString(fmt.Sprintf(`Local\TurboMENU.webview.%s.%d`, userTag(), i))
		h, _, err := procCreateMutex.Call(0, 1, uintptr(unsafe.Pointer(name)))
		if h != 0 && err != xwin.ERROR_ALREADY_EXISTS {
			return filepath.Join(cacheDir(), "webview", fmt.Sprint(i)) // el mutex muere con el proceso
		}
		if h != 0 {
			xwin.CloseHandle(xwin.Handle(h))
		}
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("TurboMENU-webview-%d", os.Getpid()))
}

func runWindow(job *Job, idle bool) {
	app := NewApp(job, idle)
	err := wails.Run(&options.App{
		Title:            "TurboMENU",
		Width:            460,
		Height:           420,
		MinWidth:         340,
		MinHeight:        200,
		Frameless:        true,
		StartHidden:      true,
		BackgroundColour: &options.RGBA{R: 14, G: 15, B: 24, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		OnShutdown:       app.shutdown,
		OnBeforeClose:    app.beforeClose,
		Bind:             []interface{}{app},
		DragAndDrop:      &options.DragAndDrop{EnableFileDrop: true},
		Windows: &windows.Options{
			Theme:               windows.Dark,
			WebviewUserDataPath: webviewSlot(),
			DisablePinchZoom:    true,
			WindowClassName:     windowClass,
		},
	})
	if err != nil {
		t, _ := xwin.UTF16PtrFromString("TurboMENU")
		m, _ := xwin.UTF16PtrFromString("No se pudo abrir TurboMENU:\n\n" + err.Error())
		xwin.MessageBox(0, m, t, xwin.MB_ICONERROR)
	}
}
