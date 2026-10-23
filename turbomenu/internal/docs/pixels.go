package docs

import (
	"image"
	"image/color"
	"image/draw"
)

// Utilidades para pasar cualquier imagen a los tres tipos que usa el paquete:
// *image.YCbCr (fotos JPEG), *image.Gray y *image.NRGBA.

func compactGray(m *image.Gray) *image.Gray {
	if m.Rect.Min == (image.Point{}) {
		return m
	}
	off := m.PixOffset(m.Rect.Min.X, m.Rect.Min.Y)
	return &image.Gray{Pix: m.Pix[off:], Stride: m.Stride, Rect: image.Rect(0, 0, m.Rect.Dx(), m.Rect.Dy())}
}

func compactNRGBA(m *image.NRGBA) *image.NRGBA {
	if m.Rect.Min == (image.Point{}) {
		return m
	}
	off := m.PixOffset(m.Rect.Min.X, m.Rect.Min.Y)
	return &image.NRGBA{Pix: m.Pix[off:], Stride: m.Stride, Rect: image.Rect(0, 0, m.Rect.Dx(), m.Rect.Dy())}
}

func compactYCbCr(m *image.YCbCr) *image.YCbCr {
	if m.Rect.Min == (image.Point{}) {
		return m
	}
	// Caso raro (subimágenes): se copia a una imagen nueva.
	b := m.Rect
	out := image.NewYCbCr(image.Rect(0, 0, b.Dx(), b.Dy()), m.SubsampleRatio)
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.Y[out.YOffset(x, y)] = m.Y[m.YOffset(x+b.Min.X, y+b.Min.Y)]
			co, ci := out.COffset(x, y), m.COffset(x+b.Min.X, y+b.Min.Y)
			out.Cb[co], out.Cr[co] = m.Cb[ci], m.Cr[ci]
		}
	}
	return out
}

// isOpaque dice si todos los píxeles tienen alfa 255.
func isOpaque(m *image.NRGBA) bool {
	b := m.Rect
	w := b.Dx()
	h := b.Dy()
	res := make(chan bool, 1)
	res <- true
	parallel(h, func(lo, hi int) {
		for y := lo; y < hi; y++ {
			off := m.PixOffset(b.Min.X, b.Min.Y+y)
			row := m.Pix[off+3 : off+w*4 : off+w*4]
			for i := 0; i < len(row); i += 4 {
				if row[i] != 255 {
					select {
					case <-res:
						res <- false
					default:
					}
					return
				}
			}
		}
	})
	return <-res
}

// hasAlpha dice si la imagen tiene algún píxel no opaco.
func hasAlpha(img image.Image) bool {
	switch m := img.(type) {
	case *image.YCbCr, *image.Gray:
		return false
	case *image.NRGBA:
		return !isOpaque(m)
	}
	return !isOpaque(toNRGBA(img))
}

