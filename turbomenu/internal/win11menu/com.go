package win11menu

// Objetos COM hechos a mano: vtables con syscall.NewCallback y punteros a
// structs de Go que se mantienen vivos en un mapa mientras COM los usa.

import (
	"crypto/sha1"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	sOK            = 0
	sFalse         = 1
	eNotImpl       = 0x80004001
	eNoInterface   = 0x80004002
	ePointer       = 0x80004003
	eFail          = 0x80004005
	classENoAggreg = 0x80040110

	ecfHasSubcommands  = 0x1
	ecfSeparatorBefore = 0x20
	ecsEnabled         = 0
	ecsDisabled        = 1
	ecsHidden          = 2
	sigdnFileSysPath   = 0x80058000
	clsctxLocalServer  = 0x4
	regclsMultipleUse  = 1
)

var (
	iidIUnknown             = guid("{00000000-0000-0000-C000-000000000046}")
	iidIClassFactory        = guid("{00000001-0000-0000-C000-000000000046}")
	iidIExplorerCommand     = guid("{A08CE4D0-FA25-44AB-B57C-C7B1C323E0B9}")
	iidIEnumExplorerCommand = guid("{A88826F8-186F-4987-AADE-EA0CEF8FBFE8}")

	modOle32               = windows.NewLazySystemDLL("ole32.dll")
	procCoRegisterClassObj = modOle32.NewProc("CoRegisterClassObject")
	procCoRevokeClassObj   = modOle32.NewProc("CoRevokeClassObject")
	procSHStrDupW          = windows.NewLazySystemDLL("shlwapi.dll").NewProc("SHStrDupW")
	procClientPID          = windows.NewLazySystemDLL("rpcrt4.dll").NewProc("I_RpcBindingInqLocalClientPID")
)

func guid(s string) windows.GUID {
	g, err := windows.GUIDFromString(s)
	if err != nil {
		panic(err)
	}
	return g
}

// iface es un puntero a interfaz COM ajena (p. ej. IShellItemArray).
type iface struct{ vtbl *[16]uintptr }

func (p *iface) call(i int, args ...uintptr) uintptr {
	a := append([]uintptr{uintptr(unsafe.Pointer(p))}, args...)
	r, _, _ := syscall.SyscallN(p.vtbl[i], a...)
	return r
}

func (p *iface) release() { p.call(2) }

// Objetos vivos: COM solo guarda el puntero, así que el GC no debe soltarlos.
var live sync.Map

var liveCount atomic.Int32

func keep(p any)   { live.Store(p, struct{}{}); liveCount.Add(1) }
func unkeep(p any) { live.Delete(p); liveCount.Add(-1) }

func strOut(s string, out **uint16) uintptr {
	if out == nil {
		return ePointer
	}
	u, _ := windows.UTF16PtrFromString(s)
	r, _, _ := procSHStrDupW.Call(uintptr(unsafe.Pointer(u)), uintptr(unsafe.Pointer(out)))
	return r
}

// guidFor da un GUID estable para cada ID de comando.
func guidFor(id string) windows.GUID {
	h := sha1.Sum([]byte("turbomenu:" + id))
	var g windows.GUID
	g.Data1 = uint32(h[0])<<24 | uint32(h[1])<<16 | uint32(h[2])<<8 | uint32(h[3])
	g.Data2 = uint16(h[4])<<8 | uint16(h[5])
	g.Data3 = uint16(h[6])<<8&0x0fff | 0x5000
	copy(g.Data4[:], h[8:16])
	g.Data4[0] = g.Data4[0]&0x3f | 0x80
	return g
}

// clientPID: proceso que hace la llamada COM actual (para el log).
func clientPID() uint32 {
	var pid uint32
	if procClientPID.Find() != nil {
		return 0
	}
	if r, _, _ := procClientPID.Call(0, uintptr(unsafe.Pointer(&pid))); r != 0 {
		return 0
	}
	return pid
}

// ---- IClassFactory ----

type factory struct {
	vtbl *[5]uintptr
	bg   bool
	exec bool // CLSIDExecute
}

var (
	factoryVtbl [5]uintptr
	commandVtbl [11]uintptr
	enumVtbl    [7]uintptr
	vtblOnce    sync.Once
)

// initVtables crea las callbacks una sola vez (solo si se sirve COM).
func initVtables() {
	vtblOnce.Do(func() {
		initFactoryVtbl()
		initCommandVtbl()
		initEnumVtbl()
		initExecVtbls()
		initSiteVtbl()
	})
}

