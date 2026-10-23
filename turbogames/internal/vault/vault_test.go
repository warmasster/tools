package vault

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestVault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "boveda.dat")
	v := Open(path)
	if v.Exists() || v.Unlocked() {
		t.Fatal("no debería existir")
	}
	if err := v.Create("corta"); err != ErrShort {
		t.Fatalf("contraseña corta: %v", err)
	}
	if err := v.Create("maestra-segura"); err != nil {
		t.Fatal(err)
	}
	if err := v.Create("otra-maestra"); err != ErrExists {
		t.Fatalf("crear dos veces: %v", err)
	}
	e := Entry{Password: "hunter2-ñ", Email: "a@b.c", Recovery: "R12345"}
	if err := v.Set("cuenta", e); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(path)
	if bytes.Contains(raw, []byte("hunter2")) || bytes.Contains(raw, []byte("a@b.c")) || bytes.Contains(raw, []byte("cuenta")) {
		t.Fatal("hay datos sin cifrar en el archivo")
	}

	v.Lock()
	if _, _, err := v.Get("cuenta"); err != ErrLocked {
		t.Fatal("cerrada debería negarse")
	}
	v2 := Open(path)
	if err := v2.Unlock("maestra-mala"); err != ErrWrong {
		t.Fatalf("contraseña mala: %v", err)
	}
	if err := v2.Unlock("maestra-segura"); err != nil {
		t.Fatal(err)
	}
	got, ok, _ := v2.Get("cuenta")
	if !ok || got.Password != e.Password || got.Recovery != "R12345" || got.Updated == 0 {
		t.Fatalf("leído mal: %+v", got)
	}

	if err := v2.Rename("cuenta", "nueva"); err != nil {
		t.Fatal(err)
	}
	if names, _ := v2.Names(); len(names) != 1 || names[0] != "nueva" {
		t.Fatalf("renombrar: %v", names)
	}

	if err := v2.ChangeMaster("maestra-mala", "otra-maestra-1"); err != ErrWrong {
		t.Fatalf("cambiar con la vieja mal: %v", err)
	}
	if err := v2.ChangeMaster("maestra-segura", "otra-maestra-1"); err != nil {
		t.Fatal(err)
	}
	v3 := Open(path)
	if err := v3.Unlock("maestra-segura"); err != ErrWrong {
		t.Fatal("la maestra vieja ya no debería valer")
	}
	if err := v3.Unlock("otra-maestra-1"); err != nil {
		t.Fatal(err)
	}
	if got, ok, _ := v3.Get("nueva"); !ok || got.Password != e.Password {
		t.Fatal("se perdieron datos al cambiar la maestra")
	}

	// borrar una entrada guardándola vacía
	if err := v3.Set("nueva", Entry{}); err != nil {
		t.Fatal(err)
	}
	if names, _ := v3.Names(); len(names) != 0 {
		t.Fatal("no se borró")
	}

	if err := v3.Destroy("mala-mala"); err != ErrWrong {
		t.Fatal("destruir sin la maestra debería negarse")
	}
	if err := v3.Destroy("otra-maestra-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("el archivo sigue ahí")
	}
}

func TestTamper(t *testing.T) {
	path := filepath.Join(t.TempDir(), "boveda.dat")
	v := Open(path)
	if err := v.Create("maestra-segura"); err != nil {
		t.Fatal(err)
	}
	_ = v.Set("x", Entry{Password: "p"})
	v.Lock()

	f := Open(path).file
	f.Data[len(f.Data)/2] ^= 1
	v2 := &Vault{path: path, file: f}
	if err := v2.Unlock("maestra-segura"); err != ErrWrong {
		t.Fatalf("un archivo cambiado no debería abrirse: %v", err)
	}

	_ = os.WriteFile(path, []byte("basura"), 0o600)
	v3 := Open(path)
	if v3.Damaged() != ErrCorrupted || v3.Unlock("maestra-segura") != ErrCorrupted {
		t.Fatal("debería avisar de que está dañado")
	}
}
