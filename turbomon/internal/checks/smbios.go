package checks

// Módulos de RAM, leídos de la tabla SMBIOS del firmware (la misma que usa
// Windows): velocidad a la que van, a la que pueden ir, ranura y modelo.

import (
	"encoding/binary"
	"regexp"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modKernel   = windows.NewLazySystemDLL("kernel32.dll")
	pGetFwTable = modKernel.NewProc("GetSystemFirmwareTable")
)

// DIMM es un módulo de RAM instalado.
type DIMM struct {
	SizeMB     uint64 `json:"sizeMB"`
	Type       string `json:"type"` // DDR4, DDR5…
	Speed      int    `json:"speed"`      // MT/s máximos según el módulo (JEDEC)
	Configured int    `json:"configured"` // MT/s a los que va ahora
	Rated      int    `json:"rated"`      // MT/s de su perfil XMP/EXPO (por el modelo), si se sabe
	Locator    string `json:"locator"`
	Bank       string `json:"bank"`
	Maker      string `json:"maker"`
	Part       string `json:"part"`
}

func readSMBIOS() []byte {
	const rsmb = 0x52534D42
	n, _, _ := pGetFwTable.Call(rsmb, 0, 0, 0)
	if n == 0 {
		return nil
	}
	buf := make([]byte, n)
	r, _, _ := pGetFwTable.Call(rsmb, 0, uintptr(unsafe.Pointer(&buf[0])), n)
	if r == 0 || len(buf) < 8 {
		return nil
	}
	return buf[8:] // RawSMBIOSData: 8 bytes de cabecera
}

// DIMMs devuelve los módulos ocupados.
func DIMMs() []DIMM { return parseDIMMs(readSMBIOS()) }

func memType(t byte) string {
	switch t {
	case 0x18:
		return "DDR3"
	case 0x1A:
		return "DDR4"
	case 0x1B:
		return "LPDDR"
	case 0x1C:
		return "LPDDR2"
	case 0x1D:
		return "LPDDR3"
	case 0x1E:
		return "LPDDR4"
	case 0x22:
		return "DDR5"
	case 0x23:
		return "LPDDR5"
	}
	return ""
}

func parseDIMMs(data []byte) []DIMM {
	var out []DIMM
	for off := 0; off+4 <= len(data); {
		typ, ln := data[off], int(data[off+1])
		if ln < 4 || off+ln > len(data) {
			break
		}
		f := data[off : off+ln]
		// Las cadenas van detrás, separadas por ceros, y acaban en dos ceros.
		end := off + ln
		var strs []string
		for end < len(data) {
			k := end
			for k < len(data) && data[k] != 0 {
				k++
			}
			if k == end {
				end++
				break
			}
			strs = append(strs, strings.TrimSpace(string(data[end:k])))
			end = k + 1
		}
		if end < len(data) && data[end] == 0 && len(strs) == 0 {
			end++
		}
		str := func(i int) string {
			if i < len(f) && f[i] > 0 && int(f[i]) <= len(strs) {
				return strs[f[i]-1]
			}
			return ""
		}
		u16 := func(i int) int {
			if i+2 <= len(f) {
				return int(binary.LittleEndian.Uint16(f[i:]))
			}
			return 0
		}
		if typ == 127 {
			break
		}
		if typ == 17 && ln >= 0x15 {
			size := uint64(u16(0x0C))
			switch {
			case size == 0 || size == 0xFFFF:
				size = 0
			case size == 0x7FFF && ln >= 0x20:
				size = uint64(binary.LittleEndian.Uint32(f[0x1C:]))
			case size&0x8000 != 0:
				size = (size & 0x7FFF) / 1024 // en KB
			}
			if size > 0 {
				d := DIMM{SizeMB: size, Locator: str(0x10), Bank: str(0x11), Type: memType(f[0x12])}
				if ln > 0x16 {
					d.Speed = u16(0x15)
					if d.Speed == 0xFFFF && ln >= 0x58 {
						d.Speed = int(binary.LittleEndian.Uint32(f[0x54:]))
					}
				}
				if ln > 0x1A {
					d.Maker, d.Part = str(0x17), str(0x1A)
				}
				if ln >= 0x22 {
					d.Configured = u16(0x20)
					if d.Configured == 0xFFFF && ln >= 0x5C {
						d.Configured = int(binary.LittleEndian.Uint32(f[0x58:]))
					}
				}
				d.Rated = PartSpeed(d.Part)
				out = append(out, d)
			}
		}
		if end <= off {
			break
		}
		off = end
	}
	return out
}

var partPatterns = []struct {
	re   *regexp.Regexp
	mult int
}{
	{regexp.MustCompile(`(?i)([1-9]\d{3})C\d{2}`), 1},           // Corsair CMK16GX4M2B3200C16, Kingston KF3200C16, G.Skill F4-3600C16
	{regexp.MustCompile(`(?i)([1-9]\d{3})[HJ]C?\d{2}`), 1},       // Team TF3D416G3200HC16F, G.Skill F5-6000J3038
	{regexp.MustCompile(`(?i)^F[345]-([1-9]\d{3})`), 1},           // G.Skill
	{regexp.MustCompile(`(?i)^(?:KF|HX)[3456](\d{2})C`), 100},     // Kingston FURY KF436C16, KF560C40
	{regexp.MustCompile(`(?i)^(?:BL|CP|CT)\d+G(\d{2})C`), 100},    // Crucial Ballistix/Pro BL8G32C16, CP16G56C46
	{regexp.MustCompile(`(?i)^KVR(\d{2})`), 100},                  // Kingston ValueRAM (JEDEC)
}

// PartSpeed saca del modelo del módulo la velocidad para la que está hecho
// (la de su perfil XMP/EXPO). 0 si no se reconoce.
func PartSpeed(part string) int {
	part = strings.TrimSpace(part)
	for _, p := range partPatterns {
		m := p.re.FindStringSubmatch(part)
		if m == nil {
			continue
		}
		v, _ := strconv.Atoi(m[1])
		v *= p.mult
		if v >= 1600 && v <= 10000 {
			return v
		}
	}
	return 0
}

var chanRe = regexp.MustCompile(`(?i)(?:channel\s*|chan\s*|ch\s*)([a-h])\b|(?:dimm|ddr\d|ch)[_\s-]*([a-h])\d|\b([a-h])\d\b|(?:^|[_\s-])([a-h])\d?$`)

// Channel intenta saber en qué canal está un módulo por el nombre de su
// ranura («DDR4-A2», «ChannelB-DIMM0», «DIMM_A1»…). "" si no se sabe.
func Channel(d DIMM) string {
	for _, s := range []string{d.Locator, d.Bank} {
		m := chanRe.FindStringSubmatch(strings.TrimSpace(s))
		if m == nil {
			continue
		}
		for _, g := range m[1:] {
			if g != "" {
				return strings.ToUpper(g)
			}
		}
	}
	return ""
}
