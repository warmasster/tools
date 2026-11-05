package main

// El Workshop se maneja desde un proceso aparte (TurboGAMES.exe --workshop
// <appid> <carpeta de Steam>) que Steam toma por el juego: así se le puede pedir
// que suscriba o quite mods, igual que lo hace el juego desde su menú. Ese
// proceso no abre el juego ni toca sus archivos, se cierra solo a los pocos
// segundos sin usarlo y mientras vive los amigos ven «jugando a…».
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

// Cuánto sigue vivo el proceso sin peticiones (cuanto menos, menos rato sale
// «jugando a…»).
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
	mu   sync.Mutex // una petición cada vez
	p    *wsProc
	smu  sync.Mutex
	app  uint32    // juego que «está abierto» por nosotros
	till time.Time // hasta cuándo no contar ese juego como abierto
}

// ownRunning dice si el juego que Steam da por abierto es nuestro proceso.
func (h *wsHost) ownRunning(appid uint32) bool {
	h.smu.Lock()
	defer h.smu.Unlock()
	return appid != 0 && appid == h.app && time.Now().Before(h.till)
}

func (h *wsHost) mark(appid uint32, alive bool) {
	h.smu.Lock()
	defer h.smu.Unlock()
	if alive {
		h.app, h.till = appid, time.Now().Add(24*time.Hour)
	} else if h.app == appid {
		// Steam tarda unos segundos en darse cuenta de que se ha cerrado
		h.till = time.Now().Add(15 * time.Second)
	}
}

func (h *wsHost) stopLocked() {
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

// forgetOwn olvida que el juego abierto pueda ser nuestro (cuando se va a abrir
// el juego de verdad: a partir de ahí, si Steam lo da por abierto, lo está).
func (h *wsHost) forgetOwn() {
	h.smu.Lock()
	h.app, h.till = 0, time.Time{}
	h.smu.Unlock()
}

// wsAlive dice si ya hay un proceso del Workshop abierto para ese juego (lo
// que se pide en segundo plano solo lo usa si ya está: no se conecta solo).
func (a *App) wsAlive(appid uint32) bool {
	if !a.ws.mu.TryLock() {
		return true // ocupado con otra petición: está vivo
	}
	defer a.ws.mu.Unlock()
	p := a.ws.p
	return p != nil && p.appid == appid && !p.exited.Load()
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

// wsCall manda una petición al proceso del Workshop de ese juego (lo abre si
// hace falta) y espera su respuesta.
func (a *App) wsCall(appid uint32, req wsRequest, timeout time.Duration, out any) error {
	return a.wsCallEx(appid, req, timeout, out, nil, nil)
}

// wsCallEx es wsCall para las tandas: progress recibe cada mod que termina (y
// el plazo vuelve a empezar), y si cancel dice que pare, se le pide al proceso.
func (a *App) wsCallEx(appid uint32, req wsRequest, timeout time.Duration, out any, progress func(wsProgress), cancel func() bool) error {
	in := a.steam()
	if in == nil {
		return steam.ErrNotFound
	}
	if a.busySwitching() {
		return errors.New("Espera a que termine el cambio de cuenta")
	}
	h := &a.ws
	h.mu.Lock()
	defer h.mu.Unlock()
	st := in.ReadStatus()
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
		name := a.GameName(st.RunningApp)
		if name == "" {
			name = "el juego que tienes abierto"
		}
		return fmt.Errorf("Cierra %s para gestionar los mods desde aquí (Steam lo tomaría como abrir otro juego a la vez)", name)
	}
	if p := h.p; p != nil && (p.appid != appid || p.user != st.ActiveUser || p.exited.Load()) {
		h.stopLocked()
	}
	if h.p == nil {
		p, err := startWorkshop(in.Dir, appid, func() { h.mark(appid, false) })
		if err != nil {
			// Steam corta la conexión del juego si la cuenta abierta no lo tiene, y
			// eso llega como «no hay ninguna cuenta dentro». Pero ya se ha mirado
			// arriba que sí la hay: es que no tiene el juego.
			if err.Error() == steamworks.ErrNoUser.Error() && st.ActiveUser != 0 {
				return steamworks.ErrNotOwned
			}
			return err
		}
		p.user = st.ActiveUser
		h.p = p
		h.mark(appid, true)
	}
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
			if cancel != nil && !canceled && cancel() {
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
				if progress != nil {
					progress(*r.Progress)
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
