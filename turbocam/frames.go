package main

// Imagen en el PC. Todo se hace en YCbCr 4:2:0, que es lo que sale del JPEG del
// móvil y lo que lee la cámara (NV12): no se pasa nunca por RGB salvo para la
// captura de pantalla. Lo normal es que el móvil mande justo el tamaño de salida y
// aquí solo se copie.

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"math"
)

const (
	blackY = 16
	blackC = 128
)

func newYUV(w, h int) *image.YCbCr {
	return image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
}

// decodeJPEG devuelve el fotograma como 4:2:0 con origen en (0, 0) y lados pares.
func decodeJPEG(b []byte) (*image.YCbCr, error) {
	img, err := jpeg.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	r := img.Bounds()
	w, h := r.Dx()&^1, r.Dy()&^1
	if w < 16 || h < 16 || w > 4096 || h > 4096 {
		return nil, errors.New("tamaño de imagen no válido")
	}
	if m, ok := img.(*image.YCbCr); ok && m.SubsampleRatio == image.YCbCrSubsampleRatio420 && r.Min == (image.Point{}) {
		if m.Rect.Dx() != w || m.Rect.Dy() != h {
			m.Rect = image.Rect(0, 0, w, h)
		}
		return m, nil
	}
	return toYUV420(img, w, h), nil
}

// toYUV420 convierte cualquier imagen (4:4:4, 4:2:2, gris…) a 4:2:0.
func toYUV420(img image.Image, w, h int) *image.YCbCr {
	out := newYUV(w, h)
	r := img.Bounds()
	switch m := img.(type) {
	case *image.Gray:
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				out.Y[y*out.YStride+x] = m.Pix[m.PixOffset(r.Min.X+x, r.Min.Y+y)]
			}
		}
		fillPlane(out.Cb, out.CStride, 0, 0, w/2, h/2, blackC)
		fillPlane(out.Cr, out.CStride, 0, 0, w/2, h/2, blackC)
		return out
	case *image.YCbCr:
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				out.Y[y*out.YStride+x] = m.Y[m.YOffset(r.Min.X+x, r.Min.Y+y)]
			}
		}
		for y := 0; y < h/2; y++ {
			for x := 0; x < w/2; x++ {
				var cb, cr int
				for k := 0; k < 4; k++ {
					o := m.COffset(r.Min.X+2*x+k&1, r.Min.Y+2*y+k>>1)
					cb += int(m.Cb[o])
					cr += int(m.Cr[o])
				}
				out.Cb[y*out.CStride+x] = uint8((cb + 2) / 4)
				out.Cr[y*out.CStride+x] = uint8((cr + 2) / 4)
			}
		}
		return out
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.YCbCrModel.Convert(img.At(r.Min.X+x, r.Min.Y+y)).(color.YCbCr)
			out.Y[y*out.YStride+x] = c.Y
			if x&1 == 0 && y&1 == 0 {
				out.Cb[(y/2)*out.CStride+x/2] = c.Cb
				out.Cr[(y/2)*out.CStride+x/2] = c.Cr
			}
		}
	}
	return out
}

func fillPlane(p []byte, stride, x0, y0, w, h int, v byte) {
	for y := y0; y < y0+h; y++ {
		row := p[y*stride+x0 : y*stride+x0+w]
		for i := range row {
			row[i] = v
		}
	}
}

// fillBlack pinta de negro un rectángulo (coordenadas pares).
func fillBlack(m *image.YCbCr, r image.Rectangle) {
	r = r.Intersect(m.Rect)
	if r.Empty() {
		return
	}
	fillPlane(m.Y, m.YStride, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), blackY)
	fillPlane(m.Cb, m.CStride, r.Min.X/2, r.Min.Y/2, r.Dx()/2, r.Dy()/2, blackC)
	fillPlane(m.Cr, m.CStride, r.Min.X/2, r.Min.Y/2, r.Dx()/2, r.Dy()/2, blackC)
}

// plane es un plano de un 4:2:0: sus bytes, su ancho de línea y su tamaño.
type plane struct {
	pix    []byte
	stride int
	w, h   int // tamaño total del plano (para no salirse al interpolar)
}

