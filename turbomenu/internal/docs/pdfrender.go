package docs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// Conversión de PDF a imágenes con el motor de PDF de Windows (Windows.Data.Pdf).

const psRenderPDF = `
$null = [Windows.Data.Pdf.PdfDocument, Windows.Data.Pdf, ContentType = WindowsRuntime]
$null = [Windows.Graphics.Imaging.BitmapEncoder, Windows.Graphics.Imaging, ContentType = WindowsRuntime]
$fs = [IO.File]::OpenRead($in)
try {
  try { $doc = AwaitOp ([Windows.Data.Pdf.PdfDocument]::LoadFromStreamAsync((RAS $fs))) ([Windows.Data.Pdf.PdfDocument]) }
  catch {
    $e = $_.Exception; while ($e.InnerException) { $e = $e.InnerException }
    if ($e.HResult -eq -2147023573) { 'ERR password' } else { 'ERR open ' + $e.HResult }
    exit 3
  }
  "PAGES $($doc.PageCount)"
  for ($i = 0; $i -lt $doc.PageCount; $i++) {
    $page = $doc.GetPage($i)
    try {
      $opt = New-Object Windows.Data.Pdf.PdfPageRenderOptions
      $w = [math]::Max(1, [math]::Round($page.Size.Width * $dpi / 96))
      $h = [math]::Max(1, [math]::Round($page.Size.Height * $dpi / 96))
      $f = [math]::Min(1, $maxpx / [math]::Max($w, $h))
      $opt.DestinationWidth = [uint32][math]::Max(1, [math]::Round($w * $f))
      $opt.DestinationHeight = [uint32][math]::Max(1, [math]::Round($h * $f))
      if ($jpg) { $opt.BitmapEncoderId = [Windows.Graphics.Imaging.BitmapEncoder]::JpegEncoderId }
      if ($single) { $path = $single } else { $path = Join-Path $dir ($base + ' - página ' + ($i + 1).ToString('D' + $digits) + $ext) }
      $o = [IO.File]::Create($path)
      try {
        $ras = RAS $o
        AwaitAct ($page.RenderToStreamAsync($ras, $opt))
        $ras.Dispose()
      } finally { $o.Dispose() }
    } finally { $page.Dispose() }
    "PAGE $($i + 1)"
  }
} finally { $fs.Dispose() }
`

// PDFToImages convierte cada página en una imagen (format "png" o "jpg") a
// dpi puntos por pulgada (0 = 150). Con una sola página se guarda «doc.png»
// junto al PDF; con varias, en la carpeta nueva «doc (imágenes)», y
// Result.Out es esa carpeta.
func PDFToImages(ctx context.Context, path, format string, dpi int, progress Progress) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	format = normFormat(format)
	if format != "png" && format != "jpeg" {
		return Result{}, uerr(nil, "Formato de imagen no válido")
	}
	if dpi <= 0 {
		dpi = 150
	}
	dpi = min(max(dpi, 36), 600)
	// Nº de páginas (rápido, sin validar).
	pages := 0
	if pc, err := readPDF(ctx, path, pdfConf(model.VALIDATE), false); err == nil {
		if pc.EnsurePageCount() == nil {
			pages = pc.PageCount
		}
	} else if err == ErrCanceled {
		return Result{}, err
	}
	ext := extFor(format)
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	var dir, singleOut string
	if pages == 1 {
		o, err := createOut(path, "", ext)
		if err != nil {
			return Result{}, err
		}
		o.f.Close()
		os.Remove(o.Tmp)
		singleOut = o.Final
	} else {
		var err error
		dir, err = makeOutDir(path, "imágenes")
		if err != nil {
			return Result{}, err
		}
	}
	digits := 3
	if pages > 1 {
		digits = len(strconv.Itoa(pages))
	}
	cleanup := func() {
		if dir != "" {
			os.RemoveAll(dir)
		} else {
			os.Remove(singleOut)
		}
	}
	var sb strings.Builder
	sb.WriteString(psWinRT)
	sb.WriteString("$in = " + psQuote(path) + "\n")
	fmt.Fprintf(&sb, "$dpi = %d\n$maxpx = 12000\n$digits = %d\n", dpi, digits)
	sb.WriteString("$jpg = $" + strconv.FormatBool(format == "jpeg") + "\n")
	sb.WriteString("$single = " + psQuote(singleOut) + "\n")
	sb.WriteString("$dir = " + psQuote(dir) + "\n$base = " + psQuote(base) + "\n$ext = " + psQuote(ext) + "\n")
	sb.WriteString(psRenderPDF)
	total := max(pages, 1)
	done := 0
	err := runPS(ctx, sb.String(), func(line string) {
		switch {
		case strings.HasPrefix(line, "PAGES "):
			if n, err := strconv.Atoi(strings.TrimPrefix(line, "PAGES ")); err == nil && n > 0 {
				total = n
			}
		case strings.HasPrefix(line, "PAGE "):
			done++
			progress.set(float64(done) / float64(total))
		}
	})
	if err != nil {
		cleanup()
		if err == ErrCanceled {
			return Result{}, err
		}
		msg := psMessage(err)
		if strings.Contains(msg, "password") {
			return Result{}, uerr(err, "El PDF está protegido con contraseña")
		}
		if strings.HasPrefix(msg, "open") {
			return Result{}, uerr(err, "Windows no puede abrir este PDF (¿está dañado?)")
		}
		return Result{}, uerr(err, "No se han podido convertir las páginas")
	}
	res := Result{Before: fileSize(path)}
	if singleOut != "" {
		res.Out = singleOut
		res.After = fileSize(singleOut)
	} else {
		res.Out = dir
		if ents, err := os.ReadDir(dir); err == nil {
			for _, e := range ents {
				if inf, err := e.Info(); err == nil {
					res.After += inf.Size()
				}
			}
		}
	}
	progress.set(1)
	return res, nil
}
