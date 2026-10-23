package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testApp(action string, paths ...string) *App {
	return NewApp(&Job{Action: action, Paths: paths}, false)
}

func TestZip(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "Fotos")
	os.MkdirAll(filepath.Join(sub, "dentro"), 0o755)
	os.WriteFile(filepath.Join(sub, "a.txt"), []byte(strings.Repeat("hola ", 10000)), 0o644)
	os.WriteFile(filepath.Join(sub, "dentro", "b.jpg"), []byte("jpg"), 0o644)
	a := testApp("reduce", sub)
	p := a.Plan("reduce")
	if p.Kind != "zip" || len(p.Presets) != 1 {
		t.Fatalf("plan: %+v", p)
	}
	r := a.Run(RunReq{Mode: "reduce", Preset: "zip"})
	if len(r.Results) != 1 || r.Results[0].Error != "" {
		t.Fatalf("run: %+v", r)
	}
	if !strings.HasSuffix(r.Results[0].Out, "Fotos.zip") || r.Results[0].After >= r.Results[0].Before {
		t.Fatalf("zip: %+v", r.Results[0])
	}
}

func TestSuggestNames(t *testing.T) {
	got := suggestNames([]string{`C:\x\Vacaciones playa 01.jpg`, `C:\x\Vacaciones playa 02.jpg`, `C:\x\Vacaciones playa 03.jpg`})
	if len(got) == 0 || got[0] != "Vacaciones playa" {
		t.Fatalf("sugerencias: %v", got)
	}
}

// TURBOMENU_VIDEO=<ruta> go test -run VideoReal -v .
func TestVideoReal(t *testing.T) {
	v := os.Getenv("TURBOMENU_VIDEO")
	if v == "" || testing.Short() {
		t.Skip("sin TURBOMENU_VIDEO")
	}
	a := testApp("reduce", v)
	p := a.Plan("reduce")
	t.Logf("plan: %s %+v", p.Info, p.Qualities)
	if p.Error != "" || p.Needs != "" {
		t.Fatalf("plan: %+v", p)
	}
	r := a.Run(RunReq{Mode: "reduce", Preset: "discord"})
	if len(r.Results) != 1 || r.Results[0].Error != "" {
		t.Fatalf("run: %+v", r)
	}
	t.Logf("%s → %s (%d → %d)", r.Results[0].In, r.Results[0].Out, r.Results[0].Before, r.Results[0].After)
	if r.Results[0].After > 10<<20 {
		t.Fatal("no cabe en Discord")
	}
	os.Remove(r.Results[0].Out)
	c := testApp("convert", v)
	cp := c.Plan("convert")
	if len(cp.Targets) == 0 {
		t.Fatalf("sin formatos: %+v", cp)
	}
	cr := c.Run(RunReq{Mode: "convert", Target: "gif", GIFFps: 10, GIFWidth: 320})
	if cr.Results[0].Error != "" {
		t.Fatalf("gif: %+v", cr)
	}
	t.Logf("gif: %s %d", cr.Results[0].Out, cr.Results[0].After)
	os.Remove(cr.Results[0].Out)
}

// TURBOMENU_REG=1 go test -run MenuReal -v .  (escribe y borra las claves de HKCU)
func TestMenuReal(t *testing.T) {
	if os.Getenv("TURBOMENU_REG") == "" {
		t.Skip()
	}
	exe := `C:\Prueba\TurboMENU.exe`
	if err := registerMenu(exe); err != nil {
		t.Fatal(err)
	}
	defer unregisterMenu()
	if !menuInstalled(exe) {
		t.Fatal("no aparece instalado")
	}
	for root, verbs := range menuLayout() {
		t.Logf("%s: %d entradas", root, len(verbs))
	}
}

// Imagen y PDF de punta a punta con una foto de los fondos de Windows (copiada).
func TestImagePDFFlow(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	src, _ := filepath.Glob(`C:\Windows\Web\Wallpaper\*\*.jpg`)
	if len(src) == 0 {
		t.Skip("sin fondos")
	}
	dir := t.TempDir()
	foto := filepath.Join(dir, "foto playa.jpg")
	b, _ := os.ReadFile(src[0])
	os.WriteFile(foto, b, 0o644)

	a := testApp("reduce", foto)
	p := a.Plan("reduce")
	if p.Kind != "image" || p.Image == nil {
		t.Fatalf("plan: %+v", p)
	}
	pv, err := a.ImagePreview(foto, 60, "50")
	if err != nil {
		t.Fatal(err)
	}
	r := a.Run(RunReq{Mode: "reduce", Quality: 60, Scale: "50"})
	if r.Results[0].Error != "" || r.Results[0].After != pv["size"].(int64) {
		t.Fatalf("reducir: %+v previa=%v", r.Results[0], pv["size"])
	}
	t.Logf("foto: %d → %d", r.Results[0].Before, r.Results[0].After)

	c := testApp("convert", foto, r.Results[0].Out)
	cp := c.Plan("convert")
	if cp.Targets[0].ID != "pdfjoin" {
		t.Fatalf("formatos: %+v", cp.Targets)
	}
	cr := c.Run(RunReq{Mode: "convert", Target: "pdfjoin"})
	if cr.Results[0].Error != "" {
		t.Fatalf("pdf: %+v", cr)
	}
	pdf := cr.Results[0].Out
	d := testApp("reduce", pdf)
	dr := d.Run(RunReq{Mode: "reduce", Preset: "screen"})
	t.Logf("pdf: %+v", dr.Results[0])
	e := testApp("convert", pdf)
	er := e.Run(RunReq{Mode: "convert", Target: "png"})
	if er.Results[0].Error != "" {
		t.Fatalf("pdf→png: %+v", er)
	}
	t.Logf("pdf→png: %s", er.Results[0].Out)
}
