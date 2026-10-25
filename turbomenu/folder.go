package main

// «Nueva carpeta con esto»: mete la selección en una carpeta nueva, con un nombre
// sugerido a partir de lo que tienen en común, y se puede deshacer.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/sys/windows"
)

// lastFolder es lo último que hizo «Nueva carpeta con esto» en este proceso: lo
// único que «Deshacer» puede devolver (no se fía de lo que mande la página).
var (
	folderMu   sync.Mutex
	lastFolder *FolderResult
)

// moveNoReplace mueve sin pisar nunca un archivo que ya exista en el destino
// (os.Rename en Windows lo reemplazaría sin avisar).
func moveNoReplace(from, to string) error {
	f, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	t, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(f, t, 0)
}

var months = []string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}

var stopWords = map[string]bool{
	"de": true, "del": true, "la": true, "el": true, "los": true, "las": true, "y": true, "en": true, "a": true, "un": true, "una": true,
	"the": true, "of": true, "and": true, "copy": true, "copia": true, "img": true, "dsc": true, "vid": true, "pxl": true, "screenshot": true,
	"final": true, "nuevo": true, "new": true, "file": true, "archivo": true, "documento": true, "captura": true, "foto": true, "photo": true,
}

var tokenRe = regexp.MustCompile(`[\p{L}]+|\d+`)
var dateInName = regexp.MustCompile(`(19|20)\d{2}[-_. ]?(0[1-9]|1[0-2])[-_. ]?(0[1-9]|[12]\d|3[01])`)

func nameStem(p string) string {
	b := filepath.Base(p)
	return strings.TrimSuffix(b, filepath.Ext(b))
}

func titleCase(s string) string {
	r := []rune(s)
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r)
}

// commonPrefix devuelve el inicio común de los nombres, cortado en una palabra entera.
func commonPrefix(names []string) string {
	if len(names) < 2 {
		return ""
	}
	p := []rune(names[0])
	for _, n := range names[1:] {
		r := []rune(n)
		i := 0
		for i < len(p) && i < len(r) && unicode.ToLower(p[i]) == unicode.ToLower(r[i]) {
			i++
		}
		p = p[:i]
	}
	s := string(p)
	// Si se corta a mitad de palabra o número, retroceder hasta un separador.
	if len(p) > 0 && len(p) < len([]rune(names[0])) {
		next := []rune(names[0])[len(p)]
		if unicode.IsLetter(next) || unicode.IsDigit(next) {
			if i := strings.LastIndexFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }); i >= 0 {
				s = s[:i]
			} else {
				s = ""
			}
		}
	}
	s = strings.TrimFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	return s
}

func kindWord(kinds map[string]int, n int) string {
	best, bn := "", 0
	for k, c := range kinds {
		if c > bn {
			best, bn = k, c
		}
	}
	if bn*10 < n*8 { // menos del 80 % del mismo tipo
		return "Archivos"
	}
	switch best {
	case "image":
		return "Fotos"
	case "video":
		return "Vídeos"
	case "audio":
		return "Audios"
	case "pdf", "doc", "text":
		return "Documentos"
	case "folder":
		return "Carpetas"
	}
	return "Archivos"
}

// SuggestFolderNames propone nombres para la carpeta (el primero es el mejor).
func (a *App) SuggestFolderNames() []string {
	job := a.currentJob()
	if job == nil {
		return []string{"Nueva carpeta"}
	}
	return suggestNames(job.Paths)
}

func suggestNames(paths []string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.Join(strings.Fields(s), " ")
		if len([]rune(s)) < 3 || seen[strings.ToLower(s)] {
			return
		}
		if len([]rune(s)) > 60 {
			s = string([]rune(s)[:60])
		}
		seen[strings.ToLower(s)] = true
		out = append(out, s)
	}
	names := make([]string, len(paths))
	kinds := map[string]int{}
	var minT, maxT time.Time
	for i, p := range paths {
		names[i] = nameStem(p)
		fi := statFile(p)
		kinds[fi.Kind]++
		t := time.Unix(fi.Mod, 0)
		if m := dateInName.FindString(names[i]); m != "" {
			digits := strings.Map(func(r rune) rune {
				if unicode.IsDigit(r) {
					return r
				}
				return -1
			}, m)
			if tt, err := time.ParseInLocation("20060102", digits, time.Local); err == nil {
				t = tt
			}
		}
		if fi.Mod > 0 || !t.IsZero() {
			if minT.IsZero() || t.Before(minT) {
				minT = t
			}
			if t.After(maxT) {
				maxT = t
			}
		}
	}
	n := len(paths)

	// Palabras que se repiten en la mayoría de los nombres («Vacaciones playa 01», «… 02»).
	type wc struct {
		w     string
		c, at int
	}
	counts := map[string]*wc{}
	for _, nm := range names {
		local := map[string]bool{}
		for i, t := range tokenRe.FindAllString(nm, -1) {
			lt := strings.ToLower(t)
			isYear := len(t) == 4 && (strings.HasPrefix(t, "19") || strings.HasPrefix(t, "20"))
			if local[lt] || stopWords[lt] || (unicode.IsDigit([]rune(t)[0]) && !isYear) || (len([]rune(t)) < 3 && !isYear) {
				continue
			}
			local[lt] = true
			if counts[lt] == nil {
				counts[lt] = &wc{w: t, at: i}
			}
			counts[lt].c++
		}
	}
	var common []*wc
	for _, v := range counts {
		if n == 1 || v.c*10 >= n*6 {
			common = append(common, v)
		}
	}
	sort.Slice(common, func(i, j int) bool { return common[i].at < common[j].at })
	if len(common) > 0 && n > 1 {
		ws := []string{}
		for _, c := range common {
			if len(ws) < 4 {
				ws = append(ws, c.w)
			}
		}
		add(titleCase(strings.Join(ws, " ")))
	}
	if p := commonPrefix(names); p != "" && n > 1 {
		add(titleCase(p))
	}
	kw := kindWord(kinds, n)
	if !minT.IsZero() {
		if minT.Year() == maxT.Year() && minT.Month() == maxT.Month() {
			add(fmt.Sprintf("%s %s %d", kw, months[minT.Month()-1], minT.Year()))
		} else if minT.Year() == maxT.Year() {
			add(fmt.Sprintf("%s %d", kw, minT.Year()))
		}
		if len(out) > 0 && !strings.ContainsAny(out[0], "0123456789") && minT.Year() == maxT.Year() {
			add(fmt.Sprintf("%s %d", out[0], minT.Year()))
		}
	}
	add(kw)
	add("Nueva carpeta")
	if len(out) > 4 {
		out = out[:4]
	}
	return out
}

