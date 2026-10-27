package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"rsc.io/qr"
)

type App struct {
	ctx     context.Context
	nonce   string
	shm     *camShm
	engine  *Engine
	server  *Server
	preview *Preview
	tunnel  *Tunnel
	direct  *Direct
	hotkeys *hotkeyManager

	camMu  sync.Mutex
	camErr string

	allowClose atomic.Bool
	windowMode atomic.Bool
	savedW     int
	savedH     int

	notify chan struct{}
	quit   chan struct{}

	qrMu    sync.Mutex
	qrCache map[string]string
}

func NewApp() *App {
	return &App{nonce: randHex(16), notify: make(chan struct{}, 1), quit: make(chan struct{}), qrCache: map[string]string{}}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	go a.notifier()

	if err := installCamera(); err != nil {
		a.setCamErr(err.Error())
	}
	shm, err := openShm()
	if err != nil {
		fatal("No se pudo preparar la cámara virtual:\n\n" + err.Error())
	}
	a.shm = shm
	a.engine = newEngine(shm, a.changed, a.toast)
	if a.server, err = startServer(a.engine, a.nonce); err != nil {
		fatal(err.Error())
	}
	if a.preview, err = startPreview(a.engine); err != nil {
		fatal("No se pudo preparar la vista previa: " + err.Error())
	}
	a.tunnel = newTunnel(cacheDir(), a.nonce, a.changed)
	a.direct = newDirect(a.nonce, a.changed)
	// Si TurboCAM se cerró de golpe, el puerto que dejó abierto en el router se cierra.
	a.direct.recordPath = filepath.Join(dataDir(), "upnp.json")
	a.startNetwork(true, true)
	a.hotkeys = startHotkeys(a.onHotkey)
	go a.watchQuit()
}

func (a *App) startNetwork(tunnel, direct bool) {
	s := getSettings()
	if tunnel {
		go a.tunnel.Start(a.server.port, s.CFToken, s.CFHost)
	}
	if direct {
		go a.direct.Start(a.server.port, s.UPnP, false)
	}
}

func (a *App) domReady(ctx context.Context) {
	roundCorners(findWindow(windowClass))
	a.changed()
}

func (a *App) beforeClose(ctx context.Context) bool {
	if a.allowClose.Load() || a.engine == nil {
		return false
	}
	st := a.engine.State()
	if len(st.Phones) == 0 && len(st.Consumers) == 0 {
		return false
	}
	// Hay móviles o apps usando la cámara: mejor preguntar.
	runtime.EventsEmit(ctx, "close-request")
	return true
}

func (a *App) shutdown(ctx context.Context) {
	close(a.quit)
	if a.hotkeys != nil {
		a.hotkeys.Stop()
	}
	if a.engine != nil {
		a.engine.Close()
	}
	var wg sync.WaitGroup
	if a.direct != nil {
		wg.Add(1)
		go func() { defer wg.Done(); a.direct.Stop() }() // cierra el puerto del router
	}
	if a.tunnel != nil {
		wg.Add(1)
		go func() { defer wg.Done(); a.tunnel.Stop() }()
	}
	if a.server != nil {
		a.server.Close()
	}
	if a.preview != nil {
		a.preview.Close()
	}
	wg.Wait()
	a.shm.Close()
}

// watchQuit cierra TurboCAM si se lo pide su desinstalador.
func (a *App) watchQuit() {
	ev, err := quitEvent()
	if err != nil {
		return
	}
	if waitEvent(ev, a.quit) {
		a.allowClose.Store(true)
		runtime.Quit(a.ctx)
	}
}

// ---------- avisos a la ventana ----------

func (a *App) changed() {
	select {
	case a.notify <- struct{}{}:
	default:
	}
}

// notifier junta los avisos: como mucho unas 8 veces por segundo.
func (a *App) notifier() {
	for {
		select {
		case <-a.quit:
			return
		case <-a.notify:
		}
		if a.ctx != nil && a.engine != nil {
			runtime.EventsEmit(a.ctx, "state", a.State())
		}
		select {
		case <-a.quit:
			return
		case <-time.After(120 * time.Millisecond):
		}
	}
}

func (a *App) toast(kind, msg string) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "toast", map[string]string{"kind": kind, "msg": msg})
	}
}

func (a *App) setCamErr(s string) {
	a.camMu.Lock()
	a.camErr = s
	a.camMu.Unlock()
	a.changed()
}

// ---------- estado ----------

