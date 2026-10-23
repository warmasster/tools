package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"turbogames/internal/steam"
	"turbogames/internal/vault"
)

func TestCleanTags(t *testing.T) {
	got := cleanTags([]string{" #Smurf ", "smurf", "", "cs2,", "una-etiqueta-demasiado-larga-para-caber"})
	if len(got) != 2 || got[0] != "Smurf" || got[1] != "cs2" {
		t.Fatalf("%q", got)
	}
}

func TestAccountName(t *testing.T) {
	for _, ok := range []string{"usuario_1", "ABC", "x_9_y"} {
		if msg := accountNameError(ok); msg != "" {
			t.Errorf("%q debería valer: %s", ok, msg)
		}
	}
	bad := map[string]string{
		"":            "Escribe",
		"ab":          "al menos 3",
		"yo@mail.com": "no el correo",
		"con espacio": "espacios",
		"cañón":       "sin acentos",
		"x/y":         "sin acentos",
	}
	for n, want := range bad {
		if msg := accountNameError(n); !strings.Contains(msg, want) {
			t.Errorf("%q: %q (esperaba que dijera %q)", n, msg, want)
		}
	}
}

func TestEntryError(t *testing.T) {
	ok := []vault.Entry{{}, {Password: "x"}, {Email: "a@b.es", EmailPassword: "y", Recovery: "R12345"}}
	for _, e := range ok {
		if msg := entryError(e); msg != "" {
			t.Errorf("%+v debería valer: %s", e, msg)
		}
	}
	bad := []vault.Entry{{Email: "sin-arroba"}, {Email: "a@b"}, {Email: "a b@c.es"}, {Recovery: "12345"}, {Recovery: "R1234"}, {EmailPassword: "solo"}}
	for _, e := range bad {
		if entryError(e) == "" {
			t.Errorf("%+v no debería valer", e)
		}
	}
}

func TestInstallState(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		flags int
		dir   string
		want  string
	}{
		{4, dir, "ok"},
		{0, dir, "ok"},
		{6, dir, "update"},
		{1026, dir, "updating"},
		{4, filepath.Join(dir, "no-existe"), "missing"},
		{2, dir, "incomplete"},
		{36, dir, "incomplete"},
	}
	for _, c := range cases {
		if got := installState(steam.InstalledApp{StateFlags: c.flags, InstallDir: c.dir}); got != c.want {
			t.Errorf("flags %d: %s, esperaba %s", c.flags, got, c.want)
		}
	}
}

func TestMetaStore(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cuentas.json")
	s := loadMetaStore(p)
	if err := s.update("MiCuenta", func(m *Meta) { m.Alias = "Principal"; m.Tags = []string{"a"} }); err != nil {
		t.Fatal(err)
	}
	s2 := loadMetaStore(p)
	m, ok := s2.get("micuenta")
	if !ok || m.Name != "MiCuenta" || m.Alias != "Principal" || m.Added == 0 {
		t.Fatalf("no se guardó bien: %+v", m)
	}
	// get devuelve una copia: cambiarla no toca lo guardado
	m.Tags[0] = "cambiada"
	if m2, _ := s2.get("MICUENTA"); m2.Tags[0] != "a" {
		t.Fatal("get no devuelve una copia")
	}
	if err := s2.remove("micuenta"); err != nil {
		t.Fatal(err)
	}
	if _, ok := loadMetaStore(p).get("micuenta"); ok {
		t.Fatal("no se borró")
	}
}

func TestImageRoutes(t *testing.T) {
	a := &App{}
	s := newImageServer(a)
	s.dir = t.TempDir()
	for _, path := range []string{"/img/app/abc/cover", "/img/app/10/otra", "/img/avatar/123", "/otra/cosa", "/img/app/0/cover"} {
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
}
