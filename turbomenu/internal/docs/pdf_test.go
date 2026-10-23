package docs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func makeTestPDF(t *testing.T) string {
	f := makeFixtures(t)
	// PNG de foto (va con Flate) para probar también esa recompresión.
	r, err := ImagesToPDF(context.Background(), []string{f.photoBig, f.alphaPNG, f.graphicPNG, f.photo}, PDFOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(f.dir, "documento.pdf")
	os.Remove(p)
	os.Rename(r.Out, p)
	return p
}

func TestPDF(t *testing.T) {
	ctx := context.Background()
	p := makeTestPDF(t)
	info, err := InspectPDF(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("info: %+v", info)
	if info.Pages != 4 || info.Images < 4 {
		t.Errorf("info mal: %+v", info)
	}
	for _, lv := range []PDFLevel{PDFScreen, PDFPrint} {
		t0 := time.Now()
		r, err := ReducePDF(ctx, p, lv, nil)
		if err != nil {
			t.Fatal(err)
		}
		n := validatePDF(t, r.Out)
		t.Logf("reducir %s: %d KB -> %d KB (%d páginas) en %v", lv, r.Before/1024, r.After/1024, n, time.Since(t0).Round(time.Millisecond))
		if lv == PDFScreen && !testing.Short() {
			// Renderizar la página 1 y 2 para verla.
			ri, err := PDFToImages(ctx, r.Out, "png", 60, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("renderizado: %s", ri.Out)
			renameForView(t, ri.Out, "reducido_paginas")
		}
		renameForView(t, r.Out, "reducido_"+string(lv)+".pdf")
	}
	// PDF -> imágenes.
	t0 := time.Now()
	var last float64
	r, err := PDFToImages(ctx, p, "jpg", 100, func(f float64) { last = f })
	if err != nil {
		t.Fatal(err)
	}
	ents, _ := os.ReadDir(r.Out)
	t.Logf("PDF -> JPG 100 ppp: %d imágenes en %v (progreso final %.2f) en %s", len(ents), time.Since(t0).Round(time.Millisecond), last, filepath.Base(r.Out))
	if len(ents) != 4 {
		t.Errorf("se esperaban 4 imágenes")
	}
	for _, e := range ents {
		checkDecodeAny(t, filepath.Join(r.Out, e.Name()))
	}
	cleanOut(r.Out)
	// Unir y dividir.
	m, err := MergePDFs(ctx, []string{p, p}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n := validatePDF(t, m.Out); n != 8 {
		t.Errorf("unido: %d páginas", n)
	}
	t.Logf("unido: %s %d KB", filepath.Base(m.Out), m.After/1024)
	s, err := SplitPDF(ctx, m.Out, nil)
	if err != nil {
		t.Fatal(err)
	}
	ents, _ = os.ReadDir(s.Out)
	t.Logf("dividido en %d archivos (%s), p.ej. %s", len(ents), filepath.Base(s.Out), ents[0].Name())
	if len(ents) != 8 {
		t.Errorf("dividido: %d", len(ents))
	}
	for _, e := range ents {
		if validatePDF(t, filepath.Join(s.Out, e.Name())) != 1 {
			t.Error("cada página debería tener 1 página")
		}
	}
	cleanOut(m.Out)
	cleanOut(s.Out)
	// PDF sin imágenes: no se gana -> ErrNotSmaller o algo más pequeño.
	one := filepath.Join(filepath.Dir(p), "una.pdf")
	data, _ := os.ReadFile(filepath.Join(s.Out, ents[0].Name()))
	_ = data
	// Archivo que no es PDF.
	bad := filepath.Join(filepath.Dir(p), "malo.pdf")
	os.WriteFile(bad, []byte("esto no es un pdf"), 0o644)
	if _, err := ReducePDF(ctx, bad, PDFScreen, nil); err == nil {
		t.Error("debería fallar")
	} else {
		t.Logf("PDF malo: %v", err)
	}
	if _, err := PDFToImages(ctx, bad, "png", 0, nil); err == nil {
		t.Error("debería fallar")
	} else {
		t.Logf("PDF malo a imágenes: %v", err)
	}
	_ = one
}

func TestEncryptedPDF(t *testing.T) {
	if testing.Short() {
		t.Skip("omitido con -short")
	}
	ctx := context.Background()
	p := makeTestPDF(t)
	dir := filepath.Dir(p)
	// Con contraseña para abrir.
	locked := filepath.Join(dir, "con clave.pdf")
	if err := api.EncryptFile(ctx, p, locked, model.NewAESConfiguration("abc", "abc", 256)); err != nil {
		t.Fatal(err)
	}
	if _, err := ReducePDF(ctx, locked, PDFScreen, nil); err == nil || !strings.Contains(err.Error(), "contraseña") {
		t.Errorf("reducir con clave: %v", err)
	}
	if _, err := PDFToImages(ctx, locked, "png", 50, nil); err == nil || !strings.Contains(err.Error(), "contraseña") {
		t.Errorf("a imágenes con clave: %v", err)
	}
	if _, err := InspectPDF(locked); err == nil {
		t.Error("info con clave debería fallar")
	} else {
		t.Logf("info con clave: %v", err)
	}
	// Solo con restricciones (se abre sin contraseña).
	restr := filepath.Join(dir, "restringido.pdf")
	if err := api.EncryptFile(ctx, p, restr, model.NewAESConfiguration("", "dueño", 256)); err != nil {
		t.Fatal(err)
	}
	r, err := ReducePDF(ctx, restr, PDFScreen, nil)
	if err != nil {
		t.Fatalf("reducir restringido: %v", err)
	}
	t.Logf("restringido: %d KB -> %d KB", r.Before/1024, r.After/1024)
	ri, err := PDFToImages(ctx, r.Out, "png", 40, nil)
	if err != nil {
		t.Fatalf("el reducido restringido no se abre en Windows: %v", err)
	}
	cleanOut(ri.Out)
	cleanOut(r.Out)
}

func TestRealPDF(t *testing.T) {
	src := os.Getenv("TURBO_REALPDF")
	dir := os.Getenv("TURBO_TESTDIR")
	if src == "" || dir == "" {
		t.Skip("sin PDF real")
	}
	os.MkdirAll(dir, 0o755)
	data, _ := os.ReadFile(src)
	p := filepath.Join(dir, "real.pdf")
	os.WriteFile(p, data, 0o644)
	info, err := InspectPDF(p)
	t.Logf("%+v %v", info, err)
	for _, lv := range []PDFLevel{PDFScreen, PDFPrint} {
		t0 := time.Now()
		r, err := ReducePDF(context.Background(), p, lv, nil)
		if errors.Is(err, ErrNotSmaller) {
			t.Logf("%s: no se gana nada (%d -> %d)", lv, r.Before, r.After)
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %d KB -> %d KB en %v", lv, r.Before/1024, r.After/1024, time.Since(t0).Round(time.Millisecond))
		validatePDF(t, r.Out)
		renameForView(t, r.Out, "realpdf_"+string(lv)+".pdf")
	}
}
