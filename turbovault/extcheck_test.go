package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtensionIntegrity(t *testing.T) {
	if bad := compareExtension("extension"); bad != "" {
		t.Fatalf("la carpeta del repositorio debería coincidir con la incluida: %s", bad)
	}
	t.Setenv("TURBOVAULT_DIR", t.TempDir())
	dir, err := installExtension()
	if err != nil {
		t.Fatal(err)
	}
	if bad := compareExtension(dir); bad != "" {
		t.Fatalf("recién instalada: %s", bad)
	}
	os.WriteFile(filepath.Join(dir, "background.js"), []byte("robar()"), 0o644)
	if compareExtension(dir) != "background.js" {
		t.Fatal("no detecta un archivo cambiado")
	}
	installExtension()
	os.WriteFile(filepath.Join(dir, "extra.js"), []byte("x"), 0o644)
	if compareExtension(dir) == "" {
		t.Fatal("no detecta un archivo de más")
	}
}
