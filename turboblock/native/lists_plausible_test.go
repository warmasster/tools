package main

import (
	"strings"
	"testing"
)

func TestPlausibleList(t *testing.T) {
	good := "[Adblock Plus 2.0]\n! Title: Prueba\n" + strings.Repeat("||anuncio.example^\n", 500)
	if err := plausibleList(good, nil, false); err != nil {
		t.Errorf("una lista normal debería valer: %v", err)
	}
	if plausibleList("", []byte(good), true) == nil {
		t.Error("una lista vacía no debería sustituir a la buena")
	}
	if plausibleList("error code: 1015", []byte(good), true) == nil {
		t.Error("un error de CDN con 200 no debería sustituir a 500 filtros")
	}
	if plausibleList("! Title: x\n! solo comentarios\n", nil, false) == nil {
		t.Error("una lista sin filtros no vale")
	}
	if err := plausibleList("! Title: x\n!#include parte.txt\n", nil, false); err != nil {
		t.Errorf("una lista hecha de !#include sí vale: %v", err)
	}
	shrunk := strings.Repeat("||a.example^\n", 450)
	if err := plausibleList(shrunk, []byte(good), true); err != nil {
		t.Errorf("perder un 10 %% es normal: %v", err)
	}
}
