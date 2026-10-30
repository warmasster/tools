package main

// Cámara nativa de Windows 11. La cámara de siempre es un filtro de DirectShow que
// cargan las propias apps; algunas ya no lo dejan (Discord, desde su versión de
// octubre de 2026, no carga DLL de terceros en su proceso). La nativa la carga el
// servicio de cámaras de Windows (Frame Server) y las apps la abren como una webcam
// de verdad, sin cargar nada nuestro. Además la ven las apps que solo usan Media
// Foundation (la app Cámara de Windows).
//
// Darla de alta pide administrador una vez: la DLL (camera/mfcam.cpp) va a
// %ProgramFiles%\TurboCAM (el servicio corre como LOCAL SERVICE y no puede leer la
// carpeta del usuario), su CLSID se registra en HKLM y se crea la cámara con
// MFCreateVirtualCamera, que queda en el sistema aunque se reinicie.

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

//go:embed camera/bin/turbocammf64.dll
var camDLLMF []byte

const nativeCLSID = "{EA00E74F-EAEE-4F98-B997-53B20BE0F23A}" // también en camera/mfcam.cpp

var (
	mfsensorgroup             = windows.NewLazySystemDLL("mfsensorgroup.dll")
	procMFCreateVirtualCamera = mfsensorgroup.NewProc("MFCreateVirtualCamera")
)

const (
	vcamTypeSoftware   = 0
	vcamLifetimeSystem = 1
	vcamAccessAllUsers = 1
	vtblVCamStart      = 36
	vtblVCamRemove     = 38
	vtblVCamShutdown   = 43
)

// nativeCamSupported: Windows 11 (MFCreateVirtualCamera existe desde la 22000).
func nativeCamSupported() bool { return procMFCreateVirtualCamera.Find() == nil }

func nativeCamRoot() string {
	pf := os.Getenv("ProgramW6432")
	if pf == "" {
		pf = os.Getenv("ProgramFiles")
	}
	return filepath.Join(pf, "TurboCAM", "camera")
}

func nativeCamHash() string {
	h := sha256.Sum256(camDLLMF)
	return hex.EncodeToString(h[:])[:12]
}

