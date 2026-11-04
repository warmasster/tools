// Package parts saca el modelo exacto de cada pieza del PC con lo que trae
// Windows: la tabla SMBIOS del firmware, SetupAPI, los discos, el EDID de las
// pantallas, la red y los dispositivos HID. Sin drivers propios.
package parts

import (
	"encoding/binary"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modKernel   = windows.NewLazySystemDLL("kernel32.dll")
	pGetFwTable = modKernel.NewProc("GetSystemFirmwareTable")
)

// Structure es una estructura de la tabla SMBIOS.
type Structure struct {
	Type    byte
	Data    []byte // zona con formato (incluye la cabecera de 4 bytes)
	Strings []string
}

// Str devuelve la cadena a la que apunta el byte off.
func (s Structure) Str(off int) string {
	if off < len(s.Data) && s.Data[off] > 0 && int(s.Data[off]) <= len(s.Strings) {
		return cleanStr(s.Strings[s.Data[off]-1])
	}
	return ""
}

// U8 lee un byte.
func (s Structure) U8(off int) int {
	if off < len(s.Data) {
		return int(s.Data[off])
	}
	return 0
}

// U16 lee un entero de 16 bits.
func (s Structure) U16(off int) int {
	if off+2 <= len(s.Data) {
		return int(binary.LittleEndian.Uint16(s.Data[off:]))
	}
	return 0
}

// U32 lee un entero de 32 bits.
func (s Structure) U32(off int) uint32 {
	if off+4 <= len(s.Data) {
		return binary.LittleEndian.Uint32(s.Data[off:])
	}
	return 0
}

// Los fabricantes rellenan con frases de relleno: se quitan.
var junk = map[string]bool{
	"to be filled by o.e.m.": true, "to be filled by o.e.m": true, "default string": true, "system product name": true,
	"system manufacturer": true, "not applicable": true, "not specified": true, "none": true, "unknown": true,
	"o.e.m.": true, "oem": true, "base board product name": true, "type1productconfigid": true, "x.x": true,
}

func cleanStr(s string) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 32 {
			return -1
		}
		return r
	}, s))
	if junk[strings.ToLower(s)] {
		return ""
	}
	return s
}

var (
	smbiosOnce sync.Once
	smbiosData []Structure
)

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

// Structures devuelve la tabla SMBIOS (se lee una vez).
func Structures() []Structure {
	smbiosOnce.Do(func() { smbiosData = parseSMBIOS(readSMBIOS()) })
	return smbiosData
}

// ByType devuelve las estructuras de un tipo.
func ByType(t byte) []Structure {
	var out []Structure
	for _, s := range Structures() {
		if s.Type == t {
			out = append(out, s)
		}
	}
	return out
}

func parseSMBIOS(data []byte) []Structure {
	var out []Structure
	for off := 0; off+4 <= len(data); {
		typ, ln := data[off], int(data[off+1])
		if ln < 4 || off+ln > len(data) {
			break
		}
		f := data[off : off+ln]
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
			strs = append(strs, string(data[end:k]))
			end = k + 1
		}
		if end < len(data) && data[end] == 0 && len(strs) == 0 {
			end++
		}
		out = append(out, Structure{Type: typ, Data: f, Strings: strs})
		if typ == 127 || end <= off {
			break
		}
		off = end
	}
	return out
}

// ---------- RAM ----------

// DIMM es un módulo de RAM instalado.
type DIMM struct {
	SizeMB     uint64 `json:"sizeMB"`
	Type       string `json:"type"`       // DDR4, DDR5…
	Speed      int    `json:"speed"`      // MT/s máximos según el módulo (JEDEC)
	Configured int    `json:"configured"` // MT/s a los que va ahora
	Rated      int    `json:"rated"`      // MT/s de su perfil XMP/EXPO (por el modelo), si se sabe
	Locator    string `json:"locator"`
	Bank       string `json:"bank"`
	Maker      string `json:"maker"`
	Part       string `json:"part"`
	Ranks      int    `json:"ranks"`
	Voltage    int    `json:"voltage"` // mV configurados
}

// DIMMs devuelve los módulos ocupados.
func DIMMs() []DIMM { return dimmsFrom(Structures()) }

