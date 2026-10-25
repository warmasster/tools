// Package winsec reúne las protecciones de Windows que usa TurboVAULT: que otros
// programas no puedan leer la memoria de sus procesos, que no se les cuelen
// DLL, que su carpeta solo la vea el usuario, que la ventana no salga en
// capturas de pantalla, y saber quién está al otro lado de una conexión.
package winsec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                    = windows.NewLazySystemDLL("kernel32.dll")
	user32                      = windows.NewLazySystemDLL("user32.dll")
	procSetProcessMitigation    = kernel32.NewProc("SetProcessMitigationPolicy")
	procSetWindowDisplayAffin   = user32.NewProc("SetWindowDisplayAffinity")
	procQueryFullProcessImageNm = kernel32.NewProc("QueryFullProcessImageNameW")
)

// UserSID devuelve el SID del usuario actual ("S-1-5-21-...").
func UserSID() (string, error) {
	tok := windows.GetCurrentProcessToken()
	u, err := tok.GetTokenUser()
	if err != nil {
		return "", err
	}
	return u.User.Sid.String(), nil
}

// Permisos que se dejan sobre los procesos de TurboVAULT a cualquier programa
// del usuario: ver que existe, esperar a que acabe y cerrarlo (el Administrador
// de tareas y taskkill lo necesitan). Nada de leer o escribir su memoria, ni de cambiar
// estos permisos.
const limitedAccess = windows.PROCESS_QUERY_LIMITED_INFORMATION | windows.PROCESS_QUERY_INFORMATION | windows.SYNCHRONIZE | windows.PROCESS_TERMINATE

