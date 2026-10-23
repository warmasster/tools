//go:build ignore

// Hace build/windows/icon.ico y build/appicon.png a partir de appsheet.png.
// Uso (desde turbomenu/): go run dev/icons/genapp.go
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/draw"
	"image/png"
	"os"
)

func main() {
	f, _ := os.Open("dev/icons/appsheet.png")
	sheet, err := png.Decode(f)
	if err != nil {
		panic(err)
	}
	sizes := []int{16, 24, 32, 48, 64, 128, 256}
	var imgs [][]byte
	for i, s := range sizes {
		dst := image.NewNRGBA(image.Rect(0, 0, s, s))
		draw.Draw(dst, dst.Bounds(), sheet, image.Pt(i*256, 0), draw.Src)
		var buf bytes.Buffer
		png.Encode(&buf, dst)
		imgs = append(imgs, buf.Bytes())
		if s == 256 {
			os.WriteFile("build/appicon.png", buf.Bytes(), 0o644)
		}
	}
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, [3]uint16{0, 1, uint16(len(imgs))})
	off := 6 + 16*len(imgs)
	for i, data := range imgs {
		w := byte(sizes[i])
		if sizes[i] >= 256 {
			w = 0
		}
		out.Write([]byte{w, w, 0, 0})
		binary.Write(&out, binary.LittleEndian, [2]uint16{1, 32})
		binary.Write(&out, binary.LittleEndian, [2]uint32{uint32(len(data)), uint32(off)})
		off += len(data)
	}
	for _, d := range imgs {
		out.Write(d)
	}
	os.WriteFile("build/windows/icon.ico", out.Bytes(), 0o644)
}
