// Package card entiende los datos de una tarjeta de pago: deja el número en
// cifras, reconoce la marca, comprueba la cifra de control (Luhn) y pone la
// caducidad siempre igual (mes de dos cifras y año de cuatro).
package card

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// Digits deja solo las cifras (quita espacios, guiones y puntos).
func Digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Last4 son las cuatro últimas cifras del número.
func Last4(num string) string {
	n := Digits(num)
	if len(n) < 4 {
		return n
	}
	return n[len(n)-4:]
}

func prefix(n string, size int) int {
	if len(n) < size {
		return -1
	}
	v, _ := strconv.Atoi(n[:size])
	return v
}

func between(v, lo, hi int) bool { return v >= lo && v <= hi }

// Brand reconoce la marca por las primeras cifras ("" si no se sabe).
func Brand(num string) string {
	n := Digits(num)
	p1, p2, p3, p4, p6 := prefix(n, 1), prefix(n, 2), prefix(n, 3), prefix(n, 4), prefix(n, 6)
	switch {
	case p2 == 34 || p2 == 37:
		return "amex"
	case p1 == 4:
		return "visa"
	case between(p2, 51, 55) || between(p4, 2221, 2720):
		return "mastercard"
	case between(p3, 300, 305) || p2 == 36 || p2 == 38 || p2 == 39:
		return "diners"
	case between(p4, 3528, 3589):
		return "jcb"
	case p4 == 6011 || p2 == 65 || between(p3, 644, 649) || between(p6, 622126, 622925):
		return "discover"
	case p2 == 62:
		return "unionpay"
	case p2 == 50 || between(p2, 56, 69):
		return "maestro"
	}
	return ""
}

// BrandName es el nombre para enseñar.
func BrandName(b string) string {
	switch b {
	case "visa":
		return "Visa"
	case "mastercard":
		return "Mastercard"
	case "amex":
		return "American Express"
	case "diners":
		return "Diners Club"
	case "jcb":
		return "JCB"
	case "discover":
		return "Discover"
	case "unionpay":
		return "UnionPay"
	case "maestro":
		return "Maestro"
	}
	return "Tarjeta"
}

// Luhn comprueba la cifra de control que llevan todas las tarjetas de pago.
func Luhn(num string) bool {
	n := Digits(num)
	if len(n) < 12 {
		return false
	}
	sum := 0
	double := false
	for i := len(n) - 1; i >= 0; i-- {
		d := int(n[i] - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}

var (
	ErrMonth = errors.New("el mes de caducidad va del 1 al 12")
	ErrYear  = errors.New("el año de caducidad no es válido (por ejemplo, 28 o 2028)")
)

// Expiry pone la caducidad como "08" y "2028". Las dos vacías valen (sin
// caducidad); una sola, no.
func Expiry(month, year string) (string, string, error) {
	month, year = Digits(month), Digits(year)
	if month == "" && year == "" {
		return "", "", nil
	}
	m, err := strconv.Atoi(month)
	if err != nil || m < 1 || m > 12 {
		return "", "", ErrMonth
	}
	y, err := strconv.Atoi(year)
	if err != nil {
		return "", "", ErrYear
	}
	switch len(year) {
	case 2:
		y += 2000
	case 4:
	default:
		return "", "", ErrYear
	}
	if y < 2000 || y > 2099 {
		return "", "", ErrYear
	}
	return twoDigits(m), strconv.Itoa(y), nil
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// Expired dice si una tarjeta que caduca en ese mes ya no vale (vale hasta el
// último día del mes). Sin caducidad, no está caducada.
func Expired(month, year string, now time.Time) bool {
	m, err1 := strconv.Atoi(month)
	y, err2 := strconv.Atoi(year)
	if err1 != nil || err2 != nil {
		return false
	}
	return now.Year() > y || (now.Year() == y && int(now.Month()) > m)
}