type LinkInfo struct {
	Status   string  `json:"status"`
	URL      string  `json:"url"`
	Message  string  `json:"message"`
	Progress float64 `json:"progress"`
	QR       string  `json:"qr"`
}

type UIState struct {
	Engine     EngineState         `json:"engine"`
	Settings   Settings            `json:"settings"`
	Links      map[string]LinkInfo `json:"links"`
	Camera     map[string]any      `json:"camera"`
	Hotkeys    map[string]string   `json:"hotkeyErrors"`
	Preview    string              `json:"preview"`
	Version    string              `json:"version"`
	PC         string              `json:"pc"`
	WindowMode bool                `json:"windowMode"`
	PhotoDir   string              `json:"photoDir"`
}

func (a *App) State() UIState {
	s := getSettings()
	host, _ := os.Hostname()
	st := UIState{
		Engine:     a.engine.State(),
		Settings:   s,
		Links:      a.links(s),
		Preview:    a.preview.Base() + "|" + a.preview.token,
		Version:    version,
		PC:         host,
		WindowMode: a.windowMode.Load(),
		PhotoDir:   photoDir(),
	}
	st.Settings.CFToken = "" // el token del túnel no hace falta en la ventana
	if s.CFToken != "" {
		st.Settings.CFToken = "•"
	}
	a.camMu.Lock()
	st.Camera = map[string]any{"registered": cameraRegistered(), "error": a.camErr}
	a.camMu.Unlock()
	if a.hotkeys != nil {
		st.Hotkeys = a.hotkeys.Errors()
	}
	return st
}

func (a *App) links(s Settings) map[string]LinkInfo {
	path := "/c/" + s.Token + "/"
	out := map[string]LinkInfo{}

	ts := a.tunnel.State()
	in := LinkInfo{Status: ts.Status, Message: ts.Message, Progress: ts.Progress}
	if ts.Mode == "domain" && ts.DomainErr == "" && ts.Status == "online" {
		in.Message = "Con tu dominio"
	} else if ts.DomainErr != "" {
		in.Message = ts.DomainErr
	}
	if ts.URL != "" && (ts.Status == "online" || ts.Status == "checking") {
		in.URL = strings.TrimSuffix(ts.URL, "/") + path
	}
	out["internet"] = in

	ds := a.direct.State()
	d := LinkInfo{Status: ds.Status, Message: ds.Message}
	if base := a.direct.Base(); base != "" {
		d.URL = base + path
	}
	out["direct"] = d

	lan := LinkInfo{Status: "off", Message: "No hay red local"}
	if ip := lanIP(); ip != "" {
		lan = LinkInfo{Status: "online", URL: fmt.Sprintf("https://%s:%d%s", ip, a.server.port, path)}
	}
	out["lan"] = lan

	for k, l := range out {
		if l.URL != "" {
			l.QR = a.qrFor(l.URL)
			out[k] = l
		}
	}
	return out
}

func (a *App) qrFor(url string) string {
	a.qrMu.Lock()
	defer a.qrMu.Unlock()
	if q, ok := a.qrCache[url]; ok {
		return q
	}
	code, err := qr.Encode(url, qr.M)
	if err != nil {
		return ""
	}
	code.Scale = 8
	q := "data:image/png;base64," + base64.StdEncoding.EncodeToString(code.PNG())
	if len(a.qrCache) > 16 {
		a.qrCache = map[string]string{}
	}
	a.qrCache[url] = q
	return q
}

// ---------- lo que pide la ventana ----------

func (a *App) GetState() UIState { return a.State() }

func (a *App) SetVisible(v bool) { a.engine.SetUIVisible(v) }

func (a *App) SetLink(mode string) {
	updateSettings(func(s *Settings) { s.Link = mode })
	a.changed()
}

func (a *App) SetOutput(resolution string, fps int) {
	updateSettings(func(s *Settings) { s.Resolution, s.FPS = resolution, fps })
	a.engine.SceneChanged()
}

func (a *App) SetIdle(on bool) {
	updateSettings(func(s *Settings) { s.Idle = on })
	a.engine.sendConfigs(false)
	a.changed()
}

func (a *App) SetUPnP(on bool) {
	updateSettings(func(s *Settings) { s.UPnP = on })
	go func() {
		a.direct.Stop()
		a.startNetwork(false, true)
	}()
	a.changed()
}

