// Package stutter caza los tirones de una partida: cada fotograma que tarda
// mucho más de lo normal, a qué hora exacta pasó, quién estaba haciendo algo
// raro justo en ese segundo (otro programa, el disco, los drivers, la
// temperatura, un vídeo en la otra pantalla) y si se repiten cada X segundos,
// que es la firma de una tarea de fondo.
//
// No guarda nada por cada proceso y segundo: solo contadores, así que una
// partida de horas ocupa lo mismo que una de minutos.
package stutter

import (
	"math"
	"sort"
	"strings"
)

// Proc es lo que gastó un programa (que no es el juego) en un segundo.
type Proc struct {
	Name   string
	CPU    float64 // % del equipo
	ReadMB float64 // MB/s leídos
}

// Context es lo que pasaba en el equipo durante un segundo.
type Context struct {
	Procs    []Proc
	GameHard uint32  // páginas que el juego tuvo que leer del disco
	GameRead float64 // MB/s que leyó el juego
	DPC      float64 // % del núcleo más castigado por drivers (DPC + interrupciones)
	Limited  bool    // el procesador iba frenado (temperatura o potencia)
	Decode   bool    // otro programa estaba reproduciendo vídeo
}

type sample struct {
	t0, t1  int64
	ctx     Context
	hitches int
}

type cand struct {
	sum    float64 // para la media (CPU o lo que toque)
	spikes int
	hits   int
}

// Analyzer analiza una partida.
type Analyzer struct {
	freq    float64
	start   int64
	lastT   int64
	ring    []sample
	pending []int64

	recent [64]float32
	nRec   int
	posRec int
	med    float32
	inHit  bool

	Secs        int
	StutterSecs int
	Hitches     int
	Worst       float32
	times       []uint32

	procs  map[string]*cand
	disk   cand
	dpc    cand
	limit  cand
	decode cand
}

// New empieza una partida. freq y start son del reloj de los fotogramas.
func New(freq, start int64) *Analyzer {
	return &Analyzer{freq: float64(freq), start: start, lastT: start, procs: map[string]*cand{}}
}

func (a *Analyzer) median() float32 {
	n := a.nRec
	if n > len(a.recent) {
		n = len(a.recent)
	}
	if n < 8 {
		return 0
	}
	tmp := make([]float32, n)
	copy(tmp, a.recent[:n])
	sort.Slice(tmp, func(i, j int) bool { return tmp[i] < tmp[j] })
	return tmp[n/2]
}

// IsHitch dice si un fotograma de ms es un tirón comparado con lo normal (med).
// Tiene que tardar el doble y al menos 8 ms más: a 60 FPS es perder un
// fotograma; a 144, un salto que se ve.
func IsHitch(ms, med float32) bool {
	return med > 0 && med < 100 && ms > 2*med && ms-med >= 8 && ms < 2000
}

// Frames recibe los fotogramas nuevos: su duración (ms) y cuándo acabaron.
func (a *Analyzer) Frames(ms []float32, ts []int64) {
	for i, v := range ms {
		if a.nRec%16 == 0 || a.med == 0 {
			a.med = a.median()
		}
		if IsHitch(v, a.med) {
			if !a.inHit {
				a.Hitches++
				if i < len(ts) {
					a.pending = append(a.pending, ts[i])
				}
			}
			if v > a.Worst {
				a.Worst = v
			}
			a.inHit = true
			continue // el tirón no entra en «lo normal»
		}
		a.inHit = false
		a.recent[a.posRec] = v
		a.posRec = (a.posRec + 1) % len(a.recent)
		a.nRec++
	}
}

// Skip olvida lo normal de antes (tras volver al juego desde el escritorio).
func (a *Analyzer) Skip(now int64) {
	a.nRec, a.posRec, a.med, a.inHit = 0, 0, 0, false
	a.pending = a.pending[:0]
	a.ring = a.ring[:0]
	a.lastT = now
}

