//go:build ignore

// Hace todos los iconos de TurboVAULT a partir de los SVG:
//
//	frontend/logo.svg        → build/appicon.png, build/windows/icon.ico,
//	                           extension/icons/icon-*.png e icono de TurboSETUP
//	dev/icons/tray-open.svg  → assets/tray-open.ico, extension/icons/open-*.png
//	dev/icons/tray-locked.svg→ assets/tray-locked.ico, extension/icons/locked-*.png
//
// Dibuja cada SVG con Edge sin ventana (así se ve igual que en la app).
//
// Uso (desde turbovault/): go run dev/icons/genicons.go
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

func render(svgPath string, sizes []int) map[int][]byte {
	svg, err := os.ReadFile(svgPath)
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
	tmp, _ := filepath.Abs("dev/icons/_sheet.html")
	if err := os.WriteFile(tmp, []byte(html.String()), 0o644); err != nil {
		panic(err)
	}
	defer os.Remove(tmp)
	sheetPath, _ := filepath.Abs("dev/icons/_sheet.png")
	defer os.Remove(sheetPath)
	edge := `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`
	prof, _ := os.MkdirTemp("", "tv-edge")
	defer os.RemoveAll(prof)
	cmd := exec.Command(edge, "--headless=new", "--disable-gpu", "--hide-scrollbars", "--force-device-scale-factor=1", "--user-data-dir="+prof,
		"--default-background-color=00000000", fmt.Sprintf("--window-size=%d,256", len(sizes)*256),
		"--screenshot="+sheetPath, "file:///"+filepath.ToSlash(tmp))
	eout, err := cmd.CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("%v: %s", err, eout))
	}
	if _, err := os.Stat(sheetPath); err != nil {
		panic(fmt.Sprintf("Edge no dejó la imagen: %s", eout))
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
	out := map[int][]byte{}
	for i, s := range sizes {
		dst := image.NewNRGBA(image.Rect(0, 0, s, s))
		draw.Draw(dst, dst.Bounds(), sheet, image.Pt(i*256, 0), draw.Src)
		var buf bytes.Buffer
		png.Encode(&buf, dst)
		out[s] = buf.Bytes()
	}
	return out
}

func writeICO(path string, sizes []int, imgs map[int][]byte) {
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	off := 6 + 16*len(sizes)
	for _, s := range sizes {
		w := byte(s)
		if s >= 256 {
			w = 0
		}
		out.Write([]byte{w, w, 0, 0})
		binary.Write(&out, binary.LittleEndian, [2]uint16{1, 32})
		binary.Write(&out, binary.LittleEndian, [2]uint32{uint32(len(imgs[s])), uint32(off)})
		off += len(imgs[s])
	}
	for _, s := range sizes {
		out.Write(imgs[s])
	}
	must(os.WriteFile(path, out.Bytes(), 0o644))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	appSizes := []int{16, 20, 24, 32, 40, 48, 64, 128, 256}
	app := render("frontend/logo.svg", appSizes)
	must(os.WriteFile("build/appicon.png", app[256], 0o644))
	writeICO("build/windows/icon.ico", appSizes, app)
	for _, s := range []int{16, 32, 48, 128} {
		must(os.WriteFile(fmt.Sprintf("extension/icons/icon-%d.png", s), app[s], 0o644))
	}
	logo, _ := os.ReadFile("frontend/logo.svg")
	must(os.WriteFile("../installer/frontend/icons/turbovault.svg", logo, 0o644))
	must(os.WriteFile("extension/icons/logo.svg", logo, 0o644))

	traySizes := []int{16, 20, 24, 32, 40, 48, 64}
	for _, kind := range []string{"open", "locked"} {
		imgs := render("dev/icons/tray-"+kind+".svg", traySizes)
		writeICO("assets/tray-"+kind+".ico", traySizes, imgs)
		for _, s := range []int{16, 24, 32} {
			must(os.WriteFile(fmt.Sprintf("extension/icons/%s-%d.png", kind, s), imgs[s], 0o644))
		}
	}
	fmt.Println("Listo: iconos de la app, la bandeja y la extensión")
}
