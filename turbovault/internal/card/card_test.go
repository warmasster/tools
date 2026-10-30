package card

import (
	"testing"
	"time"
)

func TestBrandAndLuhn(t *testing.T) {
	cases := []struct {
		num, brand string
		luhn       bool
	}{
		{"4242 4242 4242 4242", "visa", true},
		{"4000-0566-5566-5556", "visa", true},
		{"5555 5555 5555 4444", "mastercard", true},
		{"2223 0031 2200 3222", "mastercard", true},
		{"3782 822463 10005", "amex", true},
		{"3056 9300 0902 0004", "diners", true},
		{"3566 0020 2036 0505", "jcb", true},
		{"6011 1111 1111 1117", "discover", true},
		{"6200 0000 0000 0005", "unionpay", true},
		{"6759 6498 2643 8453", "maestro", true},
		{"4242 4242 4242 4241", "visa", false},
		{"1234", "", false},
	}
	for _, c := range cases {
		if b := Brand(c.num); b != c.brand {
			t.Errorf("Brand(%q) = %q, quería %q", c.num, b, c.brand)
		}
		if l := Luhn(c.num); l != c.luhn {
			t.Errorf("Luhn(%q) = %v", c.num, l)
		}
	}
	if Last4("4242 4242 4242 1234") != "1234" || Digits(" 42-42.4 ") != "42424" {
		t.Fatal("cifras")
	}
}

func TestExpiry(t *testing.T) {
	ok := []struct{ m, y, wm, wy string }{
		{"8", "28", "08", "2028"},
		{"08", "2028", "08", "2028"},
		{"12", "2031", "12", "2031"},
		{"", "", "", ""},
	}
	for _, c := range ok {
		m, y, err := Expiry(c.m, c.y)
		if err != nil || m != c.wm || y != c.wy {
			t.Errorf("Expiry(%q, %q) = %q %q %v", c.m, c.y, m, y, err)
		}
	}
	for _, c := range [][2]string{{"13", "28"}, {"0", "28"}, {"5", ""}, {"", "28"}, {"5", "202"}, {"5", "1999"}} {
		if _, _, err := Expiry(c[0], c[1]); err == nil {
			t.Errorf("Expiry(%q, %q) aceptada", c[0], c[1])
		}
	}
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.Local)
	if Expired("10", "2026", now) || !Expired("09", "2026", now) || !Expired("12", "2025", now) || Expired("", "", now) {
		t.Fatal("caducada")
	}
}
