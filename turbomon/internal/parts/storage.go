package parts

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"golang.org/x/sys/windows"
)

const (
	ioctlStorageQueryProperty = 0x2D1400
	ioctlDiskGeometryEx       = 0x700A0
	ioctlVolumeDiskExtents    = 0x560000
	ioctlAtaPassThrough       = 0x4D02C
)

// Disk es un disco físico.
type Disk struct {
	Number   int      `json:"number"`
	Model    string   `json:"model"`
	Maker    string   `json:"maker"`
	Firmware string   `json:"firmware"`
	Bus      string   `json:"bus"`  // NVMe, SATA, USB, RAID…
	Kind     string   `json:"kind"` // nvme, ssd, hdd, usb
	Size     uint64   `json:"size"`
	Letters  []string `json:"letters"`
	RPM      int      `json:"rpm,omitempty"` // discos duros
	MaxMBps  int      `json:"maxMBps"`       // lo máximo que permite su conexión (aprox.)
	TempC    int      `json:"tempC"`         // 0 = no se sabe
	Health   *Health  `json:"health,omitempty"`
}

// Health es la salud del disco (SMART).
type Health struct {
	UsedPct         int     `json:"usedPct"`  // desgaste (0-100+)
	SparePct        int     `json:"sparePct"` // reserva que le queda (NVMe)
	WrittenTB       float64 `json:"writtenTB"`
	ReadTB          float64 `json:"readTB"`
	PowerOnHours    int     `json:"powerOnHours"`
	PowerCycles     int     `json:"powerCycles"`
	UnsafeShutdowns int     `json:"unsafeShutdowns"`
	MediaErrors     int     `json:"mediaErrors"`
	Reallocated     int     `json:"reallocated"` // sectores reasignados (SATA)
	Warning         bool    `json:"warning"`     // el propio disco avisa de un problema
	tempK           int     // temperatura del registro SMART (kelvin)
}

var busNames = map[uint32]string{1: "SCSI", 2: "ATAPI", 3: "ATA", 7: "USB", 8: "RAID", 10: "SAS", 11: "SATA", 12: "SD", 13: "MMC", 17: "NVMe", 18: "SCM", 19: "UFS"}

