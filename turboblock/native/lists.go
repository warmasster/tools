package main

// Catálogo de listas de filtros y su descarga (con caché y peticiones condicionales).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Category agrupa listas que el usuario activa o desactiva juntas. Cada una se
// compila a su propio ruleset de declarativeNetRequest y su propia hoja de estilos.
type Category struct {
	ID      string
	Name    string
	Default bool // activada al instalar
	Hidden  bool // siempre activa, no sale en la interfaz (excepciones que arreglan webs)
	Sources []Source
	// DedupeAgainst: no repetir filtros que ya estén en esa categoría (la lista de
	// molestias de Fanboy incluye entera la de avisos de cookies).
	DedupeAgainst string
}

type Source struct {
	ID    string
	Title string
	URL   string
	// Trusted: listas de uBlock Origin, las únicas que pueden usar scriptlets "trusted-*".
	Trusted bool
	// Local: lista propia que va dentro de la extensión (ruta relativa a ella) en vez
	// de descargarse.
	Local string
	// HostsOnly: solo los filtros de dominio entero. La lista de URLhaus tiene
	// decenas de miles de URLs sueltas que gastarían el cupo de reglas de Chrome.
	HostsOnly bool
}

var categories = []Category{
	{ID: "base", Name: "Arreglos", Default: true, Hidden: true, Sources: []Source{
		{ID: "ubo-unbreak", Title: "uBlock filters – Unbreak", URL: "https://ublockorigin.github.io/uAssets/filters/unbreak.txt", Trusted: true},
	}},
	{ID: "ads", Name: "Anuncios", Default: true, Sources: []Source{
		{ID: "easylist", Title: "EasyList", URL: "https://easylist.to/easylist/easylist.txt"},
		{ID: "easylist-es", Title: "EasyList Spanish", URL: "https://easylist-downloads.adblockplus.org/easylistspanish.txt"},
		{ID: "ubo-filters", Title: "uBlock filters", URL: "https://ublockorigin.github.io/uAssets/filters/filters.txt", Trusted: true},
		{ID: "ubo-quick", Title: "uBlock filters – Quick fixes", URL: "https://ublockorigin.github.io/uAssets/filters/quick-fixes.txt", Trusted: true},
		{ID: "turboblock", Title: "TurboBLOCK – Vídeo y extras", Local: "filters/turboblock.txt", Trusted: true},
		{ID: "pgl", Title: "Peter Lowe's Ad and tracking server list", URL: "https://pgl.yoyo.org/adservers/serverlist.php?hostformat=adblockplus&showintro=0&mimetype=plaintext"},
	}},
	{ID: "privacy", Name: "Rastreadores", Default: true, Sources: []Source{
		{ID: "easyprivacy", Title: "EasyPrivacy", URL: "https://easylist.to/easylist/easyprivacy.txt"},
		{ID: "ubo-privacy", Title: "uBlock filters – Privacy", URL: "https://ublockorigin.github.io/uAssets/filters/privacy.txt", Trusted: true},
	}},
	{ID: "malware", Name: "Malware y estafas", Default: true, Sources: []Source{
		{ID: "ubo-badware", Title: "uBlock filters – Badware risks", URL: "https://ublockorigin.github.io/uAssets/filters/badware.txt", Trusted: true},
		{ID: "urlhaus", Title: "Online Malicious URL Blocklist", URL: "https://malware-filter.gitlab.io/malware-filter/urlhaus-filter-online.txt", Trusted: false, HostsOnly: true},
	}},
	{ID: "cookies", Name: "Avisos de cookies", Default: false, Sources: []Source{
		{ID: "easylist-cookie", Title: "EasyList Cookie", URL: "https://secure.fanboy.co.nz/fanboy-cookiemonster.txt"},
	}},
	{ID: "annoyances", Name: "Molestias", Default: false, DedupeAgainst: "cookies", Sources: []Source{
		{ID: "fanboy-annoyance", Title: "Fanboy's Annoyance List", URL: "https://easylist.to/easylist/fanboy-annoyance.txt"},
		{ID: "ubo-annoyances", Title: "uBlock filters – Annoyances", URL: "https://ublockorigin.github.io/uAssets/filters/annoyances-others.txt", Trusted: true},
	}},
}

