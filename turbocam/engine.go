package main

// El motor: recibe los fotogramas de los móviles (y de la pantalla), compone la
// escena y la publica en la cámara virtual. También decide qué le pide a cada móvil
// (tamaño, fotogramas por segundo) según lo que haga falta en cada momento: si
// nadie mira la cámara ni la ventana, los móviles bajan a 1-2 fotogramas por segundo
// para no gastar batería.

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// transport es la conexión con un móvil (WebSocket, o HTTP si el WebSocket no va).
type transport interface {
	send(msg any) error
	close()
}

type phone struct {
	id        string
	t         transport
	via       string // internet | direct | lan | local
	connected time.Time
	hub       *jpegHub // miniatura en la ventana
	photo     chan []byte

	mu       sync.Mutex
	name     string
	model    string
	caps     json.RawMessage
	visible  bool
	live     bool
	errMsg   string
	battery  float64
	charging bool
	quality  float64
	rtt      int
	frame    *image.YCbCr
	jpeg     []byte
	seq      uint64
	frameAt  time.Time
	winStart time.Time
	winCount int
	winBytes int
	fps      float64
	kbps     float64
	sentCfg  string
}

// PhoneInfo es lo que la ventana sabe de un móvil.
type PhoneInfo struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Model     string          `json:"model"`
	Via       string          `json:"via"`
	Visible   bool            `json:"visible"`
	Live      bool            `json:"live"`
	Error     string          `json:"error"`
	Battery   float64         `json:"battery"`
	Charging  bool            `json:"charging"`
	FPS       float64         `json:"fps"`
	Kbps      float64         `json:"kbps"`
	Quality   float64         `json:"quality"`
	RTT       int             `json:"rtt"`
	Width     int             `json:"width"`
	Height    int             `json:"height"`
	Role      string          `json:"role"`
	Caps      json.RawMessage `json:"caps"`
	Adjust    Adjust          `json:"adjust"`
	Connected int64           `json:"connected"`
}

type Engine struct {
	mu          sync.Mutex
	shm         *camShm
	phones      map[string]*phone
	order       []string
	frozen      bool
	covered     bool
	consumers   []Consumer
	consKey     string
	autoProfile string
	restore     *restorePoint
	outW, outH  int
	fps         int
	uiVisible   bool
	windowMode  bool
	screenErr   string
	outFPS      float64
	outCount    int
	outWin      time.Time

	notify    chan struct{}
	quit      chan struct{}
	done      chan struct{}
	outHub    *jpegHub
	lastOut   []byte // último JPEG de la salida (foto sin móvil)
	lastEnc   time.Time
	canvas    *image.YCbCr
	grab      grabber
	primary   Region // pantalla principal (solo la usa el compositor)
	primaryAt time.Time
	composed  map[string]uint64
	onChange  func()
	onToast   func(kind, msg string)
	pcName    string
}

type restorePoint struct {
	scene  Scene
	device string
	adjust Adjust
}

func newEngine(shm *camShm, onChange func(), onToast func(kind, msg string)) *Engine {
	host, _ := os.Hostname()
	e := &Engine{
		shm:      shm,
		phones:   map[string]*phone{},
		notify:   make(chan struct{}, 1),
		quit:     make(chan struct{}),
		done:     make(chan struct{}),
		outHub:   newJPEGHub(),
		composed: map[string]uint64{},
		onChange: onChange,
		onToast:  onToast,
		pcName:   host,
		outW:     1280,
		outH:     720,
		fps:      30,
	}
	e.applyOutput(getSettings(), nil)
	go e.loop()
	go e.housekeeping()
	return e
}

func (e *Engine) Close() {
	close(e.quit)
	<-e.done
	e.mu.Lock()
	phones := e.phoneList()
	e.mu.Unlock()
	for _, p := range phones {
		p.t.send(map[string]any{"t": "bye", "reason": "closed"})
		p.t.close()
	}
}

func (e *Engine) kick() {
	select {
	case e.notify <- struct{}{}:
	default:
	}
}

