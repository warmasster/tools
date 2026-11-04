// Package lab hace experimentos de verdad con los ajustes de los juegos:
// alterna el ajuste encendido y apagado en bloques cortos y al azar mientras
// se juega («switchback», como hacen las empresas con sus servidores) y
// compara con una prueba de permutaciones. Así se sabe si un ajuste mejora,
// empeora o no cambia nada, sin depender de la sensación de quien juega.
package lab

import (
	"math"
	"math/rand"
	"sort"

	"turbomon/internal/stutter"
)

// Block es un bloque medido.
type Block struct {
	Arm    int     `json:"arm"` // 0 = como estaba, 1 = con el ajuste
	FPS    float64 `json:"fps"`
	Low1   float64 `json:"low1"`
	PerMin float64 `json:"perMin"` // tirones por minuto
	Secs   int     `json:"secs"`
}

// Order reparte los bloques: en cada pareja, al azar cuál va primero.
func Order(pairs int, r *rand.Rand) []int {
	out := make([]int, 0, 2*pairs)
	for i := 0; i < pairs; i++ {
		if r.Intn(2) == 0 {
			out = append(out, 0, 1)
		} else {
			out = append(out, 1, 0)
		}
	}
	return out
}

// Measure resume los fotogramas de un bloque.
func Measure(ms []float32, secs int, arm int) Block {
	b := Block{Arm: arm, Secs: secs}
	if len(ms) < 10 {
		return b
	}
	sum := 0.0
	s := make([]float64, len(ms))
	for i, v := range ms {
		sum += float64(v)
		s[i] = float64(v)
	}
	b.FPS = round(float64(len(ms)) * 1000 / sum)
	sort.Float64s(s)
	k := int(math.Ceil(float64(len(s)) * 0.01))
	w := 0.0
	for _, v := range s[len(s)-k:] {
		w += v
	}
	b.Low1 = round(1000 / (w / float64(k)))
	med := float32(s[len(s)/2])
	hits, in := 0, false
	for _, v := range ms {
		if stutter.IsHitch(v, med) {
			if !in {
				hits++
			}
			in = true
		} else {
			in = false
		}
	}
	b.PerMin = round(float64(hits) / (sum / 60000))
	return b
}

// Metric es lo que cambia una medida con el ajuste.
type Metric struct {
	A     float64 `json:"a"`     // media sin el ajuste
	B     float64 `json:"b"`     // media con el ajuste
	Delta float64 `json:"delta"` // % de cambio (en tirones: diferencia por minuto)
	Lo    float64 `json:"lo"`    // margen del 95 %
	Hi    float64 `json:"hi"`
	P     float64 `json:"p"` // probabilidad de ver esto por casualidad
}

// Result es el veredicto.
type Result struct {
	Pairs   int       `json:"pairs"`
	FPS     Metric    `json:"fps"`
	Low1    Metric    `json:"low1"`
	Stutter Metric    `json:"stutter"`
	Diffs   []float64 `json:"diffs"`   // cambio del 1 % bajo en cada pareja (para el gráfico)
	Verdict string    `json:"verdict"` // better, worse, mixed, same, unclear
}

func round(v float64) float64 { return math.Round(v*10) / 10 }

// Analyze compara los bloques por parejas.
func Analyze(blocks []Block) Result {
	var fps, low, st []float64
	var fa, fb, la, lb, sa, sb float64
	for i := 0; i+1 < len(blocks); i += 2 {
		x, y := blocks[i], blocks[i+1]
		if x.Arm == y.Arm || x.FPS == 0 || y.FPS == 0 {
			continue
		}
		a, b := x, y
		if a.Arm == 1 {
			a, b = y, x
		}
		fps = append(fps, (b.FPS/a.FPS-1)*100)
		if a.Low1 > 0 {
			low = append(low, (b.Low1/a.Low1-1)*100)
		}
		st = append(st, b.PerMin-a.PerMin)
		fa += a.FPS
		fb += b.FPS
		la += a.Low1
		lb += b.Low1
		sa += a.PerMin
		sb += b.PerMin
	}
	n := float64(len(fps))
	r := Result{Pairs: len(fps), Verdict: "unclear"}
	if n == 0 {
		return r
	}
	r.FPS = metric(fps, fa/n, fb/n)
	r.Low1 = metric(low, la/n, lb/n)
	r.Stutter = metric(st, sa/n, sb/n)
	for _, d := range low {
		r.Diffs = append(r.Diffs, round(d))
	}
	if r.Pairs < 4 {
		return r
	}
	// Los FPS mandan; el 1 % bajo y los tirones, que varían más, tienen que
	// ser más claros para contar (si no, mirando tres cosas a la vez se
	// colarían casualidades).
	better := (r.FPS.P < 0.05 && r.FPS.Delta >= 1.5) || (r.Low1.P < 0.02 && r.Low1.Delta >= 2) || (r.Stutter.P < 0.02 && r.Stutter.Delta <= -0.5)
	worse := (r.FPS.P < 0.05 && r.FPS.Delta <= -1.5) || (r.Low1.P < 0.02 && r.Low1.Delta <= -2) || (r.Stutter.P < 0.02 && r.Stutter.Delta >= 0.5)
	switch {
	case better && worse:
		r.Verdict = "mixed"
	case better:
		r.Verdict = "better"
	case worse:
		r.Verdict = "worse"
	case r.Pairs >= 6:
		r.Verdict = "same"
	}
	return r
}

