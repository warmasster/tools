package docs

import (
	"image"
	"math"
	"runtime"
	"sync"
)

// parallel reparte [0,n) en tramos y llama a fn(lo, hi) en varios hilos.
func parallel(n int, fn func(lo, hi int)) {
	if n <= 0 {
		return
	}
	w := runtime.GOMAXPROCS(0)
	chunks := w * 4
	if chunks > n {
		chunks = n
	}
	if chunks <= 1 {
		fn(0, n)
		return
	}
	var wg sync.WaitGroup
	step := (n + chunks - 1) / chunks
	sem := make(chan struct{}, w)
	for lo := 0; lo < n; lo += step {
		hi := min(lo+step, n)
		wg.Add(1)
		sem <- struct{}{}
		go func(lo, hi int) {
			defer wg.Done()
			fn(lo, hi)
			<-sem
		}(lo, hi)
	}
	wg.Wait()
}

// Remuestreo separable con núcleo Catmull-Rom, en coma fija y en paralelo.

const wBits = 14

type contrib struct {
	start   int
	weights []int32
}

func catmullRom(x float64) float64 {
	x = math.Abs(x)
	switch {
	case x < 1:
		return (1.5*x-2.5)*x*x + 1
	case x < 2:
		return ((-0.5*x+2.5)*x-4)*x + 2
	}
	return 0
}

// makeContribs calcula los pesos de cada muestra de salida.
func makeContribs(in, out int) []contrib {
	scale := float64(in) / float64(out)
	fscale := math.Max(scale, 1)
	support := 2 * fscale
	cs := make([]contrib, out)
	tmp := make([]float64, 0, int(2*support)+3)
	for i := range out {
		center := (float64(i)+0.5)*scale - 0.5
		lo := int(math.Floor(center - support + 1))
		hi := int(math.Floor(center + support))
		lo = max(lo, 0)
		hi = min(hi, in-1)
		tmp = tmp[:0]
		sum := 0.0
		for j := lo; j <= hi; j++ {
			w := catmullRom((float64(j) - center) / fscale)
			tmp = append(tmp, w)
			sum += w
		}
		ws := make([]int32, len(tmp))
		var isum int32
		best := 0
		for k, w := range tmp {
			ws[k] = int32(math.Round(w / sum * (1 << wBits)))
			isum += ws[k]
			if ws[k] > ws[best] {
				best = k
			}
		}
		ws[best] += (1 << wBits) - isum
		cs[i] = contrib{start: lo, weights: ws}
	}
	return cs
}

