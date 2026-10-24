package gen

import (
	"strings"
	"testing"
)

func TestWordList(t *testing.T) {
	if len(Words) < 1024 {
		t.Fatalf("hacen falta al menos 1024 palabras únicas, hay %d", len(Words))
	}
	t.Logf("%d palabras", len(Words))
}

func TestRandom(t *testing.T) {
	o := Default()
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		r, err := Generate(o)
		if err != nil {
			t.Fatal(err)
		}
		p := r.Password
		if len(p) != 20 || seen[p] {
			t.Fatalf("contraseña mala o repetida: %q", p)
		}
		seen[p] = true
		if !strings.ContainsAny(p, lowerSet) || !strings.ContainsAny(p, upperSet) || !strings.ContainsAny(p, digitSet) || !strings.ContainsAny(p, symbolSet) {
			t.Fatalf("falta algún tipo: %q", p)
		}
	}
	o = Options{Mode: "random", Length: 30, Lower: true, Digits: true, NoAmbiguous: true}
	r, _ := Generate(o)
	if strings.ContainsAny(r.Password, ambiguous+upperSet+symbolSet) {
		t.Fatalf("lleva caracteres que no tocan: %q", r.Password)
	}
	if _, err := Generate(Options{Mode: "random", Length: 10}); err == nil {
		t.Fatal("sin tipos debe fallar")
	}
}

func TestPassphraseAndPin(t *testing.T) {
	r, _ := Generate(Options{Mode: "words", Words: 5, Sep: "-", Capitalize: true, Number: true})
	parts := strings.Split(r.Password, "-")
	if len(parts) != 5 {
		t.Fatalf("frase: %q", r.Password)
	}
	if r.Bits < 50 {
		t.Fatalf("pocos bits: %.1f", r.Bits)
	}
	p, _ := Generate(Options{Mode: "pin", Length: 6})
	if len(p.Password) != 6 || strings.Trim(p.Password, digitSet) != "" {
		t.Fatalf("pin: %q", p.Password)
	}
}

func TestStrength(t *testing.T) {
	cases := []struct {
		pw     string
		master bool
		max    int // puntuación máxima aceptable
		min    int
	}{
		{"123456", false, 0, 0},
		{"password", false, 0, 0},
		{"contraseña", false, 0, 0},
		{"Contraseña2024!", false, 1, 0},
		{"qwertyuiop", false, 0, 0},
		{"abcdefgh", false, 0, 0},
		{"aaaaaaaaaaaa", false, 0, 0},
		{"P@ssw0rd", false, 0, 0},
		{"Fernando1999!", true, 1, 0},
		{"teamo123", false, 0, 0},
		{"kX9#mQ2$vL7&pR4!", false, 4, 3},
		{"tH7$-qPz!8vK-2mW#9xR", false, 4, 4},
		{"caracol-Bisonte-faro7-ciruela-tambor", true, 4, 3},
	}
	for _, c := range cases {
		s := Check(c.pw, c.master)
		if s.Score > c.max || s.Score < c.min {
			t.Errorf("%q: puntuación %d (%s, %.1f bits, %s), esperaba %d-%d", c.pw, s.Score, s.Label, s.Bits, s.Warning, c.min, c.max)
		}
	}
	if s := Check("fernandoperez", false, "fernando@correo.com"); s.Warning == "" {
		t.Error("debería avisar de que lleva el usuario")
	}
	g, _ := Generate(Default())
	if s := Check(g.Password, false); s.Score < 4 {
		t.Errorf("la generada %q sale %d", g.Password, s.Score)
	}
}

func BenchmarkCheck(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Check("Esto-Es-Una-Frase-Bastante-Larga-2024!", true)
	}
}
