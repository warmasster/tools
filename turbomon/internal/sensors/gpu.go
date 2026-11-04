package sensors

// Tarjetas gráficas: cuáles hay (DXGI), su temperatura y ventilador (D3DKMT, lo
// mismo que el Administrador de tareas, vale para cualquier marca). No se usa la
// librería de NVIDIA (NVML): solo cargarla ocupa 20 MB.

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modDxgi        = windows.NewLazySystemDLL("dxgi.dll")
	modGdi         = windows.NewLazySystemDLL("gdi32.dll")
	pCreateFactory = modDxgi.NewProc("CreateDXGIFactory1")
	pKmtEnum2      = modGdi.NewProc("D3DKMTEnumAdapters2")
	pKmtQuery      = modGdi.NewProc("D3DKMTQueryAdapterInfo")
	iidFactory1    = windows.GUID{Data1: 0x770aae78, Data2: 0xf26f, Data3: 0x4dba, Data4: [8]byte{0xa8, 0x29, 0x25, 0x3c, 0x83, 0xd1, 0xb3, 0x87}}
)

// Adapter es una tarjeta gráfica.
type Adapter struct {
	Name      string `json:"name"`
	Vendor    string `json:"vendor"` // nvidia, amd, intel u otro
	VendorID  uint32 `json:"-"`
	DeviceID  uint32 `json:"-"`
	SubSysID  uint32 `json:"-"`
	VRAM      uint64 `json:"vram"` // memoria dedicada, en bytes
	Dedicated bool   `json:"dedicated"`
	Outputs   int    `json:"outputs"` // pantallas conectadas a esta tarjeta
	LuidLow   uint32 `json:"-"`
	LuidHigh  int32  `json:"-"`
}

// LUID en el formato de los contadores de Windows: «0x00000000_0x0000d1f3».
func (a Adapter) luidKey() string {
	return strings.ToLower(fmt.Sprintf("0x%08x_0x%08x", uint32(a.LuidHigh), a.LuidLow))
}

// SameLUID dice si dos tarjetas son la misma.
func SameLUID(low uint32, high int32, a Adapter) bool { return a.LuidLow == low && a.LuidHigh == high }

type comObj struct{ vtbl *[64]uintptr }

