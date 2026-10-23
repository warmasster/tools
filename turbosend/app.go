package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx context.Context
	dir string

	mu         sync.RWMutex
	settings   Settings
	port       int
	srvErr     string
	activeRoom string

	shares  *ShareStore
	rooms   *RoomStore
	hub     *Hub
	tracker *Tracker
	server  *PublicServer
	tunnel  *Tunnel
	direct  *Direct
	control *http.Server
}

func NewApp() *App {
	a := &App{dir: dataDir()}
	a.settings = defaultSettings()
	_ = loadJSON(filepath.Join(a.dir, "settings.json"), &a.settings)
	a.settings.normalize()
	a.shares = newShareStore(a.dir)
	a.rooms = newRoomStore(a.dir)
	a.hub = newHub()
	a.tracker = newTracker(func(t *Transfer) {
		if t.Dir != "in" {
			a.shares.IncDownloads(t.ShareID)
			if sh := a.shares.Get(t.ShareID); sh != nil && sh.RoomID != "" {
				a.rooms.With(sh.RoomID, func(r *Room) bool { return true })
				who := t.Who
				if who == "" {
					who = "Alguien"
				}
				a.notify(sh.RoomID, "out-done", who+" ha descargado "+sh.Name)
			}
		}
	})
	a.server = newPublicServer(a)
	a.tunnel = newTunnel(a.dir, a.server.nonce, nil)
	a.direct = newDirect(a.server.nonce, nil)
	a.rooms.onChange = func(id string) { a.emit("roomchg", id) }
	a.hub.onPresence = func(id string) { a.membersChanged(id) }
	a.importV1()
	return a
}

func (a *App) emit(name string, data ...any) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, name, data...)
	}
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
	a.startControl()

	runtime.OnFileDrop(ctx, func(x, y int, paths []string) {
		a.mu.RLock()
		room := a.activeRoom
		a.mu.RUnlock()
		go a.sendPaths(room, paths)
	})
	if args := pathArgs(os.Args[1:]); len(args) > 0 {
		go func() {
			time.Sleep(800 * time.Millisecond)
			a.sendPaths("", args)
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
				a.emit("state", a.GetState())
				a.broadcastProgress()
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
		a.tunnel.Start(port, s.TunnelToken, s.TunnelHost)
	}
	a.direct.Start(port, s.UPnP, s.ForceDirect)
}

func (a *App) shutdown(ctx context.Context) {
	a.stopControl()
	a.rooms.Flush()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); a.direct.Stop() }()
	go func() { defer wg.Done(); a.tunnel.Stop() }()
	wg.Wait()
	a.server.Close()
}

func (a *App) beforeClose(ctx context.Context) bool {
	if a.tracker.ActiveCount() == 0 {
		return false
	}
	res, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "Hay transferencias en curso",
		Message:       "Si cierras TurboSEND, las transferencias activas se cortarán (se podrán reanudar luego). ¿Cerrar igualmente?",
		Buttons:       []string{"Yes", "No"},
		DefaultButton: "No",
	})
	return err == nil && res != "Yes"
}

func (a *App) onSecondInstance(d options.SecondInstanceData) {
	if a.ctx == nil {
		return
	}
	args := pathArgs(d.Args)
	if len(args) == len(d.Args) || len(args) > 0 {
		// --background (lo usa TurboMENU) no debe sacar la ventana delante.
		runtime.WindowUnminimise(a.ctx)
		runtime.WindowShow(a.ctx)
	}
	if len(args) > 0 {
		go a.sendPaths("", args)
	}
}

// pathArgs quita de la línea de órdenes las opciones (--background…) y deja las rutas.
func pathArgs(in []string) []string {
	var out []string
	for _, a := range in {
		if !strings.HasPrefix(a, "--") {
			out = append(out, a)
		}
	}
	return out
}

func startMinimised() bool {
	for _, a := range os.Args[1:] {
		if a == "--background" {
			return true
		}
	}
	return false
}

// importV1 convierte los envíos de la versión 1 en chats (solo la primera vez).
func (a *App) importV1() {
	flag := filepath.Join(a.dir, "v1-imported")
	if _, err := os.Stat(flag); err == nil {
		return
	}
	os.WriteFile(flag, []byte("1"), 0o644)
	base, err := os.UserConfigDir()
	if err != nil {
		return
	}
	var old []*Share
	if loadJSON(filepath.Join(base, "Enviador", "shares.json"), &old) != nil {
		return
	}
	for _, sh := range old {
		if sh == nil || len(sh.Files) == 0 {
			continue
		}
		r := a.rooms.Create(sh.Name)
		sh.ID = newID()
		sh.RoomID = r.ID
		a.shares.Add(sh)
		m := newMessage("host", "out")
		m.Bundle = sh.ID
		m.Time = sh.Created * 1000
		a.rooms.Post(r.ID, m, false)
	}
}

