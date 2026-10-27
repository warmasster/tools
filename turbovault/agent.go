package main

// El agente es el proceso que se queda en la bandeja (junto a la flechita de
// la barra de tareas). Es el único que tiene la bóveda abierta: la ventana y la
// extensión del navegador le piden lo que necesitan por una tubería protegida.
//
// La sesión dura un tiempo fijo desde que se abre (10 minutos por defecto) y no
// se alarga con el uso. Además se cierra al bloquear Windows, al suspender el
// equipo y al cerrar sesión.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"turbovault/internal/ipc"
	"turbovault/internal/vault"
	"turbovault/internal/winsec"
)

const (
	roleApp      = "app"      // la ventana que lanzó el agente
	roleExt      = "ext"      // el puente del navegador
	roleLauncher = "launcher" // TurboVAULT.exe pidiendo que se abra la ventana
)

type peer struct {
	Role     string
	Browser  string
	PID      uint32
	UserData string // carpeta de perfiles del navegador si no es la normal
	LoadExt  bool   // el navegador carga extensiones por línea de órdenes
}

type connState struct {
	c        *ipc.Conn
	peer     peer
	mu       sync.Mutex // pairHash y paired (cada petición va en su goroutine)
	pairHash string
	paired   bool
}

func (cs *connState) hash() string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.pairHash
}

func (cs *connState) setPair(hash string, paired bool) {
	cs.mu.Lock()
	cs.pairHash, cs.paired = hash, paired
	cs.mu.Unlock()
}

func (cs *connState) isPaired() bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.paired
}

type pairRequest struct {
	Hash    string
	Code    string
	Browser string
	Created time.Time
}

// PairInfo es lo que ve la ventana de una vinculación pendiente.
type PairInfo struct {
	Code    string `json:"code"`
	Browser string `json:"browser"`
}

// State es el estado que se manda a la ventana y a la extensión.
type State struct {
	Exists     bool      `json:"exists"`
	Damaged    string    `json:"damaged,omitempty"`
	Unlocked   bool      `json:"unlocked"`
	Expires    int64     `json:"expires,omitempty"` // ms
	Now        int64     `json:"now"`               // ms
	SessionMin int       `json:"sessionMin"`
	WaitUntil  int64     `json:"waitUntil,omitempty"`
	Fails      int       `json:"fails,omitempty"`
	Count      int       `json:"count"`
	Version    string    `json:"version"`
	Paired     bool      `json:"paired"`
	ExtBad     string    `json:"extBad,omitempty"`     // la extensión cargada no es la original
	ExtVersion string    `json:"extVersion,omitempty"` // versión de la extensión que trae este TurboVAULT
	Pair       *PairInfo `json:"pair,omitempty"`
}

// apiError es un error con un código que entienden la ventana y la extensión.
type apiError struct {
	Code string
	Msg  string
}

func (e *apiError) Error() string { return e.Msg }

func errCode(code, msg string) error { return &apiError{Code: code, Msg: msg} }

var (
	errLocked   = errCode("locked", "La bóveda está cerrada.")
	errUnpaired = errCode("unpaired", "Este navegador no está vinculado con TurboVAULT.")
	errDenied   = errCode("denied", "No permitido.")
)

type Agent struct {
	v       *vault.Vault
	srv     *ipc.Server
	exe     string
	exeID   winsec.FileID
	pipe    string
	verify  ipc.Verify
	tray    *tray
	quit    chan struct{}
	quitter sync.Once

	mu        sync.Mutex
	expires   time.Time
	timer     *time.Timer
	conns     map[*ipc.Conn]*connState
	pending   *pairRequest
	clipSeq   uint32
	clipTimer *time.Timer
	breached  map[string]int // id → veces que aparece en filtraciones (solo en memoria)
	breachAt  int64

	winMu   sync.Mutex
	windows map[uint32]bool // ventanas lanzadas por el agente y aún vivas

	unlockMu sync.Mutex
	lockGen  atomic.Uint64
	secrets  *limiter // peticiones que revelan secretos desde el navegador
	lookups  *limiter // búsquedas desde el navegador
	checks   *limiter // «¿guardar esto?» desde el navegador
	backupCh chan struct{}
}

func newAgent(exe, pipe string) *Agent {
	return &Agent{
		v:        vault.Open(vaultPath()),
		exe:      exe,
		pipe:     pipe,
		quit:     make(chan struct{}),
		conns:    map[*ipc.Conn]*connState{},
		windows:  map[uint32]bool{},
		secrets:  &limiter{},
		lookups:  &limiter{},
		checks:   &limiter{},
		backupCh: make(chan struct{}, 1),
	}
}

