package parts

// Prueba de velocidad de un disco: escribe un archivo saltándose la caché de
// Windows, lo lee igual (con 4 lecturas a la vez, como hacen los juegos al
// cargar) y lo borra.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	fileFlagNoBuffering  = 0x20000000
	fileFlagWriteThrough = 0x80000000
	chunk                = 4 << 20
)

// SpeedResult es lo medido.
type SpeedResult struct {
	Read  float64 `json:"read"`  // MB/s
	Write float64 `json:"write"` // MB/s
	At    int64   `json:"at"`
}

func alignedBuf(n int) ([]byte, func()) {
	p, err := windows.VirtualAlloc(0, uintptr(n), windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_READWRITE)
	if err != nil {
		b := make([]byte, n)
		return b, func() {}
	}
	b := unsafe.Slice((*byte)(*(*unsafe.Pointer)(unsafe.Pointer(&p))), n)
	return b, func() { windows.VirtualFree(p, 0, windows.MEM_RELEASE) }
}

// testDir elige dónde poner el archivo en esa unidad.
func testDir(letter string) string {
	sys := os.Getenv("SystemDrive")
	if strings.EqualFold(letter, sys) {
		return os.TempDir()
	}
	return letter + `\`
}

// SpeedTest mide un disco por una de sus letras. progress recibe la fase
// («write», «read») y el avance (0-100).
func SpeedTest(letter string, sizeMB int, progress func(phase string, pct float64)) (SpeedResult, error) {
	var res SpeedResult
	if sizeMB < 64 {
		sizeMB = 64
	}
	total := int64(sizeMB) << 20
	free, _, _ := diskFree(letter)
	if free < uint64(total)*2 {
		return res, errors.New("no hay espacio libre suficiente en ese disco para la prueba")
	}
	path := filepath.Join(testDir(letter), fmt.Sprintf("TurboMON-prueba-%d.tmp", os.Getpid()))
	defer os.Remove(path)
	p, _ := windows.UTF16PtrFromString(path)
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE, 0, nil, windows.CREATE_ALWAYS, windows.FILE_ATTRIBUTE_TEMPORARY|fileFlagNoBuffering|fileFlagWriteThrough, 0)
	if err != nil {
		return res, fmt.Errorf("no se puede escribir en %s: %v", letter, err)
	}
	buf, free1 := alignedBuf(chunk)
	defer free1()
	for i := range buf {
		buf[i] = byte(i*131 + 7) // datos que no se comprimen fácil
	}
	start := time.Now()
	var done int64
	for done < total {
		var n uint32
		if err := windows.WriteFile(h, buf, &n, nil); err != nil {
			windows.CloseHandle(h)
			return res, err
		}
		done += int64(n)
		progress("write", float64(done)*100/float64(total))
	}
	windows.CloseHandle(h)
	res.Write = float64(total) / (1 << 20) / time.Since(start).Seconds()

	// Lectura: 4 a la vez, cada una de su cuarto.
	const workers = 4
	part := total / workers
	var mu sync.Mutex
	var readDone int64
	var firstErr error
	var wg sync.WaitGroup
	start = time.Now()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			rh, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, fileFlagNoBuffering, 0)
			if err != nil {
				mu.Lock()
				firstErr = err
				mu.Unlock()
				return
			}
			defer windows.CloseHandle(rh)
			b, f := alignedBuf(chunk)
			defer f()
			off := int64(w) * part
			if _, err := windows.Seek(rh, off, 0); err != nil {
				return
			}
			for got := int64(0); got < part; {
				var n uint32
				if err := windows.ReadFile(rh, b, &n, nil); err != nil || n == 0 {
					break
				}
				got += int64(n)
				mu.Lock()
				readDone += int64(n)
				progress("read", float64(readDone)*100/float64(total))
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	if firstErr != nil {
		return res, firstErr
	}
	res.Read = float64(readDone) / (1 << 20) / time.Since(start).Seconds()
	res.At = time.Now().Unix()
	return res, nil
}

func diskFree(letter string) (free, total uint64, ok bool) {
	p, _ := windows.UTF16PtrFromString(letter + `\`)
	var avail, tot, totFree uint64
	if windows.GetDiskFreeSpaceEx(p, &avail, &tot, &totFree) != nil {
		return 0, 0, false
	}
	return avail, tot, true
}