func (e *Engine) changed() {
	if e.onChange != nil {
		e.onChange()
	}
}

func (e *Engine) toast(kind, msg string) {
	if e.onToast != nil {
		e.onToast(kind, msg)
	}
}

// phoneList: con e.mu cogido.
func (e *Engine) phoneList() []*phone {
	out := make([]*phone, 0, len(e.order))
	for _, id := range e.order {
		if p := e.phones[id]; p != nil {
			out = append(out, p)
		}
	}
	return out
}

// ---------- móviles ----------

const maxPhones = 8

// addPhone da de alta un móvil. Si ya estaba conectado (recargó la página), la
// conexión vieja se cierra.
func (e *Engine) addPhone(id, name, model, via string, t transport) (*phone, error) {
	p := &phone{id: id, t: t, via: via, connected: time.Now(), hub: newJPEGHub(), photo: make(chan []byte, 1),
		name: name, model: model, battery: -1, quality: 0}
	e.mu.Lock()
	old := e.phones[id]
	if old == nil && len(e.phones) >= maxPhones {
		e.mu.Unlock()
		return nil, errors.New("demasiados móviles conectados")
	}
	e.phones[id] = p
	if old == nil {
		e.order = append(e.order, id)
	}
	e.mu.Unlock()
	if old != nil {
		old.t.send(map[string]any{"t": "bye", "reason": "replaced"})
		old.t.close()
	}
	updateSettings(func(s *Settings) {
		d := s.Devices[id]
		if d == nil {
			d = &Device{Adjust: defaultAdjust()}
			s.Devices[id] = d
		}
		if d.Name == "" {
			d.Name = name
		}
		// La escena sigue al primer móvil si no tiene ninguno elegido.
		if s.Scene.Main == "" || (s.Scene.Main != "screen" && s.Devices[s.Scene.Main] == nil) {
			s.Scene.Main = id
		}
	})
	e.sendConfigs(true)
	e.kick()
	e.changed()
	return p, nil
}

// removePhone se llama al cerrarse su conexión.
func (e *Engine) removePhone(p *phone) {
	e.mu.Lock()
	if e.phones[p.id] != p {
		e.mu.Unlock()
		return
	}
	delete(e.phones, p.id)
	e.order = slices.DeleteFunc(e.order, func(s string) bool { return s == p.id })
	delete(e.composed, p.id)
	e.mu.Unlock()
	e.sendConfigs(false)
	e.kick()
	e.changed()
}

// Kick echa a un móvil (puede volver a entrar con el mismo enlace).
func (e *Engine) Kick(id string) {
	e.mu.Lock()
	p := e.phones[id]
	e.mu.Unlock()
	if p != nil {
		p.t.send(map[string]any{"t": "bye", "reason": "kicked"})
		p.t.close()
		e.removePhone(p)
	}
}

// KickAll echa a todos (al renovar el enlace).
func (e *Engine) KickAll() {
	e.mu.Lock()
	ids := slices.Clone(e.order)
	e.mu.Unlock()
	for _, id := range ids {
		e.Kick(id)
	}
}

// onFrame llega desde la conexión del móvil: se decodifica allí mismo (cada móvil
// en su hilo) y se avisa al compositor.
func (e *Engine) onFrame(p *phone, data []byte) error {
	img, err := decodeJPEG(data)
	if err != nil {
		return err
	}
	now := time.Now()
	p.mu.Lock()
	p.frame, p.jpeg, p.frameAt = img, data, now
	p.seq++
	if p.winStart.IsZero() {
		p.winStart = now
	}
	p.winCount++
	p.winBytes += len(data)
	if d := now.Sub(p.winStart); d >= time.Second {
		p.fps = float64(p.winCount) / d.Seconds()
		p.kbps = float64(p.winBytes) * 8 / 1000 / d.Seconds()
		p.winStart, p.winCount, p.winBytes = now, 0, 0
	}
	p.mu.Unlock()
	p.hub.publish(data)
	e.kick()
	return nil
}

