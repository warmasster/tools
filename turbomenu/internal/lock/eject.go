//go:build windows

package lock

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// EjectError explica por qué Windows no deja expulsar la unidad.
type EjectError struct {
	Veto    int    // tipo de veto de Windows (PNP_VETO_TYPE), 0 si no lo hay
	Blocker string // quién lo impide, si Windows lo dice
	msg     string
}

func (e *EjectError) Error() string { return e.msg }

var (
	guidDiskIface  = windows.GUID{Data1: 0x53f56307, Data2: 0xb6bf, Data3: 0x11d0, Data4: [8]byte{0x94, 0xf2, 0x00, 0xa0, 0xc9, 0x1e, 0xfb, 0x8b}}
	guidCDROMIface = windows.GUID{Data1: 0x53f56308, Data2: 0xb6bf, Data3: 0x11d0, Data4: [8]byte{0x94, 0xf2, 0x00, 0xa0, 0xc9, 0x1e, 0xfb, 0x8b}}
	devpkeyInstID  = windows.DEVPROPKEY{
		FmtID: windows.DEVPROPGUID{Data1: 0x78c34fc8, Data2: 0x104a, Data3: 0x4aca, Data4: [8]byte{0x9e, 0xa4, 0x52, 0x4d, 0x52, 0x99, 0x6e, 0x57}},
		PID:   256,
	}
)

const (
	ioctlStorageGetDeviceNumber = 0x2D1080
	cmDrpCapabilities           = 0x10
	cmDevcapRemovable           = 0x4
	fileDeviceCDROM             = 0x2
)

type storageDeviceNumber struct {
	DeviceType      uint32
	DeviceNumber    uint32
	PartitionNumber uint32
}

