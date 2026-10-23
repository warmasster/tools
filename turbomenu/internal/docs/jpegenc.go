package docs

import (
	"context"
	"encoding/binary"
	"image"
	"math"
	"runtime"
	"sync"
)

// Codificador JPEG propio (baseline): reparte la imagen en franjas separadas
// por marcadores RST para hacer la DCT y la codificación en paralelo, y
// calcula tablas Huffman óptimas para la imagen (archivos ~5–10 % menores).

var zigzag = [64]int{
	0, 1, 8, 16, 9, 2, 3, 10, 17, 24, 32, 25, 18, 11, 4, 5,
	12, 19, 26, 33, 40, 48, 41, 34, 27, 20, 13, 6, 7, 14, 21, 28,
	35, 42, 49, 56, 57, 50, 43, 36, 29, 22, 15, 23, 30, 37, 44, 51,
	58, 59, 52, 45, 38, 31, 39, 46, 53, 60, 61, 54, 47, 55, 62, 63,
}

var stdLumaQ = [64]int{
	16, 11, 10, 16, 24, 40, 51, 61,
	12, 12, 14, 19, 26, 58, 60, 55,
	14, 13, 16, 24, 40, 57, 69, 56,
	14, 17, 22, 29, 51, 87, 80, 62,
	18, 22, 37, 56, 68, 109, 103, 77,
	24, 35, 55, 64, 81, 104, 113, 92,
	49, 64, 78, 87, 103, 121, 120, 101,
	72, 92, 95, 98, 112, 100, 103, 99,
}

var stdChromaQ = [64]int{
	17, 18, 24, 47, 99, 99, 99, 99,
	18, 21, 26, 66, 99, 99, 99, 99,
	24, 26, 56, 99, 99, 99, 99, 99,
	47, 66, 99, 99, 99, 99, 99, 99,
	99, 99, 99, 99, 99, 99, 99, 99,
	99, 99, 99, 99, 99, 99, 99, 99,
	99, 99, 99, 99, 99, 99, 99, 99,
	99, 99, 99, 99, 99, 99, 99, 99,
}

var aanScale = [8]float64{1.0, 1.387039845, 1.306562965, 1.175875602, 1.0, 0.785694958, 0.541196100, 0.275899379}

// scaleQuant escala una tabla estándar a una calidad 1–100 (como libjpeg).
func scaleQuant(base *[64]int, q int) (out [64]int) {
	q = min(max(q, 1), 100)
	s := 200 - 2*q
	if q < 50 {
		s = 5000 / q
	}
	for i, v := range base {
		out[i] = min(max((v*s+50)/100, 1), 255)
	}
	return
}

type jpegComp struct {
	h, v  int // muestreo
	table int // 0 luminancia, 1 crominancia
}

type huffTable struct {
	bits [17]uint8
	vals []uint8
	code [256]uint16
	size [256]uint8
}

type jpegEnc struct {
	w, h       int
	comps      []jpegComp
	quant      [2][64]int
	fdiv       [2][64]float32 // multiplicadores de cuantización (orden natural)
	mcuW, mcuH int
	mcusX      int
	mcusY      int
	rowsPer    int // filas de MCU por franja (intervalo de reinicio)
	blocksMCU  int
	src        image.Image
	planesFill func(e *jpegEnc, my int, yp, cbp, crp []uint8, tmp *[2][]int32)
	// resultados por franja
	coefs [][]int16
	freq  [][4][257]int64 // dc0, ac0, dc1, ac1
	huff  [4]huffTable
}

// jpegOut es el resultado de encodeJPEG.
type jpegOut struct {
	data  []byte
	recon image.Image // imagen tal como la verá el usuario (si se pidió)
}

