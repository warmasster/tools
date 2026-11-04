// Genera los iconos de TurboMON (un velocímetro con el degradado de las Turbo):
//
//	go run ./dev/icon
//
// Deja build/appicon.png, build/windows/icon.ico y los de la bandeja en assets/.
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

type style struct {
	c1, c2, c3 [3]float64 // degradado
	needle     [3]float64
	track      float64 // opacidad del arco de fondo
	progress   float64 // 0-1, cuánto marca la aguja
	badge      bool    // distintivo de «experimental»
}

// flask es el matraz del distintivo, en la caja de 64.
var flask = [][2]float64{{46.6, 8}, {51.4, 8}, {51.4, 12.3}, {55.2, 20.5}, {42.8, 20.5}, {46.6, 12.3}}

// badgeOnly dibuja solo el distintivo de «experimental» (para reutilizarlo en
// otras herramientas: dev/icon/experimental.png y experimental.svg).
func badgeOnly(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const ss = 4
	k := 27.2 / float64(size) // la caja del distintivo: 27,2 unidades alrededor de (49, 15)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var acc [4]float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					px := 49 - 13.6 + (float64(x)+(float64(sx)+.5)/ss)*k
					py := 15 - 13.6 + (float64(y)+(float64(sy)+.5)/ss)*k
					bd := math.Hypot(px-49, py-15)
					if bd > 13.6 {
						continue
					}
					c := [3]float64{0x0a, 0x0b, 0x12}
					if bd <= 12 {
						c = [3]float64{0xfb, 0xbf, 0x24}
						if inPoly(px, py, flask) {
							c = [3]float64{0x3b, 0x23, 0x00}
						}
					}
					acc[0] += c[0]
					acc[1] += c[1]
					acc[2] += c[2]
					acc[3]++
				}
			}
			if acc[3] > 0 {
				n := acc[3]
				img.SetNRGBA(x, y, color.NRGBA{uint8(acc[0] / n), uint8(acc[1] / n), uint8(acc[2] / n), uint8(255 * n / (ss * ss))})
			}
		}
	}
	return img
}

func inPoly(x, y float64, p [][2]float64) bool {
	in := false
	for i, j := 0, len(p)-1; i < len(p); j, i = i, i+1 {
		if (p[i][1] > y) != (p[j][1] > y) && x < (p[j][0]-p[i][0])*(y-p[i][1])/(p[j][1]-p[i][1])+p[i][0] {
			in = !in
		}
	}
	return in
}

var (
	normal = style{c1: rgb(0x8b5cf6), c2: rgb(0x6366f1), c3: rgb(0x06b6d4), needle: rgb(0xa5f3fc), track: .35, progress: .7}
	off    = style{c1: rgb(0x5b5f73), c2: rgb(0x4a4e60), c3: rgb(0x3b3f4f), needle: rgb(0xd0d3de), track: .3, progress: .25}
)

func rgb(v int) [3]float64 {
	return [3]float64{float64(v >> 16 & 255), float64(v >> 8 & 255), float64(v & 255)}
}

