package docs

import (
	"context"
	"image"
)

// Target es un formato al que se puede convertir un archivo.
type Target struct {
	ID   string `json:"id"`   // lo que se pasa a Convert: "jpg", "png", "pdf"…
	Ext  string `json:"ext"`  // extensión del resultado, con punto
	Name string `json:"name"` // nombre corto: «JPG»
	Desc string `json:"desc"` // para qué sirve, en una frase
}

var imageTargets = []Target{
	{"jpg", ".jpg", "JPG", "Más ligero, ideal para fotos"},
	{"png", ".png", "PNG", "Sin pérdida, admite fondo transparente"},
	{"pdf", ".pdf", "PDF", "Para enviar o imprimir como documento"},
	{"ico", ".ico", "ICO", "Para iconos de programas y accesos directos"},
	{"gif", ".gif", "GIF", "Hasta 256 colores, para gráficos sencillos"},
	{"tiff", ".tif", "TIFF", "Sin pérdida, para imprenta y escáneres"},
	{"bmp", ".bmp", "BMP", "Sin compresión, para programas antiguos"},
}

// Kind clasifica un archivo: "image", "pdf", "office" o "" si no se reconoce.
func Kind(path string) string {
	switch e := lowerExt(path); {
	case e == "pdf":
		return "pdf"
	case IsOfficeDoc(path):
		return "office"
	case imageExts[e]:
		return "image"
	}
	return ""
}

var imageExts = map[string]bool{
	"jpg": true, "jpeg": true, "jpe": true, "jfif": true, "png": true, "gif": true, "bmp": true, "dib": true,
	"tif": true, "tiff": true, "webp": true, "heic": true, "heif": true, "hif": true, "avif": true, "jxl": true,
	"jxr": true, "wdp": true, "dng": true, "cr2": true, "cr3": true, "nef": true, "arw": true, "orf": true,
	"rw2": true, "raf": true, "srw": true, "pef": true, "raw": true,
}

// Targets devuelve los formatos a los que se puede convertir el archivo.
// (WebP y AVIF como destino los ofrece el paquete media.)
func Targets(path string) []Target {
	switch Kind(path) {
	case "image":
		own := normFormat(lowerExt(path))
		var out []Target
		for _, t := range imageTargets {
			if normFormat(t.ID) != own {
				out = append(out, t)
			}
		}
		return out
	case "pdf":
		return []Target{
			{"png", ".png", "PNG", "Una imagen por página, con la máxima nitidez"},
			{"jpg", ".jpg", "JPG", "Una imagen por página, más ligera"},
		}
	case "office":
		return []Target{{"pdf", ".pdf", "PDF", "Se ve igual en cualquier equipo y no se puede editar fácilmente"}}
	}
	return nil
}

// ConvertOptions ajusta una conversión.
type ConvertOptions struct {
	Quality  int    `json:"quality"`  // JPG: 1–100 (0 = 90)
	MaxSide  int    `json:"maxSide"`  // imágenes: lado mayor máximo (0 = sin cambio)
	PageSize string `json:"pageSize"` // imagen → PDF: "a4" (defecto), "letter" o "image"
	DPI      int    `json:"dpi"`      // PDF → imágenes: puntos por pulgada (0 = 150)
}

// Convert convierte un archivo al formato target (un ID de Targets) y lo
// guarda junto al original sin pisar nada. Para PDF con varias páginas,
// Result.Out es la carpeta con las imágenes.
func Convert(ctx context.Context, path, target string, opts ConvertOptions, progress Progress) (Result, error) {
	switch Kind(path) {
	case "image":
		return ConvertImage(ctx, path, target, opts, progress)
	case "pdf":
		return PDFToImages(ctx, path, target, opts.DPI, progress)
	case "office":
		if target != "pdf" {
			return Result{}, uerr(nil, "Este documento solo se puede convertir a PDF")
		}
		progress.set(0.05)
		r, err := OfficeToPDF(ctx, path, "")
		if err == nil {
			progress.set(1)
		}
		return r, err
	}
	return Result{}, uerr(nil, "Este tipo de archivo no se puede convertir")
}

// ConvertImage convierte una imagen a jpg, png, gif, bmp, tiff, ico o pdf.
// Al pasar transparencia a JPG (o BMP/PDF con foto) se pone fondo blanco.
func ConvertImage(ctx context.Context, path, target string, opts ConvertOptions, progress Progress) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	format := normFormat(target)
	if format == "pdf" {
		return ImagesToPDF(ctx, []string{path}, PDFOptions{PageSize: opts.PageSize}, progress)
	}
	switch format {
	case "jpeg", "png", "gif", "bmp", "tiff", "ico":
	default:
		return Result{}, uerr(nil, "No se puede convertir a %s", formatName(format))
	}
	progress.set(0.05)
	d, err := loadImage(ctx, path)
	if err != nil {
		return Result{}, err
	}
	progress.set(0.4)
	var img image.Image = d.img
	if d.anim != nil {
		img = gifFrame(d.anim, 0)
	}
	img = normalize(img)
	b := img.Bounds()
	if w, h := fitSize(b.Dx(), b.Dy(), opts.MaxSide, 0); w != b.Dx() || h != b.Dy() {
		img = resizeImage(img, w, h)
	}
	q := opts.Quality
	if q <= 0 {
		q = 90
	}
	if format == "png" || format == "gif" {
		q = 100 // conversión sin pérdida (GIF: paleta exacta si cabe)
	}
	var data []byte
	var w, h int
	if d.anim != nil && format == "gif" {
		data, w, h, err = reduceAnimGIF(ctx, d.anim, ImageOptions{Quality: 100, MaxSide: opts.MaxSide})
	} else {
		var e *encoded
		e, err = encodeImage(ctx, d, img, format, q, 0)
		if e != nil {
			data, w, h = e.data, e.w, e.h
		}
	}
	if err != nil {
		return Result{}, cleanErr(ctx, err, "No se ha podido convertir la imagen")
	}
	progress.set(0.9)
	out, err := writeOut(path, "", extFor(format), data)
	if err != nil {
		return Result{}, err
	}
	progress.set(1)
	return Result{Out: out, Before: d.size, After: int64(len(data)), Width: w, Height: h}, nil
}
