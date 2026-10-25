package totp

import "testing"

// Un parámetro de otpauth:// que viene pero no vale es un error (con el valor
// por defecto los códigos saldrían mal en silencio).
func TestParseRejectsBadParams(t *testing.T) {
	bad := []string{
		"otpauth://totp/x?secret=JBSWY3DPEHPK3PXP&period=5",
		"otpauth://totp/x?secret=JBSWY3DPEHPK3PXP&digits=10",
		"otpauth://totp/x?secret=JBSWY3DPEHPK3PXP&algorithm=MD5",
	}
	for _, s := range bad {
		if _, err := Parse(s); err == nil {
			t.Errorf("debería rechazar %s", s)
		}
	}
	k, err := Parse("otpauth://totp/x?secret=JBSWY3DPEHPK3PXP&algorithm=sha256&digits=8&period=60")
	if err != nil || k.Algorithm != "SHA256" || k.Digits != 8 || k.Period != 60 {
		t.Errorf("parámetros válidos: %+v %v", k, err)
	}
	if _, err := Parse("otpauth://totp/x?secret=JBSWY3DPEHPK3PXP"); err != nil {
		t.Errorf("sin parámetros vale con los de siempre: %v", err)
	}
}
