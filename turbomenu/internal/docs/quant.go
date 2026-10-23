package docs

import (
	"image"
	"image/color"
	"math"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
)

// Cuantización de color al estilo pngquant: histograma, corte por la mediana
// guiado por el error, refinado k-means y difuminado Floyd–Steinberg.

// exactPalette devuelve la paleta exacta si la imagen tiene ≤ limit colores.
func exactPalette(m *image.NRGBA, limit int) ([]color.NRGBA, []uint8, bool) {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	index := make(map[uint32]uint8, limit)
	pal := make([]color.NRGBA, 0, limit)
	idx := make([]uint8, w*h)
	var last uint32 = 0xFFFFFFFF
	var lastI uint8
	for y := range h {
		row := m.Pix[y*m.Stride : y*m.Stride+w*4]
		for x := range w {
			p := row[x*4 : x*4+4 : x*4+4]
			k := uint32(p[0])<<24 | uint32(p[1])<<16 | uint32(p[2])<<8 | uint32(p[3])
			if p[3] == 0 {
				k = 0
			}
			if k == last {
				idx[y*w+x] = lastI
				continue
			}
			i, ok := index[k]
			if !ok {
				if len(pal) >= limit {
					return nil, nil, false
				}
				i = uint8(len(pal))
				index[k] = i
				pal = append(pal, color.NRGBA{uint8(k >> 24), uint8(k >> 16), uint8(k >> 8), uint8(k)})
			}
			last, lastI = k, i
			idx[y*w+x] = i
		}
	}
	return sortPalette(pal, idx)
}

// sortPalette pone primero los colores con transparencia (tRNS más corto).
func sortPalette(pal []color.NRGBA, idx []uint8) ([]color.NRGBA, []uint8, bool) {
	order := make([]int, len(pal))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return pal[order[a]].A < pal[order[b]].A })
	remap := make([]uint8, 256)
	np := make([]color.NRGBA, len(pal))
	for newI, oldI := range order {
		np[newI] = pal[oldI]
		remap[oldI] = uint8(newI)
	}
	parallel(len(idx), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			idx[i] = remap[idx[i]]
		}
	})
	return np, idx, true
}

type qColor struct {
	r, g, b, a float32 // color con alfa premultiplicado (0–255)
	w          float32 // peso (nº de píxeles)
}

// Pesos perceptivos de cada canal en la distancia.
const (
	wr = 0.5
	wg = 1.0
	wb = 0.35
	wa = 1.0
)

func qdist(a, b *qColor) float32 {
	dr, dg, db, da := a.r-b.r, a.g-b.g, a.b-b.b, a.a-b.a
	return wr*dr*dr + wg*dg*dg + wb*db*db + wa*da*da
}

// quantize reduce img a como mucho n colores. dither: fuerza 0–1.
func quantize(m *image.NRGBA, n int, dither float32) ([]color.NRGBA, []uint8) {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	hist := buildHistogram(m)
	pal := medianCut(hist, n)
	pal = kmeans(hist, pal, 3)
	// Paleta final en colores normales (no premultiplicados).
	out := make([]color.NRGBA, len(pal))
	for i, c := range pal {
		// Opaco y transparente exactos (si no, todo quedaría "casi opaco").
		if c.a > 247 && c.a < 255 {
			f := 255 / c.a
			c = qColor{min(c.r*f, 255), min(c.g*f, 255), min(c.b*f, 255), 255, c.w}
		} else if c.a < 8 {
			c = qColor{w: c.w}
		}
		out[i] = unpremul(c)
		// Se recalcula desde el color redondeado para que la búsqueda sea exacta.
		pal[i] = premulColor(out[i].R, out[i].G, out[i].B, out[i].A)
	}
	idx := make([]uint8, w*h)
	remap(m, pal, idx, dither)
	pal2, idx2, _ := sortPalette(out, idx)
	return pal2, idx2
}

func premulColor(r, g, b, a uint8) qColor {
	f := float32(a) / 255
	return qColor{float32(r) * f, float32(g) * f, float32(b) * f, float32(a), 1}
}

func unpremul(c qColor) color.NRGBA {
	a := math.Round(float64(c.a))
	if a <= 0 {
		return color.NRGBA{}
	}
	f := 255 / float64(c.a)
	cl := func(v float32) uint8 {
		x := math.Round(float64(v) * f)
		return uint8(min(max(x, 0), 255))
	}
	return color.NRGBA{cl(c.r), cl(c.g), cl(c.b), uint8(min(a, 255))}
}

