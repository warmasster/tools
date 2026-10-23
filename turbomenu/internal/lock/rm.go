//go:build windows

package lock

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// rmList pregunta al Restart Manager qué procesos usan esos archivos. Da el
// nombre amigable y el tipo de cada programa.
func rmList(files []string) []rmProcessInfo {
	if len(files) == 0 || procRmStartSession.Find() != nil {
		return nil
	}
	var session uint32
	key := make([]uint16, 33) // CCH_RM_SESSION_KEY + 1
	if r, _, _ := syscall.SyscallN(procRmStartSession.Addr(), uintptr(unsafe.Pointer(&session)), 0,
		uintptr(unsafe.Pointer(&key[0]))); r != 0 {
		return nil
	}
	defer syscall.SyscallN(procRmEndSession.Addr(), uintptr(session))

	ptrs := make([]*uint16, 0, len(files))
	for _, f := range files {
		if p, err := windows.UTF16PtrFromString(f); err == nil {
			ptrs = append(ptrs, p)
		}
	}
	if len(ptrs) == 0 {
		return nil
	}
	if r, _, _ := syscall.SyscallN(procRmRegisterResources.Addr(), uintptr(session),
		uintptr(len(ptrs)), uintptr(unsafe.Pointer(&ptrs[0])), 0, 0, 0, 0); r != 0 {
		return nil
	}
	n := uint32(8)
	for i := 0; i < 4; i++ {
		infos := make([]rmProcessInfo, n)
		var needed, reasons uint32
		have := n
		r, _, _ := syscall.SyscallN(procRmGetList.Addr(), uintptr(session),
			uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&have)),
			uintptr(unsafe.Pointer(&infos[0])), uintptr(unsafe.Pointer(&reasons)))
		switch r {
		case 0:
			return infos[:have]
		case uintptr(windows.ERROR_MORE_DATA):
			n = needed + 4
		default:
			return nil
		}
	}
	return nil
}