func memType(t int) string {
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

func dimmsFrom(ss []Structure) []DIMM {
	var out []DIMM
	for _, s := range ss {
		if s.Type != 17 || len(s.Data) < 0x15 {
			continue
		}
		size := uint64(s.U16(0x0C))
		switch {
		case size == 0 || size == 0xFFFF:
			size = 0
		case size == 0x7FFF && len(s.Data) >= 0x20:
			size = uint64(s.U32(0x1C))
		case size&0x8000 != 0:
			size = (size & 0x7FFF) / 1024 // en KB
		}
		if size == 0 {
			continue
		}
		d := DIMM{SizeMB: size, Locator: s.Str(0x10), Bank: s.Str(0x11), Type: memType(s.U8(0x12))}
		if len(s.Data) > 0x16 {
			d.Speed = s.U16(0x15)
			if d.Speed == 0xFFFF && len(s.Data) >= 0x58 {
				d.Speed = int(s.U32(0x54))
			}
		}
		if len(s.Data) > 0x1A {
			d.Maker, d.Part = s.Str(0x17), s.Str(0x1A)
		}
		if len(s.Data) > 0x1B {
			d.Ranks = s.U8(0x1B) & 0x0F
		}
		if len(s.Data) >= 0x22 {
			d.Configured = s.U16(0x20)
			if d.Configured == 0xFFFF && len(s.Data) >= 0x5C {
				d.Configured = int(s.U32(0x58))
			}
		}
		if len(s.Data) >= 0x28 {
			d.Voltage = s.U16(0x26)
		}
		d.Maker = memMaker(d.Maker, d.Part)
		d.Rated = PartSpeed(d.Part)
		out = append(out, d)
	}
	return out
}

// Algunos módulos dan el fabricante como código JEDEC («04CD») en vez de nombre.
var jedecMakers = map[string]string{
	"04cd": "G.Skill", "9e": "Corsair", "029e": "Corsair", "0198": "Kingston", "98": "Kingston", "859b": "Crucial",
	"9b": "Crucial", "802c": "Micron", "2c": "Micron", "80ce": "Samsung", "ce": "Samsung", "80ad": "SK hynix",
	"ad": "SK hynix", "0b04": "Team Group", "ef": "Team Group", "04ef": "Team Group", "0502": "Patriot",
	"8551": "Qimonda", "0443": "ADATA", "04cb": "ADATA", "cb": "ADATA", "8502": "Patriot", "059b": "Crucial",
}

func memMaker(m, part string) string {
	l := strings.ToLower(strings.TrimSpace(m))
	if n, ok := jedecMakers[strings.TrimPrefix(l, "0x")]; ok {
		return n
	}
	p := strings.ToUpper(part)
	switch {
	case l != "" && !strings.HasPrefix(l, "unknown") && len(l) > 3:
		return strings.TrimSpace(m)
	case strings.HasPrefix(p, "KF") || strings.HasPrefix(p, "KVR") || strings.HasPrefix(p, "HX"):
		return "Kingston"
	case strings.HasPrefix(p, "CM"):
		return "Corsair"
	case strings.HasPrefix(p, "F4-") || strings.HasPrefix(p, "F5-"):
		return "G.Skill"
	case strings.HasPrefix(p, "BL") || strings.HasPrefix(p, "CT") || strings.HasPrefix(p, "CP"):
		return "Crucial"
	}
	return strings.TrimSpace(m)
}

var partPatterns = []struct {
	re   *regexp.Regexp
	mult int
}{
	{regexp.MustCompile(`(?i)([1-9]\d{3})C\d{2}`), 1},          // Corsair CMK16GX4M2B3200C16, Kingston KF3200C16, G.Skill F4-3600C16
	{regexp.MustCompile(`(?i)([1-9]\d{3})[HJ]C?\d{2}`), 1},     // Team TF3D416G3200HC16F, G.Skill F5-6000J3038
	{regexp.MustCompile(`(?i)^F[345]-([1-9]\d{3})`), 1},        // G.Skill
	{regexp.MustCompile(`(?i)^(?:KF|HX)[3456](\d{2})C`), 100},  // Kingston FURY KF436C16, KF560C40
	{regexp.MustCompile(`(?i)^(?:BL|CP|CT)\d+G(\d{2})C`), 100}, // Crucial Ballistix/Pro BL8G32C16, CP16G56C46
	{regexp.MustCompile(`(?i)^KVR(\d{2})`), 100},               // Kingston ValueRAM (JEDEC)
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