// toNRGBA convierte cualquier imagen a *image.NRGBA con origen en (0,0).
func toNRGBA(img image.Image) *image.NRGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	switch m := img.(type) {
	case *image.NRGBA:
		return compactNRGBA(m)
	case *image.RGBA:
		out := image.NewNRGBA(image.Rect(0, 0, w, h))
		parallel(h, func(lo, hi int) {
			for y := lo; y < hi; y++ {
				si := m.PixOffset(b.Min.X, b.Min.Y+y)
				copy(out.Pix[y*out.Stride:y*out.Stride+w*4], m.Pix[si:si+w*4])
			}
		})
		unpremultiply(out.Pix)
		return out
	case *image.YCbCr:
		out := image.NewNRGBA(image.Rect(0, 0, w, h))
		parallel(h, func(lo, hi int) {
			for y := lo; y < hi; y++ {
				d := out.Pix[y*out.Stride:]
				for x := 0; x < w; x++ {
					yi := m.YOffset(b.Min.X+x, b.Min.Y+y)
					ci := m.COffset(b.Min.X+x, b.Min.Y+y)
					r, g, bb := color.YCbCrToRGB(m.Y[yi], m.Cb[ci], m.Cr[ci])
					d[x*4], d[x*4+1], d[x*4+2], d[x*4+3] = r, g, bb, 255
				}
			}
		})
		return out
	case *image.Gray:
		out := image.NewNRGBA(image.Rect(0, 0, w, h))
		parallel(h, func(lo, hi int) {
			for y := lo; y < hi; y++ {
				s := m.Pix[m.PixOffset(b.Min.X, b.Min.Y+y):]
				d := out.Pix[y*out.Stride:]
				for x := 0; x < w; x++ {
					v := s[x]
					d[x*4], d[x*4+1], d[x*4+2], d[x*4+3] = v, v, v, 255
				}
			}
		})
		return out
	case *image.Paletted:
		pal := make([]color.NRGBA, 256)
		for i, c := range m.Palette {
			pal[i] = color.NRGBAModel.Convert(c).(color.NRGBA)
		}
		out := image.NewNRGBA(image.Rect(0, 0, w, h))
		parallel(h, func(lo, hi int) {
			for y := lo; y < hi; y++ {
				s := m.Pix[m.PixOffset(b.Min.X, b.Min.Y+y):]
				d := out.Pix[y*out.Stride:]
				for x := 0; x < w; x++ {
					c := pal[s[x]]
					d[x*4], d[x*4+1], d[x*4+2], d[x*4+3] = c.R, c.G, c.B, c.A
				}
			}
		})
		return out
	case *image.NYCbCrA:
		out := image.NewNRGBA(image.Rect(0, 0, w, h))
		parallel(h, func(lo, hi int) {
			for y := lo; y < hi; y++ {
				d := out.Pix[y*out.Stride:]
				for x := 0; x < w; x++ {
					yi := m.YOffset(b.Min.X+x, b.Min.Y+y)
					ci := m.COffset(b.Min.X+x, b.Min.Y+y)
					r, g, bb := color.YCbCrToRGB(m.Y[yi], m.Cb[ci], m.Cr[ci])
					d[x*4], d[x*4+1], d[x*4+2], d[x*4+3] = r, g, bb, m.A[m.AOffset(b.Min.X+x, b.Min.Y+y)]
				}
			}
		})
		return out
	}
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	parallel(h, func(lo, hi int) {
		r := image.Rect(0, lo, w, hi)
		draw.Draw(out, r, img, b.Min.Add(image.Pt(0, lo)), draw.Src)
	})
	return out
}

// normalize deja la imagen como YCbCr, Gray o NRGBA (origen en 0,0).
func normalize(img image.Image) image.Image {
	switch m := img.(type) {
	case *image.YCbCr:
		return compactYCbCr(m)
	case *image.Gray:
		return compactGray(m)
	case *image.NRGBA:
		return compactNRGBA(m)
	case *image.Gray16:
		b := m.Bounds()
		out := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				out.Pix[y*out.Stride+x] = m.Pix[m.PixOffset(b.Min.X+x, b.Min.Y+y)]
			}
		}
		return out
	case *image.Paletted:
		// Paleta solo de grises opacos -> Gray.
		gray := true
		for _, c := range m.Palette {
			r, g, bb, a := c.RGBA()
			if a != 0xffff || r != g || g != bb {
				gray = false
				break
			}
		}
		if gray {
			b := m.Bounds()
			out := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))
			lut := make([]uint8, 256)
			for i, c := range m.Palette {
				r, _, _, _ := c.RGBA()
				lut[i] = uint8(r >> 8)
			}
			for y := 0; y < b.Dy(); y++ {
				s := m.Pix[m.PixOffset(b.Min.X, b.Min.Y+y):]
				for x := 0; x < b.Dx(); x++ {
					out.Pix[y*out.Stride+x] = lut[s[x]]
				}
			}
			return out
		}
	}
	return toNRGBA(img)
}

