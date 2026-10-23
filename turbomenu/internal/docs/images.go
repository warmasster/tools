package docs

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"io"
	"math"
	"os"
	"strings"
	"sync"

	"github.com/klauspost/compress/zlib"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
	"golang.org/x/image/webp"
)

// ImageInfo resume una imagen.
type ImageInfo struct {
	Format   string `json:"format"`   // jpeg, png, gif, bmp, tiff, webp, heic, avif…
	Name     string `json:"name"`     // nombre para enseñar: «JPG», «PNG», «HEIC»…
	Width    int    `json:"width"`    // ya girada según EXIF
	Height   int    `json:"height"`   //
	HasAlpha bool   `json:"hasAlpha"` // tiene (o puede tener) transparencia
	Size     int64  `json:"size"`     // bytes del archivo
	Frames   int    `json:"frames"`   // fotogramas (GIF animado > 1)
}

// ImageOptions controla la reducción de una imagen.
type ImageOptions struct {
	Quality    int    `json:"quality"`    // 1–100 (0 = 80). En PNG, 100 = sin pérdida
	MaxSide    int    `json:"maxSide"`    // lado mayor máximo en píxeles (0 = sin límite)
	Scale      int    `json:"scale"`      // escala en % (0 o 100 = tamaño original)
	Format     string `json:"format"`     // formato de salida (jpg, png, gif…); "" = automático
	KeepLarger bool   `json:"keepLarger"` // guardar aunque pese más que el original
}

func (o ImageOptions) quality() int {
	if o.Quality <= 0 {
		return 80
	}
	return min(o.Quality, 100)
}

// formatNames da el nombre visible de cada formato.
var formatNames = map[string]string{
	"jpeg": "JPG", "png": "PNG", "gif": "GIF", "bmp": "BMP", "tiff": "TIFF", "webp": "WebP",
	"heic": "HEIC", "heif": "HEIF", "avif": "AVIF", "jxl": "JPEG XL", "ico": "ICO", "pdf": "PDF",
}

func formatName(f string) string {
	if n, ok := formatNames[f]; ok {
		return n
	}
	return strings.ToUpper(f)
}

// normFormat traduce extensiones a nombres de formato internos.
func normFormat(f string) string {
	f = strings.TrimPrefix(strings.ToLower(f), ".")
	switch f {
	case "jpg", "jpe", "jfif":
		return "jpeg"
	case "tif":
		return "tiff"
	}
	return f
}

// extFor devuelve la extensión de archivo para un formato.
func extFor(f string) string {
	switch f {
	case "jpeg":
		return ".jpg"
	case "tiff":
		return ".tif"
	}
	return "." + f
}

// InspectImage devuelve formato, tamaño y transparencia de una imagen. Con
// JPEG y WebP solo lee la cabecera (rápido); el resto se abre entero y queda
// en caché para la vista previa. HEIC, AVIF… se abren con Windows.
func InspectImage(path string) (ImageInfo, error) {
	if info, ok := quickInfo(path); ok {
		return info, nil
	}
	d, err := loadImage(context.Background(), path)
	if err != nil {
		return ImageInfo{}, err
	}
	b := d.img.Bounds()
	info := ImageInfo{Format: d.format, Name: formatName(d.format), Width: b.Dx(), Height: b.Dy(), HasAlpha: d.alpha, Size: d.size, Frames: 1}
	if d.anim != nil {
		info.Frames = len(d.anim.Image)
	}
	return info, nil
}

