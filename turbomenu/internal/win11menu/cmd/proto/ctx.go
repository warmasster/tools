package main

// Cliente de prueba: pide al shell el IContextMenu de unos archivos (o del fondo
// de una carpeta), lista el menú con submenús y, si se pide, invoca una entrada.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type comPtr struct{ vtbl *[32]uintptr }

func (p *comPtr) call(i int, args ...uintptr) uintptr {
	a := append([]uintptr{uintptr(unsafe.Pointer(p))}, args...)
	r, _, _ := syscall.SyscallN(p.vtbl[i], a...)
	return r
}

var (
	shell32             = windows.NewLazySystemDLL("shell32.dll")
	pSHParseDisplayName = shell32.NewProc("SHParseDisplayName")
	pSHBindToParent     = shell32.NewProc("SHBindToParent")
	pSHBindToObject     = shell32.NewProc("SHBindToObject")
	user32              = windows.NewLazySystemDLL("user32.dll")
	pCreatePopupMenu    = user32.NewProc("CreatePopupMenu")
	pGetMenuItemCount   = user32.NewProc("GetMenuItemCount")
	pGetMenuItemInfoW   = user32.NewProc("GetMenuItemInfoW")
	pDestroyMenu        = user32.NewProc("DestroyMenu")
	iidIContextMenu     = mustGUID("{000214E4-0000-0000-C000-000000000046}")
	iidIContextMenu2    = mustGUID("{000214F4-0000-0000-C000-000000000046}")
	iidIShellFolder     = mustGUID("{000214E6-0000-0000-C000-000000000046}")
)

func mustGUID(s string) windows.GUID { g, _ := windows.GUIDFromString(s); return g }

type menuItemInfo struct {
	cbSize, fMask, fType, fState, wID                uint32
	hSubMenu, hbmpChecked, hbmpUnchecked, dwItemData uintptr
	dwTypeData                                       *uint16
	cch                                              uint32
	hbmpItem                                         uintptr
}

type cmInvoke struct {
	cbSize, fMask uint32
	hwnd          uintptr
	lpVerb        uintptr
	lpParameters  uintptr
	lpDirectory   uintptr
	nShow         int32
	dwHotKey      uint32
	hIcon         uintptr
}

func parse(p string) uintptr {
	u, _ := windows.UTF16PtrFromString(p)
	var pidl uintptr
	pSHParseDisplayName.Call(uintptr(unsafe.Pointer(u)), 0, uintptr(unsafe.Pointer(&pidl)), 0, 0)
	return pidl
}

