package gen

// Medidor de fuerza al estilo de zxcvbn, en pequeño: busca trozos fáciles de
// adivinar (contraseñas famosas, palabras, secuencias, recorridos de teclado,
// repeticiones, años y fechas) y calcula el camino más barato para un atacante
// que conoce esos trucos. Así "Contraseña2024!" no sale fuerte solo por llevar
// mayúscula, número y símbolo.

import (
	"fmt"
	"math"
	"strings"
	"unicode"
)

// Strength es el resultado de medir una contraseña.
type Strength struct {
	Score   int      `json:"score"` // 0 muy débil … 4 muy fuerte
	Label   string   `json:"label"`
	Bits    float64  `json:"bits"`
	Crack   string   `json:"crack"` // cuánto tardaría un atacante
	Warning string   `json:"warning,omitempty"`
	Tips    []string `json:"tips,omitempty"`
}

var labels = [5]string{"Muy débil", "Débil", "Aceptable", "Fuerte", "Muy fuerte"}

// Intentos por segundo que se suponen para el atacante:
//   - contraseñas de webs: robo de la base de datos con un hash rápido;
//   - contraseña maestra: Argon2id con 256 MB por intento, en muchas tarjetas.
const (
	rateWeb    = 1e10
	rateMaster = 1e4
)

var dict = loadDict()

func loadDict() map[string]int {
	m := map[string]int{}
	rank := 1
	add := func(w string) {
		w = strings.ToLower(strings.TrimSpace(w))
		if len([]rune(w)) < 3 {
			return
		}
		if _, ok := m[w]; !ok {
			m[w] = rank
			rank++
		}
	}
	for _, w := range strings.Fields(comunesTxt) {
		add(w)
	}
	for _, w := range strings.Fields(palabrasTxt) {
		add(w)
	}
	// Palabras inglesas muy usadas en contraseñas.
	for _, w := range strings.Fields("love baby angel lover girl boy cool king queen star blue black red green pink happy sweet magic power money hot sexy super fire dark light life time home world game player gamer pro best test word pass") {
		add(w)
	}
	return m
}

func isCommon(pw string) (int, bool) {
	r, ok := dict[strings.ToLower(pw)]
	return r, ok && r <= 400
}

var leet = map[rune]rune{'4': 'a', '@': 'a', '3': 'e', '1': 'i', '!': 'i', '0': 'o', '5': 's', '$': 's', '7': 't', '+': 't', '8': 'b', '9': 'g'}

var keyRows = []string{"qwertyuiop", "asdfghjklñ", "zxcvbnm", "1234567890", "qazwsxedcrfvtgbyhnujmikolp"}

type match struct {
	i, j int // [i, j)
	bits float64
	kind string
}

func charsetSize(rs []rune) float64 {
	var lo, up, di, sy, ot bool
	for _, r := range rs {
		switch {
		case r >= 'a' && r <= 'z':
			lo = true
		case r >= 'A' && r <= 'Z':
			up = true
		case r >= '0' && r <= '9':
			di = true
		case r < 128:
			sy = true
		default:
			ot = true
		}
	}
	n := 0.0
	if lo {
		n += 26
	}
	if up {
		n += 26
	}
	if di {
		n += 10
	}
	if sy {
		n += 33
	}
	if ot {
		n += 100
	}
	if n == 0 {
		n = 10
	}
	return n
}

func capsBits(seg []rune) float64 {
	up, lo := 0, 0
	for _, r := range seg {
		if unicode.IsUpper(r) {
			up++
		} else if unicode.IsLower(r) {
			lo++
		}
	}
	switch {
	case up == 0:
		return 0
	case up == 1 && unicode.IsUpper(seg[0]), lo == 0:
		return 1 // Primera en mayúscula, o todo en mayúsculas
	default:
		return math.Log2(float64(binom(len(seg), up)) + 1)
	}
}

func binom(n, k int) float64 {
	if k > n {
		return 0
	}
	r := 1.0
	for i := 1; i <= k; i++ {
		r = r * float64(n-k+i) / float64(i)
	}
	return r
}

