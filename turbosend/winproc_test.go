package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// La firma se lee del certificado del firmante validado, no de cualquiera de los
// que lleva el archivo: con Edge (firmado por Microsoft) sale Microsoft y no pasa.
func TestVerifiedSigner(t *testing.T) {
	edge := filepath.Join(os.Getenv("ProgramFiles(x86)"), `Microsoft\Edge\Application\msedge.exe`)
	if _, err := os.Stat(edge); err != nil {
		t.Skip("no está Edge")
	}
	name, err := verifiedSigner(edge)
	if err != nil {
		t.Fatalf("Edge: %v", err)
	}
	if !strings.HasPrefix(name, "Microsoft") {
		t.Errorf("firmante de Edge = %q", name)
	}
	if verifyCloudflareSignature(edge) == nil {
		t.Error("Edge no debería pasar por cloudflared")
	}
	// Sin firma (el propio ejecutable de la prueba).
	self, _ := os.Executable()
	if _, err := verifiedSigner(self); err == nil {
		t.Error("un ejecutable sin firma no debería validarse")
	}
	if p := os.Getenv("TURBOSEND_CLOUDFLARED"); p != "" {
		if err := verifyCloudflareSignature(p); err != nil {
			t.Errorf("cloudflared: %v", err)
		}
	}
}