// HardenProcess cambia los permisos del proceso actual para que los demás
// programas del mismo usuario no puedan leer su memoria ni inyectarle código.
// La entrada "OWNER RIGHTS" quita al dueño el permiso implícito de cambiar los
// permisos, así que tampoco puede deshacerse. Un administrador con privilegio
// de depuración sí podría.
func HardenProcess() error {
	sid, err := UserSID()
	if err != nil {
		return err
	}
	sddl := fmt.Sprintf("D:P(A;;GA;;;SY)(A;;0x%x;;;%s)(A;;0x%x;;;OW)", limitedAccess, sid, limitedAccess)
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetSecurityInfo(windows.CurrentProcess(), windows.SE_KERNEL_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

// Mitigaciones de SetProcessMitigationPolicy.
const (
	policySignature          = 8  // ProcessSignaturePolicy
	policyExtensionPoint     = 6  // ProcessExtensionPointDisablePolicy
	policyImageLoad          = 10 // ProcessImageLoadPolicy
	flagMicrosoftSignedOnly  = 1
	flagDisableExtensionPts  = 1
	flagNoRemoteImages       = 1
	flagNoLowMandatoryImages = 2
)

func setMitigation(policy int, flags uint32) error {
	r, _, err := procSetProcessMitigation.Call(uintptr(policy), uintptr(unsafe.Pointer(&flags)), unsafe.Sizeof(flags))
	if r == 0 {
		return err
	}
	return nil
}

// BlockInjection impide que se carguen en el proceso DLL de los "puntos de
// extensión" de Windows (AppInit, ganchos globales, LSP), DLL de red o de
// procesos de baja integridad. Con strict, además solo DLL firmadas por
// Microsoft (para los procesos sin ventana web).
func BlockInjection(strict bool) error {
	var errs []error
	if err := setMitigation(policyExtensionPoint, flagDisableExtensionPts); err != nil {
		errs = append(errs, err)
	}
	if err := setMitigation(policyImageLoad, flagNoRemoteImages|flagNoLowMandatoryImages); err != nil {
		errs = append(errs, err)
	}
	if strict {
		if err := setMitigation(policySignature, flagMicrosoftSignedOnly); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// SecureDir crea la carpeta (si no existe) y deja que solo la abran el usuario
// actual y el sistema. Lo que haya dentro hereda esos permisos.
func SecureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	sid, err := UserSID()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString(fmt.Sprintf("D:P(A;OICI;FA;;;%s)(A;OICI;FA;;;SY)", sid))
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

// ExcludeFromCapture hace que la ventana salga en negro en capturas de
// pantalla, grabaciones y pantallas compartidas.
func ExcludeFromCapture(hwnd uintptr) bool {
	const wdaExcludeFromCapture = 0x11
	r, _, _ := procSetWindowDisplayAffin.Call(hwnd, wdaExcludeFromCapture)
	return r != 0
}

// ProcessPath devuelve la ruta completa del ejecutable de un proceso.
func ProcessPath(pid uint32) (string, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	return imagePath(h)
}

func imagePath(h windows.Handle) (string, error) {
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	r, _, err := procQueryFullProcessImageNm.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return "", err
	}
	return windows.UTF16ToString(buf[:n]), nil
}

// RawCommandLine devuelve la línea de órdenes de un proceso tal cual.
func RawCommandLine(pid uint32) (string, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	const processCommandLineInformation = 60
	buf := make([]byte, 64*1024)
	var ret uint32
	if err := windows.NtQueryInformationProcess(h, processCommandLineInformation, unsafe.Pointer(&buf[0]), uint32(len(buf)), &ret); err != nil {
		return "", err
	}
	us := (*windows.NTUnicodeString)(unsafe.Pointer(&buf[0]))
	return us.String(), nil
}

// CommandLine devuelve la línea de órdenes de un proceso, ya separada.
func CommandLine(pid uint32) ([]string, error) {
	line, err := RawCommandLine(pid)
	if err != nil {
		return nil, err
	}
	return windows.DecomposeCommandLine(line)
}

// ParentPID devuelve el proceso que lanzó a pid.
func ParentPID(pid uint32) (uint32, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if e.ProcessID == pid {
			return e.ParentProcessID, nil
		}
	}
	return 0, errors.New("proceso no encontrado")
}

// SamePath compara dos rutas de Windows.
func SamePath(a, b string) bool {
	return a != "" && strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// FileID identifica un archivo por su volumen y su número de archivo (no por
// la ruta): si alguien renombra el .exe y pone otro en su sitio, el número cambia.
type FileID struct {
	Volume uint32
	Index  uint64
}

// FileIDOf devuelve el FileID del archivo de esa ruta.
func FileIDOf(path string) (FileID, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return FileID{}, err
	}
	h, err := windows.CreateFile(p, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return FileID{}, err
	}
	defer windows.CloseHandle(h)
	var fi windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &fi); err != nil {
		return FileID{}, err
	}
	return FileID{Volume: fi.VolumeSerialNumber, Index: uint64(fi.FileIndexHigh)<<32 | uint64(fi.FileIndexLow)}, nil
}

// ProcessFileID devuelve el FileID del ejecutable de un proceso.
func ProcessFileID(pid uint32) (string, FileID, error) {
	path, err := ProcessPath(pid)
	if err != nil {
		return "", FileID{}, err
	}
	id, err := FileIDOf(path)
	return path, id, err
}

// ProcessAttributes son los permisos con los que nacen los procesos que lanza
// TurboVAULT: protegidos desde el primer instante, sin esperar a que ellos
// mismos llamen a HardenProcess.
func ProcessAttributes() (*syscall.SecurityAttributes, error) {
	sid, err := UserSID()
	if err != nil {
		return nil, err
	}
	sd, err := windows.SecurityDescriptorFromString(fmt.Sprintf("D:P(A;;GA;;;SY)(A;;0x%x;;;%s)(A;;0x%x;;;OW)", limitedAccess, sid, limitedAccess))
	if err != nil {
		return nil, err
	}
	return &syscall.SecurityAttributes{Length: uint32(unsafe.Sizeof(syscall.SecurityAttributes{})), SecurityDescriptor: uintptr(unsafe.Pointer(sd))}, nil
}

// Permisos que se dejan sobre los hilos: ver que existen y esperar a que
// acaben. Nada de pararlos, cambiar sus registros (para desviarlos a otro
// código) o cambiar sus permisos.
const (
	threadQueryLimited = 0x0800 // THREAD_QUERY_LIMITED_INFORMATION
	threadAccess       = threadQueryLimited | windows.SYNCHRONIZE
	tokenDefaultDacl   = 6 // TokenDefaultDacl
)

func threadSD() (*windows.SECURITY_DESCRIPTOR, error) {
	sid, err := UserSID()
	if err != nil {
		return nil, err
	}
	return windows.SecurityDescriptorFromString(fmt.Sprintf("D:P(A;;GA;;;SY)(A;;0x%x;;;%s)(A;;0x%x;;;OW)", threadAccess, sid, threadAccess))
}

// ThreadAttributes son los permisos con los que nace el primer hilo de un
// proceso lanzado por TurboVAULT.
func ThreadAttributes() (*syscall.SecurityAttributes, error) {
	sd, err := threadSD()
	if err != nil {
		return nil, err
	}
	return &syscall.SecurityAttributes{Length: uint32(unsafe.Sizeof(syscall.SecurityAttributes{})), SecurityDescriptor: uintptr(unsafe.Pointer(sd))}, nil
}

// HardenThreads protege los hilos del proceso: los que ya existen y los que
// se creen después (cambiando los permisos por defecto del token del proceso).
// Sin esto, otro programa del usuario podría abrir un hilo, pararlo y
// cambiarle los registros para ejecutar lo que quisiera dentro del proceso.
// Solo para procesos sin navegador interno (los permisos por defecto también
// afectan a otros objetos sin permisos propios).
func HardenThreads() error {
	sid, err := UserSID()
	if err != nil {
		return err
	}
	// 1. Por defecto, los objetos nuevos (hilos incluidos) nacen protegidos.
	dsd, err := windows.SecurityDescriptorFromString(fmt.Sprintf("D:(A;;GA;;;SY)(A;;0x%x;;;%s)(A;;0x%x;;;OW)", threadAccess, sid, threadAccess))
	if err != nil {
		return err
	}
	ddacl, _, err := dsd.DACL()
	if err != nil {
		return err
	}
	defaultMu.Lock()
	if origDefault == nil {
		origDefault = currentDefaultDacl()
		// Si ya venían protegidos (lo lanzó otro proceso protegido), los
		// «originales» para los hijos son los normales de un usuario.
		hard := unsafe.Slice((*byte)(unsafe.Pointer(ddacl)), int(*(*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(ddacl)) + 2))))
		if origDefault == nil || string(origDefault) == string(hard) {
			if nsd, err := windows.SecurityDescriptorFromString(fmt.Sprintf("D:(A;;GA;;;%s)(A;;GA;;;SY)", sid)); err == nil {
				if nd, _, err := nsd.DACL(); err == nil {
					size := int(*(*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(nd)) + 2)))
					origDefault = append([]byte(nil), unsafe.Slice((*byte)(unsafe.Pointer(nd)), size)...)
				}
			}
		}
	}
	hardDefault = ddacl
	err = setDefaultDacl(ddacl)
	defaultMu.Unlock()
	if err != nil {
		return err
	}
	// 2. Los hilos que ya existen.
	return HardenExistingThreads()
}