// SetDomain configura un túnel de Cloudflare con dominio propio (token = "" lo quita).
func (a *App) SetDomain(token, host string) {
	updateSettings(func(s *Settings) {
		if token != "•" { // «•» = no tocar el que ya hay
			s.CFToken = token
		}
		s.CFHost = host
		if s.CFToken == "" {
			s.CFHost = ""
		}
	})
	go func() {
		a.tunnel.Stop()
		a.startNetwork(true, false)
	}()
	a.changed()
}

func (a *App) SetScene(sc Scene) {
	updateSettings(func(s *Settings) { s.Scene = sc })
	a.engine.SceneChanged()
}

// ShowPhone pone ese móvil de protagonista.
func (a *App) ShowPhone(id string) {
	updateSettings(func(s *Settings) {
		s.Scene.Main = id
		if s.Scene.Kind == "screen" {
			s.Scene.Kind = "phone"
		}
		if s.Scene.Pip == id {
			s.Scene.Pip = ""
		}
	})
	a.engine.SceneChanged()
}

func (a *App) SetAdjust(id string, adj Adjust) { a.engine.SetAdjust(id, adj) }

func (a *App) RenamePhone(id, name string) {
	name = cleanName(name, 40)
	updateSettings(func(s *Settings) {
		if d := s.Devices[id]; d != nil {
			d.Name = name
		}
	})
	a.engine.sendConfigs(false)
	a.changed()
}

func (a *App) KickPhone(id string) { a.engine.Kick(id) }

// ForgetPhone echa al móvil y olvida sus ajustes.
func (a *App) ForgetPhone(id string) {
	a.engine.Kick(id)
	updateSettings(func(s *Settings) { delete(s.Devices, id) })
	a.changed()
}

// RenewLink cambia el secreto del enlace: los móviles que lo tuvieran ya no entran.
func (a *App) RenewLink() {
	updateSettings(func(s *Settings) { s.Token = randHex(16) })
	a.engine.KickAll()
	a.changed()
}

func (a *App) ToggleFreeze() bool {
	on := !a.engine.State().Frozen
	a.engine.SetFrozen(on)
	return on
}

func (a *App) ToggleCover() bool {
	on := !a.engine.State().Covered
	a.engine.SetCovered(on)
	return on
}

func (a *App) TakePhoto() (string, error) {
	p, err := a.engine.TakePhoto()
	if err != nil {
		return "", err
	}
	return p, nil
}

func (a *App) onHotkey(action string) {
	switch action {
	case "photo":
		if p, err := a.engine.TakePhoto(); err != nil {
			a.toast("error", "No se pudo hacer la foto: "+err.Error())
		} else {
			runtime.EventsEmit(a.ctx, "photo", p)
		}
	case "freeze":
		if a.ToggleFreeze() {
			a.toast("info", "Imagen congelada")
		} else {
			a.toast("info", "Imagen en directo otra vez")
		}
	case "cover":
		if a.ToggleCover() {
			a.toast("info", "Cámara tapada")
		} else {
			a.toast("info", "Cámara destapada")
		}
	}
}

func (a *App) SetHotkey(action, combo string) (string, error) {
	combo = strings.TrimSpace(combo)
	if combo != "" {
		if _, _, err := parseHotkey(combo); err != nil {
			return "", err
		}
	}
	updateSettings(func(s *Settings) {
		switch action {
		case "photo":
			s.Hotkeys.Photo = combo
		case "freeze":
			s.Hotkeys.Freeze = combo
		case "cover":
			s.Hotkeys.Cover = combo
		}
	})
	a.hotkeys.Reload()
	a.changed()
	return a.hotkeys.Errors()[action], nil
}

func (a *App) PickPhotoDir() string {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Carpeta para las fotos", DefaultDirectory: photoDir(), CanCreateDirectories: true,
	})
	if err != nil || dir == "" {
		return ""
	}
	updateSettings(func(s *Settings) { s.PhotoDir = dir })
	a.changed()
	return dir
}

func (a *App) OpenPhotos() {
	os.MkdirAll(photoDir(), 0o755)
	openExplorer(photoDir(), false)
}

func (a *App) RevealFile(path string) {
	if !strings.HasPrefix(strings.ToLower(filepath.Clean(path)), strings.ToLower(filepath.Clean(photoDir()))) {
		return // solo fotos de TurboCAM
	}
	openExplorer(path, true)
}

func openExplorer(path string, sel bool) {
	cmd := exec.Command("explorer.exe")
	arg := `"` + path + `"`
	if sel {
		arg = `/select,"` + path + `"`
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: "explorer.exe " + arg}
	if cmd.Start() == nil {
		go cmd.Wait()
	}
}

func (a *App) Copy(text string) { runtime.ClipboardSetText(a.ctx, text) }

