package docs

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
	"golang.org/x/image/webp"
)

// decoded es una imagen ya abierta (y orientada) lista para procesar.
type decoded struct {
	key     string
	path    string
	format  string      // jpeg, png, gif, bmp, tiff, webp o la extensión (heic, avif…)
	img     image.Image // YCbCr, Gray o NRGBA, ya girada según EXIF
	icc     []byte      // perfil de color del original, si lo hay
	size    int64       // bytes del archivo
	alpha   bool        // tiene píxeles transparentes
	raw     []byte      // bytes del JPEG original (para meterlo en PDF sin recodificar)
	rawOK   bool        // raw se puede incrustar tal cual (sin girar, YCbCr/gris)
	anim    *gif.GIF    // GIF animado (varios fotogramas)
	viaWIC  bool
	jpegSub image.YCbCrSubsampleRatio
}

// Formatos que Go lee directamente.
var nativeFormats = map[string]bool{"jpeg": true, "png": true, "gif": true, "bmp": true, "tiff": true, "webp": true}

// sniff reconoce el formato por los primeros bytes.
func sniff(h []byte) string {
	switch {
	case len(h) >= 3 && h[0] == 0xFF && h[1] == 0xD8 && h[2] == 0xFF:
		return "jpeg"
	case bytes.HasPrefix(h, []byte("\x89PNG\r\n\x1a\n")):
		return "png"
	case bytes.HasPrefix(h, []byte("GIF8")):
		return "gif"
	case bytes.HasPrefix(h, []byte("BM")):
		return "bmp"
	case bytes.HasPrefix(h, []byte("II*\x00")), bytes.HasPrefix(h, []byte("MM\x00*")):
		return "tiff"
	case len(h) >= 12 && string(h[0:4]) == "RIFF" && string(h[8:12]) == "WEBP":
		return "webp"
	case len(h) >= 12 && string(h[4:8]) == "ftyp":
		brand := string(h[8:12])
		if strings.HasPrefix(brand, "avi") {
			return "avif"
		}
		return "heic"
	case len(h) >= 2 && h[0] == 0xFF && h[1] == 0x0A, bytes.HasPrefix(h, []byte("\x00\x00\x00\x0cJXL ")):
		return "jxl"
	}
	return ""
}

func fileKey(path string) (string, int64, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", 0, err
	}
	if st.IsDir() {
		return "", 0, uerr(nil, "Es una carpeta, no una imagen")
	}
	abs, _ := filepath.Abs(path)
	return strings.ToLower(abs) + "|" + strconv.FormatInt(st.ModTime().UnixNano(), 10) + "|" + strconv.FormatInt(st.Size(), 10), st.Size(), nil
}

// Caché de las dos últimas imágenes abiertas (para la vista previa).
var imgCache struct {
	sync.Mutex
	items []*decoded
}

// loadImage abre una imagen (con caché por ruta + fecha + tamaño).
func loadImage(ctx context.Context, path string) (*decoded, error) {
	key, size, err := fileKey(path)
	if err != nil {
		return nil, cleanErr(ctx, err, "No se puede abrir la imagen")
	}
	imgCache.Lock()
	for _, d := range imgCache.items {
		if d.key == key {
			imgCache.Unlock()
			return d, nil
		}
	}
	imgCache.Unlock()
	d, err := decodeFile(ctx, path)
	if err != nil {
		return nil, err
	}
	d.key, d.size, d.path = key, size, path
	imgCache.Lock()
	imgCache.items = append([]*decoded{d}, imgCache.items...)
	if len(imgCache.items) > 2 {
		imgCache.items = imgCache.items[:2]
	}
	imgCache.Unlock()
	return d, nil
}

func decodeFile(ctx context.Context, path string) (*decoded, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, cleanErr(ctx, err, "No se puede leer la imagen")
	}
	f := sniff(data[:min(len(data), 32)])
	if f == "" {
		f = lowerExt(path)
		if f == "jpg" || f == "jpe" || f == "jfif" {
			f = "jpeg"
		}
		if f == "tif" {
			f = "tiff"
		}
	}
	if nativeFormats[f] {
		d, err := decodeNative(data, f)
		if err == nil {
			return d, nil
		}
		// Variantes que Go no entiende (p. ej. TIFF raros): se intenta con Windows.
		if d2, err2 := decodeWIC(ctx, path, f); err2 == nil {
			return d2, nil
		} else if err2 == ErrCanceled {
			return nil, err2
		}
		return nil, uerr(err, "La imagen está dañada o no se puede leer")
	}
	return decodeWIC(ctx, path, f)
}

