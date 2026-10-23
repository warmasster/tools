package docs

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/adler32"
	"hash/crc32"
	"image"
	"image/color"
	"runtime"
	"sync"

	"github.com/klauspost/compress/flate"
	"github.com/klauspost/compress/zlib"
)

// Codificador PNG propio: elige el tipo de color más pequeño sin pérdida,
// filtra las filas en paralelo y comprime por trozos en paralelo (como pigz:
// cada trozo usa los 32 KB anteriores como diccionario).

// pngImage es una imagen lista para PNG.
type pngImage struct {
	w, h     int
	colorTyp byte // 0 gris, 2 RGB, 3 paleta, 4 gris+alfa, 6 RGBA
	depth    byte
	rows     func(y int, dst []byte) // escribe la fila y (sin byte de filtro)
	rowBytes int
	palette  []color.NRGBA
}

// pngPaletted prepara una imagen de paleta (índices de 8 bits).
func pngPaletted(idx []uint8, w, h int, pal []color.NRGBA) *pngImage {
	depth := byte(8)
	switch {
	case len(pal) <= 2:
		depth = 1
	case len(pal) <= 4:
		depth = 2
	case len(pal) <= 16:
		depth = 4
	}
	p := &pngImage{w: w, h: h, colorTyp: 3, depth: depth, palette: pal}
	p.rowBytes = (w*int(depth) + 7) / 8
	ppb := 8 / int(depth)
	p.rows = func(y int, dst []byte) {
		src := idx[y*w : (y+1)*w]
		if depth == 8 {
			copy(dst, src)
			return
		}
		clear(dst)
		for x, v := range src {
			shift := uint(8 - int(depth)*(x%ppb+1))
			dst[x/ppb] |= v << shift
		}
	}
	return p
}

// pngFromImage prepara una imagen Gray o NRGBA, eligiendo el tipo más pequeño
// sin pérdida (quita el alfa si es opaca y usa gris si no hay color).
func pngFromImage(img image.Image) *pngImage {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	switch m := img.(type) {
	case *image.Gray:
		m = compactGray(m)
		return &pngImage{w: w, h: h, colorTyp: 0, depth: 8, rowBytes: w,
			rows: func(y int, dst []byte) { copy(dst, m.Pix[y*m.Stride:y*m.Stride+w]) }}
	case *image.NRGBA:
		m = compactNRGBA(m)
		opaque, gray := analyzeNRGBA(m)
		switch {
		case gray && opaque:
			return &pngImage{w: w, h: h, colorTyp: 0, depth: 8, rowBytes: w, rows: func(y int, dst []byte) {
				s := m.Pix[y*m.Stride:]
				for x := range w {
					dst[x] = s[x*4]
				}
			}}
		case gray:
			return &pngImage{w: w, h: h, colorTyp: 4, depth: 8, rowBytes: w * 2, rows: func(y int, dst []byte) {
				s := m.Pix[y*m.Stride:]
				for x := range w {
					dst[x*2], dst[x*2+1] = s[x*4], s[x*4+3]
				}
			}}
		case opaque:
			return &pngImage{w: w, h: h, colorTyp: 2, depth: 8, rowBytes: w * 3, rows: func(y int, dst []byte) {
				s := m.Pix[y*m.Stride:]
				for x := range w {
					dst[x*3], dst[x*3+1], dst[x*3+2] = s[x*4], s[x*4+1], s[x*4+2]
				}
			}}
		default:
			return &pngImage{w: w, h: h, colorTyp: 6, depth: 8, rowBytes: w * 4, rows: func(y int, dst []byte) {
				copy(dst, m.Pix[y*m.Stride:y*m.Stride+w*4])
			}}
		}
	}
	return pngFromImage(toNRGBA(img))
}

// analyzeNRGBA dice si la imagen es opaca y si es toda gris.
func analyzeNRGBA(m *image.NRGBA) (opaque, gray bool) {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	var mu sync.Mutex
	opaque, gray = true, true
	parallel(h, func(lo, hi int) {
		op, gr := true, true
		for y := lo; y < hi && (op || gr); y++ {
			s := m.Pix[y*m.Stride : y*m.Stride+w*4]
			for i := 0; i < len(s); i += 4 {
				if s[i+3] != 255 {
					op = false
				}
				if s[i] != s[i+1] || s[i+1] != s[i+2] {
					gr = false
				}
			}
		}
		mu.Lock()
		opaque = opaque && op
		gray = gray && gr
		mu.Unlock()
	})
	return
}

func (p *pngImage) bpp() int {
	switch p.colorTyp {
	case 0:
		return 1
	case 2:
		return 3
	case 4:
		return 2
	case 6:
		return 4
	}
	return 1
}

// filtered devuelve las filas con su byte de filtro, comprimidas en zlib.
func (p *pngImage) idat(ctx context.Context, level int) ([]byte, error) {
	stride := p.rowBytes + 1
	raw := make([]byte, stride*p.h)
	adaptive := p.colorTyp != 3 && p.depth == 8
	bpp := p.bpp()
	// Filtrado en paralelo por bloques de filas.
	parallel(p.h, func(lo, hi int) {
		prev := make([]byte, p.rowBytes)
		cur := make([]byte, p.rowBytes)
		if lo > 0 {
			p.rows(lo-1, prev)
		}
		var cand [5][]byte
		if adaptive {
			for i := range cand {
				cand[i] = make([]byte, p.rowBytes)
			}
		}
		for y := lo; y < hi; y++ {
			p.rows(y, cur)
			dst := raw[y*stride : (y+1)*stride]
			if !adaptive {
				dst[0] = 0
				copy(dst[1:], cur)
			} else {
				f := filterRow(cur, prev, y == 0, bpp, &cand)
				dst[0] = byte(f)
				copy(dst[1:], cand[f])
			}
			prev, cur = cur, prev
		}
	})
	if ctx != nil && ctx.Err() != nil {
		return nil, context.Canceled
	}
	return parallelZlib(ctx, raw, level)
}

