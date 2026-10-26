//go:build windows

package lock

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// Proceso auxiliar: abre el archivo sin compartir, entra en su carpeta y espera.
func TestHelperProcess(t *testing.T) {
	path := os.Getenv("LOCK_HELPER_FILE")
	if path == "" {
		t.Skip("solo como proceso auxiliar")
	}
	p, _ := windows.UTF16PtrFromString(path)
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		os.Exit(3)
	}
	_ = h
	os.Chdir(filepath.Dir(path))
	os.Stdout.WriteString("listo\n")
	time.Sleep(60 * time.Second)
	os.Exit(0)
}

// startHolder lanza el auxiliar y espera a que tenga el archivo abierto.
func startHolder(t *testing.T, file string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$")
	cmd.Env = append(os.Environ(), "LOCK_HELPER_FILE="+file)
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	line, _ := bufio.NewReader(out).ReadString('\n')
	if !strings.Contains(line, "listo") {
		t.Fatalf("el auxiliar no arrancó: %q", line)
	}
	return cmd
}

func tempFile(t *testing.T) (dir, file string) {
	dir = t.TempDir()
	file = filepath.Join(dir, "bloqueado.txt")
	if err := os.WriteFile(file, []byte("hola"), 0o644); err != nil {
		t.Fatal(err)
	}
	return
}

func findPID(r *Report, pid int) *Process {
	for i := range r.Processes {
		if r.Processes[i].PID == pid {
			return &r.Processes[i]
		}
	}
	return nil
}

func TestFindFileAndFolder(t *testing.T) {
	st0 := time.Now()
	Warmup()
	t.Logf("Warmup: %v", time.Since(st0))
	dir, file := tempFile(t)
	cmd := startHolder(t, file)
	pid := cmd.Process.Pid
	ctx := context.Background()

	if err := os.Remove(file); err == nil {
		t.Fatal("el archivo debería estar bloqueado")
	}

	for _, target := range []string{file, dir} {
		st := time.Now()
		rep, err := Find(ctx, []string{target})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Find(%s): %v (%d ms internos), %d procesos, nota=%q ocultos=%v",
			filepath.Base(target), time.Since(st), rep.Millis, len(rep.Processes), rep.Note, rep.Hidden)
		p := findPID(rep, pid)
		if p == nil {
			t.Fatalf("no detecta el proceso %d en %s: %+v", pid, target, rep.Processes)
		}
		t.Logf("  %+v", *p)
		if p.Kind != KindApp || !p.CanClose || !p.CanUnlock {
			t.Errorf("datos inesperados: %+v", *p)
		}
		found := false
		for _, f := range p.Files {
			if strings.EqualFold(f, file) {
				found = true
			}
		}
		if !found {
			t.Errorf("no lista el archivo bloqueado: %v", p.Files)
		}
	}
}

