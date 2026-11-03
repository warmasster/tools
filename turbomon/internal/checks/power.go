package checks

// Planes de energía de Windows.

import (
	"errors"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modPowrprof     = windows.NewLazySystemDLL("powrprof.dll")
	pGetActive      = modPowrprof.NewProc("PowerGetActiveScheme")
	pSetActive      = modPowrprof.NewProc("PowerSetActiveScheme")
	pReadAC         = modPowrprof.NewProc("PowerReadACValueIndex")
	pReadDC         = modPowrprof.NewProc("PowerReadDCValueIndex")
	pWriteAC        = modPowrprof.NewProc("PowerWriteACValueIndex")
	pReadName       = modPowrprof.NewProc("PowerReadFriendlyName")
	pEnumerate      = modPowrprof.NewProc("PowerEnumerate")
	pGetOverlay     = modPowrprof.NewProc("PowerGetEffectiveOverlayScheme")
	modKernelLocal  = windows.NewLazySystemDLL("kernel32.dll")
	pLocalFree      = modKernelLocal.NewProc("LocalFree")
)

// Planes conocidos.
var (
	PlanSaver    = mustGUID("a1841308-3541-4fab-bc81-f71556f20b4a")
	PlanBalanced = mustGUID("381b4222-f694-41f0-9685-ff5bb260df2e")
	PlanHigh     = mustGUID("8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c")
	PlanUltimate = mustGUID("e9a42b02-d5df-448d-aa00-03f14749eb61")

	subProcessor = mustGUID("54533251-82be-4824-96c1-47b60b740d00")
	procMax      = mustGUID("bc5038f7-23e0-4960-96da-33abaf5935ec")

	overlayBestEfficiency = mustGUID("961cc777-2547-4f9d-8174-7d86181b8a7a")
	overlayBestPerf       = mustGUID("ded574b5-45a0-4f42-8737-46345c09c238")
)

func mustGUID(s string) windows.GUID {
	g, err := windows.GUIDFromString("{" + s + "}")
	if err != nil {
		panic(err)
	}
	return g
}

// Plan es un plan de energía.
type Plan struct {
	GUID windows.GUID
	Name string
}

// ActivePlan devuelve el plan activo.
func ActivePlan() (Plan, error) {
	var p *windows.GUID
	if r, _, _ := pGetActive.Call(0, uintptr(unsafe.Pointer(&p))); r != 0 || p == nil {
		return Plan{}, syscall.Errno(r)
	}
	g := *p
	pLocalFree.Call(uintptr(unsafe.Pointer(p)))
	return Plan{GUID: g, Name: planName(g)}, nil
}

func planName(g windows.GUID) string {
	var size uint32
	pReadName.Call(0, uintptr(unsafe.Pointer(&g)), 0, 0, 0, uintptr(unsafe.Pointer(&size)))
	if size == 0 {
		return ""
	}
	buf := make([]uint16, size/2+1)
	if r, _, _ := pReadName.Call(0, uintptr(unsafe.Pointer(&g)), 0, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))); r != 0 {
		return ""
	}
	return strings.TrimSpace(windows.UTF16ToString(buf))
}

// Plans devuelve los planes que hay en el equipo.
func Plans() []Plan {
	var out []Plan
	for i := uint32(0); i < 64; i++ {
		var g windows.GUID
		size := uint32(unsafe.Sizeof(g))
		if r, _, _ := pEnumerate.Call(0, 0, 0, 16, uintptr(i), uintptr(unsafe.Pointer(&g)), uintptr(unsafe.Pointer(&size))); r != 0 { // ACCESS_SCHEME
			break
		}
		out = append(out, Plan{GUID: g, Name: planName(g)})
	}
	return out
}

// SetPlan activa un plan.
func SetPlan(g windows.GUID) error {
	if r, _, _ := pSetActive.Call(0, uintptr(unsafe.Pointer(&g))); r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

// BestPlan devuelve el plan de más rendimiento que hay instalado (o false si solo está el equilibrado).
func BestPlan() (Plan, bool) {
	var high *Plan
	for _, p := range Plans() {
		p := p
		if p.GUID == PlanUltimate || strings.Contains(strings.ToLower(p.Name), "máximo rendimiento") || strings.Contains(strings.ToLower(p.Name), "ultimate") {
			return p, true
		}
		if p.GUID == PlanHigh {
			high = &p
		}
	}
	if high != nil {
		return *high, true
	}
	return Plan{}, false
}

// OverlayMode es el «modo de energía» de Windows 10/11 (la barra de
// Configuración > Energía): "efficiency", "balanced" o "performance".
func OverlayMode() string {
	if pGetOverlay.Find() != nil {
		return ""
	}
	var g windows.GUID
	if r, _, _ := pGetOverlay.Call(uintptr(unsafe.Pointer(&g))); r != 0 {
		return ""
	}
	switch g {
	case overlayBestEfficiency:
		return "efficiency"
	case overlayBestPerf:
		return "performance"
	}
	return "balanced"
}

// ProcessorMax devuelve el «estado máximo del procesador» del plan activo (en %), enchufado y con batería.
func ProcessorMax() (ac, dc int, err error) {
	p, err := ActivePlan()
	if err != nil {
		return 0, 0, err
	}
	var v uint32
	if r, _, _ := pReadAC.Call(0, uintptr(unsafe.Pointer(&p.GUID)), uintptr(unsafe.Pointer(&subProcessor)), uintptr(unsafe.Pointer(&procMax)), uintptr(unsafe.Pointer(&v))); r != 0 {
		return 0, 0, syscall.Errno(r)
	}
	ac = int(v)
	if r, _, _ := pReadDC.Call(0, uintptr(unsafe.Pointer(&p.GUID)), uintptr(unsafe.Pointer(&subProcessor)), uintptr(unsafe.Pointer(&procMax)), uintptr(unsafe.Pointer(&v))); r == 0 {
		dc = int(v)
	}
	return ac, dc, nil
}

// SetProcessorMax100 quita el límite del procesador en el plan activo.
func SetProcessorMax100() error {
	p, err := ActivePlan()
	if err != nil {
		return err
	}
	if r, _, _ := pWriteAC.Call(0, uintptr(unsafe.Pointer(&p.GUID)), uintptr(unsafe.Pointer(&subProcessor)), uintptr(unsafe.Pointer(&procMax)), 100); r != 0 {
		if r == uintptr(windows.ERROR_ACCESS_DENIED) {
			return errors.New("Windows no deja cambiar el plan sin permiso de administrador")
		}
		return syscall.Errno(r)
	}
	// Con batería se deja como esté: ahí limitarlo puede ser a propósito.
	return SetPlan(p.GUID) // aplica el cambio
}