type FolderResult struct {
	Folder string   `json:"folder"`
	Moved  []string `json:"moved"` // rutas nuevas
	From   []string `json:"from"`  // rutas originales (para deshacer)
	Failed []string `json:"failed"`
	Errors []string `json:"errors"`
}

// MakeFolder crea la carpeta junto a los archivos y los mueve dentro.
func (a *App) MakeFolder(name string) (*FolderResult, error) {
	job := a.currentJob()
	if job == nil || len(job.Paths) == 0 {
		return nil, errors.New("No hay nada seleccionado")
	}
	name = cleanFileName(strings.TrimSpace(name))
	if name == "" || name == "archivo" {
		name = "Nueva carpeta"
	}
	parent := filepath.Dir(job.Paths[0])
	dst := filepath.Join(parent, name)
	if st, err := os.Stat(dst); err == nil && !st.IsDir() {
		dst = uniquePath(parent, name)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return nil, errors.New("No se pudo crear la carpeta: " + err.Error())
	}
	res := &FolderResult{Folder: dst}
	for _, p := range job.Paths {
		if strings.EqualFold(filepath.Clean(p), filepath.Clean(dst)) {
			continue
		}
		to := uniquePath(dst, filepath.Base(p))
		if err := moveNoReplace(p, to); err != nil {
			res.Failed = append(res.Failed, p)
			msg := "No se pudo mover"
			if errors.Is(err, os.ErrPermission) || strings.Contains(err.Error(), "being used") || strings.Contains(err.Error(), "en uso") {
				msg = "Está en uso por otro programa"
			}
			res.Errors = append(res.Errors, msg)
			continue
		}
		res.Moved = append(res.Moved, to)
		res.From = append(res.From, p)
	}
	notifyChanged(parent)
	folderMu.Lock()
	keep := *res
	lastFolder = &keep
	folderMu.Unlock()
	return res, nil
}

// UndoFolder devuelve los archivos a su sitio y borra la carpeta si queda vacía.
// Trabaja con lo que guardó MakeFolder (lo que mande la página no se usa) y nunca
// pisa un archivo que haya aparecido después en el sitio de origen.
func (a *App) UndoFolder(_ FolderResult) error {
	folderMu.Lock()
	r := lastFolder
	lastFolder = nil
	folderMu.Unlock()
	if r == nil {
		return errors.New("No hay nada que deshacer")
	}
	var failed, taken int
	left := &FolderResult{Folder: r.Folder} // lo que no se ha podido devolver (para reintentar)
	for i, to := range r.Moved {
		if i >= len(r.From) {
			break
		}
		if _, err := os.Lstat(r.From[i]); err == nil {
			taken++ // ya hay otro con ese nombre donde estaba: no se pisa
			continue
		}
		if err := moveNoReplace(to, r.From[i]); err != nil {
			failed++
			left.Moved, left.From = append(left.Moved, to), append(left.From, r.From[i])
		}
	}
	os.Remove(r.Folder) // solo se borra si está vacía
	notifyChanged(filepath.Dir(r.Folder))
	if failed > 0 {
		folderMu.Lock()
		lastFolder = left
		folderMu.Unlock()
	}
	var msgs []string
	if taken > 0 {
		msgs = append(msgs, plural(taken, "archivo se ha quedado", "archivos se han quedado")+" en la carpeta nueva porque donde estaba ya hay otro con el mismo nombre")
	}
	if failed > 0 {
		msgs = append(msgs, "no se ha podido devolver "+plural(failed, "archivo", "archivos")+" (puede que esté en uso): vuelve a pulsar «Deshacer»")
	}
	if len(msgs) > 0 {
		m := strings.Join(msgs, "; ")
		return errors.New(strings.ToUpper(m[:1]) + m[1:])
	}
	return nil
}

// plural: «1 archivo» / «3 archivos».
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// RenameInExplorer abre la carpeta nueva seleccionada en el Explorador.
func (a *App) RevealRename(p string) { selectInExplorer(p, false) }
