package parts

import "testing"

// Tabla SMBIOS de juguete con un módulo DDR4 a 2133 que es de 3200.
func TestParseDIMMs(t *testing.T) {
	f := make([]byte, 0x28)
	f[0], f[1] = 17, byte(len(f))
	f[0x0C], f[0x0D] = 0x00, 0x40 // 16384 MB
	f[0x10], f[0x11] = 1, 2       // cadenas
	f[0x12] = 0x1A                // DDR4
	f[0x15], f[0x16] = 0x60, 0x09 // 2400
	f[0x17], f[0x1A] = 3, 4
	f[0x20], f[0x21] = 0x55, 0x08 // 2133
	data := append(f, []byte("DDR4-A2\x00BANK 0\x00Kingston\x00KF3200C16D4/16GX\x00\x00")...)
	data = append(data, 127, 4, 0, 0, 0, 0)
	ds := dimmsFrom(parseSMBIOS(data))
	if len(ds) != 1 {
		t.Fatalf("%d módulos", len(ds))
	}
	d := ds[0]
	if d.SizeMB != 16384 || d.Type != "DDR4" || d.Speed != 2400 || d.Configured != 2133 || d.Rated != 3200 || d.Locator != "DDR4-A2" || d.Part != "KF3200C16D4/16GX" {
		t.Fatalf("módulo mal leído: %+v", d)
	}
}
