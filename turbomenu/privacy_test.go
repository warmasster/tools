package main

import (
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// La clave de VirusTotal se guarda cifrada con DPAPI y se lee igual.
func TestVTKeyProtected(t *testing.T) {
	const key = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	enc, err := protect(key)
	if err != nil {
		t.Fatal(err)
	}
	if enc == "" || enc == key {
		t.Fatal("la clave no se ha cifrado")
	}
	got, err := unprotect(enc)
	if err != nil || got != key {
		t.Fatalf("descifrar: %q %v", got, err)
	}
	if _, err := unprotect("no-es-base64!"); err == nil {
		t.Error("un valor dañado no debería descifrarse")
	}
}

// Cada lado de las tuberías comprueba que el otro proceso es de este usuario.
func TestSameUser(t *testing.T) {
	if !sameUser(uint32(os.Getpid())) {
		t.Fatal("este proceso es de este usuario")
	}
	if sameUser(0) {
		t.Error("el pid 0 no es nadie")
	}
	// System (pid 4) es de SYSTEM, no de este usuario.
	if sameUser(4) {
		t.Error("System no es de este usuario")
	}
	if sa := pipeSA(); sa == nil || sa.SecurityDescriptor == nil {
		t.Error("las tuberías deberían tener permisos propios")
	} else if s := sa.SecurityDescriptor.String(); s == "" {
		t.Error("descriptor vacío")
	}
	_ = windows.GetCurrentProcessId()
}

// Con los permisos nuevos, un proceso del mismo usuario sigue pudiendo entregar trabajo.
func TestPipeRoundTrip(t *testing.T) {
	name := pipeName("prueba." + t.Name())
	got := make(chan []string, 1)
	srv := servePipe(name, func(j *Job) bool { got <- j.Paths; return true })
	defer srv.stop()
	if !sendJob(name, time.Second, &Job{Action: "reduce", Paths: []string{`C:\a.txt`}}) {
		t.Fatal("no se ha podido entregar el trabajo")
	}
	if p := <-got; len(p) != 1 || p[0] != `C:\a.txt` {
		t.Fatalf("rutas recibidas: %v", p)
	}
}