// runAgent arranca el agente. Vuelve cuando se cierra.
func runAgent() int {
	exe, _ := os.Executable()
	// Un agente que no puede usar su propio token (lanzado con permisos rotos)
	// no podría abrir ventanas ni reconocer al navegador: mejor no ocupar la
	// tubería y dejar que se arranque bien la próxima vez.
	if !winsec.TokenUsable() {
		logf("agente: arrancado sin permisos sobre su propio token; se cierra")
		return 1
	}
	_ = winsec.HardenProcess()
	_ = winsec.HardenThreads()
	_ = winsec.BlockInjection(true)
	_ = winsec.SecureDir(dataDir())
	a := newAgent(exe, pipeName())
	id, err := winsec.FileIDOf(exe)
	if err != nil {
		logf("agente: no se puede identificar el programa: %v", err)
		return 1
	}
	a.exeID = id
	a.verify = a.verifyClient
	srv, err := ipc.Listen(a.pipe, a.verify, a.handle)
	if err != nil {
		// Ya hay un agente en marcha (o alguien ocupa el nombre): no seguir.
		logf("agente: %v", err)
		return 1
	}
	a.srv = srv
	logf("agente %s en marcha", version)
	a.tray = startTray(a)
	go a.backupLoop()
	a.requestBackup()
	<-a.quit
	a.lockNow("salir")
	srv.Close()
	if a.tray != nil {
		a.tray.close()
	}
	return 0
}

func (a *Agent) shutdown() { a.quitter.Do(func() { close(a.quit) }) }

// verifyClient comprueba quién se conecta: tiene que ser este mismo programa.
// Si lo ha lanzado un navegador para la extensión, tiene que venir de un
// navegador firmado y con el origen de nuestra extensión.
func (a *Agent) verifyClient(pid uint32) (any, error) {
	p, err := a.checkClient(pid)
	if err != nil {
		logf("conexión rechazada (pid %d): %v", pid, err)
	}
	return p, err
}

func (a *Agent) checkClient(pid uint32) (any, error) {
	// El mismo archivo, no solo la misma ruta: si alguien renombra el .exe y
	// pone otro en su sitio, ya no cuela.
	path, id, err := winsec.ProcessFileID(pid)
	if err != nil {
		return nil, err
	}
	if !winsec.SamePath(path, a.exe) || id != a.exeID {
		return nil, fmt.Errorf("programa no autorizado: %s", path)
	}
	raw, err := winsec.RawCommandLine(pid)
	if err != nil {
		return nil, err
	}
	args, err := winsec.CommandLine(pid)
	if err != nil || len(args) == 0 {
		return nil, errors.New("línea de órdenes ilegible")
	}
	// Cualquier rastro de "chrome-extension:" convierte la conexión en puente
	// del navegador, y entonces tiene que cumplir todo; si no, se rechaza. Nunca
	// cae en el papel de la ventana por un fallo al leer los argumentos.
	if strings.Contains(strings.ToLower(raw), "chrome-extension:") {
		origin := "chrome-extension://" + extensionID + "/"
		if len(args) < 2 || args[1] != origin || strings.Count(strings.ToLower(raw), "chrome-extension:") != 1 {
			return nil, errors.New("puente con argumentos raros")
		}
		for _, x := range args[2:] {
			if !strings.HasPrefix(x, "--parent-window=") {
				return nil, errors.New("puente con argumentos raros")
			}
		}
		browser, bpid, err := winsec.BrowserProcess(pid)
		if err != nil {
			return nil, err
		}
		pr := peer{Role: roleExt, Browser: browser, PID: pid}
		// Con otra carpeta de perfiles, la comprobación de la extensión mira
		// también allí; cargar extensiones por línea de órdenes no se admite.
		if bargs, err := winsec.CommandLine(bpid); err == nil {
			for _, x := range bargs[1:] {
				lx := strings.ToLower(x)
				if strings.HasPrefix(lx, "--user-data-dir=") {
					pr.UserData = strings.Trim(x[len("--user-data-dir="):], `"`)
				}
				if strings.HasPrefix(lx, "--load-extension") || strings.HasPrefix(lx, "--disable-extensions-except") {
					pr.LoadExt = true
				}
			}
		} else {
			return nil, errors.New("no se puede leer cómo se lanzó el navegador")
		}
		return pr, nil
	}
	// La ventana: solo la que lanzó este agente (protegida desde que nació).
	if len(args) >= 2 && args[1] == "--window" {
		for _, x := range args[2:] {
			if x != "--unlock" && x != "--pair" {
				return nil, errors.New("ventana con argumentos raros")
			}
		}
		if !a.isWindow(pid) {
			return nil, errors.New("ventana que no ha abierto el agente")
		}
		return peer{Role: roleApp, PID: pid}, nil
	}
	// Cualquier otra copia del programa solo puede pedir que se abra la ventana.
	for _, x := range args[1:] {
		if x != "--unlock" && x != "--pair" {
			return nil, errors.New("argumentos raros")
		}
	}
	return peer{Role: roleLauncher, PID: pid}, nil
}

