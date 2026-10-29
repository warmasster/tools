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
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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
	quit      chan struct{}
	quitOnce  sync.Once

	// Decodificación aparte, en su propio hilo. Los JPEG son sueltos: basta el más
	// nuevo (el móvil comprime con varios hilos y alguno llega después de uno
	// posterior: ese se descarta). El H.264 hay que decodificarlo entero y en orden.
	dmu   sync.Mutex
	vq    []videoChunk
	wake  chan struct{}
	keyAt time.Time // última vez que se le pidió un fotograma clave

	rtc     *rtcPeer    // conexión WebRTC (con mu)
	rtcLive atomic.Bool // el vídeo llega por WebRTC

	// Reordenar el H.264 que llega por la conexión de siempre: por HTTP (POST) los
	// fotogramas pueden adelantarse unos a otros. Con dmu.
	nextH264 uint32
	anyH264  bool
	held     []videoChunk
	heldAt   time.Time

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
	camFPS   float64 // fotogramas por segundo que da la cámara del móvil
	camW     int
	camH     int
	hw       bool    // el móvil comprime por hardware
	bitrate  int     // kb/s que pide al codificador (H.264)
	path     string  // por dónde llega el vídeo: ws | http | rtc
	codec    string  // h264 | jpeg (lo último que ha llegado)
	decMs    float64 // lo que tarda el PC en decodificar cada fotograma
	thumbAt  time.Time
	rtcState string // cómo va WebRTC según el móvil
	frame    *image.YCbCr
	jpeg     []byte
	seq      uint64
	frameAt  time.Time
	winStart time.Time
	winCount int
	rxBytes  int
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
	CamFPS    float64         `json:"camFps"`
	CamW      int             `json:"camW"`
	CamH      int             `json:"camH"`
	Codec     string          `json:"codec"`
	HW        bool            `json:"hw"`
	Bitrate   int             `json:"bitrate"`
	DecMs     float64         `json:"decMs"`
	Path      string          `json:"path"`
	RTC       string          `json:"rtc"`
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
	rtc       *rtcHub // WebRTC (nil si no se pudo abrir el puerto UDP)
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
		name: name, model: model, battery: -1, quality: 0, quit: make(chan struct{}), wake: make(chan struct{}, 1)}
	p.path = "ws"
	if _, ok := t.(*httpTransport); ok {
		p.path = "http"
	}
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
	online := map[string]bool{}
	for k := range e.phones {
		online[k] = true
	}
	e.mu.Unlock()
	go e.decoder(p)
	if old != nil {
		old.t.send(map[string]any{"t": "bye", "reason": "replaced"})
		old.t.close()
		old.stop()
	}
	updateSettings(func(s *Settings) {
		d := s.Devices[id]
		if d == nil {
			d = adoptDevice(s, id, model, online)
		}
		if d == nil {
			d = &Device{Adjust: defaultAdjust()}
		}
		s.Devices[id] = d
		if d.Name == "" {
			d.Name = name
		}
		if model != "" {
			d.Model = model
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
	p.stop()
	e.rtcDrop(p, nil, false)
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

func (p *phone) stop() { p.quitOnce.Do(func() { close(p.quit) }) }

// Kick echa a un móvil (puede volver a entrar con el mismo enlace).
func (e *Engine) Kick(id string) { e.kickWith(id, "kicked") }

func (e *Engine) kickWith(id, reason string) {
	e.mu.Lock()
	p := e.phones[id]
	e.mu.Unlock()
	if p != nil {
		p.t.send(map[string]any{"t": "bye", "reason": reason})
		p.t.close()
		e.removePhone(p)
	}
}

// Forget: el móvil ha pedido que se le olvide. Se va y se borran sus ajustes.
func (e *Engine) Forget(p *phone) {
	e.kickWith(p.id, "forgotten")
	updateSettings(func(s *Settings) { delete(s.Devices, p.id) })
	e.toast("info", "Se ha olvidado «"+p.name+"» desde el propio móvil")
	e.changed()
}

// adoptDevice: un móvil nuevo puede ser uno de siempre que entra por otro enlace
// (cada enlace de Cloudflare es otro sitio para el navegador, que le da otra
// identidad). Si hay un único móvil guardado del mismo modelo y no está conectado,
// es él: se queda con sus ajustes, su nombre y su sitio en escenas y perfiles.
func adoptDevice(s *Settings, id, model string, online map[string]bool) *Device {
	if model == "" || model == "Móvil" {
		return nil
	}
	found := ""
	for k, d := range s.Devices {
		if k != id && d.Model == model && !online[k] {
			if found != "" {
				return nil // dos iguales: no se sabe cuál es
			}
			found = k
		}
	}
	if found == "" {
		return nil
	}
	d := s.Devices[found]
	delete(s.Devices, found)
	swap := func(sc *Scene) {
		if sc.Main == found {
			sc.Main = id
		}
		if sc.Pip == found {
			sc.Pip = id
		}
	}
	swap(&s.Scene)
	for i := range s.Profiles {
		swap(&s.Profiles[i].Scene)
	}
	return d
}

// videoChunk es un trozo de vídeo de un móvil: un JPEG o un fotograma H.264.
type videoChunk struct {
	kind byte // frameKindVideo (JPEG) | frameKindH264
	seq  uint32
	key  bool // H.264: fotograma clave (se puede empezar a decodificar por él)
	gap  bool // se ha perdido algo justo antes (por WebRTC)
	rtc  bool // ha llegado por WebRTC
	data []byte
}

// maxH264Queue: si el decodificador se queda tan atrás (no debería: tarda pocos
// milisegundos), se tira lo que hay y se pide un fotograma clave.
const maxH264Queue = 90

// queueFrame deja el fotograma para el decodificador. Un JPEG sustituye a los que
// hubiera esperando (no tiene sentido decodificar uno que ya es viejo).
func (e *Engine) queueFrame(p *phone, c videoChunk) {
	p.dmu.Lock()
	if c.kind == frameKindH264 && !c.rtc {
		p.reorderLocked(c)
		p.dmu.Unlock()
		p.mu.Lock()
		p.rxBytes += len(c.data)
		p.mu.Unlock()
		select {
		case p.wake <- struct{}{}:
		default:
		}
		return
	}
	if c.kind == frameKindVideo {
		p.vq = slices.DeleteFunc(p.vq, func(x videoChunk) bool { return x.kind == frameKindVideo })
	} else if len(p.vq) >= maxH264Queue {
		p.vq = p.vq[:0]
	}
	p.vq = append(p.vq, c)
	p.dmu.Unlock()
	p.mu.Lock()
	p.rxBytes += len(c.data)
	p.mu.Unlock()
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// reorderLocked pasa a la cola el H.264 de la conexión de siempre en su orden. Uno que
// se adelanta espera a los que faltan, como mucho 8 fotogramas o 300 ms (entonces se
// sigue sin ellos y el decodificador pide un clave). Un clave no espera a nadie.
func (p *phone) reorderLocked(c videoChunk) {
	push := func(x videoChunk) {
		if len(p.vq) >= maxH264Queue {
			p.vq = p.vq[:0]
		}
		p.vq = append(p.vq, x)
		p.nextH264, p.anyH264 = x.seq+1, true
	}
	switch d := int32(c.seq - p.nextH264); {
	case !p.anyH264 || d == 0 || (c.key && d > 0):
		push(c)
	case d < 0:
		return // repetido o tardísimo
	default:
		if len(p.held) == 0 {
			p.heldAt = time.Now()
		}
		p.held = append(p.held, c)
	}
	// Los que esperaban y ya tocan (o se ha esperado demasiado).
	for len(p.held) > 0 {
		slices.SortFunc(p.held, func(a, b videoChunk) int { return int(int32(a.seq - b.seq)) })
		h := p.held[0]
		if d := int32(h.seq - p.nextH264); d < 0 {
			p.held = p.held[1:]
			continue
		} else if d > 0 && len(p.held) < 8 && time.Since(p.heldAt) < 300*time.Millisecond {
			break
		}
		p.held = p.held[1:]
		push(h)
		p.heldAt = time.Now()
	}
}

// askKey pide al móvil un fotograma clave (como mucho dos veces por segundo).
func (e *Engine) askKey(p *phone) {
	p.dmu.Lock()
	if time.Since(p.keyAt) < 500*time.Millisecond {
		p.dmu.Unlock()
		return
	}
	p.keyAt = time.Now()
	p.dmu.Unlock()
	p.mu.Lock()
	peer := p.rtc
	p.mu.Unlock()
	if peer != nil {
		peer.pli()
	}
	p.t.send(map[string]any{"t": "key"})
}

func (e *Engine) decoder(p *phone) {
	// Media Foundation quiere siempre el mismo hilo, con COM iniciado.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var dec *h264Decoder
	defer func() {
		if dec != nil {
			dec.close()
		}
	}()
	var lastSeq uint32
	anySeq, waitKey, comOK, decFailed := false, true, false, false
	curRTC := false     // de dónde sale el H.264 que se está decodificando
	var lastH264 uint32 // número del último H.264 de la conexión de siempre
	for {
		select {
		case <-p.quit:
			return
		case <-p.wake:
		}
		for {
			p.dmu.Lock()
			if len(p.vq) == 0 {
				p.dmu.Unlock()
				break
			}
			c := p.vq[0]
			p.vq = p.vq[1:]
			if len(p.vq) == 0 {
				p.vq = nil // que no crezca para siempre por delante
			}
			p.dmu.Unlock()
			select {
			case <-p.quit:
				return
			default:
			}
			if c.kind == frameKindVideo {
				if anySeq && int32(c.seq-lastSeq) <= 0 {
					continue // llegó tarde: ya hay uno más nuevo
				}
				if img, err := decodeJPEG(c.data); err == nil {
					lastSeq, anySeq = c.seq, true
					e.onFrame(p, img, c.data, "jpeg", 0)
				}
				continue
			}
			if decFailed {
				continue
			}
			// Mientras se pasa de una conexión a otra llega vídeo por las dos: cada flujo
			// es una cadena distinta y no se pueden mezclar. Se cambia en un clave.
			if c.rtc != curRTC {
				if !c.key {
					if c.rtc {
						e.askKey(p)
					}
					continue
				}
				if !c.rtc && p.rtcLive.Load() {
					continue // aún llega por WebRTC: es uno rezagado
				}
				curRTC = c.rtc
				if dec != nil {
					dec.reset()
				}
				p.rtcLive.Store(c.rtc)
				if c.rtc {
					p.t.send(map[string]any{"t": "rtcok"})
				}
				e.changed()
			}
			// Por la conexión de siempre van numerados seguidos: un salto es que falta uno
			// (o, por HTTP, que llegan desordenados).
			if !c.rtc {
				if c.seq != lastH264+1 {
					c.gap = true
				}
				lastH264 = c.seq
			}
			if c.gap && !c.key {
				waitKey = true
			}
			if waitKey && !c.key {
				e.askKey(p)
				continue
			}
			if dec == nil {
				if !comOK {
					comInitThread()
					comOK = true
				}
				d, err := newH264Decoder()
				if err != nil {
					// Sin decodificador (Windows N sin el Media Feature Pack): JPEG.
					decFailed = true
					p.t.send(map[string]any{"t": "codec", "h264": false})
					continue
				}
				dec = d
			}
			t0 := time.Now()
			imgs, err := dec.decode(c.data)
			if err != nil {
				// Se ha roto la cadena (o el flujo no era válido): de nuevo desde un clave.
				dec.reset()
				waitKey = true
				e.askKey(p)
				continue
			}
			waitKey = false
			if len(imgs) > 0 {
				e.onFrame(p, imgs[len(imgs)-1], nil, "h264", float64(time.Since(t0).Microseconds())/1000)
			}
		}
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

// onFrame: ya hay imagen nueva de un móvil (jpg es su JPEG si llegó así). Se
// guarda y se avisa al compositor.
func (e *Engine) onFrame(p *phone, img *image.YCbCr, jpg []byte, codec string, decMs float64) {
	now := time.Now()
	p.mu.Lock()
	p.frame, p.jpeg, p.frameAt = img, jpg, now
	p.seq++
	p.codec = codec
	if decMs > 0 {
		if p.decMs == 0 {
			p.decMs = decMs
		}
		p.decMs = p.decMs*0.9 + decMs*0.1
	}
	if p.winStart.IsZero() {
		p.winStart = now
	}
	p.winCount++
	if d := now.Sub(p.winStart); d >= time.Second {
		p.fps = float64(p.winCount) / d.Seconds()
		p.kbps = float64(p.rxBytes) * 8 / 1000 / d.Seconds()
		p.winStart, p.winCount, p.rxBytes = now, 0, 0
	}
	thumb := jpg == nil && p.hub.watching() && now.Sub(p.thumbAt) >= time.Second/8
	if thumb {
		p.thumbAt = now
	}
	p.mu.Unlock()
	if jpg != nil {
		p.hub.publish(jpg)
	} else if thumb {
		p.hub.publish(thumbJPEG(img))
	}
	e.kick()
}

// thumbJPEG hace la miniatura de la ventana (como mucho 480 de ancho).
func thumbJPEG(img *image.YCbCr) []byte {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w > 480 {
		h, w = (h*480/w)&^1, 480
		small := newYUV(w, h)
		drawScaled(small, small.Rect, img, true, 0)
		img = small
	}
	return encodeJPEG(img, 75)
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
	Cam      *float64        `json:"cam"`
	HW       *bool           `json:"hw"`
	Bitrate  *int            `json:"kbps"`
	RTC      *string         `json:"rtc"`
	CamW     *int            `json:"camW"`
	CamH     *int            `json:"camH"`
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
		if m.Cam != nil {
			p.camFPS = *m.Cam
		}
		if m.CamW != nil && m.CamH != nil {
			p.camW, p.camH = *m.CamW, *m.CamH
		}
		if m.HW != nil {
			p.hw = *m.HW
		}
		if m.Bitrate != nil {
			p.bitrate = *m.Bitrate
		}
		if m.RTC != nil {
			p.rtcState = truncate(*m.RTC, 120)
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
		adj := defaultAdjust()
		name := ""
		if d := s.Devices[p.id]; d != nil {
			adj, name = d.Adjust, d.Name
		}
		if !demand && role != "spare" {
			w, h = 480, 270 // basta para la miniatura
		} else if role != "spare" {
			// Lo que se haya elegido para este móvil: menos resolución o fotogramas.
			if adj.FPS > 0 && adj.FPS < fps {
				fps = adj.FPS
			}
			if adj.Res > 0 && adj.Res < h {
				w, h = (w*adj.Res/h+1)&^1, adj.Res
			}
		}
		jobs = append(jobs, job{p, map[string]any{
			"t": "config", "role": role, "w": w, "h": h, "fps": fps, "adjust": adj,
			"name": name, "pc": e.pcName, "frozen": e.frozen, "covered": e.covered,
			"watched": len(e.consumers) > 0, "h264": h264OK(), "rtc": e.rtc != nil,
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
	case "480":
		w, h = 854, 480
	case "360":
		w, h = 640, 360
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
	if pipID == "" && main.img.Rect.Dx() == w && main.img.Rect.Dy() == h {
		e.shm.WriteFrame(w, h, func(dst []byte) { writeNV12(dst, main.img) })
		e.shm.SetState(tcStateLive)
		e.publishOut(main.jpeg, main.img, windowMode)
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
	if jpg != nil { // ya viene en JPEG del móvil: no hay que codificar nada
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
		e.rtcWatch()
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
			CamFPS: p.camFPS, CamW: p.camW, CamH: p.camH,
			Codec: p.codec, HW: p.hw, Bitrate: p.bitrate, DecMs: p.decMs, Path: p.path, RTC: p.rtcState,
			Role: role, Caps: p.caps, Connected: p.connected.UnixMilli(),
		}
		if time.Since(p.frameAt) > 2*time.Second {
			info.FPS, info.Kbps, info.CamFPS = 0, 0, 0
		}
		if p.frame != nil {
			info.Width, info.Height = p.frame.Rect.Dx(), p.frame.Rect.Dy()
		}
		p.mu.Unlock()
		if p.rtcLive.Load() {
			info.Path = "rtc"
		}
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
