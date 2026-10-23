//go:build windows

package lock

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Llamadas Win32/NT que no trae x/sys (sin cgo).
var (
	ntdll                        = windows.NewLazySystemDLL("ntdll.dll")
	procNtQueryInformationFile   = ntdll.NewProc("NtQueryInformationFile")
	procNtQuerySystemInformation = ntdll.NewProc("NtQuerySystemInformation")

	rstrtmgr                = windows.NewLazySystemDLL("rstrtmgr.dll")
	procRmStartSession      = rstrtmgr.NewProc("RmStartSession")
	procRmEndSession        = rstrtmgr.NewProc("RmEndSession")
	procRmRegisterResources = rstrtmgr.NewProc("RmRegisterResources")
	procRmGetList           = rstrtmgr.NewProc("RmGetList")

	user32             = windows.NewLazySystemDLL("user32.dll")
	procGetWindowTextW = user32.NewProc("GetWindowTextW")
	procGetWindow      = user32.NewProc("GetWindow")
	procPostMessageW   = user32.NewProc("PostMessageW")

	version                    = windows.NewLazySystemDLL("version.dll")
	procGetFileVersionInfoSize = version.NewProc("GetFileVersionInfoSizeW")
	procGetFileVersionInfo     = version.NewProc("GetFileVersionInfoW")
	procVerQueryValue          = version.NewProc("VerQueryValueW")

	cfgmgr32                         = windows.NewLazySystemDLL("cfgmgr32.dll")
	procCMLocateDevNode              = cfgmgr32.NewProc("CM_Locate_DevNodeW")
	procCMGetParent                  = cfgmgr32.NewProc("CM_Get_Parent")
	procCMRequestDeviceEject         = cfgmgr32.NewProc("CM_Request_Device_EjectW")
	procCMGetDevNodeRegistryProperty = cfgmgr32.NewProc("CM_Get_DevNode_Registry_PropertyW")
	procCMGetDeviceInterfaceProperty = cfgmgr32.NewProc("CM_Get_Device_Interface_PropertyW")

	shell32              = windows.NewLazySystemDLL("shell32.dll")
	procSHFileOperationW = shell32.NewProc("SHFileOperationW")
)

const (
	fileProcessIdsUsingFileInformation = 47
	systemExtendedHandleInformation    = 64

	statusInfoLengthMismatch = 0xC0000004
	statusBufferOverflow     = 0x80000005
	statusBufferTooSmall     = 0xC0000023

	wmClose = 0x0010
	gwOwner = 4
)

type ioStatusBlock struct {
	Status      uintptr
	Information uintptr
}

// Entrada de SYSTEM_HANDLE_INFORMATION_EX (40 bytes en x64).
type handleEntry struct {
	Object           uintptr
	PID              uintptr
	Handle           uintptr
	GrantedAccess    uint32
	CreatorBackTrace uint16
	TypeIndex        uint16
	Attributes       uint32
	Reserved         uint32
}

// RM_PROCESS_INFO.
type rmProcessInfo struct {
	PID         uint32
	StartTime   windows.Filetime
	AppName     [256]uint16
	ServiceName [64]uint16
	AppType     uint32
	AppStatus   uint32
	TSSessionID uint32
	Restartable int32
}

// pidsUsingFile devuelve los PID de los procesos que tienen abierto el
// archivo o carpeta del handle, incluidos programas de administrador y
// ejecutables o bibliotecas cargados. ok es false si Windows no lo admite
// (p. ej. en unidades de red).
func pidsUsingFile(h windows.Handle) (pids []uint32, ok bool) {
	buf := make([]byte, 4096)
	for i := 0; i < 6; i++ {
		var iosb ioStatusBlock
		r, _, _ := syscall.SyscallN(procNtQueryInformationFile.Addr(), uintptr(h),
			uintptr(unsafe.Pointer(&iosb)), uintptr(unsafe.Pointer(&buf[0])),
			uintptr(len(buf)), fileProcessIdsUsingFileInformation)
		st := uint32(r)
		if st == statusInfoLengthMismatch || st == statusBufferOverflow || st == statusBufferTooSmall {
			buf = make([]byte, len(buf)*4)
			continue
		}
		if st != 0 {
			return nil, false
		}
		n := int(*(*uint32)(unsafe.Pointer(&buf[0])))
		if max := (len(buf) - 8) / 8; n > max {
			n = max
		}
		for j := 0; j < n; j++ {
			pids = append(pids, uint32(*(*uintptr)(unsafe.Pointer(&buf[8+j*8]))))
		}
		return pids, true
	}
	return nil, false
}

// openForQuery abre un archivo o carpeta solo para leer atributos, sin
// chocar con el modo de compartir del programa que lo bloquea.
func openForQuery(path string) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(longPath(path))
	if err != nil {
		return 0, err
	}
	return windows.CreateFile(p, windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
}

// longPath añade el prefijo \\?\ a rutas largas.
func longPath(p string) string {
	if len(p) < 248 || len(p) >= 4 && p[:4] == `\\?\` {
		return p
	}
	if len(p) >= 2 && p[:2] == `\\` {
		return `\\?\UNC\` + p[2:]
	}
	return `\\?\` + p
}

// finalPathNT devuelve la ruta NT (\Device\HarddiskVolumeN\...) de un handle.
func finalPathNT(h windows.Handle, buf []uint16) (string, []uint16) {
	const flags = 0x0 | 0x2 // FILE_NAME_NORMALIZED | VOLUME_NAME_NT
	if len(buf) < 512 {
		buf = make([]uint16, 512)
	}
	n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), flags)
	if err != nil {
		return "", buf
	}
	if int(n) >= len(buf) {
		buf = make([]uint16, n+1)
		n, err = windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), flags)
		if err != nil || int(n) >= len(buf) {
			return "", buf
		}
	}
	return windows.UTF16ToString(buf[:n]), buf
}