func findMatches(rs []rune, inputs map[string]bool) []match {
	n := len(rs)
	low := make([]rune, n)
	unl := make([]rune, n)
	for i, r := range rs {
		low[i] = unicode.ToLower(r)
		if m, ok := leet[low[i]]; ok {
			unl[i] = m
		} else {
			unl[i] = low[i]
		}
	}
	var ms []match
	// palabras (también al revés y con sustituciones tipo l33t)
	for i := 0; i < n; i++ {
		for j := i + 3; j <= n && j-i <= 24; j++ {
			w := string(low[i:j])
			seg := rs[i:j]
			if inputs[w] {
				ms = append(ms, match{i, j, 1 + capsBits(seg), "input"})
				continue
			}
			if r, ok := dict[w]; ok {
				ms = append(ms, match{i, j, math.Log2(float64(r)+1) + capsBits(seg), "dict"})
			}
			if u := string(unl[i:j]); u != w {
				if r, ok := dict[u]; ok {
					ms = append(ms, match{i, j, math.Log2(float64(r)+1) + capsBits(seg) + 1, "dict"})
				} else if inputs[u] {
					ms = append(ms, match{i, j, 2 + capsBits(seg), "input"})
				}
			}
			if rev := reverse(w); rev != w {
				if r, ok := dict[rev]; ok {
					ms = append(ms, match{i, j, math.Log2(float64(r)+1) + capsBits(seg) + 1, "dict"})
				}
			}
		}
	}
	// secuencias: abc, 123, 9876, xyz
	for i := 0; i+2 < n; i++ {
		d := low[i+1] - low[i]
		if d != 1 && d != -1 {
			continue
		}
		j := i + 1
		for j+1 < n && low[j+1]-low[j] == d {
			j++
		}
		for e := i + 3; e <= j+1; e++ {
			b := math.Log2(26) + math.Log2(float64(e-i))
			if unicode.IsDigit(low[i]) {
				b = math.Log2(10) + math.Log2(float64(e-i))
			}
			if d < 0 {
				b++
			}
			ms = append(ms, match{i, e, b, "seq"})
		}
	}
	// recorridos de teclado
	for i := 0; i < n; i++ {
		for j := i + 4; j <= n && j-i <= 16; j++ {
			w := string(low[i:j])
			for _, row := range keyRows {
				if strings.Contains(row, w) || strings.Contains(row, reverse(w)) {
					ms = append(ms, match{i, j, math.Log2(float64(len(keyRows)*10)) + math.Log2(float64(j-i)), "keyboard"})
					break
				}
			}
		}
	}
	// repeticiones: aaa, 1111, y bloques repetidos: abcabc
	for i := 0; i < n; i++ {
		j := i + 1
		for j < n && low[j] == low[i] {
			j++
		}
		for e := i + 3; e <= j; e++ {
			ms = append(ms, match{i, e, math.Log2(charsetSize(rs[i:i+1])) + math.Log2(float64(e-i)), "repeat"})
		}
		for k := 2; i+2*k <= n && k <= 12; k++ {
			if string(low[i:i+k]) == string(low[i+k:i+2*k]) {
				times := 2
				for i+(times+1)*k <= n && string(low[i:i+k]) == string(low[i+times*k:i+(times+1)*k]) {
					times++
				}
				base := float64(k) * math.Log2(charsetSize(rs[i:i+k]))
				ms = append(ms, match{i, i + times*k, base + math.Log2(float64(times)), "repeat"})
			}
		}
	}
	// años y fechas
	for i := 0; i+4 <= n; i++ {
		if allDigits(rs[i : i+4]) {
			y := atoi(rs[i : i+4])
			if y >= 1900 && y <= 2049 {
				ms = append(ms, match{i, i + 4, math.Log2(150), "year"})
			}
		}
		for _, l := range []int{6, 8} {
			if i+l <= n && allDigits(rs[i:i+l]) && looksLikeDate(rs[i:i+l]) {
				ms = append(ms, match{i, i + l, math.Log2(366 * 120), "date"})
			}
		}
	}
	return ms
}

func reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