func clamp8(v int32) uint8 {
	v = (v + (1 << (wBits - 1))) >> wBits
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// resizePlane remuestrea un plano de 1 o 4 canales (bpp) a ow×oh.
func resizePlane(src []uint8, sw, sh, sstride, bpp, ow, oh int) []uint8 {
	// Pasada horizontal: sw×sh -> ow×sh.
	var tmp []uint8
	tstride := ow * bpp
	if ow == sw {
		tmp = src
		tstride = sstride
	} else {
		tmp = make([]uint8, tstride*sh)
		cx := makeContribs(sw, ow)
		parallel(sh, func(lo, hi int) {
			for y := lo; y < hi; y++ {
				row := src[y*sstride:]
				dst := tmp[y*tstride:]
				for x, c := range cx {
					if bpp == 1 {
						var s int32
						p := c.start
						for _, w := range c.weights {
							s += int32(row[p]) * w
							p++
						}
						dst[x] = clamp8(s)
						continue
					}
					var s0, s1, s2, s3 int32
					p := c.start * 4
					for _, w := range c.weights {
						s0 += int32(row[p]) * w
						s1 += int32(row[p+1]) * w
						s2 += int32(row[p+2]) * w
						s3 += int32(row[p+3]) * w
						p += 4
					}
					d := dst[x*4 : x*4+4 : x*4+4]
					d[0], d[1], d[2], d[3] = clamp8(s0), clamp8(s1), clamp8(s2), clamp8(s3)
				}
			}
		})
	}
	if oh == sh {
		if ow == sw {
			// Copia (no debería pasar: el llamador lo evita).
			out := make([]uint8, ow*bpp*oh)
			for y := range oh {
				copy(out[y*ow*bpp:(y+1)*ow*bpp], src[y*sstride:])
			}
			return out
		}
		return tmp
	}
	// Pasada vertical: ow×sh -> ow×oh.
	ostride := ow * bpp
	out := make([]uint8, ostride*oh)
	cy := makeContribs(sh, oh)
	parallel(oh, func(lo, hi int) {
		acc := make([]int32, ostride)
		for y := lo; y < hi; y++ {
			c := cy[y]
			clear(acc)
			for k, w := range c.weights {
				row := tmp[(c.start+k)*tstride : (c.start+k)*tstride+ostride]
				for i, v := range row {
					acc[i] += int32(v) * w
				}
			}
			dst := out[y*ostride : (y+1)*ostride]
			for i, v := range acc {
				dst[i] = clamp8(v)
			}
		}
	})
	return out
}

// resizeImage reescala img (YCbCr, Gray o NRGBA) a w×h conservando el tipo.
func resizeImage(img image.Image, w, h int) image.Image {
	b := img.Bounds()
	if b.Dx() == w && b.Dy() == h {
		return img
	}
	switch m := img.(type) {
	case *image.Gray:
		m = compactGray(m)
		return &image.Gray{Pix: resizePlane(m.Pix, b.Dx(), b.Dy(), m.Stride, 1, w, h), Stride: w, Rect: image.Rect(0, 0, w, h)}
	case *image.YCbCr:
		m = compactYCbCr(m)
		out := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
		cw, ch := (w+1)/2, (h+1)/2
		scw, sch := chromaSize(m)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); out.Cb = resizePlane(m.Cb, scw, sch, m.CStride, 1, cw, ch) }()
		go func() { defer wg.Done(); out.Cr = resizePlane(m.Cr, scw, sch, m.CStride, 1, cw, ch) }()
		out.Y = resizePlane(m.Y, b.Dx(), b.Dy(), m.YStride, 1, w, h)
		wg.Wait()
		out.YStride, out.CStride = w, cw
		return out
	case *image.NRGBA:
		m = compactNRGBA(m)
		opaque := isOpaque(m)
		src := m.Pix
		if !opaque {
			src = premultiply(m.Pix)
		}
		pix := resizePlane(src, b.Dx(), b.Dy(), m.Stride, 4, w, h)
		if !opaque {
			unpremultiply(pix)
		}
		return &image.NRGBA{Pix: pix, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}
	}
	return resizeImage(toNRGBA(img), w, h)
}

func chromaSize(m *image.YCbCr) (int, int) {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	switch m.SubsampleRatio {
	case image.YCbCrSubsampleRatio422:
		return (w + 1) / 2, h
	case image.YCbCrSubsampleRatio420:
		return (w + 1) / 2, (h + 1) / 2
	case image.YCbCrSubsampleRatio440:
		return w, (h + 1) / 2
	case image.YCbCrSubsampleRatio411:
		return (w + 3) / 4, h
	case image.YCbCrSubsampleRatio410:
		return (w + 3) / 4, (h + 1) / 2
	}
	return w, h
}

func premultiply(p []uint8) []uint8 {
	out := make([]uint8, len(p))
	parallel(len(p)/4, func(lo, hi int) {
		for i := lo * 4; i < hi*4; i += 4 {
			a := uint32(p[i+3])
			out[i] = uint8((uint32(p[i])*a + 127) / 255)
			out[i+1] = uint8((uint32(p[i+1])*a + 127) / 255)
			out[i+2] = uint8((uint32(p[i+2])*a + 127) / 255)
			out[i+3] = uint8(a)
		}
	})
	return out
}

func unpremultiply(p []uint8) {
	parallel(len(p)/4, func(lo, hi int) {
		for i := lo * 4; i < hi*4; i += 4 {
			a := uint32(p[i+3])
			switch a {
			case 255:
			case 0:
				p[i], p[i+1], p[i+2] = 0, 0, 0
			default:
				p[i] = uint8(min((uint32(p[i])*255+a/2)/a, 255))
				p[i+1] = uint8(min((uint32(p[i+1])*255+a/2)/a, 255))
				p[i+2] = uint8(min((uint32(p[i+2])*255+a/2)/a, 255))
			}
		}
	})
}

// fitSize calcula el tamaño final según lado máximo y escala (%).
func fitSize(w, h, maxSide, scalePct int) (int, int) {
	f := 1.0
	if scalePct > 0 && scalePct < 100 {
		f = float64(scalePct) / 100
	}
	if maxSide > 0 {
		if m := max(w, h); float64(m)*f > float64(maxSide) {
			f = float64(maxSide) / float64(m)
		}
	}
	if f >= 1 {
		return w, h
	}
	nw := max(1, int(math.Round(float64(w)*f)))
	nh := max(1, int(math.Round(float64(h)*f)))
	return nw, nh
}