func initFactoryVtbl() {
	factoryVtbl = [5]uintptr{
		syscall.NewCallback(func(this *factory, riid *windows.GUID, ppv *uintptr) uintptr {
			if ppv == nil {
				return ePointer
			}
			if *riid == iidIUnknown || *riid == iidIClassFactory {
				*ppv = uintptr(unsafe.Pointer(this))
				return sOK
			}
			*ppv = 0
			return eNoInterface
		}),
		syscall.NewCallback(func(this *factory) uintptr { return 2 }),
		syscall.NewCallback(func(this *factory) uintptr { return 1 }),
		syscall.NewCallback(func(this *factory, outer uintptr, riid *windows.GUID, ppv *uintptr) uintptr {
			if ppv == nil {
				return ePointer
			}
			*ppv = 0
			if outer != 0 {
				return classENoAggreg
			}
			srv.touch()
			srv.logf("CreateInstance bg=%v exec=%v iid=%s", this.bg, this.exec, riid.String())
			if this.exec {
				e := newExecCmd()
				r := execQI(e, riid, ppv)
				execRelease(e)
				return r
			}
			c := newCommand(nil, this.bg)
			r := cmdQI(c, riid, ppv)
			cmdRelease(c)
			return r
		}),
		syscall.NewCallback(func(this *factory, lock uintptr) uintptr {
			if lock != 0 {
				srv.locks.Add(1)
			} else {
				srv.locks.Add(-1)
			}
			srv.touch()
			return sOK
		}),
	}
}

// ---- IExplorerCommand ----

type command struct {
	vtbl  *[11]uintptr // IExplorerCommand (primero: identidad IUnknown)
	vSite *[5]uintptr  // IObjectWithSite
	ref   atomic.Int32
	cmd   *Command // nil = raíz
	bg    bool
}

func newCommand(c *Command, bg bool) *command {
	o := &command{vtbl: &commandVtbl, vSite: &siteVtbl, cmd: c, bg: bg}
	o.ref.Store(1)
	keep(o)
	return o
}

func (o *command) name() string {
	if o.cmd == nil {
		return "raíz"
	}
	return o.cmd.ID
}

func cmdQI(this *command, riid *windows.GUID, ppv *uintptr) uintptr {
	if ppv == nil {
		return ePointer
	}
	switch *riid {
	case iidIUnknown, iidIExplorerCommand:
		*ppv = uintptr(unsafe.Pointer(this))
	case iidIObjectWithSite:
		*ppv = uintptr(unsafe.Pointer(&this.vSite))
	default:
		*ppv = 0
		return eNoInterface
	}
	this.ref.Add(1)
	return sOK
}

func cmdRelease(this *command) uintptr {
	n := this.ref.Add(-1)
	if n == 0 {
		unkeep(this)
		srv.touch()
	}
	return uintptr(n)
}

// info devuelve título, icono, etc. del comando (la raíz usa los del Menu).
func (o *command) title(s Selection) string {
	if o.cmd == nil {
		return srv.menu.Title
	}
	t := o.cmd.Title
	if o.cmd.Disabled != nil {
		if why := o.cmd.Disabled(s); why != "" {
			t += " (" + why + ")"
		}
	}
	return t
}

func (o *command) icon() string {
	if o.cmd == nil {
		return srv.menu.Icon
	}
	return o.cmd.Icon
}

func (o *command) subs() []Command {
	if o.cmd == nil {
		return srv.menu.Commands
	}
	return o.cmd.Sub
}