// quickInfo lee solo la cabecera de JPEG y WebP.
func quickInfo(path string) (ImageInfo, bool) {
	f, err := os.Open(path)
	if err != nil {
		return ImageInfo{}, false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return ImageInfo{}, false
	}
	// La cabecera (con EXIF y miniatura) cabe de sobra en 256 KB.
	head := make([]byte, min(st.Size(), 256<<10))
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	format := sniff(head[:min(len(head), 32)])
	info := ImageInfo{Format: format, Name: formatName(format), Size: st.Size(), Frames: 1}
	switch format {
	case "jpeg":
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(head))
		if err != nil {
			return info, false
		}
		info.Width, info.Height = cfg.Width, cfg.Height
		if parseJPEGMeta(head).orientation >= 5 {
			info.Width, info.Height = cfg.Height, cfg.Width
		}
	case "webp":
		cfg, err := webp.DecodeConfig(bytes.NewReader(head))
		if err != nil {
			return info, false
		}
		info.Width, info.Height = cfg.Width, cfg.Height
		info.HasAlpha = cfg.ColorModel == color.NYCbCrAModel
	default:
		return info, false
	}
	return info, true
}

// autoFormat elige el formato de salida al reducir.
func autoFormat(d *decoded, q int) string {
	switch d.format {
	case "jpeg", "png", "gif":
		return d.format
	}
	if d.alpha {
		return "png"
	}
	if q >= 100 && (d.format == "bmp" || d.format == "tiff") {
		return "png"
	}
	return "jpeg"
}

// paletteSize decide cuántos colores usar en PNG/GIF según la calidad.
func paletteSize(q int) int {
	if q >= 60 {
		return 256
	}
	n := int(math.Round(256 * math.Pow(2, float64(q-60)/15)))
	return min(max(n, 8), 256)
}

func ditherFor(q int) float32 {
	return 0.5 + 0.5*float32(q)/100
}

// encoded es un resultado codificado en memoria.
type encoded struct {
	data []byte
	view image.Image // aspecto del resultado (solo si se pidió)
	w, h int
}

// encodeImage codifica img en el formato pedido.
// view: lado mayor de la miniatura que se quiere del resultado (0 = ninguna).
func encodeImage(ctx context.Context, d *decoded, img image.Image, format string, q int, view int) (*encoded, error) {
	wantView := view > 0
	b := img.Bounds()
	out := &encoded{w: b.Dx(), h: b.Dy()}
	switch format {
	case "jpeg":
		img = flatten(img)
		r, err := encodeJPEG(ctx, img, q, d.icc, view)
		if err != nil {
			return nil, err
		}
		out.data, out.view = r.data, r.recon
	case "png":
		data, view, err := encodePNG(ctx, img, q, d.icc, wantView)
		if err != nil {
			return nil, err
		}
		out.data, out.view = data, view
	case "gif":
		pm := gifPaletted(img, q)
		var buf bytes.Buffer
		if err := gif.Encode(&buf, pm, &gif.Options{NumColors: len(pm.Palette)}); err != nil {
			return nil, err
		}
		out.data = buf.Bytes()
		if wantView {
			out.view = pm
		}
	case "bmp":
		var buf bytes.Buffer
		src := img
		if _, ok := img.(*image.YCbCr); ok {
			src = toNRGBA(img)
		}
		if err := bmp.Encode(&buf, src); err != nil {
			return nil, err
		}
		out.data, out.view = buf.Bytes(), img
	case "tiff":
		var buf bytes.Buffer
		src := img
		if _, ok := img.(*image.YCbCr); ok {
			src = toNRGBA(img)
		}
		if err := tiff.Encode(&buf, src, &tiff.Options{Compression: tiff.Deflate, Predictor: true}); err != nil {
			return nil, err
		}
		out.data, out.view = buf.Bytes(), img
	case "ico":
		data, err := encodeICO(ctx, img)
		if err != nil {
			return nil, err
		}
		out.data, out.view = data, img
	default:
		return nil, uerr(nil, "No se puede guardar en formato %s", formatName(format))
	}
	if ctx != nil && ctx.Err() != nil {
		return nil, ErrCanceled
	}
	return out, nil
}

