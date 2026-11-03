package checks

// Pantallas: a cuántos hercios y qué resolución van, cuál es su máximo, cómo se
// llaman y a qué tarjeta gráfica están conectadas.

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modUser          = windows.NewLazySystemDLL("user32.dll")
	pEnumSettings    = modUser.NewProc("EnumDisplaySettingsExW")
	pChangeSettings  = modUser.NewProc("ChangeDisplaySettingsExW")
	pBufferSizes     = modUser.NewProc("GetDisplayConfigBufferSizes")
	pQueryConfig     = modUser.NewProc("QueryDisplayConfig")
	pDeviceInfo      = modUser.NewProc("DisplayConfigGetDeviceInfo")
)

// Monitor es una pantalla encendida.
type Monitor struct {
	Device    string `json:"device"` // \\.\DISPLAY1
	Name      string `json:"name"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Hz        int    `json:"hz"`
	MaxHz     int    `json:"maxHz"` // a la resolución actual
	MaxWidth  int    `json:"maxWidth"`
	MaxHeight int    `json:"maxHeight"`
	Internal  bool   `json:"internal"` // la pantalla de un portátil
	LuidLow   uint32 `json:"-"`
	LuidHigh  int32  `json:"-"`
}

type devMode struct {
	DeviceName    [32]uint16
	SpecVersion   uint16
	DriverVersion uint16
	Size          uint16
	DriverExtra   uint16
	Fields        uint32
	PosX, PosY    int32
	Orientation   uint32
	FixedOutput   uint32
	Color         int16
	Duplex        int16
	YResolution   int16
	TTOption      int16
	Collate       int16
	FormName      [32]uint16
	LogPixels     uint16
	BitsPerPel    uint32
	PelsWidth     uint32
	PelsHeight    uint32
	DisplayFlags  uint32
	Frequency     uint32
	_             [32]byte
}

const (
	enumCurrent    = 0xFFFFFFFF
	dmPelsWidth    = 0x00080000
	dmPelsHeight   = 0x00100000
	dmFrequency    = 0x00400000
	dmInterlaced   = 0x2
	cdsTest        = 0x2
	cdsUpdateReg   = 0x1
	qdcOnlyActive  = 0x2
	techInternal   = 0x80000000 // DISPLAYCONFIG_OUTPUT_TECHNOLOGY_INTERNAL
	techEmbeddedDP = 11
	techUDIEmbed   = 13
)

func enumMode(dev *uint16, i uint32) (devMode, bool) {
	var dm devMode
	dm.Size = 220
	r, _, _ := pEnumSettings.Call(uintptr(unsafe.Pointer(dev)), uintptr(i), uintptr(unsafe.Pointer(&dm)), 0)
	return dm, r != 0
}

type pathInfo struct {
	SrcLuidLow    uint32
	SrcLuidHigh   int32
	SrcID         uint32
	SrcModeIdx    uint32
	SrcFlags      uint32
	TgtLuidLow    uint32
	TgtLuidHigh   int32
	TgtID         uint32
	TgtModeIdx    uint32
	OutputTech    uint32
	Rotation      uint32
	Scaling       uint32
	RefreshNum    uint32
	RefreshDen    uint32
	ScanLine      uint32
	TgtAvailable  int32
	TgtFlags      uint32
	Flags         uint32
}

type dcHeader struct {
	Type     uint32
	Size     uint32
	LuidLow  uint32
	LuidHigh int32
	ID       uint32
}

type sourceName struct {
	H   dcHeader
	GDI [32]uint16
}

type targetName struct {
	H            dcHeader
	Flags        uint32
	OutputTech   uint32
	EdidMaker    uint16
	EdidProduct  uint16
	Connector    uint32
	FriendlyName [64]uint16
	DevicePath   [128]uint16
}

// Monitors devuelve las pantallas encendidas.
func Monitors() []Monitor {
	var np, nm uint32
	if r, _, _ := pBufferSizes.Call(qdcOnlyActive, uintptr(unsafe.Pointer(&np)), uintptr(unsafe.Pointer(&nm))); r != 0 || np == 0 {
		return nil
	}
	paths := make([]pathInfo, np)
	modes := make([]byte, 64*int(nm)+64)
	if r, _, _ := pQueryConfig.Call(qdcOnlyActive, uintptr(unsafe.Pointer(&np)), uintptr(unsafe.Pointer(&paths[0])), uintptr(unsafe.Pointer(&nm)), uintptr(unsafe.Pointer(&modes[0])), 0); r != 0 {
		return nil
	}
	var out []Monitor
	seen := map[string]bool{}
	for _, p := range paths[:np] {
		sn := sourceName{H: dcHeader{Type: 1, Size: uint32(unsafe.Sizeof(sourceName{})), LuidLow: p.SrcLuidLow, LuidHigh: p.SrcLuidHigh, ID: p.SrcID}}
		if r, _, _ := pDeviceInfo.Call(uintptr(unsafe.Pointer(&sn))); r != 0 {
			continue
		}
		dev := windows.UTF16ToString(sn.GDI[:])
		if seen[dev] {
			continue // pantallas duplicadas: la misma fuente
		}
		seen[dev] = true
		tn := targetName{H: dcHeader{Type: 2, Size: uint32(unsafe.Sizeof(targetName{})), LuidLow: p.TgtLuidLow, LuidHigh: p.TgtLuidHigh, ID: p.TgtID}}
		name := ""
		if r, _, _ := pDeviceInfo.Call(uintptr(unsafe.Pointer(&tn))); r == 0 {
			name = windows.UTF16ToString(tn.FriendlyName[:])
		}
		m := Monitor{Device: dev, Name: name, LuidLow: p.TgtLuidLow, LuidHigh: p.TgtLuidHigh}
		tech := p.OutputTech
		m.Internal = tech == techInternal || tech == techEmbeddedDP || tech == techUDIEmbed
		if m.Name == "" {
			if m.Internal {
				m.Name = "Pantalla del portátil"
			} else {
				m.Name = "Pantalla"
			}
		}
		devp, _ := windows.UTF16PtrFromString(dev)
		cur, ok := enumMode(devp, enumCurrent)
		if !ok {
			continue
		}
		m.Width, m.Height, m.Hz = int(cur.PelsWidth), int(cur.PelsHeight), int(cur.Frequency)
		for i := uint32(0); i < 2000; i++ {
			dm, ok := enumMode(devp, i)
			if !ok {
				break
			}
			if dm.DisplayFlags&dmInterlaced != 0 || dm.BitsPerPel < 24 {
				continue
			}
			w, h, f := int(dm.PelsWidth), int(dm.PelsHeight), int(dm.Frequency)
			if w*h > m.MaxWidth*m.MaxHeight {
				m.MaxWidth, m.MaxHeight = w, h
			}
			if w == m.Width && h == m.Height && f > m.MaxHz {
				m.MaxHz = f
			}
		}
		// Con el giro de pantalla (vertical), los modos vienen ya girados.
		if m.MaxHz == 0 {
			m.MaxHz = m.Hz
		}
		out = append(out, m)
	}
	return out
}

func changeMode(device string, w, h, hz int) error {
	devp, _ := windows.UTF16PtrFromString(device)
	dm, ok := enumMode(devp, enumCurrent)
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
			dm, ok := enumMode(devp, i)
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
