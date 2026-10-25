package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32              = windows.NewLazySystemDLL("shell32.dll")
	procSHFileOperationW = shell32.NewProc("SHFileOperationW")
	procSHEmptyRecycleW  = shell32.NewProc("SHEmptyRecycleBinW")
	procSHQueryRecycleW  = shell32.NewProc("SHQueryRecycleBinW")

	rstrtmgr                = windows.NewLazySystemDLL("rstrtmgr.dll")
	procRmStartSession      = rstrtmgr.NewProc("RmStartSession")
	procRmRegisterResources = rstrtmgr.NewProc("RmRegisterResources")
	procRmGetList           = rstrtmgr.NewProc("RmGetList")
	procRmEndSession        = rstrtmgr.NewProc("RmEndSession")
)

type shFileOpStruct struct {
	hwnd          uintptr
	wFunc         uint32
	pFrom         *uint16
	pTo           *uint16
	fFlags        uint16
	fAnyAborted   int32
	hNameMappings uintptr
	lpszTitle     *uint16
}

const (
	foDelete           = 3
	fofSilent          = 0x0004
	fofNoConfirmation  = 0x0010
	fofAllowUndo       = 0x0040
	fofNoErrorUI       = 0x0400
	fofWantNukeWarning = 0x4000
)

// toRecycleBin manda las rutas a la Papelera de una vez. Si algo no cabe en la
// papelera, Windows avisa antes de borrarlo para siempre.
func toRecycleBin(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	var buf []uint16
	for _, p := range paths {
		u, err := windows.UTF16FromString(p)
		if err != nil {
			return err
		}
		buf = append(buf, u...) // UTF16FromString ya trae su 0 final
	}
	buf = append(buf, 0)
	op := shFileOpStruct{
		wFunc:  foDelete,
		pFrom:  &buf[0],
		fFlags: fofAllowUndo | fofNoConfirmation | fofWantNukeWarning | fofNoErrorUI,
	}
	r, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	if op.fAnyAborted != 0 {
		return errors.New("cancelado")
	}
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

// emptyRecycleBin vacía la papelera de una unidad (de este usuario).
func emptyRecycleBin(root string) error {
	p, _ := windows.UTF16PtrFromString(root)
	const flags = 0x1 | 0x2 | 0x4 // sin confirmación, sin progreso, sin sonido
	r, _, _ := procSHEmptyRecycleW.Call(0, uintptr(unsafe.Pointer(p)), flags)
	if r != 0 && r != 0x8000FFFF { // E_UNEXPECTED = ya estaba vacía
		return syscall.Errno(r)
	}
	return nil
}

// recycleBinSize dice cuánto ocupa la papelera de una unidad.
func recycleBinSize(root string) (size, items int64) {
	type info struct {
		cbSize      uint32
		_           uint32
		i64Size     int64
		i64NumItems int64
	}
	in := info{cbSize: uint32(unsafe.Sizeof(info{}))}
	p, _ := windows.UTF16PtrFromString(root)
	r, _, _ := procSHQueryRecycleW.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&in)))
	if r != 0 {
		return 0, 0
	}
	return in.i64Size, in.i64NumItems
}

// lockingApps usa el Restart Manager para saber qué programas tienen abiertos estos archivos.
func lockingApps(files []string) []string {
	if len(files) == 0 || procRmStartSession.Find() != nil {
		return nil
	}
	var h uint32
	key := make([]uint16, 64)
	if r, _, _ := procRmStartSession.Call(uintptr(unsafe.Pointer(&h)), 0, uintptr(unsafe.Pointer(&key[0]))); r != 0 {
		return nil
	}
	defer procRmEndSession.Call(uintptr(h))
	ptrs := make([]*uint16, 0, len(files))
	for _, f := range files {
		if p, err := windows.UTF16PtrFromString(f); err == nil {
			ptrs = append(ptrs, p)
		}
	}
	if len(ptrs) == 0 {
		return nil
	}
	if r, _, _ := procRmRegisterResources.Call(uintptr(h), uintptr(len(ptrs)), uintptr(unsafe.Pointer(&ptrs[0])), 0, 0, 0, 0); r != 0 {
		return nil
	}
	type rmProcessInfo struct {
		pid         uint32
		start       windows.Filetime
		appName     [256]uint16
		serviceName [64]uint16
		appType     uint32
		appStatus   uint32
		session     uint32
		restartable int32
	}
	var needed, count uint32
	var reasons uint32
	list := make([]rmProcessInfo, 16)
	count = uint32(len(list))
	r, _, _ := procRmGetList.Call(uintptr(h), uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&count)),
		uintptr(unsafe.Pointer(&list[0])), uintptr(unsafe.Pointer(&reasons)))
	if r != 0 && r != 234 { // ERROR_MORE_DATA: nos quedamos con los primeros
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for i := 0; i < int(count) && i < len(list); i++ {
		name := windows.UTF16ToString(list[i].appName[:])
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// explorerPath: la ruta va entre comillas en la línea de explorer.exe, así que una
// comilla dentro la rompería (y dejaría colar otros argumentos).
func explorerPath(path string) error {
	if path == "" || strings.ContainsAny(path, "\"\r\n") || !filepath.IsAbs(path) {
		return errors.New("ruta no válida")
	}
	return nil
}

func revealInExplorer(path string) error {
	if err := explorerPath(path); err != nil {
		return err
	}
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + path + `"`}
	return cmd.Start()
}

// shellOpen abre el archivo con su programa. En modo turbo (administrador) se pide
// al Explorador, que lo abre sin permisos de administrador: si no, un .exe o un
// script del disco se ejecutaría elevado sin el aviso de Windows.
func shellOpen(path string) error {
	if isAdmin() {
		if err := explorerPath(path); err != nil {
			return err
		}
		cmd := exec.Command("explorer.exe")
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe "` + path + `"`}
		return cmd.Start()
	}
	return windows.ShellExecute(0, ptr("open"), ptr(path), nil, nil, windows.SW_SHOWNORMAL)
}

// relaunchAsAdmin vuelve a abrir TurboCUT con permisos de administrador (sale el aviso de Windows).
func relaunchAsAdmin(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = syscall.EscapeArg(a)
	}
	return windows.ShellExecute(0, ptr("runas"), ptr(exe), ptr(strings.Join(quoted, " ")), nil, windows.SW_SHOWNORMAL)
}

func ptr(s string) *uint16 {
	p, _ := windows.UTF16PtrFromString(s)
	return p
}
