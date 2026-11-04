package prefetch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"turbomon/internal/etw"
)

// Pruebas con eventos de verdad (como administrador):
//
//	$env:TURBOMON_REAL="1"; go test -run Real -v ./internal/prefetch/
func needReal(t *testing.T) {
	if os.Getenv("TURBOMON_REAL") == "" {
		t.Skip("TURBOMON_REAL no está puesto")
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("hace falta ser administrador")
	}
}

func TestRealKernelFile(t *testing.T) {
	needReal(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "datos.pak")
	os.WriteFile(f, make([]byte, 12<<20), 0o644)
	s, err := etw.Start("TurboMON-PruebaArchivos", etw.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	rec := NewRecorder()
	rec.Pid = uint32(os.Getpid())
	if err := s.Enable(Provider, 4, Keywords, []uint32{uint32(os.Getpid())}, rec.Handle); err != nil {
		t.Fatal(err)
	}
	time.Sleep(700 * time.Millisecond)
	// Lectura sin caché: va al disco seguro.
	name, _ := windows.UTF16PtrFromString(f)
	h, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_NO_BUFFERING, 0)
	if err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, 2<<20)
	buf := raw[4096-int(uintptr(unsafe.Pointer(&raw[0]))%4096):][:1<<20]
	for off := int64(0); off < 12<<20; off += 1 << 20 {
		ov := windows.Overlapped{Offset: uint32(off), OffsetHigh: uint32(off >> 32)}
		var n uint32
		windows.ReadFile(h, buf, &n, &ov)
	}
	windows.CloseHandle(h)
	time.Sleep(2 * time.Second)
	s.Disable(Provider)
	p := rec.Plan(1 << 30)
	t.Logf("archivos vistos: %d", len(p.Files))
	if len(p.Files) > 5 {
		t.Errorf("se cuelan archivos de otros programas: %d", len(p.Files))
	}
	for _, pf := range p.Files {
		t.Logf("  %s: %d trozos", pf.Path, len(pf.Chunks))
		if strings.EqualFold(pf.Path, f) {
			if len(pf.Chunks) != 3 {
				t.Fatalf("trozos %v", pf.Chunks)
			}
			return
		}
	}
	t.Fatal("no ha visto el archivo")
}

func TestRealProcessStart(t *testing.T) {
	needReal(t)
	s, err := etw.Start("TurboMON-PruebaProcesos", etw.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	var mu sync.Mutex
	seen := map[uint32]string{}
	raw := map[uint32]string{}
	ids := map[uint16]int{}
	if err := s.Enable(ProcessProvider, 4, 0x10, nil, func(r *etw.Record) {
		mu.Lock()
		ids[r.Header.ID]++
		if b := r.Data(); r.Header.ID == 1 && len(b) >= 4 {
			raw[uint32(b[0])|uint32(b[1])<<8|uint32(b[2])<<16|uint32(b[3])<<24] = fmt.Sprintf("v%d, %d bytes", r.Header.Version, len(b))
		}
		mu.Unlock()
		if pid, img, ok := ParseProcessStart(r); ok {
			mu.Lock()
			seen[pid] = img
			mu.Unlock()
		}
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(700 * time.Millisecond)
	cmd := exec.Command("cmd.exe", "/c", "ping -n 2 127.0.0.1 >nul")
	cmd.Run()
	pid := uint32(cmd.ProcessState.Pid())
	time.Sleep(4 * time.Second)
	mu.Lock()
	img := seen[pid]
	t.Logf("eventos por id: %v", ids)
	t.Logf("evento del hijo: %s", raw[pid])
	mu.Unlock()
	r := NewRecorder()
	t.Logf("pid %d → %q → %q", pid, img, r.DOSPath(img))
	if !strings.HasSuffix(strings.ToLower(img), `\cmd.exe`) {
		t.Fatalf("no ha visto arrancar cmd.exe (%d eventos)", len(seen))
	}
	if !strings.HasSuffix(strings.ToLower(r.DOSPath(img)), `\system32\cmd.exe`) {
		t.Fatal("la ruta no se ha pasado a letra")
	}
}
