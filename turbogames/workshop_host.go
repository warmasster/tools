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
const wsIdle = 20 * time.Second

type wsRequest struct {
	ID     int      `json:"id"`
	Op     string   `json:"op"`
	IDs    []uint64 `json:"ids,omitempty"`
	Text   string   `json:"text,omitempty"`
	Sort   int      `json:"sort,omitempty"`
	Cursor string   `json:"cursor,omitempty"`
}

type wsReply struct {
	ID    int             `json:"id"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
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
	c, err := steamworks.Open(args[1], uint32(id), steam.SignedByValve)
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

func wsHandle(c *steamworks.Client, r wsRequest) (any, error) {
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
		return c.Search(r.Text, r.Sort, r.Cursor)
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
		// Steam tarda un poco en darse cuenta de que se ha cerrado
		h.till = time.Now().Add(5 * time.Second)
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
	deadline := time.After(timeout)
	for {
		select {
		case r, ok := <-p.replies:
			if !ok {
				h.stopLocked()
				return errors.New("Se ha cerrado la conexión con Steam: vuelve a intentarlo")
			}
			if r.ID != req.ID {
				continue // respuesta de una petición anterior que se dio por perdida
			}
			if r.Error != "" {
				return errors.New(r.Error)
			}
			if out != nil && len(r.Data) > 0 {
				return json.Unmarshal(r.Data, out)
			}
			return nil
		case <-deadline:
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