// scalePlane lleva la zona (sx, sy, sw, sh) de src a la zona (dx, dy, dw, dh) de dst
// con interpolación bilineal en coma fija. inset(y) dice cuántos píxeles saltarse por
// cada lado en la fila y del destino (para las esquinas redondeadas); puede ser nil.
func scalePlane(dst plane, dx, dy, dw, dh int, src plane, sx, sy, sw, sh int, inset func(y int) int) {
	if dw <= 0 || dh <= 0 || sw <= 0 || sh <= 0 {
		return
	}
	// Mismo tamaño y sin esquinas: copia directa.
	if dw == sw && dh == sh && inset == nil {
		for y := 0; y < dh; y++ {
			copy(dst.pix[(dy+y)*dst.stride+dx:(dy+y)*dst.stride+dx+dw], src.pix[(sy+y)*src.stride+sx:])
		}
		return
	}
	xs := make([]int32, dw)
	xf := make([]int32, dw)
	for x := 0; x < dw; x++ {
		fx := (int64(2*x+1)*int64(sw)*65536)/int64(2*dw) - 32768 + int64(sx)*65536
		if fx < 0 {
			fx = 0
		}
		ix := int(fx >> 16)
		if ix >= src.w-1 {
			ix = src.w - 1
			fx = int64(ix) << 16
		}
		xs[x], xf[x] = int32(ix), int32(fx&0xffff)
	}
	for y := 0; y < dh; y++ {
		fy := (int64(2*y+1)*int64(sh)*65536)/int64(2*dh) - 32768 + int64(sy)*65536
		if fy < 0 {
			fy = 0
		}
		iy, wy := int(fy>>16), int32(fy&0xffff)
		if iy >= src.h-1 {
			iy, wy = src.h-1, 0
		}
		r0 := src.pix[iy*src.stride:]
		r1 := r0
		if iy+1 < src.h {
			r1 = src.pix[(iy+1)*src.stride:]
		}
		x0, x1 := 0, dw
		if inset != nil {
			k := inset(y)
			x0, x1 = k, dw-k
		}
		out := dst.pix[(dy+y)*dst.stride+dx:]
		wy8 := wy >> 8
		for x := x0; x < x1; x++ {
			ix, wx := xs[x], xf[x]
			nx := ix + 1
			if int(nx) >= src.w {
				nx = ix
			}
			a, b := int32(r0[ix]), int32(r0[nx])
			c, d := int32(r1[ix]), int32(r1[nx])
			top := a<<8 + ((b-a)*wx)>>8
			bot := c<<8 + ((d-c)*wx)>>8
			out[x] = uint8((top<<8 + (bot-top)*wy8 + 32768) >> 16)
		}
	}
}

func planes(m *image.YCbCr) (y, cb, cr plane) {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	return plane{m.Y, m.YStride, w, h}, plane{m.Cb, m.CStride, w / 2, h / 2}, plane{m.Cr, m.CStride, w / 2, h / 2}
}

// fitRects calcula qué parte del origen va a qué parte del destino.
// cover recorta lo que sobra; contain deja bandas negras.
func fitRects(sw, sh int, dst image.Rectangle, cover bool) (src, out image.Rectangle) {
	dw, dh := dst.Dx(), dst.Dy()
	src, out = image.Rect(0, 0, sw, sh), dst
	if int64(sw)*int64(dh) == int64(dw)*int64(sh) {
		return
	}
	wider := int64(sw)*int64(dh) > int64(dw)*int64(sh)
	if cover {
		if wider {
			cw := int(int64(sh)*int64(dw)/int64(dh)) &^ 1
			cx := ((sw - cw) / 2) &^ 1
			src = image.Rect(cx, 0, cx+cw, sh)
		} else {
			ch := int(int64(sw)*int64(dh)/int64(dw)) &^ 1
			cy := ((sh - ch) / 2) &^ 1
			src = image.Rect(0, cy, sw, cy+ch)
		}
		return
	}
	if wider {
		oh := int(int64(dw)*int64(sh)/int64(sw)) &^ 1
		oy := dst.Min.Y + ((dh-oh)/2)&^1
		out = image.Rect(dst.Min.X, oy, dst.Max.X, oy+oh)
	} else {
		ow := int(int64(dh)*int64(sw)/int64(sh)) &^ 1
		ox := dst.Min.X + ((dw-ow)/2)&^1
		out = image.Rect(ox, dst.Min.Y, ox+ow, dst.Max.Y)
	}
	return
}

