package steamworks

import (
	"os"
	"slices"
	"strconv"
	"testing"

	"turbogames/internal/steam"
)

// Prueba con el Steam abierto, solo leyendo (no suscribe ni quita nada):
//
//	$env:TURBOGAMES_WORKSHOP = "<appid de un juego con mods de la cuenta abierta>"
//	go test -run Real -v ./internal/steamworks/
//
// Mientras dura, Steam enseña a tus amigos que estás «jugando» a ese juego.
func TestRealReadOnly(t *testing.T) {
	v := os.Getenv("TURBOGAMES_WORKSHOP")
	if v == "" {
		t.Skip("pon TURBOGAMES_WORKSHOP con el appid de un juego")
	}
	appid, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		t.Fatal(err)
	}
	in, err := steam.Locate("")
	if err != nil {
		t.Fatal(err)
	}
	c, err := Open(in.Dir, uint32(appid), steam.SignedByValve)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ids := c.Subscribed()
	t.Logf("suscrito a %d mods", len(ids))
	for i, id := range ids {
		if i >= 3 {
			break
		}
		t.Logf("estado %d: %+v", id, c.State(id))
	}
	if len(ids) > 0 {
		items, err := c.Details(ids[:min(3, len(ids))])
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range items {
			t.Logf("%d %q autor=%s tam=%d votos=%d/%d nota=%.2f subs=%d coment=%d deps=%v tags=%v img=%v", it.ID, it.Title, it.Owner, it.Size, it.VotesUp, it.VotesDown, it.Score, it.Subscriptions, it.Comments, it.Children, it.Tags, it.Preview != "")
			if it.Title == "" || it.Missing {
				t.Errorf("mod %d sin datos", it.ID)
			}
		}
	}
	p, err := c.Search("", QueryByTrend, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("búsqueda: %d de %d, siguiente=%q", len(p.Items), p.Total, p.Cursor)
	for i, it := range p.Items {
		if i >= 3 {
			break
		}
		t.Logf("  %q votos=%d/%d", it.Title, it.VotesUp, it.VotesDown)
	}
	if len(p.Items) == 0 {
		t.Error("la búsqueda no ha devuelto nada")
	}
	p2, err := c.Search("map", QueryByText, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("búsqueda «map»: %d de %d", len(p2.Items), p2.Total)
	if len(p.Items) > 0 && len(p.Items[0].Tags) > 0 {
		tag := p.Items[0].Tags[0]
		p4, err := c.Search("", QueryByVote, "", []string{tag})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("con la etiqueta %q: %d de %d", tag, len(p4.Items), p4.Total)
		for _, it := range p4.Items {
			if !slices.Contains(it.Tags, tag) {
				t.Errorf("%q no tiene la etiqueta %q: %v", it.Title, tag, it.Tags)
				break
			}
		}
	}
	if len(ids) > 0 {
		items, _ := c.Details(ids)
		n := 0
		for _, it := range items {
			n += len(it.Previews)
			if len(it.Previews) > 0 && n == len(it.Previews) {
				t.Logf("galería de %q: %+v", it.Title, it.Previews[:min(3, len(it.Previews))])
			}
		}
		t.Logf("imágenes y vídeos en sus %d mods: %d", len(items), n)
	}
	if p.Cursor != "" {
		p3, err := c.Search("", QueryByTrend, p.Cursor, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("página 2: %d (primera de la 2: %q)", len(p3.Items), func() string {
			if len(p3.Items) > 0 {
				return p3.Items[0].Title
			}
			return ""
		}())
	}
}
