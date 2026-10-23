//go:build windows

package lock

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// openHandle es un handle de archivo abierto en otro proceso.
type openHandle struct {
	pid   uint32
	value uintptr
	nt    string // ruta NT (\Device\HarddiskVolumeN\...)
}

var (
	sysBufMu   sync.Mutex
	sysBufHint = 4 << 20

	fileTypeOnce  sync.Once
	fileTypeIndex uint16 // índice del tipo de objeto «File» en esta versión de Windows
)

// systemHandles devuelve todos los handles del sistema (copia propia).
func systemHandles() ([]handleEntry, error) {
	sysBufMu.Lock()
	defer sysBufMu.Unlock()
	size := sysBufHint
	for i := 0; i < 8; i++ {
		buf := make([]byte, size)
		var ret uint32
		r, _, _ := syscall.SyscallN(procNtQuerySystemInformation.Addr(), systemExtendedHandleInformation,
			uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&ret)))
		if uint32(r) == statusInfoLengthMismatch {
			size *= 2
			if int(ret) > size {
				size = int(ret) + 1<<20
			}
			continue
		}
		if r != 0 {
			return nil, errors.New("no se pudo leer la lista de archivos abiertos")
		}
		n := int(*(*uintptr)(unsafe.Pointer(&buf[0])))
		const hdr = 16
		const sz = int(unsafe.Sizeof(handleEntry{}))
		if max := (len(buf) - hdr) / sz; n > max {
			n = max
		}
		out := make([]handleEntry, n)
		if n > 0 {
			copy(out, unsafe.Slice((*handleEntry)(unsafe.Pointer(&buf[hdr])), n))
		}
		sysBufHint = size
		return out, nil
	}
	return nil, errors.New("no se pudo leer la lista de archivos abiertos")
}

// detectFileType averigua el índice del tipo «File» buscando un handle propio.
func detectFileType() {
	f, err := os.Open(os.Args[0])
	if err != nil {
		f, err = os.Open(`C:\Windows\System32\kernel32.dll`)
		if err != nil {
			return
		}
	}
	defer f.Close()
	hs, err := systemHandles()
	if err != nil {
		return
	}
	me := uintptr(windows.GetCurrentProcessId())
	h := f.Fd()
	for _, e := range hs {
		if e.PID == me && e.Handle == h {
			fileTypeIndex = e.TypeIndex
			return
		}
	}
}

// scanOpts limita el escaneo de handles.
type scanOpts struct {
	pid      uint32               // 0 = todos los procesos
	match    func(nt string) bool // qué rutas NT interesan
	budget   time.Duration        // tiempo máximo total
	perQuery time.Duration        // tiempo máximo por handle antes de darlo por colgado
}

// scanResult es el resultado del escaneo.
type scanResult struct {
	handles []openHandle
	denied  map[uint32]bool // procesos que no se pudieron abrir
	partial bool            // se acabó el tiempo
}

