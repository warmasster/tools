package main

// El Workshop se maneja desde un proceso aparte (TurboGAMES.exe --workshop
// <appid> <carpeta de Steam>) que Steam toma por el juego: así se le puede pedir
// que suscriba o quite mods, igual que lo hace el juego desde su menú. Ese
// proceso no abre el juego ni toca sus archivos. Solo se abre para lo que lo
// necesita (leer o cambiar las suscripciones, buscar) y TurboGAMES lo cierra a
// los dos segundos de la última petición: mientras vive, Steam da el juego por
// abierto (los amigos ven «jugando a…» y Steam no deja jugarlo de verdad).
//
// Se habla con él por su entrada y salida estándar, una línea JSON por mensaje.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"turbogames/internal/steam"
	"turbogames/internal/steamworks"
)

// Cuánto sigue vivo el proceso sin peticiones si TurboGAMES no lo cierra antes
// (por si acaso: normalmente lo cierra a los wsLinger).
const wsIdle = 10 * time.Second

type wsRequest struct {
	ID     int      `json:"id"`
	Op     string   `json:"op"`
	IDs    []uint64 `json:"ids,omitempty"`
	Text   string   `json:"text,omitempty"`
	Sort   int      `json:"sort,omitempty"`
	Cursor string   `json:"cursor,omitempty"`
	Tags   []string `json:"tags,omitempty"`
	Excl   []string `json:"excl,omitempty"`
	Days   int      `json:"days,omitempty"`
}

