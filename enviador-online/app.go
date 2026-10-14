package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx context.Context
	dir string

	mu       sync.RWMutex
	settings Settings
	port     int
	srvErr   string

	shares  *ShareStore
	tracker *Tracker
	server  *PublicServer
	tunnel  *Tunnel
	direct  *Direct
}

func NewApp() *App {
	a := &App{dir: dataDir()}
	a.settings = defaultSettings()
	_ = loadJSON(filepath.Join(a.dir, "settings.json"), &a.settings)
	a.settings.normalize()
	a.shares = newShareStore(a.dir)
	a.tracker = newTracker(func(t *Transfer) { a.shares.IncDownloads(t.ShareID) })
	a.server = newPublicServer(a)
	a.tunnel = newTunnel(a.dir, a.server.nonce, nil)
	a.direct = newDirect(a.server.nonce, nil)
	return a
}

func (a *App) getSettings() Settings {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.settings
}

func (a *App) directBase() string { return a.direct.Base() }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.startNetwork()

	runtime.OnFileDrop(ctx, func(x, y int, paths []string) {
		go a.createAndNotify(paths)
	})

	if args := os.Args[1:]; len(args) > 0 {
		go func() {
			time.Sleep(800 * time.Millisecond)
			a.createAndNotify(args)
		}()
	}

	go func() {
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				a.tracker.Tick()
				runtime.EventsEmit(ctx, "state", a.GetState())
			}
		}
	}()
}

func (a *App) startNetwork() {
	s := a.getSettings()
	port, err := a.server.Start(s.Port)
	a.mu.Lock()
	a.port = port
	if err != nil {
		a.srvErr = "No se pudo abrir ningún puerto local: " + err.Error()
	} else {
		a.srvErr = ""
	}
	a.mu.Unlock()
	if err != nil {
		return
	}
	if s.Tunnel {
		a.tunnel.Start(port)
	}
	a.direct.Start(port, s.UPnP, s.ForceDirect)
}

func (a *App) stopNetwork() {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); a.direct.Stop() }()
	go func() { defer wg.Done(); a.tunnel.Stop() }()
	wg.Wait()
	a.server.Close()
}

func (a *App) shutdown(ctx context.Context) {
	a.stopNetwork()
}

func (a *App) beforeClose(ctx context.Context) bool {
	if a.tracker.ActiveCount() == 0 {
		return false
	}
	res, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "Hay descargas en curso",
		Message:       "Si cierras Enviador, las descargas activas se cortarán. ¿Cerrar igualmente?",
		Buttons:       []string{"Yes", "No"},
		DefaultButton: "No",
	})
	return err == nil && res != "Yes"
}

func (a *App) onSecondInstance(d options.SecondInstanceData) {
	if a.ctx == nil {
		return
	}
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowShow(a.ctx)
	if len(d.Args) > 0 {
		go a.createAndNotify(d.Args)
	}
}

func (a *App) createAndNotify(paths []string) {
	runtime.EventsEmit(a.ctx, "busy", true)
	sh, err := a.shares.Create(paths)
	runtime.EventsEmit(a.ctx, "busy", false)
	if err != nil {
		runtime.EventsEmit(a.ctx, "toast", map[string]string{"kind": "error", "text": err.Error()})
		return
	}
	runtime.EventsEmit(a.ctx, "state", a.GetState())
	runtime.EventsEmit(a.ctx, "created", sh.ID)
}

// ---------- Estado para la interfaz ----------

type ShareLinks struct {
	Cloudflare string `json:"cloudflare"`
	Direct     string `json:"direct"`
	Lan        string `json:"lan"`
}

type ShareView struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Kind      string     `json:"kind"`
	Count     int        `json:"count"`
	Total     int64      `json:"total"`
	Created   int64      `json:"created"`
	Downloads int        `json:"downloads"`
	Preview   []string   `json:"preview"`
	Links     ShareLinks `json:"links"`
}

type State struct {
	Port      int            `json:"port"`
	ServerErr string         `json:"serverErr"`
	Tunnel    TunnelState    `json:"tunnel"`
	Direct    DirectState    `json:"direct"`
	Lan       string         `json:"lan"`
	Shares    []ShareView    `json:"shares"`
	Transfers []TransferView `json:"transfers"`
	Speed     float64        `json:"speed"`
	History   []float64      `json:"history"`
	Settings  Settings       `json:"settings"`
	DataDir   string         `json:"dataDir"`
}