const nativeCLSIDKey = `SOFTWARE\Classes\CLSID\` + nativeCLSID

// nativeCamInstalled dice si la cámara nativa está dada de alta (con esta versión
// de la DLL o con una anterior: el contrato con TurboCAM es el mismo, así que una
// actualización no tiene que volver a pedir permiso de administrador).
func nativeCamInstalled() bool {
	if !nativeCamSupported() {
		return false
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, nativeCLSIDKey+`\InprocServer32`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return false
	}
	defer k.Close()
	path, _, err := k.GetStringValue("")
	return err == nil && fileExists(path)
}

// withMF ejecuta f en un hilo fijo con COM y Media Foundation iniciados.
func withMF(f func() error) error {
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		comInitThread()
		if err := procMFStartup.Find(); err != nil {
			done <- err
			return
		}
		procMFStartup.Call(mfVersion, 0)
		done <- f()
	}()
	return <-done
}

// virtualCamera crea (o abre, si ya existe) la cámara nativa.
func virtualCamera() (*comObj, error) {
	if !nativeCamSupported() {
		return nil, errors.New("hace falta Windows 11")
	}
	name, _ := windows.UTF16PtrFromString(camName)
	src, _ := windows.UTF16PtrFromString(nativeCLSID)
	var vc *comObj
	r, _, _ := procMFCreateVirtualCamera.Call(vcamTypeSoftware, vcamLifetimeSystem, vcamAccessAllUsers,
		uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(src)), 0, 0, uintptr(unsafe.Pointer(&vc)))
	if err := hrErr("MFCreateVirtualCamera", uint32(r)); err != nil {
		return nil, err
	}
	return vc, nil
}

// installNativeCam da de alta la cámara nativa. Hace falta ser administrador.
func installNativeCam() error {
	dir := filepath.Join(nativeCamRoot(), nativeCamHash())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("no se pudo crear %s: %w", dir, err)
	}
	dll := filepath.Join(dir, "turbocammf64.dll")
	if err := writeIfDifferent(dll, camDLLMF); err != nil {
		return fmt.Errorf("no se pudo guardar la cámara nativa: %w", err)
	}
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, nativeCLSIDKey, registry.ALL_ACCESS|registry.WOW64_64KEY)
	if err != nil {
		return fmt.Errorf("no se pudo registrar la cámara nativa (¿sin permiso de administrador?): %w", err)
	}
	k.SetStringValue("", camName)
	k.Close()
	k, _, err = registry.CreateKey(registry.LOCAL_MACHINE, nativeCLSIDKey+`\InprocServer32`, registry.ALL_ACCESS|registry.WOW64_64KEY)
	if err != nil {
		return err
	}
	k.SetStringValue("", dll)
	k.SetStringValue("ThreadingModel", "Both")
	k.Close()
	err = withMF(func() error {
		vc, err := virtualCamera()
		if err != nil {
			return err
		}
		defer vc.release()
		hr := vc.call(vtblVCamStart, 0)
		vc.call(vtblVCamShutdown)
		return hrErr("IMFVirtualCamera.Start", hr)
	})
	if err != nil {
		return fmt.Errorf("Windows no ha aceptado la cámara nativa: %w", err)
	}
	cleanNativeDirs(nativeCamHash())
	return nil
}

// removeNativeCam la quita del sistema. Hace falta ser administrador.
func removeNativeCam() error {
	err := withMF(func() error {
		vc, err := virtualCamera()
		if err != nil {
			return err
		}
		defer vc.release()
		return hrErr("IMFVirtualCamera.Remove", vc.call(vtblVCamRemove))
	})
	registry.DeleteKey(registry.LOCAL_MACHINE, nativeCLSIDKey+`\InprocServer32`)
	registry.DeleteKey(registry.LOCAL_MACHINE, nativeCLSIDKey)
	cleanNativeDirs("")
	os.Remove(filepath.Dir(nativeCamRoot()))
	return err
}

// cleanNativeDirs borra las versiones viejas de la DLL (las que no estén en uso).
func cleanNativeDirs(keep string) {
	ents, _ := os.ReadDir(nativeCamRoot())
	for _, e := range ents {
		if e.IsDir() && e.Name() != keep {
			os.RemoveAll(filepath.Join(nativeCamRoot(), e.Name()))
		}
	}
	if keep == "" {
		os.Remove(nativeCamRoot())
	}
}

// cliNativeCam: TurboCAM.exe nativecam install|remove [resultado.txt]. Corre elevado
// (sin consola): el resultado se deja en el archivo, si se da.
func cliNativeCam(args []string) int {
	if len(args) == 0 {
		return 2
	}
	var err error
	switch args[0] {
	case "install":
		err = installNativeCam()
	case "remove":
		err = removeNativeCam()
	default:
		return 2
	}
	msg := "ok"
	if err != nil {
		msg = err.Error()
	}
	if len(args) > 1 {
		os.WriteFile(args[1], []byte(msg), 0o644)
	}
	if err != nil {
		return 1
	}
	return 0
}

// ---------- con permiso de administrador ----------

var (
	shell32             = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteExW = shell32.NewProc("ShellExecuteExW")
)

// shellExecuteInfo es SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     uintptr
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    uintptr
	dwHotKey     uint32
	hIcon        uintptr
	hProcess     windows.Handle
}

// errUACCancelled: no se ha dado el permiso de administrador.
var errUACCancelled = errors.New("no se ha dado permiso de administrador")

// nativeCamElevated lanza «TurboCAM.exe nativecam <op>» con permiso de
// administrador (Windows lo pregunta) y espera a que acabe.
func nativeCamElevated(op string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	res := filepath.Join(os.TempDir(), fmt.Sprintf("turbocam-nativecam-%d.txt", os.Getpid()))
	os.Remove(res)
	defer os.Remove(res)
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	params, _ := windows.UTF16PtrFromString(fmt.Sprintf(`nativecam %s "%s"`, op, res))
	info := shellExecuteInfo{
		fMask:        0x40 | 0x100, // SEE_MASK_NOCLOSEPROCESS | SEE_MASK_NOASYNC
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: params,
		nShow:        0, // SW_HIDE
	}
	info.cbSize = uint32(unsafe.Sizeof(info))
	r, _, e := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		if e == windows.ERROR_CANCELLED {
			return errUACCancelled
		}
		return fmt.Errorf("no se pudo pedir permiso de administrador: %v", e)
	}
	if info.hProcess != 0 {
		windows.WaitForSingleObject(info.hProcess, 60000)
		windows.CloseHandle(info.hProcess)
	}
	b, err := os.ReadFile(res)
	if err != nil {
		return errors.New("no ha contestado (¿se ha cancelado?)")
	}
	if msg := strings.TrimSpace(string(b)); msg != "ok" {
		return errors.New(msg)
	}
	return nil
}

// enableNativeCam activa la cámara nativa y esconde la de siempre (si no, saldrían
// dos «TurboCAM»: la nativa ya la ven también las apps de DirectShow).
func enableNativeCam() error {
	if !nativeCamSupported() {
		return errors.New("la cámara nativa necesita Windows 11")
	}
	if err := nativeCamElevated("install"); err != nil {
		return err
	}
	return installCamera()
}

// disableNativeCam la quita y vuelve a dejar a la vista la de siempre.
func disableNativeCam() error {
	err := nativeCamElevated("remove")
	if ierr := installCamera(); err == nil {
		err = ierr
	}
	return err
}