// Second cierra un segundo de partida (hasta now) con lo que pasaba en el equipo.
func (a *Analyzer) Second(now int64, ctx Context) {
	a.ring = append(a.ring, sample{t0: a.lastT, t1: now, ctx: ctx})
	a.lastT = now
	// Cada tirón va al segundo en que pasó.
	keep := a.pending[:0]
	for _, ts := range a.pending {
		placed := false
		for i := range a.ring {
			if ts > a.ring[i].t0 && ts <= a.ring[i].t1 {
				a.ring[i].hitches++
				placed = true
				break
			}
		}
		if !placed && ts > now {
			keep = append(keep, ts) // aún no ha llegado su segundo
		}
	}
	a.pending = keep
	// Los eventos de los fotogramas llegan con hasta 1 s de retraso: un segundo
	// se da por cerrado 2,5 s después.
	lag := int64(2.5 * a.freq)
	n := 0
	for _, s := range a.ring {
		if now-s.t1 >= lag {
			a.finalize(s)
			n++
		} else {
			break
		}
	}
	a.ring = append(a.ring[:0], a.ring[n:]...)
}

func (a *Analyzer) finalize(s sample) {
	a.Secs++
	hit := s.hitches > 0
	if hit {
		a.StutterSecs++
		if len(a.times) < 3000 {
			a.times = append(a.times, uint32(float64(s.t1-a.start)/a.freq))
		}
	}
	mark := func(c *cand, spike bool) {
		if spike {
			c.spikes++
			if hit {
				c.hits++
			}
		}
	}
	// Otros programas: se juntan por nombre (Chrome son veinte procesos).
	by := map[string]Proc{}
	for _, p := range s.ctx.Procs {
		k := strings.ToLower(p.Name)
		q := by[k]
		q.Name = p.Name
		q.CPU += p.CPU
		q.ReadMB += p.ReadMB
		by[k] = q
	}
	for k, p := range by {
		c := a.procs[k]
		if c == nil {
			if len(a.procs) > 400 {
				continue
			}
			c = &cand{}
			a.procs[k] = c
		}
		avg := c.sum / math.Max(1, float64(a.Secs-1))
		spike := (p.CPU >= 1 && p.CPU >= 2.5*avg) || p.ReadMB >= 20
		c.sum += p.CPU
		mark(c, spike)
	}
	mark(&a.disk, s.ctx.GameHard >= 100 || s.ctx.GameRead >= 50)
	avg := a.dpc.sum / math.Max(1, float64(a.Secs-1))
	mark(&a.dpc, s.ctx.DPC >= 3 && s.ctx.DPC >= 3*avg)
	a.dpc.sum += s.ctx.DPC
	mark(&a.limit, s.ctx.Limited)
	mark(&a.decode, s.ctx.Decode)
}

// Suspect es un posible culpable.
type Suspect struct {
	Kind  string  `json:"kind"`           // proc, disk, dpc, limit, decode
	Name  string  `json:"name,omitempty"` // el programa
	Share float64 `json:"share"`          // % de los tirones en que estaba
	Lift  float64 `json:"lift"`           // cuántas veces más probable es un tirón cuando está
	Hits  int     `json:"hits"`
}

// Split compara los tirones por minuto con y sin algo (un vídeo en la otra pantalla).
type Split struct {
	With     float64 `json:"with"`
	Without  float64 `json:"without"`
	WithSecs int     `json:"withSecs"`
}

// Report es el análisis de la partida.
type Report struct {
	Secs        int       `json:"secs"`
	Hitches     int       `json:"hitches"`
	StutterSecs int       `json:"stutterSecs"`
	PerMin      float64   `json:"perMin"`
	Worst       float64   `json:"worst"` // ms
	Period      int       `json:"period,omitempty"`
	PeriodFit   float64   `json:"periodFit,omitempty"` // % de tirones que caen en el ritmo
	Times       []uint32  `json:"times,omitempty"`     // segundo de partida de cada tirón
	Suspects    []Suspect `json:"suspects,omitempty"`
	Decode      *Split    `json:"decode,omitempty"`
}

