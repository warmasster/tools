package docs

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"os"
	"runtime"
	"strings"
)

// Escritor PDF mínimo para meter imágenes (una por página).

// PDFOptions controla la creación de un PDF a partir de imágenes.
type PDFOptions struct {
	// PageSize: "a4" (por defecto), "letter" o "image" (cada página mide lo que su imagen).
	PageSize string `json:"pageSize"`
}

// pdfImage es una imagen ya preparada para incrustar.
type pdfImage struct {
	w, h   int
	cs     string // DeviceRGB o DeviceGray
	filter string // DCTDecode o FlateDecode
	parms  string // DecodeParms (predictor PNG) si hay
	data   []byte
	smask  []byte // canal alfa (FlateDecode + predictor), si hay
}

type pdfWriter struct {
	w       *bufio.Writer
	off     int64
	offsets map[int]int64
	next    int
	pages   []int
}

func newPDFWriter(w io.Writer) *pdfWriter {
	p := &pdfWriter{w: bufio.NewWriterSize(w, 1<<20), offsets: map[int]int64{}, next: 3}
	p.printf("%%PDF-1.5\n%%\xe2\xe3\xcf\xd3\n")
	return p
}

func (p *pdfWriter) printf(format string, a ...any) {
	n, _ := fmt.Fprintf(p.w, format, a...)
	p.off += int64(n)
}

func (p *pdfWriter) write(b []byte) {
	n, _ := p.w.Write(b)
	p.off += int64(n)
}

func (p *pdfWriter) alloc() int {
	n := p.next
	p.next++
	return n
}

func (p *pdfWriter) begin(n int) {
	p.offsets[n] = p.off
	p.printf("%d 0 obj\n", n)
}

func (p *pdfWriter) stream(n int, dict string, data []byte) {
	p.begin(n)
	p.printf("<<%s /Length %d>>\nstream\n", dict, len(data))
	p.write(data)
	p.printf("\nendstream\nendobj\n")
}

// addPage añade una página con la imagen centrada. pw/ph en puntos.
func (p *pdfWriter) addPage(img *pdfImage, pw, ph, x, y, dw, dh float64) {
	pageN, contN, imgN := p.alloc(), p.alloc(), p.alloc()
	smaskN := 0
	if img.smask != nil {
		smaskN = p.alloc()
	}
	p.pages = append(p.pages, pageN)
	p.begin(pageN)
	p.printf("<</Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] /Resources <</XObject <</Im0 %d 0 R>> /ProcSet [/PDF /ImageC /ImageB]>> /Contents %d 0 R>>\nendobj\n",
		fnum(pw), fnum(ph), imgN, contN)
	content := fmt.Sprintf("q %s 0 0 %s %s %s cm /Im0 Do Q", fnum(dw), fnum(dh), fnum(x), fnum(y))
	p.stream(contN, "", []byte(content))
	dict := fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /%s /BitsPerComponent 8 /Filter /%s", img.w, img.h, img.cs, img.filter)
	if img.parms != "" {
		dict += " /DecodeParms " + img.parms
	}
	if smaskN > 0 {
		dict += fmt.Sprintf(" /SMask %d 0 R", smaskN)
	}
	p.stream(imgN, dict, img.data)
	if smaskN > 0 {
		sd := fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceGray /BitsPerComponent 8 /Filter /FlateDecode /DecodeParms <</Predictor 15 /Colors 1 /BitsPerComponent 8 /Columns %d>>", img.w, img.h, img.w)
		p.stream(smaskN, sd, img.smask)
	}
}

func (p *pdfWriter) finish() error {
	p.begin(2)
	var kids strings.Builder
	for _, n := range p.pages {
		fmt.Fprintf(&kids, "%d 0 R ", n)
	}
	p.printf("<</Type /Pages /Kids [%s] /Count %d>>\nendobj\n", kids.String(), len(p.pages))
	p.begin(1)
	p.printf("<</Type /Catalog /Pages 2 0 R>>\nendobj\n")
	info := p.alloc()
	p.begin(info)
	p.printf("<</Producer (TurboMENU)>>\nendobj\n")
	xref := p.off
	p.printf("xref\n0 %d\n0000000000 65535 f \n", p.next)
	for i := 1; i < p.next; i++ {
		p.printf("%010d 00000 n \n", p.offsets[i])
	}
	p.printf("trailer\n<</Size %d /Root 1 0 R /Info %d 0 R>>\nstartxref\n%d\n%%%%EOF\n", p.next, info, xref)
	return p.w.Flush()
}