// encodePNG: con q=100 sin pérdida (paleta exacta si cabe); por debajo, paleta
// cuantizada con difuminado.
func encodePNG(ctx context.Context, img image.Image, q int, icc []byte, wantView bool) ([]byte, image.Image, error) {
	var p *pngImage
	var view image.Image = img
	switch m := img.(type) {
	case *image.Gray:
		if q >= 100 {
			p = pngFromImage(m)
			break
		}
		img = toNRGBA(m)
	case *image.YCbCr:
		img = toNRGBA(m)
	}
	if p == nil {
		m := toNRGBA(img)
		if q >= 100 {
			m = cleanTransparent(m)
			opaque, gray := analyzeNRGBA(m)
			pal, idx, ok := exactPalette(m, 256)
			switch {
			case ok && (len(pal) <= 16 || !gray):
				p = pngPaletted(idx, m.Rect.Dx(), m.Rect.Dy(), pal)
			default:
				_ = opaque
				p = pngFromImage(m)
			}
			view = m
		} else {
			n := paletteSize(q)
			pal, idx, ok := exactPalette(m, n)
			if !ok {
				pal, idx = quantize(m, n, ditherFor(q))
			}
			p = pngPaletted(idx, m.Rect.Dx(), m.Rect.Dy(), pal)
			if wantView {
				view = palettedView(idx, m.Rect.Dx(), m.Rect.Dy(), pal)
			}
		}
	}
	data, err := p.encode(ctx, 9, icc)
	if err != nil {
		return nil, nil, err
	}
	return data, view, nil
}

// cleanTransparent pone a cero el color de los píxeles invisibles (comprime mejor).
func cleanTransparent(m *image.NRGBA) *image.NRGBA {
	dirty := false
	w, h := m.Rect.Dx(), m.Rect.Dy()
	for y := 0; y < h && !dirty; y++ {
		row := m.Pix[y*m.Stride : y*m.Stride+w*4]
		for i := 0; i < len(row); i += 4 {
			if row[i+3] == 0 && (row[i]|row[i+1]|row[i+2]) != 0 {
				dirty = true
				break
			}
		}
	}
	if !dirty {
		return m
	}
	out := &image.NRGBA{Pix: make([]uint8, w*h*4), Stride: w * 4, Rect: image.Rect(0, 0, w, h)}
	parallel(h, func(lo, hi int) {
		for y := lo; y < hi; y++ {
			s := m.Pix[y*m.Stride : y*m.Stride+w*4]
			d := out.Pix[y*w*4 : (y+1)*w*4]
			copy(d, s)
			for i := 0; i < len(d); i += 4 {
				if d[i+3] == 0 {
					d[i], d[i+1], d[i+2] = 0, 0, 0
				}
			}
		}
	})
	return out
}

func palettedView(idx []uint8, w, h int, pal []color.NRGBA) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	parallel(h, func(lo, hi int) {
		for i := lo * w; i < hi*w; i++ {
			c := pal[idx[i]]
			out.Pix[i*4], out.Pix[i*4+1], out.Pix[i*4+2], out.Pix[i*4+3] = c.R, c.G, c.B, c.A
		}
	})
	return out
}

// gifPaletted cuantiza para GIF (transparencia de 1 bit: alfa < 128).
func gifPaletted(img image.Image, q int) *image.Paletted {
	m := toNRGBA(img)
	w, h := m.Rect.Dx(), m.Rect.Dy()
	bin := &image.NRGBA{Pix: make([]uint8, len(m.Pix)), Stride: m.Stride, Rect: m.Rect}
	copy(bin.Pix, m.Pix)
	for i := 3; i < len(bin.Pix); i += 4 {
		if bin.Pix[i] < 128 {
			bin.Pix[i-3], bin.Pix[i-2], bin.Pix[i-1], bin.Pix[i] = 0, 0, 0, 0
		} else {
			bin.Pix[i] = 255
		}
	}
	n := paletteSize(q)
	pal, idx, ok := exactPalette(bin, n)
	if !ok {
		pal, idx = quantize(bin, n, ditherFor(q))
	}
	cp := make(color.Palette, len(pal))
	for i, c := range pal {
		cp[i] = c
	}
	return &image.Paletted{Pix: idx, Stride: w, Rect: image.Rect(0, 0, w, h), Palette: cp}
}

