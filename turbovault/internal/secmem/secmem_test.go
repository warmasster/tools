package secmem

import (
	"bytes"
	"testing"
)

func TestKeyRoundTrip(t *testing.T) {
	plain := []byte("0123456789abcdef0123456789abcdef")
	want := append([]byte(nil), plain...)
	k, err := New(plain)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain, make([]byte, len(plain))) {
		t.Fatal("New debe borrar el original")
	}
	// Mientras no se usa, la memoria no contiene la clave en claro.
	if bytes.Equal(k.buf[:k.n], want) {
		t.Fatal("la clave está en claro en memoria")
	}
	err = k.Use(func(b []byte) error {
		if !bytes.Equal(b, want) {
			t.Fatalf("clave distinta: %x", b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(k.buf[:k.n], want) {
		t.Fatal("la clave se quedó en claro después de usarla")
	}
	k.Destroy()
	if err := k.Use(func([]byte) error { return nil }); err != ErrDestroyed {
		t.Fatalf("después de Destroy: %v", err)
	}
}

func TestOddLength(t *testing.T) {
	k, err := New([]byte("corta"))
	if err != nil {
		t.Fatal(err)
	}
	defer k.Destroy()
	k.Use(func(b []byte) error {
		if string(b) != "corta" {
			t.Fatalf("got %q", b)
		}
		return nil
	})
}
