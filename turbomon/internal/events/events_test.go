package events

import (
	"encoding/json"
	"testing"
)

// Lee los registros de este equipo (solo lee).
func TestCollect(t *testing.T) {
	r := Collect(Options{Days: 60, IntelRaptor: true})
	b, _ := json.MarshalIndent(r, "", "  ")
	t.Logf("%s", b)
}

func TestClassify(t *testing.T) {
	if title, _, _, _ := classifyCrash("game.exe", "nvwgf2umx.dll", "0xc0000005"); title != "Fallo en el driver de la gráfica" {
		t.Fatal(title)
	}
	if title, _, _, _ := classifyCrash("game.exe", "DiscordHook64.dll", "0xc0000005"); title != "Lo ha tirado un overlay" {
		t.Fatal(title)
	}
	if _, _, _, unst := classifyCrash("game.exe", "game.exe", "0xc000001d"); !unst {
		t.Fatal("instrucción ilegal debería ser inestabilidad")
	}
	if parseBugcheck("0x00000116 (0xffff, 0x0)") != 0x116 {
		t.Fatal("bugcheck")
	}
	if gameLabel("The Perfect Tower II.exe", `C:\Program Files (x86)\Steam\steamapps\common\The Perfect Tower II\The Perfect Tower II.exe`) != "The Perfect Tower II" {
		t.Fatal("nombre")
	}
}