func (a *Agent) handle(c *ipc.Conn) {
	p, ok := c.Info.(peer)
	if !ok || p.Role == "" {
		return // nunca debería pasar: sin papel no se atiende
	}
	cs := &connState{c: c, peer: p}
	logf("conexión: %s %s (pid %d)", p.Role, p.Browser, p.PID)
	a.mu.Lock()
	a.conns[c] = cs
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.conns, c)
		a.mu.Unlock()
	}()
	c.Write(ipc.Message{Event: "state", Data: mustJSON(a.stateFor(cs))})
	for {
		var m ipc.Message
		if err := c.Read(&m); err != nil {
			return
		}
		go func(m ipc.Message) {
			defer func() {
				if r := recover(); r != nil {
					logf("pánico en %s: %v\n%s", m.Cmd, r, debug.Stack())
					c.Write(ipc.Message{ID: m.ID, Error: "Error interno", Code: "internal"})
				}
			}()
			data, err := a.dispatch(cs, m.Cmd, m.Args)
			resp := ipc.Message{ID: m.ID}
			if err != nil {
				resp.Error = err.Error()
				var ae *apiError
				switch {
				case errors.As(err, &ae):
					resp.Code = ae.Code
				case errors.Is(err, vault.ErrLocked):
					resp.Code = "locked"
				case errors.Is(err, vault.ErrWrong):
					resp.Code = "wrong"
				case errors.Is(err, vault.ErrNoVault):
					resp.Code = "novault"
				case errors.Is(err, vault.ErrNotFound):
					resp.Code = "notfound"
				default:
					resp.Code = "error"
				}
			} else {
				resp.OK = true
				resp.Data = mustJSON(data)
			}
			c.Write(resp)
		}(m)
	}
}