// ---------- mensajería interna ----------

// post guarda un mensaje y lo reparte a todos.
func (a *App) post(roomID string, m *Message, fromGuest bool) Message {
	cp, ok := a.rooms.Post(roomID, m, fromGuest)
	if !ok {
		return cp
	}
	a.hub.Broadcast(roomID, "msg", cp)
	if cp.Kind == "out" {
		a.hub.Broadcast(roomID, "out", a.outView(cp.Bundle))
	}
	if cp.Kind == "in" {
		a.bundleChanged(roomID, cp.Bundle)
	}
	if fromGuest {
		name := a.rooms.MemberName(roomID, cp.From)
		text := cp.Text
		if cp.Kind == "in" {
			text = "quiere enviarte archivos"
		}
		a.notify(roomID, "msg", name+": "+text)
	}
	return cp
}

func (a *App) notice(roomID, text string) {
	m := newMessage("system", "system")
	m.Text = text
	a.post(roomID, m, false)
	a.membersChanged(roomID)
}

func (a *App) notify(roomID, kind, text string) {
	a.emit("notify", map[string]string{"room": roomID, "kind": kind, "text": text})
}

func (a *App) bundleChanged(roomID, bid string) {
	var v InView
	var found bool
	a.rooms.With(roomID, func(r *Room) bool {
		if b := r.inBundle(bid); b != nil {
			v, found = inView(b, true, ""), true
		}
		return false
	})
	if found {
		// cada invitado recibe la lista de archivos solo de lo suyo
		a.hub.Broadcast(roomID, "in", v)
	}
	a.emit("roomchg", roomID)
}

func (a *App) membersChanged(roomID string) {
	v, ok := a.roomView(roomID, true, "")
	if ok {
		a.hub.Broadcast(roomID, "members", v.Members)
	}
	a.emit("roomchg", roomID)
}

func (a *App) typingChanged(roomID string) {
	a.hub.Broadcast(roomID, "typing", a.typingWho(roomID))
	a.emit("typing", map[string]any{"room": roomID, "who": a.typingNames(roomID, false, "")})
}

// typingWho devuelve quién escribe como lista de {pid, name} para los navegadores.
func (a *App) typingWho(roomID string) []map[string]string {
	out := []map[string]string{}
	host := a.getSettings().HostName
	for _, who := range a.hub.Typing(roomID) {
		if who == "host" {
			out = append(out, map[string]string{"pid": "host", "name": host})
		} else if n := a.rooms.MemberName(roomID, who); n != "" {
			out = append(out, map[string]string{"pid": who, "name": n})
		}
	}
	return out
}

// broadcastProgress manda a los navegadores el progreso de los envíos entrantes activos.
func (a *App) broadcastProgress() {
	list, _, _ := a.tracker.Snapshot()
	for _, t := range list {
		if t.Dir == "in" && t.Status == "active" && t.Room != "" {
			a.hub.Broadcast(t.Room, "progress", map[string]any{"bundle": t.ShareID, "bytes": t.Bytes, "total": t.Total, "speed": t.Speed})
		}
	}
}

// resumeUploadsFor pide a un invitado que acaba de conectarse que siga con lo aceptado.
func (a *App) resumeUploadsFor(roomID, pid string) {
	var ids []string
	a.rooms.With(roomID, func(r *Room) bool {
		for _, b := range r.In {
			if b.From == pid && (b.Status == "accepted" || b.Status == "transferring") {
				ids = append(ids, b.ID)
			}
		}
		return false
	})
	for _, id := range ids {
		a.hub.SendTo(roomID, pid, "upload", map[string]string{"bundle": id})
	}
}

// sendPaths crea un envío en un chat (o en uno nuevo si room está vacío).
func (a *App) sendPaths(room string, paths []string) {
	a.emit("busy", true)
	defer a.emit("busy", false)
	if room == "" || !a.rooms.Exists(room) {
		r := a.rooms.Create("Chat nuevo")
		room = r.ID
		defer a.emit("open", room)
	}
	sh, err := a.shares.Create(paths, room)
	if err != nil {
		a.emit("toast", map[string]string{"kind": "error", "text": err.Error()})
		return
	}
	a.rooms.With(room, func(r *Room) bool {
		if !r.NameSet && r.Name == "Chat nuevo" && len(r.Members) == 0 {
			r.Name = sh.Name
		}
		return true
	})
	m := newMessage("host", "out")
	m.Bundle = sh.ID
	a.post(room, m, false)
}