// ctx: proto ctx [-bg] [-invoke Título] rutas...
func ctx(args []string) error {
	runtime.LockOSThread()
	windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
	bg, invoke := false, ""
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-bg":
			bg = true
		case "-invoke":
			invoke = args[1]
			args = args[1:]
		}
		args = args[1:]
	}
	t0 := time.Now()
	var cm *comPtr
	if bg {
		var sf *comPtr
		r, _, _ := pSHBindToObject.Call(0, parse(args[0]), 0, uintptr(unsafe.Pointer(&iidIShellFolder)), uintptr(unsafe.Pointer(&sf)))
		if r != 0 {
			return fmt.Errorf("SHBindToObject 0x%X", uint32(r))
		}
		if r := sf.call(8, 0, uintptr(unsafe.Pointer(&iidIContextMenu)), uintptr(unsafe.Pointer(&cm))); r != 0 {
			return fmt.Errorf("CreateViewObject 0x%X", uint32(r))
		}
	} else {
		var sf *comPtr
		var child uintptr
		r, _, _ := pSHBindToParent.Call(parse(args[0]), uintptr(unsafe.Pointer(&iidIShellFolder)), uintptr(unsafe.Pointer(&sf)), uintptr(unsafe.Pointer(&child)))
		if r != 0 {
			return fmt.Errorf("SHBindToParent 0x%X", uint32(r))
		}
		kids := []uintptr{child}
		for _, a := range args[1:] {
			var sf2 *comPtr
			var c uintptr
			pSHBindToParent.Call(parse(a), uintptr(unsafe.Pointer(&iidIShellFolder)), uintptr(unsafe.Pointer(&sf2)), uintptr(unsafe.Pointer(&c)))
			kids = append(kids, c)
		}
		if r := sf.call(10, 0, uintptr(len(kids)), uintptr(unsafe.Pointer(&kids[0])), uintptr(unsafe.Pointer(&iidIContextMenu)), 0, uintptr(unsafe.Pointer(&cm))); r != 0 {
			return fmt.Errorf("GetUIObjectOf 0x%X", uint32(r))
		}
	}
	hm, _, _ := pCreatePopupMenu.Call()
	t1 := time.Now()
	if r := cm.call(3, hm, 0, 1, 0x7FFF, 0); int32(r) < 0 {
		return fmt.Errorf("QueryContextMenu 0x%X", uint32(r))
	}
	fmt.Printf("QueryContextMenu: %v (total %v)\n", time.Since(t1), time.Since(t0))
	var cm2 *comPtr
	cm.call(0, uintptr(unsafe.Pointer(&iidIContextMenu2)), uintptr(unsafe.Pointer(&cm2)))
	var found uint32
	var walk func(h uintptr, depth int)
	walk = func(h uintptr, depth int) {
		n, _, _ := pGetMenuItemCount.Call(h)
		for i := 0; i < int(int32(n)); i++ {
			buf := make([]uint16, 256)
			mi := menuItemInfo{fMask: 0x1 | 0x2 | 0x4 | 0x40 | 0x100, dwTypeData: &buf[0], cch: 255}
			mi.cbSize = uint32(unsafe.Sizeof(mi))
			pGetMenuItemInfoW.Call(h, uintptr(i), 1, uintptr(unsafe.Pointer(&mi)))
			name := windows.UTF16ToString(buf)
			if mi.fType&0x800 != 0 {
				name = "----"
			}
			state := ""
			if mi.fState&0x3 != 0 {
				state = " [desactivado]"
			}
			fmt.Printf("%s%s (id %d)%s\n", strings.Repeat("  ", depth+1), name, mi.wID, state)
			if invoke != "" && strings.EqualFold(strings.ReplaceAll(name, "&", ""), invoke) {
				found = mi.wID
			}
			if mi.hSubMenu != 0 {
				if cm2 != nil { // WM_INITMENUPOPUP para que se rellene
					t := time.Now()
					cm2.call(6, 0x0117, mi.hSubMenu, uintptr(i))
					if d := time.Since(t); d > time.Millisecond {
						fmt.Printf("%s(relleno del submenú: %v)\n", strings.Repeat("  ", depth+2), d)
					}
				}
				walk(mi.hSubMenu, depth+1)
			}
		}
	}
	walk(hm, 0)
	if invoke != "" {
		if found == 0 {
			return fmt.Errorf("no encuentro %q", invoke)
		}
		ci := cmInvoke{lpVerb: uintptr(found - 1), nShow: 1}
		ci.cbSize = uint32(unsafe.Sizeof(ci))
		dir, _ := windows.UTF16PtrFromString(filepath.Dir(args[0]))
		ci.lpDirectory = uintptr(unsafe.Pointer(dir))
		t := time.Now()
		r := cm.call(4, uintptr(unsafe.Pointer(&ci)))
		fmt.Printf("InvokeCommand(%q) = 0x%X en %v\n", invoke, uint32(r), time.Since(t))
	}
	pDestroyMenu.Call(hm)
	return nil
}

// cocreate: activa la clase como cliente COM normal y llama a GetTitle.
func cocreate() error {
	runtime.LockOSThread()
	windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
	clsid := mustGUID("{321E46AB-2041-4428-9D9F-CF84AEA715F0}")
	iid := mustGUID("{A08CE4D0-FA25-44AB-B57C-C7B1C323E0B9}")
	if len(os.Args) > 2 {
		clsid = mustGUID(os.Args[2])
		iid = mustGUID("{00000000-0000-0000-C000-000000000046}")
	}
	for _, ctxFlag := range []uintptr{0x4, 0x17} {
		var p *comPtr
		t := time.Now()
		r, _, _ := windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance").Call(
			uintptr(unsafe.Pointer(&clsid)), 0, ctxFlag, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&p)))
		fmt.Printf("CoCreateInstance(ctx=0x%X) = 0x%08X en %v\n", ctxFlag, uint32(r), time.Since(t))
		if r == 0 && len(os.Args) > 2 {
			p.call(2)
			break
		}
		if r == 0 {
			var s *uint16
			t = time.Now()
			r = p.call(3, 0, uintptr(unsafe.Pointer(&s)))
			fmt.Printf("GetTitle = 0x%X %q en %v\n", uint32(r), windows.UTF16PtrToString(s), time.Since(t))
			p.call(2)
		}
	}
	return nil
}

