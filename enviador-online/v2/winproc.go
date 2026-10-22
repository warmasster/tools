package main

import (
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const createNoWindow = 0x08000000

// hideConsole hace que el proceso hijo no abra ninguna ventana de terminal.
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

var killJob windows.Handle

// attachKillOnClose mete el proceso en un Job que Windows mata si TurboSEND muere.
func attachKillOnClose(pid int) {
	if killJob == 0 {
		h, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			return
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
			windows.CloseHandle(h)
			return
		}
		killJob = h
	}
	ph, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return
	}
	windows.AssignProcessToJobObject(killJob, ph)
	windows.CloseHandle(ph)
}

// verifyCloudflareSignature comprueba la firma Authenticode y que el firmante sea Cloudflare.
func verifyCloudflareSignature(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	fi := &windows.WinTrustFileInfo{Size: uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})), FilePath: p}
	data := &windows.WinTrustData{
		Size:                            uint32(unsafe.Sizeof(windows.WinTrustData{})),
		UIChoice:                        windows.WTD_UI_NONE,
		RevocationChecks:                windows.WTD_REVOKE_NONE,
		UnionChoice:                     windows.WTD_CHOICE_FILE,
		StateAction:                     windows.WTD_STATEACTION_VERIFY,
		FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(fi),
	}
	verr := windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	data.StateAction = windows.WTD_STATEACTION_CLOSE
	windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	if verr != nil {
		return errors.New("firma digital no válida")
	}

	var enc, ctype, ftype uint32
	var store, msg windows.Handle
	if err := windows.CryptQueryObject(windows.CERT_QUERY_OBJECT_FILE, unsafe.Pointer(p),
		windows.CERT_QUERY_CONTENT_FLAG_PKCS7_SIGNED_EMBED, windows.CERT_QUERY_FORMAT_FLAG_BINARY, 0,
		&enc, &ctype, &ftype, &store, &msg, nil); err != nil {
		return errors.New("no se pudo leer la firma")
	}
	defer windows.CertCloseStore(store, 0)
	var cert *windows.CertContext
	for {
		cert, err = windows.CertEnumCertificatesInStore(store, cert)
		if err != nil || cert == nil {
			break
		}
		buf := make([]uint16, 256)
		n := windows.CertGetNameString(cert, windows.CERT_NAME_SIMPLE_DISPLAY_TYPE, 0, nil, &buf[0], uint32(len(buf)))
		if n > 1 && strings.HasPrefix(windows.UTF16ToString(buf[:n]), "Cloudflare") {
			windows.CertFreeCertificateContext(cert)
			return nil
		}
	}
	return errors.New("el ejecutable no está firmado por Cloudflare")
}

var (
	user32            = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW   = user32.NewProc("FindWindowW")
	procFlashWindowEx = user32.NewProc("FlashWindowEx")
	procGetForeground = user32.NewProc("GetForegroundWindow")
)

const windowClass = "EnviadorV2Window"

type flashInfo struct {
	Size    uint32
	Hwnd    uintptr
	Flags   uint32
	Count   uint32
	Timeout uint32
}

// flashWindow hace parpadear el botón de la barra de tareas hasta que el usuario vuelva.
func flashWindow(_ string) {
	cls, _ := windows.UTF16PtrFromString(windowClass)
	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(cls)), 0)
	if hwnd == 0 {
		return
	}
	if fg, _, _ := procGetForeground.Call(); fg == hwnd {
		return
	}
	fi := flashInfo{Hwnd: hwnd, Flags: 0x3 | 0xC, Count: 0} // FLASHW_ALL | FLASHW_TIMERNOFG
	fi.Size = uint32(unsafe.Sizeof(fi))
	procFlashWindowEx.Call(uintptr(unsafe.Pointer(&fi)))
}
