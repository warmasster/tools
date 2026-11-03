package sensors

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"testing"
	"time"
)

// Lee los sensores de este equipo dos veces y comprueba que lo básico tiene sentido.
func TestSample(t *testing.T) {
	s := NewSampler()
	defer s.Close()
	time.Sleep(300 * time.Millisecond)
	s.Sample(true)
	time.Sleep(700 * time.Millisecond)
	start := time.Now()
	snap := s.Sample(true)
	cost := time.Since(start)
	b, _ := json.MarshalIndent(snap, "", "  ")
	t.Logf("lectura completa en %v:\n%s", cost, b)
	if snap.Mem.Total == 0 || snap.Mem.Used == 0 {
		t.Fatal("sin memoria")
	}
	if snap.CPU.Threads == 0 || snap.CPU.Usage < 0 || snap.CPU.Usage > 100 {
		t.Fatalf("procesador raro: %+v", snap.CPU)
	}
	light := time.Now()
	s.Sample(false)
	t.Logf("lectura ligera en %v", time.Since(light))
}

func TestParseHWiNFO(t *testing.T) {
	const off, size = 64, 316
	mem := make([]byte, off+size*3)
	binary.LittleEndian.PutUint32(mem[0:], 0x53695748)
	binary.LittleEndian.PutUint32(mem[32:], off)
	binary.LittleEndian.PutUint32(mem[36:], size)
	binary.LittleEndian.PutUint32(mem[40:], 3)
	put := func(i int, typ uint32, label string, v float64) {
		o := off + i*size
		binary.LittleEndian.PutUint32(mem[o:], typ)
		copy(mem[o+12:], label)
		binary.LittleEndian.PutUint64(mem[o+284:], math.Float64bits(v))
	}
	put(0, 1, "Core Max", 71)
	put(1, 1, "CPU Package", 68.5)
	put(2, 5, "CPU Package Power", 87.2)
	r, ok := parseHWiNFO(mem)
	if !ok || r.cpuTemp != 68.5 || r.cpuPower != 87.2 {
		t.Fatalf("lectura de HWiNFO: %+v %v", r, ok)
	}
}

func TestLuidOf(t *testing.T) {
	got := luidOf("pid_1234_luid_0x00000000_0x0000D1F3_phys_0_eng_0_engtype_3D")
	a := Adapter{LuidLow: 0xd1f3}
	if got != a.luidKey() {
		t.Fatalf("%q != %q", got, a.luidKey())
	}
}