// encodeJPEG codifica img (YCbCr, Gray, NRGBA opaca o RGBA) con calidad q.
// Con reconSide > 0 devuelve también la imagen decodificada, a una escala
// 1/8, 1/4, 1/2 o 1 cuyo lado mayor llegue al menos a reconSide.
func encodeJPEG(ctx context.Context, img image.Image, q int, icc []byte, reconSide int) (*jpegOut, error) {
	b := img.Bounds()
	if b.Dx() > 65535 || b.Dy() > 65535 {
		return nil, uerr(nil, "La imagen es demasiado grande para JPG (máximo 65535 píxeles por lado)")
	}
	e := &jpegEnc{w: b.Dx(), h: b.Dy(), src: img}
	switch m := img.(type) {
	case *image.Gray:
		e.comps = []jpegComp{{1, 1, 0}}
		e.planesFill = fillGray
		_ = m
	case *image.YCbCr:
		e.setColor(q >= 95 && m.SubsampleRatio != image.YCbCrSubsampleRatio420)
		e.planesFill = fillYCbCr
	case *image.NRGBA, *image.RGBA:
		e.setColor(q >= 95)
		e.planesFill = fillRGB
	default:
		e.src = toNRGBA(img)
		e.setColor(q >= 95)
		e.planesFill = fillRGB
	}
	e.quant[0] = scaleQuant(&stdLumaQ, q)
	e.quant[1] = scaleQuant(&stdChromaQ, q)
	for t := range 2 {
		for i := range 64 {
			r, c := i/8, i%8
			e.fdiv[t][i] = float32(1.0 / (float64(e.quant[t][i]) * aanScale[r] * aanScale[c] * 8))
		}
	}
	hmax, vmax := e.comps[0].h, e.comps[0].v
	e.mcuW, e.mcuH = 8*hmax, 8*vmax
	e.mcusX = (e.w + e.mcuW - 1) / e.mcuW
	e.mcusY = (e.h + e.mcuH - 1) / e.mcuH
	for _, c := range e.comps {
		e.blocksMCU += c.h * c.v
	}
	strips := runtime.GOMAXPROCS(0) * 3
	e.rowsPer = max(1, (e.mcusY+strips-1)/strips)
	if e.rowsPer*e.mcusX > 65535 {
		e.rowsPer = max(1, 65535/e.mcusX)
	}
	nStrips := (e.mcusY + e.rowsPer - 1) / e.rowsPer
	e.coefs = make([][]int16, nStrips)
	e.freq = make([][4][257]int64, nStrips)

	// Pasada 1: DCT + cuantización + estadísticas.
	if err := e.forStrips(ctx, nStrips, e.analyzeStrip); err != nil {
		return nil, err
	}
	var total [4][257]int64
	for s := range e.freq {
		for t := range 4 {
			for i := range 257 {
				total[t][i] += e.freq[s][t][i]
			}
		}
	}
	nt := 2
	if len(e.comps) == 3 {
		nt = 4
	}
	for t := range nt {
		e.huff[t] = optimalHuffman(&total[t])
	}
	// Pasada 2: codificación entrópica de cada franja.
	parts := make([][]byte, nStrips)
	if err := e.forStrips(ctx, nStrips, func(s int) { parts[s] = e.encodeStrip(s) }); err != nil {
		return nil, err
	}
	out := &jpegOut{data: e.assemble(parts, icc)}
	if reconSide > 0 {
		n := 1
		for n < 8 && max(e.w, e.h)*n/8 < reconSide {
			n *= 2
		}
		out.recon = e.reconstruct(n)
	}
	return out, nil
}

func (e *jpegEnc) setColor(full bool) {
	if full {
		e.comps = []jpegComp{{1, 1, 0}, {1, 1, 1}, {1, 1, 1}}
	} else {
		e.comps = []jpegComp{{2, 2, 0}, {1, 1, 1}, {1, 1, 1}}
	}
}

