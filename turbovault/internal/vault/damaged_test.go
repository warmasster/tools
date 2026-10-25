package vault

import (
	"os"
	"path/filepath"
	"testing"
)

// Con un archivo de bóveda que no se puede leer, «crear» no lo pisa.
func TestCreateRefusesOverDamagedVault(t *testing.T) {
	p := filepath.Join(t.TempDir(), "boveda.tvault")
	os.WriteFile(p, []byte("esto no es una bóveda"), 0o600)
	v := Open(p)
	if v.Damaged() == nil {
		t.Fatal("el archivo roto debería verse como dañado")
	}
	if err := v.Create("Una-Maestra-Larga-De-Prueba-91!"); err == nil {
		t.Fatal("no debería crear una bóveda encima de una dañada")
	}
	if b, _ := os.ReadFile(p); string(b) != "esto no es una bóveda" {
		t.Error("el archivo dañado se ha tocado")
	}
}