// Report resume lo que va de partida.
func (a *Analyzer) Report() Report {
	r := Report{Secs: a.Secs, Hitches: a.Hitches, StutterSecs: a.StutterSecs, Worst: math.Round(float64(a.Worst)*10) / 10}
	if a.Secs > 0 {
		r.PerMin = math.Round(float64(a.Hitches)/float64(a.Secs)*60*10) / 10
	}
	r.Times = append([]uint32(nil), a.times...)
	r.Period, r.PeriodFit = Period(a.times, a.Secs)
	if a.Secs < 30 || a.StutterSecs < 3 {
		return r
	}
	base := float64(a.StutterSecs) / float64(a.Secs)
	judge := func(kind, name string, c *cand) {
		if c.spikes < 3 || c.hits < 3 {
			return
		}
		p := float64(c.hits) / float64(c.spikes)
		lift := p / base
		share := float64(c.hits) / float64(a.StutterSecs) * 100
		if lift >= 2 && p >= 0.25 && share >= 10 {
			r.Suspects = append(r.Suspects, Suspect{Kind: kind, Name: name, Share: math.Round(share), Lift: math.Round(lift*10) / 10, Hits: c.hits})
		}
	}
	for k, c := range a.procs {
		if k == "dwm.exe" || k == "turbomon.exe" {
			continue // DWM trabaja más justo porque hay tirón; TurboMON es quien mide
		}
		judge("proc", k, c)
	}
	judge("disk", "", &a.disk)
	judge("dpc", "", &a.dpc)
	judge("limit", "", &a.limit)
	judge("decode", "", &a.decode)
	sort.Slice(r.Suspects, func(i, j int) bool {
		return r.Suspects[i].Share*r.Suspects[i].Lift > r.Suspects[j].Share*r.Suspects[j].Lift
	})
	if len(r.Suspects) > 4 {
		r.Suspects = r.Suspects[:4]
	}
	if d := a.decode; d.spikes >= 60 && a.Secs-d.spikes >= 60 {
		with := float64(d.hits) / float64(d.spikes) * 60
		without := float64(a.StutterSecs-d.hits) / float64(a.Secs-d.spikes) * 60
		r.Decode = &Split{With: math.Round(with*10) / 10, Without: math.Round(without*10) / 10, WithSecs: d.spikes}
	}
	return r
}

// Period busca un ritmo en los tirones: si muchos caen cada N segundos justos
// (con ±1 s de margen), es una tarea de fondo con temporizador. Devuelve N y
// qué parte de los tirones lo sigue.
func Period(times []uint32, secs int) (int, float64) {
	// Los tirones seguidos (a menos de 3 s) son un mismo episodio.
	var ev []uint32
	for i, t := range times {
		if i == 0 || t-times[i-1] > 2 {
			ev = append(ev, t)
		}
	}
	times = ev
	n := len(times)
	if n < 6 || secs < 60 {
		return 0, 0
	}
	set := make(map[uint32]bool, n)
	for _, t := range times {
		set[t] = true
	}
	rate := float64(n) / float64(secs)
	baseline := 1 - math.Pow(1-rate, 3)
	if baseline > 0.25 {
		return 0, 0 // tantos tirones que cualquier ritmo encajaría
	}
	type sc struct {
		l     int
		score float64
		exact int // coincidencias sin margen (para afinar el ritmo)
	}
	var scores []sc
	best := 0.0
	maxL := secs / 3
	if maxL > 300 {
		maxL = 300
	}
	for l := 5; l <= maxL; l++ {
		match, total, exact := 0, 0, 0
		for _, t := range times {
			if int(t)+l+1 > secs {
				continue
			}
			total++
			u := t + uint32(l)
			if set[u] {
				exact++
			}
			if set[u] || set[u-1] || set[u+1] {
				match++
			}
		}
		if total < 5 {
			continue
		}
		s := float64(match) / float64(total)
		scores = append(scores, sc{l, s, exact})
		if s > best {
			best = s
		}
	}
	if best < 0.45 || best < 2.5*baseline {
		return 0, 0
	}
	// El ritmo de verdad es el más corto que encaja casi tan bien como el mejor
	// (los múltiplos también encajan); con el margen de ±1 s, se elige el mejor
	// de sus vecinos.
	for i, s := range scores {
		if s.score >= 0.85*best {
			// Con el margen, un ritmo de N encaja igual en N-1, N y N+1: se
			// elige el que más coincide al segundo justo.
			pick := s
			for _, v := range scores[i:] {
				if v.l > s.l+2 {
					break
				}
				if v.exact > pick.exact {
					pick = v
				}
			}
			return pick.l, math.Round(math.Max(pick.score, s.score) * 100)
		}
	}
	return 0, 0
}
