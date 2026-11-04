package sensors

// Contadores de rendimiento de Windows (PDH), los mismos que usa el
// Administrador de tareas. Una consulta junta varios contadores y se lee de una
// vez; los de «por segundo» necesitan dos lecturas para tener valor.

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modPdh         = windows.NewLazySystemDLL("pdh.dll")
	pOpenQuery     = modPdh.NewProc("PdhOpenQueryW")
	pAddCounter    = modPdh.NewProc("PdhAddEnglishCounterW")
	pRemoveCounter = modPdh.NewProc("PdhRemoveCounter")
	pCollect       = modPdh.NewProc("PdhCollectQueryData")
	pGetValue      = modPdh.NewProc("PdhGetFormattedCounterValue")
	pGetArray      = modPdh.NewProc("PdhGetFormattedCounterArrayW")
	pCloseQuery    = modPdh.NewProc("PdhCloseQuery")
	pdhAvailable   = modPdh.Load() == nil
)

const (
	pdhFmtDouble = 0x00000200
	pdhFmtNoCap  = 0x00008000
	pdhMoreData  = 0x800007D2
)

type pdhQuery struct{ h uintptr }

type pdhCounter struct {
	h   uintptr
	buf []byte
}

type pdhItem struct {
	Name  string
	Value float64
}

type fmtValue struct {
	CStatus uint32
	_       uint32
	Value   float64
}

type fmtItem struct {
	Name  *uint16
	Value fmtValue
}

func newQuery() (*pdhQuery, error) {
	if !pdhAvailable {
		return nil, syscall.ERROR_PROC_NOT_FOUND
	}
	var h uintptr
	r, _, _ := pOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&h)))
	if r != 0 {
		return nil, syscall.Errno(r)
	}
	return &pdhQuery{h: h}, nil
}

// add añade un contador por su nombre en inglés (vale en cualquier idioma de
// Windows). Devuelve nil si no existe en este equipo.
func (q *pdhQuery) add(path string) *pdhCounter {
	if q == nil {
		return nil
	}
	p, _ := windows.UTF16PtrFromString(path)
	var h uintptr
	r, _, _ := pAddCounter.Call(q.h, uintptr(unsafe.Pointer(p)), 0, uintptr(unsafe.Pointer(&h)))
	if r != 0 {
		return nil
	}
	return &pdhCounter{h: h}
}

func (q *pdhQuery) remove(c *pdhCounter) {
	if q != nil && c != nil {
		pRemoveCounter.Call(c.h)
	}
}

func (q *pdhQuery) collect() {
	if q != nil {
		pCollect.Call(q.h)
	}
}

func (q *pdhQuery) close() {
	if q != nil && q.h != 0 {
		pCloseQuery.Call(q.h)
		q.h = 0
	}
}

func (c *pdhCounter) value() (float64, bool) {
	if c == nil {
		return 0, false
	}
	var v fmtValue
	r, _, _ := pGetValue.Call(c.h, pdhFmtDouble|pdhFmtNoCap, 0, uintptr(unsafe.Pointer(&v)))
	if r != 0 || v.CStatus > 1 {
		return 0, false
	}
	return v.Value, true
}

// array lee un contador con comodín (p. ej. «\GPU Engine(*)\…»): un valor por instancia.
func (c *pdhCounter) array() []pdhItem {
	if c == nil {
		return nil
	}
	for try := 0; try < 3; try++ {
		size, count := uint32(len(c.buf)), uint32(0)
		var p uintptr
		if size > 0 {
			p = uintptr(unsafe.Pointer(&c.buf[0]))
		}
		r, _, _ := pGetArray.Call(c.h, pdhFmtDouble|pdhFmtNoCap, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), p)
		if r == pdhMoreData {
			c.buf = make([]byte, size+4096)
			continue
		}
		if r != 0 || count == 0 {
			return nil
		}
		items := unsafe.Slice((*fmtItem)(unsafe.Pointer(&c.buf[0])), count)
		out := make([]pdhItem, 0, count)
		for _, it := range items {
			if it.Value.CStatus > 1 {
				continue
			}
			out = append(out, pdhItem{Name: windows.UTF16PtrToString(it.Name), Value: it.Value.Value})
		}
		return out
	}
	return nil
}
