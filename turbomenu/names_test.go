package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanFileNameReserved(t *testing.T) {
	cases := map[string]string{
		"CON.txt":           "_CON.txt",
		"aux.min.js":        "_aux.min.js",
		"com3":              "_com3",
		"console.log":       "console.log",
		"foto\u202Egpj.exe": "fotogpj.exe",
		`..\..\x.txt`:       "x.txt",
	}
	for in, want := range cases {
		if got := cleanFileName(in); got != want {
			t.Errorf("cleanFileName(%q) = %q, se esperaba %q", in, got, want)
		}
	}
}

func TestCreateExclusive(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("viejo"), 0o644)
	p, f, err := createExclusive(dir, "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if filepath.Base(p) != "a (2).txt" {
		t.Errorf("nombre = %s", filepath.Base(p))
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(b) != "viejo" {
		t.Error("ha pisado el archivo existente")
	}
}

func TestSelectionListOnlyTemp(t *testing.T) {
	if selectionList(filepath.Join(t.TempDir(), "importante.docx")) {
		t.Error("un archivo cualquiera no es una lista de selección")
	}
	if !selectionList(filepath.Join(os.TempDir(), "TurboMENU-sel-123.txt")) {
		t.Error("las listas del menú nuevo sí")
	}
}
