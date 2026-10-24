package totp

import (
	"encoding/base32"
	"testing"
	"time"
)

// Vectores de la RFC 6238 (apéndice B), con 8 dígitos.
func TestRFC6238(t *testing.T) {
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	seeds := map[string]string{
		"SHA1":   "12345678901234567890",
		"SHA256": "12345678901234567890123456789012",
		"SHA512": "1234567890123456789012345678901234567890123456789012345678901234",
	}
	cases := []struct {
		t    int64
		alg  string
		want string
	}{
		{59, "SHA1", "94287082"}, {59, "SHA256", "46119246"}, {59, "SHA512", "90693936"},
		{1111111109, "SHA1", "07081804"}, {1111111109, "SHA256", "68084774"}, {1111111109, "SHA512", "25091201"},
		{20000000000, "SHA1", "65353130"}, {20000000000, "SHA256", "77737706"}, {20000000000, "SHA512", "47863826"},
	}
	for _, c := range cases {
		uri := "otpauth://totp/Prueba:yo?secret=" + enc.EncodeToString([]byte(seeds[c.alg])) + "&digits=8&algorithm=" + c.alg
		k, err := Parse(uri)
		if err != nil {
			t.Fatal(err)
		}
		if got := k.CodeAt(time.Unix(c.t, 0)); got != c.want {
			t.Errorf("%s t=%d: %s, esperaba %s", c.alg, c.t, got, c.want)
		}
	}
}

func TestParse(t *testing.T) {
	k, err := Parse("jbsw y3dp ehpk 3pxp")
	if err != nil || k.Digits != 6 || k.Period != 30 {
		t.Fatalf("%v %+v", err, k)
	}
	k, err = Parse("otpauth://totp/GitHub:fer?secret=JBSWY3DPEHPK3PXP&issuer=GitHub")
	if err != nil || k.Issuer != "GitHub" || k.Account != "fer" {
		t.Fatalf("%v %+v", err, k)
	}
	for _, bad := range []string{"", "hola", "otpauth://hotp/x?secret=JBSWY3DPEHPK3PXP", "!!!!!!!!!!!!!!!!!!"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q debería fallar", bad)
		}
	}
	code, left := k.Code(time.Unix(65, 0))
	if len(code) != 6 || left != 25 {
		t.Fatalf("código %q, quedan %d", code, left)
	}
}