// scanHandles recorre los handles de archivo de los procesos que se pueden
// abrir y devuelve los que cumplen o.match.
func scanHandles(ctx context.Context, o scanOpts) (scanResult, error) {
	fileTypeOnce.Do(detectFileType)
	res := scanResult{denied: map[uint32]bool{}}
	all, err := systemHandles()
	if err != nil {
		return res, err
	}
	if o.budget <= 0 {
		o.budget = 2 * time.Second
	}
	if o.perQuery <= 0 {
		o.perQuery = 150 * time.Millisecond
	}
	me := windows.GetCurrentProcessId()

	// Abre cada proceso una sola vez.
	type job struct {
		proc  windows.Handle
		pid   uint32
		value uintptr
	}
	procs := map[uint32]windows.Handle{}
	jobs := make([]job, 0, 1<<14)
	for i := range all {
		e := &all[i]
		pid := uint32(e.PID)
		if pid <= 4 || pid == me || (o.pid != 0 && pid != o.pid) {
			continue
		}
		if fileTypeIndex != 0 && e.TypeIndex != fileTypeIndex {
			continue
		}
		ph, ok := procs[pid]
		if !ok {
			ph, err = windows.OpenProcess(windows.PROCESS_DUP_HANDLE, false, pid)
			if err != nil {
				ph = 0
				res.denied[pid] = true
			}
			procs[pid] = ph
		}
		if ph == 0 {
			continue
		}
		jobs = append(jobs, job{ph, pid, e.Handle})
	}
	all = nil
	if len(jobs) == 0 {
		closeAll(procs)
		return res, nil
	}

	ctx, cancel := context.WithTimeout(ctx, o.budget)
	defer cancel()

	var (
		next     atomic.Int64
		mu       sync.Mutex
		done     bool // tras devolver, los trabajadores colgados descartan su resultado
		found    []openHandle
		active   atomic.Int32
		running  sync.WaitGroup // trabajadores que todavía usan los handles de proceso
		finished = make(chan struct{})
		finOnce  sync.Once
		cur      = windows.CurrentProcess()
		workers  = runtime.NumCPU()
	)
	if workers > 12 {
		workers = 12
	}
	if workers < 2 {
		workers = 2
	}
	if len(jobs) < 64 {
		workers = 1
	}

	// Estado de cada trabajador: 0 en marcha, 1 terminado, 2 abandonado.
	type worker struct {
		since atomic.Int64 // inicio de la consulta actual (0 = libre)
		state atomic.Int32
	}
	leave := func() {
		if active.Add(-1) == 0 {
			finOnce.Do(func() { close(finished) })
		}
	}
	var wmu sync.Mutex
	var ws []*worker
	run := func(w *worker) {
		defer running.Done()
		runtime.LockOSThread() // el hilo puede quedarse bloqueado; mejor no compartirlo
		defer runtime.UnlockOSThread()
		defer func() {
			if w.state.CompareAndSwap(0, 1) {
				leave()
			}
		}()
		buf := make([]uint16, 512)
		for ctx.Err() == nil && w.state.Load() == 0 {
			i := int(next.Add(1) - 1)
			if i >= len(jobs) {
				return
			}
			j := jobs[i]
			w.since.Store(time.Now().UnixNano())
			var dup windows.Handle
			if windows.DuplicateHandle(j.proc, windows.Handle(j.value), cur, &dup, 0, false, 0) == nil {
				t, _ := windows.GetFileType(dup)
				var nt string
				if t == windows.FILE_TYPE_DISK {
					nt, buf = finalPathNT(dup, buf)
				}
				windows.CloseHandle(dup)
				if nt != "" && o.match(nt) {
					mu.Lock()
					if !done {
						found = append(found, openHandle{j.pid, j.value, nt})
					}
					mu.Unlock()
				}
			}
			w.since.Store(0)
		}
	}
	start := func() {
		w := &worker{}
		wmu.Lock()
		ws = append(ws, w)
		wmu.Unlock()
		active.Add(1)
		running.Add(1)
		go run(w)
	}
	for i := 0; i < workers; i++ {
		start()
	}

	// Vigila: si un trabajador lleva demasiado en un handle (p. ej. una
	// tubería síncrona que cuelga), se abandona y se lanza otro en su lugar.
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	extra := 0
loop:
	for {
		select {
		case <-finished:
			break loop
		case <-ctx.Done():
			res.partial = true
			break loop
		case <-tick.C:
			now := time.Now().UnixNano()
			wmu.Lock()
			list := append([]*worker(nil), ws...)
			wmu.Unlock()
			for _, w := range list {
				s := w.since.Load()
				if s == 0 || time.Duration(now-s) <= o.perQuery {
					continue
				}
				if w.state.CompareAndSwap(0, 2) {
					if extra < 32 {
						extra++
						start()
					} else {
						res.partial = true
					}
					leave()
				}
			}
		}
	}
	mu.Lock()
	done = true
	res.handles = found
	mu.Unlock()
	// Los handles de proceso se cierran cuando acaben todos los trabajadores,
	// también los que se quedaron colgados.
	go func() { running.Wait(); closeAll(procs) }()
	return res, nil
}

func closeAll(m map[uint32]windows.Handle) {
	for _, h := range m {
		if h != 0 {
			windows.CloseHandle(h)
		}
	}
}

// ntTarget es una ruta buscada, en formato NT.
type ntTarget struct {
	nt  string
	dir bool // carpeta o unidad: también cuenta lo que hay dentro
}

// ntMatcher crea un comparador de rutas NT: igual a una de las rutas o, si
// es carpeta, dentro de ella.
func ntMatcher(targets []ntTarget) func(string) bool {
	return func(nt string) bool {
		for _, t := range targets {
			if len(nt) < len(t.nt) || !strings.EqualFold(nt[:len(t.nt)], t.nt) {
				continue
			}
			if len(nt) == len(t.nt) {
				return true
			}
			if t.dir && (nt[len(t.nt)] == '\\' || strings.HasSuffix(t.nt, `\`)) {
				return true
			}
		}
		return false
	}
}