// buildHistogram agrupa los colores (5 bits por canal) con su color medio.
func buildHistogram(m *image.NRGBA) []qColor {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	total := w * h
	// Con imágenes grandes basta una muestra de ~2 millones de píxeles.
	step := max(1, total/2_000_000)
	acc := histPool.Get().(*histAcc)
	defer histPool.Put(acc)
	clear(acc.n[:])
	clear(acc.s[:])
	for y := range h {
		row := m.Pix[y*m.Stride : y*m.Stride+w*4]
		for x := (y * 7) % step; x < w; x += step {
			p := row[x*4 : x*4+4 : x*4+4]
			var key uint32
			if p[3] != 0 {
				key = uint32(p[0]>>3)<<15 | uint32(p[1]>>3)<<10 | uint32(p[2]>>3)<<5 | uint32(p[3]>>3)
			}
			a := uint64(p[3])
			acc.n[key]++
			s := &acc.s[key]
			s[0] += uint64(p[0]) * a
			s[1] += uint64(p[1]) * a
			s[2] += uint64(p[2]) * a
			s[3] += a
		}
	}
	out := make([]qColor, 0, 4096)
	for k, n := range acc.n {
		if n == 0 {
			continue
		}
		s := &acc.s[k]
		fn := float32(n)
		out = append(out, qColor{float32(s[0]) / 255 / fn, float32(s[1]) / 255 / fn, float32(s[2]) / 255 / fn, float32(s[3]) / fn, fn})
	}
	return out
}

// histAcc acumula el histograma (se reutiliza: ocupa ~36 MB).
type histAcc struct {
	n [1 << 20]uint32
	s [1 << 20][4]uint64
}

var histPool = sync.Pool{New: func() any { return new(histAcc) }}

type qBox struct {
	lo, hi int
	err    float64
	mean   qColor
}

func boxStats(c []qColor) (qColor, float64) {
	var m qColor
	var sw float64
	var sr, sg, sb, sa float64
	for _, x := range c {
		w := float64(x.w)
		sr += float64(x.r) * w
		sg += float64(x.g) * w
		sb += float64(x.b) * w
		sa += float64(x.a) * w
		sw += w
	}
	if sw == 0 {
		return m, 0
	}
	m = qColor{float32(sr / sw), float32(sg / sw), float32(sb / sw), float32(sa / sw), float32(sw)}
	var e float64
	for i := range c {
		e += float64(qdist(&c[i], &m) * c[i].w)
	}
	return m, e
}

// medianCut divide el histograma en n cajas, partiendo siempre la de más error.
func medianCut(hist []qColor, n int) []qColor {
	if len(hist) <= n {
		return slices.Clone(hist)
	}
	mean, e := boxStats(hist)
	boxes := []qBox{{0, len(hist), e, mean}}
	for len(boxes) < n {
		bi := -1
		for i, b := range boxes {
			if b.hi-b.lo > 1 && (bi < 0 || b.err > boxes[bi].err) {
				bi = i
			}
		}
		if bi < 0 || boxes[bi].err <= 0 {
			break
		}
		b := boxes[bi]
		c := hist[b.lo:b.hi]
		// Canal con más varianza ponderada.
		var vr, vg, vb, va float64
		for _, x := range c {
			w := float64(x.w)
			vr += w * sq(float64(x.r-b.mean.r)) * wr
			vg += w * sq(float64(x.g-b.mean.g)) * wg
			vb += w * sq(float64(x.b-b.mean.b)) * wb
			va += w * sq(float64(x.a-b.mean.a)) * wa
		}
		ch := maxOf(vr, vg, vb, va)
		val := func(x *qColor) int {
			var v float32
			switch ch {
			case 0:
				v = x.r
			case 1:
				v = x.g
			case 2:
				v = x.b
			default:
				v = x.a
			}
			return min(max(int(v), 0), 255)
		}
		// Mediana por peso con un histograma de 256 valores (sin ordenar).
		var bins [256]float64
		lo, hi := 255, 0
		for i := range c {
			v := val(&c[i])
			bins[v] += float64(c[i].w)
			lo, hi = min(lo, v), max(hi, v)
		}
		if lo == hi {
			// Todos iguales en ese canal: no se puede partir así.
			boxes[bi].err = 0
			continue
		}
		half := float64(b.mean.w) / 2
		var acc float64
		cutV := lo
		for v := lo; v < hi; v++ {
			acc += bins[v]
			cutV = v
			if acc >= half {
				break
			}
		}
		// Partición: valores <= cutV a la izquierda.
		cut := 0
		for i := range c {
			if val(&c[i]) <= cutV {
				c[i], c[cut] = c[cut], c[i]
				cut++
			}
		}
		m1, e1 := boxStats(c[:cut])
		m2, e2 := boxStats(c[cut:])
		boxes[bi] = qBox{b.lo, b.lo + cut, e1, m1}
		boxes = append(boxes, qBox{b.lo + cut, b.hi, e2, m2})
	}
	pal := make([]qColor, len(boxes))
	for i, b := range boxes {
		pal[i] = b.mean
	}
	return pal
}

