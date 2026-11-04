package parts

import (
	"encoding/json"
	"testing"
	"time"
)

// Lee las piezas de este equipo (solo lee: no cambia nada).
func TestRead(t *testing.T) {
	start := time.Now()
	c := Read(0)
	b, _ := json.MarshalIndent(c, "", "  ")
	t.Logf("piezas en %v:\n%s", time.Since(start), b)
	if c.CPU.Name == "" || c.CPU.Threads == 0 {
		t.Fatal("sin procesador")
	}
}

func TestEDID(t *testing.T) {
	e := make([]byte, 128)
	copy(e, []byte{0, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0})
	e[8], e[9] = 0x05, 0xE3 // AOC
	e[21], e[22] = 53, 30
	copy(e[54:], []byte{0, 0, 0, 0xFC, 0, '2', '4', 'G', '4', '\n', ' ', ' '})
	got := parseEDID(e)
	if got == nil || got.maker != "AOC" || got.name != "24G4" || got.inches < 23.5 || got.inches > 24.5 {
		t.Fatalf("EDID: %+v", got)
	}
}

func TestShortNames(t *testing.T) {
	if s := shortCPU("Intel(R) Core(TM) i5-14400F"); s != "Intel Core i5-14400F" {
		t.Fatal(s)
	}
	if s := shortCPU("AMD Ryzen 7 7800X3D 8-Core Processor"); s != "AMD Ryzen 7 7800X3D" {
		t.Fatal(s)
	}
	if s := diskSize(2000398934016); s != "2 TB" {
		t.Fatal(s)
	}
}
