package main

// Overlay: una ventanita encima de todo con los FPS y los sensores. Es una
// ventana normal de Windows (no se mete nada en el juego), transparente a los
// clics y dibujada con GDI: redibujarla una vez por segundo cuesta microsegundos.
// Se ve encima de los juegos en ventana o «sin bordes»; en «pantalla completa
// exclusiva» Windows no deja poner nada encima.

import (
	"math"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procSetTextAlign   = gdi32.NewProc("SetTextAlign")
	procGetTextMetrics = gdi32.NewProc("GetTextMetricsW")
)

const (
	fontBig = iota
	fontNormal
	fontSmall
	fontLabel
	numFonts
)

// Colores (0xRRGGBB).
const (
	colBg     = 0x0e0f18
	colBorder = 0x2c2f44
	colLabel  = 0x8b90a8
	colValue  = 0xeceef8
	colDim    = 0x6b7090
	colGood   = 0x34d399
	colMid    = 0xfbbf24
	colBad    = 0xf87171
	colAccent = 0x22d3ee
)

type seg struct {
	text  string
	color uint32
	font  int
	x, w  int // calculados
}

type ovLine struct {
	label string
	segs  []seg
	y     int // línea base
	h     int
}

type ovContent struct {
	lines   []ovLine
	show    bool
	anchor  uintptr // ventana de delante (para saber en qué pantalla va)
	corner  string
	scale   int // %
	opacity int // %
}

type overlay struct {
	hwnd    uintptr
	mu      sync.Mutex
	next    ovContent // lo último que ha mandado el agente
	cur     ovContent // lo que está dibujado
	fonts   [numFonts]uintptr
	ascent  [numFonts]int
	height  [numFonts]int
	fontKey float64
	w, h    int
	labelW  int
	visible bool
}

var ovCallback = windows.NewCallback(overlayWndProc)
var ovInstance *overlay

func colorRef(c uint32) uintptr { return uintptr((c>>16)&0xff | (c & 0xff00) | (c&0xff)<<16) }

func newOverlay(inst windows.Handle) *overlay {
	o := &overlay{}
	ovInstance = o
	cls, _ := windows.UTF16PtrFromString("TurboMONOverlay")
	wc := wndClassEx{WndProc: ovCallback, Instance: inst, ClassName: cls}
	wc.Size = uint32(unsafe.Sizeof(wc))
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	title, _ := windows.UTF16PtrFromString("TurboMON overlay")
	const exStyle = 0x8 | 0x80 | 0x80000 | 0x20 | 0x08000000 // TOPMOST TOOLWINDOW LAYERED TRANSPARENT NOACTIVATE
	h, _, _ := procCreateWindowEx.Call(exStyle, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(title)), 0x80000000, // WS_POPUP
		0, 0, 10, 10, 0, 0, uintptr(inst), 0)
	o.hwnd = h
	procSetLayeredAttrs.Call(h, 0, 230, 2) // LWA_ALPHA
	return o
}

// set lo llama el agente (desde su goroutine) con lo nuevo que hay que enseñar.
func (o *overlay) set(c ovContent) {
	if o == nil || o.hwnd == 0 {
		return
	}
	o.mu.Lock()
	o.next = c
	o.mu.Unlock()
	procPostMessage.Call(o.hwnd, msgOverlay, 0, 0)
}

func (o *overlay) makeFonts(scale float64) {
	if math.Abs(scale-o.fontKey) < 0.01 && o.fonts[0] != 0 {
		return
	}
	for i, f := range o.fonts {
		if f != 0 {
			procDeleteObject.Call(f)
			o.fonts[i] = 0
		}
	}
	face, _ := windows.UTF16PtrFromString("Segoe UI")
	spec := [numFonts]struct {
		px     float64
		weight int
	}{{25, 700}, {14.5, 600}, {11.5, 600}, {11, 700}}
	dc, _, _ := procGetDC.Call(0)
	for i, s := range spec {
		h := -int(math.Round(s.px * scale))
		f, _, _ := procCreateFont.Call(uintptr(h), 0, 0, 0, uintptr(s.weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face))) // DEFAULT_CHARSET, CLEARTYPE_QUALITY
		o.fonts[i] = f
		old, _, _ := procSelectObject.Call(dc, f)
		var tm [64]byte
		procGetTextMetrics.Call(dc, uintptr(unsafe.Pointer(&tm[0])))
		o.height[i] = int(*(*int32)(unsafe.Pointer(&tm[0])))
		o.ascent[i] = int(*(*int32)(unsafe.Pointer(&tm[4])))
		procSelectObject.Call(dc, old)
	}
	procReleaseDC.Call(0, dc)
	o.fontKey = scale
}

func textWidth(dc uintptr, s string) int {
	u, _ := windows.UTF16FromString(s)
	if len(u) <= 1 {
		return 0
	}
	var sz struct{ CX, CY int32 }
	procGetTextExtent.Call(dc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&sz)))
	return int(sz.CX)
}