// flatten compone sobre un fondo blanco (para formatos sin transparencia).
func flatten(img image.Image) image.Image {
	m, ok := img.(*image.NRGBA)
	if !ok || isOpaque(m) {
		return img
	}
	out := image.NewNRGBA(m.Rect)
	parallel(m.Rect.Dy(), func(lo, hi int) {
		for y := lo; y < hi; y++ {
			s := m.Pix[y*m.Stride : y*m.Stride+m.Rect.Dx()*4]
			d := out.Pix[y*out.Stride:]
			for i := 0; i < len(s); i += 4 {
				a := uint32(s[i+3])
				ia := 255 - a
				d[i] = uint8((uint32(s[i])*a + 255*ia + 127) / 255)
				d[i+1] = uint8((uint32(s[i+1])*a + 255*ia + 127) / 255)
				d[i+2] = uint8((uint32(s[i+2])*a + 255*ia + 127) / 255)
				d[i+3] = 255
			}
		}
	})
	return out
}

// orient aplica la orientación EXIF (1–8) a la imagen.
func orient(img image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return img
	}
	switch m := img.(type) {
	case *image.Gray:
		w, h := m.Rect.Dx(), m.Rect.Dy()
		p, nw, nh := orientPlane(m.Pix, w, h, m.Stride, 1, o)
		return &image.Gray{Pix: p, Stride: nw, Rect: image.Rect(0, 0, nw, nh)}
	case *image.NRGBA:
		w, h := m.Rect.Dx(), m.Rect.Dy()
		p, nw, nh := orientPlane(m.Pix, w, h, m.Stride, 4, o)
		return &image.NRGBA{Pix: p, Stride: nw * 4, Rect: image.Rect(0, 0, nw, nh)}
	case *image.YCbCr:
		ratio := m.SubsampleRatio
		swap := o >= 5
		if swap {
			switch ratio {
			case image.YCbCrSubsampleRatio422:
				ratio = image.YCbCrSubsampleRatio440
			case image.YCbCrSubsampleRatio440:
				ratio = image.YCbCrSubsampleRatio422
			case image.YCbCrSubsampleRatio444, image.YCbCrSubsampleRatio420:
			default:
				return orient(toNRGBA(m), o)
			}
		}
		w, h := m.Rect.Dx(), m.Rect.Dy()
		cw, ch := chromaSize(m)
		y, nw, nh := orientPlane(m.Y, w, h, m.YStride, 1, o)
		cb, ncw, _ := orientPlane(m.Cb, cw, ch, m.CStride, 1, o)
		cr, _, _ := orientPlane(m.Cr, cw, ch, m.CStride, 1, o)
		return &image.YCbCr{Y: y, Cb: cb, Cr: cr, YStride: nw, CStride: ncw, SubsampleRatio: ratio, Rect: image.Rect(0, 0, nw, nh)}
	}
	return orient(toNRGBA(img), o)
}

// orientPlane gira/voltea un plano de bpp bytes por píxel según EXIF.
func orientPlane(src []uint8, w, h, stride, bpp, o int) ([]uint8, int, int) {
	nw, nh := w, h
	if o >= 5 {
		nw, nh = h, w
	}
	dst := make([]uint8, nw*nh*bpp)
	parallel(h, func(lo, hi int) {
		for y := lo; y < hi; y++ {
			row := src[y*stride:]
			for x := 0; x < w; x++ {
				var dx, dy int
				switch o {
				case 2:
					dx, dy = w-1-x, y
				case 3:
					dx, dy = w-1-x, h-1-y
				case 4:
					dx, dy = x, h-1-y
				case 5:
					dx, dy = y, x
				case 6:
					dx, dy = h-1-y, x
				case 7:
					dx, dy = h-1-y, w-1-x
				case 8:
					dx, dy = y, w-1-x
				}
				di := (dy*nw + dx) * bpp
				si := x * bpp
				for k := 0; k < bpp; k++ {
					dst[di+k] = row[si+k]
				}
			}
		}
	})
	return dst, nw, nh
}