func (e *Engine) onPhoto(p *phone, data []byte) {
	select {
	case p.photo <- data:
	default:
	}
}

// phoneMsg son los mensajes de texto del móvil.
type phoneMsg struct {
	T        string          `json:"t"`
	Visible  *bool           `json:"visible"`
	Live     *bool           `json:"live"`
	Error    *string         `json:"error"`
	Battery  *float64        `json:"battery"`
	Charging *bool           `json:"charging"`
	Quality  *float64        `json:"q"`
	RTT      *int            `json:"rtt"`
	Caps     json.RawMessage `json:"caps"`
	Adjust   *Adjust         `json:"adjust"`
}

func (e *Engine) onMessage(p *phone, m phoneMsg) {
	switch m.T {
	case "state":
		p.mu.Lock()
		wasVisible := p.visible
		if m.Visible != nil {
			p.visible = *m.Visible
		}
		if m.Live != nil {
			p.live = *m.Live
		}
		if m.Error != nil {
			p.errMsg = truncate(*m.Error, 200)
		}
		if m.Battery != nil {
			p.battery = *m.Battery
		}
		if m.Charging != nil {
			p.charging = *m.Charging
		}
		if m.Quality != nil {
			p.quality = *m.Quality
		}
		if m.RTT != nil {
			p.rtt = *m.RTT
		}
		if len(m.Caps) > 0 && len(m.Caps) < 16<<10 {
			p.caps = m.Caps
		}
		nowVisible := p.visible
		p.mu.Unlock()
		if wasVisible != nowVisible {
			e.kick()
		}
		e.changed()
	case "adjust":
		if m.Adjust != nil {
			e.SetAdjust(p.id, *m.Adjust)
		}
	}
}

