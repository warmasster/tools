//go:build windows

package shellx

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Guarda las imágenes en TURBOMENU_SHELLX_OUT (si está) para verlas a mano.
func save(t *testing.T, name string, b []byte) {
	if dir := os.Getenv("TURBOMENU_SHELLX_OUT"); dir != "" {
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, name), b, 0o644)
	}
}

// fullyTransparent: la imagen no tiene ni un punto visible.
func fullyTransparent(img image.Image) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0 {
				return false
			}
		}
	}
	return true
}

func decode(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("PNG no válido: %v", err)
	}
	return img
}

// samplePNG crea una imagen con degradado y una zona transparente.
func samplePNG(t *testing.T, dir string) string {
	img := image.NewNRGBA(image.Rect(0, 0, 640, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 640; x++ {
			a := uint8(255)
			if x > 480 {
				a = 0
			}
			img.Set(x, y, color.NRGBA{uint8(x * 255 / 640), 60, uint8(y * 255 / 400), a})
		}
	}
	p := filepath.Join(dir, "muestra.png")
	f, _ := os.Create(p)
	png.Encode(f, img)
	f.Close()
	return p
}

func TestImages(t *testing.T) {
	dir := t.TempDir()
	pngPath := samplePNG(t, dir)
	sys := os.Getenv("SystemRoot")
	cases := []struct {
		name  string
		path  string
		size  int
		thumb bool
	}{
		{"explorer", sys + `\explorer.exe`, 64, false},
		{"explorer256", sys + `\explorer.exe`, 256, false},
		{"carpeta", sys, 48, false},
		{"unidad", `C:\`, 48, false},
		{"pdf-inexistente", `C:\no\existe\documento.pdf`, 48, false},
		{"txt-inexistente", `C:\no\existe\nota.txt`, 256, false},
		{"carpeta-inexistente", `C:\no\existe\`, 32, false},
		{"miniatura-png", pngPath, 256, true},
		{"miniatura-carpeta", dir, 128, true},
		{"miniatura-exe", sys + `\notepad.exe`, 96, true},
	}
	for _, c := range cases {
		st := time.Now()
		var b []byte
		var err error
		if c.thumb {
			b, err = Thumbnail(c.path, c.size)
		} else {
			b, err = Icon(c.path, c.size)
		}
		d := time.Since(st)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		img := decode(t, b)
		bd := img.Bounds()
		if bd.Dx() > c.size || bd.Dy() > c.size || bd.Dx() < 8 {
			t.Errorf("%s: tamaño raro %v", c.name, bd)
		}
		// Segunda vez: caché.
		st2 := time.Now()
		if c.thumb {
			Thumbnail(c.path, c.size)
		} else {
			Icon(c.path, c.size)
		}
		t.Logf("%-20s %4dx%-4d %6d bytes  %v (caché %v)", c.name, bd.Dx(), bd.Dy(), len(b), d, time.Since(st2))
		save(t, c.name+".png", b)
	}
}

func TestThumbnailAlpha(t *testing.T) {
	dir := t.TempDir()
	b, err := Thumbnail(samplePNG(t, dir), 160)
	if err != nil {
		t.Fatal(err)
	}
	img := decode(t, b)
	bd := img.Bounds()
	// Arriba a la izquierda: opaco y oscuro; a la derecha: transparente.
	_, _, _, a := img.At(bd.Min.X+2, bd.Min.Y+bd.Dy()/2).RGBA()
	_, _, _, a2 := img.At(bd.Max.X-2, bd.Min.Y+bd.Dy()/2).RGBA()
	t.Logf("tamaño %v alfa izq=%d der=%d", bd, a>>8, a2>>8)
	if a == 0 && a2 == 0 && (fullyTransparent(img) || os.Getenv("GITHUB_ACTIONS") != "") {
		// Windows sin miniaturas (los servidores de GitHub Actions): el Explorador
		// devuelve una imagen vacía o el icono genérico, y aquí no hay nada que comprobar.
		t.Skip("este Windows no genera miniaturas de imágenes")
	}
	if a>>8 < 200 {
		t.Errorf("la parte opaca sale transparente (alfa %d)", a>>8)
	}
	// Orientación: abajo debe ser más azul que arriba.
	_, _, bTop, _ := img.At(bd.Min.X+10, bd.Min.Y+2).RGBA()
	_, _, bBot, _ := img.At(bd.Min.X+10, bd.Max.Y-3).RGBA()
	if bBot <= bTop {
		t.Errorf("imagen boca abajo: azul arriba %d, abajo %d", bTop>>8, bBot>>8)
	}
}

func TestParallel(t *testing.T) {
	sys := os.Getenv("SystemRoot")
	files := []string{sys + `\explorer.exe`, sys + `\regedit.exe`, sys + `\notepad.exe`, sys + `\System32\cmd.exe`,
		sys + `\System32\calc.exe`, sys + `\System32\mspaint.exe`, sys + `\System32\taskmgr.exe`, sys + `\System32\control.exe`}
	st := time.Now()
	done := make(chan error, len(files))
	for _, f := range files {
		go func(f string) { _, err := Icon(f, 40); done <- err }(f)
	}
	for range files {
		if err := <-done; err != nil {
			t.Error(err)
		}
	}
	t.Logf("%d iconos en paralelo: %v", len(files), time.Since(st))
}
