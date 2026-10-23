package win11menu

// IObjectWithSite: en el fondo de una carpeta el Explorador puede no pasar
// IShellItemArray; entonces la carpeta se saca del sitio (la ventana del
// Explorador), como hace la Terminal de Windows. Todas estas interfaces tienen
// proxy en el sistema, así que funcionan entre procesos.

import (
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	iidIObjectWithSite  = guid("{FC4801A3-2BA9-11CF-A229-00AA003D7352}")
	iidIServiceProvider = guid("{6D5140C1-7436-11CE-8034-00AA006009FA}")
	iidIShellBrowser    = guid("{000214E2-0000-0000-C000-000000000046}")
	iidIFolderView      = guid("{CDE725B0-CCC9-4519-917E-325D72FAB4CE}")
	iidIShellItem       = guid("{43826D1E-E718-42EE-BC55-A1E261C37BFE}")
	sidSTopLevelBrowser = guid("{4C96BE40-915C-11CF-99D3-00AA004AE837}")
	siteVtbl            [5]uintptr
	siteMu              sync.Mutex
	lastSite            *iface // último sitio recibido (con AddRef)
)

func fromSite(p **[5]uintptr) *command {
	return (*command)(unsafe.Add(unsafe.Pointer(p), -int(unsafe.Offsetof(command{}.vSite))))
}

func initSiteVtbl() {
	siteVtbl = [5]uintptr{
		syscall.NewCallback(func(this **[5]uintptr, riid *windows.GUID, ppv *uintptr) uintptr {
			return cmdQI(fromSite(this), riid, ppv)
		}),
		syscall.NewCallback(func(this **[5]uintptr) uintptr { return uintptr(fromSite(this).ref.Add(1)) }),
		syscall.NewCallback(func(this **[5]uintptr) uintptr { return cmdRelease(fromSite(this)) }),
		// SetSite
		syscall.NewCallback(func(this **[5]uintptr, site *iface) uintptr {
			if site == nil {
				return sOK
			}
			site.call(1) // AddRef
			siteMu.Lock()
			old := lastSite
			lastSite = site
			siteMu.Unlock()
			if old != nil {
				old.release()
			}
			return sOK
		}),
		// GetSite
		syscall.NewCallback(func(this **[5]uintptr, riid *windows.GUID, ppv *uintptr) uintptr {
			if ppv == nil {
				return ePointer
			}
			*ppv = 0
			siteMu.Lock()
			s := lastSite
			siteMu.Unlock()
			if s == nil {
				return eFail
			}
			return s.call(0, uintptr(unsafe.Pointer(riid)), uintptr(unsafe.Pointer(ppv)))
		}),
	}
}

// siteSelection: la carpeta de la ventana del Explorador que pidió el menú.
func (s *server) siteSelection() Selection {
	sel := Selection{Background: true}
	if p := folderFromSite(); p != "" {
		sel.Paths, sel.Dirs = []string{p}, []bool{true}
	}
	s.logf("carpeta por el sitio: %q", sel.Paths)
	return sel
}

func folderFromSite() string {
	siteMu.Lock()
	site := lastSite
	if site != nil {
		site.call(1)
	}
	siteMu.Unlock()
	if site == nil {
		return ""
	}
	defer site.release()
	var sp *iface
	if site.call(0, uintptr(unsafe.Pointer(&iidIServiceProvider)), uintptr(unsafe.Pointer(&sp))) != 0 || sp == nil {
		return ""
	}
	defer sp.release()
	var fv *iface
	// Primero el IFolderView directo; si no, navegador -> vista activa -> IFolderView.
	if sp.call(3, uintptr(unsafe.Pointer(&iidIFolderView)), uintptr(unsafe.Pointer(&iidIFolderView)), uintptr(unsafe.Pointer(&fv))) != 0 || fv == nil {
		var sb, view *iface
		if sp.call(3, uintptr(unsafe.Pointer(&sidSTopLevelBrowser)), uintptr(unsafe.Pointer(&iidIShellBrowser)), uintptr(unsafe.Pointer(&sb))) != 0 || sb == nil {
			return ""
		}
		defer sb.release()
		if sb.call(15, uintptr(unsafe.Pointer(&view))) != 0 || view == nil { // QueryActiveShellView
			return ""
		}
		defer view.release()
		if view.call(0, uintptr(unsafe.Pointer(&iidIFolderView)), uintptr(unsafe.Pointer(&fv))) != 0 || fv == nil {
			return ""
		}
	}
	defer fv.release()
	var item *iface
	if fv.call(5, uintptr(unsafe.Pointer(&iidIShellItem)), uintptr(unsafe.Pointer(&item))) != 0 || item == nil { // GetFolder
		return ""
	}
	defer item.release()
	var p *uint16
	if item.call(5, sigdnFileSysPath, uintptr(unsafe.Pointer(&p))) != 0 || p == nil {
		return ""
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(p))
	return windows.UTF16PtrToString(p)
}