// metric usa estadística robusta: si en alguna pareja la escena del juego
// cambió de golpe (de un pasillo a una ciudad), esa pareja no arrastra el
// resultado. El cambio es el estimador de Hodges-Lehmann (la mediana de las
// medias de cada dos parejas) y la prueba, la de rangos con signo.
func metric(d []float64, a, b float64) Metric {
	m := Metric{A: round(a), B: round(b)}
	if len(d) == 0 {
		m.P = 1
		return m
	}
	n := len(d)
	var walsh []float64
	for i := 0; i < n; i++ {
		for j := i; j < n; j++ {
			walsh = append(walsh, (d[i]+d[j])/2)
		}
	}
	sort.Float64s(walsh)
	M := len(walsh)
	if M%2 == 1 {
		m.Delta = round(walsh[M/2])
	} else {
		m.Delta = round((walsh[M/2-1] + walsh[M/2]) / 2)
	}
	k := int(math.Floor(float64(M)/2 - 1.96*math.Sqrt(float64(n*(n+1)*(2*n+1))/24)))
	if k < 0 {
		k = 0
	}
	m.Lo, m.Hi = round(walsh[k]), round(walsh[M-1-k])
	m.P = math.Round(SignFlip(signedRanks(d))*1000) / 1000
	return m
}

// signedRanks cambia cada diferencia por su puesto (de menor a mayor tamaño) con su signo.
func signedRanks(d []float64) []float64 {
	idx := make([]int, len(d))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return math.Abs(d[idx[a]]) < math.Abs(d[idx[b]]) })
	out := make([]float64, len(d))
	for i := 0; i < len(idx); {
		j := i
		for j+1 < len(idx) && math.Abs(d[idx[j+1]]) == math.Abs(d[idx[i]]) {
			j++
		}
		r := float64(i+j)/2 + 1 // empates: el puesto medio
		for k := i; k <= j; k++ {
			v := d[idx[k]]
			switch {
			case v > 0:
				out[idx[k]] = r
			case v < 0:
				out[idx[k]] = -r
			}
		}
		i = j + 1
	}
	return out
}

// SignFlip es la prueba de permutaciones de signos (dos colas): con qué
// probabilidad saldría una media tan lejos de cero si el ajuste no hiciera
// nada (cada diferencia sería igual de probable con un signo que con otro).
func SignFlip(d []float64) float64 {
	n := len(d)
	if n == 0 {
		return 1
	}
	obs := 0.0
	for _, v := range d {
		obs += v
	}
	obs = math.Abs(obs) - 1e-9
	count, total := 0, 0
	if n <= 16 {
		for mask := 0; mask < 1<<n; mask++ {
			s := 0.0
			for i, v := range d {
				if mask&(1<<i) != 0 {
					s -= v
				} else {
					s += v
				}
			}
			if math.Abs(s) >= obs {
				count++
			}
			total++
		}
	} else {
		r := rand.New(rand.NewSource(1))
		for k := 0; k < 20000; k++ {
			s := 0.0
			for _, v := range d {
				if r.Intn(2) == 0 {
					s -= v
				} else {
					s += v
				}
			}
			if math.Abs(s) >= obs {
				count++
			}
			total++
		}
	}
	return float64(count) / float64(total)
}
