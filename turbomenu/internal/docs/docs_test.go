package docs

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// synthPhoto genera una "foto" sintética: degradados suaves, ruido de grano,
// bordes nítidos y texto simulado. Reproducible.
func synthPhoto(w, h int, alpha bool) *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	// Ruido de valor a baja resolución, interpolado.
	const g = 24
	gw, gh := w/g+2, h/g+2
	rnd := rand.New(rand.NewPCG(1, 2))
	grid := make([][3]float64, gw*gh)
	for i := range grid {
		grid[i] = [3]float64{rnd.Float64(), rnd.Float64(), rnd.Float64()}
	}
	parallel(h, func(lo, hi int) {
		r := rand.New(rand.NewPCG(uint64(lo), 7))
		for y := lo; y < hi; y++ {
			fy := float64(y) / g
			iy := int(fy)
			ty := fy - float64(iy)
			ty = ty * ty * (3 - 2*ty)
			for x := range w {
				fx := float64(x) / g
				ix := int(fx)
				tx := fx - float64(ix)
				tx = tx * tx * (3 - 2*tx)
				var c [3]float64
				for k := range 3 {
					a := grid[iy*gw+ix][k]*(1-tx) + grid[iy*gw+ix+1][k]*tx
					b := grid[(iy+1)*gw+ix][k]*(1-tx) + grid[(iy+1)*gw+ix+1][k]*tx
					c[k] = a*(1-ty) + b*ty
				}
				// Formas con bordes duros.
				if (x/(w/7)+y/(h/5))%3 == 0 && (x%(w/7)) < w/14 {
					c[0], c[1], c[2] = c[0]*0.3, c[1]*0.3+0.5, c[2]*0.2
				}
				// "Texto": rayas finas.
				if y%(h/6) < h/60 && (x/3)%5 < 2 {
					c = [3]float64{0.05, 0.05, 0.05}
				}
				n := (r.Float64() - 0.5) * 0.04
				i := y*m.Stride + x*4
				for k := range 3 {
					m.Pix[i+k] = uint8(math.Max(0, math.Min(255, (c[k]+n)*255)))
				}
				m.Pix[i+3] = 255
				if alpha {
					dx, dy := float64(x-w/2)/float64(w/2), float64(y-h/2)/float64(h/2)
					d := math.Sqrt(dx*dx + dy*dy)
					a := math.Max(0, math.Min(1, (1.0-d)*4))
					m.Pix[i+3] = uint8(a * 255)
				}
			}
		}
	})
	return m
}

// synthGraphic: captura de pantalla / gráfico con pocos colores.
func synthGraphic(w, h int) *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	cols := []color.NRGBA{{245, 245, 247, 255}, {30, 30, 30, 255}, {0, 120, 215, 255}, {220, 50, 47, 255}, {255, 255, 255, 255}, {120, 120, 120, 255}}
	for y := range h {
		for x := range w {
			c := cols[0]
			switch {
			case y < 40:
				c = cols[2]
			case x < 200:
				c = cols[4]
			case (y/18)%3 == 0 && (x/7)%4 != 0 && x < w-300:
				c = cols[1]
			case x > w-300 && y > 100 && y < 400:
				c = cols[3]
			case (x+y)%97 == 0:
				c = cols[5]
			}
			m.SetNRGBA(x, y, c)
		}
	}
	return m
}

func psnr(a, b image.Image) float64 {
	bb := a.Bounds()
	var se float64
	n := 0
	for y := bb.Min.Y; y < bb.Max.Y; y += 3 {
		for x := bb.Min.X; x < bb.Max.X; x += 3 {
			r1, g1, b1, _ := a.At(x, y).RGBA()
			r2, g2, b2, _ := b.At(x, y).RGBA()
			for _, d := range []float64{float64(r1>>8) - float64(r2>>8), float64(g1>>8) - float64(g2>>8), float64(b1>>8) - float64(b2>>8)} {
				se += d * d
			}
			n += 3
		}
	}
	mse := se / float64(n)
	if mse == 0 {
		return 99
	}
	return 10 * math.Log10(255*255/mse)
}