// Eject expulsa con seguridad una unidad USB (o un CD) por su letra: «E:»,
// «E:\» o «E». Si Windows lo impide devuelve un *EjectError con el motivo.
func Eject(drive string) error {
	d := strings.TrimRight(strings.TrimSpace(drive), `\/:`)
	if len(d) != 1 || !(d[0] >= 'A' && d[0] <= 'Z' || d[0] >= 'a' && d[0] <= 'z') {
		return errors.New("Unidad no válida.")
	}
	letter := strings.ToUpper(d) + ":"
	if sys := os.Getenv("SystemDrive"); sys != "" && strings.EqualFold(sys, letter) {
		return errors.New("Es el disco de Windows: no se puede expulsar.")
	}
	root, _ := windows.UTF16PtrFromString(letter + `\`)
	switch windows.GetDriveType(root) {
	case windows.DRIVE_NO_ROOT_DIR, windows.DRIVE_UNKNOWN:
		return errors.New("No se encuentra la unidad " + letter + ".")
	case windows.DRIVE_REMOTE:
		return errors.New("Es una unidad de red: desconéctala desde el Explorador.")
	case windows.DRIVE_RAMDISK:
		return errors.New("Esta unidad no se puede expulsar.")
	}

	_, target, err := ejectTarget(letter)
	if err != nil {
		return err
	}

	var last *EjectError
	for try := 0; try < 3; try++ {
		var veto int32
		name := make([]uint16, windows.MAX_PATH)
		r, _, _ := syscall.SyscallN(procCMRequestDeviceEject.Addr(), uintptr(target),
			uintptr(unsafe.Pointer(&veto)), uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)), 0)
		if r == uintptr(windows.CR_SUCCESS) && veto == 0 {
			return nil
		}
		last = &EjectError{Veto: int(veto), Blocker: windows.UTF16ToString(name)}
		last.msg = vetoMessage(int(veto), windows.CONFIGRET(r))
		time.Sleep(300 * time.Millisecond)
	}
	return last
}

// ejectTarget busca el dispositivo de la unidad (dev) y el que hay que
// expulsar (target): el USB que la contiene.
func ejectTarget(letter string) (dev, target windows.DEVINST, err error) {
	num, err := deviceNumber(`\\.\` + letter)
	if err != nil {
		return 0, 0, errors.New("No se puede acceder a la unidad " + letter + ".")
	}
	guid := &guidDiskIface
	if num.DeviceType == fileDeviceCDROM {
		guid = &guidCDROMIface
	}
	ifaces, err := windows.CM_Get_Device_Interface_List("", guid, windows.CM_GET_DEVICE_INTERFACE_LIST_PRESENT)
	if err != nil {
		return 0, 0, errors.New("No se encuentra el dispositivo de la unidad.")
	}
	for _, iface := range ifaces {
		n, err := deviceNumber(iface)
		if err != nil || n.DeviceNumber != num.DeviceNumber || n.DeviceType != num.DeviceType {
			continue
		}
		if id := interfaceInstanceID(iface); id != "" {
			p, _ := windows.UTF16PtrFromString(id)
			if r, _, _ := syscall.SyscallN(procCMLocateDevNode.Addr(), uintptr(unsafe.Pointer(&dev)), uintptr(unsafe.Pointer(p)), 0); r != 0 {
				dev = 0
			}
		}
		break
	}
	if dev == 0 {
		return 0, 0, errors.New("No se encuentra el dispositivo de la unidad.")
	}

	// Se expulsa el dispositivo padre del disco (el USB, como hace «Quitar
	// hardware de forma segura») o, si no se puede quitar, el propio disco.
	var parent windows.DEVINST
	if r, _, _ := syscall.SyscallN(procCMGetParent.Addr(), uintptr(unsafe.Pointer(&parent)), uintptr(dev), 0); r == 0 && devCaps(parent)&cmDevcapRemovable != 0 {
		target = parent
	} else if devCaps(dev)&cmDevcapRemovable != 0 {
		target = dev
	}
	if target == 0 {
		return dev, 0, errors.New("Esta unidad no se puede expulsar: es un disco interno.")
	}
	return dev, target, nil
}

func deviceNumber(path string) (storageDeviceNumber, error) {
	var n storageDeviceNumber
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return n, err
	}
	h, err := windows.CreateFile(p, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return n, err
	}
	defer windows.CloseHandle(h)
	var ret uint32
	err = windows.DeviceIoControl(h, ioctlStorageGetDeviceNumber, nil, 0,
		(*byte)(unsafe.Pointer(&n)), uint32(unsafe.Sizeof(n)), &ret, nil)
	return n, err
}

func interfaceInstanceID(iface string) string {
	p, err := windows.UTF16PtrFromString(iface)
	if err != nil {
		return ""
	}
	buf := make([]uint16, 512)
	size := uint32(len(buf) * 2)
	var typ uint32
	r, _, _ := syscall.SyscallN(procCMGetDeviceInterfaceProperty.Addr(), uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&devpkeyInstID)), uintptr(unsafe.Pointer(&typ)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0)
	if r != 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

func devCaps(dev windows.DEVINST) uint32 {
	var caps, typ uint32
	size := uint32(4)
	r, _, _ := syscall.SyscallN(procCMGetDevNodeRegistryProperty.Addr(), uintptr(dev), cmDrpCapabilities,
		uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&caps)), uintptr(unsafe.Pointer(&size)), 0)
	if r != 0 {
		return 0
	}
	return caps
}

// vetoMessage traduce el motivo de Windows (PNP_VETO_TYPE).
func vetoMessage(veto int, cr windows.CONFIGRET) string {
	switch veto {
	case 0:
		if cr == windows.CR_REMOVE_VETOED {
			return "Windows no deja expulsarla ahora mismo. Cierra lo que esté usando la unidad y vuelve a probar."
		}
		return "No se ha podido expulsar la unidad."
	case 2: // PendingClose
		return "Windows está terminando de cerrar archivos. Vuelve a probar en unos segundos."
	case 3, 5: // WindowsApp, OutstandingOpen
		return "Hay programas usando la unidad. Usa «¿Qué lo bloquea?» para verlos."
	case 4: // WindowsService
		return "Un servicio de Windows está usando la unidad."
	case 6, 7, 11: // Device, Driver, LegacyDriver
		return "Un controlador del sistema está usando la unidad."
	case 9: // InsufficientPower
		return "No se puede expulsar por un problema de energía del dispositivo."
	case 10: // NonDisableable
		return "Windows necesita esta unidad y no deja quitarla."
	case 12: // InsufficientRights
		return "Hace falta ser administrador para expulsar esta unidad."
	case 13: // AlreadyRemoved
		return "La unidad ya estaba quitada."
	}
	return "Windows no deja expulsar la unidad."
}