// ---------- estado global para la interfaz ----------

type ShareLinks struct {
	Cloudflare string `json:"cloudflare"`
	Direct     string `json:"direct"`
	Lan        string `json:"lan"`
}

type RoomSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Updated  int64  `json:"updated"`
	Unread   int    `json:"unread"`
	Preview  string `json:"preview"`
	Online   int    `json:"online"`
	Members  int    `json:"members"`
	Pending  int    `json:"pending"`
	LastFrom string `json:"lastFrom"`
}

type State struct {
	Port      int            `json:"port"`
	ServerErr string         `json:"serverErr"`
	Tunnel    TunnelState    `json:"tunnel"`
	Direct    DirectState    `json:"direct"`
	Lan       string         `json:"lan"`
	Rooms     []RoomSummary  `json:"rooms"`
	Transfers []TransferView `json:"transfers"`
	Speed     float64        `json:"speed"`
	History   []float64      `json:"history"`
	SpeedIn   float64        `json:"speedIn"`
	HistoryIn []float64      `json:"historyIn"`
	Settings  Settings       `json:"settings"`
	DataDir   string         `json:"dataDir"`
}

func (a *App) bases() (cf, direct, lan string) {
	a.mu.RLock()
	port := a.port
	a.mu.RUnlock()
	ts := a.tunnel.State()
	if ts.Status == "online" {
		cf = ts.URL
	}
	direct = a.direct.Base()
	ds := a.direct.State()
	ip := ds.LocalIP
	if ip == "" {
		ip = lanIP()
	}
	if ip != "" {
		lan = fmt.Sprintf("http://%s:%d", ip, port)
	}
	return
}

func (a *App) GetState() State {
	a.mu.RLock()
	port, srvErr, set := a.port, a.srvErr, a.settings
	a.mu.RUnlock()
	ts := a.tunnel.State()
	ts.Log = nil
	_, _, lan := a.bases()
	list, speed, hist := a.tracker.Snapshot()
	sin, hin := a.tracker.SpeedIn()
	st := State{Port: port, ServerErr: srvErr, Tunnel: ts, Direct: a.direct.State(), Lan: lan,
		Transfers: list, Speed: speed, History: hist, SpeedIn: sin, HistoryIn: hin, Settings: set, DataDir: a.dir}
	if st.Transfers == nil {
		st.Transfers = []TransferView{}
	}
	st.Rooms = []RoomSummary{}
	for _, id := range a.rooms.IDs() {
		online := len(a.hub.Online(id))
		var sum RoomSummary
		var outID string
		a.rooms.With(id, func(r *Room) bool {
			sum = RoomSummary{ID: r.ID, Name: r.Name, Updated: r.Updated, Unread: r.Unread, Online: online, Members: len(r.Members)}
			for _, b := range r.In {
				if b.Status == "offered" {
					sum.Pending++
				}
			}
			if n := len(r.Messages); n > 0 {
				m := r.Messages[n-1]
				sum.LastFrom = m.From
				switch m.Kind {
				case "text", "system":
					sum.Preview = m.Text
				case "in":
					if b := r.inBundle(m.Bundle); b != nil {
						sum.Preview = "📎 " + b.Name
					}
				case "out":
					outID = m.Bundle
				}
				if m.From != "host" && m.From != "system" {
					if mm := r.member(m.From); mm != nil {
						sum.Preview = mm.Name + ": " + sum.Preview
					}
				}
			}
			return false
		})
		if outID != "" {
			sum.Preview = "Tú: 📎 " + a.outView(outID).Name
		}
		st.Rooms = append(st.Rooms, sum)
	}
	return st
}

// ---------- métodos llamados desde la interfaz ----------

func (a *App) GetRoom(id string) (RoomView, error) {
	v, ok := a.roomView(id, false, "")
	if !ok {
		return v, fmt.Errorf("chat no encontrado")
	}
	cf, direct, lan := a.bases()
	v.Links = &ShareLinks{}
	if cf != "" {
		v.Links.Cloudflare = cf + "/r/" + id
	}
	if direct != "" {
		v.Links.Direct = direct + "/r/" + id
	}
	if lan != "" {
		v.Links.Lan = lan + "/r/" + id
	}
	v.Typing = a.typingNames(id, false, "")
	return v, nil
}

func (a *App) SetActiveRoom(id string) {
	a.mu.Lock()
	a.activeRoom = id
	a.mu.Unlock()
	if id != "" {
		a.MarkRead(id)
	}
}

func (a *App) MarkRead(id string) {
	a.rooms.With(id, func(r *Room) bool {
		if r.Unread == 0 {
			return false
		}
		r.Unread = 0
		return true
	})
}