func sq(x float64) float64 { return x * x }

func maxOf(v ...float64) int {
	bi := 0
	for i, x := range v {
		if x > v[bi] {
			bi = i
		}
	}
	return bi
}

// kmeans refina la paleta con unas pocas iteraciones sobre el histograma.
func kmeans(hist []qColor, pal []qColor, iters int) []qColor {
	n := len(pal)
	for range iters {
		type sum struct{ r, g, b, a, w float64 }
		sums := make([]sum, n)
		var mu sync.Mutex
		nn := newSearcher(pal)
		parallel(len(hist), func(lo, hi int) {
			loc := make([]sum, n)
			for i := lo; i < hi; i++ {
				c := &hist[i]
				best := nn.search(c)
				w := float64(c.w)
				s := &loc[best]
				s.r += float64(c.r) * w
				s.g += float64(c.g) * w
				s.b += float64(c.b) * w
				s.a += float64(c.a) * w
				s.w += w
			}
			mu.Lock()
			for j := range sums {
				sums[j].r += loc[j].r
				sums[j].g += loc[j].g
				sums[j].b += loc[j].b
				sums[j].a += loc[j].a
				sums[j].w += loc[j].w
			}
			mu.Unlock()
		})
		for j, s := range sums {
			if s.w > 0 {
				pal[j] = qColor{float32(s.r / s.w), float32(s.g / s.w), float32(s.b / s.w), float32(s.a / s.w), float32(s.w)}
			}
		}
	}
	return pal
}

// nearest busca el color de paleta más cercano (paleta ordenada por verde).
type nearest struct {
	pal   []qColor
	order []int // índices ordenados por g
	gs    []float32
	lut   []atomic.Uint32 // caché por color cuantizado (0 = vacío)
	alpha bool
}

// newSearcher prepara la búsqueda sin caché.
func newSearcher(pal []qColor) *nearest {
	nn := &nearest{pal: pal}
	nn.order = make([]int, len(pal))
	for i := range nn.order {
		nn.order[i] = i
	}
	sort.Slice(nn.order, func(a, b int) bool { return pal[nn.order[a]].g < pal[nn.order[b]].g })
	nn.gs = make([]float32, len(pal))
	for i, o := range nn.order {
		nn.gs[i] = pal[o].g
	}
	return nn
}

func newNearest(pal []qColor, alpha bool) *nearest {
	nn := &nearest{pal: pal, alpha: alpha}
	nn.order = make([]int, len(pal))
	for i := range nn.order {
		nn.order[i] = i
	}
	sort.Slice(nn.order, func(a, b int) bool { return pal[nn.order[a]].g < pal[nn.order[b]].g })
	nn.gs = make([]float32, len(pal))
	for i, o := range nn.order {
		nn.gs[i] = pal[o].g
	}
	if alpha {
		nn.lut = make([]atomic.Uint32, 1<<22) // 6-6-6-4 bits
	} else {
		nn.lut = make([]atomic.Uint32, 1<<21) // 7-7-7 bits
	}
	return nn
}

func (nn *nearest) search(c *qColor) int {
	// Empieza en la posición de verde más parecida y se extiende a los lados.
	n := len(nn.gs)
	pos := sort.Search(n, func(i int) bool { return nn.gs[i] >= c.g })
	best, bd := 0, float32(math.MaxFloat32)
	for lo, hi := pos-1, pos; lo >= 0 || hi < n; {
		if hi < n {
			dg := nn.gs[hi] - c.g
			if wg*dg*dg >= bd {
				hi = n
			} else {
				if d := qdist(c, &nn.pal[nn.order[hi]]); d < bd {
					best, bd = nn.order[hi], d
				}
				hi++
			}
		}
		if lo >= 0 {
			dg := c.g - nn.gs[lo]
			if wg*dg*dg >= bd {
				lo = -1
			} else {
				if d := qdist(c, &nn.pal[nn.order[lo]]); d < bd {
					best, bd = nn.order[lo], d
				}
				lo--
			}
		}
	}
	return best
}

