package checks

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPartSpeed(t *testing.T) {
	cases := map[string]int{
		"KF3200C16D4/16GX":    3200,
		"CMK16GX4M2B3200C16":  3200,
		"F4-3600C16-8GVKC":    3600,
		"F5-6000J3038F16G":    6000,
		"KF436C16BB/8":        3600,
		"KF560C40BBK2-32":     6000,
		"BL8G32C16U4B.M8FE1":  3200,
		"CP16G56C46U5.M8D1":   5600,
		"TF3D416G3200HC16F01": 3200,
		"KVR32N22S8/8":        3200,
		"M378A1K43DB2-CTD":    0,
		"":                    0,
	}
	for part, want := range cases {
		if got := PartSpeed(part); got != want {
			t.Errorf("%q: %d, quería %d", part, got, want)
		}
	}
}

func TestChannel(t *testing.T) {
	cases := map[[2]string]string{
		{"DDR4-A2", "BANK 0"}:              "A",
		{"ChannelB-DIMM0", "BANK 2"}:       "B",
		{"DIMM_A1", ""}:                    "A",
		{"DIMM 0", "P0 CHANNEL B"}:         "B",
		{"A1", "BANK 0"}:                   "A",
		{"Controller0-ChannelA-DIMM0", ""}: "A",
		{"DIMM 1", "BANK 1"}:               "",
	}
	for in, want := range cases {
		if got := Channel(DIMM{Locator: in[0], Bank: in[1]}); got != want {
			t.Errorf("%v: %q, quería %q", in, got, want)
		}
	}
}

func TestNvidiaVersion(t *testing.T) {
	if v := nvidiaVersion("32.0.16.1664"); v != "616.64" {
		t.Fatal(v)
	}
}

// Revisión de verdad de este equipo (solo lee, no cambia nada).
func TestRun(t *testing.T) {
	start := time.Now()
	r := Run(nil)
	b, _ := json.MarshalIndent(r, "", "  ")
	t.Logf("revisión en %v:\n%s", time.Since(start), b)
	t.Logf("módulos: %+v", DIMMs())
	t.Logf("pantallas: %+v", Monitors())
	t.Logf("juegos: %+v", GameLibraries())
	if len(r.Results) < 8 {
		t.Fatalf("muy pocas comprobaciones: %d", len(r.Results))
	}
}
