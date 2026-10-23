//go:build windows

// Package shellx saca iconos y miniaturas de archivos como los enseña el
// Explorador, en PNG.
package shellx

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/image/draw"
	"golang.org/x/sys/windows"
)

// Errores que se enseñan al usuario.
var (
	ErrNoPreview = errors.New("No hay vista previa para este archivo.")
	ErrTimeout   = errors.New("La vista previa está tardando demasiado.")
)

const (
	minSize    = 16
	maxSize    = 1024
	jobTimeout = 6 * time.Second
	staWorkers = 3
	maxWorkers = 8
	cacheMax   = 192
)

// Thumbnail devuelve un PNG con la miniatura que pondría el Explorador
// (fotos, vídeos, PDF…) de como mucho size×size px. Si el archivo no tiene
// miniatura devuelve su icono.
func Thumbnail(path string, size int) ([]byte, error) {
	return get(path, size, false)
}

// Icon devuelve un PNG con el icono de un archivo, carpeta, unidad o .exe de
// size×size px. Con rutas que no existen da el icono de su tipo (por la extensión).
func Icon(path string, size int) ([]byte, error) {
	return get(path, size, true)
}

// Warmup arranca en segundo plano los hilos de la shell para que la primera
// miniatura salga igual de rápida que las demás.
func Warmup() {
	go func() { _, _ = Icon(os.Getenv("SystemRoot")+`\explorer.exe`, 32) }()
}