func allDigits(rs []rune) bool {
	for _, r := range rs {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func atoi(rs []rune) int {
	n := 0
	for _, r := range rs {
		n = n*10 + int(r-'0')
	}
	return n
}

func looksLikeDate(rs []rune) bool {
	d, m := atoi(rs[0:2]), atoi(rs[2:4])
	if d >= 1 && d <= 31 && m >= 1 && m <= 12 {
		return true
	}
	if len(rs) == 8 { // aaaammdd
		y, m2, d2 := atoi(rs[0:4]), atoi(rs[4:6]), atoi(rs[6:8])
		return y >= 1900 && y <= 2049 && m2 >= 1 && m2 <= 12 && d2 >= 1 && d2 <= 31
	}
	return false
}

// Check mide una contraseña. master=true usa el ritmo de ataque de la
// contraseña maestra (Argon2id); inputs son cosas que el atacante conoce
// (usuario, nombre de la web).
func Check(pw string, master bool, inputs ...string) Strength {
	rs := []rune(pw)
	n := len(rs)
	if n == 0 {
		return Strength{Score: 0, Label: labels[0], Crack: "al instante"}
	}
	in := map[string]bool{}
	for _, s := range inputs {
		for _, part := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
			if len([]rune(part)) >= 3 {
				in[part] = true
			}
		}
	}
	brute := math.Log2(charsetSize(rs))
	ms := findMatches(rs, in)
	byEnd := make([][]match, n+1)
	for _, m := range ms {
		byEnd[m.j] = append(byEnd[m.j], m)
	}
	best := make([]float64, n+1)
	from := make([]match, n+1)
	for j := 1; j <= n; j++ {
		best[j] = best[j-1] + brute
		from[j] = match{j - 1, j, brute, "brute"}
		for _, m := range byEnd[j] {
			// Cada trozo "con truco" cuenta un poco más: hay que acertar el orden.
			c := best[m.i] + m.bits + 1
			if c < best[j] {
				best[j] = c
				from[j] = m
			}
		}
	}
	bits := best[n]
	kinds := map[string]bool{}
	for j := n; j > 0; j = from[j].i {
		kinds[from[j].kind] = true
	}
	if r, ok := isCommon(pw); ok {
		bits = math.Min(bits, math.Log2(float64(r)+1))
		kinds["common"] = true
	}
	rate := rateWeb
	if master {
		rate = rateMaster
	}
	secs := math.Pow(2, bits-1) / rate
	s := Strength{Bits: math.Round(bits*10) / 10, Crack: humanTime(secs)}
	switch {
	case secs < 3600:
		s.Score = 0
	case secs < 30*86400:
		s.Score = 1
	case secs < 100*365*86400:
		s.Score = 2
	case secs < 1e6*365*86400:
		s.Score = 3
	default:
		s.Score = 4
	}
	if n < 8 && s.Score > 1 {
		s.Score = 1
	}
	s.Label = labels[s.Score]
	switch {
	case kinds["common"]:
		s.Warning = "Es una de las contraseñas más usadas: es lo primero que se prueba."
	case kinds["input"]:
		s.Warning = "Lleva tu usuario o el nombre de la web."
	case kinds["keyboard"]:
		s.Warning = "Los recorridos del teclado como «qwerty» se adivinan enseguida."
	case kinds["seq"]:
		s.Warning = "Las secuencias como «abc» o «123» se adivinan enseguida."
	case kinds["repeat"]:
		s.Warning = "Las repeticiones como «aaa» o «abcabc» se adivinan enseguida."
	case kinds["year"] || kinds["date"]:
		s.Warning = "Los años y las fechas se adivinan enseguida."
	case kinds["dict"] && s.Score < 3:
		s.Warning = "Las palabras y nombres sueltos se adivinan enseguida."
	case n < 10 && s.Score < 3:
		s.Warning = "Es demasiado corta."
	}
	if s.Score < 3 {
		if master {
			s.Tips = append(s.Tips, "Usa cinco o más palabras al azar: es fácil de recordar y muy difícil de adivinar.")
		} else {
			s.Tips = append(s.Tips, "Usa el generador: una contraseña larga y aleatoria no hace falta recordarla.")
		}
		if n < 14 {
			s.Tips = append(s.Tips, "Hazla más larga: cada carácter cuenta más que cualquier símbolo.")
		}
	}
	return s
}

func humanTime(secs float64) string {
	plural := func(v float64, uno, varios string) string {
		n := int64(math.Round(v))
		if n <= 1 {
			return "1 " + uno
		}
		return fmt.Sprintf("%d %s", n, varios)
	}
	const (
		min  = 60.0
		hora = 3600.0
		dia  = 86400.0
		mes  = 30 * dia
		ano  = 365 * dia
	)
	switch {
	case secs < 1:
		return "al instante"
	case secs < min:
		return plural(secs, "segundo", "segundos")
	case secs < hora:
		return plural(secs/min, "minuto", "minutos")
	case secs < dia:
		return plural(secs/hora, "hora", "horas")
	case secs < mes:
		return plural(secs/dia, "día", "días")
	case secs < ano:
		return plural(secs/mes, "mes", "meses")
	case secs < 100*ano:
		return plural(secs/ano, "año", "años")
	case secs < 1e6*ano:
		return "siglos"
	default:
		return "millones de años"
	}
}
