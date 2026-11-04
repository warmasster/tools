package parts

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

var (
	pLPIEx      = modKernel.NewProc("GetLogicalProcessorInformationEx")
	pGetCpuSets = modKernel.NewProc("GetSystemCpuSetInformation")
)

// CPU es el procesador.
type CPU struct {
	Name      string `json:"name"`
	Vendor    string `json:"vendor"` // intel, amd u otro
	Socket    string `json:"socket"`
	Cores     int    `json:"cores"`
	Threads   int    `json:"threads"`
	PCores    int    `json:"pCores"` // núcleos de rendimiento (híbridos)
	ECores    int    `json:"eCores"` // núcleos de eficiencia
	BaseMHz   int    `json:"baseMHz"`
	MaxMHz    int    `json:"maxMHz"`
	L2KB      int    `json:"l2KB"`
	L3KB      int    `json:"l3KB"`
	L3Groups  int    `json:"l3Groups"` // bloques de caché L3 (chips o «CCD» en AMD)
	Microcode uint32 `json:"microcode"`
	Family    int    `json:"family"`
	Model     int    `json:"model"`
	Stepping  int    `json:"stepping"`
}

// CPUSet es un procesador lógico tal como lo ve el planificador de Windows.
type CPUSet struct {
	ID         uint32 `json:"id"`
	Logical    int    `json:"logical"`
	Core       int    `json:"core"`
	Cache      int    `json:"cache"`      // índice de la caché de último nivel
	Efficiency int    `json:"efficiency"` // más alto = más rendimiento (en híbridos)
	Parked     bool   `json:"parked"`
}

// CPUSets enumera los procesadores lógicos (para elegir núcleos).
func CPUSets() []CPUSet {
	var n uint32
	pGetCpuSets.Call(0, 0, uintptr(unsafe.Pointer(&n)), 0, 0)
	if n == 0 {
		return nil
	}
	buf := make([]byte, n)
	if r, _, _ := pGetCpuSets.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(n), uintptr(unsafe.Pointer(&n)), 0, 0); r == 0 {
		return nil
	}
	var out []CPUSet
	for off := uint32(0); off+32 <= n; {
		size := binary.LittleEndian.Uint32(buf[off:])
		if size == 0 {
			break
		}
		b := buf[off:]
		out = append(out, CPUSet{
			ID: binary.LittleEndian.Uint32(b[8:]), Logical: int(b[14]), Core: int(b[15]),
			Cache: int(b[16]), Efficiency: int(b[18]), Parked: b[19]&0x1 != 0,
		})
		off += size
	}
	return out
}

func readCPU() CPU {
	c := CPU{}
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE); err == nil {
		s, _, _ := k.GetStringValue("ProcessorNameString")
		c.Name = strings.Join(strings.Fields(s), " ")
		v, _, _ := k.GetStringValue("VendorIdentifier")
		switch {
		case strings.Contains(v, "Intel"):
			c.Vendor = "intel"
		case strings.Contains(v, "AMD"):
			c.Vendor = "amd"
		default:
			c.Vendor = "otro"
		}
		mhz, _, _ := k.GetIntegerValue("~MHz")
		c.BaseMHz = int(mhz)
		id, _, _ := k.GetStringValue("Identifier") // «Intel64 Family 6 Model 191 Stepping 2»
		if i := strings.Index(id, "Family"); i >= 0 {
			fmt.Sscanf(id[i:], "Family %d Model %d Stepping %d", &c.Family, &c.Model, &c.Stepping)
		}
		if b, _, err := k.GetBinaryValue("Update Revision"); err == nil && len(b) >= 4 {
			// Según la versión de Windows son 4 u 8 bytes (con 8, la revisión va en los altos).
			c.Microcode = binary.LittleEndian.Uint32(b[:4])
			if len(b) >= 8 {
				if hi := binary.LittleEndian.Uint32(b[4:]); hi != 0 {
					c.Microcode = hi
				}
			}
		}
		k.Close()
	}
	for _, s := range ByType(4) {
		if v := s.Str(0x04); v != "" {
			c.Socket = v
		}
		if v := s.U16(0x14); v > c.MaxMHz {
			c.MaxMHz = v
		}
		break
	}
	// Núcleos, hilos y cachés.
	sets := CPUSets()
	c.Threads = len(sets)
	cores := map[int]int{}
	maxEff := 0
	for _, s := range sets {
		cores[s.Core] = s.Efficiency
		if s.Efficiency > maxEff {
			maxEff = s.Efficiency
		}
	}
	c.Cores = len(cores)
	if maxEff > 0 {
		for _, e := range cores {
			if e == maxEff {
				c.PCores++
			} else {
				c.ECores++
			}
		}
	}
	var size uint32
	pLPIEx.Call(2, 0, uintptr(unsafe.Pointer(&size))) // RelationCache
	if size > 0 {
		buf := make([]byte, size)
		if r, _, _ := pLPIEx.Call(2, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))); r != 0 {
			for off := uint32(0); off+24 <= size; {
				rec := binary.LittleEndian.Uint32(buf[off+4:])
				if rec == 0 {
					break
				}
				level := buf[off+8]
				kb := int(binary.LittleEndian.Uint32(buf[off+12:]) / 1024)
				typ := binary.LittleEndian.Uint32(buf[off+16:])
				if typ == 0 || typ == 2 { // unificada o de datos
					switch level {
					case 2:
						c.L2KB += kb
					case 3:
						c.L3KB += kb
						c.L3Groups++
					}
				}
				off += rec
			}
		}
	}
	return c
}

