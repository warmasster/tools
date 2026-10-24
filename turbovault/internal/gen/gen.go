// Package gen crea contraseñas aleatorias (con crypto/rand) y mide lo fuertes
// que son.
package gen

import (
	"crypto/rand"
	_ "embed"
	"errors"
	"math"
	"math/big"
	"strings"
	"unicode"
)

//go:embed palabras.txt
var palabrasTxt string

//go:embed comunes.txt
var comunesTxt string

// Words es la lista de palabras para las frases de contraseña (sin tildes ni
// eñes, para que se puedan escribir en cualquier teclado).
var Words = loadWords()

func loadWords() []string {
	seen := map[string]bool{}
	var out []string
	for _, w := range strings.Fields(palabrasTxt) {
		w = strings.ToLower(w)
		if len(w) < 4 || len(w) > 10 || seen[w] {
			continue
		}
		ok := true
		for _, r := range w {
			if r < 'a' || r > 'z' {
				ok = false
				break
			}
		}
		if ok {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}

const (
	lowerSet   = "abcdefghijklmnopqrstuvwxyz"
	upperSet   = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digitSet   = "0123456789"
	symbolSet  = "!#$%&*+-=?@^_~.,:;()[]{}/"
	ambiguous  = "Il1O0o|"
	maxLength  = 128
	minLength  = 6
	maxWords   = 12
	minWords   = 3
	defaultSep = "-"
)

// Options dice cómo generar.
type Options struct {
	Mode        string `json:"mode"` // "random", "words" o "pin"
	Length      int    `json:"length"`
	Lower       bool   `json:"lower"`
	Upper       bool   `json:"upper"`
	Digits      bool   `json:"digits"`
	Symbols     bool   `json:"symbols"`
	NoAmbiguous bool   `json:"noAmbiguous"`
	Words       int    `json:"words"`
	Sep         string `json:"sep"`
	Capitalize  bool   `json:"capitalize"`
	Number      bool   `json:"number"`
}

// Default son las opciones recomendadas: 20 caracteres de todo tipo.
func Default() Options {
	return Options{Mode: "random", Length: 20, Lower: true, Upper: true, Digits: true, Symbols: true,
		Words: 5, Sep: defaultSep, Capitalize: true, Number: true}
}

func randInt(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic(err)
	}
	return int(v.Int64())
}

func filter(set string, noAmb bool) string {
	if !noAmb {
		return set
	}
	var b strings.Builder
	for _, r := range set {
		if !strings.ContainsRune(ambiguous, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Result es una contraseña generada y su fuerza teórica.
type Result struct {
	Password string  `json:"password"`
	Bits     float64 `json:"bits"`
}

// Generate crea una contraseña según las opciones.
func Generate(o Options) (Result, error) {
	switch o.Mode {
	case "words":
		return passphrase(o)
	case "pin":
		n := clamp(o.Length, 4, 12)
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteByte(digitSet[randInt(10)])
		}
		return Result{Password: b.String(), Bits: float64(n) * math.Log2(10)}, nil
	default:
		return random(o)
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func random(o Options) (Result, error) {
	var sets []string
	if o.Lower {
		sets = append(sets, filter(lowerSet, o.NoAmbiguous))
	}
	if o.Upper {
		sets = append(sets, filter(upperSet, o.NoAmbiguous))
	}
	if o.Digits {
		sets = append(sets, filter(digitSet, o.NoAmbiguous))
	}
	if o.Symbols {
		sets = append(sets, filter(symbolSet, o.NoAmbiguous))
	}
	if len(sets) == 0 {
		return Result{}, errors.New("elige al menos un tipo de carácter")
	}
	n := clamp(o.Length, minLength, maxLength)
	if n < len(sets) {
		n = len(sets)
	}
	all := strings.Join(sets, "")
	// Se repite hasta que salga al menos uno de cada tipo: así todas las
	// contraseñas válidas tienen la misma probabilidad.
	for {
		out := make([]byte, n)
		for i := range out {
			out[i] = all[randInt(len(all))]
		}
		ok := true
		for _, s := range sets {
			if !strings.ContainsAny(string(out), s) {
				ok = false
				break
			}
		}
		if ok {
			return Result{Password: string(out), Bits: float64(n) * math.Log2(float64(len(all)))}, nil
		}
	}
}

func passphrase(o Options) (Result, error) {
	n := clamp(o.Words, minWords, maxWords)
	sep := o.Sep
	if len([]rune(sep)) > 3 {
		sep = defaultSep
	}
	words := make([]string, n)
	for i := range words {
		w := Words[randInt(len(Words))]
		if o.Capitalize {
			r := []rune(w)
			r[0] = unicode.ToUpper(r[0])
			w = string(r)
		}
		words[i] = w
	}
	bits := float64(n) * math.Log2(float64(len(Words)))
	if o.Number {
		i := randInt(n)
		words[i] += string(digitSet[randInt(10)])
		bits += math.Log2(10) + math.Log2(float64(n))
	}
	return Result{Password: strings.Join(words, sep), Bits: bits}, nil
}
