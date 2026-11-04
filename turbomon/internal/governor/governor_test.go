package governor

import (
	"math/rand"
	"testing"
)

// Juego limitado a 144 FPS que solo necesita 3200 MHz para llegar: el
// controlador tiene que bajar hasta cerca de 3200 y no pasarse.
func TestCappedGame(t *testing.T) {
	c := New()
	r := rand.New(rand.NewSource(1))
	cap := 0
	mins := 5000
	for s := 0; s < 600; s++ {
		mhz := 4700
		if cap > 0 {
			mhz = cap
		}
		fps := 144.0
		p99 := 8.2 + r.Float64()*0.4
		if mhz < 3200 {
			fps = 144 * float64(mhz) / 3200
			p99 = 1000 / fps * 1.4
		}
		cap, _ = c.Step(Input{FPS: fps, P99: p99, Cap: 144, GPU: 70, MHz: mhz})
		if cap > 0 && cap < mins {
			mins = cap
		}
	}
	t.Logf("tope final %d MHz, el más bajo %d, bajadas %d, subidas %d", cap, mins, c.Down, c.Up)
	if cap == 0 || cap > 3600 {
		t.Fatalf("no ha bajado lo bastante: %d", cap)
	}
	if mins < 2900 {
		t.Fatalf("se ha pasado: %d", mins)
	}
}

// Juego que necesita todo el procesador (la gráfica va sobrada): nunca hay tope.
func TestCPUBound(t *testing.T) {
	c := New()
	for s := 0; s < 300; s++ {
		cap, _ := c.Step(Input{FPS: 120, P99: 11, GPU: 60, MHz: 4700})
		if cap != 0 {
			t.Fatalf("ha puesto tope con el procesador al límite: %d", cap)
		}
	}
}

// Limitado por la gráfica: baja, y si la escena cambia y hace falta procesador, quita el tope.
func TestGPUBoundThenCPU(t *testing.T) {
	c := New()
	cap := 0
	for s := 0; s < 200; s++ {
		cap, _ = c.Step(Input{FPS: 90, P99: 13, GPU: 99, MHz: 4700})
	}
	if cap == 0 {
		t.Fatal("no ha bajado con la gráfica al 99 %")
	}
	cap, _ = c.Step(Input{FPS: 80, P99: 16, GPU: 80, MHz: cap})
	if cap != 0 {
		t.Fatalf("no ha quitado el tope: %d", cap)
	}
}
