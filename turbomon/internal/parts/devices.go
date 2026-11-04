package parts

// Ayudas para SetupAPI y cfgmgr32: propiedades de dispositivos y sus recursos.

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modSetupapi  = windows.NewLazySystemDLL("setupapi.dll")
	pGetDevProp  = modSetupapi.NewProc("SetupDiGetDevicePropertyW")
	modCfgmgr    = windows.NewLazySystemDLL("cfgmgr32.dll")
	pFirstLogCf  = modCfgmgr.NewProc("CM_Get_First_Log_Conf")
	pNextResDes  = modCfgmgr.NewProc("CM_Get_Next_Res_Des")
	pResDesSize  = modCfgmgr.NewProc("CM_Get_Res_Des_Data_Size")
	pResDesData  = modCfgmgr.NewProc("CM_Get_Res_Des_Data")
	pFreeResDes  = modCfgmgr.NewProc("CM_Free_Res_Des_Handle")
	pFreeLogConf = modCfgmgr.NewProc("CM_Free_Log_Conf_Handle")
)

func guid(s string) windows.GUID {
	g, err := windows.GUIDFromString("{" + s + "}")
	if err != nil {
		panic(err)
	}
	return g
}

func devKey(fmtid string, pid uint32) windows.DEVPROPKEY {
	return windows.DEVPROPKEY{FmtID: windows.DEVPROPGUID(guid(fmtid)), PID: windows.DEVPROPID(pid)}
}

var (
	keyLinkSpeed    = devKey("3ab22e31-8264-4b4e-9af5-a8d2d8e33e62", 9)
	keyLinkWidth    = devKey("3ab22e31-8264-4b4e-9af5-a8d2d8e33e62", 10)
	keyMaxSpeed     = devKey("3ab22e31-8264-4b4e-9af5-a8d2d8e33e62", 11)
	keyMaxWidth     = devKey("3ab22e31-8264-4b4e-9af5-a8d2d8e33e62", 12)
	keyDrvVersion   = devKey("a8b865dd-2e3d-4094-ad97-e593a70c75d6", 3)
	keyDrvDate      = devKey("a8b865dd-2e3d-4094-ad97-e593a70c75d6", 2)
	keyFriendlyName = devKey("a45c254e-df1c-4efd-8020-67d146a850e0", 14)
	keyDeviceDesc   = devKey("a45c254e-df1c-4efd-8020-67d146a850e0", 2)
	keyManufacturer = devKey("a45c254e-df1c-4efd-8020-67d146a850e0", 13)

	classDisplay = guid("4d36e968-e325-11ce-bfc1-08002be10318")
	ifaceHID     = guid("4d1e55b2-f16f-11cf-88cb-001111000030")
	ifaceBattery = guid("72631e54-78a4-11d0-bcf7-00aa00b7b32a")
)

const (
	devpropTypeUint32   = 0x7
	devpropTypeString   = 0x12
	devpropTypeFiletime = 0x10
)

// devProp lee una propiedad de un dispositivo (tipo y bytes).
func devProp(set windows.DevInfo, d *windows.DevInfoData, key windows.DEVPROPKEY) (uint32, []byte) {
	var typ, need uint32
	buf := make([]byte, 512)
	for try := 0; try < 2; try++ {
		r, _, e := pGetDevProp.Call(uintptr(set), uintptr(unsafe.Pointer(d)), uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&typ)),
			uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&need)), 0)
		if r != 0 {
			return typ, buf[:need]
		}
		if e == windows.ERROR_INSUFFICIENT_BUFFER && need > uint32(len(buf)) {
			buf = make([]byte, need)
			continue
		}
		break
	}
	return 0, nil
}

func devPropU32(set windows.DevInfo, d *windows.DevInfoData, key windows.DEVPROPKEY) (uint32, bool) {
	t, b := devProp(set, d, key)
	if t != devpropTypeUint32 || len(b) < 4 {
		return 0, false
	}
	return binary.LittleEndian.Uint32(b), true
}

func devPropStr(set windows.DevInfo, d *windows.DevInfoData, key windows.DEVPROPKEY) string {
	t, b := devProp(set, d, key)
	if t != devpropTypeString || len(b) < 2 {
		return ""
	}
	u := unsafe.Slice((*uint16)(unsafe.Pointer(&b[0])), len(b)/2)
	return strings.TrimSpace(windows.UTF16ToString(u))
}

func devPropTime(set windows.DevInfo, d *windows.DevInfoData, key windows.DEVPROPKEY) time.Time {
	t, b := devProp(set, d, key)
	if t != devpropTypeFiletime || len(b) < 8 {
		return time.Time{}
	}
	ft := windows.Filetime{LowDateTime: binary.LittleEndian.Uint32(b), HighDateTime: binary.LittleEndian.Uint32(b[4:])}
	return time.Unix(0, ft.Nanoseconds()).UTC()
}

// device es un dispositivo de una clase, con su lista de identificadores.
type device struct {
	set  windows.DevInfo
	data *windows.DevInfoData
	ids  []string
}

// eachDevice recorre los dispositivos presentes de una clase.
func eachDevice(class windows.GUID, f func(d device) bool) {
	set, err := windows.SetupDiGetClassDevsEx(&class, "", 0, windows.DIGCF_PRESENT, 0, "")
	if err != nil {
		return
	}
	defer set.Close()
	for i := 0; i < 64; i++ {
		data, err := set.EnumDeviceInfo(i)
		if err != nil {
			break
		}
		var ids []string
		if v, err := set.DeviceRegistryProperty(data, windows.SPDRP_HARDWAREID); err == nil {
			if l, ok := v.([]string); ok {
				ids = l
			}
		}
		if !f(device{set: set, data: data, ids: ids}) {
			return
		}
	}
}

// largestMemRange devuelve el mayor rango de memoria asignado a un dispositivo
// (en una gráfica, la «ventana» por la que el procesador ve la VRAM).
func largestMemRange(devInst windows.DEVINST) uint64 {
	var logConf uintptr
	if r, _, _ := pFirstLogCf.Call(uintptr(unsafe.Pointer(&logConf)), uintptr(devInst), 2); r != 0 { // ALLOC_LOG_CONF
		return 0
	}
	defer pFreeLogConf.Call(logConf)
	var best uint64
	cur := logConf
	for i := 0; i < 64; i++ {
		var next uintptr
		var typ uint32
		if r, _, _ := pNextResDes.Call(uintptr(unsafe.Pointer(&next)), cur, 0, uintptr(unsafe.Pointer(&typ)), 0); r != 0 { // ResType_All
			break
		}
		if cur != logConf {
			pFreeResDes.Call(cur)
		}
		cur = next
		if typ != 1 && typ != 7 { // ResType_Mem, ResType_MemLarge
			continue
		}
		var size uint32
		if r, _, _ := pResDesSize.Call(uintptr(unsafe.Pointer(&size)), cur, 0); r != 0 || size < 24 {
			continue
		}
		buf := make([]byte, size)
		if r, _, _ := pResDesData.Call(cur, uintptr(unsafe.Pointer(&buf[0])), uintptr(size), 0); r != 0 {
			continue
		}
		base := binary.LittleEndian.Uint64(buf[8:])
		end := binary.LittleEndian.Uint64(buf[16:])
		if end > base && end-base+1 > best {
			best = end - base + 1
		}
	}
	if cur != logConf {
		pFreeResDes.Call(cur)
	}
	return best
}

func hexPrefix(ven, dev uint32) string { return fmt.Sprintf(`PCI\VEN_%04X&DEV_%04X`, ven, dev) }
