package main

// Tarjetas: la extensión puede rellenar una tarjeta guardada en los campos de
// pago de una web (se activa o desactiva en Ajustes).
//
//   - La extensión solo ve la lista con la marca, las cuatro últimas cifras y la
//     caducidad. El número, el CVC y el titular se piden al rellenar.
//   - Antes de darlos, TurboVAULT abre su ventana para que escribas la
//     contraseña maestra (nunca en la página). Si la acabas de escribir (al abrir
//     la bóveda o al confirmar otra tarjeta), durante un minuto no la vuelve a
//     pedir. Este paso se puede desactivar en Ajustes.
//   - El PIN nunca sale hacia el navegador.

import (
	"sort"
	"strings"
	"time"

	"turbovault/internal/card"
	"turbovault/internal/match"
	"turbovault/internal/vault"
)

const cardWait = 2 * time.Minute // lo que se espera a que escribas la contraseña maestra

// cardGrace: tras escribir bien la contraseña maestra, durante este rato no se
// vuelve a pedir para rellenar una tarjeta.
func cardGrace() time.Duration {
	if d, err := time.ParseDuration(testEnv("TURBOVAULT_CARD_GRACE")); err == nil {
		return d
	}
	return time.Minute
}

// cardWindow abre la ventana de TurboVAULT para confirmar (las pruebas la cambian).
var cardWindow = func(a *Agent) error { return a.openWindow("card") }

var errCardsOff = errCode("off", "El relleno de tarjetas está desactivado (TurboVAULT → Ajustes → Tarjetas).")

// CardInfo es lo que ve la ventana de una tarjeta que espera confirmación.
type CardInfo struct {
	Nonce   string `json:"nonce"`
	Title   string `json:"title"`
	Card    string `json:"card"` // "Visa •••• 4242"
	Brand   string `json:"brand"`
	Host    string `json:"host"`
	Browser string `json:"browser"`
}

type cardRequest struct {
	info    CardInfo
	created time.Time
	done    chan bool // un solo envío: quien la quita de a.cardReq
}

// CardItem es lo que ve la extensión de una tarjeta: nunca el número entero.
type CardItem struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Brand   string `json:"brand"`
	Last4   string `json:"last4"`
	Exp     string `json:"exp,omitempty"` // "08/28"
	Expired bool   `json:"expired,omitempty"`
}

func cardLabel(it vault.Item) string {
	s := card.BrandName(it.Brand)
	if it.Last4 != "" {
		s += " •••• " + it.Last4
	}
	return s
}

// shortExp pasa "2028-08" a "08/28".
func shortExp(exp string) string {
	y, m, ok := strings.Cut(exp, "-")
	if !ok || len(y) != 4 {
		return ""
	}
	return m + "/" + y[2:]
}

// cleanCard valida y deja bien escritos los datos de una tarjeta antes de guardarla.
func cleanCard(it *vault.Item, sec *vault.Secret) error {
	it.URLs, it.Username = nil, ""
	if sec == nil {
		return nil // solo cambia el título, la favorita…
	}
	sec.Password, sec.TOTP, sec.History = "", "", nil
	if sec.Card == nil {
		return errCode("invalid", "Faltan los datos de la tarjeta.")
	}
	c := *sec.Card
	c.Number = card.Digits(c.Number)
	if len(c.Number) < 12 || len(c.Number) > 19 {
		return errCode("invalid", "El número de la tarjeta debe tener entre 12 y 19 cifras.")
	}
	m, y, err := card.Expiry(c.ExpMonth, c.ExpYear)
	if err != nil {
		return errCode("invalid", "Caducidad: "+err.Error()+".")
	}
	c.ExpMonth, c.ExpYear = m, y
	c.CVC = card.Digits(c.CVC)
	if n := len(c.CVC); n != 0 && n != 3 && n != 4 {
		return errCode("invalid", "El código de seguridad (CVC) tiene 3 o 4 cifras.")
	}
	c.Holder = strings.Join(strings.Fields(c.Holder), " ")
	c.PIN = strings.TrimSpace(c.PIN)
	sec.Card = &c
	if it.Title == "" {
		it.Title = card.BrandName(card.Brand(c.Number)) + " •••• " + card.Last4(c.Number)
	}
	return nil
}

// cardList son las tarjetas para el menú de la extensión: las favoritas y las
// usadas hace poco primero, las caducadas al final.
func (a *Agent) cardList() ([]CardItem, error) {
	if !getSettings().Cards {
		return nil, errCardsOff
	}
	items, err := a.v.Items(false)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	type row struct {
		CardItem
		fav  bool
		used int64
	}
	var rows []row
	for _, it := range items {
		if it.Type != vault.TypeCard {
			continue
		}
		c := CardItem{ID: it.ID, Title: it.Title, Brand: it.Brand, Last4: it.Last4, Exp: shortExp(it.Exp)}
		if y, m, ok := strings.Cut(it.Exp, "-"); ok {
			c.Expired = card.Expired(m, y, now)
		}
		rows = append(rows, row{c, it.Fav, it.Used})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Expired != b.Expired {
			return !a.Expired
		}
		if a.fav != b.fav {
			return a.fav
		}
		if a.used != b.used {
			return a.used > b.used
		}
		return strings.ToLower(a.Title) < strings.ToLower(b.Title)
	})
	out := make([]CardItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.CardItem)
	}
	return out, nil
}