func (a *App) GetState() State {
	a.mu.RLock()
	port, srvErr, set := a.port, a.srvErr, a.settings
	a.mu.RUnlock()
	ts := a.tunnel.State()
	ts.Log = nil
	ds := a.direct.State()
	lan := ""
	if ip := ds.LocalIP; ip != "" {
		lan = fmt.Sprintf("http://%s:%d", ip, port)
	} else if ip := lanIP(); ip != "" {
		lan = fmt.Sprintf("http://%s:%d", ip, port)
	}
	dbase := a.direct.Base()

	list, speed, hist := a.tracker.Snapshot()
	st := State{
		Port: port, ServerErr: srvErr, Tunnel: ts, Direct: ds, Lan: lan,
		Transfers: list, Speed: speed, History: hist, Settings: set, DataDir: a.dir,
	}
	if st.Transfers == nil {
		st.Transfers = []TransferView{}
	}
	st.Shares = []ShareView{}
	for _, sh := range a.shares.List() {
		v := ShareView{ID: sh.ID, Name: sh.Name, Kind: sh.Kind, Count: len(sh.Files), Total: sh.Total, Created: sh.Created, Downloads: sh.Downloads}
		for i, f := range sh.Files {
			if i >= 4 {
				break
			}
			v.Preview = append(v.Preview, f.Rel)
		}
		if ts.URL != "" {
			v.Links.Cloudflare = ts.URL + "/s/" + sh.ID
		}
		if dbase != "" {
			v.Links.Direct = dbase + "/s/" + sh.ID
		}
		if lan != "" {
			v.Links.Lan = lan + "/s/" + sh.ID
		}
		st.Shares = append(st.Shares, v)
	}
	return st
}

// ---------- Métodos llamados desde la interfaz ----------

func (a *App) PickFiles() error {
	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{Title: "Elige los archivos que quieres enviar"})
	if err != nil || len(paths) == 0 {
		return err
	}
	go a.createAndNotify(paths)
	return nil
}

func (a *App) PickFolder() error {
	p, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Elige la carpeta que quieres enviar"})
	if err != nil || p == "" {
		return err
	}
	go a.createAndNotify([]string{p})
	return nil
}

func (a *App) DeleteShare(id string) { a.shares.Delete(id) }

func (a *App) RenameShare(id, name string) { a.shares.Rename(id, name) }

func (a *App) CopyText(text string) error { return runtime.ClipboardSetText(a.ctx, text) }

func (a *App) OpenURL(u string) { runtime.BrowserOpenURL(a.ctx, u) }

func explorer(args string) {
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: "explorer.exe " + args}
	_ = cmd.Start()
	go cmd.Wait()
}

func (a *App) RevealShare(id string) {
	sh := a.shares.Get(id)
	if sh == nil || len(sh.Sources) == 0 {
		return
	}
	explorer(`/select,"` + sh.Sources[0] + `"`)
}

func (a *App) OpenDataDir() { explorer(`"` + a.dir + `"`) }

func (a *App) TunnelLog() []string { return a.tunnel.State().Log }

func (a *App) RestartTunnel() {
	a.mu.RLock()
	port, enabled := a.port, a.settings.Tunnel
	a.mu.RUnlock()
	if enabled && port > 0 {
		go a.tunnel.Start(port)
	}
}

func (a *App) RetryDirect() {
	a.mu.RLock()
	port, s := a.port, a.settings
	a.mu.RUnlock()
	if port > 0 {
		go a.direct.Start(port, s.UPnP, s.ForceDirect)
	}
}

func (a *App) SaveSettings(s Settings) error {
	s.normalize()
	a.mu.Lock()
	old := a.settings
	a.settings = s
	a.mu.Unlock()
	if err := saveJSON(filepath.Join(a.dir, "settings.json"), s); err != nil {
		return err
	}
	// El cambio de puerto se aplica al reiniciar la app.
	go func() {
		if s.Tunnel != old.Tunnel {
			if s.Tunnel {
				a.RestartTunnel()
			} else {
				a.tunnel.Stop()
			}
		}
		if s.UPnP != old.UPnP || s.ForceDirect != old.ForceDirect {
			a.RetryDirect()
		}
	}()
	return nil
}
