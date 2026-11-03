package sensors

// Windows no da la temperatura del procesador sin un driver. Si HWiNFO está
// abierto con «Shared Memory Support», TurboMON la lee de su memoria compartida
// (solo lectura, no toca nada de HWiNFO).

import (
	"encoding/binary"
	"math"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var pOpenFileMapping = modKernel.NewProc("OpenFileMappingW")

type hwinfoReading struct {
	cpuTemp  float64
	cpuPower float64
}

func readHWiNFO() (hwinfoReading, bool) {
	var res hwinfoReading
	name, _ := windows.UTF16PtrFromString(`Global\HWiNFO_SENS_SM2`)
	h, _, _ := pOpenFileMapping.Call(windows.FILE_MAP_READ, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return res, false
	}
	defer windows.CloseHandle(windows.Handle(h))
	addr, err := windows.MapViewOfFile(windows.Handle(h), windows.FILE_MAP_READ, 0, 0, 0)
	if err != nil {
		return res, false
	}
	defer windows.UnmapViewOfFile(addr)
	var mbi windows.MemoryBasicInformation
	if windows.VirtualQuery(addr, &mbi, unsafe.Sizeof(mbi)) != nil {
		return res, false
	}
	view := *(*unsafe.Pointer)(unsafe.Pointer(&addr))
	mem := unsafe.Slice((*byte)(view), mbi.RegionSize)
	return parseHWiNFO(mem)
}

// parseHWiNFO lee la cabecera y busca las lecturas del procesador.
func parseHWiNFO(mem []byte) (hwinfoReading, bool) {
	var res hwinfoReading
	if len(mem) < 44 || binary.LittleEndian.Uint32(mem[0:]) != 0x53695748 { // "HWiS"
		return res, false
	}
	// Firma(4) Versión(4) Revisión(4) Sondeo(8) OffSens(4) TamSens(4) NumSens(4) OffLect(4) TamLect(4) NumLect(4)
	offR := int(binary.LittleEndian.Uint32(mem[32:]))
	sizeR := int(binary.LittleEndian.Uint32(mem[36:]))
	numR := int(binary.LittleEndian.Uint32(mem[40:]))
	if sizeR < 316 || numR > 20000 {
		return res, false
	}
	best, bestScore := math.NaN(), 0
	for i := 0; i < numR; i++ {
		o := offR + i*sizeR
		if o+316 > len(mem) {
			break
		}
		typ := binary.LittleEndian.Uint32(mem[o:])
		label := cstr(mem[o+12 : o+140])
		val := math.Float64frombits(binary.LittleEndian.Uint64(mem[o+284:]))
		l := strings.ToLower(label)
		switch typ {
		case 1: // temperatura
			score := 0
			switch {
			case strings.HasPrefix(l, "cpu package"), strings.HasPrefix(l, "cpu (tctl/tdie)"):
				score = 3
			case strings.HasPrefix(l, "cpu die"), strings.HasPrefix(l, "core max"):
				score = 2
			case strings.HasPrefix(l, "cpu"):
				score = 1
			}
			if score > bestScore && val > 0 && val < 150 {
				best, bestScore = val, score
			}
		case 5: // potencia
			if res.cpuPower == 0 && strings.HasPrefix(l, "cpu package power") && val > 0 && val < 1000 {
				res.cpuPower = val
			}
		}
	}
	if bestScore == 0 {
		return res, false
	}
	res.cpuTemp = best
	return res, true
}

func cstr(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
