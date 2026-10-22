package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/options"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx context.Context

	mu      sync.Mutex
	man     *Manifest
	running map[string]context.CancelFunc // descargas en marcha, por id
}

func NewApp() *App { return &App{running: map[string]context.CancelFunc{}} }

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// shutdown: si ya no queda ninguna herramienta, no dejamos ni la carpeta.
func (a *App) shutdown(ctx context.Context) {
	if len(listInstalled()) == 0 {
		removeEverythingLater()
	}
}

func (a *App) onSecondInstance(options.SecondInstanceData) {
	wruntime.WindowUnminimise(a.ctx)
	wruntime.WindowShow(a.ctx)
}

// Catalog lee el catálogo de GitHub y lo cruza con lo instalado.
func (a *App) Catalog() *CatalogView {
	m, err := fetchManifest()
	a.mu.Lock()
	if err == nil {
		a.man = m
	} else if a.man != nil {
		m, err = a.man, nil // sin red: lo último que se leyó
	}
	a.mu.Unlock()
	v := buildView(m, err)
	sortedTools(v.Tools)
	return v
}

func (a *App) manifest() (*Manifest, error) {
	a.mu.Lock()
	m := a.man
	a.mu.Unlock()
	if m != nil {
		return m, nil
	}
	m, err := fetchManifest()
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.man = m
	a.mu.Unlock()
	return m, nil
}

// Install descarga e instala (o actualiza) una herramienta. Emite "progress".
func (a *App) Install(id string, desktop bool) (*InstallResult, error) {
	m, err := a.manifest()
	if err != nil {
		return nil, err
	}
	t := m.find(id)
	if t == nil {
		return nil, errors.New("esa herramienta no está en el catálogo")
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	if _, busy := a.running[id]; busy {
		a.mu.Unlock()
		cancel()
		return nil, errors.New("ya se está instalando")
	}
	a.running[id] = cancel
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.running, id)
		a.mu.Unlock()
		cancel()
	}()

	res, err := installTool(ctx, t, desktop, func(done, total int64) {
		wruntime.EventsEmit(a.ctx, "progress", map[string]any{"id": id, "done": done, "total": total})
	})
	if errors.Is(err, context.Canceled) {
		return nil, errors.New("cancelado")
	}
	return res, err
}

// Cancel corta una descarga en marcha.
func (a *App) Cancel(id string) {
	a.mu.Lock()
	if c, ok := a.running[id]; ok {
		c()
	}
	a.mu.Unlock()
}

func (a *App) Uninstall(id string) (string, error) { return uninstallTool(id) }

// Open abre una herramienta instalada.
func (a *App) Open(id string) error {
	in, ok := listInstalled()[id]
	if !ok || in.Exe == "" {
		return errors.New("no está instalada")
	}
	return startDetached(in.Exe)
}

func (a *App) OpenURL(url string) { wruntime.BrowserOpenURL(a.ctx, url) }

func (a *App) OpenFolder() {
	os.MkdirAll(rootDir(), 0o755)
	startDetached(filepath.Join(os.Getenv("WINDIR"), "explorer.exe"), rootDir())
}

// UpdateSelf baja el TurboSETUP nuevo, se sustituye y se reinicia.
func (a *App) UpdateSelf() error {
	m, err := a.manifest()
	if err != nil {
		return err
	}
	if m.Setup.Asset == "" {
		return errors.New("no hay versión nueva")
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	tmp := self + ".new"
	err = download(context.Background(), m.Setup, tmp, func(done, total int64) {
		wruntime.EventsEmit(a.ctx, "progress", map[string]any{"id": "__setup", "done": done, "total": total})
	})
	if err != nil {
		return err
	}
	if _, err := replaceFile(tmp, self); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("no se pudo reemplazar TurboSETUP: %w", err)
	}
	if err := startDetached(self, "--wait-pid", strconv.Itoa(os.Getpid())); err != nil {
		return err
	}
	wruntime.Quit(a.ctx)
	return nil
}