func TestUnlockKeepsProcess(t *testing.T) {
	dir, file := tempFile(t)
	cmd := startHolder(t, file)
	pid := cmd.Process.Pid
	ctx := context.Background()
	if _, err := Find(ctx, []string{dir}); err != nil {
		t.Fatal(err)
	}
	st := time.Now()
	r := Unlock(ctx, pid, []string{dir})
	t.Logf("Unlock: %v %v", time.Since(st), r)
	if r.Status != StatusReleased || r.Handles < 2 || r.StillLocked {
		t.Fatalf("resultado inesperado: %+v", r)
	}
	if err := os.Remove(file); err != nil {
		t.Fatalf("sigue sin poder borrarse: %v", err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatalf("la carpeta sigue bloqueada: %v", err)
	}
	if _, ok := sameProcess(uint32(pid)); !ok {
		t.Fatal("el programa no debería haberse cerrado")
	}
}

func TestReleaseForce(t *testing.T) {
	_, file := tempFile(t)
	cmd := startHolder(t, file)
	pid := cmd.Process.Pid
	ctx := context.Background()
	if _, err := Find(ctx, []string{file}); err != nil {
		t.Fatal(err)
	}
	// Sin ventanas: sin forzar no se puede cerrar.
	if r := Release(ctx, pid, []string{file}, false); r.Status != StatusNeedsForce {
		t.Fatalf("esperaba needsForce: %v", r)
	}
	st := time.Now()
	r := Release(ctx, pid, []string{file}, true)
	t.Logf("Release forzado: %v %v", time.Since(st), r)
	if r.Status != StatusClosed {
		t.Fatalf("no se cerró: %v", r)
	}
	if err := os.Remove(file); err != nil {
		t.Fatalf("sigue sin poder borrarse: %v", err)
	}
	if r := Release(ctx, pid, []string{file}, true); r.Status != StatusGone {
		t.Errorf("esperaba gone: %v", r)
	}
}

// Cierre amable de un programa con ventana (PowerShell con un formulario).
func TestReleaseGentle(t *testing.T) {
	if testing.Short() {
		t.Skip("lento")
	}
	_, file := tempFile(t)
	script := `Add-Type -AssemblyName System.Windows.Forms; ` +
		`$h=[IO.File]::Open('` + file + `','Open','ReadWrite','None'); ` +
		`$w=New-Object Windows.Forms.Form; $w.Text='Prueba bloqueo'; $w.ShowInTaskbar=$false; ` +
		`$w.StartPosition='Manual'; $w.Location=New-Object Drawing.Point(-2000,-2000); ` +
		`[Windows.Forms.Application]::Run($w)`
	cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
	if err := cmd.Start(); err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	pid := cmd.Process.Pid
	ctx := context.Background()
	var p *Process
	for i := 0; i < 50 && (p == nil || !p.HasWindows); i++ {
		time.Sleep(200 * time.Millisecond)
		rep, err := Find(ctx, []string{file})
		if err != nil {
			t.Fatal(err)
		}
		p = findPID(rep, pid)
	}
	if p == nil || !p.HasWindows {
		t.Fatalf("no aparece con ventana: %+v", p)
	}
	t.Logf("%+v", *p)
	st := time.Now()
	r := Release(ctx, pid, []string{file}, false)
	t.Logf("Release amable: %v %v", time.Since(st), r)
	if r.Status != StatusClosed {
		t.Fatalf("no se cerró: %v", r)
	}
	if err := os.Remove(file); err != nil {
		t.Fatalf("sigue sin poder borrarse: %v", err)
	}
}

func TestExplorerNeverClosed(t *testing.T) {
	for pid, e := range processSnapshot() {
		if strings.EqualFold(e.exeName, "explorer.exe") {
			if k := kindOf(pid, e.exeName, nil); k != KindExplorer {
				t.Fatalf("explorer.exe clasificado como %s", k)
			}
			// Sin rutas bloqueadas no debe hacer nada (y nunca cerrarlo).
			dir := t.TempDir()
			r := Release(context.Background(), int(pid), []string{dir}, true)
			if _, ok := sameProcess(pid); !ok {
				t.Fatal("¡se ha cerrado el Explorador!")
			}
			t.Logf("Release(explorer): %v", r)
			return
		}
	}
	t.Skip("no hay Explorador abierto")
}

func TestFindMissing(t *testing.T) {
	if _, err := Find(context.Background(), []string{`C:\no\existe\nada.txt`}); err == nil {
		t.Fatal("esperaba error")
	}
}

func TestFindSystemFolder(t *testing.T) {
	st := time.Now()
	rep, err := Find(context.Background(), []string{`C:\Windows\System32`})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Find(System32): %v, %d procesos, parcial=%v, ocultos=%d", time.Since(st), len(rep.Processes), rep.Partial, len(rep.Hidden))
}

func TestRecycleAndMove(t *testing.T) {
	if testing.Short() {
		t.Skip("deja un archivo en la papelera")
	}
	dir := t.TempDir()
	a := filepath.Join(dir, "turbomenu-prueba-papelera.txt")
	os.WriteFile(a, []byte("x"), 0o644)
	st := time.Now()
	if err := Recycle([]string{a}); err != nil {
		t.Fatal(err)
	}
	t.Logf("Recycle: %v", time.Since(st))
	if _, err := os.Stat(a); err == nil {
		t.Fatal("sigue existiendo")
	}
	b := filepath.Join(dir, "mover.txt")
	os.WriteFile(b, []byte("x"), 0o644)
	dst := filepath.Join(dir, "destino", "sub")
	if err := Move([]string{b}, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "mover.txt")); err != nil {
		t.Fatal("no se movió")
	}
	// Un archivo bloqueado no se puede mover: error claro.
	c := filepath.Join(dir, "bloq.txt")
	os.WriteFile(c, []byte("x"), 0o644)
	startHolder(t, c)
	err := Move([]string{c}, dst)
	t.Logf("Move bloqueado: %v", err)
	if err == nil {
		t.Fatal("esperaba error")
	}
}

func TestEjectRefuses(t *testing.T) {
	for _, d := range []string{"C:", `C:\`} {
		if err := Eject(d); err == nil {
			t.Fatal("no debe expulsar el disco del sistema")
		} else {
			t.Logf("Eject(%s): %v", d, err)
		}
	}
	mask, _ := windows.GetLogicalDrives()
	for i := 25; i >= 3; i-- {
		if mask&(1<<i) == 0 {
			l := string(rune('A'+i)) + ":"
			err := Eject(l)
			t.Logf("Eject(%s): %v", l, err)
			if err == nil {
				t.Fatal("esperaba error")
			}
			break
		}
	}
	if err := Eject("12"); err == nil {
		t.Fatal("esperaba error")
	}
}

// Prueba real: TURBOMENU_EJECT=E: go test -run EjectReal -v
func TestEjectReal(t *testing.T) {
	d := os.Getenv("TURBOMENU_EJECT")
	if d == "" {
		t.Skip("define TURBOMENU_EJECT")
	}
	rep, err := Find(context.Background(), []string{d})
	if err == nil {
		t.Logf("bloqueos: %+v", rep.Processes)
	}
	t.Logf("Eject: %v", Eject(d))
}

func TestFindDrive(t *testing.T) {
	if testing.Short() {
		t.Skip("lento")
	}
	Warmup()
	for i := 0; i < 2; i++ {
		st := time.Now()
		rep, err := Find(context.Background(), []string{`C:`})
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, p := range rep.Processes {
			if p.Kind == KindApp {
				n++
			}
		}
		t.Logf("Find(C:): %v, %d procesos (%d programas), parcial=%v", time.Since(st), len(rep.Processes), n, rep.Partial)
	}
}

// Comprueba la búsqueda del dispositivo con el disco del sistema (sin expulsar).
func TestEjectTargetLookup(t *testing.T) {
	dev, target, err := ejectTarget(os.Getenv("SystemDrive"))
	t.Logf("dev=%d target=%d err=%v", dev, target, err)
	if dev == 0 {
		t.Fatal("no encuentra el dispositivo del disco del sistema")
	}
	if target != 0 {
		if os.Getenv("GITHUB_ACTIONS") != "" {
			// En las máquinas virtuales de GitHub los discos se pueden quitar en caliente.
			t.Skip("en GitHub Actions el disco del sistema se ve como extraíble")
		}
		t.Fatal("el disco interno no debería poder expulsarse")
	}
}
