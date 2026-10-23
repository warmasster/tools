package docs

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// withOrientation añade un bloque EXIF con la orientación dada a un JPEG.
func withOrientation(j []byte, o int) []byte {
	tiffHdr := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 0x01, 3, 0, 1, 0, 0, 0, byte(o), 0, 0, 0, 0, 0, 0, 0}
	payload := append([]byte("Exif\x00\x00"), tiffHdr...)
	seg := []byte{0xFF, 0xE1, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}
	out := append([]byte{}, j[:2]...)
	out = append(out, seg...)
	out = append(out, payload...)
	return append(out, j[2:]...)
}

type fixtures struct {
	dir                       string
	photo, photoBig, alphaPNG string
	graphicPNG, gifAnim       string
}

var fixOnce struct {
	done bool
	f    fixtures
}

func makeFixtures(t *testing.T) fixtures {
	t.Helper()
	if fixOnce.done {
		return fixOnce.f
	}
	dir := tmpDir(t)
	if os.Getenv("TURBO_TESTDIR") == "" {
		dir, _ = os.MkdirTemp("", "turbo-docs-test-*")
	}
	f := fixtures{dir: dir}
	w, h := 6000, 4000
	if testing.Short() {
		w, h = 1500, 1000
	}
	src := synthPhoto(w, h, false)
	var buf bytes.Buffer
	jpeg.Encode(&buf, src, &jpeg.Options{Quality: 95})
	f.photoBig = filepath.Join(dir, "foto grande.jpg")
	os.WriteFile(f.photoBig, withOrientation(buf.Bytes(), 6), 0o644)
	buf.Reset()
	small := synthPhoto(1600, 1200, false)
	jpeg.Encode(&buf, small, &jpeg.Options{Quality: 92})
	f.photo = filepath.Join(dir, "foto.jpg")
	os.WriteFile(f.photo, buf.Bytes(), 0o644)
	buf.Reset()
	png.Encode(&buf, synthPhoto(2000, 1500, true))
	f.alphaPNG = filepath.Join(dir, "alfa.png")
	os.WriteFile(f.alphaPNG, buf.Bytes(), 0o644)
	buf.Reset()
	png.Encode(&buf, synthGraphic(1920, 1080))
	f.graphicPNG = filepath.Join(dir, "captura.png")
	os.WriteFile(f.graphicPNG, buf.Bytes(), 0o644)
	// GIF animado de 6 fotogramas.
	g := &gif.GIF{}
	for i := range 6 {
		fr := synthPhoto(320, 240, false)
		for y := 0; y < 240; y++ {
			for x := i * 40; x < i*40+40; x++ {
				fr.Pix[y*fr.Stride+x*4] = 255
			}
		}
		pm := gifPaletted(fr, 100)
		g.Image = append(g.Image, pm)
		g.Delay = append(g.Delay, 10)
	}
	buf.Reset()
	gif.EncodeAll(&buf, g)
	f.gifAnim = filepath.Join(dir, "anim.gif")
	os.WriteFile(f.gifAnim, buf.Bytes(), 0o644)
	fixOnce.done, fixOnce.f = true, f
	return f
}

func cleanOut(p string) {
	if p != "" && os.Getenv("TURBO_TESTDIR") == "" {
		os.RemoveAll(p)
	}
}

func renameForView(t *testing.T, p, name string) {
	if os.Getenv("TURBO_TESTDIR") == "" {
		os.RemoveAll(p)
		return
	}
	dst := filepath.Join(filepath.Dir(p), "ver_"+name)
	os.RemoveAll(dst)
	os.Rename(p, dst)
}