// forStrips ejecuta fn para cada franja en paralelo, comprobando la cancelación.
func (e *jpegEnc) forStrips(ctx context.Context, n int, fn func(s int)) error {
	var wg sync.WaitGroup
	next := make(chan int, n)
	for s := range n {
		next <- s
	}
	close(next)
	workers := min(runtime.GOMAXPROCS(0), n)
	canceled := false
	var mu sync.Mutex
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s := range next {
				if ctx != nil && ctx.Err() != nil {
					mu.Lock()
					canceled = true
					mu.Unlock()
					return
				}
				fn(s)
			}
		}()
	}
	wg.Wait()
	if canceled || (ctx != nil && ctx.Err() != nil) {
		return context.Canceled
	}
	return nil
}

func (e *jpegEnc) stripRows(s int) (int, int) {
	lo := s * e.rowsPer
	return lo, min(lo+e.rowsPer, e.mcusY)
}

// Rellenado de planos: una fila de MCU en Y (mcuH filas) y Cb/Cr (8 filas).

func (e *jpegEnc) planeDims() (pw, cw, ch int) {
	pw = e.mcusX * e.mcuW
	if len(e.comps) == 1 {
		return pw, 0, 0
	}
	if e.comps[0].h == 2 {
		return pw, pw / 2, e.mcuH / 2
	}
	return pw, pw, e.mcuH
}

func fillGray(e *jpegEnc, my int, yp, _, _ []uint8, tmp *[2][]int32) {
	m := e.src.(*image.Gray)
	pw, _, _ := e.planeDims()
	for r := range e.mcuH {
		y := min(my*e.mcuH+r, e.h-1)
		src := m.Pix[m.PixOffset(m.Rect.Min.X, m.Rect.Min.Y+y):]
		dst := yp[r*pw : (r+1)*pw]
		copy(dst, src[:e.w])
		last := src[e.w-1]
		for x := e.w; x < pw; x++ {
			dst[x] = last
		}
	}
}

func fillYCbCr(e *jpegEnc, my int, yp, cbp, crp []uint8, tmp *[2][]int32) {
	m := e.src.(*image.YCbCr)
	pw, cw, ch := e.planeDims()
	bx, by := m.Rect.Min.X, m.Rect.Min.Y
	for r := range e.mcuH {
		y := min(my*e.mcuH+r, e.h-1)
		src := m.Y[m.YOffset(bx, by+y):]
		dst := yp[r*pw : (r+1)*pw]
		copy(dst, src[:e.w])
		last := src[e.w-1]
		for x := e.w; x < pw; x++ {
			dst[x] = last
		}
	}
	sub := e.comps[0].h == 2
	if sub && m.SubsampleRatio == image.YCbCrSubsampleRatio420 && m.Rect.Min == (image.Point{}) {
		scw, sch := chromaSize(m)
		for r := range ch {
			cy := min(my*ch+r, sch-1)
			for i, pl := range [2][]uint8{m.Cb, m.Cr} {
				src := pl[cy*m.CStride:]
				dst := cbp
				if i == 1 {
					dst = crp
				}
				dst = dst[r*cw : (r+1)*cw]
				copy(dst, src[:scw])
				last := src[scw-1]
				for x := scw; x < cw; x++ {
					dst[x] = last
				}
			}
		}
		return
	}
	if !sub && m.SubsampleRatio == image.YCbCrSubsampleRatio444 && m.Rect.Min == (image.Point{}) {
		for r := range ch {
			y := min(my*ch+r, e.h-1)
			for i, pl := range [2][]uint8{m.Cb, m.Cr} {
				src := pl[y*m.CStride:]
				dst := cbp
				if i == 1 {
					dst = crp
				}
				dst = dst[r*cw : (r+1)*cw]
				copy(dst, src[:e.w])
				last := src[e.w-1]
				for x := e.w; x < cw; x++ {
					dst[x] = last
				}
			}
		}
		return
	}
	// Caso general: se toma la crominancia de cada píxel y se promedia.
	f := 1
	if sub {
		f = 2
	}
	for r := range ch {
		for cx := range cw {
			var sb, sr, n int
			for dy := range f {
				y := min((my*ch+r)*f+dy, e.h-1)
				for dx := range f {
					x := min(cx*f+dx, e.w-1)
					ci := m.COffset(bx+x, by+y)
					sb += int(m.Cb[ci])
					sr += int(m.Cr[ci])
					n++
				}
			}
			cbp[r*cw+cx] = uint8((sb + n/2) / n)
			crp[r*cw+cx] = uint8((sr + n/2) / n)
		}
	}
}