// SetAdjust cambia los retoques de un móvil (desde el móvil o desde la ventana).
func (e *Engine) SetAdjust(id string, a Adjust) {
	updateSettings(func(s *Settings) {
		d := s.Devices[id]
		if d == nil {
			d = &Device{}
			s.Devices[id] = d
		}
		d.Adjust = a.normalized()
	})
	e.sendConfigs(false)
	e.changed()
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// ---------- qué se le pide a cada móvil ----------

// demand: alguien está mirando (una app con la cámara, la ventana o el modo ventana).
func (e *Engine) demandLocked(s Settings) bool {
	return len(e.consumers) > 0 || e.uiVisible || e.windowMode || !s.Idle
}

// resolveLocked convierte un id de la escena en una fuente que existe: si el móvil
// elegido no está, se usa el primero conectado.
func (e *Engine) resolveLocked(id string) string {
	if id == "screen" {
		return id
	}
	if _, ok := e.phones[id]; ok {
		return id
	}
	if len(e.order) > 0 {
		return e.order[0]
	}
	return ""
}

// roleLocked dice qué papel tiene cada móvil en la escena y a qué tamaño lo necesita.
func (e *Engine) roleLocked(s Settings, id string) (role string, w, h int) {
	sc := s.Scene
	main := e.resolveLocked(sc.Main)
	switch {
	case sc.Kind != "screen" && main == id:
		return "main", e.outW, e.outH
	case sc.Kind == "pip" && e.resolveLocked(sc.Pip) == id && main != id:
		pw, ph := e.pipRectLocked(sc)
		return "pip", pw, ph
	}
	return "spare", 320, 180
}

func (e *Engine) pipRectLocked(sc Scene) (int, int) {
	pw := int(float64(e.outW)*sc.PipSize) &^ 1
	ph := (pw * e.outH / e.outW) &^ 1
	return pw, ph
}

// sendConfigs manda a cada móvil lo que se le pide (solo si ha cambiado, o a todos
// con force).
func (e *Engine) sendConfigs(force bool) {
	s := getSettings()
	e.mu.Lock()
	demand := e.demandLocked(s)
	paused := e.frozen || e.covered
	type job struct {
		p   *phone
		msg map[string]any
	}
	var jobs []job
	for _, p := range e.phoneList() {
		role, w, h := e.roleLocked(s, p.id)
		fps := s.FPS
		switch {
		case role == "spare" && e.uiVisible:
			fps = 4
		case role == "spare", !demand, paused:
			fps = 1
		}
		if !demand && role != "spare" {
			w, h = 480, 270 // basta para la miniatura
		}
		adj := defaultAdjust()
		name := ""
		if d := s.Devices[p.id]; d != nil {
			adj, name = d.Adjust, d.Name
		}
		jobs = append(jobs, job{p, map[string]any{
			"t": "config", "role": role, "w": w, "h": h, "fps": fps, "adjust": adj,
			"name": name, "pc": e.pcName, "frozen": e.frozen, "covered": e.covered,
			"watched": len(e.consumers) > 0,
		}})
	}
	e.mu.Unlock()
	for _, j := range jobs {
		b, _ := json.Marshal(j.msg)
		j.p.mu.Lock()
		same := string(b) == j.p.sentCfg
		j.p.sentCfg = string(b)
		j.p.mu.Unlock()
		if !same || force {
			j.p.t.send(json.RawMessage(b))
		}
	}
}

// applyOutput fija el tamaño de salida: el elegido en ajustes o, en «automático», el
// mayor que pida alguna de las apps que usan la cámara.
func (e *Engine) applyOutput(s Settings, consumers []Consumer) {
	w, h := 1280, 720
	switch s.Resolution {
	case "1080":
		w, h = 1920, 1080
	case "540":
		w, h = 960, 540
	case "auto":
		best := 0
		for _, c := range consumers {
			if c.Width*c.Height > best && c.Width >= 160 && c.Height >= 120 {
				best, w, h = c.Width*c.Height, c.Width, c.Height
			}
		}
	}
	w, h = evenSize(w, h)
	e.mu.Lock()
	changed := w != e.outW || h != e.outH || s.FPS != e.fps
	e.outW, e.outH, e.fps = w, h, s.FPS
	if changed {
		e.canvas = nil
	}
	e.mu.Unlock()
	e.shm.SetFormat(w, h, s.FPS)
}

// ---------- compositor ----------

func (e *Engine) loop() {
	defer close(e.done)
	defer e.grab.release()
	var tick *time.Ticker
	var tickFPS int
	idle := time.NewTicker(500 * time.Millisecond)
	defer idle.Stop()
	for {
		s := getSettings()
		e.mu.Lock()
		screen := s.Scene.Kind != "phone" && (s.Scene.Kind == "screen" || s.Scene.Main == "screen" || s.Scene.Pip == "screen")
		fps := e.fps
		if !e.demandLocked(s) || e.frozen || e.covered {
			fps = 2
		}
		e.mu.Unlock()
		// Con la pantalla en la escena se compone a ritmo fijo; si no, al llegar cada
		// fotograma del móvil.
		var tc <-chan time.Time
		if screen {
			if tick == nil || tickFPS != fps {
				if tick != nil {
					tick.Stop()
				}
				tick, tickFPS = time.NewTicker(time.Second/time.Duration(fps)), fps
			}
			tc = tick.C
		} else if tick != nil {
			tick.Stop()
			tick = nil
		}
		select {
		case <-e.quit:
			if tick != nil {
				tick.Stop()
			}
			return
		case <-e.notify:
			if screen {
				continue // se recoge en el siguiente tic
			}
		case <-tc:
		case <-idle.C:
		}
		e.compose(s, screen)
	}
}

type srcFrame struct {
	img  *image.YCbCr
	jpeg []byte
	seq  uint64
	ok   bool // hay imagen
	gone bool // el móvil está en segundo plano
}

func (e *Engine) phoneFrameLocked(id string) srcFrame {
	p := e.phones[id]
	if p == nil {
		return srcFrame{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	// Una imagen de hace más de 3 s ya no vale (el móvil se ha parado).
	fresh := p.frame != nil && time.Since(p.frameAt) < 3*time.Second
	return srcFrame{img: p.frame, jpeg: p.jpeg, seq: p.seq, ok: fresh && p.visible, gone: !p.visible || !p.live}
}

func (e *Engine) compose(s Settings, withScreen bool) {
	e.mu.Lock()
	if e.covered {
		e.mu.Unlock()
		e.shm.SetState(tcStateCover)
		return
	}
	if e.frozen {
		e.mu.Unlock()
		return // la cámara repite el último fotograma
	}
	sc := s.Scene
	mainID := e.resolveLocked(sc.Main)
	if sc.Kind == "screen" {
		mainID = "screen"
	}
	pipID := ""
	if sc.Kind == "pip" {
		pipID = e.resolveLocked(sc.Pip)
		if pipID == mainID {
			pipID = ""
		}
	}
	main, pip := srcFrame{}, srcFrame{}
	if mainID != "screen" && mainID != "" {
		main = e.phoneFrameLocked(mainID)
	}
	if pipID != "" && pipID != "screen" {
		pip = e.phoneFrameLocked(pipID)
	}
	w, h := e.outW, e.outH
	if e.canvas == nil || e.canvas.Rect.Dx() != w || e.canvas.Rect.Dy() != h {
		e.canvas = newYUV(w, h)
	}
	canvas := e.canvas
	anyPhone := len(e.phones) > 0
	windowMode := e.windowMode
	e.mu.Unlock()

	if withScreen && (mainID == "screen" || pipID == "screen") {
		r := s.Scene.Region
		if r.W == 0 {
			r = e.primaryRegion()
		}
		img, err := e.grab.grab(r, s.Scene.Cursor)
		e.mu.Lock()
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		changed := msg != e.screenErr
		e.screenErr = msg
		e.mu.Unlock()
		if changed {
			e.changed()
		}
		f := srcFrame{img: img, ok: err == nil, seq: uint64(time.Now().UnixNano())}
		if mainID == "screen" {
			main = f
		} else {
			pip = f
		}
	}

	if !main.ok {
		st := uint32(tcStateWaiting)
		if main.gone && anyPhone && mainID != "screen" && e.phoneConnected(mainID) {
			st = tcStatePaused
		}
		e.shm.SetState(st)
		return
	}
	// ¿Ha cambiado algo desde la última vez?
	e.mu.Lock()
	same := !withScreen && e.composed[mainID] == main.seq && (pipID == "" || e.composed[pipID] == pip.seq)
	e.composed[mainID] = main.seq
	if pipID != "" {
		e.composed[pipID] = pip.seq
	}
	e.mu.Unlock()
	if same {
		e.shm.SetState(tcStateLive)
		return
	}

	// Lo normal: un móvil que ya manda el tamaño justo. Se copia tal cual y la vista
	// previa reutiliza su JPEG.
	if pipID == "" && main.jpeg != nil && main.img.Rect.Dx() == w && main.img.Rect.Dy() == h {
		e.shm.WriteFrame(w, h, func(dst []byte) { writeNV12(dst, main.img) })
		e.shm.SetState(tcStateLive)
		e.publishOut(main.jpeg, nil, windowMode)
		e.countOut()
		return
	}

	cover := true
	if mainID == "screen" {
		cover = false // la pantalla entera, con bandas si hace falta
	}
	drawScaled(canvas, canvas.Rect, main.img, cover, 0)
	if pip.ok {
		pw := int(float64(w)*sc.PipSize) &^ 1
		ph := (pw * h / w) &^ 1
		margin := (w / 40) &^ 1
		x, y := w-pw-margin, h-ph-margin
		switch sc.PipCorner {
		case "tl":
			x, y = margin, margin
		case "tr":
			y = margin
		case "bl":
			x = margin
		}
		drawScaled(canvas, image.Rect(x, y, x+pw, y+ph), pip.img, true, ph/10)
	}
	e.shm.WriteFrame(w, h, func(dst []byte) { writeNV12(dst, canvas) })
	e.shm.SetState(tcStateLive)
	e.publishOut(nil, canvas, windowMode)
	e.countOut()
}

// primaryRegion es la pantalla principal entera. Se mira como mucho cada 2 s (no en
// cada fotograma): basta para enterarse de que se ha cambiado de monitor.
func (e *Engine) primaryRegion() Region {
	if time.Since(e.primaryAt) < 2*time.Second && e.primary.W > 0 {
		return e.primary
	}
	e.primaryAt = time.Now()
	if ms := listMonitors(); len(ms) > 0 {
		m := ms[0]
		for _, x := range ms {
			if x.Primary {
				m = x
			}
		}
		e.primary = Region{X: m.X, Y: m.Y, W: m.W, H: m.H}
	}
	return e.primary
}

func (e *Engine) phoneConnected(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.phones[id]
	return ok
}

// publishOut alimenta la vista previa. Si hay que codificar, solo cuando alguien
// mira y como mucho 15 veces por segundo (a ritmo completo en modo ventana).
func (e *Engine) publishOut(jpg []byte, img *image.YCbCr, full bool) {
	if jpg != nil {
		e.mu.Lock()
		e.lastOut = jpg
		e.mu.Unlock()
		e.outHub.publish(jpg)
		return
	}
	now := time.Now()
	limit := time.Second / 15
	if full {
		limit = 0
	}
	if !e.outHub.watching() && now.Sub(e.lastEnc) < 2*time.Second {
		return // sin nadie mirando, una cada 2 s (para las fotos sin móvil)
	}
	if now.Sub(e.lastEnc) < limit {
		return
	}
	e.lastEnc = now
	b := encodeJPEG(img, 82)
	e.mu.Lock()
	e.lastOut = b
	e.mu.Unlock()
	e.outHub.publish(b)
}

func (e *Engine) countOut() {
	now := time.Now()
	e.mu.Lock()
	if e.outWin.IsZero() {
		e.outWin = now
	}
	e.outCount++
	if d := now.Sub(e.outWin); d >= time.Second {
		e.outFPS = float64(e.outCount) / d.Seconds()
		e.outWin, e.outCount = now, 0
	}
	e.mu.Unlock()
}

// ---------- mantenimiento: latido, apps que usan la cámara, perfiles ----------

func (e *Engine) housekeeping() {
	beat := time.NewTicker(250 * time.Millisecond)
	defer beat.Stop()
	n := 0
	for {
		select {
		case <-e.quit:
			return
		case <-beat.C:
		}
		e.shm.Beat()
		if n++; n%4 != 0 {
			continue
		}
		cons := e.shm.Consumers()
		sort := func(a, b Consumer) int { return strings.Compare(a.Exe+fmt.Sprint(a.PID), b.Exe+fmt.Sprint(b.PID)) }
		slices.SortFunc(cons, sort)
		key := fmt.Sprint(cons)
		e.mu.Lock()
		changed := key != e.consKey
		e.consumers, e.consKey = cons, key
		var fpsReset bool
		if time.Since(e.outWin) > 2*time.Second && e.outFPS != 0 {
			e.outFPS, fpsReset = 0, true
		}
		e.mu.Unlock()
		if changed {
			e.applyOutput(getSettings(), cons)
			e.autoProfiles(cons)
			e.sendConfigs(false)
			e.kick()
		}
		if changed || fpsReset {
			e.changed()
		}
		e.refreshStats()
	}
}

// refreshStats manda a la ventana los números de los móviles cada segundo.
func (e *Engine) refreshStats() {
	e.mu.Lock()
	n := len(e.phones)
	e.mu.Unlock()
	if n > 0 {
		e.changed()
	}
}

// autoProfiles pone el perfil de la primera app que lo tenga asignado y, cuando ya
// no queda ninguna, vuelve a dejarlo todo como estaba.
func (e *Engine) autoProfiles(cons []Consumer) {
	s := getSettings()
	var match *Profile
	var app string
	for _, c := range cons {
		exe := strings.ToLower(c.Exe)
		for i := range s.Profiles {
			if slices.Contains(s.Profiles[i].Apps, exe) {
				match, app = &s.Profiles[i], c.Exe
				break
			}
		}
		if match != nil {
			break
		}
	}
	e.mu.Lock()
	cur := e.autoProfile
	e.mu.Unlock()
	switch {
	case match != nil && match.ID != cur:
		e.mu.Lock()
		if e.restore == nil {
			main := e.resolveLocked(s.Scene.Main)
			rp := &restorePoint{scene: s.Scene, device: main}
			if d := s.Devices[main]; d != nil {
				rp.adjust = d.Adjust
			}
			e.restore = rp
		}
		e.autoProfile = match.ID
		e.mu.Unlock()
		e.applyProfile(*match)
		e.toast("info", fmt.Sprintf("Perfil «%s» para %s", match.Name, strings.TrimSuffix(app, ".exe")))
	case match == nil && cur != "":
		e.mu.Lock()
		rp := e.restore
		e.restore, e.autoProfile = nil, ""
		e.mu.Unlock()
		if rp != nil {
			updateSettings(func(s *Settings) {
				s.Scene = rp.scene
				if d := s.Devices[rp.device]; d != nil {
					d.Adjust = rp.adjust
				}
			})
			e.toast("info", "Vuelve la configuración de antes")
		}
	}
}

// applyProfile pone la escena del perfil y sus retoques en el móvil principal.
func (e *Engine) applyProfile(p Profile) {
	e.mu.Lock()
	main := e.resolveLocked(p.Scene.Main)
	e.mu.Unlock()
	updateSettings(func(s *Settings) {
		s.Scene = p.Scene
		if main != "" && main != "screen" {
			d := s.Devices[main]
			if d == nil {
				d = &Device{}
				s.Devices[main] = d
			}
			d.Adjust = p.Adjust
		}
	})
	e.sendConfigs(false)
	e.kick()
	e.changed()
}

// ---------- acciones ----------

func (e *Engine) SetFrozen(on bool) {
	e.mu.Lock()
	e.frozen = on
	e.mu.Unlock()
	e.sendConfigs(false)
	e.kick()
	e.changed()
}

func (e *Engine) SetCovered(on bool) {
	e.mu.Lock()
	e.covered = on
	e.mu.Unlock()
	if !on {
		e.mu.Lock()
		e.composed = map[string]uint64{} // volver a publicar aunque no haya fotograma nuevo
		e.mu.Unlock()
	}
	e.sendConfigs(false)
	e.kick()
	e.changed()
}

func (e *Engine) SetUIVisible(v bool) {
	e.mu.Lock()
	changed := e.uiVisible != v
	e.uiVisible = v
	e.mu.Unlock()
	if changed {
		e.sendConfigs(false)
		e.kick()
	}
}

func (e *Engine) SetWindowMode(v bool) {
	e.mu.Lock()
	e.windowMode = v
	e.mu.Unlock()
	e.sendConfigs(false)
	e.kick()
}

// SceneChanged se llama al cambiar la escena o la resolución desde la ventana.
func (e *Engine) SceneChanged() {
	e.mu.Lock()
	cons := e.consumers
	e.composed = map[string]uint64{}
	e.mu.Unlock()
	e.applyOutput(getSettings(), cons)
	e.sendConfigs(false)
	e.kick()
	e.changed()
}

// TakePhoto guarda una foto. Con un móvil de protagonista le pide una a toda
// resolución; si no responde (o no hay móvil), guarda lo que sale por la cámara.
func (e *Engine) TakePhoto() (string, error) {
	s := getSettings()
	e.mu.Lock()
	var p *phone
	if s.Scene.Kind != "screen" {
		p = e.phones[e.resolveLocked(s.Scene.Main)]
	}
	e.mu.Unlock()
	var data []byte
	if p != nil {
		select { // descartar una foto vieja que llegara tarde
		case <-p.photo:
		default:
		}
		if p.t.send(map[string]any{"t": "photo"}) == nil {
			select {
			case data = <-p.photo:
			case <-time.After(5 * time.Second):
			}
		}
	}
	if data == nil {
		e.mu.Lock()
		data = e.lastOut
		e.mu.Unlock()
	}
	if data == nil {
		return "", errors.New("todavía no hay imagen")
	}
	dir := photoDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	base := "TurboCAM " + time.Now().Format("2006-01-02 15.04.05")
	path := filepath.Join(dir, base+".jpg")
	for i := 2; fileExists(path); i++ {
		path = filepath.Join(dir, fmt.Sprintf("%s (%d).jpg", base, i))
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ---------- estado para la ventana ----------

type EngineState struct {
	Phones      []PhoneInfo `json:"phones"`
	Consumers   []Consumer  `json:"consumers"`
	OutW        int         `json:"outW"`
	OutH        int         `json:"outH"`
	FPS         int         `json:"fps"`
	OutFPS      float64     `json:"outFps"`
	Frozen      bool        `json:"frozen"`
	Covered     bool        `json:"covered"`
	AutoProfile string      `json:"autoProfile"`
	ScreenError string      `json:"screenError"`
	Status      string      `json:"status"` // waiting | live | paused | frozen | covered
}

func (e *Engine) State() EngineState {
	s := getSettings()
	e.mu.Lock()
	defer e.mu.Unlock()
	st := EngineState{
		Consumers: append([]Consumer{}, e.consumers...), OutW: e.outW, OutH: e.outH, FPS: e.fps,
		OutFPS: e.outFPS, Frozen: e.frozen, Covered: e.covered, ScreenError: e.screenErr,
		Phones: []PhoneInfo{},
	}
	if p := findProfile(s, e.autoProfile); p != nil {
		st.AutoProfile = p.Name
	}
	for _, p := range e.phoneList() {
		role, _, _ := e.roleLocked(s, p.id)
		p.mu.Lock()
		info := PhoneInfo{
			ID: p.id, Name: p.name, Model: p.model, Via: p.via, Visible: p.visible, Live: p.live, Error: p.errMsg,
			Battery: p.battery, Charging: p.charging, FPS: p.fps, Kbps: p.kbps, Quality: p.quality, RTT: p.rtt,
			Role: role, Caps: p.caps, Connected: p.connected.UnixMilli(),
		}
		if time.Since(p.frameAt) > 2*time.Second {
			info.FPS, info.Kbps = 0, 0
		}
		if p.frame != nil {
			info.Width, info.Height = p.frame.Rect.Dx(), p.frame.Rect.Dy()
		}
		p.mu.Unlock()
		if d := s.Devices[p.id]; d != nil {
			info.Adjust = d.Adjust
			if d.Name != "" {
				info.Name = d.Name
			}
		} else {
			info.Adjust = defaultAdjust()
		}
		st.Phones = append(st.Phones, info)
	}
	switch {
	case e.covered:
		st.Status = "covered"
	case e.frozen:
		st.Status = "frozen"
	default:
		st.Status = "waiting"
		main := e.resolveLocked(s.Scene.Main)
		if s.Scene.Kind == "screen" || main == "screen" {
			st.Status = "live"
		} else if p := e.phones[main]; p != nil {
			p.mu.Lock()
			if p.visible && p.live && time.Since(p.frameAt) < 3*time.Second {
				st.Status = "live"
			} else if !p.visible || !p.live {
				st.Status = "paused"
			}
			p.mu.Unlock()
		}
	}
	return st
}

func findProfile(s Settings, id string) *Profile {
	for i := range s.Profiles {
		if s.Profiles[i].ID == id {
			return &s.Profiles[i]
		}
	}
	return nil
}

func (e *Engine) phoneHub(id string) *jpegHub {
	e.mu.Lock()
	defer e.mu.Unlock()
	if p := e.phones[id]; p != nil {
		return p.hub
	}
	return nil
}
