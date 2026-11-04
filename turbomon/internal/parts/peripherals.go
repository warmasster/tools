package parts

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	modHid         = windows.NewLazySystemDLL("hid.dll")
	pHidAttrs      = modHid.NewProc("HidD_GetAttributes")
	pHidProduct    = modHid.NewProc("HidD_GetProductString")
	pHidMaker      = modHid.NewProc("HidD_GetManufacturerString")
	pHidPreparsed  = modHid.NewProc("HidD_GetPreparsedData")
	pHidFreePrep   = modHid.NewProc("HidD_FreePreparsedData")
	pHidCaps       = modHid.NewProc("HidP_GetCaps")
	modXInput      = windows.NewLazySystemDLL("xinput1_4.dll")
	pXInputGetCaps = modXInput.NewProc("XInputGetCapabilities")
)

// Peripheral es un ratón, teclado, mando o dispositivo de audio.
type Peripheral struct {
	Kind  string `json:"kind"` // raton, teclado, mando, audio, microfono
	Name  string `json:"name"`
	Maker string `json:"maker"`
}

var usbMakers = map[uint16]string{
	0x046D: "Logitech", 0x1532: "Razer", 0x1B1C: "Corsair", 0x1038: "SteelSeries", 0x0951: "HyperX", 0x03F0: "HP",
	0x2516: "Cooler Master", 0x258A: "Glorious", 0x3554: "Pulsar", 0x1A7C: "Endgame Gear", 0x093A: "Pixart",
	0x045E: "Microsoft", 0x054C: "Sony", 0x057E: "Nintendo", 0x0B05: "ASUS", 0x1E7D: "ROCCAT", 0x04D9: "Holtek",
	0x3434: "Keychron", 0x0C45: "Sonix", 0x24AE: "Shenzhen Rapoo", 0x1EA7: "SHARKOON", 0x2DC8: "8BitDo", 0x0738: "Mad Catz",
	0x0E6F: "PDP", 0x0F0D: "HORI", 0x2F24: "GameSir", 0x3537: "GameSir", 0x1915: "Nordic", 0x373B: "Compx", 0x361D: "Finalmouse",
	0x2EA8: "Wooting", 0x31E3: "Wooting", 0x320F: "Glorious", 0x0D8C: "C-Media", 0x1395: "EPOS", 0x1E71: "NZXT",
}

func hidString(proc *windows.LazyProc, h windows.Handle) string {
	var buf [127]uint16
	if r, _, _ := proc.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)*2)); r == 0 {
		return ""
	}
	return strings.TrimSpace(windows.UTF16ToString(buf[:]))
}