func (a *Agent) recentMaster() bool {
	a.mu.Lock()
	t := a.masterAt
	a.mu.Unlock()
	return !t.IsZero() && time.Since(t) < cardGrace()
}

// fillCard da los datos de una tarjeta a la extensión para rellenarlos.
func (a *Agent) fillCard(cs *connState, p extArgs) (any, error) {
	s := getSettings()
	if !s.Cards {
		return nil, errCardsOff
	}
	origin := match.Origin(p.URL)
	if origin == "" {
		return nil, errCode("denied", "Solo se rellena en webs http o https.")
	}
	it, _, err := a.v.Get(p.ID)
	if err != nil {
		return nil, err
	}
	if it.Deleted != 0 || it.Type != vault.TypeCard {
		return nil, errCode("denied", "Esa entrada no es una tarjeta.")
	}
	if err := a.secretGate(cs); err != nil {
		return nil, err
	}
	host := match.Host(p.URL)
	if s.CardAsk && !a.recentMaster() {
		if err := a.confirmCard(cs, it, host); err != nil {
			return nil, err
		}
	}
	// Otra vez: mientras se esperaba ha podido cerrarse la bóveda o cambiar la tarjeta.
	it, sec, err := a.v.Get(p.ID)
	if err != nil {
		return nil, err
	}
	c := sec.Card
	if it.Deleted != 0 || it.Type != vault.TypeCard || c == nil || c.Number == "" {
		return nil, errCode("invalid", "Esa tarjeta ya no está en la bóveda.")
	}
	a.v.Touch(it.ID)
	title := it.Title
	if fh := match.Host(p.Frame); fh != "" && fh != host {
		title += " (pago en " + fh + ")"
	}
	a.event(vault.Event{Kind: "fillCard", Item: it.ID, Title: title, Origin: origin, Src: cs.peer.Browser})
	if s.NotifyFill && a.tray != nil {
		a.tray.notify("Tarjeta rellenada", cardLabel(it)+" en "+host)
	}
	return map[string]string{
		"holder": c.Holder, "number": c.Number, "expMonth": c.ExpMonth, "expYear": c.ExpYear, "cvc": c.CVC,
		"brand": it.Brand, "last4": it.Last4,
	}, nil
}

// confirmCard abre la ventana de TurboVAULT y espera a que se escriba la
// contraseña maestra (o se cancele, se cierre la bóveda o pase el tiempo).
func (a *Agent) confirmCard(cs *connState, it vault.Item, host string) error {
	req := &cardRequest{
		info:    CardInfo{Nonce: vault.NewID(), Title: it.Title, Card: cardLabel(it), Brand: it.Brand, Host: host, Browser: cs.peer.Browser},
		created: time.Now(),
		done:    make(chan bool, 1),
	}
	a.mu.Lock()
	old := a.cardReq
	a.cardReq = req
	a.mu.Unlock()
	if old != nil {
		old.done <- false // solo vale la última que se ha pedido
	}
	a.broadcast()
	if err := cardWindow(a); err != nil {
		a.finishCard(req, false)
		return errCode("error", "No se pudo abrir TurboVAULT para confirmar con la contraseña maestra.")
	}
	t := time.NewTimer(cardWait)
	defer t.Stop()
	select {
	case ok := <-req.done:
		if !ok {
			if !a.v.Unlocked() {
				return errLocked
			}
			return errCode("cancelled", "No se ha confirmado en TurboVAULT: la tarjeta no se ha rellenado.")
		}
		return nil
	case <-t.C:
		a.finishCard(req, false)
		return errCode("timeout", "No se confirmó a tiempo en TurboVAULT: la tarjeta no se ha rellenado.")
	}
}

// finishCard quita la petición (si sigue siendo la actual) y avisa a quien espera.
func (a *Agent) finishCard(req *cardRequest, ok bool) bool {
	a.mu.Lock()
	cur := a.cardReq == req
	if cur {
		a.cardReq = nil
		if ok {
			a.masterAt = time.Now()
		}
	}
	a.mu.Unlock()
	if !cur {
		return false
	}
	req.done <- ok
	a.broadcast()
	return true
}

// cancelCard cancela la tarjeta que esperaba (si había una).
func (a *Agent) cancelCard() {
	a.mu.Lock()
	req := a.cardReq
	a.mu.Unlock()
	if req != nil {
		a.finishCard(req, false)
	}
}

// answerCard es la respuesta de la ventana: con la contraseña maestra, se
// rellena; sin ella (cancelar), no.
func (a *Agent) answerCard(nonce string, approve bool, master string) error {
	a.mu.Lock()
	req := a.cardReq
	a.mu.Unlock()
	if req == nil || req.info.Nonce != nonce || time.Since(req.created) > cardWait {
		return errCode("notfound", "Ya no hay ninguna tarjeta esperando confirmación.")
	}
	if !approve {
		a.finishCard(req, false)
		return nil
	}
	if err := a.masterTry(func() error { return a.v.VerifyMaster(master) }); err != nil {
		return err
	}
	// Como al abrir la bóveda: acertar borra los fallos de antes.
	updateSettings(func(s *Settings) { s.Fails, s.WaitUntil = 0, 0 })
	if !a.finishCard(req, true) {
		return errCode("notfound", "Ya no hay ninguna tarjeta esperando confirmación.")
	}
	return nil
}
