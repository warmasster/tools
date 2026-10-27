package main

import "testing"

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.0.1", "1.0.0", true},
		{"1.10.0", "1.9.9", true},
		{"2.0", "1.99.99", true},
		{"1.0.0", "1.0.0", false},
		{"1.0", "1.0.0", false},
		{"1.0.0", "1.0.1", false},
		{"v1.2.0", "1.1.0", true},
	}
	for _, c := range cases {
		if got := newer(c.a, c.b); got != c.want {
			t.Errorf("newer(%q, %q) = %v, quería %v", c.a, c.b, got, c.want)
		}
	}
}

func TestVersionedDownload(t *testing.T) {
	m := &Manifest{
		Setup: Asset{Asset: "TurboSETUP.exe", Download: "TurboSETUP-1.0.7.exe"},
		Tools: []Tool{
			{ID: "turbocut", Name: "TurboCUT", Asset: Asset{Asset: "TurboCUT.exe", Download: "../TurboCUT-1.1.2.exe"}},
			{ID: "turbosend", Name: "TurboSEND", Asset: Asset{Asset: "TurboSEND.exe"}},
		},
	}
	m.clean()
	if got := m.Setup.remoteName(); got != "TurboSETUP-1.0.7.exe" {
		t.Errorf("TurboSETUP se pide como %q", got)
	}
	// Un nombre versionado raro se ignora, pero la herramienta sigue en el catálogo.
	if got := m.find("turbocut").remoteName(); got != "TurboCUT.exe" {
		t.Errorf("TurboCUT se pide como %q", got)
	}
	// Catálogo de antes de los nombres con versión: el de siempre.
	if got := m.find("turbosend").remoteName(); got != "TurboSEND.exe" {
		t.Errorf("TurboSEND se pide como %q", got)
	}
}
