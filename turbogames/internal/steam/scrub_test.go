package steam

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"turbogames/internal/vdf"
)

// Olvidar una cuenta quita su sesión también de las copias de local.vdf (el resto
// de cada copia se queda igual).
func TestForgetScrubsBackups(t *testing.T) {
	in, _ := fakeSteam(t)
	// Una copia de antes, con la sesión de «primera».
	in.backup(LocalVDF())
	if err := in.Forget("primera"); err != nil {
		t.Fatal(err)
	}
	ents, _ := os.ReadDir(in.BackupDir)
	seen := 0
	for _, e := range ents {
		if !strings.HasPrefix(e.Name(), "local-") {
			continue
		}
		seen++
		root, err := vdf.ParseFile(filepath.Join(in.BackupDir, e.Name()))
		if err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		keys := map[string]bool{}
		for _, c := range root.Path(connectCachePath...).Kids() {
			keys[strings.ToLower(c.Key)] = true
		}
		if HasToken(keys, "primera") {
			t.Errorf("la copia %s sigue con la sesión de la cuenta olvidada", e.Name())
		}
	}
	if seen == 0 {
		t.Fatal("no hay copias de local.vdf")
	}
}
