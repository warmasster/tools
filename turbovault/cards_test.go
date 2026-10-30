package main

import (
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"turbovault/internal/vault"
)

func saveCard(t *testing.T, a *Agent, title string, c vault.Card) vault.Item {
	t.Helper()
	var it vault.Item
	must(t, a, app(), "save", map[string]any{"item": vault.Item{Type: vault.TypeCard, Title: title}, "secret": vault.Secret{Card: &c}}, &it)
	return it
}

// fakeCardWindow cuenta las veces que se abriría la ventana para confirmar.
func fakeCardWindow(t *testing.T) *atomic.Int32 {
	var n atomic.Int32
	prev := cardWindow
	cardWindow = func(*Agent) error { n.Add(1); return nil }
	t.Cleanup(func() { cardWindow = prev })
	return &n
}

// waitCardReq espera a que la ventana vea una tarjeta pendiente.
func waitCardReq(t *testing.T, a *Agent) *CardInfo {
	t.Helper()
	for i := 0; i < 200; i++ {
		var st State
		must(t, a, app(), "state", nil, &st)
		if st.CardReq != nil {
			return st.CardReq
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("la ventana no ve ninguna tarjeta esperando")
	return nil
}

func oldMaster(a *Agent) {
	a.mu.Lock()
	a.masterAt = time.Now().Add(-2 * time.Hour)
	a.mu.Unlock()
}

type fillResult struct {
	data map[string]string
	err  error
}

func fillAsync(t *testing.T, a *Agent, cs *connState, id string) chan fillResult {
	ch := make(chan fillResult, 1)
	go func() {
		raw, err := do(t, a, cs, "fillCard", map[string]string{"id": id, "url": "https://tienda.es/pagar", "frame": "https://js.stripe.com/v3/"})
		var m map[string]string
		if err == nil {
			json.Unmarshal(raw, &m)
		}
		ch <- fillResult{m, err}
	}()
	return ch
}

func TestCardSaveRules(t *testing.T) {
	a := newTestAgent(t)
	must(t, a, app(), "create", map[string]string{"master": testMaster}, nil)
	bad := []vault.Card{
		{Number: "4242 4242"},
		{Number: "4242424242424242", ExpMonth: "13", ExpYear: "28"},
		{Number: "4242424242424242", ExpMonth: "08"},
		{Number: "4242424242424242", CVC: "12"},
	}
	for _, c := range bad {
		if _, err := do(t, a, app(), "save", map[string]any{"item": vault.Item{Type: vault.TypeCard}, "secret": vault.Secret{Card: &c}}); code(err) != "invalid" {
			t.Fatalf("tarjeta mala aceptada %+v: %v", c, err)
		}
	}
	it := saveCard(t, a, "", vault.Card{Holder: "  Fernando   García ", Number: "4242 4242-4242 4242", ExpMonth: "8", ExpYear: "28", CVC: "123", PIN: "9876"})
	if it.Title != "Visa •••• 4242" || it.Brand != "visa" || it.Exp != "2028-08" || len(it.URLs) != 0 {
		t.Fatalf("tarjeta guardada: %+v", it)
	}
	var d struct {
		Secret vault.Secret `json:"secret"`
	}
	must(t, a, app(), "get", map[string]string{"id": it.ID}, &d)
	c := d.Secret.Card
	if c == nil || c.Number != "4242424242424242" || c.Holder != "Fernando García" || c.ExpMonth != "08" || c.ExpYear != "2028" {
		t.Fatalf("datos guardados: %+v", c)
	}
	// Copiar desde la ventana.
	must(t, a, app(), "copy", map[string]string{"id": it.ID, "field": "card:number"}, nil)
	clipMu.Lock()
	got := clipText
	clipMu.Unlock()
	if got != "4242424242424242" {
		t.Fatalf("copiado: %q", got)
	}
	must(t, a, app(), "copy", map[string]string{"id": it.ID, "field": "card:exp"}, nil)
	clipMu.Lock()
	got = clipText
	clipMu.Unlock()
	if got != "08/28" {
		t.Fatalf("caducidad copiada: %q", got)
	}
	// El CSV de contraseñas no lleva tarjetas.
	saveLogin(t, a, "GitHub", "https://github.com", "fer", "pw-github-123456")
	var n int
	must(t, a, app(), "export", map[string]string{"path": t.TempDir() + "/x.csv", "master": testMaster}, &n)
	if n != 1 {
		t.Fatalf("exportadas %d (las tarjetas no se exportan)", n)
	}
}

func TestCardFillFromBrowser(t *testing.T) {
	a := newTestAgent(t)
	opened := fakeCardWindow(t)
	must(t, a, app(), "create", map[string]string{"master": testMaster}, nil)
	visa := saveCard(t, a, "Visa del banco", vault.Card{Holder: "FERNANDO", Number: "4242424242424242", ExpMonth: "08", ExpYear: "2028", CVC: "123", PIN: "9876"})
	saveCard(t, a, "Vieja", vault.Card{Number: "5555555555554444", ExpMonth: "01", ExpYear: "2020"})
	gh := saveLogin(t, a, "GitHub", "https://github.com", "fer", "pw-github-123456")

	e := ext()
	if _, err := do(t, a, e, "cards", nil); code(err) != "unpaired" {
		t.Fatalf("sin vincular: %v", err)
	}
	pairExt(t, a, e)
	var st State
	must(t, a, e, "state", nil, &st)
	if !st.Cards || !st.CardAsk || !st.CardMask {
		t.Fatalf("ajustes de tarjetas para la extensión: %+v", st)
	}

	// La lista: sin el número, la caducada al final.
	raw, err := do(t, a, e, "cards", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "424242424242") || strings.Contains(string(raw), "FERNANDO") {
		t.Fatalf("la lista enseña datos de la tarjeta: %s", raw)
	}
	var list []CardItem
	json.Unmarshal(raw, &list)
	if len(list) != 2 || list[0].ID != visa.ID || list[0].Exp != "08/28" || list[0].Expired || !list[1].Expired {
		t.Fatalf("lista: %+v", list)
	}

	// Recién creada la bóveda (contraseña maestra escrita hace nada): no la vuelve a pedir.
	r := <-fillAsync(t, a, e, visa.ID)
	if r.err != nil || r.data["number"] != "4242424242424242" || r.data["cvc"] != "123" || r.data["expYear"] != "2028" || r.data["holder"] != "FERNANDO" {
		t.Fatalf("relleno: %v %v", r.data, r.err)
	}
	if _, ok := r.data["pin"]; ok || strings.Contains(strings.Join(mapValues(r.data), " "), "9876") {
		t.Fatal("el PIN sale hacia el navegador")
	}
	if opened.Load() != 0 {
		t.Fatal("ha pedido la contraseña maestra recién escrita")
	}

	// Un rato después, sí: espera a que se confirme en la ventana.
	oldMaster(a)
	ch := fillAsync(t, a, e, visa.ID)
	req := waitCardReq(t, a)
	if opened.Load() != 1 || req.Card != "Visa •••• 4242" || req.Host != "tienda.es" || req.Browser != "Chrome" {
		t.Fatalf("petición: %+v (ventanas %d)", req, opened.Load())
	}
	must(t, a, e, "state", nil, &st)
	if st.CardReq != nil {
		t.Fatal("el navegador ve la petición de la ventana")
	}
	select {
	case r := <-ch:
		t.Fatalf("ha rellenado sin confirmar: %v", r)
	case <-time.After(100 * time.Millisecond):
	}
	if _, err := do(t, a, app(), "approveCard", map[string]string{"nonce": req.Nonce, "master": "no-es-la-maestra-123"}); code(err) != "wrong" {
		t.Fatalf("maestra mala: %v", err)
	}
	if _, err := do(t, a, app(), "approveCard", map[string]string{"nonce": "otra", "master": testMaster}); code(err) != "notfound" {
		t.Fatalf("otra petición: %v", err)
	}
	must(t, a, app(), "approveCard", map[string]string{"nonce": req.Nonce, "master": testMaster}, nil)
	r = <-ch
	if r.err != nil || r.data["number"] != "4242424242424242" {
		t.Fatalf("tras confirmar: %v %v", r.data, r.err)
	}
	if getSettings().Fails != 0 {
		t.Fatal("el fallo no se ha borrado al acertar")
	}

	// Cancelar en la ventana.
	oldMaster(a)
	ch = fillAsync(t, a, e, visa.ID)
	req = waitCardReq(t, a)
	must(t, a, app(), "rejectCard", map[string]string{"nonce": req.Nonce}, nil)
	if r := <-ch; code(r.err) != "cancelled" {
		t.Fatalf("cancelada: %v", r.err)
	}

	// Una entrada que no es tarjeta, no.
	if _, err := do(t, a, e, "fillCard", map[string]string{"id": gh.ID, "url": "https://tienda.es"}); code(err) != "denied" {
		t.Fatalf("inicio de sesión como tarjeta: %v", err)
	}

	// La actividad lo apunta con la web y el marco de pago.
	var evs []vault.Event
	must(t, a, app(), "events", nil, &evs)
	okEv := false
	for _, ev := range evs {
		if ev.Kind == "fillCard" && ev.Origin == "https://tienda.es" && strings.Contains(ev.Title, "js.stripe.com") {
			okEv = true
		}
	}
	if !okEv {
		t.Fatalf("actividad sin el relleno de la tarjeta: %+v", evs)
	}

	// Sin pedir la contraseña maestra (ajuste apagado).
	must(t, a, app(), "setSettings", map[string]bool{"cardAsk": false}, nil)
	oldMaster(a)
	before := opened.Load()
	if r := <-fillAsync(t, a, e, visa.ID); r.err != nil || opened.Load() != before {
		t.Fatalf("sin pedirla: %v (ventanas %d → %d)", r.err, before, opened.Load())
	}

	// Desactivado del todo.
	must(t, a, app(), "setSettings", map[string]bool{"cards": false}, nil)
	must(t, a, e, "state", nil, &st)
	if st.Cards {
		t.Fatal("la extensión no se entera de que se desactivó")
	}
	if _, err := do(t, a, e, "cards", nil); code(err) != "off" {
		t.Fatalf("lista desactivada: %v", err)
	}
	if _, err := do(t, a, e, "fillCard", map[string]string{"id": visa.ID, "url": "https://tienda.es"}); code(err) != "off" {
		t.Fatalf("relleno desactivado: %v", err)
	}
}

func TestCardWaitEndsOnLock(t *testing.T) {
	a := newTestAgent(t)
	fakeCardWindow(t)
	must(t, a, app(), "create", map[string]string{"master": testMaster}, nil)
	visa := saveCard(t, a, "Visa", vault.Card{Number: "4242424242424242"})
	e := ext()
	pairExt(t, a, e)
	oldMaster(a)
	ch := fillAsync(t, a, e, visa.ID)
	req := waitCardReq(t, a)
	a.lockNow("windows")
	if r := <-ch; code(r.err) != "locked" {
		t.Fatalf("al cerrar mientras espera: %v", r.err)
	}
	// Al volver a abrir no queda nada pendiente, y la confirmación vieja ya no vale.
	must(t, a, app(), "unlock", map[string]string{"master": testMaster}, nil)
	var st State
	must(t, a, app(), "state", nil, &st)
	if st.CardReq != nil {
		t.Fatal("queda una tarjeta pendiente tras cerrar")
	}
	if _, err := do(t, a, app(), "approveCard", map[string]string{"nonce": req.Nonce, "master": testMaster}); code(err) != "notfound" {
		t.Fatalf("confirmación vieja: %v", err)
	}
	// Una nueva sustituye a la anterior: la primera se cancela.
	oldMaster(a)
	ch1 := fillAsync(t, a, e, visa.ID)
	r1 := waitCardReq(t, a)
	ch2 := fillAsync(t, a, e, visa.ID)
	for i := 0; i < 200; i++ {
		if r := waitCardReq(t, a); r.Nonce != r1.Nonce {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if r := <-ch1; code(r.err) != "cancelled" {
		t.Fatalf("la primera: %v", r.err)
	}
	must(t, a, app(), "approveCard", map[string]string{"nonce": waitCardReq(t, a).Nonce, "master": testMaster}, nil)
	if r := <-ch2; r.err != nil {
		t.Fatalf("la segunda: %v", r.err)
	}
}

func mapValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}
