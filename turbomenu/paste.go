package main

// «Pegar como archivo»: lo copiado (captura, texto o enlace) se guarda como archivo en
// la carpeta y queda seleccionado con el nombre listo para cambiarlo. Sin ventanas.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procOpenClipboard      = user32.NewProc("OpenClipboard")
	procCloseClipboard     = user32.NewProc("CloseClipboard")
	procGetClipboardData   = user32.NewProc("GetClipboardData")
	procIsClipboardFormat  = user32.NewProc("IsClipboardFormatAvailable")
	procRegisterClipFormat = user32.NewProc("RegisterClipboardFormatW")
	procGlobalLock         = kernel32.NewProc("GlobalLock")
	procGlobalUnlock       = kernel32.NewProc("GlobalUnlock")
	procGlobalSize         = kernel32.NewProc("GlobalSize")
	procMessageBox         = user32.NewProc("MessageBoxW")
	procMessageBeep        = user32.NewProc("MessageBeep")
)

const (
	cfUnicodeText = 13
	cfDIB         = 8
	cfDIBV5       = 17
	cfHDROP       = 15
)

func msgBox(title, text string, icon uint32) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	procMessageBox.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), uintptr(icon|0x00040000)) // MB_TOPMOST
}

func pasteAsFile(dir string) {
	dir = strings.Trim(dir, `"`)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		msgBox("Pegar como archivo", "No se encuentra la carpeta.", 0x10)
		return
	}
	data, ext, base, err := readClipboard()
	if err != nil {
		msgBox("Pegar como archivo", err.Error(), 0x40)
		return
	}
	name := fmt.Sprintf("%s %s.%s", base, time.Now().Format("02-01-2006 15.04"), ext)
	p := uniquePath(dir, cleanFileName(name))
	if err := os.WriteFile(p, data, 0o644); err != nil {
		msgBox("Pegar como archivo", "No se pudo guardar el archivo:\n"+err.Error(), 0x10)
		return
	}
	notifyCreated(p)
	if isDesktop(dir) {
		return // en el escritorio aparece solo; abrir una ventana sería un estorbo
	}
	selectInExplorer(p, true)
}

func isDesktop(dir string) bool {
	d, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, 0)
	return err == nil && strings.EqualFold(filepath.Clean(d), filepath.Clean(dir))
}

func notifyCreated(p string) {
	pp, _ := windows.UTF16PtrFromString(p)
	procSHChangeNotify.Call(0x00000002, 0x0005|0x1000, uintptr(unsafe.Pointer(pp)), 0) // SHCNE_CREATE
}

func clipBytes(format uintptr) []byte {
	h, _, _ := procGetClipboardData.Call(format)
	if h == 0 {
		return nil
	}
	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		return nil
	}
	defer procGlobalUnlock.Call(h)
	n, _, _ := procGlobalSize.Call(h)
	out := make([]byte, n)
	copy(out, unsafe.Slice((*byte)(*(*unsafe.Pointer)(unsafe.Pointer(&ptr))), n))
	return out
}

func hasFormat(f uintptr) bool {
	r, _, _ := procIsClipboardFormat.Call(f)
	return r != 0
}

