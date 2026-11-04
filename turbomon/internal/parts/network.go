package parts

import (
	"encoding/binary"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modWlan        = windows.NewLazySystemDLL("wlanapi.dll")
	pWlanOpen      = modWlan.NewProc("WlanOpenHandle")
	pWlanClose     = modWlan.NewProc("WlanCloseHandle")
	pWlanEnum      = modWlan.NewProc("WlanEnumInterfaces")
	pWlanQuery     = modWlan.NewProc("WlanQueryInterface")
	pWlanSet       = modWlan.NewProc("WlanSetInterface")
	pWlanBssList   = modWlan.NewProc("WlanGetNetworkBssList")
	pWlanFree      = modWlan.NewProc("WlanFreeMemory")
	modIphlp       = windows.NewLazySystemDLL("iphlpapi.dll")
	pGetBestIfaceE = modIphlp.NewProc("GetBestInterfaceEx")
)

// NetAdapter es una conexión de red.
type NetAdapter struct {
	Name       string `json:"name"`
	Model      string `json:"model"`
	Kind       string `json:"kind"` // cable, wifi
	Up         bool   `json:"up"`
	Default    bool   `json:"default"`    // por la que sale internet
	LinkMbps   int    `json:"linkMbps"`   // velocidad del enlace
	WifiStd    string `json:"wifiStd"`    // Wi-Fi 4/5/6/6E/7
	WifiBand   string `json:"wifiBand"`   // 2,4 / 5 / 6 GHz
	WifiSignal int    `json:"wifiSignal"` // %
	WifiRxMbps int    `json:"wifiRxMbps"`
	SSID       string `json:"-"` // no se enseña (privacidad)
	ifIndex    uint32
	guid       windows.GUID
}

func readNetwork() []NetAdapter {
	var size uint32 = 16 << 10
	var buf []byte
	for i := 0; i < 3; i++ {
		buf = make([]byte, size)
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, windows.GAA_FLAG_INCLUDE_GATEWAYS|windows.GAA_FLAG_SKIP_DNS_SERVER|windows.GAA_FLAG_SKIP_MULTICAST|windows.GAA_FLAG_SKIP_ANYCAST,
			0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			break
		}
		if err != windows.ERROR_BUFFER_OVERFLOW {
			return nil
		}
	}
	def := defaultIfIndex()
	var out []NetAdapter
	for a := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); a != nil; a = a.Next {
		kind := ""
		switch a.IfType {
		case windows.IF_TYPE_ETHERNET_CSMACD:
			kind = "cable"
		case windows.IF_TYPE_IEEE80211:
			kind = "wifi"
		default:
			continue
		}
		desc := windows.UTF16PtrToString(a.Description)
		ld := strings.ToLower(desc)
		if strings.Contains(ld, "virtual") || strings.Contains(ld, "hyper-v") || strings.Contains(ld, "vpn") || strings.Contains(ld, "tap-") ||
			strings.Contains(ld, "wan miniport") || strings.Contains(ld, "bluetooth") || strings.Contains(ld, "loopback") || strings.Contains(ld, "wireguard") {
			continue
		}
		n := NetAdapter{
			Name: windows.UTF16PtrToString(a.FriendlyName), Model: desc, Kind: kind, Up: a.OperStatus == windows.IfOperStatusUp,
			ifIndex: a.IfIndex,
		}
		if n.Up && a.ReceiveLinkSpeed > 0 && a.ReceiveLinkSpeed != ^uint64(0) {
			n.LinkMbps = int(a.ReceiveLinkSpeed / 1_000_000)
		}
		n.Default = n.Up && a.IfIndex == def
		if g, err := windows.GUIDFromString(windows.BytePtrToString(a.AdapterName)); err == nil {
			n.guid = g
		}
		out = append(out, n)
	}
	wifiDetails(out)
	return out
}

// defaultIfIndex es la interfaz por la que se sale a internet.
func defaultIfIndex() uint32 {
	var sa windows.RawSockaddrInet4
	sa.Family = windows.AF_INET
	sa.Addr = [4]byte{1, 1, 1, 1}
	var idx uint32
	if r, _, _ := pGetBestIfaceE.Call(uintptr(unsafe.Pointer(&sa)), uintptr(unsafe.Pointer(&idx))); r != 0 {
		return 0
	}
	return idx
}

func openWlan() (uintptr, bool) {
	if modWlan.Load() != nil {
		return 0, false
	}
	var ver uint32
	var h uintptr
	if r, _, _ := pWlanOpen.Call(2, 0, uintptr(unsafe.Pointer(&ver)), uintptr(unsafe.Pointer(&h))); r != 0 {
		return 0, false
	}
	return h, true
}