func fillRGB(e *jpegEnc, my int, yp, cbp, crp []uint8, tmp *[2][]int32) {
	var pix []uint8
	var stride int
	var min0 image.Point
	switch m := e.src.(type) {
	case *image.NRGBA:
		pix, stride, min0 = m.Pix, m.Stride, m.Rect.Min
	case *image.RGBA:
		pix, stride, min0 = m.Pix, m.Stride, m.Rect.Min
	}
	pw, cw, ch := e.planeDims()
	sub := e.comps[0].h == 2
	// Crominancia a resolución completa en int32 para promediar con precisión.
	var cbFull, crFull []int32
	if sub {
		if len(tmp[0]) < pw*e.mcuH {
			tmp[0] = make([]int32, pw*e.mcuH)
			tmp[1] = make([]int32, pw*e.mcuH)
		}
		cbFull, crFull = tmp[0], tmp[1]
	}
	for r := range e.mcuH {
		y := min(my*e.mcuH+r, e.h-1)
		row := pix[(min0.Y+y)*stride+min0.X*4:]
		for x := range pw {
			sx := min(x, e.w-1) * 4
			R, G, B := int32(row[sx]), int32(row[sx+1]), int32(row[sx+2])
			yy := (19595*R + 38470*G + 7471*B + 1<<15) >> 16
			cb := -11056*R - 21712*G + 32768*B
			cr := 32768*R - 27440*G - 5328*B
			yp[r*pw+x] = uint8(yy)
			if sub {
				cbFull[r*pw+x] = cb
				crFull[r*pw+x] = cr
			} else {
				cbp[r*cw+x] = clampC((cb + 257<<15) >> 16)
				crp[r*cw+x] = clampC((cr + 257<<15) >> 16)
			}
		}
	}
	if sub {
		for r := range ch {
			a := cbFull[(2*r)*pw:]
			b := cbFull[(2*r+1)*pw:]
			c := crFull[(2*r)*pw:]
			d := crFull[(2*r+1)*pw:]
			for cx := range cw {
				x := 2 * cx
				sb := a[x] + a[x+1] + b[x] + b[x+1]
				sr := c[x] + c[x+1] + d[x] + d[x+1]
				cbp[r*cw+cx] = clampC((sb + 4*(128<<16) + 1<<17) >> 18)
				crp[r*cw+cx] = clampC((sr + 4*(128<<16) + 1<<17) >> 18)
			}
		}
	}
}

func clampC(v int32) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// analyzeStrip calcula y guarda los coeficientes cuantizados de una franja.
func (e *jpegEnc) analyzeStrip(s int) {
	lo, hi := e.stripRows(s)
	pw, cw, ch := e.planeDims()
	yp := make([]uint8, pw*e.mcuH)
	var cbp, crp []uint8
	if len(e.comps) == 3 {
		cbp = make([]uint8, cw*ch)
		crp = make([]uint8, cw*ch)
	}
	coefs := make([]int16, (hi-lo)*e.mcusX*e.blocksMCU*64)
	freq := &e.freq[s]
	var pred [3]int
	var blk [64]float32
	k := 0
	var tmp [2][]int32
	for my := lo; my < hi; my++ {
		e.planesFill(e, my, yp, cbp, crp, &tmp)
		for mx := range e.mcusX {
			for ci, c := range e.comps {
				plane, stride := yp, pw
				if ci == 1 {
					plane, stride = cbp, cw
				} else if ci == 2 {
					plane, stride = crp, cw
				}
				for by := range c.v {
					for bx := range c.h {
						x0 := (mx*c.h + bx) * 8
						y0 := by * 8
						for r := range 8 {
							row := plane[(y0+r)*stride+x0 : (y0+r)*stride+x0+8]
							for i, v := range row {
								blk[r*8+i] = float32(v) - 128
							}
						}
						out := coefs[k*64 : k*64+64]
						fdctQuant(&blk, &e.fdiv[c.table], out)
						t := c.table * 2
						d := int(out[0]) - pred[ci]
						pred[ci] = int(out[0])
						freq[t][bitLen(d)]++
						countAC(out, &freq[t+1])
						k++
					}
				}
			}
		}
	}
	e.coefs[s] = coefs
}

