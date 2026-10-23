//go:build ignore

// Recorta sheet.png en un .ico por icono (PNG dentro, varios tamaños) en menuicons/.
// Uso (desde turbomenu/): go run dev/icons/gen.go
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

func main() {
	var lay struct {
		IDs   []string `json:"ids"`
		Sizes []int    `json:"sizes"`
		Cell  int      `json:"cell"`
	}
	b, _ := os.ReadFile("dev/icons/layout.json")
	json.Unmarshal(b, &lay)
	f, err := os.Open("dev/icons/sheet.png")
	if err != nil {
		panic(err)
	}
	sheet, err := png.Decode(f)
	if err != nil {
		panic(err)
	}
	os.MkdirAll("menuicons", 0o755)
	for r, id := range lay.IDs {
		var imgs [][]byte
		for c, s := range lay.Sizes {
			dst := image.NewNRGBA(image.Rect(0, 0, s, s))
			draw.Draw(dst, dst.Bounds(), sheet, image.Pt(c*lay.Cell, r*lay.Cell), draw.Src)
			var buf bytes.Buffer
			png.Encode(&buf, dst)
			imgs = append(imgs, buf.Bytes())
		}
		var out bytes.Buffer
		binary.Write(&out, binary.LittleEndian, [3]uint16{0, 1, uint16(len(imgs))})
		off := 6 + 16*len(imgs)
		for i, data := range imgs {
			s := lay.Sizes[i]
			w := byte(s)
			if s >= 256 {
				w = 0
			}
			out.Write([]byte{w, w, 0, 0})
			binary.Write(&out, binary.LittleEndian, uint16(1))
			binary.Write(&out, binary.LittleEndian, uint16(32))
			binary.Write(&out, binary.LittleEndian, uint32(len(data)))
			binary.Write(&out, binary.LittleEndian, uint32(off))
			off += len(data)
		}
		for _, data := range imgs {
			out.Write(data)
		}
		os.WriteFile(filepath.Join("menuicons", id+".ico"), out.Bytes(), 0o644)
	}
}
