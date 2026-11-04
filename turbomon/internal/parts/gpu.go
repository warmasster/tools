package parts

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"turbomon/internal/sensors"
)

// GPU es una tarjeta gráfica con lo que se sabe de ella.
type GPU struct {
	Name          string `json:"name"`
	Vendor        string `json:"vendor"`  // nvidia, amd, intel
	Partner       string `json:"partner"` // quien la fabrica (ASUS, MSI…)
	VRAM          uint64 `json:"vram"`
	Dedicated     bool   `json:"dedicated"`
	Outputs       int    `json:"outputs"`
	DriverVersion string `json:"driverVersion"`
	DriverDate    string `json:"driverDate"` // aaaa-mm-dd
	LinkGen       int    `json:"linkGen"`    // PCIe a la que va ahora (en reposo baja para ahorrar)
	LinkWidth     int    `json:"linkWidth"`
	MaxGen        int    `json:"maxGen"`
	MaxWidth      int    `json:"maxWidth"`
	ReBAR         int    `json:"rebar"` // 1 activado, 0 no, -1 no se sabe
	BARMB         int    `json:"barMB"`
}

// Fabricantes de tarjetas, por el «subsistema» PCI.
var boardPartners = map[uint32]string{
	0x1043: "ASUS", 0x1462: "MSI", 0x1458: "Gigabyte", 0x3842: "EVGA", 0x19DA: "Zotac", 0x10B0: "Gainward",
	0x1569: "Palit", 0x196E: "PNY", 0x1DA2: "Sapphire", 0x148C: "PowerColor", 0x1682: "XFX", 0x1849: "ASRock",
	0x7377: "Colorful", 0x1B4C: "Galax", 0x10DE: "NVIDIA (Founders Edition)", 0x1002: "AMD (modelo de referencia)",
	0x8086: "Intel", 0x1028: "Dell", 0x103C: "HP", 0x17AA: "Lenovo", 0x1025: "Acer", 0x1558: "Clevo",
	0x174B: "PC Partner", 0x1ACC: "Point of View", 0x152D: "Quanta", 0x1D05: "Tongfang", 0x1D72: "Xiaomi",
}

func readGPUs() []GPU {
	var out []GPU
	for _, a := range sensors.Adapters() {
		g := GPU{Name: a.Name, Vendor: a.Vendor, VRAM: a.VRAM, Dedicated: a.Dedicated, Outputs: a.Outputs, ReBAR: -1}
		if p, ok := boardPartners[a.SubSysID&0xFFFF]; ok {
			g.Partner = p
		}
		prefix := hexPrefix(a.VendorID, a.DeviceID)
		sub := fmt.Sprintf("SUBSYS_%08X", a.SubSysID)
		eachDevice(classDisplay, func(d device) bool {
			match := false
			for _, id := range d.ids {
				u := strings.ToUpper(id)
				if strings.HasPrefix(u, prefix) && (a.SubSysID == 0 || strings.Contains(u, sub)) {
					match = true
					break
				}
			}
			if !match {
				return true
			}
			g.DriverVersion = devPropStr(d.set, d.data, keyDrvVersion)
			if t := devPropTime(d.set, d.data, keyDrvDate); !t.IsZero() {
				g.DriverDate = t.Format("2006-01-02")
			}
			if v, ok := devPropU32(d.set, d.data, keyLinkSpeed); ok {
				g.LinkGen = int(v)
			}
			if v, ok := devPropU32(d.set, d.data, keyLinkWidth); ok {
				g.LinkWidth = int(v)
			}
			if v, ok := devPropU32(d.set, d.data, keyMaxSpeed); ok {
				g.MaxGen = int(v)
			}
			if v, ok := devPropU32(d.set, d.data, keyMaxWidth); ok {
				g.MaxWidth = int(v)
			}
			// Resizable BAR: sin él, el procesador ve la VRAM por una ventana de
			// 256 MB; con él, la ve entera.
			if a.Dedicated {
				if bar := largestMemRange(d.data.DevInst); bar > 0 {
					g.BARMB = int(bar >> 20)
					if bar >= 1<<30 {
						g.ReBAR = 1
					} else {
						g.ReBAR = 0
					}
				}
			}
			return false
		})
		out = append(out, g)
	}
	return out
}

// NvidiaVersion pasa «32.0.16.1664» a «616.64», que es como lo llama NVIDIA.
func NvidiaVersion(v string) string {
	p := strings.Split(v, ".")
	if len(p) != 4 || len(p[2]) < 2 {
		return v
	}
	s := p[2][len(p[2])-1:] + fmt.Sprintf("%04s", p[3])
	s = strings.ReplaceAll(s, " ", "0")
	if len(s) < 5 {
		return v
	}
	return s[:3] + "." + s[3:]
}

// DriverAge dice cuántos días tiene el driver (fecha aaaa-mm-dd).
func DriverAge(date string) int {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return -1
	}
	return int(time.Since(t).Hours() / 24)
}

// gpuIDs devuelve los adaptadores por fabricante de chip (para «a medida»).
func gpuVendors(gs []GPU) (nvidia, amd, intel bool) {
	for _, g := range gs {
		switch g.Vendor {
		case "nvidia":
			nvidia = true
		case "amd":
			amd = true
		case "intel":
			intel = true
		}
	}
	return
}

var _ = windows.DIGCF_PRESENT
