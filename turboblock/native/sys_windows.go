package main

import (
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// lockRules coge un candado del sistema compartido por todos los TurboBLOCK.exe de
// la sesión (espera como mucho 30 s) y devuelve cómo soltarlo.
func lockRules() func() {
	// Un mutex de Windows es del hilo que lo coge: la goroutine se queda en ese hilo
	// hasta soltarlo (si no, ReleaseMutex fallaría desde otro hilo).
	runtime.LockOSThread()
	name, _ := windows.UTF16PtrFromString(`Local\TurboBLOCK-reglas`)
	h, err := windows.CreateMutex(nil, false, name)
	if err != nil && h == 0 {
		runtime.UnlockOSThread()
		return func() {}
	}
	ev, _ := windows.WaitForSingleObject(h, 30_000)
	if ev != windows.WAIT_OBJECT_0 && ev != windows.WAIT_ABANDONED {
		windows.CloseHandle(h)
		runtime.UnlockOSThread()
		return func() {}
	}
	return func() {
		windows.ReleaseMutex(h)
		windows.CloseHandle(h)
		runtime.UnlockOSThread()
	}
}

// AppDir: %LOCALAPPDATA%\TurboBLOCK (host instalado, extensión, listas y log).
func AppDir() string {
	base, err := os.UserCacheDir() // %LOCALAPPDATA%
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "TurboBLOCK")
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