func categoryByID(id string) *Category {
	for i := range categories {
		if categories[i].ID == id {
			return &categories[i]
		}
	}
	return nil
}

// ListText es el contenido de una lista ya descargada (con sus !#include resueltos).
type ListText struct {
	Source  Source
	Text    string
	Version string    // "! Version:" o "! Last modified:" de la cabecera
	Fetched time.Time // última vez que se comprobó en el servidor
}

// cacheMeta se guarda junto a cada archivo de la caché.
type cacheMeta struct {
	URL          string    `json:"url"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"lastModified,omitempty"`
	Fetched      time.Time `json:"fetched"`
	Expires      float64   `json:"expiresHours"`
}

var httpClient = &http.Client{Timeout: 90 * time.Second}

// FetchOptions controla cuándo se vuelve a pedir una lista al servidor.
type FetchOptions struct {
	CacheDir string
	Force    bool // pedirlas aunque no hayan caducado (sigue siendo condicional)
	Offline  bool // solo caché
}

// fetchAll descarga (o lee de la caché) todas las listas. changed dice si alguna
// ha cambiado respecto a la caché. Si alguna lista no está disponible devuelve
// error: compilar sin ella quitaría reglas.
func fetchAll(ctx context.Context, o FetchOptions, extDir string) (map[string]*ListText, bool, error) {
	out := map[string]*ListText{}
	changed := false
	for _, c := range categories {
		for _, s := range c.Sources {
			if s.Local != "" {
				raw, err := os.ReadFile(filepath.Join(extDir, filepath.FromSlash(s.Local)))
				if err != nil {
					return nil, false, fmt.Errorf("%s: %w", s.Title, err)
				}
				out[s.ID] = &ListText{Source: s, Text: string(raw), Version: listVersion(string(raw)), Fetched: time.Now()}
				continue
			}
			lt, ch, err := fetchList(ctx, o, s)
			if err != nil {
				return nil, false, fmt.Errorf("%s: %w", s.Title, err)
			}
			changed = changed || ch
			out[s.ID] = lt
		}
	}
	return out, changed, nil
}

func fetchList(ctx context.Context, o FetchOptions, s Source) (*ListText, bool, error) {
	text, fetched, changed, err := fetchCached(ctx, o, s.ID, s.URL, 0)
	if err != nil {
		return nil, false, err
	}
	lt := &ListText{Source: s, Text: text, Version: listVersion(text), Fetched: fetched}
	return lt, changed, nil
}

var includeRe = regexp.MustCompile(`(?m)^!#include +(\S+)\s*$`)

