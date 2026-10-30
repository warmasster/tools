package main

// Retoques que pone el PC al vídeo H.264: espejo, brillo, contraste y saturación.
// Así el móvil manda el fotograma de la cámara tal cual (solo recortado, sin copiar
// nada) en vez de repintarlo en un lienzo, que en un móvil flojo se come el hilo de
// la página y hace perder fotogramas de la cámara. Aquí cuesta menos de 1 ms a 720p.
// El JPEG ya llega retocado del móvil.

import "image"

// needsFX dice si hay algo que hacer.
func needsFX(a Adjust) bool {
	return a.Mirror || fxVal(a.Brightness) != 1 || fxVal(a.Contrast) != 1 || fxVal(a.Saturation) != 1
}

func fxVal(v float64) float64 {
	if v <= 0 {
		return 1
	}
	return v
}

// applyFX retoca m en su sitio.
func applyFX(m *image.YCbCr, a Adjust) {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	if a.Mirror {
		flipRows(m.Y, m.YStride, w, h)
		flipRows(m.Cb, m.CStride, w/2, h/2)
		flipRows(m.Cr, m.CStride, w/2, h/2)
	}
	b, c, s := fxVal(a.Brightness), fxVal(a.Contrast), fxVal(a.Saturation)
	if b == 1 && c == 1 && s == 1 {
		return
	}
	// Como los filtros de CSS que usa la vista previa del móvil, en este orden:
	// brillo (multiplica), contraste (alrededor del gris medio) y saturación.
	var ly, lc [256]byte
	for i := range 256 {
		y := (float64(i) - 16) / 219
		y = (y*b-0.5)*c + 0.5
		ly[i] = clamp8(y*219 + 16)
		d := (float64(i) - 128) * b * c * s
		lc[i] = clamp8(d + 128)
	}
	lut(m.Y, m.YStride, w, h, &ly)
	lut(m.Cb, m.CStride, w/2, h/2, &lc)
	lut(m.Cr, m.CStride, w/2, h/2, &lc)
}

func flipRows(p []byte, stride, w, h int) {
	for y := range h {
		row := p[y*stride : y*stride+w]
		for i, j := 0, w-1; i < j; i, j = i+1, j-1 {
			row[i], row[j] = row[j], row[i]
		}
	}
}

func lut(p []byte, stride, w, h int, t *[256]byte) {
	for y := range h {
		row := p[y*stride : y*stride+w]
		for i, v := range row {
			row[i] = t[v]
		}
	}
}

func clamp8(v float64) byte {
	switch {
	case v < 0:
		return 0
	case v > 255:
		return 255
	}
	return byte(v + 0.5)
}
