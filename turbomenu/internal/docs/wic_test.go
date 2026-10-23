package docs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// makeHEIC crea un HEIC con el codificador de Windows (si está instalado).
func makeHEIC(t *testing.T, src, dst string) bool {
	script := psWinRT + "$in = " + psQuote(src) + "\n$out = " + psQuote(dst) + `
$null = [Windows.Graphics.Imaging.BitmapDecoder, Windows.Graphics.Imaging, ContentType = WindowsRuntime]
$fs = [IO.File]::OpenRead($in)
$dec = AwaitOp ([Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync((RAS $fs))) ([Windows.Graphics.Imaging.BitmapDecoder])
$sb = AwaitOp ($dec.GetSoftwareBitmapAsync()) ([Windows.Graphics.Imaging.SoftwareBitmap])
$o = [IO.File]::Create($out)
$enc = AwaitOp ([Windows.Graphics.Imaging.BitmapEncoder]::CreateAsync([Windows.Graphics.Imaging.BitmapEncoder]::HeifEncoderId, (RAS $o))) ([Windows.Graphics.Imaging.BitmapEncoder])
$enc.SetSoftwareBitmap($sb)
AwaitAct ($enc.FlushAsync())
$o.Dispose(); $fs.Dispose()
`
	if err := runPS(context.Background(), script, nil); err != nil {
		t.Logf("no se puede crear HEIC: %v", err)
		return false
	}
	return fileSize(dst) > 0
}

func TestWIC(t *testing.T) {
	f := makeFixtures(t)
	ctx := context.Background()
	// El respaldo de Windows con un JPEG girado debe dar lo mismo que Go.
	d, err := decodeWIC(ctx, f.photoBig, "jpeg")
	if err != nil {
		t.Fatal(err)
	}
	if b := d.img.Bounds(); b.Dx() != 4000 && !testing.Short() {
		t.Errorf("WIC: %v (debería estar girada)", b)
	}
	// Archivo con extensión HEIC pero contenido basura.
	bad := filepath.Join(f.dir, "roto.heic")
	os.WriteFile(bad, []byte("no es una imagen de verdad, solo texto"), 0o644)
	_, err = InspectImage(bad)
	t.Logf("HEIC roto: %v", err)
	if err == nil || !strings.Contains(err.Error(), "dañada") {
		t.Errorf("error poco claro: %v", err)
	}
	if testing.Short() {
		t.Skip("HEIC real omitido con -short")
	}
	heic := filepath.Join(f.dir, "foto movil.heic")
	if !makeHEIC(t, f.photo, heic) {
		t.Skip("sin códec HEIF")
	}
	t0 := time.Now()
	info, err := InspectImage(heic)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("HEIC: %+v (%v)", info, time.Since(t0).Round(time.Millisecond))
	r, err := ReduceImage(ctx, heic, ImageOptions{Quality: 80, KeepLarger: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("HEIC -> %s %d KB", filepath.Base(r.Out), r.After/1024)
	checkDecode(t, r.Out, 1600, 1200)
	cleanOut(r.Out)
	r, err = Convert(ctx, heic, "png", ConvertOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	checkDecode(t, r.Out, 1600, 1200)
	cleanOut(r.Out)
	r, err = ImagesToPDF(ctx, []string{heic, f.photo}, PDFOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	validatePDF(t, r.Out)
	cleanOut(r.Out)
}

func TestCancel(t *testing.T) {
	f := makeFixtures(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReduceImage(ctx, f.alphaPNG, ImageOptions{Quality: 50, Scale: 90}, nil); !errors.Is(err, ErrCanceled) {
		t.Errorf("reducir: %v", err)
	}
	if _, err := ImagesToPDF(ctx, []string{f.photo, f.alphaPNG}, PDFOptions{}, nil); !errors.Is(err, ErrCanceled) {
		t.Errorf("a PDF: %v", err)
	}
	p := makeTestPDF(t)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel2()
	_, err := PDFToImages(ctx2, p, "png", 300, nil)
	if !errors.Is(err, ErrCanceled) {
		t.Errorf("PDF a imágenes: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(p), "documento (imágenes)")); err == nil {
		t.Error("no se ha borrado la carpeta a medias")
	}
	// Nada de temporales sueltos.
	ents, _ := os.ReadDir(f.dir)
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".turbo-") {
			t.Errorf("temporal olvidado: %s", e.Name())
		}
	}
}