// excmd: hace de Explorador contra el servidor COM (IExplorerCommand fuera de
// proceso) con un IShellItemArray real.  proto excmd [-bg] [-invoke ID] rutas...
func excmd(args []string) error {
	runtime.LockOSThread()
	windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
	clsid := mustGUID("{321E46AB-2041-4428-9D9F-CF84AEA715F0}")
	invoke := ""
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-bg":
			clsid = mustGUID("{2454DDF6-7D73-4DCE-8D17-0E01611E9110}")
		case "-invoke":
			invoke = args[1]
			args = args[1:]
		}
		args = args[1:]
	}
	var paths []string
	for _, a := range args {
		if d, ok := strings.CutPrefix(a, "@"); ok { // @carpeta = todo lo que contiene
			es, _ := os.ReadDir(d)
			for _, e := range es {
				paths = append(paths, filepath.Join(d, e.Name()))
			}
		} else {
			paths = append(paths, a)
		}
	}
	pidls := make([]uintptr, len(paths))
	for i, a := range paths {
		pidls[i] = parse(a)
	}
	var arr *comPtr
	r, _, _ := shell32.NewProc("SHCreateShellItemArrayFromIDLists").Call(uintptr(len(pidls)), uintptr(unsafe.Pointer(&pidls[0])), uintptr(unsafe.Pointer(&arr)))
	if r != 0 {
		return fmt.Errorf("SHCreateShellItemArrayFromIDLists 0x%X", uint32(r))
	}
	iid := mustGUID("{A08CE4D0-FA25-44AB-B57C-C7B1C323E0B9}")
	t0 := time.Now()
	var root *comPtr
	r, _, _ = windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance").Call(
		uintptr(unsafe.Pointer(&clsid)), 0, 0x4, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&root)))
	if r != 0 {
		return fmt.Errorf("CoCreateInstance 0x%X", uint32(r))
	}
	tCreate := time.Since(t0)
	a := uintptr(unsafe.Pointer(arr))
	str := func(c *comPtr, m int) string {
		var s *uint16
		if c.call(m, a, uintptr(unsafe.Pointer(&s))) != 0 || s == nil {
			return ""
		}
		defer windows.CoTaskMemFree(unsafe.Pointer(s))
		return windows.UTF16PtrToString(s)
	}
	var target *comPtr
	var walk func(c *comPtr, depth int)
	walk = func(c *comPtr, depth int) {
		var st, fl uint32
		c.call(7, a, 0, uintptr(unsafe.Pointer(&st)))
		c.call(9, uintptr(unsafe.Pointer(&fl)))
		title := str(c, 3)
		icon := str(c, 4)
		var g windows.GUID
		c.call(6, uintptr(unsafe.Pointer(&g)))
		if st&2 != 0 {
			fmt.Printf("%s(oculto) %s\n", strings.Repeat("  ", depth), title)
			return
		}
		extra := ""
		if st&1 != 0 {
			extra = " [desactivado]"
		}
		if fl&0x20 != 0 {
			fmt.Printf("%s----\n", strings.Repeat("  ", depth))
		}
		fmt.Printf("%s%s%s  icono=%q\n", strings.Repeat("  ", depth), title, extra, icon)
		if fl&1 != 0 {
			var e *comPtr
			if c.call(10, uintptr(unsafe.Pointer(&e))) == 0 && e != nil {
				for {
					var sub *comPtr
					var got uint32
					if e.call(3, 1, uintptr(unsafe.Pointer(&sub)), uintptr(unsafe.Pointer(&got))) != 0 || got == 0 {
						break
					}
					walk(sub, depth+1)
					if target == nil && invoke != "" && guidStr(sub) == invoke {
						target = sub
						continue
					}
					sub.call(2)
				}
				e.call(2)
			}
		}
	}
	walk(root, 0)
	fmt.Printf("CoCreateInstance %v; menú completo %v\n", tCreate, time.Since(t0))
	if invoke != "" {
		if target == nil {
			return fmt.Errorf("no encuentro %q", invoke)
		}
		t := time.Now()
		r := target.call(8, a, 0)
		fmt.Printf("Invoke = 0x%X en %v\n", uint32(r), time.Since(t))
		target.call(2)
	}
	root.call(2)
	return nil
}

// guidStr: GetCanonicalName como texto, para elegir comando por ID.
func guidStr(c *comPtr) string {
	var s *uint16
	if c.call(3, 0, uintptr(unsafe.Pointer(&s))) != 0 || s == nil {
		return ""
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(s))
	return windows.UTF16PtrToString(s)
}
