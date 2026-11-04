package sensors

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	modKernel      = windows.NewLazySystemDLL("kernel32.dll")
	modNtdll       = windows.NewLazySystemDLL("ntdll.dll")
	pSystemTimes   = modKernel.NewProc("GetSystemTimes")
	pMemStatus     = modKernel.NewProc("GlobalMemoryStatusEx")
	pLPIEx         = modKernel.NewProc("GetLogicalProcessorInformationEx")
	pPowerStatus   = modKernel.NewProc("GetSystemPowerStatus")
	pTickCount64   = modKernel.NewProc("GetTickCount64")
	pNtQuerySysInf = modNtdll.NewProc("NtQuerySystemInformation")
)

// CPUInfo es lo que no cambia del procesador.
type CPUInfo struct {
	Name    string `json:"name"`
	Cores   int    `json:"cores"`
	Threads int    `json:"threads"`
	BaseMHz int    `json:"baseMHz"`
}

func readCPUInfo() CPUInfo {
	var ci CPUInfo
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE); err == nil {
		s, _, _ := k.GetStringValue("ProcessorNameString")
		ci.Name = strings.Join(strings.Fields(s), " ")
		mhz, _, _ := k.GetIntegerValue("~MHz")
		ci.BaseMHz = int(mhz)
		k.Close()
	}
	ci.Cores, ci.Threads = coreCount()
	return ci
}

// coreCount cuenta núcleos físicos e hilos.
func coreCount() (cores, threads int) {
	var size uint32
	pLPIEx.Call(0, 0, uintptr(unsafe.Pointer(&size))) // 0 = RelationProcessorCore
	if size == 0 {
		return 0, 0
	}
	buf := make([]byte, size)
	r, _, _ := pLPIEx.Call(0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return 0, 0
	}
	for off := uint32(0); off+8 <= size; {
		recSize := *(*uint32)(unsafe.Pointer(&buf[off+4]))
		if recSize == 0 {
			break
		}
		cores++
		// PROCESSOR_RELATIONSHIP: Flags(1) EfficiencyClass(1) Reserved(20) GroupCount(2) GroupMask[]
		// GroupMask: Mask(8) Group(2) Reserved(6)
		if off+8+24+8 <= size {
			mask := *(*uint64)(unsafe.Pointer(&buf[off+8+24]))
			for ; mask != 0; mask &= mask - 1 {
				threads++
			}
		}
		off += recSize
	}
	return cores, threads
}

func filetime(f windows.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }

type cpuTimes struct{ idle, total uint64 }

func systemTimes() (cpuTimes, bool) {
	var i, k, u windows.Filetime
	r, _, _ := pSystemTimes.Call(uintptr(unsafe.Pointer(&i)), uintptr(unsafe.Pointer(&k)), uintptr(unsafe.Pointer(&u)))
	if r == 0 {
		return cpuTimes{}, false
	}
	// El tiempo de núcleo ya incluye el de reposo.
	return cpuTimes{idle: filetime(i), total: filetime(k) + filetime(u)}, true
}

// SYSTEM_PROCESSOR_PERFORMANCE_INFORMATION
type sppi struct {
	Idle, Kernel, User, Dpc, Interrupt int64
	InterruptCount                     uint32
	_                                  uint32
}

func coreTimes(buf []sppi) []sppi {
	if len(buf) == 0 {
		return nil
	}
	var ret uint32
	r, _, _ := pNtQuerySysInf.Call(8, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))*unsafe.Sizeof(buf[0]), uintptr(unsafe.Pointer(&ret)))
	if r != 0 {
		return nil
	}
	return buf[:ret/uint32(unsafe.Sizeof(buf[0]))]
}

func pct(busy, total uint64) float64 {
	if total == 0 || busy > total {
		if total == 0 {
			return 0
		}
		return 100
	}
	return float64(busy) * 100 / float64(total)
}

// ---------- memoria ----------

type memStatus struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys, AvailPhys uint64
	TotalPage, AvailPage uint64
	TotalVirt, AvailVirt uint64
	AvailExtendedVirtual uint64
}

func memoryStatus() (memStatus, bool) {
	var m memStatus
	m.Length = uint32(unsafe.Sizeof(m))
	r, _, _ := pMemStatus.Call(uintptr(unsafe.Pointer(&m)))
	return m, r != 0
}

// TotalRAM devuelve la memoria física total en bytes.
func TotalRAM() uint64 {
	m, _ := memoryStatus()
	return m.TotalPhys
}

// AvailableRAM devuelve la memoria física disponible en bytes.
func AvailableRAM() uint64 {
	m, _ := memoryStatus()
	return m.AvailPhys
}

// PageFileBytes devuelve el tamaño de la memoria virtual que hay además de la RAM.
func PageFileBytes() uint64 {
	m, _ := memoryStatus()
	if m.TotalPage > m.TotalPhys {
		return m.TotalPage - m.TotalPhys
	}
	return 0
}

// ---------- batería ----------

type powerStatus struct {
	ACLine, BatteryFlag, BatteryPercent, SystemStatus uint8
	BatteryLifeTime, BatteryFullLifeTime              uint32
}

// Battery dice si el equipo tiene batería, si está enchufado y el porcentaje.
func Battery() (has, plugged bool, percent int) {
	var p powerStatus
	if r, _, _ := pPowerStatus.Call(uintptr(unsafe.Pointer(&p))); r == 0 {
		return false, true, 0
	}
	has = p.BatteryFlag != 128 && p.BatteryFlag != 255
	plugged = p.ACLine != 0
	percent = int(p.BatteryPercent)
	if percent > 100 {
		percent = 0
	}
	return
}

func uptimeSeconds() int64 {
	r, _, _ := pTickCount64.Call()
	return int64(r / 1000)
}
