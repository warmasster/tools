//go:build ignore

// Hace build/windows/icon.ico y build/appicon.png a partir de frontend/logo.svg.
// Dibuja el logo a cada tamaño con Edge sin ventana (así se ve igual que en la
// app) y junta los PNG en un .ico.
//
// Uso (desde turbogames/): go run dev/icons/genapp.go
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var sizes = []int{16, 24, 32, 48, 64, 128, 256}

func main() {
	svg, err := os.ReadFile("frontend/logo.svg")
	if err != nil {
		panic(err)
	}
	logo := strings.Replace(string(svg), "<svg ", `<svg width="100%" height="100%" `, 1)
	var html strings.Builder
	html.WriteString(`<!doctype html><html><head><style>html,body{margin:0;background:transparent}div{position:absolute;top:0}</style></head><body>`)
	for i, s := range sizes {
		fmt.Fprintf(&html, `<div style="left:%dpx;width:%dpx;height:%dpx">%s</div>`, i*256, s, s, logo)
	}
	html.WriteString(`</body></html>`)
	tmp, _ := filepath.Abs("dev/icons/appicon.html")
	if err := os.WriteFile(tmp, []byte(html.String()), 0o644); err != nil {
		panic(err)
	}
	sheetPath, _ := filepath.Abs("dev/icons/appsheet.png")
	edge := `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`
	prof, _ := os.MkdirTemp("", "tg-edge")
	defer os.RemoveAll(prof)
	cmd := exec.Command(edge, "--headless=new", "--disable-gpu", "--hide-scrollbars", "--force-device-scale-factor=1", "--user-data-dir="+prof,
		"--default-background-color=00000000", fmt.Sprintf("--window-size=%d,256", len(sizes)*256),
		"--screenshot="+sheetPath, "file:///"+filepath.ToSlash(tmp))
	if out, err := cmd.CombinedOutput(); err != nil {
		panic(fmt.Sprintf("%v: %s", err, out))
	}

	f, err := os.Open(sheetPath)
	if err != nil {
		panic(err)
	}
	sheet, err := png.Decode(f)
	f.Close()
	if err != nil {
		panic(err)
	}
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
	if err := os.WriteFile("build/windows/icon.ico", out.Bytes(), 0o644); err != nil {
		panic(err)
	}
	os.Remove(tmp)
	fmt.Println("Listo: build/windows/icon.ico y build/appicon.png")
}