// encodeICO genera un icono con varios tamaños (PNG dentro de cada entrada).
func encodeICO(ctx context.Context, img image.Image) ([]byte, error) {
	src := toNRGBA(img)
	w, h := src.Rect.Dx(), src.Rect.Dy()
	side := max(w, h)
	// Lienzo cuadrado transparente con la imagen centrada.
	sq := src
	if w != h {
		sq = image.NewNRGBA(image.Rect(0, 0, side, side))
		ox, oy := (side-w)/2, (side-h)/2
		for y := range h {
			copy(sq.Pix[(oy+y)*sq.Stride+ox*4:], src.Pix[y*src.Stride:y*src.Stride+w*4])
		}
	}
	var sizes []int
	for _, s := range []int{16, 24, 32, 48, 64, 128, 256} {
		if s <= side || s <= 48 {
			sizes = append(sizes, s)
		}
	}
	pngs := make([][]byte, len(sizes))
	errs := make([]error, len(sizes))
	var wg sync.WaitGroup
	for i, s := range sizes {
		wg.Add(1)
		go func(i, s int) {
			defer wg.Done()
			r := resizeImage(sq, s, s).(*image.NRGBA)
			pngs[i], errs[i] = pngFromImage(r).encode(ctx, 9, nil)
		}(i, s)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	var buf bytes.Buffer
	hdr := []byte{0, 0, 1, 0, byte(len(sizes)), 0}
	buf.Write(hdr)
	off := 6 + 16*len(sizes)
	for i, s := range sizes {
		e := make([]byte, 16)
		if s < 256 {
			e[0], e[1] = byte(s), byte(s)
		}
		binary.LittleEndian.PutUint16(e[4:], 1)
		binary.LittleEndian.PutUint16(e[6:], 32)
		binary.LittleEndian.PutUint32(e[8:], uint32(len(pngs[i])))
		binary.LittleEndian.PutUint32(e[12:], uint32(off))
		off += len(pngs[i])
		buf.Write(e)
	}
	for _, p := range pngs {
		buf.Write(p)
	}
	return buf.Bytes(), nil
}

// ReduceImage guarda una versión más ligera de la imagen junto al original
// («foto (reducida).jpg»). Si no pesa menos que el original, no guarda nada
// (salvo KeepLarger) y devuelve Result.Larger = true junto con ErrNotSmaller.
func ReduceImage(ctx context.Context, path string, opts ImageOptions, progress Progress) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	progress.set(0.02)
	d, err := loadImage(ctx, path)
	if err != nil {
		return Result{}, err
	}
	progress.set(0.25)
	q := opts.quality()
	format := normFormat(opts.Format)
	if format == "" {
		format = autoFormat(d, q)
	}
	var data []byte
	var w, h int
	if d.anim != nil && format == "gif" {
		data, w, h, err = reduceAnimGIF(ctx, d.anim, opts)
	} else {
		var e *encoded
		e, err = encodeCached(ctx, d, opts, format, 0)
		if e != nil {
			data, w, h = e.data, e.w, e.h
		}
	}
	if err != nil {
		return Result{}, cleanErr(ctx, err, "No se ha podido reducir la imagen")
	}
	progress.set(0.9)
	res := Result{Before: d.size, After: int64(len(data)), Width: w, Height: h}
	res.Larger = res.After >= res.Before
	if res.Larger && !opts.KeepLarger {
		progress.set(1)
		return res, ErrNotSmaller
	}
	out, err := writeOut(path, "reducida", extFor(format), data)
	if err != nil {
		return Result{}, err
	}
	res.Out = out
	progress.set(1)
	return res, nil
}

// Caché del último resultado codificado: así «Guardar» tras la vista previa es inmediato.
var encCache struct {
	sync.Mutex
	key  string
	e    *encoded
	view int
}

// Caché de la imagen ya reescalada (al mover la barra de calidad no cambia).
var sizedCache struct {
	sync.Mutex
	key string
	img image.Image
}