func bitLen(v int) int {
	if v < 0 {
		v = -v
	}
	n := 0
	for v > 0 {
		n++
		v >>= 1
	}
	return n
}

func countAC(c []int16, f *[257]int64) {
	run := 0
	for i := 1; i < 64; i++ {
		v := int(c[i])
		if v == 0 {
			run++
			continue
		}
		for run > 15 {
			f[0xF0]++
			run -= 16
		}
		f[run<<4|bitLen(v)]++
		run = 0
	}
	if run > 0 {
		f[0]++
	}
}

// fdctQuant: DCT flotante AAN (como jfdctflt de libjpeg) + cuantización.
// La salida queda en orden zigzag.
func fdctQuant(d *[64]float32, div *[64]float32, out []int16) {
	for r := 0; r < 64; r += 8 {
		p := d[r : r+8 : r+8]
		tmp0 := p[0] + p[7]
		tmp7 := p[0] - p[7]
		tmp1 := p[1] + p[6]
		tmp6 := p[1] - p[6]
		tmp2 := p[2] + p[5]
		tmp5 := p[2] - p[5]
		tmp3 := p[3] + p[4]
		tmp4 := p[3] - p[4]
		tmp10 := tmp0 + tmp3
		tmp13 := tmp0 - tmp3
		tmp11 := tmp1 + tmp2
		tmp12 := tmp1 - tmp2
		p[0] = tmp10 + tmp11
		p[4] = tmp10 - tmp11
		z1 := (tmp12 + tmp13) * 0.707106781
		p[2] = tmp13 + z1
		p[6] = tmp13 - z1
		tmp10 = tmp4 + tmp5
		tmp11 = tmp5 + tmp6
		tmp12 = tmp6 + tmp7
		z5 := (tmp10 - tmp12) * 0.382683433
		z2 := 0.541196100*tmp10 + z5
		z4 := 1.306562965*tmp12 + z5
		z3 := tmp11 * 0.707106781
		z11 := tmp7 + z3
		z13 := tmp7 - z3
		p[5] = z13 + z2
		p[3] = z13 - z2
		p[1] = z11 + z4
		p[7] = z11 - z4
	}
	for c := range 8 {
		tmp0 := d[c] + d[56+c]
		tmp7 := d[c] - d[56+c]
		tmp1 := d[8+c] + d[48+c]
		tmp6 := d[8+c] - d[48+c]
		tmp2 := d[16+c] + d[40+c]
		tmp5 := d[16+c] - d[40+c]
		tmp3 := d[24+c] + d[32+c]
		tmp4 := d[24+c] - d[32+c]
		tmp10 := tmp0 + tmp3
		tmp13 := tmp0 - tmp3
		tmp11 := tmp1 + tmp2
		tmp12 := tmp1 - tmp2
		d[c] = tmp10 + tmp11
		d[32+c] = tmp10 - tmp11
		z1 := (tmp12 + tmp13) * 0.707106781
		d[16+c] = tmp13 + z1
		d[48+c] = tmp13 - z1
		tmp10 = tmp4 + tmp5
		tmp11 = tmp5 + tmp6
		tmp12 = tmp6 + tmp7
		z5 := (tmp10 - tmp12) * 0.382683433
		z2 := 0.541196100*tmp10 + z5
		z4 := 1.306562965*tmp12 + z5
		z3 := tmp11 * 0.707106781
		z11 := tmp7 + z3
		z13 := tmp7 - z3
		d[40+c] = z13 + z2
		d[24+c] = z13 - z2
		d[8+c] = z11 + z4
		d[56+c] = z11 - z4
	}
	for k := range 64 {
		n := zigzag[k]
		v := d[n] * div[n]
		out[k] = int16(int32(v+16384.5) - 16384)
	}
}

