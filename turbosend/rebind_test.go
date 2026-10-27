package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// Sin red local, UPnP ni directo forzado escucha solo en este PC; al activar algo
// de eso pasa a todas las redes sin reiniciar, y sigue contestando.
func TestRebind(t *testing.T) {
	a, _, _ := testApp(t)
	p := newPublicServer(a)
	port, err := p.Start(48731, false)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	// Sin conexiones reutilizadas: cada ping tiene que entrar por el punto de escucha actual.
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	ping := func() string {
		resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/ping", port))
		if err != nil {
			return "error: " + err.Error()
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	if !strings.HasPrefix(p.ln.Addr().String(), "127.0.0.1:") || ping() != p.nonce {
		t.Fatalf("solo este PC: %s, ping %q", p.ln.Addr(), ping())
	}
	// Escuchar en todas las redes haría saltar el aviso del firewall de Windows con
	// cada ejecutable de prueba: solo en GitHub Actions.
	if os.Getenv("GITHUB_ACTIONS") == "" {
		if err := p.Rebind(false); err != nil || ping() != p.nonce {
			t.Fatalf("volver a abrir el punto de escucha: %v", err)
		}
		return
	}
	if err := p.Rebind(true); err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(p.ln.Addr().String(), "127.0.0.1:") || ping() != p.nonce {
		t.Fatalf("todas las redes: %s, ping %q", p.ln.Addr(), ping())
	}
	if err := p.Rebind(false); err != nil || ping() != p.nonce {
		t.Fatalf("vuelta a solo este PC: %v", err)
	}
}
