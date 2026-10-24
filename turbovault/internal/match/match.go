// Package match decide si una entrada guardada vale para la web abierta.
//
// Reglas (pensadas contra el phishing):
//   - Solo http y https.
//   - La web tiene que ser exactamente la guardada (con o sin "www."): lo
//     guardado para github.com no vale en gist.github.com, ni en
//     github.com.evil.net, ni en otro-github.com. Un subdominio de la misma web
//     podría ser de otra persona (páginas de usuarios) o estar comprometido.
//   - Para valer en todos los subdominios hay que pedirlo escribiendo la web
//     como "*.google.com".
//   - Si se guardó con https, una página http no la recibe.
//   - El puerto tiene que coincidir. Las IP, exactas.
package match

import (
	"net"
	"net/url"
	"strings"
)

// Site es una web ya interpretada.
type Site struct {
	Scheme string
	Host   string // en minúsculas y sin "www."
	Port   string
}

// Parse interpreta una dirección; sin esquema se supone https.
func Parse(raw string) (Site, bool) {
	s, wild, ok := parse(raw)
	if wild {
		return Site{}, false
	}
	return s, ok
}

func parse(raw string) (Site, bool, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Site{}, false, false
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	wild := false
	if i := strings.Index(raw, "://*."); i >= 0 {
		wild = true
		raw = raw[:i+3] + raw[i+5:]
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Site{}, false, false
	}
	sc := strings.ToLower(u.Scheme)
	if sc != "http" && sc != "https" {
		return Site{}, false, false
	}
	h := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	h = strings.TrimPrefix(h, "www.")
	if h == "" || strings.ContainsAny(h, ` /\@*`) {
		return Site{}, false, false
	}
	port := u.Port()
	if (sc == "https" && port == "443") || (sc == "http" && port == "80") {
		port = ""
	}
	return Site{Scheme: sc, Host: h, Port: port}, wild, true
}

// Origin devuelve "esquema://host[:puerto]" de una dirección (para mostrar y
// para el registro de actividad).
func Origin(raw string) string {
	s, ok := Parse(raw)
	if !ok {
		return ""
	}
	o := s.Scheme + "://" + s.Host
	if s.Port != "" {
		o += ":" + s.Port
	}
	return o
}

// Host devuelve solo el nombre de la web ("*.google.com" da "google.com").
func Host(raw string) string {
	s, _, ok := parse(raw)
	if !ok {
		return ""
	}
	return s.Host
}

func isLocal(h string) bool {
	return h == "localhost" || strings.HasSuffix(h, ".localhost") || h == "127.0.0.1" || h == "::1" || h == "[::1]"
}

// Matches dice si lo guardado con la dirección entry vale para la página page.
func Matches(entry, page string) bool { return Score(entry, page) > 0 }

// Score dice lo bien que encaja (más es mejor): 2 la misma web, 1 un
// subdominio permitido con "*.", 0 nada.
func Score(entry, page string) int {
	e, wild, ok1 := parse(entry)
	p, ok2 := Parse(page)
	if !ok1 || !ok2 {
		return 0
	}
	if e.Scheme == "https" && p.Scheme == "http" && !isLocal(p.Host) {
		return 0
	}
	if e.Port != p.Port {
		return 0
	}
	if e.Host == p.Host {
		return 2
	}
	if !wild || net.ParseIP(strings.Trim(e.Host, "[]")) != nil || net.ParseIP(strings.Trim(p.Host, "[]")) != nil {
		return 0
	}
	// "*.com" o "*.es" no cuentan: tiene que haber al menos un nombre y su dominio.
	if !strings.Contains(e.Host, ".") {
		return 0
	}
	if strings.HasSuffix(p.Host, "."+e.Host) {
		return 1
	}
	return 0
}