func initCommandVtbl() {
	commandVtbl = [11]uintptr{
		syscall.NewCallback(cmdQI),
		syscall.NewCallback(func(this *command) uintptr { return uintptr(this.ref.Add(1)) }),
		syscall.NewCallback(cmdRelease),
		// GetTitle
		syscall.NewCallback(func(this *command, items *iface, out **uint16) uintptr {
			s := srv.selection(items, this.bg)
			t := this.title(s)
			srv.logf("GetTitle %s -> %q", this.name(), t)
			return strOut(t, out)
		}),
		// GetIcon
		syscall.NewCallback(func(this *command, items *iface, out **uint16) uintptr {
			ic := this.icon()
			if ic == "" {
				if out != nil {
					*out = nil
				}
				return eNotImpl
			}
			return strOut(ic, out)
		}),
		// GetToolTip
		syscall.NewCallback(func(this *command, items *iface, out **uint16) uintptr {
			if this.cmd == nil || this.cmd.Tooltip == "" {
				if out != nil {
					*out = nil
				}
				return eNotImpl
			}
			return strOut(this.cmd.Tooltip, out)
		}),
		// GetCanonicalName
		syscall.NewCallback(func(this *command, g *windows.GUID) uintptr {
			if g == nil {
				return ePointer
			}
			*g = guidFor(this.name())
			return sOK
		}),
		// GetState
		syscall.NewCallback(func(this *command, items *iface, slow uintptr, state *uint32) uintptr {
			if state == nil {
				return ePointer
			}
			srv.touch()
			s := srv.selection(items, this.bg)
			st := uint32(ecsEnabled)
			switch {
			case s.NonFS || len(s.Paths) == 0:
				st = ecsHidden
			case this.cmd == nil:
				if !anyVisible(srv.menu.Commands, s) {
					st = ecsHidden
				}
			case !this.cmd.visible(s):
				st = ecsHidden
			case this.cmd.Disabled != nil && this.cmd.Disabled(s) != "":
				st = ecsDisabled
			}
			srv.logf("GetState %s slow=%d n=%d bg=%v -> %d (cliente=%s)", this.name(), slow, len(s.Paths), s.Background, st, procName(clientPID()))
			*state = st
			return sOK
		}),
		// Invoke
		syscall.NewCallback(func(this *command, items *iface, bc uintptr) uintptr {
			srv.touch()
			s := srv.readSelection(items, this.bg)
			srv.logf("Invoke %s n=%d paths=%q cliente=%s", this.name(), len(s.Paths), s.Paths, procName(clientPID()))
			if this.cmd == nil || len(this.cmd.Sub) > 0 {
				return sOK
			}
			if this.cmd.Disabled != nil && this.cmd.Disabled(s) != "" {
				return sOK
			}
			srv.invoke(this.cmd.ID, s)
			return sOK
		}),
		// GetFlags
		syscall.NewCallback(func(this *command, flags *uint32) uintptr {
			if flags == nil {
				return ePointer
			}
			var f uint32
			if this.cmd == nil || len(this.cmd.Sub) > 0 {
				f |= ecfHasSubcommands
			}
			if this.cmd != nil && this.cmd.SeparatorBefore {
				f |= ecfSeparatorBefore
			}
			*flags = f
			return sOK
		}),
		// EnumSubCommands
		syscall.NewCallback(func(this *command, out *uintptr) uintptr {
			if out == nil {
				return ePointer
			}
			*out = 0
			subs := this.subs()
			if len(subs) == 0 {
				return eNotImpl
			}
			// Solo los visibles para la última selección consultada.
			s, ok := srv.lastSelection(this.bg)
			var list []*Command
			for i := range subs {
				if !ok || subs[i].visible(s) {
					list = append(list, &subs[i])
				}
			}
			srv.logf("EnumSubCommands %s -> %d", this.name(), len(list))
			e := &enumCmd{vtbl: &enumVtbl, items: list, bg: this.bg}
			e.ref.Store(1)
			keep(e)
			*out = uintptr(unsafe.Pointer(e))
			return sOK
		}),
	}
}

// ---- IEnumExplorerCommand ----

type enumCmd struct {
	vtbl  *[7]uintptr
	ref   atomic.Int32
	mu    sync.Mutex
	items []*Command
	pos   int
	bg    bool
}

func initEnumVtbl() {
	enumVtbl = [7]uintptr{
		syscall.NewCallback(func(this *enumCmd, riid *windows.GUID, ppv *uintptr) uintptr {
			if ppv == nil {
				return ePointer
			}
			if *riid == iidIUnknown || *riid == iidIEnumExplorerCommand {
				this.ref.Add(1)
				*ppv = uintptr(unsafe.Pointer(this))
				return sOK
			}
			*ppv = 0
			return eNoInterface
		}),
		syscall.NewCallback(func(this *enumCmd) uintptr { return uintptr(this.ref.Add(1)) }),
		syscall.NewCallback(func(this *enumCmd) uintptr {
			n := this.ref.Add(-1)
			if n == 0 {
				unkeep(this)
				srv.touch()
			}
			return uintptr(n)
		}),
		// Next
		syscall.NewCallback(func(this *enumCmd, celt uintptr, out *uintptr, fetched *uint32) uintptr {
			if out == nil || celt == 0 {
				return ePointer
			}
			this.mu.Lock()
			defer this.mu.Unlock()
			dst := unsafe.Slice(out, celt)
			n := 0
			for n < int(celt) && this.pos < len(this.items) {
				c := newCommand(this.items[this.pos], this.bg)
				dst[n] = uintptr(unsafe.Pointer(c))
				n++
				this.pos++
			}
			if fetched != nil {
				*fetched = uint32(n)
			}
			if n < int(celt) {
				return sFalse
			}
			return sOK
		}),
		// Skip
		syscall.NewCallback(func(this *enumCmd, celt uintptr) uintptr {
			this.mu.Lock()
			defer this.mu.Unlock()
			this.pos += int(celt)
			if this.pos > len(this.items) {
				this.pos = len(this.items)
				return sFalse
			}
			return sOK
		}),
		// Reset
		syscall.NewCallback(func(this *enumCmd) uintptr {
			this.mu.Lock()
			this.pos = 0
			this.mu.Unlock()
			return sOK
		}),
		// Clone
		syscall.NewCallback(func(this *enumCmd, out *uintptr) uintptr {
			if out == nil {
				return ePointer
			}
			this.mu.Lock()
			e := &enumCmd{vtbl: &enumVtbl, items: this.items, pos: this.pos, bg: this.bg}
			this.mu.Unlock()
			e.ref.Store(1)
			keep(e)
			*out = uintptr(unsafe.Pointer(e))
			return sOK
		}),
	}
}