// wifiDetails añade estándar, banda y señal a los adaptadores wifi conectados.
func wifiDetails(ads []NetAdapter) {
	h, ok := openWlan()
	if !ok {
		return
	}
	defer pWlanClose.Call(h, 0)
	for i := range ads {
		a := &ads[i]
		if a.Kind != "wifi" || !a.Up {
			continue
		}
		var size uint32
		var data unsafe.Pointer
		if r, _, _ := pWlanQuery.Call(h, uintptr(unsafe.Pointer(&a.guid)), 7, 0, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&data)), 0); r != 0 || data == nil { // current_connection
			continue
		}
		b := unsafe.Slice((*byte)(data), size)
		var bssid [6]byte
		if len(b) >= 604 {
			as := b[520:]
			copy(bssid[:], as[40:46])
			ssidLen := binary.LittleEndian.Uint32(as[0:])
			if ssidLen <= 32 {
				a.SSID = string(as[4 : 4+ssidLen])
			}
			switch binary.LittleEndian.Uint32(as[48:]) {
			case 7:
				a.WifiStd = "Wi-Fi 4"
			case 8:
				a.WifiStd = "Wi-Fi 5"
			case 10:
				a.WifiStd = "Wi-Fi 6"
			case 11:
				a.WifiStd = "Wi-Fi 7"
			case 4, 5, 6:
				a.WifiStd = "Wi-Fi antiguo"
			}
			a.WifiSignal = int(binary.LittleEndian.Uint32(as[56:]))
			a.WifiRxMbps = int(binary.LittleEndian.Uint32(as[60:]) / 1000)
			if a.LinkMbps == 0 {
				a.LinkMbps = a.WifiRxMbps
			}
		}
		pWlanFree.Call(uintptr(data))
		// Banda: por la frecuencia del punto de acceso al que está conectado.
		var list unsafe.Pointer
		if r, _, _ := pWlanBssList.Call(h, uintptr(unsafe.Pointer(&a.guid)), 0, 1, 0, 0, uintptr(unsafe.Pointer(&list))); r == 0 && list != nil { // dot11_BSS_type_infrastructure
			lb := unsafe.Slice((*byte)(list), 8)
			n := binary.LittleEndian.Uint32(lb[4:])
			entries := unsafe.Slice((*byte)(unsafe.Add(list, 8)), int(n)*360)
			best := uint32(0)
			for k := 0; k < int(n); k++ {
				e := entries[360*k:]
				if [6]byte(e[40:46]) == bssid { // el punto de acceso al que está conectado
					best = binary.LittleEndian.Uint32(e[92:])
					break
				}
			}
			switch {
			case best >= 5925000:
				a.WifiBand = "6 GHz"
				if a.WifiStd == "Wi-Fi 6" {
					a.WifiStd = "Wi-Fi 6E"
				}
			case best >= 4900000:
				a.WifiBand = "5 GHz"
			case best > 0:
				a.WifiBand = "2,4 GHz"
			}
			pWlanFree.Call(uintptr(list))
		}
	}
}

// WifiBackgroundScan activa o desactiva la búsqueda de redes de fondo (y el
// modo «streaming») de todas las conexiones wifi. Devuelve las que ha cambiado.
func WifiBackgroundScan(enabled bool) int {
	h, ok := openWlan()
	if !ok {
		return 0
	}
	defer pWlanClose.Call(h, 0)
	var list unsafe.Pointer
	if r, _, _ := pWlanEnum.Call(h, 0, uintptr(unsafe.Pointer(&list))); r != 0 || list == nil {
		return 0
	}
	defer pWlanFree.Call(uintptr(list))
	hdr := unsafe.Slice((*byte)(list), 8)
	n := binary.LittleEndian.Uint32(hdr[0:])
	changed := 0
	for i := 0; i < int(n) && i < 8; i++ {
		info := unsafe.Slice((*byte)(unsafe.Add(list, 8+532*i)), 532)
		var g windows.GUID
		copy((*[16]byte)(unsafe.Pointer(&g))[:], info[:16])
		if binary.LittleEndian.Uint32(info[528:]) != 1 { // wlan_interface_state_connected
			continue
		}
		on := uint32(0)
		if enabled {
			on = 1
		}
		stream := 1 - on
		r1, _, _ := pWlanSet.Call(h, uintptr(unsafe.Pointer(&g)), 2, 4, uintptr(unsafe.Pointer(&on)), 0) // background_scan_enabled
		pWlanSet.Call(h, uintptr(unsafe.Pointer(&g)), 3, 4, uintptr(unsafe.Pointer(&stream)), 0)         // media_streaming_mode
		if r1 == 0 {
			changed++
		}
	}
	return changed
}
