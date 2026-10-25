// Package csvio lee las contraseñas exportadas por otros gestores (Chrome,
// Edge, Firefox, Bitwarden, KeePass/KeePassXC, 1Password, LastPass, Proton
// Pass...) y escribe una exportación compatible con Chrome.
package csvio

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

// Record es una entrada importada.
type Record struct {
	Title    string
	URL      string
	Username string
	Password string
	Notes    string
	TOTP     string
}

// Result es lo que sale de leer un CSV.
type Result struct {
	Records []Record
	Source  string // "Chrome", "Bitwarden"... o "CSV"
	Skipped int    // filas vacías o sin nada útil
}

var ErrNoHeader = errors.New("no se reconoce el archivo: la primera fila debe tener los nombres de las columnas (url, usuario, contraseña...)")

// Nombres de columna de cada gestor, en minúsculas.
var columns = map[string][]string{
	"title":    {"name", "title", "nombre", "título", "titulo", "account", "item name"},
	"url":      {"url", "login_uri", "website", "web site", "uri", "web", "sitio", "origin", "hostname", "login url", "urls"},
	"username": {"username", "login_username", "user", "login", "usuario", "nombre de usuario", "email", "login name", "user name"},
	"password": {"password", "login_password", "contraseña", "contrasena", "pass", "clave"},
	"notes":    {"note", "notes", "extra", "comments", "notas", "nota", "comment"},
	"totp":     {"totp", "login_totp", "otpauth", "one-time password", "otp", "2fa", "otp secret"},
}

func detectSource(h []string) string {
	has := func(s string) bool {
		for _, x := range h {
			if x == s {
				return true
			}
		}
		return false
	}
	switch {
	case has("login_uri") && has("login_password"):
		return "Bitwarden"
	case has("httprealm") || has("formactionorigin"):
		return "Firefox"
	case has("grouping") && has("extra"):
		return "LastPass"
	case has("group") && has("title"):
		return "KeePass"
	case has("otpauth"):
		return "1Password"
	case has("name") && has("url") && has("username") && has("password"):
		return "Chrome"
	}
	return "CSV"
}

// Read lee un CSV. Detecta el separador (coma o punto y coma) y la cabecera.
func Read(r io.Reader) (*Result, error) {
	raw, err := io.ReadAll(io.LimitReader(r, 64<<20))
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(raw) {
		raw = latin1ToUTF8(raw)
	}
	first := string(raw)
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	cr := csv.NewReader(bytes.NewReader(raw))
	if strings.Count(first, ";") > strings.Count(first, ",") {
		cr.Comma = ';'
	}
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, errors.New("el CSV está mal formado: " + err.Error())
	}
	if len(rows) == 0 {
		return nil, ErrNoHeader
	}
	head := make([]string, len(rows[0]))
	for i, h := range rows[0] {
		head[i] = strings.ToLower(strings.TrimSpace(h))
	}
	idx := map[string]int{}
	for field, names := range columns {
		idx[field] = -1
		for _, n := range names {
			for i, h := range head {
				if h == n {
					idx[field] = i
					break
				}
			}
			if idx[field] >= 0 {
				break
			}
		}
	}
	if idx["password"] < 0 || (idx["url"] < 0 && idx["title"] < 0 && idx["username"] < 0) {
		return nil, ErrNoHeader
	}
	res := &Result{Source: detectSource(head)}
	get := func(row []string, f string) string {
		i := idx[f]
		if i < 0 || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}
	getRaw := func(row []string, f string) string {
		i := idx[f]
		if i < 0 || i >= len(row) {
			return ""
		}
		return strings.TrimRight(row[i], "\r\n")
	}
	for _, row := range rows[1:] {
		rec := Record{
			Title:    get(row, "title"),
			URL:      get(row, "url"),
			Username: get(row, "username"),
			Password: getRaw(row, "password"), // un espacio al principio o al final es parte de la contraseña
			Notes:    get(row, "notes"),
			TOTP:     get(row, "totp"),
		}
		// Bitwarden y otros meten varias webs separadas por comas o saltos.
		if i := strings.IndexAny(rec.URL, ",\n"); i > 0 {
			rec.URL = strings.TrimSpace(rec.URL[:i])
		}
		if rec.Password == "" && rec.Username == "" && rec.Notes == "" {
			res.Skipped++
			continue
		}
		res.Records = append(res.Records, rec)
	}
	return res, nil
}

func latin1ToUTF8(b []byte) []byte {
	var out bytes.Buffer
	for _, c := range b {
		out.WriteRune(rune(c))
	}
	return out.Bytes()
}

// Write escribe una exportación con las columnas de Chrome más notas y 2FA.
func Write(w io.Writer, recs []Record) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"name", "url", "username", "password", "note", "totp"}); err != nil {
		return err
	}
	for _, r := range recs {
		if err := cw.Write([]string{r.Title, r.URL, r.Username, r.Password, r.Notes, r.TOTP}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
