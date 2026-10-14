package main

import (
	"encoding/json"
	"sync"
	"time"
)

// Hub reparte eventos en tiempo real a los navegadores de cada chat mediante
// long-polling: los túneles temporales de Cloudflare retienen las conexiones SSE,
// así que cada navegador pregunta "¿hay algo después de X?" y la respuesta sale en
// cuanto ocurre algo (o a los 25 s si no pasa nada).

type hubEvent struct {
	Seq  int64           `json:"seq"`
	To   string          `json:"-"` // "" = para todos, si no, pid
	Name string          `json:"name"`
	Data json.RawMessage `json:"data"`
}

type roomFeed struct {
	seq      int64
	events   []hubEvent
	notify   chan struct{}
	pending  map[string]int       // pid -> peticiones esperando
	lastSeen map[string]time.Time // pid -> última vez que preguntó
	online   map[string]bool      // último estado notificado
}

const feedKeep = 400

type Hub struct {
	mu         sync.Mutex
	feeds      map[string]*roomFeed
	typing     map[string]map[string]time.Time // room -> pid/"host" -> hasta cuándo
	onPresence func(roomID string)
}

func newHub() *Hub {
	h := &Hub{feeds: map[string]*roomFeed{}, typing: map[string]map[string]time.Time{}}
	go h.sweep()
	return h
}

func (h *Hub) feed(room string) *roomFeed {
	f := h.feeds[room]
	if f == nil {
		f = &roomFeed{notify: make(chan struct{}), pending: map[string]int{}, lastSeen: map[string]time.Time{}, online: map[string]bool{}}
		h.feeds[room] = f
	}
	return f
}

func (h *Hub) push(room, to, name string, data any) {
	b, _ := json.Marshal(data)
	h.mu.Lock()
	f := h.feed(room)
	f.seq++
	f.events = append(f.events, hubEvent{Seq: f.seq, To: to, Name: name, Data: b})
	if len(f.events) > feedKeep {
		f.events = f.events[len(f.events)-feedKeep:]
	}
	close(f.notify)
	f.notify = make(chan struct{})
	h.mu.Unlock()
}

func (h *Hub) Broadcast(room, event string, data any) { h.push(room, "", event, data) }

func (h *Hub) SendTo(room, pid, event string, data any) bool {
	h.push(room, pid, event, data)
	return h.isOnline(room, pid)
}

// Since devuelve los eventos posteriores a since para ese invitado.
// reset = true si se ha perdido historial (o la app se reinició) y hay que recargar todo.
func (h *Hub) Since(room, pid string, since int64) (evs []hubEvent, seq int64, wait <-chan struct{}, reset bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.feed(room)
	seq = f.seq
	if since > f.seq || (len(f.events) > 0 && since < f.events[0].Seq-1) {
		return nil, seq, nil, true
	}
	for _, e := range f.events {
		if e.Seq > since && (e.To == "" || e.To == pid) {
			evs = append(evs, e)
		}
	}
	return evs, seq, f.notify, false
}

// Enter/Leave marcan una petición de sondeo abierta (para saber quién está conectado).
func (h *Hub) Enter(room, pid string) {
	h.mu.Lock()
	f := h.feed(room)
	f.pending[pid]++
	f.lastSeen[pid] = time.Now()
	changed := !f.online[pid]
	f.online[pid] = true
	h.mu.Unlock()
	if changed && h.onPresence != nil {
		h.onPresence(room)
	}
}

func (h *Hub) Leave(room, pid string) {
	h.mu.Lock()
	f := h.feed(room)
	f.pending[pid]--
	if f.pending[pid] <= 0 {
		delete(f.pending, pid)
	}
	f.lastSeen[pid] = time.Now()
	h.mu.Unlock()
}

func (h *Hub) isOnline(room, pid string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.feeds[room]
	return f != nil && f.online[pid]
}

// sweep da por desconectado a quien lleva un rato sin preguntar.
func (h *Hub) sweep() {
	for range time.Tick(3 * time.Second) {
		var changed []string
		h.mu.Lock()
		for room, f := range h.feeds {
			for pid, on := range f.online {
				if on && f.pending[pid] == 0 && time.Since(f.lastSeen[pid]) > 12*time.Second {
					delete(f.online, pid)
					changed = append(changed, room)
				}
			}
		}
		h.mu.Unlock()
		for _, r := range changed {
			if h.onPresence != nil {
				h.onPresence(r)
			}
		}
	}
}

// Online devuelve los invitados con el chat abierto ahora mismo.
func (h *Hub) Online(room string) map[string]bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := map[string]bool{}
	if f := h.feeds[room]; f != nil {
		for pid, on := range f.online {
			if on {
				out[pid] = true
			}
		}
	}
	return out
}

func (h *Hub) Disconnect(room string) {
	h.Broadcast(room, "deleted", true)
}

// SetTyping marca a alguien como "escribiendo…" durante unos segundos.
func (h *Hub) SetTyping(room, who string) {
	h.mu.Lock()
	if h.typing[room] == nil {
		h.typing[room] = map[string]time.Time{}
	}
	h.typing[room][who] = time.Now().Add(4 * time.Second)
	h.mu.Unlock()
}

func (h *Hub) ClearTyping(room, who string) {
	h.mu.Lock()
	if h.typing[room] != nil {
		delete(h.typing[room], who)
	}
	h.mu.Unlock()
}

func (h *Hub) Typing(room string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	now := time.Now()
	for who, until := range h.typing[room] {
		if until.After(now) {
			out = append(out, who)
		} else {
			delete(h.typing[room], who)
		}
	}
	return out
}
