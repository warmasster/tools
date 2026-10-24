// Package totp calcula los códigos de verificación en dos pasos (RFC 6238), los
// mismos que Google Authenticator.
package totp

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Key es una clave TOTP ya interpretada.
type Key struct {
	Secret    []byte
	Digits    int
	Period    int
	Algorithm string // SHA1, SHA256, SHA512
	Issuer    string
	Account   string
}

var ErrInvalid = errors.New("no es una clave 2FA válida (pega la clave de texto o el enlace otpauth://)")

// Parse acepta la clave en base32 (con espacios o guiones) o un enlace
// otpauth://totp/...
func Parse(s string) (*Key, error) {
	s = strings.TrimSpace(s)
	k := &Key{Digits: 6, Period: 30, Algorithm: "SHA1"}
	secret := s
	if strings.HasPrefix(strings.ToLower(s), "otpauth://") {
		u, err := url.Parse(s)
		if err != nil || !strings.EqualFold(u.Host, "totp") {
			return nil, ErrInvalid
		}
		q := u.Query()
		secret = q.Get("secret")
		if d, err := strconv.Atoi(q.Get("digits")); err == nil && d >= 6 && d <= 8 {
			k.Digits = d
		}
		if p, err := strconv.Atoi(q.Get("period")); err == nil && p >= 10 && p <= 120 {
			k.Period = p
		}
		if a := strings.ToUpper(q.Get("algorithm")); a == "SHA256" || a == "SHA512" {
			k.Algorithm = a
		}
		label := strings.TrimPrefix(u.Path, "/")
		k.Issuer = q.Get("issuer")
		if i := strings.Index(label, ":"); i >= 0 {
			if k.Issuer == "" {
				k.Issuer = label[:i]
			}
			label = label[i+1:]
		}
		k.Account = strings.TrimSpace(label)
	}
	clean := strings.ToUpper(strings.NewReplacer(" ", "", "-", "", "=", "").Replace(secret))
	if len(clean) < 16 {
		return nil, ErrInvalid
	}
	b, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(clean)
	if err != nil || len(b) < 10 {
		return nil, ErrInvalid
	}
	k.Secret = b
	return k, nil
}

func (k *Key) hasher() func() hash.Hash {
	switch k.Algorithm {
	case "SHA256":
		return sha256.New
	case "SHA512":
		return sha512.New
	}
	return sha1.New
}

// CodeAt devuelve el código para ese momento.
func (k *Key) CodeAt(t time.Time) string {
	counter := uint64(t.Unix()) / uint64(k.Period)
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	m := hmac.New(k.hasher(), k.Secret)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	mod := uint32(1)
	for i := 0; i < k.Digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", k.Digits, v%mod)
}

// Code devuelve el código actual y los segundos que le quedan.
func (k *Key) Code(now time.Time) (string, int) {
	left := k.Period - int(now.Unix()%int64(k.Period))
	return k.CodeAt(now), left
}