func decodeNative(data []byte, f string) (*decoded, error) {
	d := &decoded{format: f}
	r := bytes.NewReader(data)
	var img image.Image
	var err error
	orientation := 1
	switch f {
	case "jpeg":
		var meta jpegMeta
		meta = parseJPEGMeta(data)
		orientation = meta.orientation
		d.icc = meta.icc
		img, err = jpeg.Decode(r)
		if err == nil {
			d.raw = data
			switch m := img.(type) {
			case *image.YCbCr:
				d.jpegSub = m.SubsampleRatio
				d.rawOK = orientation <= 1
			case *image.Gray:
				d.rawOK = orientation <= 1
			}
		}
	case "png":
		d.icc = pngICC(data)
		img, err = png.Decode(r)
	case "gif":
		var g *gif.GIF
		g, err = gif.DecodeAll(r)
		if err == nil {
			if len(g.Image) > 1 {
				d.anim = g
			}
			img = gifFrame(g, 0)
		}
	case "bmp":
		img, err = bmp.Decode(r)
	case "tiff":
		img, err = tiff.Decode(r)
	case "webp":
		img, err = webp.Decode(r)
	default:
		err = fmt.Errorf("formato %s", f)
	}
	if err != nil {
		return nil, err
	}
	img = normalize(img)
	img = orient(img, orientation)
	d.img = img
	d.alpha = hasAlpha(img)
	return d, nil
}

// gifFrame compone el fotograma i del GIF sobre el lienzo completo.
func gifFrame(g *gif.GIF, n int) image.Image {
	if len(g.Image) == 1 && g.Image[0].Bounds() == image.Rect(0, 0, g.Config.Width, g.Config.Height) {
		return g.Image[0]
	}
	w, h := g.Config.Width, g.Config.Height
	if w == 0 || h == 0 {
		b := g.Image[0].Bounds()
		w, h = b.Max.X, b.Max.Y
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i <= n && i < len(g.Image); i++ {
		drawOver(canvas, g.Image[i])
	}
	return canvas
}

func drawOver(dst *image.NRGBA, src *image.Paletted) {
	b := src.Bounds().Intersect(dst.Rect)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := src.Palette[src.Pix[src.PixOffset(x, y)]]
			r, g, bb, a := c.RGBA()
			if a == 0 {
				continue
			}
			i := dst.PixOffset(x, y)
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = uint8(r>>8), uint8(g>>8), uint8(bb>>8), uint8(a>>8)
		}
	}
}

type jpegMeta struct {
	orientation int
	icc         []byte
	cmyk        bool
}

// parseJPEGMeta lee la orientación EXIF y el perfil ICC de un JPEG.
func parseJPEGMeta(data []byte) jpegMeta {
	m := jpegMeta{orientation: 1}
	iccParts := map[int][]byte{}
	iccCount := 0
	i := 2
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			break
		}
		marker := data[i+1]
		if marker == 0xFF {
			i++
			continue
		}
		if marker == 0xD8 || (marker >= 0xD0 && marker <= 0xD7) || marker == 0x01 {
			i += 2
			continue
		}
		if marker == 0xDA || marker == 0xD9 {
			break
		}
		l := int(binary.BigEndian.Uint16(data[i+2:]))
		if l < 2 || i+2+l > len(data) {
			break
		}
		seg := data[i+4 : i+2+l]
		switch {
		case marker == 0xE1 && bytes.HasPrefix(seg, []byte("Exif\x00\x00")):
			if o := exifOrientation(seg[6:]); o > 0 {
				m.orientation = o
			}
		case marker == 0xE2 && bytes.HasPrefix(seg, []byte("ICC_PROFILE\x00")) && len(seg) > 14:
			iccParts[int(seg[12])] = seg[14:]
			iccCount = int(seg[13])
		case marker == 0xEE && bytes.HasPrefix(seg, []byte("Adobe")):
			m.cmyk = true
		}
		i += 2 + l
	}
	if iccCount > 0 && len(iccParts) == iccCount {
		for k := 1; k <= iccCount; k++ {
			m.icc = append(m.icc, iccParts[k]...)
		}
	}
	return m
}

func exifOrientation(t []byte) int {
	if len(t) < 8 {
		return 0
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0
	}
	off := int(bo.Uint32(t[4:]))
	if off+2 > len(t) {
		return 0
	}
	n := int(bo.Uint16(t[off:]))
	for k := range n {
		e := off + 2 + k*12
		if e+12 > len(t) {
			return 0
		}
		if bo.Uint16(t[e:]) == 0x0112 {
			v := int(bo.Uint16(t[e+8:]))
			if v >= 1 && v <= 8 {
				return v
			}
			return 0
		}
	}
	return 0
}

// pngICC extrae el perfil ICC (chunk iCCP) de un PNG.
func pngICC(data []byte) []byte {
	i := 8
	for i+8 <= len(data) {
		l := int(binary.BigEndian.Uint32(data[i:]))
		typ := string(data[i+4 : i+8])
		if i+12+l > len(data) || l < 0 {
			return nil
		}
		if typ == "iCCP" {
			c := data[i+8 : i+8+l]
			z := bytes.IndexByte(c, 0)
			if z < 0 || z+2 > len(c) {
				return nil
			}
			zr, err := zlibReader(c[z+2:])
			if err != nil {
				return nil
			}
			icc, err := io.ReadAll(io.LimitReader(zr, 4<<20))
			if err != nil {
				return nil
			}
			return icc
		}
		if typ == "IDAT" || typ == "IEND" {
			return nil
		}
		i += 12 + l
	}
	return nil
}