// drawScaled pinta src dentro de r de dst. radius > 0 redondea las esquinas.
func drawScaled(dst *image.YCbCr, r image.Rectangle, src *image.YCbCr, cover bool, radius int) {
	if src == nil || r.Empty() {
		return
	}
	sr, out := fitRects(src.Rect.Dx(), src.Rect.Dy(), r, cover)
	if !cover && out != r {
		fillBlack(dst, r)
	}
	dy, dcb, dcr := planes(dst)
	sy, scb, scr := planes(src)
	var inY, inC func(int) int
	if radius > 0 {
		h := out.Dy()
		inY = func(y int) int { return cornerInset(y, h, radius) }
		inC = func(y int) int { return cornerInset(2*y, h, radius) / 2 }
	}
	scalePlane(dy, out.Min.X, out.Min.Y, out.Dx(), out.Dy(), sy, sr.Min.X, sr.Min.Y, sr.Dx(), sr.Dy(), inY)
	cx, cy, cw, ch := out.Min.X/2, out.Min.Y/2, out.Dx()/2, out.Dy()/2
	scx, scy, scw, sch := sr.Min.X/2, sr.Min.Y/2, sr.Dx()/2, sr.Dy()/2
	scalePlane(dcb, cx, cy, cw, ch, scb, scx, scy, scw, sch, inC)
	scalePlane(dcr, cx, cy, cw, ch, scr, scx, scy, scw, sch, inC)
}

// cornerInset: cuántos píxeles quitar por cada lado en la fila y de un rectángulo de
// alto h con esquinas de radio r.
func cornerInset(y, h, r int) int {
	d := -1
	if y < r {
		d = r - y
	} else if y >= h-r {
		d = y - (h - r - 1)
	}
	if d <= 0 {
		return 0
	}
	return r - int(math.Sqrt(float64(r*r-d*d)))
}

// writeNV12 copia un 4:2:0 compacto al formato de la cámara.
func writeNV12(dst []byte, m *image.YCbCr) {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	for y := 0; y < h; y++ {
		copy(dst[y*w:y*w+w], m.Y[y*m.YStride:])
	}
	uv := dst[w*h:]
	for y := 0; y < h/2; y++ {
		row := uv[y*w : y*w+w]
		cb := m.Cb[y*m.CStride : y*m.CStride+w/2]
		cr := m.Cr[y*m.CStride : y*m.CStride+w/2]
		for x := range cb {
			row[2*x] = cb[x]
			row[2*x+1] = cr[x]
		}
	}
}

// bgraToYUV pasa una captura de pantalla (BGRA, de arriba abajo) a 4:2:0 con BT.601.
func bgraToYUV(dst *image.YCbCr, bgra []byte, stride int) {
	w, h := dst.Rect.Dx(), dst.Rect.Dy()
	for y := 0; y < h; y++ {
		row := bgra[y*stride:]
		out := dst.Y[y*dst.YStride:]
		for x := 0; x < w; x++ {
			b, g, r := int32(row[4*x]), int32(row[4*x+1]), int32(row[4*x+2])
			out[x] = uint8(((66*r + 129*g + 25*b + 128) >> 8) + 16)
		}
	}
	for y := 0; y < h/2; y++ {
		r0, r1 := bgra[2*y*stride:], bgra[(2*y+1)*stride:]
		cb, cr := dst.Cb[y*dst.CStride:], dst.Cr[y*dst.CStride:]
		for x := 0; x < w/2; x++ {
			i := 8 * x
			b := (int32(r0[i]) + int32(r0[i+4]) + int32(r1[i]) + int32(r1[i+4])) >> 2
			g := (int32(r0[i+1]) + int32(r0[i+5]) + int32(r1[i+1]) + int32(r1[i+5])) >> 2
			r := (int32(r0[i+2]) + int32(r0[i+6]) + int32(r1[i+2]) + int32(r1[i+6])) >> 2
			cb[x] = uint8(((-38*r - 74*g + 112*b + 128) >> 8) + 128)
			cr[x] = uint8(((112*r - 94*g - 18*b + 128) >> 8) + 128)
		}
	}
}

func encodeJPEG(m image.Image, quality int) []byte {
	var b bytes.Buffer
	b.Grow(128 << 10)
	if err := jpeg.Encode(&b, m, &jpeg.Options{Quality: quality}); err != nil {
		return nil
	}
	return b.Bytes()
}

// evenSize deja un tamaño en pares y dentro de lo que admite la cámara.
func evenSize(w, h int) (int, int) {
	if w > tcMaxW {
		h = h * tcMaxW / w
		w = tcMaxW
	}
	if h > tcMaxH {
		w = w * tcMaxH / h
		h = tcMaxH
	}
	w, h = w&^1, h&^1
	if w < 64 {
		w = 64
	}
	if h < 64 {
		h = 64
	}
	return w, h
}
