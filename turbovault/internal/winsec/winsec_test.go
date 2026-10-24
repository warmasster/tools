package winsec

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestProcessInfo(t *testing.T) {
	pid := windows.GetCurrentProcessId()
	p, err := ProcessPath(pid)
	if err != nil {
		t.Fatal(err)
	}
	self, _ := os.Executable()
	if !SamePath(p, self) {
		t.Fatalf("ruta %q, esperaba %q", p, self)
	}
	args, err := CommandLine(pid)
	if err != nil || len(args) == 0 {
		t.Fatalf("línea de órdenes: %v %v", args, err)
	}
	if pp, err := ParentPID(pid); err != nil || pp == 0 {
		t.Fatalf("padre: %v %v", pp, err)
	}
	if _, err := BrowserAncestor(pid); err == nil {
		t.Fatal("las pruebas no las lanza un navegador")
	}
}

func TestSigner(t *testing.T) {
	notepad := filepath.Join(windowsDir(), "System32", "notepad.exe")
	if _, err := os.Stat(notepad); err != nil {
		t.Skip("sin notepad")
	}
	// notepad va firmado por catálogo, no lleva firma dentro: debe fallar
	// limpiamente. Explorer tampoco. Se prueba con el propio Go si está firmado.
	if s, err := Signer(notepad); err == nil {
		t.Logf("notepad firmado por %q", s)
	}
	chrome := `C:\Program Files\Google\Chrome\Application\chrome.exe`
	if _, err := os.Stat(chrome); err == nil {
		s, err := Signer(chrome)
		if err != nil || s != "Google LLC" {
			t.Fatalf("Chrome: %q %v", s, err)
		}
	}
	self, _ := os.Executable()
	if _, err := Signer(self); err == nil {
		t.Fatal("el binario de pruebas no está firmado")
	}
}

func TestSecureDir(t *testing.T) {
	d := filepath.Join(t.TempDir(), "segura")
	if err := SecureDir(d); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(d, "x"), []byte("1"), 0o600)
	out, err := exec.Command("icacls", d).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "BUILTIN\\Users") || strings.Contains(s, "Everyone") || strings.Contains(s, "Todos") || strings.Contains(s, "BUILTIN\\Usuarios") {
		t.Fatalf("la carpeta deja entrar a otros:\n%s", s)
	}
}

func TestHardenChild(t *testing.T) {
	if os.Getenv("WINSEC_CHILD") == "1" {
		if err := HardenProcess(); err != nil {
			os.Exit(3)
		}
		if err := BlockInjection(true); err != nil {
			os.Exit(4)
		}
		os.Stdin.Read(make([]byte, 1)) // esperar a que el padre mire
		os.Exit(0)
	}
	skipIfDebugger(t)
	self, _ := os.Executable()
	cmd := exec.Command(self, "-test.run", "TestHardenChild")
	cmd.Env = append(os.Environ(), "WINSEC_CHILD=1")
	in, _ := cmd.StdinPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := uint32(cmd.Process.Pid)
	// Esperar a que se proteja: hasta que deje de poder leerse su memoria.
	ok := false
	for i := 0; i < 200; i++ {
		h, err := windows.OpenProcess(windows.PROCESS_VM_READ, false, pid)
		if err != nil {
			ok = true
			break
		}
		windows.CloseHandle(h)
		windows.SleepEx(20, false)
	}
	if !ok {
		in.Close()
		cmd.Wait()
		t.Fatal("otro proceso del usuario sigue pudiendo leer su memoria")
	}
	if h, err := windows.OpenProcess(windows.WRITE_DAC, false, pid); err == nil {
		windows.CloseHandle(h)
		t.Error("se pueden cambiar sus permisos")
	}
	if h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid); err != nil {
		t.Error("debe seguir pudiéndose consultar")
	} else {
		windows.CloseHandle(h)
	}
	in.Write([]byte{1})
	in.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("el hijo falló: %v", err)
	}
}

func TestHardenThreadsChild(t *testing.T) {
	if os.Getenv("WINSEC_THREADS") == "1" {
		if err := HardenProcess(); err != nil {
			os.Exit(3)
		}
		if err := HardenThreads(); err != nil {
			os.Exit(4)
		}
		// Hilos nuevos después de proteger, y trabajo que obliga a Go a
		// interrumpir hilos (lo hace con SuspendThread): no debe romperse nada.
		done := make(chan bool)
		for i := 0; i < 8; i++ {
			go func() {
				runtime.LockOSThread()
				x := 0
				for t0 := time.Now(); time.Since(t0) < 800*time.Millisecond; {
					x++
				}
				done <- x > 0
			}()
		}
		for i := 0; i < 8; i++ {
			<-done
		}
		runtime.GC()
		os.Stdout.Write([]byte("listo\n"))
		os.Stdin.Read(make([]byte, 1))
		os.Exit(0)
	}
	skipIfDebugger(t)
	self, _ := os.Executable()
	cmd := exec.Command(self, "-test.run", "TestHardenThreadsChild")
	cmd.Env = append(os.Environ(), "WINSEC_THREADS=1")
	in, _ := cmd.StdinPipe()
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	n, _ := out.Read(buf)
	if !strings.Contains(string(buf[:n]), "listo") {
		in.Close()
		cmd.Wait()
		t.Fatalf("el hijo no terminó su trabajo: %q", buf[:n])
	}
	pid := uint32(cmd.Process.Pid)
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		t.Fatal(err)
	}
	var te windows.ThreadEntry32
	te.Size = uint32(unsafe.Sizeof(te))
	threads, open := 0, 0
	for err = windows.Thread32First(snap, &te); err == nil; err = windows.Thread32Next(snap, &te) {
		if te.OwnerProcessID != pid {
			continue
		}
		threads++
		const setContext, suspend = 0x0010, 0x0002
		if h, err := windows.OpenThread(setContext|suspend, false, te.ThreadID); err == nil {
			windows.CloseHandle(h)
			open++
		}
	}
	windows.CloseHandle(snap)
	in.Write([]byte{1})
	in.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("el hijo falló: %v", err)
	}
	if threads < 8 || open > 0 {
		t.Fatalf("%d de %d hilos se pueden parar y desviar desde fuera", open, threads)
	}
	t.Logf("%d hilos, ninguno accesible", threads)
}