// fetchCached baja una URL con caché y resuelve sus !#include (solo archivos de la
// misma carpeta, como uBlock Origin).
func fetchCached(ctx context.Context, o FetchOptions, id, rawURL string, depth int) (string, time.Time, bool, error) {
	if err := os.MkdirAll(o.CacheDir, 0o755); err != nil {
		return "", time.Time{}, false, err
	}
	txtPath := filepath.Join(o.CacheDir, id+".txt")
	metaPath := filepath.Join(o.CacheDir, id+".json")
	var meta cacheMeta
	if raw, err := os.ReadFile(metaPath); err == nil {
		json.Unmarshal(raw, &meta)
	}
	cached, cacheErr := os.ReadFile(txtPath)
	hasCache := cacheErr == nil && meta.URL == rawURL
	changed := false

	fresh := hasCache && time.Since(meta.Fetched) < time.Duration(meta.Expires*float64(time.Hour))
	if !o.Offline && (!hasCache || o.Force || !fresh) {
		body, newMeta, notModified, err := httpGet(ctx, rawURL, meta, hasCache)
		switch {
		case err != nil && hasCache:
			// Sin red: se sigue con lo que hay.
		case err != nil:
			return "", time.Time{}, false, err
		case notModified:
			meta.Fetched = time.Now()
			saveJSON(metaPath, meta)
		default:
			if !hasCache || string(cached) != body {
				changed = true
			}
			newMeta.Expires = parseExpires(body)
			if err := writeFileAtomic(txtPath, []byte(body)); err != nil {
				return "", time.Time{}, false, err
			}
			saveJSON(metaPath, newMeta)
			meta = newMeta
			cached = []byte(body)
			hasCache = true
		}
	}
	if !hasCache {
		return "", time.Time{}, false, errors.New("no está en la caché y no se puede descargar")
	}

	text := string(cached)
	if depth < 2 && strings.Contains(text, "!#include") {
		base, err := url.Parse(rawURL)
		if err != nil {
			return "", time.Time{}, false, err
		}
		var firstErr error
		text = includeRe.ReplaceAllStringFunc(text, func(line string) string {
			name := includeRe.FindStringSubmatch(line)[1]
			if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
				return "" // solo archivos de la misma carpeta
			}
			u := *base
			u.Path = path.Join(path.Dir(base.Path), name)
			u.RawQuery = ""
			sub, _, ch, err := fetchCached(ctx, o, id+"~"+strings.TrimSuffix(name, ".txt"), u.String(), depth+1)
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("%s: %w", name, err)
				}
				return ""
			}
			changed = changed || ch
			return sub
		})
		if firstErr != nil {
			return "", time.Time{}, false, firstErr
		}
	}
	return text, meta.Fetched, changed, nil
}

func httpGet(ctx context.Context, rawURL string, prev cacheMeta, conditional bool) (string, cacheMeta, bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return "", cacheMeta{}, false, err
	}
	req.Header.Set("User-Agent", "TurboBLOCK/"+version)
	if conditional {
		if prev.ETag != "" {
			req.Header.Set("If-None-Match", prev.ETag)
		}
		if prev.LastModified != "" {
			req.Header.Set("If-Modified-Since", prev.LastModified)
		}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", cacheMeta{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified && conditional {
		return "", cacheMeta{}, true, nil
	}
	if resp.StatusCode != 200 {
		return "", cacheMeta{}, false, fmt.Errorf("el servidor respondió %s", resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return "", cacheMeta{}, false, err
	}
	body := strings.TrimPrefix(string(raw), "\ufeff")
	// Una página de error con 200 no es una lista: mejor quedarse con la de antes.
	if strings.HasPrefix(strings.TrimSpace(strings.ToLower(body)), "<!doctype html") || strings.HasPrefix(strings.TrimSpace(strings.ToLower(body)), "<html") {
		return "", cacheMeta{}, false, errors.New("el servidor devolvió una página web en vez de la lista")
	}
	return body, cacheMeta{
		URL:          rawURL,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		Fetched:      time.Now(),
	}, false, nil
}

var expiresRe = regexp.MustCompile(`(?mi)^! *Expires: *(\d+) *(day|days|hour|hours|d|h)\b`)

// parseExpires lee "! Expires: 4 days" de la cabecera (en horas). Entre 6 h y 4 días.
func parseExpires(text string) float64 {
	head := text
	if len(head) > 4096 {
		head = head[:4096]
	}
	h := 24.0
	if m := expiresRe.FindStringSubmatch(head); m != nil {
		n, _ := strconv.Atoi(m[1])
		if strings.HasPrefix(strings.ToLower(m[2]), "d") {
			h = float64(n * 24)
		} else {
			h = float64(n)
		}
	}
	return min(max(h, 6), 96)
}

var versionRe = regexp.MustCompile(`(?mi)^! *(?:Version|Last modified|Updated): *(.+?)\s*$`)

func listVersion(text string) string {
	head := text
	if len(head) > 4096 {
		head = head[:4096]
	}
	if m := versionRe.FindStringSubmatch(head); m != nil {
		return m[1]
	}
	return ""
}

func saveJSON(p string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(p, raw)
}

// writeFileAtomic escribe en un temporal y lo renombra encima: nunca queda un
// archivo a medias aunque el proceso muera.
func writeFileAtomic(p string, data []byte) error {
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