// Decodificación con los códecs de Windows (HEIC, AVIF, RAW, JXL…).
const psDecodeImage = `
$null = [Windows.Graphics.Imaging.BitmapDecoder, Windows.Graphics.Imaging, ContentType = WindowsRuntime]
$null = [Windows.Storage.Streams.Buffer, Windows.Storage.Streams, ContentType = WindowsRuntime]
$fs = [IO.File]::OpenRead($in)
try {
  try { $dec = AwaitOp ([Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync((RAS $fs))) ([Windows.Graphics.Imaging.BitmapDecoder]) }
  catch {
    $known = [Windows.Graphics.Imaging.BitmapDecoder]::GetDecoderInformationEnumerator() | ? { $_.FileExtensions -contains $ext }
    if ($known) { 'ERR damaged' } else { 'ERR nocodec' }
    exit 3
  }
  $tr = New-Object Windows.Graphics.Imaging.BitmapTransform
  $sb = AwaitOp ($dec.GetSoftwareBitmapAsync([Windows.Graphics.Imaging.BitmapPixelFormat]::Bgra8, [Windows.Graphics.Imaging.BitmapAlphaMode]::Straight, $tr, [Windows.Graphics.Imaging.ExifOrientationMode]::RespectExifOrientation, [Windows.Graphics.Imaging.ColorManagementMode]::ColorManageToSRgb)) ([Windows.Graphics.Imaging.SoftwareBitmap])
  $w = $sb.PixelWidth; $h = $sb.PixelHeight
  $buf = New-Object Windows.Storage.Streams.Buffer ([uint32]($w * $h * 4))
  $sb.CopyToBuffer($buf)
  $src = [System.Runtime.InteropServices.WindowsRuntime.WindowsRuntimeBufferExtensions]::AsStream($buf)
  $dst = [IO.File]::Create($out)
  try { $src.CopyTo($dst, 4194304) } finally { $dst.Dispose(); $src.Dispose() }
  "SIZE $w $h"
} finally { $fs.Dispose() }
`

func decodeWIC(ctx context.Context, path, f string) (*decoded, error) {
	tmp, err := os.CreateTemp("", "turbo-img-*.raw")
	if err != nil {
		return nil, cleanErr(ctx, err, "No se puede abrir la imagen")
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	script := psWinRT + "$in = " + psQuote(path) + "\n$out = " + psQuote(tmp.Name()) +
		"\n$ext = " + psQuote(strings.ToLower(filepath.Ext(path))) + "\n" + psDecodeImage
	w, h := 0, 0
	err = runPS(ctx, script, func(line string) {
		if strings.HasPrefix(line, "SIZE ") {
			fmt.Sscanf(line, "SIZE %d %d", &w, &h)
		}
	})
	if err == ErrCanceled {
		return nil, err
	}
	if err != nil || w == 0 || h == 0 {
		if strings.Contains(psMessage(err), "damaged") {
			return nil, uerr(err, "La imagen está dañada o no se puede leer")
		}
		if strings.Contains(psMessage(err), "nocodec") || err == nil {
			return nil, noCodecErr(f)
		}
		return nil, uerr(err, "Windows no ha podido abrir esta imagen")
	}
	raw, err := os.ReadFile(tmp.Name())
	if err != nil || len(raw) < w*h*4 {
		return nil, uerr(err, "Windows no ha podido abrir esta imagen")
	}
	// BGRA -> RGBA
	parallel(h, func(lo, hi int) {
		for i := lo * w * 4; i < hi*w*4; i += 4 {
			raw[i], raw[i+2] = raw[i+2], raw[i]
		}
	})
	m := &image.NRGBA{Pix: raw[:w*h*4], Stride: w * 4, Rect: image.Rect(0, 0, w, h)}
	d := &decoded{format: f, viaWIC: true}
	if isOpaque(m) {
		d.img = m
	} else {
		d.img = m
		d.alpha = true
	}
	return d, nil
}

func noCodecErr(f string) error {
	switch f {
	case "heic", "heif":
		return uerr(nil, "Windows no sabe abrir este tipo de imagen; instala la «Extensión de imágenes HEIF» de Microsoft Store")
	case "avif":
		return uerr(nil, "Windows no sabe abrir este tipo de imagen; instala la «Extensión de vídeo AV1» de Microsoft Store")
	case "jxl":
		return uerr(nil, "Windows no sabe abrir este tipo de imagen; instala la «Extensión JPEG XL» de Microsoft Store")
	case "webp":
		return uerr(nil, "Windows no sabe abrir este tipo de imagen; instala la «Extensión de imágenes WebP» de Microsoft Store")
	}
	return uerr(nil, "Windows no sabe abrir este tipo de imagen")
}