// readClipboard devuelve el contenido a guardar, su extensión y el nombre base.
func readClipboard() ([]byte, string, string, error) {
	var opened bool
	for i := 0; i < 10; i++ { // otro programa puede tenerlo abierto un instante
		if r, _, _ := procOpenClipboard.Call(0); r != 0 {
			opened = true
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if !opened {
		return nil, "", "", errors.New("No se pudo leer lo copiado. Vuelve a intentarlo.")
	}
	defer procCloseClipboard.Call()

	pngName, _ := windows.UTF16PtrFromString("PNG")
	cfPNG, _, _ := procRegisterClipFormat.Call(uintptr(unsafe.Pointer(pngName)))
	if cfPNG != 0 && hasFormat(cfPNG) {
		if b := clipBytes(cfPNG); len(b) > 8 && bytes.HasPrefix(b, []byte("\x89PNG")) {
			return b, "png", "Captura", nil
		}
	}
	for _, f := range []uintptr{cfDIBV5, cfDIB} {
		if hasFormat(f) {
			if b := clipBytes(f); len(b) > 40 {
				if img, err := dibToImage(b); err == nil {
					var buf bytes.Buffer
					enc := png.Encoder{CompressionLevel: png.BestSpeed}
					if enc.Encode(&buf, img) == nil {
						return buf.Bytes(), "png", "Captura", nil
					}
				}
			}
		}
	}
	if hasFormat(cfUnicodeText) {
		b := clipBytes(cfUnicodeText)
		u := make([]uint16, len(b)/2)
		for i := range u {
			u[i] = binary.LittleEndian.Uint16(b[i*2:])
		}
		for i, c := range u {
			if c == 0 {
				u = u[:i]
				break
			}
		}
		text := string(utf16.Decode(u))
		t := strings.TrimSpace(text)
		if t == "" {
			return nil, "", "", errors.New("Lo copiado está vacío.")
		}
		if link := asURL(t); link != nil {
			body := "[InternetShortcut]\r\nURL=" + link.String() + "\r\n"
			host := strings.TrimPrefix(link.Hostname(), "www.")
			return []byte(body), "url", "Enlace " + host, nil
		}
		text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "\r\n")
		return []byte(text), "txt", "Texto", nil
	}
	if hasFormat(cfHDROP) {
		return nil, "", "", errors.New("Lo copiado son archivos: para eso usa «Pegar» normal.")
	}
	return nil, "", "", errors.New("No hay nada copiado que se pueda guardar como archivo.\n\nCopia una captura, un texto o un enlace y vuelve a probar.")
}

func asURL(t string) *url.URL {
	if strings.ContainsAny(t, " \n\t") || len(t) > 4000 {
		return nil
	}
	u, err := url.Parse(t)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil
	}
	return u
}

// dibToImage convierte un DIB del portapapeles (32 o 24 bits) en imagen.
func dibToImage(b []byte) (image.Image, error) {
	hdr := binary.LittleEndian.Uint32(b[0:])
	w := int(int32(binary.LittleEndian.Uint32(b[4:])))
	h := int(int32(binary.LittleEndian.Uint32(b[8:])))
	bpp := int(binary.LittleEndian.Uint16(b[14:]))
	comp := binary.LittleEndian.Uint32(b[16:])
	clrUsed := int(binary.LittleEndian.Uint32(b[32:]))
	if w <= 0 || h == 0 || w > 30000 || (bpp != 32 && bpp != 24) {
		return nil, errors.New("formato de imagen no soportado")
	}
	topDown := h < 0
	if topDown {
		h = -h
	}
	off := int(hdr)
	if comp == 3 && hdr == 40 { // BI_BITFIELDS: tres máscaras detrás de la cabecera
		off += 12
	}
	off += clrUsed * 4
	stride := ((w*bpp + 31) / 32) * 4
	if off+stride*h > len(b) {
		return nil, errors.New("imagen incompleta")
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	// Muchas apps ponen el canal alfa a 0 en capturas opacas: si todo es 0, es opaca.
	alphaUsed := false
	if bpp == 32 {
		for y := 0; y < h && !alphaUsed; y++ {
			row := b[off+y*stride:]
			for x := 0; x < w; x++ {
				if row[x*4+3] != 0 {
					alphaUsed = true
					break
				}
			}
		}
	}
	for y := 0; y < h; y++ {
		sy := y
		if !topDown {
			sy = h - 1 - y
		}
		row := b[off+sy*stride:]
		for x := 0; x < w; x++ {
			var c color.NRGBA
			if bpp == 32 {
				p := row[x*4:]
				c = color.NRGBA{p[2], p[1], p[0], 255}
				if alphaUsed {
					c.A = p[3]
				}
			} else {
				p := row[x*3:]
				c = color.NRGBA{p[2], p[1], p[0], 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img, nil
}

var _ = procMessageBeep
