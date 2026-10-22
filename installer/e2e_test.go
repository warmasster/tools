package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// Prueba completa contra un servidor con manifest.json y los .exe (lo que deja
// release.ps1 -OutDir dist). Instala de verdad para el usuario actual (registro
// y menú Inicio) en una carpeta temporal, y luego desinstala.
//
//	python -m http.server 8799 --directory dist
//	$env:TURBOSETUP_E2E="http://localhost:8799"; go test -run E2E -v .
func TestE2EInstallUninstall(t *testing.T) {
	base := os.Getenv("TURBOSETUP_E2E")
	if base == "" {
		t.Skip("define TURBOSETUP_E2E con la URL del servidor de prueba")
	}
	t.Setenv("TURBOSETUP_BASE", base)
	t.Setenv("LOCALAPPDATA", t.TempDir())

	m, err := fetchManifest()
	if err != nil {
		t.Fatal(err)
	}
	tool := m.find("turbocut")
	if tool == nil {
		t.Fatal("turbocut no está en el manifest")
	}
	if _, ok := listInstalled()["turbocut"]; ok {
		t.Skip("TurboCUT ya está instalado de verdad; no lo toco")
	}

	var last int64
	res, err := installTool(context.Background(), tool, false, func(done, total int64) { last = done })
	if err != nil {
		t.Fatal(err)
	}
	t.Log(res.Message)
	exe := filepath.Join(rootDir(), "TurboCUT", "TurboCUT.exe")
	if st, err := os.Stat(exe); err != nil || st.Size() != tool.Size || last != tool.Size {
		t.Fatalf("exe mal instalado: %v (progreso %d de %d)", err, last, tool.Size)
	}
	if _, err := os.Stat(startMenuLink("TurboCUT")); err != nil {
		t.Errorf("falta el acceso directo: %v", err)
	}
	if _, err := os.Stat(setupPath()); err != nil {
		t.Errorf("falta la copia de TurboSETUP: %v", err)
	}
	in, ok := listInstalled()["turbocut"]
	if !ok || in.Version != tool.Version {
		t.Fatalf("no aparece en el registro: %+v", in)
	}
	v := buildView(m, nil)
	for _, tv := range v.Tools {
		if tv.ID == "turbocut" && (!tv.Installed || tv.Update) {
			t.Errorf("vista incorrecta: %+v", tv)
		}
	}

	// Reinstalar encima (actualizar) también tiene que funcionar.
	if _, err := installTool(context.Background(), tool, false, func(int64, int64) {}); err != nil {
		t.Fatalf("reinstalar: %v", err)
	}

	msg, err := uninstallTool("turbocut")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(msg)
	if _, err := os.Stat(exe); !os.IsNotExist(err) {
		t.Errorf("el exe sigue ahí")
	}
	if _, err := os.Stat(startMenuLink("TurboCUT")); !os.IsNotExist(err) {
		t.Errorf("el acceso directo sigue ahí")
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, regKeyName("turbocut"), registry.QUERY_VALUE); err == nil {
		k.Close()
		t.Errorf("la entrada del registro sigue ahí")
	}
	if len(listInstalled()) == 0 {
		os.Remove(startMenuLink("TurboSETUP"))
	}
}

func TestE2EBadChecksum(t *testing.T) {
	base := os.Getenv("TURBOSETUP_E2E")
	if base == "" {
		t.Skip("define TURBOSETUP_E2E")
	}
	t.Setenv("TURBOSETUP_BASE", base)
	dst := filepath.Join(t.TempDir(), "x.exe")
	a := Asset{Asset: "TurboCUT.exe", SHA256: "00"}
	if err := download(context.Background(), a, dst, func(int64, int64) {}); err == nil {
		t.Fatal("debería fallar con un SHA-256 incorrecto")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Error("dejó el archivo a medias")
	}
}
