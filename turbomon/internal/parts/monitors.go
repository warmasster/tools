package parts

// Pantallas: a cuántos hercios y qué resolución van, su máximo, a qué gráfica
// están conectadas y, de su EDID, marca, modelo y pulgadas.

import (
	"math"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	modUser       = windows.NewLazySystemDLL("user32.dll")
	pEnumSettings = modUser.NewProc("EnumDisplaySettingsExW")
	pBufferSizes  = modUser.NewProc("GetDisplayConfigBufferSizes")
	pQueryConfig  = modUser.NewProc("QueryDisplayConfig")
	pDeviceInfo   = modUser.NewProc("DisplayConfigGetDeviceInfo")
)

// Monitor es una pantalla encendida.
type Monitor struct {
	Device     string  `json:"device"` // \\.\DISPLAY1
	Name       string  `json:"name"`
	Maker      string  `json:"maker"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Hz         int     `json:"hz"`
	MaxHz      int     `json:"maxHz"` // a la resolución actual
	MaxWidth   int     `json:"maxWidth"`
	MaxHeight  int     `json:"maxHeight"`
	Inches     float64 `json:"inches"`
	HDR        bool    `json:"hdr"`   // la admite
	HDROn      bool    `json:"hdrOn"` // está activada
	Internal   bool    `json:"internal"`
	Primary    bool    `json:"primary"`
	LuidLow    uint32  `json:"-"`
	LuidHigh   int32   `json:"-"`
	TargetID   uint32  `json:"-"`
	devicePath string
}

// DevMode es DEVMODEW (la parte de pantallas).
type DevMode struct {
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

// EnumCurrent pide el modo actual a EnumMode.
const EnumCurrent = 0xFFFFFFFF

// EnumMode lee el modo i de una pantalla (o el actual con EnumCurrent).
func EnumMode(dev *uint16, i uint32) (DevMode, bool) {
	var dm DevMode
	dm.Size = 220
	r, _, _ := pEnumSettings.Call(uintptr(unsafe.Pointer(dev)), uintptr(i), uintptr(unsafe.Pointer(&dm)), 0)
	return dm, r != 0
}

type pathInfo struct {
	SrcLuidLow   uint32
	SrcLuidHigh  int32
	SrcID        uint32
	SrcModeIdx   uint32
	SrcFlags     uint32
	TgtLuidLow   uint32
	TgtLuidHigh  int32
	TgtID        uint32
	TgtModeIdx   uint32
	OutputTech   uint32
	Rotation     uint32
	Scaling      uint32
	RefreshNum   uint32
	RefreshDen   uint32
	ScanLine     uint32
	TgtAvailable int32
	TgtFlags     uint32
	Flags        uint32
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

type colorInfo struct {
	H        dcHeader
	Value    uint32
	Encoding uint32
	Bits     uint32
}

const (
	techInternal   = 0x80000000
	techEmbeddedDP = 11
	techUDIEmbed   = 13
	dmInterlaced   = 0x2
)

// Monitors devuelve las pantallas encendidas.
func Monitors() []Monitor {
	var np, nm uint32
	if r, _, _ := pBufferSizes.Call(2, uintptr(unsafe.Pointer(&np)), uintptr(unsafe.Pointer(&nm))); r != 0 || np == 0 { // QDC_ONLY_ACTIVE_PATHS
		return nil
	}
	paths := make([]pathInfo, np)
	modes := make([]byte, 64*int(nm)+64)
	if r, _, _ := pQueryConfig.Call(2, uintptr(unsafe.Pointer(&np)), uintptr(unsafe.Pointer(&paths[0])), uintptr(unsafe.Pointer(&nm)), uintptr(unsafe.Pointer(&modes[0])), 0); r != 0 {
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
			continue
		}
		seen[dev] = true
		m := Monitor{Device: dev, LuidLow: p.TgtLuidLow, LuidHigh: p.TgtLuidHigh, TargetID: p.TgtID}
		tn := targetName{H: dcHeader{Type: 2, Size: uint32(unsafe.Sizeof(targetName{})), LuidLow: p.TgtLuidLow, LuidHigh: p.TgtLuidHigh, ID: p.TgtID}}
		if r, _, _ := pDeviceInfo.Call(uintptr(unsafe.Pointer(&tn))); r == 0 {
			m.Name = windows.UTF16ToString(tn.FriendlyName[:])
			m.devicePath = windows.UTF16ToString(tn.DevicePath[:])
		}
		ci := colorInfo{H: dcHeader{Type: 9, Size: uint32(unsafe.Sizeof(colorInfo{})), LuidLow: p.TgtLuidLow, LuidHigh: p.TgtLuidHigh, ID: p.TgtID}}
		if r, _, _ := pDeviceInfo.Call(uintptr(unsafe.Pointer(&ci))); r == 0 {
			m.HDR = ci.Value&0x1 != 0
			m.HDROn = ci.Value&0x2 != 0
		}
		tech := p.OutputTech
		m.Internal = tech == techInternal || tech == techEmbeddedDP || tech == techUDIEmbed
		devp, _ := windows.UTF16PtrFromString(dev)
		cur, ok := EnumMode(devp, EnumCurrent)
		if !ok {
			continue
		}
		m.Width, m.Height, m.Hz = int(cur.PelsWidth), int(cur.PelsHeight), int(cur.Frequency)
		m.Primary = cur.PosX == 0 && cur.PosY == 0
		for i := uint32(0); i < 2000; i++ {
			dm, ok := EnumMode(devp, i)
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
		if m.MaxHz == 0 {
			m.MaxHz = m.Hz
		}
		if e := readEDID(m.devicePath); e != nil {
			m.Maker = e.maker
			if e.name != "" && (m.Name == "" || strings.HasPrefix(m.Name, "Generic")) {
				m.Name = e.name
			}
			m.Inches = e.inches
		}
		if m.Name == "" {
			if m.Internal {
				m.Name = "Pantalla del portátil"
			} else {
				m.Name = "Pantalla"
			}
		}
		out = append(out, m)
	}
	return out
}

type edidInfo struct {
	maker  string
	name   string
	inches float64
}

// Fabricantes por su código PNP de tres letras.
var pnpMakers = map[string]string{
	"AOC": "AOC", "ACR": "Acer", "AUS": "ASUS", "BNQ": "BenQ", "DEL": "Dell", "GSM": "LG", "HWP": "HP", "HPN": "HP",
	"LEN": "Lenovo", "MSI": "MSI", "PHL": "Philips", "SAM": "Samsung", "SNY": "Sony", "VSC": "ViewSonic", "GBT": "Gigabyte",
	"HKC": "HKC", "KTC": "KTC", "IVM": "iiyama", "AOP": "AOPEN", "XMI": "Xiaomi", "HSD": "HannStar", "HPC": "HP",
	"EIZ": "EIZO", "NEC": "NEC", "FUS": "Fujitsu", "LGD": "LG Display", "BOE": "BOE", "AUO": "AU Optronics",
	"CMN": "Innolux", "SHP": "Sharp", "SDC": "Samsung Display", "CSO": "CSOT", "TCL": "TCL", "HIS": "Hisense",
	"MEI": "Panasonic", "PNS": "Pixio", "ALI": "Alienware", "ZOW": "BenQ ZOWIE", "NVD": "NVIDIA", "INN": "Innocn",
	"TTE": "Titan Army", "MZI": "Medion", "ONN": "onn", "GWD": "GreenWorld", "VIZ": "Vizio",
}

// readEDID busca el EDID de una pantalla en el registro a partir de su ruta
// («\\?\DISPLAY#AOC2402#5&…&UID4353#{…}»).
func readEDID(path string) *edidInfo {
	p := strings.TrimPrefix(path, `\\?\`)
	parts := strings.Split(p, "#")
	if len(parts) < 3 {
		return nil
	}
	key := `SYSTEM\CurrentControlSet\Enum\` + parts[0] + `\` + parts[1] + `\` + parts[2] + `\Device Parameters`
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, key, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	b, _, err := k.GetBinaryValue("EDID")
	if err != nil || len(b) < 128 {
		return nil
	}
	return parseEDID(b)
}

func parseEDID(b []byte) *edidInfo {
	if len(b) < 128 || b[0] != 0 || b[1] != 0xFF {
		return nil
	}
	e := &edidInfo{}
	id := uint16(b[8])<<8 | uint16(b[9])
	code := string([]byte{byte('A' - 1 + (id>>10)&0x1F), byte('A' - 1 + (id>>5)&0x1F), byte('A' - 1 + id&0x1F)})
	if n, ok := pnpMakers[code]; ok {
		e.maker = n
	} else {
		e.maker = code
	}
	if wcm, hcm := float64(b[21]), float64(b[22]); wcm > 0 && hcm > 0 {
		e.inches = math.Round(math.Hypot(wcm, hcm)/2.54*10) / 10
	}
	for d := 54; d+18 <= 126; d += 18 {
		if b[d] == 0 && b[d+1] == 0 && b[d+3] == 0xFC { // nombre del monitor
			e.name = strings.TrimSpace(strings.TrimRight(string(b[d+5:d+18]), "\n \x00"))
		}
	}
	return e
}