// optimalHuffman genera una tabla Huffman limitada a 16 bits (libjpeg, K.2).
func optimalHuffman(f *[257]int64) huffTable {
	var freq [257]int64
	copy(freq[:], f[:])
	any := false
	for i := range 256 {
		if freq[i] > 0 {
			any = true
			break
		}
	}
	if !any {
		freq[0] = 1
	}
	freq[256] = 1
	var codesize [257]int
	var others [257]int
	for i := range others {
		others[i] = -1
	}
	for {
		c1, c2 := -1, -1
		var v int64 = math.MaxInt64
		for i := range 257 {
			if freq[i] > 0 && freq[i] <= v {
				v = freq[i]
				c1 = i
			}
		}
		v = math.MaxInt64
		for i := range 257 {
			if freq[i] > 0 && freq[i] <= v && i != c1 {
				v = freq[i]
				c2 = i
			}
		}
		if c2 < 0 {
			break
		}
		freq[c1] += freq[c2]
		freq[c2] = 0
		codesize[c1]++
		for others[c1] >= 0 {
			c1 = others[c1]
			codesize[c1]++
		}
		others[c1] = c2
		codesize[c2]++
		for others[c2] >= 0 {
			c2 = others[c2]
			codesize[c2]++
		}
	}
	var bits [33]int
	for i := range 257 {
		if codesize[i] > 0 {
			bits[min(codesize[i], 32)]++
		}
	}
	for i := 32; i > 16; i-- {
		for bits[i] > 0 {
			j := i - 2
			for bits[j] == 0 {
				j--
			}
			bits[i] -= 2
			bits[i-1]++
			bits[j+1] += 2
			bits[j]--
		}
	}
	i := 16
	for bits[i] == 0 {
		i--
	}
	bits[i]--
	var t huffTable
	for l := 1; l <= 16; l++ {
		t.bits[l] = uint8(bits[l])
	}
	for l := 1; l <= 32; l++ {
		for s := range 256 {
			if codesize[s] == l {
				t.vals = append(t.vals, uint8(s))
			}
		}
	}
	// El reajuste de longitudes reparte los símbolos por orden de frecuencia.
	t.vals = t.vals[:sumBits(&t.bits)]
	code := uint16(0)
	k := 0
	for l := 1; l <= 16; l++ {
		for range int(t.bits[l]) {
			s := t.vals[k]
			t.code[s] = code
			t.size[s] = uint8(l)
			code++
			k++
		}
		code <<= 1
	}
	return t
}

func sumBits(b *[17]uint8) int {
	n := 0
	for _, v := range b[1:] {
		n += int(v)
	}
	return n
}

// bitWriter escribe bits con relleno de bytes 0xFF (0xFF 0x00).
type bitWriter struct {
	buf   []byte
	acc   uint64
	nbits uint
}

func (w *bitWriter) put(code uint32, size uint) {
	w.acc = w.acc<<size | uint64(code)&(1<<size-1)
	w.nbits += size
	for w.nbits >= 8 {
		w.nbits -= 8
		b := byte(w.acc >> w.nbits)
		w.buf = append(w.buf, b)
		if b == 0xFF {
			w.buf = append(w.buf, 0)
		}
	}
}

func (w *bitWriter) pad() {
	if w.nbits > 0 {
		w.put(0x7F, 8-w.nbits)
	}
}