// lookup usa la caché con el color redondeado (r,g,b,a en 0–255, sin premultiplicar).
func (nn *nearest) lookup(r, g, b, a uint8) int {
	var key uint32
	if nn.alpha {
		key = uint32(r>>2)<<16 | uint32(g>>2)<<10 | uint32(b>>2)<<4 | uint32(a>>4)
	} else {
		key = uint32(r>>1)<<14 | uint32(g>>1)<<7 | uint32(b>>1)
	}
	if v := nn.lut[key].Load(); v != 0 {
		return int(v - 1)
	}
	// Centro de la celda de la caché.
	var cr, cg, cb, ca uint8
	if nn.alpha {
		cr, cg, cb, ca = r|2, g|2, b|2, a|8
		if a < 16 {
			ca = a
		}
	} else {
		cr, cg, cb, ca = r|1, g|1, b|1, 255
	}
	c := premulColor(cr, cg, cb, ca)
	i := nn.search(&c)
	nn.lut[key].Store(uint32(i + 1))
	return i
}

// remap asigna a cada píxel un índice de paleta, con difuminado opcional.
// Se trabaja por franjas en paralelo (el error no cruza de una franja a otra).
func remap(m *image.NRGBA, pal []qColor, idx []uint8, dither float32) {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	alpha := !isOpaque(m)
	nn := newNearest(pal, alpha)
	transp := -1
	for i, c := range pal {
		if c.a < 1 && (transp < 0 || c.a < pal[transp].a) {
			transp = i
		}
	}
	const band = 64
	nb := (h + band - 1) / band
	parallel(nb, func(blo, bhi int) {
		cur := make([]float32, (w+2)*4)
		nxt := make([]float32, (w+2)*4)
		for bi := blo; bi < bhi; bi++ {
			clear(cur)
			clear(nxt)
			for y := bi * band; y < min((bi+1)*band, h); y++ {
				row := m.Pix[y*m.Stride : y*m.Stride+w*4]
				out := idx[y*w : (y+1)*w]
				ltr := y%2 == 0
				for i := range w {
					x := i
					if !ltr {
						x = w - 1 - i
					}
					p := row[x*4 : x*4+4 : x*4+4]
					if p[3] == 0 && transp >= 0 {
						out[x] = uint8(transp)
						continue
					}
					e := cur[(x+1)*4 : (x+1)*4+4 : (x+1)*4+4]
					r := clampF(float32(p[0]) + e[0])
					g := clampF(float32(p[1]) + e[1])
					b := clampF(float32(p[2]) + e[2])
					a := clampF(float32(p[3]) + e[3])
					if !alpha {
						a = 255
					}
					k := nn.lookup(uint8(r+0.5), uint8(g+0.5), uint8(b+0.5), uint8(a+0.5))
					out[x] = uint8(k)
					if dither <= 0 {
						continue
					}
					c := unpremulQ(&pal[k])
					er := limitErr(r-c[0]) * dither
					eg := limitErr(g-c[1]) * dither
					eb := limitErr(b-c[2]) * dither
					ea := limitErr(a-c[3]) * dither
					diffuse := func(buf []float32, xx int, f float32) {
						if xx < 0 || xx >= w {
							return
						}
						d := buf[(xx+1)*4 : (xx+1)*4+4 : (xx+1)*4+4]
						d[0] += er * f
						d[1] += eg * f
						d[2] += eb * f
						d[3] += ea * f
					}
					dx := 1
					if !ltr {
						dx = -1
					}
					diffuse(cur, x+dx, 7.0/16)
					diffuse(nxt, x-dx, 3.0/16)
					diffuse(nxt, x, 5.0/16)
					diffuse(nxt, x+dx, 1.0/16)
				}
				cur, nxt = nxt, cur
				clear(nxt)
			}
		}
	})
}

func unpremulQ(c *qColor) [4]float32 {
	if c.a <= 0 {
		return [4]float32{}
	}
	f := 255 / c.a
	return [4]float32{c.r * f, c.g * f, c.b * f, c.a}
}

func clampF(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// limitErr suaviza errores grandes (evita "ruido" en bordes y zonas planas).
func limitErr(e float32) float32 {
	const lim = 40
	if e > lim {
		return lim + (e-lim)*0.25
	}
	if e < -lim {
		return -lim + (e+lim)*0.25
	}
	return e
}