func fnum(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" || s == "-0" {
		return "0"
	}
	return s
}

// pageLayout calcula página y colocación de una imagen de w×h píxeles.
func pageLayout(size string, w, h int) (pw, ph, x, y, dw, dh float64) {
	// Tamaño natural a 96 ppp.
	nw, nh := float64(w)*72/96, float64(h)*72/96
	var sw, sh float64
	switch size {
	case "image":
		return nw, nh, 0, 0, nw, nh
	case "letter":
		sw, sh = 612, 792
	default:
		sw, sh = 595.28, 841.89
	}
	if w > h {
		sw, sh = sh, sw
	}
	const margin = 20
	aw, ah := sw-2*margin, sh-2*margin
	f := min(aw/nw, ah/nh, 1)
	dw, dh = nw*f, nh*f
	return sw, sh, (sw - dw) / 2, (sh - dh) / 2, dw, dh
}

// preparePDFImage abre una imagen y la deja lista para el PDF. Los JPEG que
// no hace falta girar se incrustan tal cual, sin recodificar.
func preparePDFImage(ctx context.Context, path string) (*pdfImage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, cleanErr(ctx, err, "No se puede abrir la imagen")
	}
	head := make([]byte, 32)
	n, _ := io.ReadFull(f, head)
	f.Close()
	if sniff(head[:n]) == "jpeg" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, cleanErr(ctx, err, "No se puede leer la imagen")
		}
		meta := parseJPEGMeta(data)
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err == nil && meta.orientation <= 1 {
			cs := ""
			switch cfg.ColorModel {
			case color.YCbCrModel:
				cs = "DeviceRGB"
			case color.GrayModel:
				cs = "DeviceGray"
			}
			if cs != "" {
				return &pdfImage{w: cfg.Width, h: cfg.Height, cs: cs, filter: "DCTDecode", data: data}, nil
			}
		}
	}
	d, err := decodeFile(ctx, path)
	if err != nil {
		return nil, err
	}
	img := d.img
	if d.anim != nil {
		img = gifFrame(d.anim, 0)
	}
	photo := d.format == "jpeg" || d.viaWIC || d.format == "webp"
	return pdfImageFrom(ctx, img, photo, 92)
}

// pdfImageFrom prepara una imagen decodificada: JPEG si es foto, si no Flate sin pérdida.
func pdfImageFrom(ctx context.Context, img image.Image, photo bool, q int) (*pdfImage, error) {
	img = normalize(img)
	b := img.Bounds()
	pi := &pdfImage{w: b.Dx(), h: b.Dy(), cs: "DeviceRGB"}
	var alpha []byte
	if m, ok := img.(*image.NRGBA); ok && !isOpaque(m) {
		alpha = make([]byte, b.Dx()*b.Dy())
		for y := range b.Dy() {
			row := m.Pix[y*m.Stride:]
			for x := range b.Dx() {
				alpha[y*b.Dx()+x] = row[x*4+3]
			}
		}
	}
	if _, ok := img.(*image.Gray); ok {
		pi.cs = "DeviceGray"
	}
	if photo {
		src := img
		if alpha != nil {
			src = opaqueCopy(img.(*image.NRGBA))
		}
		r, err := encodeJPEG(ctx, src, q, nil, 0)
		if err != nil {
			return nil, err
		}
		pi.filter, pi.data = "DCTDecode", r.data
	} else {
		var p *pngImage
		switch m := img.(type) {
		case *image.Gray:
			p = pngFromImage(m)
		default:
			nm := toNRGBA(img)
			w := b.Dx()
			p = &pngImage{w: w, h: b.Dy(), colorTyp: 2, depth: 8, rowBytes: w * 3, rows: func(y int, dst []byte) {
				s := nm.Pix[y*nm.Stride:]
				for x := range w {
					dst[x*3], dst[x*3+1], dst[x*3+2] = s[x*4], s[x*4+1], s[x*4+2]
				}
			}}
		}
		z, err := p.idat(ctx, 9)
		if err != nil {
			return nil, err
		}
		colors := 3
		if pi.cs == "DeviceGray" {
			colors = 1
		}
		pi.filter, pi.data = "FlateDecode", z
		pi.parms = fmt.Sprintf("<</Predictor 15 /Colors %d /BitsPerComponent 8 /Columns %d>>", colors, b.Dx())
	}
	if alpha != nil {
		w := b.Dx()
		p := &pngImage{w: w, h: b.Dy(), colorTyp: 0, depth: 8, rowBytes: w, rows: func(y int, dst []byte) {
			copy(dst, alpha[y*w:(y+1)*w])
		}}
		z, err := p.idat(ctx, 9)
		if err != nil {
			return nil, err
		}
		pi.smask = z
	}
	return pi, nil
}

