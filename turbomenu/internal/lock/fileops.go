//go:build windows

package lock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ErrNoRecycleBin: la unidad (USB, red…) no tiene papelera y se borraría
// para siempre.
var ErrNoRecycleBin = errors.New("Esta unidad no tiene papelera: se borraría para siempre.")

// SHFILEOPSTRUCTW (alineación natural en x64).
type shFileOp struct {
	hwnd    uintptr
	wFunc   uint32
	pFrom   *uint16
	pTo     *uint16
	fFlags  uint16
	aborted int32
	nameMap uintptr
	title   *uint16
}

const (
	foMove   = 1
	foDelete = 3

	fofSilent           = 0x0004
	fofRenameOnCollison = 0x0008
	fofNoConfirmation   = 0x0010
	fofAllowUndo        = 0x0040
	fofNoConfirmMkdir   = 0x0200
	fofNoErrorUI        = 0x0400
	fofWantNukeWarning  = 0x4000
)

// Recycle manda archivos o carpetas a la papelera sin preguntar. Si alguno
// está en una unidad sin papelera devuelve ErrNoRecycleBin y no toca nada.
func Recycle(paths []string) error {
	abs, err := absPaths(paths)
	if err != nil {
		return err
	}
	for _, p := range abs {
		root, _ := windows.UTF16PtrFromString(filepath.VolumeName(p) + `\`)
		switch windows.GetDriveType(root) {
		case windows.DRIVE_REMOVABLE, windows.DRIVE_REMOTE, windows.DRIVE_CDROM:
			return ErrNoRecycleBin
		}
	}
	// Si algo no cupiera en la papelera, Windows avisa antes de borrarlo para siempre.
	return shOp(foDelete, abs, "", fofAllowUndo|fofNoConfirmation|fofSilent|fofNoErrorUI|fofWantNukeWarning)
}

// Move mueve archivos o carpetas a dstDir (la crea si hace falta). Si ya
// existe uno con el mismo nombre, el nuevo se renombra («archivo (2)»).
func Move(paths []string, dstDir string) error {
	abs, err := absPaths(paths)
	if err != nil {
		return err
	}
	dst, err := filepath.Abs(dstDir)
	if err != nil || dstDir == "" {
		return errors.New("Carpeta de destino no válida.")
	}
	// Debe existir: si no, SHFileOperation tomaría la ruta como nombre nuevo.
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return errors.New("No se puede crear la carpeta de destino.")
	}
	return shOp(foMove, abs, dst, fofAllowUndo|fofNoConfirmMkdir|fofSilent|fofNoErrorUI|fofRenameOnCollison)
}

func absPaths(paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		a, err := filepath.Abs(p)
		if err != nil {
			return nil, errors.New("Ruta no válida.")
		}
		out = append(out, a)
	}
	if len(out) == 0 {
		return nil, errors.New("No hay nada que hacer.")
	}
	return out, nil
}

// doubleNull arma la lista «a\0b\0\0» que pide SHFileOperation.
func doubleNull(list []string) []uint16 {
	var buf []uint16
	for _, s := range list {
		u, _ := syscall.UTF16FromString(s)
		buf = append(buf, u...) // ya incluye su \0
	}
	return append(buf, 0)
}

func shOp(fn uint32, from []string, to string, flags uint16) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err == nil {
		defer windows.CoUninitialize()
	}
	src := doubleNull(from)
	op := shFileOp{wFunc: fn, pFrom: &src[0], fFlags: flags}
	if to != "" {
		dst := doubleNull([]string{to})
		op.pTo = &dst[0]
	}
	r, _, _ := syscall.SyscallN(procSHFileOperationW.Addr(), uintptr(unsafe.Pointer(&op)))
	runtime.KeepAlive(src)
	return shError(r, op.aborted != 0)
}

// shError traduce los códigos (antiguos) de SHFileOperation.
func shError(code uintptr, aborted bool) error {
	switch code {
	case 0:
		if aborted {
			return errors.New("Se ha cancelado.")
		}
		return nil
	case 0x75, 1223: // DE_OPCANCELLED, ERROR_CANCELLED
		return errors.New("Se ha cancelado.")
	case 32, 33: // ERROR_SHARING_VIOLATION, ERROR_LOCK_VIOLATION
		return errors.New("Algún archivo sigue en uso.")
	case 5, 0x78, 0x10000 | 5: // acceso denegado
		return errors.New("No hay permiso para hacerlo.")
	case 2, 3, 0x7C, 0x402:
		return errors.New("No se encuentra el archivo.")
	case 0x71: // DE_SAMEFILE
		return errors.New("El origen y el destino son el mismo.")
	case 0x76: // DE_DESTSUBTREE
		return errors.New("No se puede mover una carpeta dentro de sí misma.")
	case 0x79, 206: // DE_PATHTOODEEP, ERROR_FILENAME_EXCED_RANGE
		return errors.New("La ruta es demasiado larga.")
	case 112, 39: // sin espacio
		return errors.New("No queda espacio en el disco.")
	case 0x10000: // DE_ERRORONDEST
		return errors.New("No se puede escribir en la carpeta de destino.")
	}
	return fmt.Errorf("No se ha podido completar (código %d).", code)
}
