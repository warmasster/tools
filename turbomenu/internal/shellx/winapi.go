//go:build windows

package shellx

import (
	"errors"
	"image"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32                         = windows.NewLazySystemDLL("shell32.dll")
	procSHCreateItemFromParsingName = shell32.NewProc("SHCreateItemFromParsingName")
	procSHGetFileInfoW              = shell32.NewProc("SHGetFileInfoW")
	procSHGetImageList              = shell32.NewProc("SHGetImageList")

	gdi32            = windows.NewLazySystemDLL("gdi32.dll")
	procGetObjectW   = gdi32.NewProc("GetObjectW")
	procGetDIBits    = gdi32.NewProc("GetDIBits")
	procDeleteObject = gdi32.NewProc("DeleteObject")

	user32          = windows.NewLazySystemDLL("user32.dll")
	procGetDC       = user32.NewProc("GetDC")
	procReleaseDC   = user32.NewProc("ReleaseDC")
	procGetIconInfo = user32.NewProc("GetIconInfo")
	procDestroyIcon = user32.NewProc("DestroyIcon")
)

var (
	iidShellItemImageFactory = windows.GUID{Data1: 0xbcc18b79, Data2: 0xba16, Data3: 0x442f, Data4: [8]byte{0x80, 0xc4, 0x8a, 0x59, 0xc3, 0x0c, 0x46, 0x3b}}
	iidImageList             = windows.GUID{Data1: 0x46eb5926, Data2: 0x582e, Data3: 0x4017, Data4: [8]byte{0x9f, 0xdf, 0xe8, 0x99, 0x8d, 0xaa, 0x09, 0x50}}
)

// Opciones de IShellItemImageFactory::GetImage.
const (
	siigbfResizeToFit   = 0x00
	siigbfBiggerSizeOK  = 0x01
	siigbfIconOnly      = 0x04
	siigbfThumbnailOnly = 0x08
)

// Listas de iconos del sistema.
const (
	shilLarge      = 0 // 32 px
	shilSmall      = 1 // 16 px
	shilExtraLarge = 2 // 48 px
	shilJumbo      = 4 // 256 px

	shgfiSysIconIndex     = 0x4000
	shgfiUseFileAttribute = 0x0010
	ildTransparent        = 0x0001
)

type bitmapInfo struct {
	Type       int32
	Width      int32
	Height     int32
	WidthBytes int32
	Planes     uint16
	BitsPixel  uint16
	Bits       uintptr
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

type iconInfo struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  windows.Handle
	HbmColor windows.Handle
}

type shFileInfo struct {
	HIcon       windows.Handle
	IIcon       int32
	Attributes  uint32
	DisplayName [260]uint16
	TypeName    [80]uint16
}

// comCall llama al método n de la vtable de un objeto COM.
func comCall(obj unsafe.Pointer, n int, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, uintptr(n)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(obj)}, args...)...)
	return r
}

func comRelease(obj unsafe.Pointer) {
	if obj != nil {
		comCall(obj, 2)
	}
}

func deleteObject(h windows.Handle) {
	if h != 0 {
		syscall.SyscallN(procDeleteObject.Addr(), uintptr(h))
	}
}

var errNoImage = errors.New("no hay imagen")

// bitmapToImage copia un HBITMAP a una imagen Go, de arriba abajo y con el
// alfa bien interpretado (premultiplicado o no, o sin alfa).
func bitmapToImage(hbm windows.Handle) (image.Image, error) {
	var bm bitmapInfo
	if r, _, _ := syscall.SyscallN(procGetObjectW.Addr(), uintptr(hbm), unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm))); r == 0 {
		return nil, errNoImage
	}
	w, h := int(bm.Width), int(bm.Height)
	if h < 0 {
		h = -h
	}
	if w <= 0 || h <= 0 || w > 4096 || h > 4096 {
		return nil, errNoImage
	}
	bi := bitmapInfoHeader{Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}
	// GetDIBits pide sitio para la paleta tras la cabecera.
	type bmi struct {
		hdr    bitmapInfoHeader
		colors [256]uint32
	}
	info := bmi{hdr: bi}
	pix := make([]byte, w*h*4)
	dc, _, _ := syscall.SyscallN(procGetDC.Addr(), 0)
	r, _, _ := syscall.SyscallN(procGetDIBits.Addr(), dc, uintptr(hbm), 0, uintptr(h),
		uintptr(unsafe.Pointer(&pix[0])), uintptr(unsafe.Pointer(&info)), 0)
	syscall.SyscallN(procReleaseDC.Addr(), 0, dc)
	if r == 0 {
		return nil, errNoImage
	}

	// BGRA → RGBA y análisis del alfa.
	hasAlpha, premult := false, true
	for i := 0; i < len(pix); i += 4 {
		b, g, rr, a := pix[i], pix[i+1], pix[i+2], pix[i+3]
		pix[i], pix[i+2] = rr, b
		if a != 0 {
			hasAlpha = true
		}
		if rr > a || g > a || b > a {
			premult = false
		}
	}
	if bm.BitsPixel < 32 || !hasAlpha {
		for i := 3; i < len(pix); i += 4 {
			pix[i] = 255
		}
		return &image.RGBA{Pix: pix, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}, nil
	}
	if premult {
		return &image.RGBA{Pix: pix, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}, nil
	}
	return &image.NRGBA{Pix: pix, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}, nil
}

// iconToImage convierte un HICON (con su máscara si no trae alfa).
func iconToImage(hicon windows.Handle) (image.Image, error) {
	var ii iconInfo
	if r, _, _ := syscall.SyscallN(procGetIconInfo.Addr(), uintptr(hicon), uintptr(unsafe.Pointer(&ii))); r == 0 {
		return nil, errNoImage
	}
	defer deleteObject(ii.HbmColor)
	defer deleteObject(ii.HbmMask)
	if ii.HbmColor == 0 {
		return nil, errNoImage // icono monocromo: no merece la pena
	}
	img, err := bitmapToImage(ii.HbmColor)
	if err != nil {
		return nil, err
	}
	// Sin canal alfa: la transparencia está en la máscara.
	if rgba, ok := img.(*image.RGBA); ok && isOpaque(rgba) && ii.HbmMask != 0 {
		if mask, err := bitmapToImage(ii.HbmMask); err == nil && mask.Bounds() == rgba.Bounds() {
			m := mask.(*image.RGBA)
			for i := 0; i < len(rgba.Pix); i += 4 {
				if m.Pix[i] != 0 { // blanco en la máscara = transparente
					rgba.Pix[i], rgba.Pix[i+1], rgba.Pix[i+2], rgba.Pix[i+3] = 0, 0, 0, 0
				}
			}
		}
	}
	return img, nil
}

func isOpaque(m *image.RGBA) bool {
	for i := 3; i < len(m.Pix); i += 4 {
		if m.Pix[i] != 255 {
			return false
		}
	}
	return true
}
