package lab

import (
	"math/rand"
	"testing"
)

// Partida simulada: la escena cambia (los FPS bajan y suben solos) y el ajuste
// da un 5 % más. El experimento lo tiene que ver; con un ajuste que no hace
// nada, tiene que decir que da igual.
func simulate(gain float64, pairs int, seed int64) Result {
	const secs = 20
	r := rand.New(rand.NewSource(seed))
	order := Order(pairs, r)
	var blocks []Block
	for i, arm := range order {
		scene := 100 + 25*float64((i/4)%3) + r.NormFloat64()*3 // la escena cambia cada pocos bloques
		fps := scene
		if arm == 1 {
			fps *= 1 + gain
		}
		ms := make([]float32, 0, 4000)
		for k := 0; k < int(fps*secs); k++ {
			v := 1000/fps + r.NormFloat64()*0.6
			if r.Intn(500) == 0 {
				v *= 3
			}
			ms = append(ms, float32(v))
		}
		blocks = append(blocks, Measure(ms, secs, arm))
	}
	return Analyze(blocks)
}

func TestDetectsGain(t *testing.T) {
	hits := 0
	for seed := int64(1); seed <= 20; seed++ {
		if simulate(0.05, 14, seed).Verdict == "better" {
			hits++
		}
	}
	t.Logf("un 5 %% de mejora se ve en %d de 20 partidas", hits)
	if hits < 14 {
		t.Fatalf("poca potencia: %d/20", hits)
	}
	res := simulate(0.05, 14, 1)
	t.Logf("FPS %+v\n1%% %+v\ntirones %+v\n%s", res.FPS, res.Low1, res.Stutter, res.Verdict)
	if res.Verdict != "better" {
		t.Fatalf("veredicto %s", res.Verdict)
	}
	if res.FPS.Delta < 3 || res.FPS.Delta > 7 {
		t.Fatalf("delta %v", res.FPS.Delta)
	}
}

func TestPlacebo(t *testing.T) {
	same := 0
	for seed := int64(1); seed <= 20; seed++ {
		res := simulate(0, 14, seed)
		if res.Verdict == "same" {
			same++
		}
	}
	t.Logf("«da igual» en %d de 20", same)
	if same < 16 {
		t.Fatalf("demasiados falsos positivos: %d/20", 20-same)
	}
}

func TestSignFlip(t *testing.T) {
	if p := SignFlip([]float64{1, 1, 1, 1, 1, 1, 1, 1}); p > 0.01 {
		t.Fatalf("p %v", p)
	}
	if p := SignFlip([]float64{1, -1, 1, -1, 1, -1}); p < 0.5 {
		t.Fatalf("p %v", p)
	}
}
