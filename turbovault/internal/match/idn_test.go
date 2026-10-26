package match

import "testing"

func TestWildcardPublicSuffixAndIDN(t *testing.T) {
	cases := []struct {
		entry, page string
		want        bool
	}{
		{"*.github.io", "https://evil.github.io/", false},          // sufijo público: de cualquiera
		{"*.co.uk", "https://banco.co.uk/", false},                 // sufijo público
		{"*.google.com", "https://mail.google.com/", true},         // dominio normal: vale
		{"*.web.co.uk", "https://login.web.co.uk/", true},          // dominio bajo un sufijo de dos partes
		{"https://münchen.de", "https://xn--mnchen-3ya.de/", true}, // lo guardado con acentos casa con lo que manda Chrome
		{"https://xn--mnchen-3ya.de", "https://münchen.de/", true},
		{"https://muenchen.de", "https://xn--mnchen-3ya.de/", false},
	}
	for _, c := range cases {
		if got := Matches(c.entry, c.page); got != c.want {
			t.Errorf("Matches(%q, %q) = %v, se esperaba %v", c.entry, c.page, got, c.want)
		}
	}
}
