package main

import (
	"errors"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                 = windows.NewLazySystemDLL("user32.dll")
	kernel32               = windows.NewLazySystemDLL("kernel32.dll")
	procOpenClipboard      = user32.NewProc("OpenClipboard")
	procCloseClipboard     = user32.NewProc("CloseClipboard")
	procEmptyClipboard     = user32.NewProc("EmptyClipboard")
	procSetClipboardData   = user32.NewProc("SetClipboardData")
	procRegisterClipFormat = user32.NewProc("RegisterClipboardFormatW")
	procGetClipboardSeq    = user32.NewProc("GetClipboardSequenceNumber")
	procGlobalAlloc        = kernel32.NewProc("GlobalAlloc")
	procGlobalLock         = kernel32.NewProc("GlobalLock")
	procGlobalUnlock       = kernel32.NewProc("GlobalUnlock")
	procGlobalFree         = kernel32.NewProc("GlobalFree")
	procMoveMemory         = kernel32.NewProc("RtlMoveMemory")
	lastSecret             atomic.Uint32
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

func openClipboard() error {
	// otro programa puede tenerlo abierto un instante: se reintenta
	for i := 0; i < 20; i++ {
		if r, _, _ := procOpenClipboard.Call(0); r != 0 {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return errors.New("el portapapeles está ocupado")
}

func globalBytes(b []byte) (uintptr, error) {
	h, _, err := procGlobalAlloc.Call(gmemMoveable, uintptr(len(b)))
	if h == 0 {
		return 0, err
	}
	p, _, err := procGlobalLock.Call(h)
	if p == 0 {
		procGlobalFree.Call(h)
		return 0, err
	}
	if len(b) > 0 {
		procMoveMemory.Call(p, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
	}
	procGlobalUnlock.Call(h)
	return h, nil
}

func setData(format uintptr, b []byte) error {
	h, err := globalBytes(b)
	if err != nil {
		return err
	}
	if r, _, err := procSetClipboardData.Call(format, h); r == 0 {
		procGlobalFree.Call(h)
		return err
	}
	return nil // ahora es de Windows: no se libera
}

func registerFormat(name string) uintptr {
	p, _ := windows.UTF16PtrFromString(name)
	r, _, _ := procRegisterClipFormat.Call(uintptr(unsafe.Pointer(p)))
	return r
}

// setClipboard pone un texto en el portapapeles. Si es secreto, le añade las
// marcas que respetan Windows y los gestores de portapapeles para no guardarlo
// en el historial (Win+V) ni subirlo a la nube. Devuelve el número de secuencia
// del portapapeles para poder borrarlo luego solo si sigue siendo lo nuestro.
func setClipboard(text string, secret bool) (uint32, error) {
	u, err := windows.UTF16FromString(text)
	if err != nil {
		return 0, err
	}
	if err := openClipboard(); err != nil {
		return 0, err
	}
	procEmptyClipboard.Call()
	b := unsafe.Slice((*byte)(unsafe.Pointer(&u[0])), len(u)*2)
	err = setData(cfUnicodeText, b)
	if err == nil && secret {
		zero := []byte{0, 0, 0, 0}
		if f := registerFormat("ExcludeClipboardContentFromMonitorProcessing"); f != 0 {
			_ = setData(f, zero)
		}
		if f := registerFormat("CanIncludeInClipboardHistory"); f != 0 {
			_ = setData(f, zero)
		}
		if f := registerFormat("CanUploadToCloudClipboard"); f != 0 {
			_ = setData(f, zero)
		}
	}
	procCloseClipboard.Call()
	for i := range u {
		u[i] = 0
	}
	if err != nil {
		return 0, err
	}
	seq := clipboardSeq()
	if secret {
		lastSecret.Store(seq)
	}
	return seq, nil
}

func clipboardSeq() uint32 {
	r, _, _ := procGetClipboardSeq.Call()
	return uint32(r)
}

func lastSecretSeq() uint32 { return lastSecret.Load() }

// clearClipboardIf vacía el portapapeles si nadie ha copiado nada después.
func clearClipboardIf(seq uint32) {
	if seq == 0 || clipboardSeq() != seq {
		return
	}
	if openClipboard() != nil {
		return
	}
	procEmptyClipboard.Call()
	procCloseClipboard.Call()
}
