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

var (
	wintrust                   = windows.NewLazySystemDLL("wintrust.dll")
	procProvDataFromStateData  = wintrust.NewProc("WTHelperProvDataFromStateData")
	procGetProvSignerFromChain = wintrust.NewProc("WTHelperGetProvSignerFromChain")
)

// Estructuras de wintrust (solo los campos del principio, que son los que se leen).
type cryptProviderSgnr struct {
	Size         uint32
	VerifyAsOf   windows.Filetime
	CertChainLen uint32
	CertChain    *cryptProviderCert
}

type cryptProviderCert struct {
	Size uint32
	Cert *windows.CertContext
}

// verifyCloudflareSignature comprueba la firma Authenticode y que quien firma sea
// Cloudflare. Se mira el certificado del firmante que Windows acaba de validar, no
// cualquiera de los que van dentro del archivo (ahí se puede meter cualquier cosa).
func verifyCloudflareSignature(path string) error {
	name, err := verifiedSigner(path)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(name, "Cloudflare") {
		return errors.New("el ejecutable no está firmado por Cloudflare")
	}
	return nil
}

// verifiedSigner valida la firma de un archivo y devuelve el nombre de su firmante.
func verifiedSigner(path string) (string, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	fi := &windows.WinTrustFileInfo{Size: uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})), FilePath: p}
	data := &windows.WinTrustData{
		Size:                            uint32(unsafe.Sizeof(windows.WinTrustData{})),
		UIChoice:                        windows.WTD_UI_NONE,
		RevocationChecks:                windows.WTD_REVOKE_NONE,
		UnionChoice:                     windows.WTD_CHOICE_FILE,
		StateAction:                     windows.WTD_STATEACTION_VERIFY,
		FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(fi),
		// Sin ir a internet a por listas de revocación: sin red no se queda esperando.
		ProvFlags: windows.WTD_REVOCATION_CHECK_NONE | windows.WTD_CACHE_ONLY_URL_RETRIEVAL,
	}
	verr := windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	defer func() {
		data.StateAction = windows.WTD_STATEACTION_CLOSE
		windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	}()
	if verr != nil {
		return "", errors.New("firma digital no válida")
	}
	if procProvDataFromStateData.Find() != nil || procGetProvSignerFromChain.Find() != nil {
		return "", errors.New("no se pudo leer la firma")
	}
	prov, _, _ := procProvDataFromStateData.Call(uintptr(data.StateData))
	if prov == 0 {
		return "", errors.New("no se pudo leer la firma")
	}
	sp, _, _ := procGetProvSignerFromChain.Call(prov, 0, 0, 0)
	if sp == 0 {
		return "", errors.New("no se pudo leer la firma")
	}
	// Memoria de wintrust (no de Go), válida hasta cerrar el estado.
	sgnr := (*cryptProviderSgnr)(*(*unsafe.Pointer)(unsafe.Pointer(&sp)))
	if sgnr.CertChainLen == 0 || sgnr.CertChain == nil || sgnr.CertChain.Cert == nil {
		return "", errors.New("no se pudo leer la firma")
	}
	buf := make([]uint16, 256)
	n := windows.CertGetNameString(sgnr.CertChain.Cert, windows.CERT_NAME_SIMPLE_DISPLAY_TYPE, 0, nil, &buf[0], uint32(len(buf)))
	if n <= 1 {
		return "", errors.New("no se pudo leer la firma")
	}
	return windows.UTF16ToString(buf[:n]), nil
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
