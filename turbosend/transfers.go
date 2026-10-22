package main

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Transfer es una descarga en curso (o reciente) vista desde el servidor.
type Transfer struct {
	ID      string
	ShareID string
	Label   string
	Via     string // cloudflare | direct | lan | local
	IP      string
	Total   int64
	Turbo   bool
	Dir     string // out (yo envío) | in (yo recibo)
	Who     string // nombre del invitado, si se sabe
	Room    string
	Started time.Time

	bytes    atomic.Int64
	active   atomic.Int32
	lastSeen atomic.Int64

	// solo los toca el muestreador
	prevBytes int64
	speed     float64
	status    string // active | done | stalled
	doneAt    time.Time
	counted   bool
}

func (t *Transfer) Add(n int64) {
	t.bytes.Add(n)
	t.lastSeen.Store(time.Now().UnixNano())
}

func (t *Transfer) Begin() { t.active.Add(1); t.lastSeen.Store(time.Now().UnixNano()) }
func (t *Transfer) End()   { t.active.Add(-1); t.lastSeen.Store(time.Now().UnixNano()) }

type TransferView struct {
	ID      string  `json:"id"`
	ShareID string  `json:"shareId"`
	Label   string  `json:"label"`
	Via     string  `json:"via"`
	IP      string  `json:"ip"`
	Total   int64   `json:"total"`
	Bytes   int64   `json:"bytes"`
	Speed   float64 `json:"speed"`
	Status  string  `json:"status"`
	Turbo   bool    `json:"turbo"`
	Dir     string  `json:"dir"`
	Who     string  `json:"who"`
	Room    string  `json:"room"`
	Started int64   `json:"started"`
}

type Tracker struct {
	mu        sync.Mutex
	items     map[string]*Transfer
	finished  map[string]bool
	totalOut  atomic.Int64
	prevOut   int64
	speed     float64
	history   []float64
	totalIn   atomic.Int64
	prevIn    int64
	speedIn   float64
	historyIn []float64
	onDone    func(t *Transfer)
	lastTick  time.Time
}

func newTracker(onDone func(t *Transfer)) *Tracker {
	return &Tracker{items: map[string]*Transfer{}, finished: map[string]bool{}, onDone: onDone, history: make([]float64, 0, 120)}
}

// Get devuelve la transferencia con esa clave o la crea.
func (tr *Tracker) Get(key string, mk func() *Transfer) *Transfer {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if t, ok := tr.items[key]; ok {
		if t.status == "stalled" {
			t.status = "active"
		}
		return t
	}
	t := mk()
	t.ID = key
	t.Started = time.Now()
	t.status = "active"
	t.lastSeen.Store(time.Now().UnixNano())
	tr.items[key] = t
	return t
}

func (tr *Tracker) Finish(key string) {
	tr.mu.Lock()
	t, ok := tr.items[key]
	tr.mu.Unlock()
	if ok {
		tr.markDone(t)
	}
}

func (tr *Tracker) markDone(t *Transfer) {
	tr.mu.Lock()
	if t.status == "done" {
		tr.mu.Unlock()
		return
	}
	t.status = "done"
	t.doneAt = time.Now()
	t.speed = 0
	first := !t.counted
	t.counted = true
	tr.mu.Unlock()
	if first && tr.onDone != nil {
		tr.onDone(t)
	}
}

// SetBytes recoloca el contador (p. ej. al reanudar con Range).
func (tr *Tracker) SetBytes(t *Transfer, v int64) {
	tr.mu.Lock()
	t.bytes.Store(v)
	t.prevBytes = v
	if t.status == "done" {
		t.status = "active"
		if v == 0 {
			t.counted = false
		}
	}
	tr.mu.Unlock()
}

// Out e In registran bytes salientes/entrantes para la gráfica global.
func (tr *Tracker) Out(n int64) { tr.totalOut.Add(n) }
func (tr *Tracker) In(n int64)  { tr.totalIn.Add(n) }

// Tick se llama cada ~500 ms y recalcula velocidades y estados.
func (tr *Tracker) Tick() {
	now := time.Now()
	tr.mu.Lock()
	dt := now.Sub(tr.lastTick).Seconds()
	if tr.lastTick.IsZero() || dt <= 0 {
		dt = 0.5
	}
	tr.lastTick = now

	out := tr.totalOut.Load()
	inst := float64(out-tr.prevOut) / dt
	tr.prevOut = out
	tr.speed = tr.speed*0.55 + inst*0.45
	if tr.speed < 1024 {
		tr.speed = 0
	}
	tr.history = append(tr.history, tr.speed)
	if len(tr.history) > 120 {
		tr.history = tr.history[len(tr.history)-120:]
	}
	in := tr.totalIn.Load()
	instIn := float64(in-tr.prevIn) / dt
	tr.prevIn = in
	tr.speedIn = tr.speedIn*0.55 + instIn*0.45
	if tr.speedIn < 1024 {
		tr.speedIn = 0
	}
	tr.historyIn = append(tr.historyIn, tr.speedIn)
	if len(tr.historyIn) > 120 {
		tr.historyIn = tr.historyIn[len(tr.historyIn)-120:]
	}

	var done []*Transfer
	for k, t := range tr.items {
		b := t.bytes.Load()
		if t.status != "done" {
			ti := float64(b-t.prevBytes) / dt
			t.speed = t.speed*0.55 + ti*0.45
			if t.speed < 1024 {
				t.speed = 0
			}
		}
		t.prevBytes = b
		idle := now.Sub(time.Unix(0, t.lastSeen.Load()))
		switch t.status {
		case "active":
			if !t.Turbo && t.Dir != "in" && t.Total > 0 && b >= t.Total && t.active.Load() == 0 {
				done = append(done, t)
			} else if t.active.Load() == 0 && idle > 12*time.Second {
				t.status = "stalled"
				t.speed = 0
			}
		case "stalled":
			if idle > 10*time.Minute {
				delete(tr.items, k)
			}
		case "done":
			if now.Sub(t.doneAt) > 2*time.Minute {
				delete(tr.items, k)
			}
		}
	}
	tr.mu.Unlock()
	for _, t := range done {
		tr.markDone(t)
	}
}

func (tr *Tracker) SpeedIn() (float64, []float64) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.speedIn, append([]float64(nil), tr.historyIn...)
}

// SpeedOf devuelve la velocidad actual de una transferencia concreta.
func (tr *Tracker) SpeedOf(key string) float64 {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if t, ok := tr.items[key]; ok && t.status == "active" {
		return t.speed
	}
	return 0
}

func (tr *Tracker) Snapshot() (list []TransferView, speed float64, hist []float64) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	for _, t := range tr.items {
		b := t.bytes.Load()
		if t.Total > 0 && b > t.Total {
			b = t.Total
		}
		list = append(list, TransferView{
			ID: t.ID, ShareID: t.ShareID, Label: t.Label, Via: t.Via, IP: t.IP,
			Total: t.Total, Bytes: b, Speed: t.speed, Status: t.status, Turbo: t.Turbo,
			Dir: t.Dir, Who: t.Who, Room: t.Room, Started: t.Started.UnixMilli(),
		})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Started > list[j].Started })
	hist = append([]float64(nil), tr.history...)
	return list, tr.speed, hist
}

func (tr *Tracker) ActiveCount() int {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	n := 0
	for _, t := range tr.items {
		if t.status == "active" {
			n++
		}
	}
	return n
}