// layout coloca todo, mueve la ventana a su esquina y la redibuja (hilo de la ventana).
func (o *overlay) layout() {
	o.mu.Lock()
	c := o.next
	o.mu.Unlock()
	if !c.show || len(c.lines) == 0 {
		if o.visible {
			procShowWindow.Call(o.hwnd, 0) // SW_HIDE
			o.visible = false
		}
		o.cur = c
		return
	}
	mon, hmon := monitorOf(c.anchor)
	scale := float64(monitorDPI(hmon)) / 96 * float64(c.scale) / 100
	o.makeFonts(scale)
	px := func(v float64) int { return int(math.Round(v * scale)) }
	pad, gap := px(9), px(8)

	dc, _, _ := procGetDC.Call(0)
	labelW := 0
	for _, l := range c.lines {
		if l.label != "" {
			procSelectObject.Call(dc, o.fonts[fontLabel])
			if w := textWidth(dc, l.label); w > labelW {
				labelW = w
			}
		}
	}
	if labelW > 0 {
		labelW += gap
	}
	y := pad
	maxW := 0
	for i := range c.lines {
		l := &c.lines[i]
		x := pad
		if l.label != "" {
			x += labelW
		}
		asc, desc := 0, 0
		for j := range l.segs {
			s := &l.segs[j]
			procSelectObject.Call(dc, o.fonts[s.font])
			s.x, s.w = x, textWidth(dc, s.text)
			x += s.w
			if o.ascent[s.font] > asc {
				asc = o.ascent[s.font]
			}
			if d := o.height[s.font] - o.ascent[s.font]; d > desc {
				desc = d
			}
		}
		if l.label != "" {
			if o.ascent[fontLabel] > asc {
				asc = o.ascent[fontLabel]
			}
		}
		l.y = y + asc
		l.h = asc + desc
		y += l.h + px(1)
		if x > maxW {
			maxW = x
		}
	}
	procReleaseDC.Call(0, dc)
	w, h := maxW+pad, y+pad-px(1)
	margin := px(10)
	x0, y0 := int(mon.Left)+margin, int(mon.Top)+margin
	if c.corner == "td" || c.corner == "bd" {
		x0 = int(mon.Right) - margin - w
	}
	if c.corner == "bi" || c.corner == "bd" {
		y0 = int(mon.Bottom) - margin - h
	}
	if w != o.w || h != o.h {
		r := px(10)
		rgn, _, _ := procCreateRoundRectRgn.Call(0, 0, uintptr(w+1), uintptr(h+1), uintptr(r), uintptr(r))
		procSetWindowRgn.Call(o.hwnd, rgn, 1)
	}
	if c.opacity != o.cur.opacity {
		procSetLayeredAttrs.Call(o.hwnd, 0, uintptr(c.opacity*255/100), 2)
	}
	o.w, o.h, o.labelW = w, h, labelW
	o.cur = c
	// HWND_TOPMOST otra vez cada vez: algunos juegos se ponen encima al enfocarse.
	procSetWindowPos.Call(o.hwnd, ^uintptr(0), uintptr(x0), uintptr(y0), uintptr(w), uintptr(h), 0x10|0x40) // NOACTIVATE SHOWWINDOW
	o.visible = true
	procInvalidateRect.Call(o.hwnd, 0, 0)
}

func (o *overlay) paint(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	if o.w == 0 || o.h == 0 {
		return
	}
	mem, _, _ := procCreateCompatibleDC.Call(hdc)
	bmp, _, _ := procCreateCompatibleBmp.Call(hdc, uintptr(o.w), uintptr(o.h))
	oldBmp, _, _ := procSelectObject.Call(mem, bmp)
	bg, _, _ := procCreateSolidBrush.Call(colorRef(colBg))
	r := rect{0, 0, int32(o.w), int32(o.h)}
	procFillRect.Call(mem, uintptr(unsafe.Pointer(&r)), bg)
	pen, _, _ := procCreatePen.Call(0, 1, colorRef(colBorder))
	oldPen, _, _ := procSelectObject.Call(mem, pen)
	oldBrush, _, _ := procSelectObject.Call(mem, bg)
	rad := int(math.Round(10 * o.fontKey))
	procRoundRect.Call(mem, 0, 0, uintptr(o.w), uintptr(o.h), uintptr(rad), uintptr(rad))
	procSelectObject.Call(mem, oldPen)
	procSelectObject.Call(mem, oldBrush)
	procDeleteObject.Call(pen)
	procDeleteObject.Call(bg)
	procSetBkMode.Call(mem, 1)       // TRANSPARENT
	procSetTextAlign.Call(mem, 24)   // TA_BASELINE
	pad := int(math.Round(9 * o.fontKey))
	draw := func(x, y int, s string, font int, col uint32) {
		u, _ := windows.UTF16FromString(s)
		if len(u) <= 1 {
			return
		}
		procSelectObject.Call(mem, o.fonts[font])
		procSetTextColor.Call(mem, colorRef(col))
		procTextOut.Call(mem, uintptr(x), uintptr(y), uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1))
	}
	for _, l := range o.cur.lines {
		if l.label != "" {
			draw(pad, l.y, l.label, fontLabel, colLabel)
		}
		for _, s := range l.segs {
			draw(s.x, l.y, s.text, s.font, s.color)
		}
	}
	procBitBlt.Call(hdc, 0, 0, uintptr(o.w), uintptr(o.h), mem, 0, 0, 0x00CC0020) // SRCCOPY
	procSelectObject.Call(mem, oldBmp)
	procDeleteObject.Call(bmp)
	procDeleteDC.Call(mem)
}

func overlayWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	o := ovInstance
	switch uint32(msg) {
	case msgOverlay:
		if o != nil {
			o.layout()
		}
		return 0
	case wmPaint:
		if o != nil {
			o.paint(hwnd)
			return 0
		}
	case wmEraseBkgnd:
		return 1
	case wmNCHitTest:
		return htTransparent
	case wmDisplayChange, wmDpiChanged:
		if o != nil {
			o.fontKey = 0
			o.w = 0
			o.layout()
		}
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wparam, lparam)
	return r
}
