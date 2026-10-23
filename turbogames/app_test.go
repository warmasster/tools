package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestCleanTags(t *testing.T) {
	got := cleanTags([]string{" #Smurf ", "smurf", "", "cs2,", "una-etiqueta-demasiado-larga-para-caber"})
	if len(got) != 2 || got[0] != "Smurf" || got[1] != "cs2" {
		t.Fatalf("%q", got)
	}
}

func TestValidAccountName(t *testing.T) {
	for _, ok := range []string{"usuario_1", "Cuenta.2", "ab"} {
		if !validAccountName(ok) {
			t.Errorf("%q debería valer", ok)
		}
	}
	for _, bad := range []string{"", "a", "con espacio", "raro\"", "x/y"} {
		if validAccountName(bad) {
			t.Errorf("%q no debería valer", bad)
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