func (o *comObj) call(i int, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(o.vtbl[i], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return r
}
func (o *comObj) release() { o.call(2) }

type adapterDesc1 struct {
	Description                                                     [128]uint16
	VendorID, DeviceID, SubSysID, Revision                          uint32
	DedicatedVideoMemory, DedicatedSystemMemory, SharedSystemMemory uintptr
	LuidLow                                                         uint32
	LuidHigh                                                        int32
	Flags                                                           uint32
	_                                                               uint32
}

func vendorName(id uint32) string {
	switch id {
	case 0x10DE:
		return "nvidia"
	case 0x1002, 0x1022:
		return "amd"
	case 0x8086:
		return "intel"
	}
	return "otro"
}

// Adapters enumera las tarjetas gráficas de verdad (sin la de software de Microsoft).
func Adapters() []Adapter {
	if modDxgi.Load() != nil {
		return nil
	}
	var f *comObj
	if r, _, _ := pCreateFactory.Call(uintptr(unsafe.Pointer(&iidFactory1)), uintptr(unsafe.Pointer(&f))); r != 0 || f == nil {
		return nil
	}
	defer f.release()
	var out []Adapter
	for i := uintptr(0); i < 16; i++ {
		var ad *comObj
		if r := f.call(12, i, uintptr(unsafe.Pointer(&ad))); r != 0 || ad == nil { // EnumAdapters1
			break
		}
		var d adapterDesc1
		ad.call(10, uintptr(unsafe.Pointer(&d))) // GetDesc1
		outputs := 0
		for j := uintptr(0); j < 16; j++ {
			var o *comObj
			if r := ad.call(7, j, uintptr(unsafe.Pointer(&o))); r != 0 || o == nil { // EnumOutputs
				break
			}
			o.release()
			outputs++
		}
		ad.release()
		if d.Flags&2 != 0 || d.VendorID == 0x1414 { // DXGI_ADAPTER_FLAG_SOFTWARE, Microsoft Basic Render
			continue
		}
		name := strings.TrimSpace(windows.UTF16ToString(d.Description[:]))
		vram := uint64(d.DedicatedVideoMemory)
		out = append(out, Adapter{
			Name: name, Vendor: vendorName(d.VendorID), VendorID: d.VendorID, DeviceID: d.DeviceID, SubSysID: d.SubSysID, VRAM: vram,
			// Las integradas de Intel y AMD dicen tener 128-512 MB «dedicados».
			Dedicated: vram >= 1<<30,
			Outputs:   outputs, LuidLow: d.LuidLow, LuidHigh: d.LuidHigh,
		})
	}
	return out
}

// ---------- D3DKMT: temperatura y ventilador de cualquier gráfica ----------

type kmtAdapterInfo struct {
	Handle        uint32
	LuidLow       uint32
	LuidHigh      int32
	NumSources    uint32
	PreciseRegion int32
}

type kmtEnum2 struct {
	Num      uint32
	_        uint32
	Adapters *kmtAdapterInfo
}

type kmtQuery struct {
	Handle uint32
	Type   uint32
	Data   unsafe.Pointer
	Size   uint32
	_      uint32
}

type kmtPerfData struct {
	PhysicalAdapterIndex uint32
	_                    uint32
	MemoryFrequency      uint64
	MaxMemoryFrequency   uint64
	MaxMemoryFrequencyOC uint64
	MemoryBandwidth      uint64
	PCIEBandwidth        uint64
	FanRPM               uint32
	Power                uint32 // décimas de % del límite
	Temperature          uint32 // décimas de grado
	PowerStateOverride   uint8
	_                    [3]byte
}

// kmtHandles abre todas las tarjetas (los identificadores valen mientras viva el programa).
func kmtHandles() map[[2]uint32]uint32 {
	if pKmtEnum2.Find() != nil {
		return nil
	}
	var e kmtEnum2
	if r, _, _ := pKmtEnum2.Call(uintptr(unsafe.Pointer(&e))); r != 0 || e.Num == 0 {
		return nil
	}
	list := make([]kmtAdapterInfo, e.Num+4)
	e.Num = uint32(len(list))
	e.Adapters = &list[0]
	if r, _, _ := pKmtEnum2.Call(uintptr(unsafe.Pointer(&e))); r != 0 {
		return nil
	}
	m := map[[2]uint32]uint32{}
	for _, a := range list[:e.Num] {
		m[[2]uint32{a.LuidLow, uint32(a.LuidHigh)}] = a.Handle
	}
	return m
}

func kmtPerf(h uint32) (kmtPerfData, bool) {
	var p kmtPerfData
	q := kmtQuery{Handle: h, Type: 62, Data: unsafe.Pointer(&p), Size: uint32(unsafe.Sizeof(p))} // KMTQAITYPE_ADAPTERPERFDATA
	r, _, _ := pKmtQuery.Call(uintptr(unsafe.Pointer(&q)))
	return p, r == 0
}

type kmtNodePerf struct {
	NodeOrdinal          uint32
	PhysicalAdapterIndex uint32
	Frequency            uint64
	MaxFrequency         uint64
	MaxFrequencyOC       uint64
	Voltage              uint32
	VoltageMax           uint32
	VoltageMaxOC         uint32
	_                    uint32
	MaxTransitionLatency uint64
}

// kmtClock devuelve la frecuencia actual del motor 3D (nodo 0), en MHz.
func kmtClock(h uint32) int {
	var p kmtNodePerf
	q := kmtQuery{Handle: h, Type: 61, Data: unsafe.Pointer(&p), Size: uint32(unsafe.Sizeof(p))} // KMTQAITYPE_NODEPERFDATA
	if r, _, _ := pKmtQuery.Call(uintptr(unsafe.Pointer(&q))); r != 0 {
		return 0
	}
	return int(p.Frequency / 1e6)
}
