package vault

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	// Más ligeros que los de verdad (256 MB, 6 pasadas) para que las pruebas vayan rápido.
	defaultKDF = kdfParams{Time: 2, Memory: 16 * 1024}
	os.Exit(m.Run())
}

func readKDF(t *testing.T, path string) kdfParams {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f fileData
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f.KDF
}

// Una bóveda de las de antes (parámetros flojos y maestra de 8) se sigue abriendo
// y, al abrirla, se refuerza sola sin perder nada.
func TestStrengthenOldVault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "boveda.dat")
	old := kdfParams{Alg: "argon2id", Salt: make([]byte, 16), Time: 1, Memory: 8 * 1024, Threads: 1}
	v := Open(path)
	data := &payload{Created: time.Now().Unix(), Entries: map[string]Entry{"cuenta": {Password: "secreta", Updated: 1}}}
	if err := v.save(derive("ocho1234", old), old, data); err != nil {
		t.Fatal(err)
	}

	v2 := Open(path)
	if err := v2.Unlock("ocho1234"); err != nil {
		t.Fatalf("una maestra de 8 de antes debe seguir abriendo: %v", err)
	}
	v2.WaitUpgrade()
	k := readKDF(t, path)
	if k.Time != defaultKDF.Time || k.Memory != defaultKDF.Memory {
		t.Fatalf("no se ha reforzado: %+v", k)
	}
	if k := readKDF(t, path+".bak"); k.Time != defaultKDF.Time {
		t.Errorf("la copia .bak sigue con los parámetros flojos: %+v", k)
	}
	// Sigue abierta y se puede usar.
	if e, ok, err := v2.Get("cuenta"); err != nil || !ok || e.Password != "secreta" {
		t.Fatalf("tras reforzar: %+v %v %v", e, ok, err)
	}
	if err := v2.Set("otra", Entry{Password: "dos"}); err != nil {
		t.Fatal(err)
	}

	v3 := Open(path)
	if err := v3.Unlock("ocho1234"); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"cuenta": "secreta", "otra": "dos"} {
		if e, _, _ := v3.Get(name); e.Password != want {
			t.Errorf("%s = %q", name, e.Password)
		}
	}
	// Una maestra nueva sí necesita 12.
	if err := v3.ChangeMaster("ocho1234", "corta1234"); err != ErrShort {
		t.Errorf("cambiar a una de 9: %v", err)
	}
}
