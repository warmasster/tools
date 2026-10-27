package steam

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Un ejecutable sin firma (el de la propia prueba) o firmado por otro no pasa.
func TestSignatureRejects(t *testing.T) {
	self, _ := os.Executable()
	if checkSignedHook(self) == nil {
		t.Error("un ejecutable sin firma no debería pasar por steam.exe")
	}
	edge := filepath.Join(os.Getenv("ProgramFiles(x86)"), `Microsoft\Edge\Application\msedge.exe`)
	if _, err := os.Stat(edge); err == nil {
		if name, err := verifiedSigner(edge); err != nil || !strings.HasPrefix(name, "Microsoft") {
			t.Errorf("firmante de Edge: %q, %v", name, err)
		}
		if checkSignedHook(edge) == nil {
			t.Error("Edge (firmado por Microsoft) no debería pasar por steam.exe")
		}
	}
}

// El steam.exe de este PC está firmado por Valve (solo lee).
//
//	$env:TURBOGAMES_REAL = "1"; go test -run RealSignature -v ./internal/steam/
func TestRealSignature(t *testing.T) {
	if os.Getenv("TURBOGAMES_REAL") == "" {
		t.Skip("pon TURBOGAMES_REAL=1 para mirar el Steam de este PC")
	}
	in, err := Locate("")
	if err != nil {
		t.Fatal(err)
	}
	name, err := verifiedSigner(in.Exe)
	t.Logf("%s firmado por %q", in.Exe, name)
	if err != nil || in.checkSigned() != nil {
		t.Fatalf("steam.exe no pasa: %v", err)
	}
	t.Logf("procesos de Steam de su carpeta: %v (principal %v)", in.ownPIDs(false), in.ownPIDs(true))
}
