package checks

// Cambiar la pantalla a sus hercios máximos o a su resolución nativa.

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"turbomon/internal/parts"
)

var (
	modUser         = windows.NewLazySystemDLL("user32.dll")
	pChangeSettings = modUser.NewProc("ChangeDisplaySettingsExW")
)

// Monitor es una pantalla (ver parts.Monitor).
type Monitor = parts.Monitor

// Monitors devuelve las pantallas encendidas.
func Monitors() []Monitor { return parts.Monitors() }

const (
	dmPelsWidth  = 0x00080000
	dmPelsHeight = 0x00100000
	dmFrequency  = 0x00400000
	dmInterlaced = 0x2
	cdsTest      = 0x2
	cdsUpdateReg = 0x1
)

func changeMode(device string, w, h, hz int) error {
	devp, _ := windows.UTF16PtrFromString(device)
	dm, ok := parts.EnumMode(devp, parts.EnumCurrent)
	if !ok {
		return errors.New("no se encuentra la pantalla")
	}
	dm.Fields = dmPelsWidth | dmPelsHeight | dmFrequency
	dm.PelsWidth, dm.PelsHeight, dm.Frequency = uint32(w), uint32(h), uint32(hz)
	if r, _, _ := pChangeSettings.Call(uintptr(unsafe.Pointer(devp)), uintptr(unsafe.Pointer(&dm)), 0, cdsTest, 0); int32(r) != 0 {
		return fmt.Errorf("la pantalla no acepta %d×%d a %d Hz", w, h, hz)
	}
	if r, _, _ := pChangeSettings.Call(uintptr(unsafe.Pointer(devp)), uintptr(unsafe.Pointer(&dm)), 0, cdsUpdateReg, 0); int32(r) != 0 {
		return fmt.Errorf("Windows no ha podido cambiar la pantalla (código %d)", int32(r))
	}
	return nil
}

// SetMaxHz pone una pantalla a su máximo de hercios (en la resolución actual).
func SetMaxHz(device string) error {
	for _, m := range Monitors() {
		if m.Device == device {
			return changeMode(device, m.Width, m.Height, m.MaxHz)
		}
	}
	return errors.New("no se encuentra la pantalla")
}

// SetNativeRes pone una pantalla a su resolución máxima, con los hercios más altos que admita en ella.
func SetNativeRes(device string) error {
	devp, _ := windows.UTF16PtrFromString(device)
	for _, m := range Monitors() {
		if m.Device != device {
			continue
		}
		best := 0
		for i := uint32(0); i < 2000; i++ {
			dm, ok := parts.EnumMode(devp, i)
			if !ok {
				break
			}
			if int(dm.PelsWidth) == m.MaxWidth && int(dm.PelsHeight) == m.MaxHeight && dm.DisplayFlags&dmInterlaced == 0 && int(dm.Frequency) > best {
				best = int(dm.Frequency)
			}
		}
		return changeMode(device, m.MaxWidth, m.MaxHeight, best)
	}
	return errors.New("no se encuentra la pantalla")
}