// TokenUsable dice si el proceso puede abrir su propio token para consultarlo
// y duplicarlo (lo necesita para lanzar procesos y comprobar firmas).
func TokenUsable() bool {
	var tok windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY|windows.TOKEN_DUPLICATE, &tok); err != nil {
		return false
	}
	tok.Close()
	return true
}

var (
	defaultMu   sync.Mutex
	origDefault []byte       // permisos por defecto originales del token (copia)
	hardDefault *windows.ACL // los protegidos
)

func setDefaultDacl(acl *windows.ACL) error {
	var tok windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_DEFAULT|windows.TOKEN_QUERY, &tok); err != nil {
		return err
	}
	defer tok.Close()
	td := struct{ DefaultDacl *windows.ACL }{acl}
	return windows.SetTokenInformation(tok, tokenDefaultDacl, (*byte)(unsafe.Pointer(&td)), uint32(unsafe.Sizeof(td)))
}

// currentDefaultDacl copia la lista de permisos por defecto del token.
func currentDefaultDacl() []byte {
	tok := windows.GetCurrentProcessToken()
	var n uint32
	windows.GetTokenInformation(tok, tokenDefaultDacl, nil, 0, &n)
	if n == 0 {
		return nil
	}
	buf := make([]byte, n)
	if err := windows.GetTokenInformation(tok, tokenDefaultDacl, &buf[0], n, &n); err != nil {
		return nil
	}
	p := *(**windows.ACL)(unsafe.Pointer(&buf[0]))
	if p == nil {
		return nil
	}
	size := int(*(*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(p)) + 2))) // ACL.AclSize
	return append([]byte(nil), unsafe.Slice((*byte)(unsafe.Pointer(p)), size)...)
}

// WithNormalDefaults ejecuta fn (lanzar un proceso hijo) con los permisos por
// defecto originales del token, que es lo que hereda el hijo: la ventana lleva
// un navegador interno que no funciona con los protegidos. Los hilos que el
// proceso cree mientras tanto se vuelven a proteger después.
func WithNormalDefaults(fn func() error) error {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if origDefault == nil || hardDefault == nil {
		return fn()
	}
	acl := (*windows.ACL)(unsafe.Pointer(&origDefault[0]))
	if err := setDefaultDacl(acl); err != nil {
		return err
	}
	err := fn()
	setDefaultDacl(hardDefault)
	HardenExistingThreads()
	return err
}

// HardenExistingThreads protege solo los hilos que existen ahora (para la
// ventana, cuyo navegador interno no admite cambiar los permisos por defecto;
// se repite de vez en cuando para cubrir los hilos nuevos).
func HardenExistingThreads() error {
	sd, err := threadSD()
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	pid := windows.GetCurrentProcessId()
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snap)
	var te windows.ThreadEntry32
	te.Size = uint32(unsafe.Sizeof(te))
	var errs []error
	for err = windows.Thread32First(snap, &te); err == nil; err = windows.Thread32Next(snap, &te) {
		if te.OwnerProcessID != pid {
			continue
		}
		h, err := windows.OpenThread(windows.WRITE_DAC, false, te.ThreadID)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := windows.SetSecurityInfo(h, windows.SE_KERNEL_OBJECT,
			windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
			errs = append(errs, err)
		}
		windows.CloseHandle(h)
	}
	return errors.Join(errs...)
}
