package safe

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Pruebas contra el Windows real (firma, Defender, base de archivos conocidos).

func sysFile(t *testing.T, name string) string {
	p := filepath.Join(os.Getenv("SystemRoot"), name)
	if _, err := os.Stat(p); err != nil {
		t.Skip("no existe " + p)
	}
	return p
}

func TestSignatureCatalog(t *testing.T) {
	p := sysFile(t, "notepad.exe")
	start := time.Now()
	s, err := CheckSignature(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s en %v: %+v", p, time.Since(start), *s)
	if s.Status != SignValid || !s.Catalog || s.Signer == "" {
		t.Fatalf("notepad.exe debería estar firmado por catálogo: %+v", s)
	}
}

func TestSignatureEmbedded(t *testing.T) {
	// Algún programa del sistema con firma incrustada.
	var p string
	for _, c := range []string{
		filepath.Join(os.Getenv("SystemRoot"), "explorer.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Windows Defender", "MpCmdRun.exe"),
		filepath.Join(os.Getenv("SystemRoot"), "System32", "ntoskrnl.exe"),
	} {
		if fileExists(c) {
			p = c
			break
		}
	}
	if p == "" {
		t.Skip("sin archivo de prueba")
	}
	start := time.Now()
	s, err := CheckSignature(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s en %v: %+v", p, time.Since(start), *s)
	if s.Status != SignValid || s.Signer == "" {
		t.Fatalf("firma esperada válida: %+v", s)
	}
}

func TestSignatureUnsignedAndTampered(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "nota.txt")
	os.WriteFile(plain, []byte("hola"), 0o644)
	s, err := CheckSignature(context.Background(), plain)
	if err != nil {
		t.Fatal(err)
	}
	if s.Status != SignUnsigned {
		t.Fatalf("txt: %+v", s)
	}

	// Un ejecutable firmado con un byte cambiado: la firma debe salir rota.
	src := filepath.Join(os.Getenv("SystemRoot"), "explorer.exe")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Skip("sin explorer.exe")
	}
	data[len(data)/3] ^= 0xFF
	bad := filepath.Join(dir, "roto.exe")
	os.WriteFile(bad, data, 0o644)
	s, err = CheckSignature(context.Background(), bad)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("modificado: %+v", *s)
	if s.Status != SignTampered {
		t.Fatalf("esperaba firma rota: %s", s.Status)
	}
}

func TestDefenderReal(t *testing.T) {
	if testing.Short() {
		t.Skip("lento")
	}
	dir := t.TempDir()
	clean := filepath.Join(dir, "limpio.txt")
	os.WriteFile(clean, []byte("archivo de prueba sin nada raro"), 0o644)
	r, err := ScanDefender(context.Background(), clean)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("limpio: %+v", *r)
	if !r.Available {
		t.Skip("Defender no disponible")
	}
	if !r.Clean {
		t.Fatalf("debería estar limpio: %+v", r)
	}

	// Archivo de prueba EICAR (inofensivo, todos los antivirus lo detectan).
	eicar := filepath.Join(dir, "eicar.com")
	body := `X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*`
	if err := os.WriteFile(eicar, []byte(body), 0o644); err != nil {
		t.Skip("no se pudo crear EICAR: " + err.Error())
	}
	time.Sleep(300 * time.Millisecond)
	if !fileExists(eicar) {
		t.Skip("la protección en tiempo real borró EICAR al crearlo")
	}
	r, err = ScanDefender(context.Background(), eicar)
	os.Remove(eicar)
	if err != nil {
		t.Skip("EICAR: " + err.Error())
	}
	t.Logf("eicar: %+v", *r)
	if r.Clean {
		t.Fatalf("EICAR debería detectarse")
	}
}

func TestKnownReal(t *testing.T) {
	if testing.Short() {
		t.Skip("red")
	}
	// SHA-1 de un archivo de Windows registrado en la NSRL.
	k, err := LookupKnown(context.Background(), "00000079FD7AAC9B2F9C988C50750E1F50B27EB5")
	if err != nil {
		t.Skip("sin red: " + err.Error())
	}
	t.Logf("conocido: %+v", *k)
	if !k.Known || k.Source != "NSRL" {
		t.Fatalf("debería ser conocido: %+v", k)
	}
	k, err = LookupKnown(context.Background(), "0123456789012345678901234567890123456789")
	if err != nil {
		t.Fatal(err)
	}
	if k.Known {
		t.Fatalf("no debería ser conocido")
	}
}

func TestAnalyzeReal(t *testing.T) {
	if testing.Short() {
		t.Skip("lento")
	}
	p := sysFile(t, "notepad.exe")
	start := time.Now()
	ev := Analyze(context.Background(), p, Options{}, func(u Update) {
		t.Logf("%6v %-10s → %s: %s", time.Since(start).Round(time.Millisecond), u.Step, u.Verdict.Level, u.Verdict.Summary)
	})
	v := Judge(ev)
	t.Logf("final: %+v errores=%v", v, ev.Errors)
	if v.Level != Clean {
		t.Fatalf("notepad debería salir limpio: %+v", v)
	}
}

// Analiza archivos concretos y enseña todo: TURBOMENU_INSPECT="ruta1;ruta2" (opcional).
func TestInspect(t *testing.T) {
	list := os.Getenv("TURBOMENU_INSPECT")
	if list == "" {
		t.Skip("define TURBOMENU_INSPECT")
	}
	for _, p := range filepath.SplitList(list) {
		start := time.Now()
		ev := Analyze(context.Background(), p, Options{}, func(u Update) {
			t.Logf("%8v %-10s → %s: %s", time.Since(start).Round(time.Millisecond), u.Step, u.Verdict.Level, u.Verdict.Summary)
		})
		if ev.Signature != nil {
			t.Logf("firma: %+v", *ev.Signature)
		}
		if ev.Defender != nil {
			t.Logf("defender: %+v", *ev.Defender)
		}
		if ev.Origin != nil {
			t.Logf("origen: %+v", *ev.Origin)
		}
		if ev.Known != nil {
			t.Logf("conocido: %+v", *ev.Known)
		}
		t.Logf("veredicto: %+v errores=%v", Judge(ev), ev.Errors)
	}
}