func mustJSON(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

// ---------- estado y sesión ----------

func (a *Agent) stateFor(cs *connState) State {
	s := getSettings()
	st := State{
		Exists:     a.v.Exists(),
		Unlocked:   a.v.Unlocked(),
		Now:        time.Now().UnixMilli(),
		SessionMin: s.SessionMin,
		Fails:      s.Fails,
		Version:    version,
	}
	if s.WaitUntil > st.Now {
		st.WaitUntil = s.WaitUntil
	}
	if err := a.v.Damaged(); err != nil {
		st.Damaged = err.Error()
	}
	a.mu.Lock()
	if st.Unlocked {
		st.Expires = a.expires.UnixMilli()
	}
	pend := a.pending
	if pend != nil && time.Since(pend.Created) > 5*time.Minute {
		a.pending, pend = nil, nil
	}
	a.mu.Unlock()
	if st.Unlocked {
		if items, err := a.v.Items(false); err == nil {
			st.Count = len(items)
		}
	}
	if cs == nil || cs.peer.Role == roleApp {
		extCheckMu.Lock()
		st.ExtBad = extCheckBad
		extCheckMu.Unlock()
	}
	if cs != nil && cs.peer.Role == roleExt {
		// Si la extensión que corre en Chrome es más vieja, se recarga sola.
		st.ExtVersion = extVersion
		st.Paired = st.Unlocked && a.checkPaired(cs)
		if !st.Paired {
			st.Count = 0 // un navegador sin vincular no sabe ni cuántas hay
		}
	} else if pend != nil {
		st.Pair = &PairInfo{Code: pend.Code, Browser: pend.Browser}
	}
	return st
}

func (a *Agent) checkPaired(cs *connState) bool {
	h := cs.hash()
	if h == "" {
		return false
	}
	ok, err := a.v.HasPair(h)
	if err != nil {
		return false
	}
	cs.mu.Lock()
	if cs.pairHash == h {
		cs.paired = ok
	}
	cs.mu.Unlock()
	return ok
}

// broadcast manda el estado a todas las conexiones.
func (a *Agent) broadcast() {
	a.mu.Lock()
	list := make([]*connState, 0, len(a.conns))
	for _, cs := range a.conns {
		list = append(list, cs)
	}
	a.mu.Unlock()
	for _, cs := range list {
		cs.c.Write(ipc.Message{Event: "state", Data: mustJSON(a.stateFor(cs))})
	}
	if a.tray != nil {
		a.tray.refresh()
	}
}

// masterTry ejecuta algo que comprueba la contraseña maestra (abrir, exportar,
// cambiarla, restaurar…) con el mismo freno para todos: uno a la vez, y tras 3
// fallos seguidos hay que esperar cada vez más.
func (a *Agent) masterTry(fn func() error) error {
	a.unlockMu.Lock()
	defer a.unlockMu.Unlock()
	s := getSettings()
	if now := time.Now().UnixMilli(); s.WaitUntil > now {
		secs := (s.WaitUntil - now + 999) / 1000
		return errCode("wait", fmt.Sprintf("Demasiados intentos. Espera %d s.", secs))
	}
	err := fn()
	debug.FreeOSMemory() // devolver los 256 MB de Argon2id
	if errors.Is(err, vault.ErrWrong) {
		updateSettings(func(s *Settings) {
			s.Fails++
			if s.Fails >= 3 {
				wait := 5 * time.Second << min(s.Fails-3, 8)
				if wait > 15*time.Minute {
					wait = 15 * time.Minute
				}
				s.WaitUntil = time.Now().Add(wait).UnixMilli()
			}
		})
		a.broadcast()
	}
	return err
}

// testAfterUnlock deja a las pruebas simular algo justo después de Argon2id.
var testAfterUnlock func(*Agent)

func (a *Agent) unlock(master string) error { return a.unlockGen(master, a.lockGen.Load()) }

// unlockGen abre la bóveda solo si no ha habido ningún cierre (bloqueo de
// Windows, suspensión…) desde gen.
func (a *Agent) unlockGen(master string, gen uint64) error {
	fails := getSettings().Fails
	sessionMin := getSettings().SessionMin
	err := a.masterTry(func() error {
		// La caducidad se pone antes de abrir: así nunca hay un momento con la
		// bóveda abierta y sin hora de cierre.
		a.mu.Lock()
		a.expires = time.Now().Add(time.Duration(sessionMin) * time.Minute)
		a.mu.Unlock()
		return a.v.Unlock(master)
	})
	if err != nil {
		return err
	}
	if testAfterUnlock != nil {
		testAfterUnlock(a)
	}
	// Si mientras se abría se bloqueó Windows o se suspendió el equipo, no se
	// deja abierta.
	if a.lockGen.Load() != gen {
		a.lockNow("windows")
		return errCode("interrupted", "Se bloqueó el equipo mientras se abría. Vuelve a intentarlo.")
	}
	s := updateSettings(func(s *Settings) { s.Fails, s.WaitUntil = 0, 0 })
	a.secrets.reset()
	a.mu.Lock()
	a.expires = time.Now().Add(time.Duration(s.SessionMin) * time.Minute)
	if a.timer != nil {
		a.timer.Stop()
	}
	a.timer = time.AfterFunc(time.Until(a.expires), func() { a.lockNow("tiempo") })
	a.mu.Unlock()
	if fails > 0 {
		a.event(vault.Event{Kind: "fails", Title: fmt.Sprintf("%d intentos fallidos antes de abrir", fails), Src: "agente"})
	}
	// ¿Es una versión más vieja que la última que se abrió? (alguien ha puesto
	// una copia antigua en su sitio)
	if rev := a.v.Rev(); s.MaxRev > 0 && rev < s.MaxRev {
		a.event(vault.Event{Kind: "rollback", Title: "La bóveda es más antigua que la última que abriste", Src: "agente"})
		if a.tray != nil {
			a.tray.notify("Bóveda más antigua de lo esperado", "El archivo de la bóveda es una versión anterior a la última que abriste. Si no has restaurado una copia, revisa la actividad.")
		}
	}
	a.noteRev()
	a.event(vault.Event{Kind: "unlock", Src: "agente"})
	a.broadcast()
	a.requestBackup()
	return nil
}

// noteRev apunta la versión más alta de la bóveda que se ha visto.
func (a *Agent) noteRev() {
	if rev := a.v.Rev(); rev > 0 {
		updateSettings(func(s *Settings) {
			if rev > s.MaxRev {
				s.MaxRev = rev
			}
		})
	}
}

// lockNow cierra la bóveda (si estaba abierta) y avisa a todos.
func (a *Agent) lockNow(reason string) {
	a.lockGen.Add(1)
	wasOpen := a.v.Unlocked()
	if wasOpen {
		a.noteRev()
		a.event(vault.Event{Kind: "lock", Title: lockReason(reason), Src: "agente"})
	}
	a.v.Lock()
	a.mu.Lock()
	if a.timer != nil {
		a.timer.Stop()
		a.timer = nil
	}
	a.expires = time.Time{}
	a.breached, a.breachAt = nil, 0
	seq := a.clipSeq
	a.clipSeq = 0
	a.mu.Unlock()
	clipClear(seq)
	if wasOpen {
		logf("bóveda cerrada: %s", reason)
		debug.FreeOSMemory()
	}
	a.broadcast()
}

func lockReason(r string) string {
	switch r {
	case "tiempo":
		return "Se acabó el tiempo de la sesión"
	case "windows":
		return "Se bloqueó Windows"
	case "suspender":
		return "El equipo se suspendió"
	case "manual":
		return "Cerrada a mano"
	case "navegador":
		return "Cerrada desde el navegador"
	case "abuso":
		return "Demasiadas peticiones desde el navegador"
	case "salir":
		return "TurboVAULT se cerró"
	}
	return r
}

// expired comprueba la sesión por si el temporizador se ha retrasado (equipo
// suspendido, reloj cambiado).
func (a *Agent) expired() bool {
	a.mu.Lock()
	exp := a.expires
	a.mu.Unlock()
	if a.v.Unlocked() && (exp.IsZero() || time.Now().After(exp)) {
		a.lockNow("tiempo")
		return true
	}
	return false
}

// warnExtension avisa (una vez por arranque) de que la extensión no es la original.
func (a *Agent) warnExtension(bad string) {
	extCheckMu.Lock()
	first := !extCheckWarn
	extCheckWarn = true
	extCheckMu.Unlock()
	if !first {
		return
	}
	logf("extensión modificada: %s", bad)
	a.event(vault.Event{Kind: "tampered", Title: "La extensión del navegador no es la original (" + bad + ")", Src: "agente"})
	if a.tray != nil {
		a.tray.notify("Extensión modificada", "La extensión de TurboVAULT en el navegador ha cambiado y no recibirá contraseñas. Reinstálala desde Ajustes → Navegador.")
	}
}

func (a *Agent) event(e vault.Event) {
	if a.v.Unlocked() {
		_ = a.v.AddEvent(e)
	}
}

// ---------- portapapeles ----------

// copySecret copia un texto marcándolo como secreto (fuera del historial de
// Windows y de la nube) y lo borra pasados unos segundos si sigue ahí.
// Se pueden cambiar en las pruebas para no tocar el portapapeles de verdad.
var (
	clipSet   = setClipboard
	clipClear = clearClipboardIf
)

func (a *Agent) copySecret(text string) (int, error) {
	seq, err := clipSet(text, true)
	if err != nil {
		return 0, err
	}
	secs := getSettings().ClipSeconds
	a.mu.Lock()
	a.clipSeq = seq
	if a.clipTimer != nil {
		a.clipTimer.Stop()
	}
	a.clipTimer = time.AfterFunc(time.Duration(secs)*time.Second, func() {
		a.mu.Lock()
		s := a.clipSeq
		if s == seq {
			a.clipSeq = 0
		}
		a.mu.Unlock()
		if s == seq {
			clipClear(seq)
		}
	})
	a.mu.Unlock()
	return secs, nil
}

// ---------- vinculación de navegadores ----------

func pairHash(token string) string {
	h := sha256.Sum256([]byte("TurboVAULT pair|" + token))
	return hex.EncodeToString(h[:])
}

// pairCode es el código de 6 cifras que enseñan a la vez la extensión y la
// ventana para comprobar que se vincula el navegador correcto.
func pairCode(token string) string {
	h := sha256.Sum256([]byte("TurboVAULT code|" + token))
	n := (uint32(h[0])<<16 | uint32(h[1])<<8 | uint32(h[2])) % 1000000
	return fmt.Sprintf("%03d %03d", n/1000, n%1000)
}

// ---------- límite de peticiones ----------

type limiter struct {
	mu    sync.Mutex
	times []time.Time
}

func (l *limiter) reset() {
	l.mu.Lock()
	l.times = nil
	l.mu.Unlock()
}

// allow apunta una petición y dice si cabe en los límites por minuto y hora.
func (l *limiter) allow(perMin, perHour int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	keep := l.times[:0]
	for _, t := range l.times {
		if now.Sub(t) < time.Hour {
			keep = append(keep, t)
		}
	}
	l.times = keep
	inMin := 0
	for _, t := range l.times {
		if now.Sub(t) < time.Minute {
			inMin++
		}
	}
	if inMin >= perMin || len(l.times) >= perHour {
		return false
	}
	l.times = append(l.times, now)
	return true
}