type wsReply struct {
	ID       int             `json:"id"`
	Error    string          `json:"error,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
	Progress *wsProgress     `json:"progress,omitempty"` // un mod de una tanda (no es la respuesta final)
}

// wsProgress es un mod de una tanda que ya ha terminado.
type wsProgress struct {
	ID    uint64 `json:"id"`
	Error string `json:"error,omitempty"`
}

// ---------- el proceso aparte ----------

// workshopChild es lo que hace TurboGAMES.exe --workshop <appid> <carpeta>.
func workshopChild(args []string) int {
	runtime.LockOSThread() // steamclient no es para varios hilos a la vez
	out := json.NewEncoder(os.Stdout)
	fail := func(err error) int { out.Encode(wsReply{Error: err.Error()}); return 1 }
	if len(args) < 2 {
		return fail(errors.New("faltan datos"))
	}
	id, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		return fail(errors.New("juego no válido"))
	}
	c, err := openWSClient(args[1], uint32(id))
	if err != nil {
		return fail(err)
	}
	defer c.Close()
	out.Encode(wsReply{}) // listo

	lines := make(chan []byte)
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		sc.Buffer(make([]byte, 1<<20), 8<<20)
		for sc.Scan() {
			lines <- append([]byte(nil), sc.Bytes()...)
		}
		close(lines)
	}()
	idle := time.NewTimer(wsIdle)
	for {
		select {
		case <-idle.C:
			return 0
		case l, ok := <-lines:
			if !ok {
				return 0
			}
			var req wsRequest
			if json.Unmarshal(l, &req) != nil {
				continue
			}
			if req.Op == "subscribeMany" || req.Op == "unsubscribeMany" {
				stopped := false
				c.Batch(req.Op == "subscribeMany", req.IDs, steamworks.BatchWindow, func(id uint64, err error) {
					p := &wsProgress{ID: id}
					if err != nil {
						p.Error = err.Error()
					}
					out.Encode(wsReply{ID: req.ID, Progress: p})
				}, func() bool {
					// «parar» llega por la misma entrada mientras va la tanda
					for !stopped {
						select {
						case l2, ok := <-lines:
							if !ok {
								stopped = true
								break
							}
							var r2 wsRequest
							if json.Unmarshal(l2, &r2) == nil && r2.Op == "cancel" {
								stopped = true
							}
							continue
						default:
						}
						break
					}
					return stopped
				})
				out.Encode(wsReply{ID: req.ID, Data: json.RawMessage("true")})
				idle.Reset(wsIdle)
				continue
			}
			if req.Op == "cancel" {
				continue
			}
			rep := wsReply{ID: req.ID}
			data, err := wsHandle(c, req)
			if err != nil {
				rep.Error = err.Error()
			} else {
				rep.Data, _ = json.Marshal(data)
			}
			out.Encode(rep)
			idle.Reset(wsIdle)
		}
	}
}

// wsClient es lo que hace el proceso del Workshop con Steam (las pruebas lo
// cambian por uno de mentira).
type wsClient interface {
	Close()
	Subscribed() []uint64
	State(id uint64) steamworks.ItemState
	Details(ids []uint64) ([]steamworks.Item, error)
	Subscribe(id uint64) error
	Unsubscribe(id uint64) error
	SearchEx(o steamworks.SearchOptions) (steamworks.Page, error)
	Batch(sub bool, ids []uint64, window int, done func(id uint64, err error), stop func() bool)
}

var openWSClient = func(steamDir string, appid uint32) (wsClient, error) {
	c, err := steamworks.Open(steamDir, appid, steam.SignedByValve)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func wsHandle(c wsClient, r wsRequest) (any, error) {
	switch r.Op {
	case "ping":
		return true, nil
	case "subscribed":
		ids := c.Subscribed()
		st := make([]steamworks.ItemState, 0, len(ids))
		for _, id := range ids {
			st = append(st, c.State(id))
		}
		return st, nil
	case "states":
		st := make([]steamworks.ItemState, 0, len(r.IDs))
		for _, id := range r.IDs {
			st = append(st, c.State(id))
		}
		return st, nil
	case "details":
		return c.Details(r.IDs)
	case "subscribe", "unsubscribe":
		if len(r.IDs) != 1 {
			return nil, errors.New("un mod cada vez")
		}
		var err error
		if r.Op == "subscribe" {
			err = c.Subscribe(r.IDs[0])
		} else {
			err = c.Unsubscribe(r.IDs[0])
		}
		return err == nil, err
	case "search":
		return c.SearchEx(steamworks.SearchOptions{Text: r.Text, Sort: r.Sort, Days: r.Days, Cursor: r.Cursor, Tags: r.Tags, Exclude: r.Excl})
	}
	return nil, fmt.Errorf("orden desconocida %q", r.Op)
}

// ---------- desde TurboGAMES ----------

// Cuánto se deja abierta la conexión después de la última petición, por si
// llega otra seguida (la lista justo después de cambiarla, el estado de lo que
// sale en una búsqueda…). Después se cierra sola: mientras está abierta, Steam
// da el juego por abierto («jugando a…») y no deja jugarlo de verdad.
const wsLinger = 2 * time.Second

// Cuánto tarda Steam como mucho en darse cuenta de que nuestro proceso se ha
// cerrado (mientras, sigue dando el juego por abierto).
const wsSteamLag = 20 * time.Second

type wsProc struct {
	appid   uint32
	user    uint32 // cuenta que estaba dentro al abrirlo
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	replies chan wsReply
	next    int
	exited  atomic.Bool
}

type wsHost struct {
	mu    sync.Mutex // una petición cada vez
	p     *wsProc
	timer *time.Timer // cierra el proceso al poco de la última petición
	gen   int         // cambia con cada petición: el cierre programado solo vale si no ha llegado otra
	hold  int         // mientras haya un cambio de mods en marcha no se cierra entre petición y petición

	smu sync.Mutex
	// juegos que Steam puede dar por abiertos por culpa nuestra, con cuándo se
	// cerró su proceso (cero: sigue abierto)
	own map[uint32]time.Time
}

// ownRunning dice si el juego que Steam da por abierto es nuestro proceso (o lo
// era hace nada y Steam aún no se ha dado cuenta de que se ha cerrado).
func (h *wsHost) ownRunning(appid uint32) bool {
	h.smu.Lock()
	defer h.smu.Unlock()
	t, ok := h.own[appid]
	return appid != 0 && ok && (t.IsZero() || time.Since(t) < wsSteamLag)
}

// settle se llama con el juego que Steam da por abierto ahora: los nuestros que
// ya se cerraron y Steam ya no ve se olvidan, así que si después Steam vuelve a
// verlos abiertos, es que se ha abierto el juego de verdad.
func (h *wsHost) settle(running uint32) {
	h.smu.Lock()
	defer h.smu.Unlock()
	for id, t := range h.own {
		if !t.IsZero() && id != running {
			delete(h.own, id)
		}
	}
}

func (h *wsHost) mark(appid uint32, alive bool) {
	h.smu.Lock()
	defer h.smu.Unlock()
	if h.own == nil {
		h.own = map[uint32]time.Time{}
	}
	if alive {
		h.own[appid] = time.Time{}
	} else if t, ok := h.own[appid]; ok && t.IsZero() {
		h.own[appid] = time.Now()
	}
}

// forgetOwn olvida que el juego abierto pueda ser nuestro (cuando se va a abrir
// el juego de verdad: a partir de ahí, si Steam lo da por abierto, lo está).
func (h *wsHost) forgetOwn() {
	h.smu.Lock()
	h.own = nil
	h.smu.Unlock()
}

func (h *wsHost) stopLocked() {
	if h.timer != nil {
		h.timer.Stop()
		h.timer = nil
	}
	p := h.p
	if p == nil {
		return
	}
	h.p = nil
	p.stdin.Close()
	done := make(chan struct{})
	go func() { p.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		p.cmd.Process.Kill()
	}
	h.mark(p.appid, false)
}

// busyLocked anula el cierre programado (llega una petición).
func (h *wsHost) busyLocked() {
	h.gen++
	if h.timer != nil {
		h.timer.Stop()
		h.timer = nil
	}
}

// lingerLocked programa el cierre del proceso para dentro de wsLinger, si no
// llega otra petición antes.
func (h *wsHost) lingerLocked() {
	if h.p == nil || h.hold > 0 {
		return
	}
	gen := h.gen
	h.timer = time.AfterFunc(wsLinger, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.gen == gen && h.hold == 0 {
			h.stopLocked()
		}
	})
}

// wsKeep deja la conexión abierta entre petición y petición hasta que se llame
// a lo que devuelve (para un cambio de mods, que son varias seguidas); después
// se cierra al poco, como siempre.
func (a *App) wsKeep() func() {
	h := &a.ws
	h.mu.Lock()
	h.hold++
	h.mu.Unlock()
	return func() {
		h.mu.Lock()
		h.hold--
		h.busyLocked()
		h.lingerLocked()
		h.mu.Unlock()
	}
}

// WorkshopDisconnect cierra la conexión con Steam (la interfaz lo pide al
// salir de la ventana, para que Steam no crea que el juego sigue abierto y
// deje jugarlo). No hace nada si se están cambiando mods.
func (a *App) WorkshopDisconnect() bool {
	if !a.modsMu.TryLock() {
		return false
	}
	defer a.modsMu.Unlock()
	a.stopWorkshop()
	return true
}

// stopWorkshop cierra el proceso del Workshop (antes de abrir un juego o cambiar
// de cuenta: Steam no debe creer que ya hay un juego abierto).
func (a *App) stopWorkshop() {
	a.ws.mu.Lock()
	a.ws.stopLocked()
	a.ws.mu.Unlock()
}

// releaseSteam cierra el proceso del Workshop y, si Steam aún da el juego por
// abierto por su culpa, espera a que lo suelte (unos segundos como mucho):
// mientras lo cree abierto, Steam no deja abrirlo de verdad.
func (a *App) releaseSteam(in *steam.Install) {
	a.stopWorkshop()
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(250 * time.Millisecond) {
		st := in.ReadStatus()
		a.ws.settle(st.RunningApp)
		if !st.Running || !a.ws.ownRunning(st.RunningApp) {
			break
		}
	}
	a.ws.forgetOwn()
}

// wsLink es el aviso "workshop-link": si ahora mismo hay conexión con Steam
// (mientras la hay, Steam enseña «jugando a…» ese juego).
type wsLink struct {
	AppID uint32 `json:"appid"`
	On    bool   `json:"on"`
}

// errGameOpen: Steam tiene abierto un juego (que no es nuestro proceso) y así
// no se conecta: Steam lo tomaría como abrir otro juego a la vez, o el mismo
// dos veces.
type errGameOpen struct {
	AppID uint32 // el que está abierto
	Name  string
	For   string // el juego cuyos mods se querían cambiar, si es otro
}

func (e *errGameOpen) Error() string {
	if e.For == "" {
		return "Tienes " + e.Name + " abierto: ciérralo para gestionar sus mods desde aquí"
	}
	return "Tienes " + e.Name + " abierto: ciérralo para gestionar desde aquí los mods de " + e.For + " (Steam no deja tener dos juegos abiertos a la vez)"
}

func (a *App) gameOpenErr(open, want uint32) error {
	e := &errGameOpen{AppID: open, Name: firstNonEmpty(a.GameName(open), "un juego")}
	if open != want {
		e.For = firstNonEmpty(a.GameName(want), "este juego")
	}
	return e
}

// errNotLinked: no hay conexión abierta (para lo que solo se pide de paso).
var errNotLinked = errors.New("sin conexión con Steam")

// wsOpts son los extras de una petición al proceso del Workshop.
type wsOpts struct {
	progress func(wsProgress) // cada mod de una tanda que termina (y el plazo vuelve a empezar)
	cancel   func() bool      // si dice que pare, se le pide al proceso
	ifLinked bool             // solo si ya hay conexión con ese juego: ni conecta ni la alarga
}

// wsCall manda una petición al proceso del Workshop de ese juego (lo abre si
// hace falta) y espera su respuesta. Al poco de la última petición el proceso
// se cierra solo.
func (a *App) wsCall(appid uint32, req wsRequest, timeout time.Duration, out any) error {
	return a.wsDo(appid, req, timeout, out, wsOpts{})
}

// wsPeek es wsCall solo si ya hay conexión con ese juego (lo que se pide de
// paso, como los datos de los mods en segundo plano, no conecta por su cuenta).
func (a *App) wsPeek(appid uint32, req wsRequest, timeout time.Duration, out any) error {
	return a.wsDo(appid, req, timeout, out, wsOpts{ifLinked: true})
}

func (a *App) wsDo(appid uint32, req wsRequest, timeout time.Duration, out any, o wsOpts) error {
	in := a.steam()
	if in == nil {
		return steam.ErrNotFound
	}
	h := &a.ws
	if o.ifLinked {
		h.mu.Lock()
		defer h.mu.Unlock()
		if p := h.p; p == nil || p.appid != appid || p.exited.Load() {
			return errNotLinked
		}
		return a.wsSendLocked(req, timeout, out, o)
	}
	if a.busySwitching() {
		return errors.New("Espera a que termine el cambio de cuenta")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.busyLocked()
	defer h.lingerLocked()
	st := in.ReadStatus()
	h.settle(st.RunningApp)
	if !st.Running || !in.Running() {
		h.stopLocked()
		return errors.New("Abre Steam para gestionar los mods desde aquí")
	}
	if st.ActiveUser == 0 {
		h.stopLocked()
		return errors.New("Entra en Steam con una cuenta para gestionar los mods")
	}
	if st.RunningApp != 0 && !h.ownRunning(st.RunningApp) {
		h.stopLocked()
		return a.gameOpenErr(st.RunningApp, appid)
	}
	if p := h.p; p != nil && (p.appid != appid || p.user != st.ActiveUser || p.exited.Load()) {
		h.stopLocked()
	}
	if h.p == nil {
		h.mark(appid, true)
		p, err := startWorkshop(in.Dir, appid, func() {
			h.mark(appid, false)
			a.emit("workshop-link", wsLink{AppID: appid})
		})
		if err != nil {
			h.mark(appid, false)
			// Steam corta la conexión del juego si la cuenta abierta no lo tiene, y
			// eso llega como «no hay ninguna cuenta dentro». Pero ya se ha mirado
			// arriba que sí la hay: es que no tiene el juego.
			if err.Error() == steamworks.ErrNoUser.Error() && st.ActiveUser != 0 {
				return steamworks.ErrNotOwned
			}
			// si se ha abierto otro juego mientras tanto, se dice eso
			if st2 := in.ReadStatus(); st2.RunningApp != 0 && st2.RunningApp != appid {
				return a.gameOpenErr(st2.RunningApp, appid)
			}
			return err
		}
		p.user = st.ActiveUser
		h.p = p
		a.emit("workshop-link", wsLink{AppID: appid, On: true})
	}
	return a.wsSendLocked(req, timeout, out, o)
}

// wsSendLocked manda la petición al proceso abierto y espera la respuesta.
func (a *App) wsSendLocked(req wsRequest, timeout time.Duration, out any, o wsOpts) error {
	h := &a.ws
	p := h.p
	p.next++
	req.ID = p.next
	b, _ := json.Marshal(req)
	if _, err := p.stdin.Write(append(b, '\n')); err != nil {
		h.stopLocked()
		return errors.New("Se ha cerrado la conexión con Steam: vuelve a intentarlo")
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	canceled := false
	for {
		select {
		case <-tick.C:
			if o.cancel != nil && !canceled && o.cancel() {
				canceled = true
				b, _ := json.Marshal(wsRequest{Op: "cancel"})
				p.stdin.Write(append(b, '\n'))
			}
			continue
		case r, ok := <-p.replies:
			if !ok {
				h.stopLocked()
				return errors.New("Se ha cerrado la conexión con Steam: vuelve a intentarlo")
			}
			if r.ID != req.ID {
				continue // respuesta de una petición anterior que se dio por perdida
			}
			if r.Progress != nil {
				if o.progress != nil {
					o.progress(*r.Progress)
				}
				deadline.Reset(timeout)
				continue
			}
			if r.Error != "" {
				return errors.New(r.Error)
			}
			if out != nil && len(r.Data) > 0 {
				return json.Unmarshal(r.Data, out)
			}
			return nil
		case <-deadline.C:
			h.stopLocked()
			return errors.New("Steam no ha respondido a tiempo")
		}
	}
}

func startWorkshop(steamDir string, appid uint32, onExit func()) (*wsProc, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	id := strconv.FormatUint(uint64(appid), 10)
	cmd := exec.Command(exe, "--workshop", id, steamDir)
	cmd.Env = append(os.Environ(), "SteamAppId="+id, "SteamGameId="+id)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &wsProc{appid: appid, cmd: cmd, stdin: stdin, replies: make(chan wsReply, 4)}
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			var r wsReply
			if json.Unmarshal(sc.Bytes(), &r) == nil {
				p.replies <- r
			}
		}
		p.exited.Store(true)
		close(p.replies)
		onExit()
	}()
	kill := func() { stdin.Close(); cmd.Process.Kill(); cmd.Wait() }
	select {
	case r, ok := <-p.replies:
		if !ok {
			kill()
			return nil, errors.New("No se pudo conectar con Steam")
		}
		if r.Error != "" {
			kill()
			return nil, errors.New(r.Error)
		}
		return p, nil
	case <-time.After(20 * time.Second):
		kill()
		return nil, errors.New("Steam no ha respondido a tiempo")
	}
}
