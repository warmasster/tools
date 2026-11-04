// Package governor baja la frecuencia máxima del procesador durante la
// partida mientras los fotogramas sigan saliendo a tiempo (la idea de
// PEGASUS y Rubik: no hace falta ir al máximo si el juego espera a la gráfica
// o a un límite de FPS). Si nota el más mínimo bajón, sube enseguida y no
// vuelve a bajar tanto.
package governor

import (
	"math"
	"sort"
)

// Input es un segundo de partida.
type Input struct {
	FPS float64
	P99 float64 // ms: el fotograma del percentil 99 de los últimos segundos
	Cap int     // límite de FPS detectado (0 = sin límite)
	GPU float64 // % de uso de la gráfica
	MHz int     // frecuencia del procesador medida
}

// Controller decide el tope de frecuencia (MHz; 0 = sin tope).
type Controller struct {
	StepMHz  int
	MinRatio float64 // no baja de esta fracción del máximo

	t         int
	maxMHz    int
	learn     []Input
	refFPS    float64
	refP99    float64
	mode      string // cap, gpu o ""
	modeCap   int
	capMHz    int // tope actual (0 = sin tope)
	floorMHz  int // por debajo de esto ya falló
	good      int
	holdUntil int
	// Lo que ha hecho (para el resumen).
	Down, Up int
}

// New prepara el controlador.
func New() *Controller { return &Controller{StepMHz: 200, MinRatio: 0.55} }

// Cap es el tope actual (0 = sin tope).
func (c *Controller) Cap() int { return c.capMHz }

// MaxMHz es la frecuencia máxima vista sin tope.
func (c *Controller) MaxMHz() int { return c.maxMHz }

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return s[len(s)/2]
}

func modeOf(in Input) string {
	switch {
	case in.Cap > 0 && in.FPS >= float64(in.Cap)*0.95:
		return "cap"
	case in.GPU >= 95:
		return "gpu"
	}
	return ""
}

func (c *Controller) reset(mode string, cap int) {
	c.mode, c.modeCap = mode, cap
	c.capMHz, c.good, c.learn = 0, 0, nil
	c.refFPS, c.refP99 = 0, 0
	c.holdUntil = c.t + 5
}

// Step recibe un segundo y devuelve el tope que hay que poner (MHz) y si ha cambiado.
func (c *Controller) Step(in Input) (int, bool) {
	c.t++
	prev := c.capMHz
	if c.capMHz == 0 && in.MHz > c.maxMHz {
		c.maxMHz = in.MHz
	}
	m := modeOf(in)
	if m == "" {
		// El procesador hace falta: sin tope.
		if c.mode != "" || c.capMHz != 0 {
			if c.capMHz != 0 {
				c.Up++
			}
			c.reset("", 0)
		}
		return c.capMHz, c.capMHz != prev
	}
	if m != c.mode || (m == "cap" && in.Cap != c.modeCap) {
		if c.capMHz != 0 {
			c.Up++
		}
		c.reset(m, in.Cap)
		return 0, prev != 0
	}
	// Primero se mide cómo va sin tope.
	if c.refFPS == 0 {
		if c.t < c.holdUntil {
			return c.capMHz, false
		}
		c.learn = append(c.learn, in)
		if len(c.learn) < 10 {
			return c.capMHz, false
		}
		var f, p []float64
		for _, x := range c.learn {
			f = append(f, x.FPS)
			p = append(p, x.P99)
		}
		c.refFPS, c.refP99 = median(f), median(p)
		c.learn = nil
	}
	ok := in.FPS >= c.refFPS*0.98 && in.P99 <= math.Max(c.refP99*1.12, c.refP99+1.5)
	if c.mode == "cap" {
		deadline := 1000 / float64(c.modeCap)
		ok = in.FPS >= float64(c.modeCap)*0.97 && in.P99 <= math.Max(deadline*1.25, c.refP99*1.12)
	}
	if !ok {
		c.good = 0
		if c.capMHz != 0 {
			// Bajón: arriba dos escalones y no volver a bajar de aquí.
			c.floorMHz = c.capMHz + c.StepMHz
			c.capMHz += 2 * c.StepMHz
			if c.maxMHz > 0 && c.capMHz >= c.maxMHz {
				c.capMHz = 0
			}
			c.Up++
			c.holdUntil = c.t + 30
		}
		return c.capMHz, c.capMHz != prev
	}
	c.good++
	if c.good < 8 || c.t < c.holdUntil || c.maxMHz == 0 {
		return c.capMHz, false
	}
	next := c.capMHz - c.StepMHz
	if c.capMHz == 0 {
		next = c.maxMHz - c.StepMHz
	}
	min := int(float64(c.maxMHz) * c.MinRatio)
	if next < min || (c.floorMHz > 0 && next < c.floorMHz) {
		return c.capMHz, false
	}
	c.capMHz = next
	c.good = 0
	c.Down++
	c.holdUntil = c.t + 6
	return c.capMHz, true
}