// opaqueCopy copia la imagen con alfa 255 (el alfa va aparte, en la SMask).
func opaqueCopy(m *image.NRGBA) *image.NRGBA {
	out := &image.NRGBA{Pix: make([]uint8, len(m.Pix)), Stride: m.Stride, Rect: m.Rect}
	copy(out.Pix, m.Pix)
	for i := 3; i < len(out.Pix); i += 4 {
		out.Pix[i] = 255
	}
	return out
}

// ImagesToPDF junta varias imágenes en un PDF (una por página, en el orden
// dado). Se guarda junto a la primera imagen con su nombre y extensión .pdf.
func ImagesToPDF(ctx context.Context, paths []string, opts PDFOptions, progress Progress) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(paths) == 0 {
		return Result{}, uerr(nil, "No hay imágenes")
	}
	var before int64
	for _, p := range paths {
		before += fileSize(p)
	}
	o, err := createOut(paths[0], "", ".pdf")
	if err != nil {
		return Result{}, err
	}
	res, err := imagesToPDFFile(ctx, paths, opts, o, progress)
	if err != nil {
		o.Abort()
		return Result{}, cleanErr(ctx, err, "No se ha podido crear el PDF")
	}
	res.Before = before
	return res, nil
}

func imagesToPDFFile(parent context.Context, paths []string, opts PDFOptions, o *outFile, progress Progress) (Result, error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	type job struct {
		img      *pdfImage
		err      error
		launched bool
		ok       chan struct{}
	}
	jobs := make([]*job, len(paths))
	for i := range jobs {
		jobs[i] = &job{ok: make(chan struct{})}
	}
	// Se preparan varias imágenes a la vez (ventana limitada), pero se escriben en orden.
	sem := make(chan struct{}, max(2, runtime.GOMAXPROCS(0)/4))
	go func() {
		for i, p := range paths {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				for _, j := range jobs[i:] {
					j.err = ErrCanceled
					close(j.ok)
				}
				return
			}
			jobs[i].launched = true
			go func(j *job, p string) {
				j.img, j.err = preparePDFImage(ctx, p)
				close(j.ok)
			}(jobs[i], p)
		}
	}()
	// drain espera a los trabajos pendientes tras un error.
	drain := func(from int) {
		cancel()
		for _, k := range jobs[from:] {
			<-k.ok
			if k.launched {
				<-sem
			}
		}
	}
	pw := newPDFWriter(o)
	for i, j := range jobs {
		<-j.ok
		if j.launched {
			<-sem
		}
		if j.err == nil {
			j.err = ctxErr(parent)
		}
		if j.err != nil {
			drain(i + 1)
			return Result{}, j.err
		}
		w, h, x, y, dw, dh := pageLayout(opts.PageSize, j.img.w, j.img.h)
		pw.addPage(j.img, w, h, x, y, dw, dh)
		j.img = nil
		progress.set(float64(i+1) / float64(len(jobs)) * 0.98)
	}
	if err := pw.finish(); err != nil {
		return Result{}, err
	}
	size, err := o.Commit()
	if err != nil {
		return Result{}, err
	}
	progress.set(1)
	return Result{Out: o.Final, After: size}, nil
}
