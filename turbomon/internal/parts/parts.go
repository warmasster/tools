package parts

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

// Components es todo el equipo.
type Components struct {
	Summary     string       `json:"summary"`
	CPU         CPU          `json:"cpu"`
	Board       Board        `json:"board"`
	OS          OS           `json:"os"`
	RAMTotal    uint64       `json:"ramTotal"`
	RAM         []DIMM       `json:"ram"`
	GPUs        []GPU        `json:"gpus"`
	Disks       []Disk       `json:"disks"`
	Monitors    []Monitor    `json:"monitors"`
	Network     []NetAdapter `json:"network"`
	Peripherals []Peripheral `json:"peripherals"`
	Battery     *Battery     `json:"battery,omitempty"`
	Time        int64        `json:"time"`
}

var (
	cacheMu sync.Mutex
	cached  *Components
	cacheAt time.Time
)

// Read lee todas las piezas. Se guarda 30 s (las temperaturas cambian, el resto no).
func Read(totalRAM uint64) Components {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if cached != nil && time.Since(cacheAt) < 30*time.Second {
		return *cached
	}
	c := Components{CPU: readCPU(), Board: readBoard(), OS: readOS(), RAMTotal: totalRAM, RAM: DIMMs(), GPUs: readGPUs(),
		Disks: readDisks(), Monitors: Monitors(), Network: readNetwork(), Peripherals: readPeripherals(), Battery: readBattery(),
		Time: time.Now().Unix()}
	if c.Board.Laptop && c.Battery == nil {
		c.Board.Laptop = false // un «portátil» sin batería: mejor no suponer
	}
	c.Summary = summary(c)
	cached, cacheAt = &c, time.Now()
	return c
}

func gbStr(b uint64) string {
	v := float64(b) / (1 << 30)
	return strings.Replace(fmt.Sprintf("%.0f GB", math.Round(v)), ".", ",", 1)
}

func diskSize(b uint64) string {
	tb := float64(b) / 1e12
	if tb >= 0.95 {
		s := fmt.Sprintf("%.1f", tb)
		s = strings.TrimSuffix(s, ".0")
		return strings.Replace(s, ".", ",", 1) + " TB"
	}
	return fmt.Sprintf("%.0f GB", float64(b)/1e9)
}

// DiskSize da el tamaño como lo pone en la caja (1 TB, 500 GB).
func DiskSize(b uint64) string { return diskSize(b) }

func summary(c Components) string {
	var parts []string
	if c.CPU.Name != "" {
		parts = append(parts, shortCPU(c.CPU.Name))
	}
	for _, g := range c.GPUs {
		if g.Dedicated {
			parts = append(parts, shortGPU(g.Name)+" "+gbStr(g.VRAM))
			break
		}
	}
	if c.RAMTotal > 0 {
		r := gbStr(c.RAMTotal)
		if len(c.RAM) > 0 && c.RAM[0].Type != "" {
			sp := c.RAM[0].Configured
			if sp == 0 {
				sp = c.RAM[0].Speed
			}
			r += fmt.Sprintf(" %s-%d", c.RAM[0].Type, sp)
		}
		parts = append(parts, r)
	}
	for _, d := range c.Disks {
		for _, l := range d.Letters {
			if l == "C:" {
				kind := map[string]string{"nvme": "NVMe", "ssd": "SSD", "hdd": "disco duro", "usb": "USB"}[d.Kind]
				parts = append(parts, diskSize(d.Size)+" "+kind)
			}
		}
	}
	return strings.Join(parts, " · ")
}

func shortCPU(n string) string {
	r := strings.NewReplacer("(R)", "", "(TM)", "", "(tm)", "", " CPU", "", " Processor", "", "  ", " ")
	n = r.Replace(n)
	if i := strings.Index(n, " @"); i > 0 {
		n = n[:i]
	}
	for _, s := range []string{"-Core", " with Radeon Graphics"} {
		if i := strings.Index(n, s); i > 0 {
			n = strings.TrimSpace(n[:i])
			if j := strings.LastIndex(n, " "); s == "-Core" && j > 0 {
				n = n[:j]
			}
		}
	}
	return strings.TrimSpace(n)
}

func shortGPU(n string) string {
	return strings.TrimSpace(strings.NewReplacer("NVIDIA GeForce ", "", "AMD Radeon ", "Radeon ", "Intel(R) ", "", "(TM)", "").Replace(n))
}