func (e *jpegEnc) encodeStrip(s int) []byte {
	coefs := e.coefs[s]
	w := &bitWriter{buf: make([]byte, 0, len(coefs)/6+64)}
	var pred [3]int
	k := 0
	lo, hi := e.stripRows(s)
	for range (hi - lo) * e.mcusX {
		for ci, c := range e.comps {
			dc := &e.huff[c.table*2]
			ac := &e.huff[c.table*2+1]
			for range c.h * c.v {
				blk := coefs[k*64 : k*64+64]
				d := int(blk[0]) - pred[ci]
				pred[ci] = int(blk[0])
				n := bitLen(d)
				w.put(uint32(dc.code[n]), uint(dc.size[n]))
				if n > 0 {
					if d < 0 {
						d--
					}
					w.put(uint32(d), uint(n))
				}
				run := 0
				for i := 1; i < 64; i++ {
					v := int(blk[i])
					if v == 0 {
						run++
						continue
					}
					for run > 15 {
						w.put(uint32(ac.code[0xF0]), uint(ac.size[0xF0]))
						run -= 16
					}
					n := bitLen(v)
					sym := run<<4 | n
					w.put(uint32(ac.code[sym]), uint(ac.size[sym]))
					if v < 0 {
						v--
					}
					w.put(uint32(v), uint(n))
					run = 0
				}
				if run > 0 {
					w.put(uint32(ac.code[0]), uint(ac.size[0]))
				}
				k++
			}
		}
	}
	w.pad()
	return w.buf
}

func (e *jpegEnc) assemble(parts [][]byte, icc []byte) []byte {
	n := 1024 + len(icc)
	for _, p := range parts {
		n += len(p) + 2
	}
	b := make([]byte, 0, n)
	seg := func(marker byte, payload []byte) {
		b = append(b, 0xFF, marker)
		b = binary.BigEndian.AppendUint16(b, uint16(len(payload)+2))
		b = append(b, payload...)
	}
	b = append(b, 0xFF, 0xD8)
	seg(0xE0, []byte{'J', 'F', 'I', 'F', 0, 1, 1, 0, 0, 1, 0, 1, 0, 0})
	// Perfil de color ICC (en trozos de hasta 65519 bytes).
	if len(icc) > 0 {
		const max = 65519 - 14
		cnt := (len(icc) + max - 1) / max
		if cnt <= 255 {
			for i := range cnt {
				part := icc[i*max : min((i+1)*max, len(icc))]
				p := append([]byte("ICC_PROFILE\x00"), byte(i+1), byte(cnt))
				seg(0xE2, append(p, part...))
			}
		}
	}
	nt := 1
	if len(e.comps) == 3 {
		nt = 2
	}
	var dqt []byte
	for t := range nt {
		dqt = append(dqt, byte(t))
		for k := range 64 {
			dqt = append(dqt, byte(e.quant[t][zigzag[k]]))
		}
	}
	seg(0xDB, dqt)
	sof := []byte{8}
	sof = binary.BigEndian.AppendUint16(sof, uint16(e.h))
	sof = binary.BigEndian.AppendUint16(sof, uint16(e.w))
	sof = append(sof, byte(len(e.comps)))
	for i, c := range e.comps {
		sof = append(sof, byte(i+1), byte(c.h<<4|c.v), byte(c.table))
	}
	seg(0xC0, sof)
	var dht []byte
	for t := range nt * 2 {
		class := byte(t % 2) // 0 DC, 1 AC
		id := byte(t / 2)
		h := &e.huff[t]
		dht = append(dht, class<<4|id)
		dht = append(dht, h.bits[1:]...)
		dht = append(dht, h.vals...)
	}
	seg(0xC4, dht)
	seg(0xDD, binary.BigEndian.AppendUint16(nil, uint16(e.rowsPer*e.mcusX)))
	sos := []byte{byte(len(e.comps))}
	for i, c := range e.comps {
		sos = append(sos, byte(i+1), byte(c.table<<4|c.table))
	}
	sos = append(sos, 0, 63, 0)
	seg(0xDA, sos)
	for i, p := range parts {
		if i > 0 {
			b = append(b, 0xFF, byte(0xD0+(i-1)%8))
		}
		b = append(b, p...)
	}
	b = append(b, 0xFF, 0xD9)
	return b
}

