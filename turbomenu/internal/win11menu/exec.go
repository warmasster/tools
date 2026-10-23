package win11menu

// IExecuteCommand para el menú clásico: un verbo de registro con
// command\DelegateExecute = CLSIDExecute hace que el Explorador active este
// servidor (fuera de proceso, sí lo admite) con TODA la selección de una vez.
// El nombre de la clave del verbo llega por IInitializeCommand y es la acción.

import (
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// CLSIDExecute es el CLSID para DelegateExecute.
var CLSIDExecute = guid("{01069FDB-192C-4B49-9F1D-7AFBA9D06CF5}")

var (
	iidIExecuteCommand      = guid("{7F9185B0-CB92-43C5-80A9-92277A4F7B54}")
	iidIObjectWithSelection = guid("{1C9CD5BB-98E9-4491-A60F-31AACC72B83C}")
	iidIInitializeCommand   = guid("{85075ACF-231F-40EA-9610-D26B7B58F638}")
)

// execCmd es un objeto con tres interfaces: cada una es un puntero a su vtable
// dentro del mismo struct (herencia múltiple a mano).
type execCmd struct {
	vExec *[13]uintptr // IExecuteCommand (debe ir primero: es la identidad IUnknown)
	vSel  *[5]uintptr  // IObjectWithSelection
	vInit *[4]uintptr  // IInitializeCommand
	ref   atomic.Int32
	mu    sync.Mutex
	verb  string
	sel   Selection
}

var (
	execVtbl [13]uintptr
	selVtbl  [5]uintptr
	initVtbl [4]uintptr
)

func newExecCmd() *execCmd {
	o := &execCmd{vExec: &execVtbl, vSel: &selVtbl, vInit: &initVtbl}
	o.ref.Store(1)
	keep(o)
	return o
}

func execQI(o *execCmd, riid *windows.GUID, ppv *uintptr) uintptr {
	if ppv == nil {
		return ePointer
	}
	switch *riid {
	case iidIUnknown, iidIExecuteCommand:
		*ppv = uintptr(unsafe.Pointer(&o.vExec))
	case iidIObjectWithSelection:
		*ppv = uintptr(unsafe.Pointer(&o.vSel))
	case iidIInitializeCommand:
		*ppv = uintptr(unsafe.Pointer(&o.vInit))
	default:
		*ppv = 0
		return eNoInterface
	}
	o.ref.Add(1)
	return sOK
}

func execAddRef(o *execCmd) uintptr { return uintptr(o.ref.Add(1)) }

func execRelease(o *execCmd) uintptr {
	n := o.ref.Add(-1)
	if n == 0 {
		unkeep(o)
		srv.touch()
	}
	return uintptr(n)
}

// Los punteros a vSel/vInit apuntan dentro del struct: se recupera el objeto
// con el desplazamiento del campo.
func fromSel(p **[5]uintptr) *execCmd {
	return (*execCmd)(unsafe.Add(unsafe.Pointer(p), -int(unsafe.Offsetof(execCmd{}.vSel))))
}

func fromInit(p **[4]uintptr) *execCmd {
	return (*execCmd)(unsafe.Add(unsafe.Pointer(p), -int(unsafe.Offsetof(execCmd{}.vInit))))
}

func initExecVtbls() {
	ok := func(this *execCmd, _ uintptr) uintptr { return sOK }
	execVtbl = [13]uintptr{
		syscall.NewCallback(execQI),
		syscall.NewCallback(execAddRef),
		syscall.NewCallback(execRelease),
		syscall.NewCallback(ok), // SetKeyState
		syscall.NewCallback(ok), // SetParameters
		syscall.NewCallback(ok), // SetPosition (POINT por valor: 8 bytes)
		syscall.NewCallback(ok), // SetShowWindow
		syscall.NewCallback(ok), // SetNoShowUI
		syscall.NewCallback(ok), // SetDirectory
		// Execute
		syscall.NewCallback(func(this *execCmd) uintptr {
			srv.touch()
			this.mu.Lock()
			verb, sel := this.verb, this.sel
			this.mu.Unlock()
			srv.logf("Execute %s n=%d paths=%q cliente=%s", verb, len(sel.Paths), sel.Paths, procName(clientPID()))
			if verb != "" && len(sel.Paths) > 0 {
				srv.invoke(verb, sel)
			}
			return sOK
		}),
	}
	selVtbl = [5]uintptr{
		syscall.NewCallback(func(this **[5]uintptr, riid *windows.GUID, ppv *uintptr) uintptr {
			return execQI(fromSel(this), riid, ppv)
		}),
		syscall.NewCallback(func(this **[5]uintptr) uintptr { return execAddRef(fromSel(this)) }),
		syscall.NewCallback(func(this **[5]uintptr) uintptr { return execRelease(fromSel(this)) }),
		// SetSelection
		syscall.NewCallback(func(this **[5]uintptr, items *iface) uintptr {
			o := fromSel(this)
			s := srv.readSelection(items, false)
			o.mu.Lock()
			o.sel = s
			o.mu.Unlock()
			return sOK
		}),
		// GetSelection
		syscall.NewCallback(func(this **[5]uintptr, riid *windows.GUID, ppv *uintptr) uintptr {
			if ppv != nil {
				*ppv = 0
			}
			return eNotImpl
		}),
	}
	initVtbl = [4]uintptr{
		syscall.NewCallback(func(this **[4]uintptr, riid *windows.GUID, ppv *uintptr) uintptr {
			return execQI(fromInit(this), riid, ppv)
		}),
		syscall.NewCallback(func(this **[4]uintptr) uintptr { return execAddRef(fromInit(this)) }),
		syscall.NewCallback(func(this **[4]uintptr) uintptr { return execRelease(fromInit(this)) }),
		// Initialize(pszCommandName, IPropertyBag*)
		syscall.NewCallback(func(this **[4]uintptr, name *uint16, bag uintptr) uintptr {
			o := fromInit(this)
			o.mu.Lock()
			o.verb = windows.UTF16PtrToString(name)
			o.mu.Unlock()
			return sOK
		}),
	}
}
