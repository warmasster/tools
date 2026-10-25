package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestClean(t *testing.T) {
	m := &Manifest{
		Setup: Asset{Asset: `..\TurboSETUP.exe`},
		Tools: []Tool{
			{ID: "turbocut", Name: "TurboCUT", Asset: Asset{Asset: "TurboCUT.exe"}},
			{ID: "malo", Name: `..\..\Windows`, Asset: Asset{Asset: "x.exe"}},
			{ID: "otro", Name: "Otro", Asset: Asset{Asset: `C:\x.exe`}},
			{ID: "a b", Name: "Ab", Asset: Asset{Asset: "ab.exe"}},
		},
	}
	m.clean()
	if m.Setup.Asset != "" {
		t.Error("el asset de TurboSETUP con ruta debería descartarse")
	}
	if len(m.Tools) != 1 || m.Tools[0].ID != "turbocut" {
		t.Errorf("solo debería quedar TurboCUT: %+v", m.Tools)
	}
}

func TestInsideRoot(t *testing.T) {
	root := rootDir()
	cases := map[string]bool{
		filepath.Join(root, "TurboCUT"):       true,
		filepath.Join(root, "TurboCUT") + `\`: true,
		root:                                  false,
		root + "X":                            false,
		filepath.Join(root, "..", "TurboX"):   false,
		"":                                    false,
	}
	for dir, want := range cases {
		if got := insideRoot(dir); got != want {
			t.Errorf("insideRoot(%q) = %v, se esperaba %v", dir, got, want)
		}
	}
}

func TestReplaceFileOldInUse(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "app.exe")
	os.WriteFile(dst, []byte("v1"), 0o644)
	// Un .old de una actualización anterior que sigue abierto (no se puede borrar).
	os.WriteFile(dst+".old", []byte("v0"), 0o644)
	f, err := os.Open(dst + ".old")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	src := filepath.Join(dir, "app.exe.part")
	os.WriteFile(src, []byte("v2"), 0o644)
	if _, err := replaceFile(src, dst); err != nil {
		t.Fatalf("replaceFile con el .old abierto: %v", err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "v2" {
		t.Errorf("dst = %q, se esperaba v2", b)
	}
}

func TestDownloadNeedsHash(t *testing.T) {
	err := download(context.Background(), Asset{Asset: "TurboCUT.exe"}, filepath.Join(t.TempDir(), "x"), func(int64, int64) {})
	if err == nil || !strings.Contains(err.Error(), "huella") {
		t.Errorf("sin SHA-256 no debería descargar: %v", err)
	}
}