// idctMats[n][y][u] = C(u)/2 · cos((2y+1)uπ/2n): IDCT de n puntos con los n
// primeros coeficientes (decodificación a escala n/8, como libjpeg).
var idctMats = func() (m [9][8][8]float32) {
	for _, n := range []int{1, 2, 4, 8} {
		for y := range n {
			for u := range n {
				c := 1.0
				if u == 0 {
					c = 1 / math.Sqrt2
				}
				m[n][y][u] = float32(c / 2 * math.Cos(float64(2*y+1)*float64(u)*math.Pi/float64(2*n)))
			}
		}
	}
	return
}()

// reconstruct decodifica los coeficientes guardados (como haría un visor) a
// escala n/8 (n = 1, 2, 4 u 8).
func (e *jpegEnc) reconstruct(n int) image.Image {
	pw, cw, ch := e.planeDims()
	pw, cw = pw*n/8, cw*n/8
	full := e.mcusY * e.mcuH * n / 8
	yplane := make([]uint8, pw*full)
	var cb, cr []uint8
	if len(e.comps) == 3 {
		chFull := e.mcusY * ch * n / 8
		cb = make([]uint8, cw*chFull)
		cr = make([]uint8, cw*chFull)
	}
	mat := &idctMats[n]
	_ = e.forStrips(nil, len(e.coefs), func(s int) {
		lo, hi := e.stripRows(s)
		coefs := e.coefs[s]
		var deq [64]float32
		var tmp [64]float32
		k := 0
		for my := lo; my < hi; my++ {
			for mx := range e.mcusX {
				for ci, c := range e.comps {
					plane, stride, rowH := yplane, pw, e.mcuH*n/8
					if ci == 1 {
						plane, stride, rowH = cb, cw, ch*n/8
					} else if ci == 2 {
						plane, stride, rowH = cr, cw, ch*n/8
					}
					q := &e.quant[c.table]
					for by := range c.v {
						for bx := range c.h {
							blk := coefs[k*64 : k*64+64]
							k++
							for i := range 64 {
								nn := zigzag[i]
								deq[nn] = float32(int(blk[i]) * q[nn])
							}
							// Columnas (solo las n primeras frecuencias) y luego filas.
							for u := range n {
								for y := range n {
									var s float32
									for v := range n {
										s += mat[y][v] * deq[v*8+u]
									}
									tmp[y*8+u] = s
								}
							}
							x0 := (mx*c.h + bx) * n
							y0 := my*rowH + by*n
							for r := range n {
								row := plane[(y0+r)*stride+x0:]
								for x := range n {
									var s float32
									for u := range n {
										s += mat[x][u] * tmp[r*8+u]
									}
									v := int32(s + 128.5)
									if v < 0 {
										v = 0
									} else if v > 255 {
										v = 255
									}
									row[x] = uint8(v)
								}
							}
						}
					}
				}
			}
		}
	})
	w, h := (e.w*n+7)/8, (e.h*n+7)/8
	if len(e.comps) == 1 {
		return &image.Gray{Pix: yplane, Stride: pw, Rect: image.Rect(0, 0, w, h)}
	}
	ratio := image.YCbCrSubsampleRatio444
	if e.comps[0].h == 2 {
		ratio = image.YCbCrSubsampleRatio420
	}
	return &image.YCbCr{Y: yplane, Cb: cb, Cr: cr, YStride: pw, CStride: cw, SubsampleRatio: ratio, Rect: image.Rect(0, 0, w, h)}
}