func sizedImage(d *decoded, opts ImageOptions) image.Image {
	b := d.img.Bounds()
	w, h := fitSize(b.Dx(), b.Dy(), opts.MaxSide, opts.Scale)
	if w == b.Dx() && h == b.Dy() {
		return d.img
	}
	key := fmt.Sprintf("%s|%d|%d", d.key, w, h)
	sizedCache.Lock()
	defer sizedCache.Unlock()
	if sizedCache.key == key {
		return sizedCache.img
	}
	img := resizeImage(d.img, w, h)
	sizedCache.key, sizedCache.img = key, img
	return img
}

func encodeCached(ctx context.Context, d *decoded, opts ImageOptions, format string, view int) (*encoded, error) {
	q := opts.quality()
	img := sizedImage(d, opts)
	b := img.Bounds()
	key := fmt.Sprintf("%s|%s|%d|%d|%d", d.key, format, q, b.Dx(), b.Dy())
	encCache.Lock()
	if encCache.key == key && (view == 0 || encCache.view == view) {
		e := encCache.e
		encCache.Unlock()
		return e, nil
	}
	encCache.Unlock()
	e, err := encodeImage(ctx, d, img, format, q, view)
	if err != nil {
		return nil, err
	}
	encCache.Lock()
	encCache.key, encCache.e, encCache.view = key, e, view
	encCache.Unlock()
	return e, nil
}

// PreviewResult es la vista previa antes/después de una reducción.
type PreviewResult struct {
	Original string `json:"original"` // data URL del original reducido a maxSide
	Result   string `json:"result"`   // data URL del resultado reducido a maxSide
	Size     int64  `json:"size"`     // bytes exactos que tendrá el archivo reducido
	OrigSize int64  `json:"origSize"` // bytes del original
	Width    int    `json:"width"`    // dimensiones del resultado
	Height   int    `json:"height"`
	Format   string `json:"format"` // formato de salida (jpeg, png…)
	Larger   bool   `json:"larger"` // el resultado pesaría igual o más que el original
}

var previewState struct {
	sync.Mutex
	cancel context.CancelFunc
	thumbs map[string]string
}

// Preview calcula la vista previa de ReduceImage sin guardar nada. Pensada
// para llamarse muchas veces seguidas: cachea la imagen abierta y reescalada,
// y si llega otra llamada mientras tanto, la anterior termina con ErrSuperseded.
func Preview(path string, opts ImageOptions, maxSide int) (PreviewResult, error) {
	ctx, cancel := context.WithCancel(context.Background())
	previewState.Lock()
	if previewState.cancel != nil {
		previewState.cancel()
	}
	previewState.cancel = cancel
	previewState.Unlock()
	defer cancel()
	res, err := preview(ctx, path, opts, maxSide)
	if err != nil && ctx.Err() != nil {
		return PreviewResult{}, ErrSuperseded
	}
	return res, err
}

