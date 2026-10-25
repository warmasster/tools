package vault

import (
	"os"
	"path/filepath"
	"testing"
)

// Con boveda.dat ilegible, ni «crear» ni «borrar» la pisan (ni a su .bak).
func TestDamagedVaultIsNotReplaced(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "boveda.dat")
	os.WriteFile(p, []byte("basura"), 0o600)
	os.WriteFile(p+".bak", []byte("copia buena"), 0o600)
	v := Open(p)
	if v.Damaged() == nil {
		t.Fatal("debería verse dañada")
	}
	if err := v.Create("maestra-de-prueba"); err == nil {
		t.Error("no debería crear otra encima de una dañada")
	}
	if err := v.Destroy("lo-que-sea"); err == nil {
		t.Error("no debería borrar una bóveda que no puede comprobar")
	}
	if b, _ := os.ReadFile(p + ".bak"); string(b) != "copia buena" {
		t.Errorf("el .bak se ha tocado: %q", b)
	}
	if b, _ := os.ReadFile(p); string(b) != "basura" {
		t.Errorf("el archivo se ha tocado: %q", b)
	}
}

// Tras cambiar la maestra, el .bak ya no se abre con la vieja.
func TestChangeMasterRefreshesBackup(t *testing.T) {
	p := filepath.Join(t.TempDir(), "boveda.dat")
	v := Open(p)
	if err := v.Create("maestra-vieja"); err != nil {
		t.Fatal(err)
	}
	if err := v.Set("cuenta", Entry{Password: "secreto"}); err != nil {
		t.Fatal(err)
	}
	if err := v.ChangeMaster("maestra-vieja", "maestra-nueva-2"); err != nil {
		t.Fatal(err)
	}
	bak := Open(p + ".bak")
	if err := bak.Unlock("maestra-vieja"); err == nil {
		t.Error("el .bak sigue abriéndose con la maestra vieja")
	}
	if err := bak.Unlock("maestra-nueva-2"); err != nil {
		t.Errorf("el .bak debería abrirse con la nueva: %v", err)
	}
}

// Parámetros de Argon2 absurdos en el archivo: dañado, no un cuelgue.
func TestKDFParamsBounded(t *testing.T) {
	p := filepath.Join(t.TempDir(), "boveda.dat")
	os.WriteFile(p, []byte(`{"v":1,"kdf":{"alg":"argon2id","salt":"AAAAAAAAAAAAAAAAAAAAAA==","t":0,"m":65536,"p":1},"nonce":"AAAAAAAAAAAAAAAA","data":"AA=="}`), 0o600)
	if Open(p).Damaged() == nil {
		t.Error("t=0 debería darse por dañado")
	}
}
