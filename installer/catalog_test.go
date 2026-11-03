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

func TestReleaseDownload(t *testing.T) {
	t.Setenv("TURBOSETUP_BASE", "")
	m := &Manifest{
		Setup: Asset{Asset: "TurboSETUP.exe", Download: "TurboSETUP-1.0.8.exe", Release: "turbosetup-v1.0.9"},
		Tools: []Tool{
			{ID: "turbocam", Name: "TurboCAM", Asset: Asset{Asset: "TurboCAM.exe", Release: "turbocam-v1.0.6"}},
			{ID: "turbocut", Name: "TurboCUT", Asset: Asset{Asset: "TurboCUT.exe", Release: "../apps"}},
		},
	}
	m.clean()
	if got := m.Setup.remoteURL(); got != "https://github.com/warmasster/tools/releases/download/turbosetup-v1.0.9/TurboSETUP.exe" {
		t.Errorf("TurboSETUP se pide a %q", got)
	}
	if got := m.find("turbocam").remoteURL(); got != "https://github.com/warmasster/tools/releases/download/turbocam-v1.0.6/TurboCAM.exe" {
		t.Errorf("TurboCAM se pide a %q", got)
	}
	// Una etiqueta rara se ignora: el .exe de siempre de "apps".
	if got := m.find("turbocut").remoteURL(); got != "https://github.com/warmasster/tools/releases/download/apps/TurboCUT.exe" {
		t.Errorf("TurboCUT se pide a %q", got)
	}
	t.Setenv("TURBOSETUP_BASE", "http://localhost:8799/")
	if got := m.find("turbocam").remoteURL(); got != "http://localhost:8799/turbocam-v1.0.6/TurboCAM.exe" {
		t.Errorf("con servidor de prueba, TurboCAM se pide a %q", got)
	}
}