// encode escribe el PNG completo. level: nivel de compresión flate (1–9).
func (p *pngImage) encode(ctx context.Context, level int, icc []byte) ([]byte, error) {
	z, err := p.idat(ctx, level)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.Grow(len(z) + 1024 + len(icc))
	out.Write([]byte("\x89PNG\r\n\x1a\n"))
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], uint32(p.w))
	binary.BigEndian.PutUint32(ihdr[4:], uint32(p.h))
	ihdr[8], ihdr[9] = p.depth, p.colorTyp
	writeChunk(&out, "IHDR", ihdr)
	if len(icc) > 0 {
		var zb bytes.Buffer
		zw, _ := zlib.NewWriterLevel(&zb, 9)
		zw.Write(icc)
		zw.Close()
		writeChunk(&out, "iCCP", append([]byte("ICC\x00\x00"), zb.Bytes()...))
	}
	if p.colorTyp == 3 {
		plte := make([]byte, 0, len(p.palette)*3)
		trns := make([]byte, 0, len(p.palette))
		lastT := -1
		for i, c := range p.palette {
			plte = append(plte, c.R, c.G, c.B)
			trns = append(trns, c.A)
			if c.A != 255 {
				lastT = i
			}
		}
		writeChunk(&out, "PLTE", plte)
		if lastT >= 0 {
			writeChunk(&out, "tRNS", trns[:lastT+1])
		}
	}
	const maxIDAT = 1 << 20
	for len(z) > 0 {
		n := min(len(z), maxIDAT)
		writeChunk(&out, "IDAT", z[:n])
		z = z[n:]
	}
	writeChunk(&out, "IEND", nil)
	return out.Bytes(), nil
}

func writeChunk(w *bytes.Buffer, typ string, data []byte) {
	var hdr [8]byte
	binary.BigEndian.PutUint32(hdr[:4], uint32(len(data)))
	copy(hdr[4:], typ)
	w.Write(hdr[:])
	w.Write(data)
	c := crc32.NewIEEE()
	c.Write(hdr[4:])
	c.Write(data)
	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], c.Sum32())
	w.Write(crc[:])
}

// filterRow aplica los 5 filtros PNG y devuelve el de menor suma absoluta.
func filterRow(cur, prev []byte, first bool, bpp int, cand *[5][]byte) int {
	n := len(cur)
	c0, c1, c2, c3, c4 := cand[0], cand[1], cand[2], cand[3], cand[4]
	copy(c0, cur)
	for i := range n {
		var a, b, c byte
		if i >= bpp {
			a = cur[i-bpp]
		}
		if !first {
			b = prev[i]
			if i >= bpp {
				c = prev[i-bpp]
			}
		}
		x := cur[i]
		c1[i] = x - a
		c2[i] = x - b
		c3[i] = x - byte((int(a)+int(b))/2)
		c4[i] = x - paeth(a, b, c)
	}
	best, bestSum := 0, -1
	for f := range 5 {
		s := 0
		for _, v := range cand[f] {
			if v < 128 {
				s += int(v)
			} else {
				s += 256 - int(v)
			}
		}
		if bestSum < 0 || s < bestSum {
			best, bestSum = f, s
		}
	}
	return best
}

func paeth(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa, pb, pc := abs(p-int(a)), abs(p-int(b)), abs(p-int(c))
	if pa <= pb && pa <= pc {
		return a
	}
	if pb <= pc {
		return b
	}
	return c
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// parallelZlib comprime data en formato zlib usando varios hilos.
func parallelZlib(ctx context.Context, data []byte, level int) ([]byte, error) {
	const chunk = 256 << 10
	const dictSize = 32 << 10
	n := (len(data) + chunk - 1) / chunk
	if n == 0 {
		n = 1
	}
	parts := make([][]byte, n)
	errs := make([]error, n)
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			if ctx != nil && ctx.Err() != nil {
				errs[i] = context.Canceled
				return
			}
			lo := i * chunk
			hi := min(lo+chunk, len(data))
			var dict []byte
			if lo > 0 {
				dict = data[max(0, lo-dictSize):lo]
			}
			var buf bytes.Buffer
			buf.Grow((hi - lo) / 2)
			fw, err := flate.NewWriterDict(&buf, level, dict)
			if err != nil {
				errs[i] = err
				return
			}
			fw.Write(data[lo:hi])
			if i == n-1 {
				errs[i] = fw.Close()
			} else {
				errs[i] = fw.Flush()
			}
			parts[i] = buf.Bytes()
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	size := 6
	for _, p := range parts {
		size += len(p)
	}
	out := make([]byte, 0, size)
	out = append(out, 0x78, 0xDA)
	for _, p := range parts {
		out = append(out, p...)
	}
	out = binary.BigEndian.AppendUint32(out, adler32.Checksum(data))
	return out, nil
}