func get(path string, size int, iconOnly bool) ([]byte, error) {
	if size < minSize {
		size = minSize
	}
	if size > maxSize {
		size = maxSize
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, ErrNoPreview
	}
	if len(path) == 2 && path[1] == ':' {
		path += `\`
	}
	key, exists := cacheKey(path, size, iconOnly)
	if b, ok := cacheGet(key); ok {
		return b, nil
	}
	var img image.Image
	var err error
	err2 := onSTA(func() {
		if exists {
			flags := uint32(siigbfBiggerSizeOK)
			if iconOnly {
				flags |= siigbfIconOnly
			}
			img, err = shellImage(path, size, flags)
			if err != nil && !iconOnly {
				img, err = shellImage(path, size, siigbfBiggerSizeOK|siigbfIconOnly)
			}
		}
		if img == nil {
			img, err = typeIcon(path, size, !exists && strings.HasSuffix(path, `\`))
		}
	})
	if err2 != nil {
		return nil, err2
	}
	if err != nil || img == nil {
		return nil, ErrNoPreview
	}
	img = fit(img, size)
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, ErrNoPreview
	}
	b := buf.Bytes()
	cachePut(key, b)
	return b, nil
}

// shellImage pide la imagen a IShellItemImageFactory.
func shellImage(path string, size int, flags uint32) (image.Image, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	var factory unsafe.Pointer
	hr, _, _ := syscall.SyscallN(procSHCreateItemFromParsingName.Addr(), uintptr(unsafe.Pointer(p)), 0,
		uintptr(unsafe.Pointer(&iidShellItemImageFactory)), uintptr(unsafe.Pointer(&factory)))
	if int32(hr) < 0 || factory == nil {
		return nil, errNoImage
	}
	defer comRelease(factory)
	var hbm windows.Handle
	sz := uintptr(uint32(size)) | uintptr(uint32(size))<<32 // SIZE va por valor en un registro
	hr = comCall(factory, 3, sz, uintptr(flags), uintptr(unsafe.Pointer(&hbm)))
	if int32(hr) < 0 || hbm == 0 {
		return nil, errNoImage
	}
	defer deleteObject(hbm)
	return bitmapToImage(hbm)
}

// typeIcon da el icono del tipo de archivo sin tocar el disco (sirve para
// rutas que no existen).
func typeIcon(path string, size int, dir bool) (image.Image, error) {
	attr := uint32(windows.FILE_ATTRIBUTE_NORMAL)
	if dir {
		attr = windows.FILE_ATTRIBUTE_DIRECTORY
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	var sfi shFileInfo
	r, _, _ := syscall.SyscallN(procSHGetFileInfoW.Addr(), uintptr(unsafe.Pointer(p)), uintptr(attr),
		uintptr(unsafe.Pointer(&sfi)), unsafe.Sizeof(sfi), shgfiSysIconIndex|shgfiUseFileAttribute)
	if r == 0 {
		return nil, errNoImage
	}
	list := shilJumbo
	switch {
	case size <= 16:
		list = shilSmall
	case size <= 32:
		list = shilLarge
	case size <= 48:
		list = shilExtraLarge
	}
	var il unsafe.Pointer
	hr, _, _ := syscall.SyscallN(procSHGetImageList.Addr(), uintptr(list),
		uintptr(unsafe.Pointer(&iidImageList)), uintptr(unsafe.Pointer(&il)))
	if int32(hr) < 0 || il == nil {
		return nil, errNoImage
	}
	defer comRelease(il)
	var hicon windows.Handle
	hr = comCall(il, 10, uintptr(sfi.IIcon), ildTransparent, uintptr(unsafe.Pointer(&hicon))) // IImageList::GetIcon
	if int32(hr) < 0 || hicon == 0 {
		return nil, errNoImage
	}
	defer syscall.SyscallN(procDestroyIcon.Addr(), uintptr(hicon))
	img, err := iconToImage(hicon)
	if err != nil {
		return nil, err
	}
	if list == shilJumbo {
		img = trimJumbo(img)
	}
	return img, nil
}

// trimJumbo: los tipos sin icono de 256 px salen como uno pequeño en una
// esquina del lienzo grande; se recorta para que se vea a buen tamaño.
func trimJumbo(img image.Image) image.Image {
	b := img.Bounds()
	maxX, maxY := 0, 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0 {
				if x > maxX {
					maxX = x
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	side := maxX + 1
	if maxY+1 > side {
		side = maxY + 1
	}
	if side >= b.Dx()/2 || side < 8 {
		return img
	}
	if s, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
		return s.SubImage(image.Rect(0, 0, side, side))
	}
	return img
}

// fit reduce la imagen para que quepa en size×size, sin deformarla.
func fit(img image.Image, size int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= size && h <= size && b.Min == (image.Point{}) {
		return img
	}
	nw, nh := w, h
	if w > size || h > size {
		if w >= h {
			nw, nh = size, max(1, h*size/w)
		} else {
			nw, nh = max(1, w*size/h), size
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

// ---- Hilos COM (STA) ----

var (
	staOnce  sync.Once
	staJobs  chan func()
	staMu    sync.Mutex
	staCount int
)

func startWorker() {
	staMu.Lock()
	if staCount >= maxWorkers {
		staMu.Unlock()
		return
	}
	staCount++
	staMu.Unlock()
	go func() {
		runtime.LockOSThread() // COM STA: el objeto vive en este hilo
		if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err == nil {
			defer windows.CoUninitialize()
		}
		for f := range staJobs {
			f()
		}
	}()
}

// onSTA ejecuta f en un hilo COM de la shell y espera (con tope de tiempo).
func onSTA(f func()) error {
	staOnce.Do(func() {
		staJobs = make(chan func(), 32)
		for i := 0; i < staWorkers; i++ {
			startWorker()
		}
	})
	done := make(chan error, 1)
	staJobs <- func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("%w", ErrNoPreview)
				return
			}
			done <- nil
		}()
		f()
	}
	select {
	case err := <-done:
		return err
	case <-time.After(jobTimeout):
		// Ese hilo se ha quedado colgado (un visor de miniaturas lento): se
		// añade otro para que los demás pedidos no esperen.
		startWorker()
		return ErrTimeout
	}
}

// ---- Caché ----

var (
	cacheMu    sync.Mutex
	cacheMap   = map[string][]byte{}
	cacheOrder []string
)

// cacheKey incluye fecha y tamaño del archivo para no dar miniaturas viejas.
// Los iconos por tipo se guardan por extensión.
func cacheKey(path string, size int, iconOnly bool) (string, bool) {
	mode := "t"
	if iconOnly {
		mode = "i"
	}
	st, err := os.Stat(path)
	if err != nil {
		ext := strings.ToLower(filepath.Ext(path))
		if strings.HasSuffix(path, `\`) {
			ext = `<dir>`
		}
		return fmt.Sprintf("x|%s|%d|%s", mode, size, ext), false
	}
	ext := strings.ToLower(filepath.Ext(path))
	if iconOnly && !st.IsDir() && ext != "" && !perFileIcon[ext] {
		return fmt.Sprintf("x|%s|%d|%s", mode, size, ext), true
	}
	return fmt.Sprintf("%s|%d|%s|%d|%d", mode, size, strings.ToLower(path), st.ModTime().UnixNano(), st.Size()), true
}

// Extensiones cuyo icono cambia en cada archivo.
var perFileIcon = map[string]bool{
	".exe": true, ".ico": true, ".lnk": true, ".url": true, ".cur": true, ".ani": true,
	".dll": true, ".scr": true, ".cpl": true, ".msc": true, ".appref-ms": true,
}

func cacheGet(k string) ([]byte, bool) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	b, ok := cacheMap[k]
	return b, ok
}

func cachePut(k string, b []byte) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if _, ok := cacheMap[k]; ok {
		return
	}
	cacheMap[k] = b
	cacheOrder = append(cacheOrder, k)
	for len(cacheOrder) > cacheMax {
		delete(cacheMap, cacheOrder[0])
		cacheOrder = cacheOrder[1:]
	}
}
