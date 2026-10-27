package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func solid(w, h int, y, cb, cr uint8) *image.YCbCr {
	m := newYUV(w, h)
	for i := range m.Y {
		m.Y[i] = y
	}
	for i := range m.Cb {
		m.Cb[i], m.Cr[i] = cb, cr
	}
	return m
}

func TestDecodeJPEG(t *testing.T) {
	src := solid(1280, 720, 120, 90, 160)
	var b bytes.Buffer
	if err := jpeg.Encode(&b, src, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	m, err := decodeJPEG(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if m.SubsampleRatio != image.YCbCrSubsampleRatio420 || m.Rect != image.Rect(0, 0, 1280, 720) {
		t.Fatalf("formato inesperado: %v %v", m.SubsampleRatio, m.Rect)
	}
	if d := int(m.Y[100]) - 120; d < -3 || d > 3 {
		t.Errorf("Y = %d, quería ~120", m.Y[100])
	}

	// Un JPEG 4:4:4 (como los de algunos navegadores) también se acepta.
	rgba := image.NewRGBA(image.Rect(0, 0, 33, 17)) // lados impares: se recortan
	for i := range rgba.Pix {
		rgba.Pix[i] = 200
	}
	b.Reset()
	jpeg.Encode(&b, rgba, &jpeg.Options{Quality: 95})
	m, err = decodeJPEG(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if m.Rect.Dx() != 32 || m.Rect.Dy() != 16 {
		t.Errorf("tamaño %v, quería 32x16", m.Rect)
	}
	if _, err := decodeJPEG([]byte("no es un jpeg")); err == nil {
		t.Error("debería fallar con datos que no son JPEG")
	}
}

func TestWriteNV12(t *testing.T) {
	m := newYUV(4, 2)
	copy(m.Y, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	m.Cb[0], m.Cb[1] = 10, 11
	m.Cr[0], m.Cr[1] = 20, 21
	dst := make([]byte, 12)
	writeNV12(dst, m)
	want := []byte{1, 2, 3, 4, 5, 6, 7, 8, 10, 20, 11, 21}
	if !bytes.Equal(dst, want) {
		t.Errorf("NV12 = %v, quería %v", dst, want)
	}
}

func TestScaleKeepsColor(t *testing.T) {
	src := solid(640, 360, 180, 70, 200)
	for _, sz := range [][2]int{{1280, 720}, {320, 180}, {640, 480}, {1920, 1080}} {
		dst := newYUV(sz[0], sz[1])
		drawScaled(dst, dst.Rect, src, true, 0)
		for _, i := range []int{0, len(dst.Y) / 2, len(dst.Y) - 1} {
			if dst.Y[i] != 180 {
				t.Fatalf("%v: Y[%d] = %d", sz, i, dst.Y[i])
			}
		}
		if dst.Cb[len(dst.Cb)-1] != 70 || dst.Cr[0] != 200 {
			t.Fatalf("%v: color cambiado", sz)
		}
	}
}

func TestFitRects(t *testing.T) {
	// 4:3 dentro de 16:9 rellenando: se recorta arriba y abajo.
	src, out := fitRects(640, 480, image.Rect(0, 0, 1280, 720), true)
	if out != image.Rect(0, 0, 1280, 720) || src.Dx() != 640 || src.Dy() != 360 || src.Min.Y != 60 {
		t.Errorf("cover: src %v out %v", src, out)
	}
	// Completo: bandas a los lados.
	src, out = fitRects(640, 480, image.Rect(0, 0, 1280, 720), false)
	if src != image.Rect(0, 0, 640, 480) || out.Dy() != 720 || out.Dx() != 960 || out.Min.X != 160 {
		t.Errorf("contain: src %v out %v", src, out)
	}
}

func TestPipStaysInside(t *testing.T) {
	bg := solid(1280, 720, 50, 128, 128)
	fg := solid(640, 360, 200, 128, 128)
	r := image.Rect(900, 500, 1240, 692)
	drawScaled(bg, r, fg, true, 19)
	if bg.Y[499*bg.YStride+1000] != 50 || bg.Y[600*bg.YStride+899] != 50 {
		t.Error("el recuadro se sale por fuera")
	}
	if bg.Y[600*bg.YStride+1000] != 200 {
		t.Error("el recuadro no se ha pintado")
	}
	// Esquina redondeada: el píxel de la esquina conserva el fondo.
	if bg.Y[500*bg.YStride+900] != 50 {
		t.Error("la esquina no está redondeada")
	}
}

func TestBGRAToYUV(t *testing.T) {
	m := newYUV(2, 2)
	white := []byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}
	bgraToYUV(m, white, 8)
	if m.Y[0] != 235 || m.Cb[0] != 128 || m.Cr[0] != 128 {
		t.Errorf("blanco = %d,%d,%d", m.Y[0], m.Cb[0], m.Cr[0])
	}
	red := []byte{0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255, 255}
	bgraToYUV(m, red, 8)
	want := color.RGBToYCbCr // como referencia (rango completo): solo se mira el sentido
	_, _, cr := want(255, 0, 0)
	if m.Cr[0] < 200 || cr < 200 {
		t.Errorf("rojo: Cr = %d", m.Cr[0])
	}
}

func TestEvenSize(t *testing.T) {
	for _, c := range [][4]int{{1281, 721, 1280, 720}, {3840, 2160, 1920, 1080}, {10, 10, 64, 64}} {
		if w, h := evenSize(c[0], c[1]); w != c[2] || h != c[3] {
			t.Errorf("evenSize(%d,%d) = %d,%d", c[0], c[1], w, h)
		}
	}
}

func BenchmarkFastPath720(b *testing.B) {
	src := solid(1280, 720, 100, 120, 130)
	dst := make([]byte, 1280*720*3/2)
	b.SetBytes(int64(len(dst)))
	for i := 0; i < b.N; i++ {
		writeNV12(dst, src)
	}
}

func BenchmarkScale1080to720(b *testing.B) {
	src := solid(1920, 1080, 100, 120, 130)
	dst := newYUV(1280, 720)
	for i := 0; i < b.N; i++ {
		drawScaled(dst, dst.Rect, src, true, 0)
	}
}
