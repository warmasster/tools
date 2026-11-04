package netmon

import (
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"turbomon/internal/etw"
)

// Con eventos de red de verdad (como administrador):
//
//	$env:TURBOMON_REAL="1"; go test -run RealNetwork -v ./internal/netmon/
func TestRealNetwork(t *testing.T) {
	if os.Getenv("TURBOMON_REAL") == "" {
		t.Skip("TURBOMON_REAL no está puesto")
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("hace falta ser administrador")
	}
	s, err := etw.Start("TurboMON-PruebaRed", etw.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	m := New()
	if err := s.Enable(Provider, 4, Keywords, nil, m.Handle); err != nil {
		t.Fatal(err)
	}
	time.Sleep(700 * time.Millisecond)
	resp, err := http.Get("https://speed.cloudflare.com/__down?bytes=3000000")
	if err != nil {
		t.Fatal(err)
	}
	n, _ := io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	time.Sleep(2 * time.Second)
	tot := m.Totals()[uint32(os.Getpid())]
	t.Logf("descargados %d bytes; los eventos dicen ↓ %d ↑ %d", n, tot[0], tot[1])
	if tot[0] < uint64(n)*8/10 {
		t.Fatalf("los eventos no cuadran con la descarga")
	}
	srv, ok := m.GameServer(uint32(os.Getpid()))
	t.Logf("servidor principal: %v %v", srv, ok)
}
