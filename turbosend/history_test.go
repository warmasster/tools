package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
)

// Quien entra recibe los últimos mensajes y pide los anteriores por páginas; el
// identificador del invitado va en la cabecera X-Member.
func TestGuestHistory(t *testing.T) {
	a, room, _ := testApp(t)
	for i := 0; i < 400; i++ {
		m := newMessage("host", "text")
		m.Text = fmt.Sprintf("mensaje %d", i)
		a.rooms.Post(room, m, false)
	}
	guest, _ := a.roomView(room, true, "")
	host, _ := a.roomView(room, false, "")
	if len(guest.Messages) != guestPage || !guest.Older {
		t.Fatalf("el invitado recibe %d mensajes (older=%v)", len(guest.Messages), guest.Older)
	}
	if len(host.Messages) < 400 || host.Older {
		t.Fatalf("la ventana de TurboSEND tiene que tenerlos todos: %d", len(host.Messages))
	}
	p := newPublicServer(a)
	page := func(before string) (msgs []Message, older bool, code int) {
		req := httptest.NewRequest("GET", "/api/r/"+room+"/history?before="+before, nil)
		req.SetPathValue("room", room)
		req.Header.Set("X-Member", "0123456789abcdef0123")
		rr := httptest.NewRecorder()
		p.handleHistory(rr, req)
		var out struct {
			Messages []Message `json:"messages"`
			Older    bool      `json:"older"`
		}
		json.Unmarshal(rr.Body.Bytes(), &out)
		return out.Messages, out.Older, rr.Code
	}
	got, older, code := page(guest.Messages[0].ID)
	if code != 200 || len(got) != guestPage || !older {
		t.Fatalf("primera página: %d mensajes, older=%v, HTTP %d", len(got), older, code)
	}
	if got[len(got)-1].ID == guest.Messages[0].ID {
		t.Fatal("la página no debe repetir el mensaje desde el que se pide")
	}
	total := len(guest.Messages) + len(got)
	for older {
		got, older, _ = page(got[0].ID)
		total += len(got)
	}
	if total != len(host.Messages) {
		t.Fatalf("paginando salen %d de %d mensajes", total, len(host.Messages))
	}
	// Sin el identificador, nada.
	req := httptest.NewRequest("GET", "/api/r/"+room+"/history?before=x", nil)
	req.SetPathValue("room", room)
	rr := httptest.NewRecorder()
	p.handleHistory(rr, req)
	if rr.Code != 403 {
		t.Fatalf("sin identificador: HTTP %d", rr.Code)
	}
}
