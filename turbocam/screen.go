package main

// Captura de pantalla con GDI (BitBlt): sirve para las escenas «región de la
// pantalla» y «móvil sobre la pantalla». Las coordenadas son píxeles físicos del
// escritorio virtual (la app declara DPI por monitor en su manifiesto).

import (
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")
	procGetCursorInfo       = user32.NewProc("GetCursorInfo")
	procGetIconInfo         = user32.NewProc("GetIconInfo")
	procDrawIconEx          = user32.NewProc("DrawIconEx")
	procEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW     = user32.NewProc("GetMonitorInfoW")
	procCreateCompatibleDC  = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection    = gdi32.NewProc("CreateDIBSection")
	procSelectObject        = gdi32.NewProc("SelectObject")
	procBitBlt              = gdi32.NewProc("BitBlt")
	procDeleteObject        = gdi32.NewProc("DeleteObject")
	procDeleteDC            = gdi32.NewProc("DeleteDC")
	procGdiFlush            = gdi32.NewProc("GdiFlush")
)

const (
	srcCopy    = 0x00CC0020
	captureBlt = 0x40000000
	diNormal   = 0x0003
)

// Monitor es una pantalla, en píxeles físicos del escritorio virtual.
type Monitor struct {
	Index   int    `json:"index"`
	Name    string `json:"name"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	W       int    `json:"w"`
	H       int    `json:"h"`
	Primary bool   `json:"primary"`
}

type monitorInfoEx struct {
	Size    uint32
	Monitor windows.Rect
	Work    windows.Rect
	Flags   uint32
	Device  [32]uint16
}

// La llamada de vuelta escribe en monitorList (con monitorMu cogido): pasarle un
// puntero de Go a través de un uintptr no está permitido.
var (
	monitorMu   sync.Mutex
	monitorList []Monitor
)

var monitorEnumCB = syscall.NewCallback(func(hmon, hdc, rect, data uintptr) uintptr {
	list := &monitorList
	mi := monitorInfoEx{Size: uint32(unsafe.Sizeof(monitorInfoEx{}))}
	if r, _, _ := procGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&mi))); r != 0 {
		*list = append(*list, Monitor{
			Index:   len(*list),
			Name:    fmt.Sprintf("Pantalla %d", len(*list)+1),
			X:       int(mi.Monitor.Left),
			Y:       int(mi.Monitor.Top),
			W:       int(mi.Monitor.Right - mi.Monitor.Left),
			H:       int(mi.Monitor.Bottom - mi.Monitor.Top),
			Primary: mi.Flags&1 != 0,
		})
	}
	return 1
})

func listMonitors() []Monitor {
	monitorMu.Lock()
	defer monitorMu.Unlock()
	monitorList = nil
	procEnumDisplayMonitors.Call(0, 0, monitorEnumCB, 0)
	return monitorList
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type cursorInfo struct {
	Size   uint32
	Flags  uint32
	Cursor windows.Handle
	Pos    struct{ X, Y int32 }
}

type iconInfo struct {
	Icon     int32
	XHotspot uint32
	YHotspot uint32
	Mask     windows.Handle
	Color    windows.Handle
}

// grabber reutiliza el mapa de bits mientras no cambie el tamaño de la región.
type grabber struct {
	dc   uintptr
	bmp  uintptr
	old  uintptr
	bits []byte
	w, h int
	img  *image.YCbCr
}

func (g *grabber) ensure(w, h int) error {
	if g.dc != 0 && g.w == w && g.h == h {
		return nil
	}
	g.release()
	dc, _, _ := procCreateCompatibleDC.Call(0)
	if dc == 0 {
		return errors.New("CreateCompatibleDC falló")
	}
	bi := bitmapInfoHeader{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}
	var bits uintptr
	bmp, _, _ := procCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 || bits == 0 {
		procDeleteDC.Call(dc)
		return errors.New("CreateDIBSection falló")
	}
	g.old, _, _ = procSelectObject.Call(dc, bmp)
	g.dc, g.bmp, g.w, g.h = dc, bmp, w, h
	g.bits = unsafe.Slice((*byte)(*(*unsafe.Pointer)(unsafe.Pointer(&bits))), w*h*4)
	g.img = newYUV(w, h)
	return nil
}

func (g *grabber) release() {
	if g.dc != 0 {
		procSelectObject.Call(g.dc, g.old)
		procDeleteObject.Call(g.bmp)
		procDeleteDC.Call(g.dc)
	}
	*g = grabber{}
}

// grab copia la región de la pantalla. El resultado se reutiliza en la siguiente
// llamada: quien lo guarde debe copiarlo.
func (g *grabber) grab(r Region, cursor bool) (*image.YCbCr, error) {
	w, h := r.W&^1, r.H&^1
	if w < 16 || h < 16 {
		return nil, errors.New("región vacía")
	}
	if err := g.ensure(w, h); err != nil {
		return nil, err
	}
	screen, _, _ := procGetDC.Call(0)
	if screen == 0 {
		return nil, errors.New("no se puede leer la pantalla")
	}
	ok, _, _ := procBitBlt.Call(g.dc, 0, 0, uintptr(w), uintptr(h), screen, uintptr(r.X), uintptr(r.Y), srcCopy|captureBlt)
	procReleaseDC.Call(0, screen)
	if ok == 0 {
		return nil, errors.New("no se puede leer la pantalla (¿bloqueada?)")
	}
	if cursor {
		g.drawCursor(r)
	}
	procGdiFlush.Call()
	bgraToYUV(g.img, g.bits, w*4)
	return g.img, nil
}

func (g *grabber) drawCursor(r Region) {
	ci := cursorInfo{Size: uint32(unsafe.Sizeof(cursorInfo{}))}
	if ok, _, _ := procGetCursorInfo.Call(uintptr(unsafe.Pointer(&ci))); ok == 0 || ci.Flags&1 == 0 || ci.Cursor == 0 {
		return
	}
	var ii iconInfo
	if ok, _, _ := procGetIconInfo.Call(uintptr(ci.Cursor), uintptr(unsafe.Pointer(&ii))); ok == 0 {
		return
	}
	if ii.Mask != 0 {
		procDeleteObject.Call(uintptr(ii.Mask))
	}
	if ii.Color != 0 {
		procDeleteObject.Call(uintptr(ii.Color))
	}
	x := int(ci.Pos.X) - int(ii.XHotspot) - r.X
	y := int(ci.Pos.Y) - int(ii.YHotspot) - r.Y
	procDrawIconEx.Call(g.dc, uintptr(x), uintptr(y), uintptr(ci.Cursor), 0, 0, 0, 0, diNormal)
}

// monitorShot hace una foto de una pantalla, reducida, para elegir la región en la
// ventana. Devuelve un data: URL con un JPEG.
func monitorShot(m Monitor) (string, error) {
	var g grabber
	defer g.release()
	img, err := g.grab(Region{X: m.X, Y: m.Y, W: m.W, H: m.H}, false)
	if err != nil {
		return "", err
	}
	w := 1280
	if m.W < w {
		w = m.W &^ 1
	}
	h := (m.H * w / m.W) &^ 1
	small := newYUV(w, h)
	drawScaled(small, small.Rect, img, true, 0)
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(encodeJPEG(small, 80)), nil
}