func tmpDir(t *testing.T) string {
	if d := os.Getenv("TURBO_TESTDIR"); d != "" {
		os.MkdirAll(d, 0o755)
		return d
	}
	return t.TempDir()
}

func TestJPEGEncoder(t *testing.T) {
	w, h := 6000, 4000
	if testing.Short() {
		w, h = 1200, 800
	}
	src := synthPhoto(w, h, false)
	for _, q := range []int{40, 75, 90, 97} {
		t0 := time.Now()
		r, err := encodeJPEG(context.Background(), src, q, nil, 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		mine := time.Since(t0)
		dec, err := jpeg.Decode(bytes.NewReader(r.data))
		if err != nil {
			t.Fatalf("q%d: el JPEG no se decodifica: %v", q, err)
		}
		t0 = time.Now()
		var std bytes.Buffer
		jpeg.Encode(&std, src, &jpeg.Options{Quality: q})
		stdT := time.Since(t0)
		stdDec, _ := jpeg.Decode(bytes.NewReader(std.Bytes()))
		// La reconstrucción interna debe coincidir con lo que decodifica Go.
		pr := psnr(dec, r.recon)
		t.Logf("q%d: propio %d KB en %v (PSNR %.2f dB) | stdlib %d KB en %v (PSNR %.2f dB) | recon vs decod %.1f dB",
			q, len(r.data)/1024, mine.Round(time.Millisecond), psnr(src, dec), std.Len()/1024, stdT.Round(time.Millisecond), psnr(src, stdDec), pr)
		if pr < 40 {
			t.Errorf("q%d: la reconstrucción no coincide con el decodificador (%.1f dB)", q, pr)
		}
		// Reconstrucción a escala 1/2, 1/4 y 1/8 frente al decodificado reducido.
		for _, side := range []int{w / 2, w / 4, w / 8} {
			rs, _ := encodeJPEG(context.Background(), src, q, nil, side)
			b := rs.recon.Bounds()
			ref := resizeImage(normalize(dec), b.Dx(), b.Dy())
			if p := psnr(ref, rs.recon); p < 22 {
				t.Errorf("q%d: reconstrucción escalada a %dx%d mala (%.1f dB)", q, b.Dx(), b.Dy(), p)
			}
		}
	}
	// YCbCr de entrada (camino de las fotos JPEG) y gris.
	var buf bytes.Buffer
	jpeg.Encode(&buf, src, &jpeg.Options{Quality: 95})
	ycc, _ := jpeg.Decode(bytes.NewReader(buf.Bytes()))
	t0 := time.Now()
	r, err := encodeJPEG(context.Background(), ycc, 75, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jpeg.Decode(bytes.NewReader(r.data)); err != nil {
		t.Fatal(err)
	}
	t.Logf("YCbCr q75: %d KB en %v", len(r.data)/1024, time.Since(t0).Round(time.Millisecond))
	gray := image.NewGray(image.Rect(0, 0, 999, 777))
	for i := range gray.Pix {
		gray.Pix[i] = uint8(i * 7 % 251)
	}
	r, err = encodeJPEG(context.Background(), gray, 80, nil, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	gd, err := jpeg.Decode(bytes.NewReader(r.data))
	if err != nil {
		t.Fatal(err)
	}
	if psnr(gd, r.recon) < 40 {
		t.Error("gris: reconstrucción distinta")
	}
	if dir := os.Getenv("TURBO_TESTDIR"); dir != "" {
		os.WriteFile(filepath.Join(dir, "enc_q75.jpg"), r.data, 0o644)
		var b2 bytes.Buffer
		png.Encode(&b2, src)
		os.WriteFile(filepath.Join(dir, "src.png"), b2.Bytes(), 0o644)
	}
}
