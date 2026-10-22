package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DownloadsDir es la carpeta "Descargas" del usuario (aunque la haya movido).
func DownloadsDir() string {
	if p, err := windows.KnownFolderPath(windows.FOLDERID_Downloads, 0); err == nil && p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Downloads")
}

// AppDir: %LOCALAPPDATA%\TurboDL (ejecutable instalado, manifiesto del host y log).
func AppDir() string {
	base, err := os.UserCacheDir() // %LOCALAPPDATA%
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "TurboDL")
}

func freeSpace(dir string) (uint64, bool) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, false
	}
	var free uint64
	if windows.GetDiskFreeSpaceEx(p, &free, nil, nil) != nil {
		return 0, false
	}
	return free, true
}

// setSparse marca (o desmarca) el archivo como disperso.
func setSparse(f *os.File, on bool) {
	const fsctlSetSparse = 0x000900c4
	var in byte
	if on {
		in = 1
	}
	var ret uint32
	windows.DeviceIoControl(windows.Handle(f.Fd()), fsctlSetSparse, &in, 1, nil, 0, &ret, nil)
}

func openPath(p string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	file, _ := windows.UTF16PtrFromString(p)
	dir, _ := windows.UTF16PtrFromString(filepath.Dir(p))
	return windows.ShellExecute(0, verb, file, nil, dir, windows.SW_SHOWNORMAL)
}

func showInFolder(p string) error {
	// explorer /select, "ruta" abre la carpeta con el archivo seleccionado.
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + p + `"`}
	return cmd.Start()
}

// messageBox para el instalador cuando se abre con doble clic.
func messageBox(title, text string, isError bool) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	proc := user32.NewProc("MessageBoxW")
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	flags := uintptr(0x40) // MB_ICONINFORMATION
	if isError {
		flags = 0x10 // MB_ICONERROR
	}
	flags |= 0x10000 // MB_SETFOREGROUND: que no quede detrás de TurboSETUP
	proc.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), flags)
}

// launchedByDoubleClick: true si el proceso tiene su propia consola recién creada
// (abierto desde el Explorador) en vez de venir de una terminal.
func launchedByDoubleClick() bool {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	proc := kernel32.NewProc("GetConsoleProcessList")
	var pids [4]uint32
	n, _, _ := proc.Call(uintptr(unsafe.Pointer(&pids[0])), 4)
	return n == 1
}