// render dibuja el icono a size px con 4×4 muestras por píxel. El diseño está en una caja de 64.
func render(size int, st style) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const ss = 4
	k := 64 / float64(size)
	cx, cy, r := 32.0, 40.0, 19.0
	w := 5.5
	if size <= 20 {
		w = 7 // trazos más gruesos para que se vea en la bandeja
	}
	endA := math.Pi * (1 - st.progress) // ángulo de la punta (π = izquierda, 0 = derecha)
	tip := [2]float64{cx + r*math.Cos(endA), cy - r*math.Sin(endA)}
	needleEnd := [2]float64{cx + (r-7)*math.Cos(endA), cy - (r-7)*math.Sin(endA)}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var acc [4]float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					px := (float64(x) + (float64(sx)+.5)/ss) * k
					py := (float64(y) + (float64(sy)+.5)/ss) * k
					if !inRoundRect(px, py, 2, 2, 62, 62, 15) {
						continue
					}
					// Distintivo de «experimental»: un matraz en un círculo ámbar
					// (no en la bandeja: ahí no se vería).
					if st.badge && size >= 24 {
						bd := math.Hypot(px-49, py-15)
						if bd <= 13.6 {
							bc := [3]float64{0x0a, 0x0b, 0x12}
							if bd <= 12 {
								bc = [3]float64{0xfb, 0xbf, 0x24}
								if inPoly(px, py, flask) {
									bc = [3]float64{0x3b, 0x23, 0x00}
								}
							}
							acc[0] += bc[0]
							acc[1] += bc[1]
							acc[2] += bc[2]
							acc[3]++
							continue
						}
					}
					t := (px + py) / 128
					var c [3]float64
					if t < .5 {
						c = mix(st.c1, st.c2, t*2)
					} else {
						c = mix(st.c2, st.c3, (t-.5)*2)
					}
					// arco de fondo (semicírculo de arriba)
					dx, dy := px-cx, py-cy
					d := math.Hypot(dx, dy)
					ang := math.Atan2(-dy, dx)
					onArc := math.Abs(d-r) <= w/2 && ang >= 0 && ang <= math.Pi
					capL := math.Hypot(px-(cx-r), py-cy) <= w/2
					capR := math.Hypot(px-(cx+r), py-cy) <= w/2
					if onArc || capL || capR {
						c = mix(c, [3]float64{255, 255, 255}, st.track)
					}
					// arco hasta la aguja
					if (math.Abs(d-r) <= w/2 && ang >= endA && ang <= math.Pi) || capL || math.Hypot(px-tip[0], py-tip[1]) <= w/2 {
						c = [3]float64{255, 255, 255}
					}
					if segDist(px, py, cx, cy, needleEnd[0], needleEnd[1]) <= w*0.42 {
						c = st.needle
					}
					if d <= w*0.85 {
						c = [3]float64{255, 255, 255}
					}
					acc[0] += c[0]
					acc[1] += c[1]
					acc[2] += c[2]
					acc[3]++
				}
			}
			if acc[3] == 0 {
				continue
			}
			n := acc[3]
			img.SetNRGBA(x, y, color.NRGBA{uint8(acc[0] / n), uint8(acc[1] / n), uint8(acc[2] / n), uint8(255 * n / (ss * ss))})
		}
	}
	return img
}

func mix(a, b [3]float64, t float64) [3]float64 {
	return [3]float64{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t, a[2] + (b[2]-a[2])*t}
}

func inRoundRect(x, y, x0, y0, x1, y1, rad float64) bool {
	if x < x0 || x > x1 || y < y0 || y > y1 {
		return false
	}
	cx := math.Max(x0+rad, math.Min(x, x1-rad))
	cy := math.Max(y0+rad, math.Min(y, y1-rad))
	return math.Hypot(x-cx, y-cy) <= rad
}

func segDist(px, py, ax, ay, bx, by float64) float64 {
	vx, vy := bx-ax, by-ay
	t := ((px-ax)*vx + (py-ay)*vy) / (vx*vx + vy*vy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(px-(ax+t*vx), py-(ay+t*vy))
}

func pngBytes(img image.Image) []byte {
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

// ico junta varias imágenes PNG en un .ico (Windows lo admite desde Vista).
func ico(st style, sizes ...int) []byte {
	var imgs [][]byte
	for _, s := range sizes {
		imgs = append(imgs, pngBytes(render(s, st)))
	}
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	off := 6 + 16*len(sizes)
	for i, s := range sizes {
		w := byte(s)
		if s >= 256 {
			w = 0
		}
		b.Write([]byte{w, w, 0, 0})
		binary.Write(&b, binary.LittleEndian, [2]uint16{1, 32})
		binary.Write(&b, binary.LittleEndian, [2]uint32{uint32(len(imgs[i])), uint32(off)})
		off += len(imgs[i])
	}
	for _, im := range imgs {
		b.Write(im)
	}
	return b.Bytes()
}

func write(p string, b []byte) {
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		panic(err)
	}
}

func main() {
	write("dev/icon/experimental.png", pngBytes(badgeOnly(256)))
	app := normal
	app.badge = true // mientras TurboMON sea experimental
	write("build/appicon.png", pngBytes(render(512, app)))
	write("build/windows/icon.ico", ico(app, 16, 20, 24, 32, 40, 48, 64, 128, 256))
	write("assets/tray-on.ico", ico(normal, 16, 20, 24, 32, 40, 48))
	write("assets/tray-off.ico", ico(off, 16, 20, 24, 32, 40, 48))
}
