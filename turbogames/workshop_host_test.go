package main

import (
	"regexp"
	"testing"
	"time"
)

// La conexión con Steam (el proceso del Workshop, aquí con el Steam de mentira)
// se cierra sola al poco de la última petición, pero no en mitad de un cambio
// de mods; y lo que Steam dé por abierto justo después sigue contando como
// nuestro hasta que Steam lo suelta.
func TestWorkshopLinger(t *testing.T) {
	t.Setenv("TURBOGAMES_FAKE_WS", "1")
	a := &App{}
	h := &a.ws
	closed := make(chan struct{})
	h.mark(7, true)
	p, err := startWorkshop(t.TempDir(), 7, func() { h.mark(7, false); close(closed) })
	if err != nil {
		t.Fatal(err)
	}
	defer a.stopWorkshop()
	h.mu.Lock()
	h.p = p
	h.busyLocked()
	var ok bool
	if err := a.wsSendLocked(wsRequest{Op: "ping"}, 5*time.Second, &ok, wsOpts{}); err != nil || !ok {
		h.mu.Unlock()
		t.Fatalf("ping: %v %v", err, ok)
	}
	h.lingerLocked()
	h.mu.Unlock()
	if !h.ownRunning(7) {
		t.Fatal("abierto y no cuenta como nuestro")
	}

	// con un cambio de mods en marcha no se cierra
	release := a.wsKeep()
	select {
	case <-closed:
		t.Fatal("se ha cerrado con un cambio en marcha")
	case <-time.After(wsLinger + 700*time.Millisecond):
	}
	start := time.Now()
	release()
	select {
	case <-closed:
	case <-time.After(wsLinger + 4*time.Second):
		t.Fatal("no se ha cerrado solo")
	}
	if took := time.Since(start); took < wsLinger-200*time.Millisecond {
		t.Fatalf("se ha cerrado antes de tiempo: %v", took)
	}
	h.mu.Lock()
	gone := h.p == nil
	h.mu.Unlock()
	if !gone {
		t.Fatal("sigue apuntado como abierto")
	}

	// Steam tarda un poco en darse cuenta: mientras, sigue siendo nuestro
	if !h.ownRunning(7) {
		t.Fatal("recién cerrado debería contar como nuestro")
	}
	h.settle(7) // Steam aún lo ve
	if !h.ownRunning(7) {
		t.Fatal("se ha olvidado antes de que Steam lo suelte")
	}
	h.settle(0) // Steam ya lo ha soltado: si vuelve a verlo, es el juego de verdad
	if h.ownRunning(7) {
		t.Fatal("tras soltarlo Steam, ya no es nuestro")
	}
}

// Lo que se pide de paso no conecta por su cuenta.
func TestWorkshopPeekDoesNotConnect(t *testing.T) {
	a := &App{settings: defaultSettings()}
	a.locate()
	if a.steam() == nil {
		t.Skip("sin Steam")
	}
	if err := a.wsPeek(7, wsRequest{Op: "ping"}, time.Second, nil); err != errNotLinked {
		t.Fatalf("debería decir que no hay conexión: %v", err)
	}
	a.ws.mu.Lock()
	defer a.ws.mu.Unlock()
	if a.ws.p != nil {
		t.Fatal("ha conectado")
	}
}

// La interfaz reconoce el aviso de juego abierto por su texto.
func TestGameOpenMessage(t *testing.T) {
	re := regexp.MustCompile(`(?i)^Tienes .+ abierto: ciérralo`)
	for _, e := range []*errGameOpen{{AppID: 1, Name: "RimWorld"}, {AppID: 1, Name: "RimWorld", For: "Arma 3"}} {
		if !re.MatchString(e.Error()) {
			t.Fatalf("la interfaz no lo reconocería: %q", e.Error())
		}
	}
}