func readPeripherals() []Peripheral {
	var out []Peripheral
	seen := map[string]int{}
	paths, _ := windows.CM_Get_Device_Interface_List("", &ifaceHID, windows.CM_GET_DEVICE_INTERFACE_LIST_PRESENT)
	for _, p := range paths {
		pp, _ := windows.UTF16PtrFromString(p)
		h, err := windows.CreateFile(pp, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
		if err != nil {
			continue
		}
		var attrs struct {
			Size                         uint32
			VendorID, ProductID, Version uint16
		}
		attrs.Size = uint32(unsafe.Sizeof(attrs))
		pHidAttrs.Call(uintptr(h), uintptr(unsafe.Pointer(&attrs)))
		kind := ""
		var prep uintptr
		if r, _, _ := pHidPreparsed.Call(uintptr(h), uintptr(unsafe.Pointer(&prep))); r != 0 {
			var caps [64]byte
			pHidCaps.Call(prep, uintptr(unsafe.Pointer(&caps[0])))
			usage, page := binary.LittleEndian.Uint16(caps[0:]), binary.LittleEndian.Uint16(caps[2:])
			switch {
			case page == 1 && usage == 2:
				kind = "raton"
			case page == 1 && usage == 6:
				kind = "teclado"
			case page == 1 && (usage == 4 || usage == 5):
				kind = "mando"
			}
			pHidFreePrep.Call(prep)
		}
		name := hidString(pHidProduct, h)
		maker := hidString(pHidMaker, h)
		windows.CloseHandle(h)
		if kind == "" || name == "" {
			continue
		}
		if m, ok := usbMakers[attrs.VendorID]; ok && len(maker) < 3 {
			maker = m
		}
		// Un mismo aparato expone varias «colecciones» (un ratón con teclas de
		// macro parece también un teclado): se queda uno por aparato.
		key := fmt.Sprintf("%04x:%04x", attrs.VendorID, attrs.ProductID)
		maker = cleanMaker(maker)
		if strings.EqualFold(maker, name) {
			maker = ""
		}
		if i, ok := seen[key]; ok {
			out[i].Kind = betterKind(out[i].Kind, kind, name)
			continue
		}
		seen[key] = len(out)
		out = append(out, Peripheral{Kind: betterKind("", kind, name), Name: name, Maker: maker})
	}
	// Mandos de Xbox (XInput no pasa por HID).
	if pXInputGetCaps.Find() == nil {
		for i := 0; i < 4; i++ {
			var caps [20]byte
			if r, _, _ := pXInputGetCaps.Call(uintptr(i), 1, uintptr(unsafe.Pointer(&caps[0]))); r == 0 {
				out = append(out, Peripheral{Kind: "mando", Name: "Mando de Xbox (o compatible)", Maker: "Microsoft"})
			}
		}
	}
	out = append(out, audioDevices()...)
	sort.SliceStable(out, func(i, j int) bool { return kindOrder(out[i].Kind) < kindOrder(out[j].Kind) })
	return out
}

func kindOrder(k string) int {
	return map[string]int{"raton": 0, "teclado": 1, "mando": 2, "audio": 3, "microfono": 4}[k]
}

// betterKind decide qué es un aparato con varias colecciones.
func betterKind(cur, nw, name string) string {
	l := strings.ToLower(name)
	switch {
	case strings.Contains(l, "mouse") || strings.Contains(l, "ratón"):
		return "raton"
	case strings.Contains(l, "keyboard") || strings.Contains(l, "teclado") || strings.Contains(l, "alloy") || strings.Contains(l, "keychron"):
		return "teclado"
	case strings.Contains(l, "controller") || strings.Contains(l, "gamepad") || strings.Contains(l, "joystick") || strings.Contains(l, "mando"):
		return "mando"
	}
	if cur == "" {
		return nw
	}
	if cur == "mando" || nw == "mando" {
		return "mando"
	}
	return "teclado" // un teclado con colección de ratón es más común que al revés
}

func cleanMaker(m string) string {
	for _, suf := range []string{" Inc.", " Inc", " Corporation", " Corp.", " Co., Ltd.", " Co.,Ltd.", " Ltd.", " GmbH", " International"} {
		m = strings.TrimSuffix(m, suf)
	}
	return strings.TrimRight(strings.TrimSpace(m), ", ")
}

// audioDevices lee los dispositivos de audio activos del registro de Windows.
func audioDevices() []Peripheral {
	var out []Peripheral
	for _, t := range []struct{ key, kind string }{{"Render", "audio"}, {"Capture", "microfono"}} {
		base := `SOFTWARE\Microsoft\Windows\CurrentVersion\MMDevices\Audio\` + t.key
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, base, registry.ENUMERATE_SUB_KEYS)
		if err != nil {
			continue
		}
		subs, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, s := range subs {
			dk, err := registry.OpenKey(registry.LOCAL_MACHINE, base+`\`+s, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			state, _, _ := dk.GetIntegerValue("DeviceState")
			dk.Close()
			if state != 1 { // DEVICE_STATE_ACTIVE
				continue
			}
			pk, err := registry.OpenKey(registry.LOCAL_MACHINE, base+`\`+s+`\Properties`, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			// {a45c254e-…},2 = nombre del dispositivo; {b3f8fa53-…},6 = nombre de la interfaz
			dev, _, _ := pk.GetStringValue("{b3f8fa53-0004-438e-9003-51a46e139bfc},6")
			name, _, _ := pk.GetStringValue("{a45c254e-df1c-4efd-8020-67d146a850e0},2")
			pk.Close()
			label := dev
			if label == "" {
				label = name
			} else if name != "" && !strings.EqualFold(name, dev) {
				label = dev + " · " + name
			}
			ld := strings.ToLower(label)
			if label != "" && !strings.Contains(ld, "nvidia high definition") && !strings.Contains(ld, "amd high definition") && !strings.Contains(ld, "intel(r) display audio") && !strings.Contains(ld, "salida digital") && !strings.Contains(ld, "digital output") {
				out = append(out, Peripheral{Kind: t.kind, Name: label})
			}
		}
	}
	return out
}

// Battery es la batería de un portátil.
type Battery struct {
	DesignWh float64 `json:"designWh"`
	FullWh   float64 `json:"fullWh"`
	Health   int     `json:"health"` // % de la capacidad de fábrica
	Cycles   int     `json:"cycles"`
}

func readBattery() *Battery {
	paths, _ := windows.CM_Get_Device_Interface_List("", &ifaceBattery, windows.CM_GET_DEVICE_INTERFACE_LIST_PRESENT)
	for _, p := range paths {
		pp, _ := windows.UTF16PtrFromString(p)
		h, err := windows.CreateFile(pp, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
		if err != nil {
			continue
		}
		var timeout, tag uint32
		var n uint32
		if windows.DeviceIoControl(h, 0x294040, (*byte)(unsafe.Pointer(&timeout)), 4, (*byte)(unsafe.Pointer(&tag)), 4, &n, nil) != nil || tag == 0 {
			windows.CloseHandle(h)
			continue
		}
		q := [3]uint32{tag, 0, 0} // BatteryInformation
		var info [36]byte
		err = windows.DeviceIoControl(h, 0x294044, (*byte)(unsafe.Pointer(&q[0])), 12, &info[0], 36, &n, nil)
		windows.CloseHandle(h)
		if err != nil {
			continue
		}
		design := binary.LittleEndian.Uint32(info[12:])
		full := binary.LittleEndian.Uint32(info[16:])
		if design < 5000 || design > 300000 || full == 0 || full > 400000 {
			continue
		}
		return &Battery{
			DesignWh: float64(design) / 1000, FullWh: float64(full) / 1000,
			Health: int(float64(full) * 100 / float64(design)), Cycles: int(binary.LittleEndian.Uint32(info[32:])),
		}
	}
	return nil
}