// ---------- perfiles ----------

// SaveProfile guarda lo que hay ahora (escena y retoques del móvil principal).
func (a *App) SaveProfile(name string) Profile {
	s := getSettings()
	p := Profile{ID: randHex(6), Name: cleanName(name, 40), Scene: s.Scene, Adjust: defaultAdjust()}
	a.engine.mu.Lock()
	main := a.engine.resolveLocked(s.Scene.Main)
	a.engine.mu.Unlock()
	if d := s.Devices[main]; d != nil {
		p.Adjust = d.Adjust
	}
	updateSettings(func(s *Settings) { s.Profiles = append(s.Profiles, p) })
	a.changed()
	return p
}

// UpdateProfile cambia nombre y apps (lo demás se guarda con OverwriteProfile).
func (a *App) UpdateProfile(id, name string, apps []string) {
	updateSettings(func(s *Settings) {
		for i := range s.Profiles {
			if s.Profiles[i].ID == id {
				s.Profiles[i].Name = cleanName(name, 40)
				s.Profiles[i].Apps = apps
			}
		}
	})
	a.engine.autoProfiles(a.engine.State().Consumers)
	a.changed()
}

// OverwriteProfile le pone a un perfil la escena y los retoques de ahora.
func (a *App) OverwriteProfile(id string) {
	cur := a.SaveProfile("tmp")
	updateSettings(func(s *Settings) {
		var keep []Profile
		for _, p := range s.Profiles {
			if p.ID == cur.ID {
				continue
			}
			if p.ID == id {
				p.Scene, p.Adjust = cur.Scene, cur.Adjust
			}
			keep = append(keep, p)
		}
		s.Profiles = keep
	})
	a.changed()
}

func (a *App) DeleteProfile(id string) {
	updateSettings(func(s *Settings) {
		var keep []Profile
		for _, p := range s.Profiles {
			if p.ID != id {
				keep = append(keep, p)
			}
		}
		s.Profiles = keep
	})
	a.changed()
}

func (a *App) ApplyProfile(id string) error {
	p := findProfile(getSettings(), id)
	if p == nil {
		return errors.New("ese perfil ya no existe")
	}
	a.engine.applyProfile(*p)
	return nil
}

// ---------- pantalla ----------

func (a *App) Monitors() []Monitor { return listMonitors() }

func (a *App) MonitorShot(index int) (string, error) {
	for _, m := range listMonitors() {
		if m.Index == index {
			return monitorShot(m)
		}
	}
	return "", errors.New("esa pantalla ya no está")
}

// ---------- cámara virtual ----------

func (a *App) ReinstallCamera() error {
	if err := installCamera(); err != nil {
		a.setCamErr(err.Error())
		return err
	}
	a.setCamErr("")
	return nil
}

// ---------- ventana ----------

// SetWindowMode convierte la ventana en un visor siempre encima, para compartirla
// en una videollamada sin usar la cámara virtual.
func (a *App) SetWindowMode(on bool) {
	if a.windowMode.Load() == on {
		return
	}
	a.windowMode.Store(on)
	a.engine.SetWindowMode(on)
	if on {
		a.savedW, a.savedH = runtime.WindowGetSize(a.ctx)
		runtime.WindowSetMinSize(a.ctx, 320, 180)
		st := a.engine.State()
		w := 640
		h := w * st.OutH / max(st.OutW, 1)
		runtime.WindowUnmaximise(a.ctx)
		runtime.WindowSetSize(a.ctx, w, h)
		runtime.WindowSetAlwaysOnTop(a.ctx, getSettings().OnTop)
	} else {
		runtime.WindowSetAlwaysOnTop(a.ctx, false)
		runtime.WindowSetMinSize(a.ctx, minW, minH)
		if a.savedW > 0 {
			runtime.WindowSetSize(a.ctx, a.savedW, a.savedH)
		}
		runtime.WindowCenter(a.ctx)
	}
	a.changed()
}

func (a *App) SetOnTop(on bool) {
	updateSettings(func(s *Settings) { s.OnTop = on })
	if a.windowMode.Load() {
		runtime.WindowSetAlwaysOnTop(a.ctx, on)
	}
	a.changed()
}

func (a *App) SetWelcomed() {
	updateSettings(func(s *Settings) { s.Welcomed = true })
	a.changed()
}

// Quit cierra de verdad (después de preguntar en la ventana).
func (a *App) Quit() {
	a.allowClose.Store(true)
	runtime.Quit(a.ctx)
}