func TestImages(t *testing.T) {
	f := makeFixtures(t)
	ctx := context.Background()

	info, err := InspectImage(f.photoBig)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("info: %+v", info)
	if info.Width >= info.Height {
		t.Errorf("no se ha respetado la orientación EXIF: %dx%d", info.Width, info.Height)
	}
	// Reducir JPEG grande.
	t0 := time.Now()
	r, err := ReduceImage(ctx, f.photoBig, ImageOptions{Quality: 75}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("JPEG %dx%d q75: %d KB -> %d KB en %v (%s)", r.Width, r.Height, r.Before/1024, r.After/1024, time.Since(t0).Round(time.Millisecond), filepath.Base(r.Out))
	checkDecode(t, r.Out, r.Width, r.Height)
	if !strings.HasSuffix(r.Out, "foto grande (reducida).jpg") {
		t.Errorf("nombre inesperado: %s", r.Out)
	}
	r2, err := ReduceImage(ctx, f.photoBig, ImageOptions{Quality: 70, MaxSide: 1200}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("JPEG q70 + 1200 px: %d KB (%dx%d) %s", r2.After/1024, r2.Width, r2.Height, filepath.Base(r2.Out))
	if !strings.HasSuffix(r2.Out, "(reducida 2).jpg") || max(r2.Width, r2.Height) != 1200 {
		t.Errorf("resultado inesperado: %+v", r2)
	}
	checkDecode(t, r2.Out, r2.Width, r2.Height)
	// Si no se gana nada: ErrNotSmaller.
	small := filepath.Join(f.dir, "ya reducida.jpg")
	data, _ := os.ReadFile(r2.Out)
	os.WriteFile(small, data, 0o644)
	if r3, err := ReduceImage(ctx, small, ImageOptions{Quality: 100}, nil); !errors.Is(err, ErrNotSmaller) || !r3.Larger || r3.Out != "" {
		t.Errorf("se esperaba ErrNotSmaller: %+v %v", r3, err)
	}
	renameForView(t, r2.Out, "reducida_1200.jpg")
	cleanOut(r.Out)

	// PNG con alfa: sin pérdida y cuantizado.
	for _, q := range []int{100, 80, 40} {
		t0 := time.Now()
		r, err := ReduceImage(ctx, f.alphaPNG, ImageOptions{Quality: q, KeepLarger: true}, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("PNG alfa q%d: %d KB -> %d KB en %v", q, r.Before/1024, r.After/1024, time.Since(t0).Round(time.Millisecond))
		m := checkDecode(t, r.Out, 2000, 1500)
		if _, _, _, a := m.At(5, 5).RGBA(); a != 0 {
			t.Errorf("q%d: se ha perdido la transparencia", q)
		}
		if _, _, _, a := m.At(1000, 750).RGBA(); a != 0xffff {
			t.Errorf("q%d: el centro debería ser opaco", q)
		}
		if q == 100 {
			orig, _ := os.ReadFile(f.alphaPNG)
			om, _ := png.Decode(bytes.NewReader(orig))
			if !samePixels(om, m) {
				t.Error("q100 debería ser sin pérdida")
			}
		}
		renameForView(t, r.Out, "alfa_q"+strconv.Itoa(q)+".png")
	}
	// Captura de pantalla: paleta exacta sin pérdida.
	r, err = ReduceImage(ctx, f.graphicPNG, ImageOptions{Quality: 100}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("PNG captura q100: %d KB -> %d KB", r.Before/1024, r.After/1024)
	gm := checkDecode(t, r.Out, 1920, 1080)
	orig, _ := os.ReadFile(f.graphicPNG)
	om, _ := png.Decode(bytes.NewReader(orig))
	if !samePixels(om, gm) {
		t.Error("la captura debería quedar idéntica")
	}
	cleanOut(r.Out)

	// GIF animado.
	r, err = ReduceImage(ctx, f.gifAnim, ImageOptions{Quality: 50, Scale: 50, KeepLarger: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	gf, _ := os.Open(r.Out)
	ga, err := gif.DecodeAll(gf)
	gf.Close()
	if err != nil || len(ga.Image) != 6 || ga.Config.Width != 160 {
		t.Errorf("GIF animado mal: %v", err)
	}
	t.Logf("GIF animado 50%% q50: %d KB -> %d KB", r.Before/1024, r.After/1024)
	renameForView(t, r.Out, "anim50.gif")
}

// samePixels compara dos imágenes (ignorando el color de los píxeles invisibles).
func samePixels(a, b image.Image) bool {
	bb := a.Bounds()
	for y := bb.Min.Y; y < bb.Max.Y; y++ {
		for x := bb.Min.X; x < bb.Max.X; x++ {
			r1, g1, b1, a1 := a.At(x, y).RGBA()
			r2, g2, b2, a2 := b.At(x, y).RGBA()
			if a1>>8 != a2>>8 {
				return false
			}
			if a1 == 0 {
				continue
			}
			if r1>>8 != r2>>8 || g1>>8 != g2>>8 || b1>>8 != b2>>8 {
				return false
			}
		}
	}
	return true
}

func checkDecode(t *testing.T, p string, w, h int) image.Image {
	t.Helper()
	fh, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	m, _, err := image.Decode(fh)
	if err != nil {
		t.Fatalf("%s no se decodifica: %v", p, err)
	}
	if b := m.Bounds(); b.Dx() != w || b.Dy() != h {
		t.Errorf("%s mide %dx%d, se esperaba %dx%d", p, b.Dx(), b.Dy(), w, h)
	}
	return m
}

func TestPreview(t *testing.T) {
	f := makeFixtures(t)
	t0 := time.Now()
	p, err := Preview(f.photoBig, ImageOptions{Quality: 80}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("primera vista previa (abre la foto): %v, %d KB, %dx%d", time.Since(t0).Round(time.Millisecond), p.Size/1024, p.Width, p.Height)
	for _, q := range []int{70, 60, 50, 40, 30, 85, 95} {
		t0 := time.Now()
		p, err := Preview(f.photoBig, ImageOptions{Quality: q}, 1000)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("q%d: %v -> %d KB", q, time.Since(t0).Round(time.Millisecond), p.Size/1024)
	}
	// El peso de la vista previa coincide con el archivo final.
	p, _ = Preview(f.photoBig, ImageOptions{Quality: 55, MaxSide: 2500}, 1000)
	t0 = time.Now()
	r, err := ReduceImage(context.Background(), f.photoBig, ImageOptions{Quality: 55, MaxSide: 2500}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("guardar tras la vista previa: %v", time.Since(t0).Round(time.Millisecond))
	if r.After != p.Size {
		t.Errorf("peso de la vista previa %d != archivo %d", p.Size, r.After)
	}
	cleanOut(r.Out)
	for _, q := range []int{90, 60, 30} {
		t0 := time.Now()
		p, err := Preview(f.alphaPNG, ImageOptions{Quality: q}, 800)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("PNG alfa 3 MP q%d: %v -> %d KB", q, time.Since(t0).Round(time.Millisecond), p.Size/1024)
		if q == 30 && os.Getenv("TURBO_TESTDIR") != "" {
			saveDataURL(t, p.Original, filepath.Join(f.dir, "ver_prev_orig.png"))
			saveDataURL(t, p.Result, filepath.Join(f.dir, "ver_prev_q30.png"))
		}
	}
	p, _ = Preview(f.photoBig, ImageOptions{Quality: 20}, 1000)
	if os.Getenv("TURBO_TESTDIR") != "" {
		saveDataURL(t, p.Result, filepath.Join(f.dir, "ver_prev_jpg_q20.jpg"))
	}
	// Llamadas solapadas: la vieja termina con ErrSuperseded (o a tiempo).
	errs := make(chan error, 2)
	go func() { _, err := Preview(f.photoBig, ImageOptions{Quality: 33, MaxSide: 3000}, 1000); errs <- err }()
	time.Sleep(5 * time.Millisecond)
	go func() { _, err := Preview(f.photoBig, ImageOptions{Quality: 34}, 1000); errs <- err }()
	e1, e2 := <-errs, <-errs
	t.Logf("solapadas: %v / %v", e1, e2)
}

func saveDataURL(t *testing.T, u, p string) {
	i := strings.Index(u, ",")
	data, err := base64.StdEncoding.DecodeString(u[i+1:])
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(p, data, 0o644)
}

func TestConvert(t *testing.T) {
	f := makeFixtures(t)
	ctx := context.Background()
	for _, tg := range Targets(f.alphaPNG) {
		r, err := Convert(ctx, f.alphaPNG, tg.ID, ConvertOptions{}, nil)
		if err != nil {
			t.Fatalf("%s: %v", tg.ID, err)
		}
		t.Logf("alfa.png -> %s: %d KB (%s)", tg.ID, r.After/1024, filepath.Base(r.Out))
		switch tg.ID {
		case "pdf":
			validatePDF(t, r.Out)
		case "ico":
			checkICO(t, r.Out)
		default:
			m := checkDecodeAny(t, r.Out)
			if tg.ID == "jpg" {
				if rr, g, b, _ := m.At(3, 3).RGBA(); rr>>8 < 250 || g>>8 < 250 || b>>8 < 250 {
					t.Error("el fondo transparente debería ser blanco en JPG")
				}
			}
		}
		renameForView(t, r.Out, "conv_"+tg.ID+tg.Ext)
	}
	r, err := Convert(ctx, f.photo, "png", ConvertOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(r.Out, "foto.png") {
		t.Errorf("nombre: %s", r.Out)
	}
	r2, _ := Convert(ctx, f.photo, "png", ConvertOptions{}, nil)
	if !strings.HasSuffix(r2.Out, "foto (2).png") {
		t.Errorf("nombre: %s", r2.Out)
	}
	cleanOut(r.Out)
	cleanOut(r2.Out)
	if len(Targets(filepath.Join(f.dir, "x.docx"))) != 1 || len(Targets(filepath.Join(f.dir, "x.pdf"))) != 2 {
		t.Error("Targets mal")
	}
}

func checkDecodeAny(t *testing.T, p string) image.Image {
	t.Helper()
	d, err := decodeFile(context.Background(), p)
	if err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	return d.img
}

func checkICO(t *testing.T, p string) {
	data, _ := os.ReadFile(p)
	if len(data) < 6 || data[2] != 1 {
		t.Fatal("ICO mal")
	}
	n := int(data[4])
	for i := range n {
		e := data[6+16*i:]
		size := int(e[8]) | int(e[9])<<8 | int(e[10])<<16
		off := int(e[12]) | int(e[13])<<8 | int(e[14])<<16
		if _, err := png.Decode(bytes.NewReader(data[off : off+size])); err != nil {
			t.Fatalf("entrada %d del ICO: %v", i, err)
		}
	}
	t.Logf("ICO con %d tamaños", n)
}

func validatePDF(t *testing.T, p string) int {
	t.Helper()
	fh, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	conf := model.NewStatelessConfiguration()
	conf.ValidationMode = model.ValidationStrict
	pc, err := api.ReadAndValidate(context.Background(), fh, conf)
	if err != nil {
		t.Fatalf("%s no es un PDF válido: %v", p, err)
	}
	return pc.PageCount
}

func TestImagesToPDF(t *testing.T) {
	f := makeFixtures(t)
	t0 := time.Now()
	r, err := ImagesToPDF(context.Background(), []string{f.photoBig, f.photo, f.alphaPNG, f.graphicPNG, f.gifAnim}, PDFOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	n := validatePDF(t, r.Out)
	t.Logf("5 imágenes -> PDF A4: %d páginas, %d KB en %v", n, r.After/1024, time.Since(t0).Round(time.Millisecond))
	if n != 5 {
		t.Errorf("páginas: %d", n)
	}
	r2, err := ImagesToPDF(context.Background(), []string{f.photo, f.alphaPNG}, PDFOptions{PageSize: "image"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	validatePDF(t, r2.Out)
	renameForView(t, r.Out, "imagenes.pdf")
	cleanOut(r2.Out)
}

// makeDocx crea un .docx mínimo válido.
func makeDocx(p string) error {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":   `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:rPr><w:b/><w:sz w:val="48"/></w:rPr><w:t>Documento de prueba</w:t></w:r></w:p><w:p><w:r><w:t>Ñandú, acentos: áéíóú. Una línea normal de texto.</w:t></w:r></w:p></w:body></w:document>`,
	}
	for name, body := range files {
		w, _ := z.Create(name)
		w.Write([]byte(body))
	}
	z.Close()
	return os.WriteFile(p, buf.Bytes(), 0o644)
}

func TestOffice(t *testing.T) {
	a := DetectOffice()
	eng, ok := OfficeEngine()
	t.Logf("Office detectado: %+v -> %q %v", a, eng, ok)
	dir := tmpDir(t)
	doc := filepath.Join(dir, "carta ñ.docx")
	if err := makeDocx(doc); err != nil {
		t.Fatal(err)
	}
	e2, ok2 := OfficeEngineFor(doc)
	t.Logf("para .docx: %q %v", e2, ok2)
	if !ok {
		t.Skip("no hay Office ni LibreOffice")
	}
	if testing.Short() {
		t.Skip("conversión real omitida con -short")
	}
	for i := range 2 {
		t0 := time.Now()
		r, err := OfficeToPDF(context.Background(), doc, "")
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("docx -> PDF (%d): %d KB en %v (%s)", i+1, r.After/1024, time.Since(t0).Round(time.Millisecond), filepath.Base(r.Out))
		if validatePDF(t, r.Out) != 1 {
			t.Error("debería tener 1 página")
		}
		if i == 1 {
			renameForView(t, r.Out, "docx.pdf")
		} else {
			cleanOut(r.Out)
		}
	}
	// Cancelación.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err := OfficeToPDF(ctx, doc, "")
	if !errors.Is(err, ErrCanceled) {
		t.Errorf("cancelación: %v", err)
	}
}

func BenchmarkQuantize(b *testing.B) {
	m := synthPhoto(2000, 1500, true)
	b.ResetTimer()
	for range b.N {
		quantize(m, 256, 0.9)
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	// Borra los archivos de prueba temporales (no los de TURBO_TESTDIR).
	if fixOnce.done && os.Getenv("TURBO_TESTDIR") == "" {
		os.RemoveAll(fixOnce.f.dir)
	}
	os.Exit(code)
}