func preview(ctx context.Context, path string, opts ImageOptions, maxSide int) (PreviewResult, error) {
	if maxSide <= 0 {
		maxSide = 800
	}
	d, err := loadImage(ctx, path)
	if err != nil {
		return PreviewResult{}, err
	}
	q := opts.quality()
	format := normFormat(opts.Format)
	if format == "" {
		format = autoFormat(d, q)
	}
	ob := d.img.Bounds()
	tw, th := fitSize(ob.Dx(), ob.Dy(), maxSide, 0)
	res := PreviewResult{OrigSize: d.size, Format: format}
	// Miniatura del original (cacheada).
	tkey := fmt.Sprintf("%s|%d|%d", d.key, tw, th)
	previewState.Lock()
	if previewState.thumbs == nil {
		previewState.thumbs = map[string]string{}
	}
	orig, ok := previewState.thumbs[tkey]
	previewState.Unlock()
	if !ok {
		src := d.img
		if d.anim != nil {
			src = gifFrame(d.anim, 0)
		}
		orig = dataURL(resizeImage(src, tw, th))
		previewState.Lock()
		if len(previewState.thumbs) > 8 {
			clear(previewState.thumbs)
		}
		previewState.thumbs[tkey] = orig
		previewState.Unlock()
	}
	res.Original = orig
	if ctx.Err() != nil {
		return res, ErrCanceled
	}
	if d.anim != nil && format == "gif" {
		data, w, h, err := reduceAnimGIF(ctx, d.anim, opts)
		if err != nil {
			return res, cleanErr(ctx, err, "No se ha podido preparar la vista previa")
		}
		res.Size, res.Width, res.Height = int64(len(data)), w, h
		sized := gifPaletted(resizeImage(gifFrame(d.anim, 0), w, h), q)
		res.Result = dataURL(resizeImage(toNRGBA(sized), tw, th))
		res.Larger = res.Size >= res.OrigSize
		return res, nil
	}
	e, err := encodeCached(ctx, d, opts, format, max(tw, th))
	if err != nil {
		return res, cleanErr(ctx, err, "No se ha podido preparar la vista previa")
	}
	res.Size, res.Width, res.Height = int64(len(e.data)), e.w, e.h
	res.Larger = res.Size >= res.OrigSize
	view := e.view
	if view == nil {
		view = sizedImage(d, opts)
	}
	view = normalize(view)
	res.Result = dataURL(resizeImage(view, tw, th))
	return res, nil
}

// dataURL codifica una miniatura: JPEG si es opaca, PNG si tiene transparencia.
func dataURL(img image.Image) string {
	img = normalize(img)
	if hasAlpha(img) {
		data, err := pngFromImage(img).encode(nil, 1, nil)
		if err != nil {
			return ""
		}
		return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 92}); err != nil {
		return ""
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// reduceAnimGIF reescala y recuantiza cada fotograma de un GIF animado.
func reduceAnimGIF(ctx context.Context, g *gif.GIF, opts ImageOptions) ([]byte, int, int, error) {
	q := opts.quality()
	cw, ch := g.Config.Width, g.Config.Height
	nw, nh := fitSize(cw, ch, opts.MaxSide, opts.Scale)
	fx, fy := float64(nw)/float64(cw), float64(nh)/float64(ch)
	out := &gif.GIF{
		Image:           make([]*image.Paletted, len(g.Image)),
		Delay:           g.Delay,
		LoopCount:       g.LoopCount,
		Disposal:        g.Disposal,
		Config:          image.Config{Width: nw, Height: nh},
		BackgroundIndex: 0,
	}
	errs := make([]error, len(g.Image))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, fr := range g.Image {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, fr *image.Paletted) {
			defer wg.Done()
			defer func() { <-sem }()
			if ctx.Err() != nil {
				errs[i] = ErrCanceled
				return
			}
			b := fr.Bounds()
			r := image.Rect(int(math.Round(float64(b.Min.X)*fx)), int(math.Round(float64(b.Min.Y)*fy)),
				int(math.Round(float64(b.Max.X)*fx)), int(math.Round(float64(b.Max.Y)*fy)))
			if r.Dx() < 1 {
				r.Max.X = r.Min.X + 1
			}
			if r.Dy() < 1 {
				r.Max.Y = r.Min.Y + 1
			}
			src := toNRGBA(fr)
			var img image.Image = src
			if r.Dx() != b.Dx() || r.Dy() != b.Dy() {
				img = resizeImage(src, r.Dx(), r.Dy())
			} else if q >= 100 {
				// Sin cambios de tamaño ni de calidad: se deja el fotograma tal cual.
				cp := *fr
				out.Image[i] = &cp
				return
			}
			pm := gifPaletted(img, q)
			pm.Rect = r
			out.Image[i] = pm
		}(i, fr)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, 0, 0, err
		}
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, out); err != nil {
		return nil, 0, 0, err
	}
	return buf.Bytes(), nw, nh, nil
}

func zlibReader(b []byte) (interface {
	Read([]byte) (int, error)
}, error) {
	return zlib.NewReader(bytes.NewReader(b))
}