// Board es la placa base (o el modelo del equipo, en portátiles y PC de marca).
type Board struct {
	Maker       string `json:"maker"`
	Model       string `json:"model"`
	Version     string `json:"version"`
	BIOSVendor  string `json:"biosVendor"`
	BIOSVersion string `json:"biosVersion"`
	BIOSDate    string `json:"biosDate"` // mm/dd/aaaa como lo da el firmware
	SystemMaker string `json:"systemMaker"`
	SystemModel string `json:"systemModel"`
	Laptop      bool   `json:"laptop"`
}

func readBoard() Board {
	var b Board
	for _, s := range ByType(2) {
		b.Maker, b.Model, b.Version = s.Str(0x04), s.Str(0x05), s.Str(0x06)
		break
	}
	for _, s := range ByType(0) {
		b.BIOSVendor, b.BIOSVersion, b.BIOSDate = s.Str(0x04), s.Str(0x05), s.Str(0x08)
		break
	}
	for _, s := range ByType(1) {
		b.SystemMaker, b.SystemModel = s.Str(0x04), s.Str(0x05)
		break
	}
	for _, s := range ByType(3) {
		switch s.U8(0x05) & 0x7F { // tipo de chasis
		case 8, 9, 10, 11, 14, 30, 31, 32:
			b.Laptop = true
		}
		break
	}
	b.Maker = prettyMaker(b.Maker)
	b.SystemMaker = prettyMaker(b.SystemMaker)
	return b
}

var makerNames = map[string]string{
	"asustek computer inc.": "ASUS", "asustek computer inc": "ASUS", "micro-star international co., ltd.": "MSI",
	"micro-star international co., ltd": "MSI", "gigabyte technology co., ltd.": "Gigabyte", "asrock": "ASRock",
	"hewlett-packard": "HP", "dell inc.": "Dell", "lenovo": "Lenovo", "acer": "Acer", "intel corporation": "Intel",
	"biostar group": "Biostar", "nzxt": "NZXT", "evga": "EVGA",
}

func prettyMaker(s string) string {
	if n, ok := makerNames[strings.ToLower(strings.TrimSpace(s))]; ok {
		return n
	}
	return s
}

// OS es la versión de Windows.
type OS struct {
	Name    string `json:"name"`
	Version string `json:"version"` // 24H2
	Build   string `json:"build"`   // 26100.4061
	Edition string `json:"edition"`
}

func readOS() OS {
	var o OS
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return o
	}
	defer k.Close()
	o.Name, _, _ = k.GetStringValue("ProductName")
	o.Version, _, _ = k.GetStringValue("DisplayVersion")
	build, _, _ := k.GetStringValue("CurrentBuild")
	ubr, _, _ := k.GetIntegerValue("UBR")
	o.Build = build
	if ubr > 0 {
		o.Build = fmt.Sprintf("%s.%d", build, ubr)
	}
	o.Edition, _, _ = k.GetStringValue("EditionID")
	// Windows 11 sigue diciendo «Windows 10» en ProductName.
	var nb int
	fmt.Sscanf(build, "%d", &nb)
	if nb >= 22000 {
		o.Name = strings.Replace(o.Name, "Windows 10", "Windows 11", 1)
	}
	return o
}

// WindowsBuild devuelve la compilación de Windows («26100.4061»).
func WindowsBuild() string { return readOS().Build }
