package main

// Llamadas a Windows que no trae golang.org/x/sys.

import (
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	advapi32             = windows.NewLazySystemDLL("advapi32.dll")
	user32               = windows.NewLazySystemDLL("user32.dll")
	gdi32                = windows.NewLazySystemDLL("gdi32.dll")
	dwmapi               = windows.NewLazySystemDLL("dwmapi.dll")
	procRegDeleteKeyExW  = advapi32.NewProc("RegDeleteKeyExW")
	procDwmSetWindowAttr = dwmapi.NewProc("DwmSetWindowAttribute")
	procFindWindowW      = user32.NewProc("FindWindowW")
)

// deleteKeyEx borra una clave vacía en la vista de registro indicada (64 o 32 bits).
func deleteKeyEx(root registry.Key, path string, view uint32) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	r, _, _ := procRegDeleteKeyExW.Call(uintptr(root), uintptr(unsafe.Pointer(p)), uintptr(view), 0)
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

const createNoWindow = 0x08000000

func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

// roundCorners pide las esquinas redondeadas de Windows 11 para la ventana sin marco.
func roundCorners(hwnd uintptr) {
	if hwnd == 0 || procDwmSetWindowAttr.Find() != nil {
		return
	}
	pref := uint32(2) // DWMWCP_ROUND
	procDwmSetWindowAttr.Call(hwnd, 33, uintptr(unsafe.Pointer(&pref)), 4)
}

func findWindow(class string) uintptr {
	c, _ := windows.UTF16PtrFromString(class)
	h, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(c)), 0)
	return h
}
