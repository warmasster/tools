package parts

import (
	"os"
	"testing"
)

// $env:TURBOMON_DISKTEST="1"; go test -run SpeedTest -v ./internal/parts/
func TestSpeedTest(t *testing.T) {
	if os.Getenv("TURBOMON_DISKTEST") == "" {
		t.Skip("TURBOMON_DISKTEST no está puesto")
	}
	r, err := SpeedTest(os.Getenv("SystemDrive"), 256, func(string, float64) {})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("lee %.0f MB/s, escribe %.0f MB/s", r.Read, r.Write)
}