func (a *App) NewRoom() string {
	r := a.rooms.Create("Chat nuevo")
	return r.ID
}

func (a *App) RenameRoom(id, name string) {
	name = cleanName(name)
	if name == "" {
		return
	}
	a.rooms.With(id, func(r *Room) bool {
		r.Name, r.NameSet = name, true
		return true
	})
	a.hub.Broadcast(id, "room", map[string]string{"name": name})
}

func (a *App) DeleteRoom(id string) {
	a.hub.Disconnect(id)
	a.shares.DeleteRoom(id)
	a.rooms.Delete(id)
}

func (a *App) SetRoomAuto(id, mode string) {
	if mode != "on" && mode != "off" {
		mode = ""
	}
	a.rooms.With(id, func(r *Room) bool { r.Auto = mode; return true })
}

func (a *App) SendText(id, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if len([]rune(text)) > 4000 {
		text = string([]rune(text)[:4000])
	}
	a.hub.ClearTyping(id, "host")
	m := newMessage("host", "text")
	m.Text = text
	a.post(id, m, false)
	return nil
}

func (a *App) Typing(id string) {
	a.hub.SetTyping(id, "host")
	a.hub.Broadcast(id, "typing", a.typingWho(id))
}

func (a *App) PickFiles(room string) error {
	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{Title: "Elige los archivos que quieres enviar"})
	if err != nil || len(paths) == 0 {
		return err
	}
	go a.sendPaths(room, paths)
	return nil
}

func (a *App) PickFolder(room string) error {
	p, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Elige la carpeta que quieres enviar"})
	if err != nil || p == "" {
		return err
	}
	go a.sendPaths(room, []string{p})
	return nil
}

func (a *App) DeleteBundle(room, bundleID string) {
	a.shares.Delete(bundleID)
	a.hub.Broadcast(room, "out", a.outView(bundleID))
	a.emit("roomchg", room)
}

func (a *App) AcceptBundle(room, bid string) error { return a.AcceptIn(room, bid) }
func (a *App) RejectBundle(room, bid string)       { a.RejectIn(room, bid) }
func (a *App) CancelBundle(room, bid string)       { a.CancelIn(room, bid) }

func (a *App) inRoot(room, bid string) string {
	root := ""
	a.rooms.With(room, func(r *Room) bool {
		if b := r.inBundle(bid); b != nil {
			root = b.Root
			if b.Kind == "file" && len(b.Files) == 1 && b.Files[0].Final != "" {
				root = b.Files[0].Final
			}
		}
		return false
	})
	return root
}

// OpenBundle abre lo recibido (el archivo con su programa, o la carpeta).
func (a *App) OpenBundle(room, bid string) {
	if p := a.inRoot(room, bid); p != "" {
		explorer(`"` + p + `"`)
	}
}

func (a *App) RevealBundle(room, bid string) {
	if p := a.inRoot(room, bid); p != "" {
		explorer(`/select,"` + p + `"`)
	}
}

func (a *App) RevealOut(bundleID string) {
	if sh := a.shares.Get(bundleID); sh != nil && len(sh.Sources) > 0 {
		explorer(`/select,"` + sh.Sources[0] + `"`)
	}
}

func (a *App) CopyText(text string) error { return runtime.ClipboardSetText(a.ctx, text) }

func (a *App) OpenURL(u string) { runtime.BrowserOpenURL(a.ctx, u) }

func explorer(args string) {
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: "explorer.exe " + args}
	_ = cmd.Start()
	go cmd.Wait()
}

func (a *App) OpenDataDir() { explorer(`"` + a.dir + `"`) }

func (a *App) OpenRecvDir() {
	d := a.getSettings().RecvDir
	os.MkdirAll(d, 0o755)
	explorer(`"` + d + `"`)
}

func (a *App) PickRecvDir() string {
	p, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "¿Dónde guardo lo que te envíen?", DefaultDirectory: a.getSettings().RecvDir})
	if err != nil {
		return ""
	}
	return p
}

func (a *App) TunnelLog() []string { return a.tunnel.State().Log }

func (a *App) RestartTunnel() {
	a.mu.RLock()
	port, s := a.port, a.settings
	a.mu.RUnlock()
	if s.Tunnel && port > 0 {
		go a.tunnel.Start(port, s.TunnelToken, s.TunnelHost)
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

func (a *App) FlashTaskbar() { flashWindow("TurboSEND") }

func (a *App) SetTitle(t string) { runtime.WindowSetTitle(a.ctx, t) }

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
		tunnelChanged := s.Tunnel != old.Tunnel || s.TunnelToken != old.TunnelToken || s.TunnelHost != old.TunnelHost
		if tunnelChanged {
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