func openDisk(n int, access uint32) (windows.Handle, error) {
	p, _ := windows.UTF16PtrFromString(fmt.Sprintf(`\\.\PhysicalDrive%d`, n))
	return windows.CreateFile(p, access, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
}

func ioctl(h windows.Handle, code uint32, in []byte, outSize int) ([]byte, bool) {
	out := make([]byte, outSize)
	var n uint32
	var ip *byte
	if len(in) > 0 {
		ip = &in[0]
	}
	if windows.DeviceIoControl(h, code, ip, uint32(len(in)), &out[0], uint32(len(out)), &n, nil) != nil {
		return nil, false
	}
	return out[:n], true
}

func cstrAt(b []byte, off uint32) string {
	if off == 0 || int(off) >= len(b) {
		return ""
	}
	e := int(off)
	for e < len(b) && b[e] != 0 {
		e++
	}
	return strings.TrimSpace(string(b[off:e]))
}

func readDisks() []Disk {
	letters := diskLetters()
	var out []Disk
	for n := 0; n < 32; n++ {
		h, err := openDisk(n, 0)
		if err != nil {
			continue
		}
		d := Disk{Number: n, Letters: letters[n]}
		q := make([]byte, 12) // STORAGE_PROPERTY_QUERY: StorageDeviceProperty, estándar
		if b, ok := ioctl(h, ioctlStorageQueryProperty, q, 1024); ok && len(b) >= 40 {
			vendor := cstrAt(b, binary.LittleEndian.Uint32(b[12:]))
			product := cstrAt(b, binary.LittleEndian.Uint32(b[16:]))
			d.Firmware = cstrAt(b, binary.LittleEndian.Uint32(b[20:]))
			bus := binary.LittleEndian.Uint32(b[28:])
			d.Bus = busNames[bus]
			d.Model = strings.TrimSpace(strings.TrimSpace(vendor) + " " + product)
			if strings.EqualFold(vendor, "nvme") || vendor == "" {
				d.Model = product
			}
			switch bus {
			case 17:
				d.Kind = "nvme"
			case 7:
				d.Kind = "usb"
			}
		}
		if b, ok := ioctl(h, ioctlDiskGeometryEx, nil, 256); ok && len(b) >= 32 {
			d.Size = binary.LittleEndian.Uint64(b[24:])
		}
		// ¿Gira? (SSD o disco duro)
		sp := make([]byte, 12)
		binary.LittleEndian.PutUint32(sp, 7) // StorageDeviceSeekPenaltyProperty
		if b, ok := ioctl(h, ioctlStorageQueryProperty, sp, 12); ok && len(b) >= 9 && d.Kind == "" {
			if b[8] != 0 {
				d.Kind = "hdd"
			} else {
				d.Kind = "ssd"
			}
		}
		if d.Kind == "" {
			d.Kind = "ssd"
		}
		d.TempC = diskTemp(h)
		windows.CloseHandle(h)
		if d.Size == 0 && d.Model == "" {
			continue
		}
		// Salud: hace falta abrir el disco con permiso de administrador.
		if ha, err := openDisk(n, windows.GENERIC_READ|windows.GENERIC_WRITE); err == nil {
			switch d.Kind {
			case "nvme":
				if hl := nvmeHealth(ha); hl != nil {
					d.Health = hl
					if d.TempC == 0 {
						d.TempC = hl.tempC()
					}
				}
			default:
				if rpm, model := ataIdentify(ha); rpm > 0 || model != "" {
					if rpm > 1 {
						d.RPM = rpm
						d.Kind = "hdd"
					}
					if d.Model == "" {
						d.Model = model
					}
				}
				if hl, t := ataSMART(ha); hl != nil {
					d.Health = hl
					if d.TempC == 0 {
						d.TempC = t
					}
				}
			}
			windows.CloseHandle(ha)
		}
		d.Maker = diskMaker(d.Model)
		switch d.Kind {
		case "hdd":
			d.MaxMBps = 250
		case "ssd":
			if d.Bus == "SATA" || d.Bus == "ATA" {
				d.MaxMBps = 560
			}
		}
		out = append(out, d)
	}
	return out
}

func (h *Health) tempC() int {
	if h.tempK > 273 {
		return h.tempK - 273
	}
	return 0
}

// diskTemp lee la temperatura que da Windows (funciona con muchos NVMe y algunos SATA).
func diskTemp(h windows.Handle) int {
	q := make([]byte, 12)
	binary.LittleEndian.PutUint32(q, 52) // StorageDeviceTemperatureProperty
	b, ok := ioctl(h, ioctlStorageQueryProperty, q, 256)
	if !ok || len(b) < 40 {
		return 0
	}
	count := int(binary.LittleEndian.Uint16(b[12:]))
	best := 0
	for i := 0; i < count && 24+16*i+4 <= len(b); i++ {
		t := int(int16(binary.LittleEndian.Uint16(b[24+16*i+2:])))
		if t > best && t < 150 {
			best = t
		}
	}
	return best
}

func nvmeHealth(h windows.Handle) *Health {
	// STORAGE_PROPERTY_QUERY (8) + STORAGE_PROTOCOL_SPECIFIC_DATA (40) + 512 de datos
	in := make([]byte, 8+40+512)
	binary.LittleEndian.PutUint32(in[0:], 50) // StorageDeviceProtocolSpecificProperty
	binary.LittleEndian.PutUint32(in[4:], 0)  // PropertyStandardQuery
	p := in[8:]
	binary.LittleEndian.PutUint32(p[0:], 3)   // ProtocolTypeNvme
	binary.LittleEndian.PutUint32(p[4:], 2)   // NVMeDataTypeLogPage
	binary.LittleEndian.PutUint32(p[8:], 2)   // registro SMART / salud
	binary.LittleEndian.PutUint32(p[16:], 40) // ProtocolDataOffset
	binary.LittleEndian.PutUint32(p[20:], 512)
	b, ok := ioctl(h, ioctlStorageQueryProperty, in, len(in))
	if !ok || len(b) < 48+512 {
		return nil
	}
	off := 8 + int(binary.LittleEndian.Uint32(b[8+16:]))
	if off+512 > len(b) {
		return nil
	}
	return parseNVMeSMART(b[off : off+512])
}

func u128(b []byte) float64 {
	lo := binary.LittleEndian.Uint64(b)
	hi := binary.LittleEndian.Uint64(b[8:])
	return float64(lo) + float64(hi)*math.Pow(2, 64)
}

func parseNVMeSMART(s []byte) *Health {
	if len(s) < 176 {
		return nil
	}
	h := &Health{
		Warning:         s[0] != 0,
		SparePct:        int(s[3]),
		UsedPct:         int(s[5]),
		ReadTB:          math.Round(u128(s[32:])*512000/1e12*100) / 100,
		WrittenTB:       math.Round(u128(s[48:])*512000/1e12*100) / 100,
		PowerCycles:     int(u128(s[112:])),
		PowerOnHours:    int(u128(s[128:])),
		UnsafeShutdowns: int(u128(s[144:])),
		MediaErrors:     int(u128(s[160:])),
	}
	h.tempK = int(binary.LittleEndian.Uint16(s[1:]))
	return h
}

// ataCmd manda una orden ATA (IDENTIFY o SMART) al disco y devuelve los 512 bytes.
func ataCmd(h windows.Handle, features, cylLow, cylHigh, command byte) []byte {
	buf := make([]byte, 48+512)
	binary.LittleEndian.PutUint16(buf[0:], 48)        // Length
	binary.LittleEndian.PutUint16(buf[2:], 0x02|0x01) // ATA_FLAGS_DATA_IN | DRDY_REQUIRED
	binary.LittleEndian.PutUint32(buf[8:], 512)       // DataTransferLength
	binary.LittleEndian.PutUint32(buf[12:], 3)        // TimeOutValue (s)
	binary.LittleEndian.PutUint64(buf[24:], 48)       // DataBufferOffset
	tf := buf[40:48]                                  // CurrentTaskFile
	tf[0], tf[1], tf[3], tf[4], tf[6] = features, 1, cylLow, cylHigh, command
	b, ok := ioctl(h, ioctlAtaPassThrough, buf, len(buf))
	if !ok || len(b) < 48+512 {
		return nil
	}
	return b[48 : 48+512]
}

func ataIdentify(h windows.Handle) (rpm int, model string) {
	id := ataCmd(h, 0, 0, 0, 0xEC)
	if id == nil {
		return 0, ""
	}
	// Modelo: palabras 27-46, con los bytes de cada palabra al revés.
	m := make([]byte, 40)
	for i := 0; i < 20; i++ {
		m[2*i], m[2*i+1] = id[54+2*i+1], id[54+2*i]
	}
	rot := int(binary.LittleEndian.Uint16(id[434:])) // palabra 217
	if rot == 1 || (rot >= 0x401 && rot < 0xFFFF) {
		rpm = rot
	}
	return rpm, strings.TrimSpace(string(m))
}

func ataSMART(h windows.Handle) (*Health, int) {
	d := ataCmd(h, 0xD0, 0x4F, 0xC2, 0xB0) // SMART READ DATA
	if d == nil {
		return nil, 0
	}
	hl := &Health{UsedPct: -1, SparePct: -1}
	temp := 0
	life := -1
	for i := 0; i < 30; i++ {
		a := d[2+12*i:]
		id := a[0]
		if id == 0 {
			continue
		}
		val := int(a[3])
		raw := uint64(a[5]) | uint64(a[6])<<8 | uint64(a[7])<<16 | uint64(a[8])<<24 | uint64(a[9])<<32 | uint64(a[10])<<40
		switch id {
		case 5:
			hl.Reallocated = int(raw)
		case 9:
			hl.PowerOnHours = int(raw & 0xFFFFFFFF)
		case 12:
			hl.PowerCycles = int(raw & 0xFFFFFFFF)
		case 194, 190:
			if t := int(raw & 0xFF); t > 0 && t < 120 && temp == 0 {
				temp = t
			}
		case 241:
			hl.WrittenTB = math.Round(float64(raw)*512/1e12*100) / 100
		case 177, 202, 231, 233, 169:
			// «Vida restante» normalizada (cada marca usa un número distinto).
			if val > 0 && val <= 100 && life < 0 {
				life = val
			}
		case 192:
			hl.UnsafeShutdowns = int(raw & 0xFFFFFFFF)
		}
	}
	if life >= 0 {
		hl.UsedPct = 100 - life
	}
	return hl, temp
}

// diskLetters dice en qué disco físico está cada letra de unidad.
func diskLetters() map[int][]string {
	m := map[int][]string{}
	for c := 'C'; c <= 'Z'; c++ {
		root := string(c) + `:\`
		rp, _ := windows.UTF16PtrFromString(root)
		t := windows.GetDriveType(rp)
		if t != windows.DRIVE_FIXED && t != windows.DRIVE_REMOVABLE {
			continue
		}
		p, _ := windows.UTF16PtrFromString(`\\.\` + string(c) + ":")
		h, err := windows.CreateFile(p, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
		if err != nil {
			continue
		}
		if b, ok := ioctl(h, ioctlVolumeDiskExtents, nil, 256); ok && len(b) >= 32 {
			n := int(binary.LittleEndian.Uint32(b[0:]))
			for i := 0; i < n && 8+24*i+4 <= len(b); i++ {
				disk := int(binary.LittleEndian.Uint32(b[8+24*i:]))
				m[disk] = append(m[disk], string(c)+":")
			}
		}
		windows.CloseHandle(h)
	}
	return m
}

var diskMakers = []struct{ prefix, name string }{
	{"samsung", "Samsung"}, {"wdc", "Western Digital"}, {"wd_black", "Western Digital"}, {"wd ", "Western Digital"},
	{"western digital", "Western Digital"}, {"kingston", "Kingston"}, {"crucial", "Crucial"}, {"ct", "Crucial"},
	{"micron", "Micron"}, {"st", "Seagate"}, {"seagate", "Seagate"}, {"toshiba", "Toshiba"}, {"kioxia", "Kioxia"},
	{"sk hynix", "SK hynix"}, {"hfm", "SK hynix"}, {"sabrent", "Sabrent"}, {"adata", "ADATA"}, {"xpg", "ADATA"},
	{"corsair", "Corsair"}, {"lexar", "Lexar"}, {"hikvision", "Hikvision"}, {"netac", "Netac"}, {"intel", "Intel"},
	{"teamgroup", "Team Group"}, {"t-force", "Team Group"}, {"patriot", "Patriot"}, {"pny", "PNY"},
	{"sandisk", "SanDisk"}, {"mushkin", "Mushkin"}, {"solidigm", "Solidigm"}, {"hitachi", "Hitachi"}, {"hgst", "HGST"},
	{"msi", "MSI"}, {"gigabyte", "Gigabyte"}, {"apacer", "Apacer"}, {"fikwot", "Fikwot"}, {"kingspec", "KingSpec"},
}

func diskMaker(model string) string {
	l := strings.ToLower(model)
	for _, m := range diskMakers {
		if strings.HasPrefix(l, m.prefix) {
			return m.name
		}
	}
	return ""
}
