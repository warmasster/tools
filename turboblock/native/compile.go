package main

// Compilación de las listas a los archivos que usa la extensión (carpeta rules/):
//
//	<cat>.json           reglas de declarativeNetRequest (las aplica Chrome por dentro)
//	<cat>.css            ocultación genérica que no depende de una clase o id (hoja fija)
//	<cat>.cosmetic.json  ocultación por dominio, procedimental y listas de ventanas emergentes
//	<cat>.slN.js         scriptlets por dominio, en grupos (cada uno se registra solo en sus dominios)
//	<cat>.slaN.js        scriptlets con comodín de dominio ("google.*"): la extensión los
//	                     registra para un sitio concreto la primera vez que se visita
//	meta.json            resumen: recuentos, versiones de las listas y excepciones globales

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Meta es rules/meta.json.
type Meta struct {
	Build      string              `json:"build"` // huella del contenido: cambia si cambia cualquier regla
	Built      time.Time           `json:"built"`
	Version    string              `json:"version"` // versión del compilador
	Categories map[string]*CatMeta `json:"categories"`
	Lists      []ListMeta          `json:"lists"`
	Ghide      []string            `json:"ghide"` // dominios sin ocultación genérica
}

type CatMeta struct {
	Name       string     `json:"name"`
	Default    bool       `json:"default"`
	Hidden     bool       `json:"hidden,omitempty"`
	Network    int        `json:"network"`    // reglas DNR
	Filters    int        `json:"filters"`    // filtros de red originales
	Generic    int        `json:"generic"`    // selectores genéricos
	GenericCSS int        `json:"genericCss"` // de ellos, en la hoja fija <cat>.css
	Specific   int        `json:"specific"`   // reglas cosméticas por dominio
	Scriptlets int        `json:"scriptlets"`
	SlBuckets  [][]string `json:"slBuckets"`  // dominios donde se registra cada <cat>.slN.js
	SlaBuckets int        `json:"slaBuckets"` // grupos <cat>.slaN.js (entidades; los registra la extensión al visitarlas)
}

type ListMeta struct {
	ID       string    `json:"id"`
	Category string    `json:"category"`
	Title    string    `json:"title"`
	URL      string    `json:"url"`
	Version  string    `json:"version,omitempty"`
	Fetched  time.Time `json:"fetched,omitempty"`
}

// specEntry: lo que se aplica en un dominio concreto (o entidad "google.*").
type specEntry struct {
	Hide   []string `json:"h,omitempty"`
	Unhide []string `json:"u,omitempty"`
	Proc   []string `json:"p,omitempty"`
	CSS    []string `json:"c,omitempty"`
	NoSpec bool     `json:"s,omitempty"`
	Ghide  bool     `json:"g,omitempty"` // sin ocultación genérica
	// J: grupos de scriptlets con comodín que tocan aquí (lo rellena la extensión).
}

type cosmeticFile struct {
	Spec       map[string]*specEntry `json:"spec"`
	Gen        map[string][]string   `json:"gen"` // genéricos por clase o id: ".ad" -> selectores
	Gx         [][2]any              `json:"gx"`  // [selector, [dominios donde NO]]
	Sla        map[string]int        `json:"sla"` // entidad "google.*" -> grupo <cat>.slaN.js
	Popup      []string              `json:"pop"` // dominios cuyas ventanas emergentes se cierran
	PopupAllow []string              `json:"popx"`
}

type catInput struct {
	cat  Category
	net  []*netFilter
	cos  []*cosFilter
	nfil int
}

type compiled struct {
	Files map[string][]byte
	Meta  *Meta
}

// compileLists hace todo el trabajo sin tocar disco (así se puede probar).
// lists: texto de cada lista por id de fuente. lib: content/scriptlets.js.
func compileLists(lists map[string]*ListText, lib string) (*compiled, error) {
	if lib == "" {
		return nil, errors.New("falta content/scriptlets.js")
	}
	slib, err := parseScriptletLib(lib)
	if err != nil {
		return nil, err
	}
	// 1. Leer los filtros de cada categoría.
	var inputs []*catInput
	linesByCat := map[string]map[string]bool{}
	for _, c := range categories {
		in := &catInput{cat: c}
		seen := map[string]bool{}
		against := linesByCat[c.DedupeAgainst]
		for _, s := range c.Sources {
			lt := lists[s.ID]
			if lt == nil {
				continue
			}
			for _, line := range preprocess(lt.Text) {
				if seen[line] || against[line] {
					continue
				}
				seen[line] = true
				if cf, isCos := parseCosmetic(line, s.Trusted); isCos {
					// Scriptlets que no están en content/scriptlets.js: fuera.
					if cf != nil && (cf.kind != cosScript || cf.scriptlet[0] == "*" || slib.has(cf.scriptlet[0])) {
						in.cos = append(in.cos, cf)
					}
					continue
				}
				if nf := parseNetFilter(line); nf != nil && (!s.HostsOnly || nf.host != "") {
					in.net = append(in.net, nf)
				}
			}
		}
		linesByCat[c.ID] = seen
		inputs = append(inputs, in)
	}

	// 2. Cosas que valen para todas las categorías.
	bad := map[string]bool{}          // $badfilter
	genericOff := map[string]bool{}   // #@#selector sin dominio: fuera en todas partes
	unhideAt := map[string][]string{} // selector -> dominios con #@#selector
	slExcept := map[string][]string{} // dominio -> firmas de scriptlet desactivadas
	for _, in := range inputs {
		for _, f := range in.net {
			if f.badfilter {
				bad[f.key] = true
			}
		}
		for _, c := range in.cos {
			if !c.exception {
				continue
			}
			if c.kind == cosScript {
				for _, d := range c.domains {
					slExcept[d] = append(slExcept[d], scriptletSig(c.scriptlet))
				}
				continue
			}
			if len(c.domains) == 0 {
				genericOff[c.selector] = true
			}
			for _, d := range c.domains {
				unhideAt[c.selector] = append(unhideAt[c.selector], d)
			}
		}
	}

	// 3. Compilar cada categoría.
	out := &compiled{Files: map[string][]byte{}}
	meta := &Meta{Version: version, Categories: map[string]*CatMeta{}}
	var ghide []string
	for _, in := range inputs {
		c := in.cat
		nr := buildDNR(in.net, bad, c.ID == "malware")
		ghide = append(ghide, nr.Ghide...)

		cf := cosmeticFile{Spec: map[string]*specEntry{}, Gen: map[string][]string{}, Gx: [][2]any{}, Sla: map[string]int{}, Popup: nr.PopupHosts, PopupAllow: nr.PopupAllow}
		entry := func(k string) *specEntry {
			e := cf.Spec[k]
			if e == nil {
				e = &specEntry{}
				cf.Spec[k] = e
			}
			return e
		}
		for _, h := range nr.Shide {
			entry(h).NoSpec = true
		}
		for _, h := range nr.Ghide {
			entry(h).Ghide = true
		}
		var generic []string
		genericSeen := map[string]bool{}
		gx := map[string][]string{}
		var gxOrder []string
		sl := map[string][][]string{}  // dominio -> scriptlets
		sla := map[string][][]string{} // entidad -> scriptlets
		nsl := 0
		for _, f := range in.cos {
			switch f.kind {
			case cosScript:
				if f.exception {
					continue
				}
				for _, d := range f.domains {
					if strings.HasSuffix(d, ".*") {
						sla[d] = append(sla[d], f.scriptlet)
					} else {
						sl[d] = append(sl[d], f.scriptlet)
					}
					nsl++
				}
				// "~dominio" en un scriptlet con dominios: se desactiva ahí.
				for _, d := range f.notDomains {
					slExcept[d] = append(slExcept[d], scriptletSig(f.scriptlet))
				}
			case cosHide, cosProc, cosStyle:
				if f.exception {
					for _, d := range f.domains {
						e := entry(d)
						e.Unhide = append(e.Unhide, f.selector)
					}
					continue
				}
				if len(f.domains) == 0 {
					if f.kind != cosHide || genericOff[f.selector] {
						continue // procedimentales genéricos: demasiado caros en todas las webs
					}
					x := append(append([]string(nil), f.notDomains...), unhideAt[f.selector]...)
					if len(x) > 0 {
						if _, ok := gx[f.selector]; !ok {
							gxOrder = append(gxOrder, f.selector)
						}
						gx[f.selector] = append(gx[f.selector], x...)
						continue
					}
					if !genericSeen[f.selector] {
						genericSeen[f.selector] = true
						generic = append(generic, f.selector)
					}
					continue
				}
				for _, d := range f.domains {
					e := entry(d)
					switch f.kind {
					case cosHide:
						e.Hide = append(e.Hide, f.selector)
					case cosProc:
						e.Proc = append(e.Proc, f.selector)
					case cosStyle:
						e.CSS = append(e.CSS, f.selector+"{"+f.style+"}")
					}
				}
				for _, d := range f.notDomains {
					e := entry(d)
					e.Unhide = append(e.Unhide, f.selector)
				}
			}
		}
		// Un selector genérico con excepciones no puede ir en la hoja fija.
		for _, s := range gxOrder {
			if genericSeen[s] {
				generic = removeString(generic, s)
			}
			cf.Gx = append(cf.Gx, [2]any{s, compactHosts(gx[s])})
		}
		nspec := 0
		for _, e := range cf.Spec {
			e.Hide, e.Unhide, e.Proc, e.CSS = dedupe(e.Hide), dedupe(e.Unhide), dedupe(e.Proc), dedupe(e.CSS)
			nspec += len(e.Hide) + len(e.Proc) + len(e.CSS)
		}

		out.Files[c.ID+".json"] = mustJSON(nr.Rules, true)
		// Genéricos: los que dependen de una clase o id van a "gen" (la extensión solo
		// los aplica si la página tiene esa clase o id); el resto, a la hoja fija.
		var complexSel []string
		for _, sel := range generic {
			if k := genericKey(sel); k != "" {
				cf.Gen[k] = append(cf.Gen[k], sel)
			} else {
				complexSel = append(complexSel, sel)
			}
		}
		out.Files[c.ID+".css"] = genericCSS(complexSel)
		buckets := [][]string{}
		for i, b := range bucketScriptlets(sl) {
			out.Files[fmt.Sprintf("%s.sl%d.js", c.ID, i)] = scriptletFile(c.ID, b, slExcept, slib)
			var hosts []string
			for h := range b {
				if hostLike.MatchString(h) && !strings.Contains(h, "_") || isIP(h) {
					hosts = append(hosts, h)
				}
			}
			buckets = append(buckets, compactHosts(hosts))
		}
		slaBuckets := bucketScriptlets(sla)
		for i, b := range slaBuckets {
			out.Files[fmt.Sprintf("%s.sla%d.js", c.ID, i)] = scriptletFile(c.ID, b, slExcept, slib)
			for e := range b {
				cf.Sla[e] = i
			}
		}
		out.Files[c.ID+".cosmetic.json"] = mustJSON(cf, false)
		meta.Categories[c.ID] = &CatMeta{
			Name: c.Name, Default: c.Default, Hidden: c.Hidden,
			Network: len(nr.Rules), Filters: len(in.net), Generic: len(generic), GenericCSS: len(complexSel),
			Specific: nspec, Scriptlets: nsl, SlBuckets: buckets, SlaBuckets: len(slaBuckets),
		}
		for _, s := range c.Sources {
			if lt := lists[s.ID]; lt != nil {
				meta.Lists = append(meta.Lists, ListMeta{ID: s.ID, Category: c.ID, Title: s.Title, URL: s.URL, Version: lt.Version, Fetched: lt.Fetched})
			}
		}
	}
	meta.Ghide = compactHosts(ghide)
	if meta.Ghide == nil {
		meta.Ghide = []string{}
	}

	// Huella: el contenido de todas las reglas (sin fechas), para saber si algo cambió.
	h := sha256.New()
	names := make([]string, 0, len(out.Files))
	for n := range out.Files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		h.Write([]byte(n))
		h.Write(out.Files[n])
	}
	h.Write(mustJSON(meta.Ghide, false))
	meta.Build = hex.EncodeToString(h.Sum(nil))[:16]
	meta.Built = time.Now().UTC().Truncate(time.Second)
	out.Meta = meta
	out.Files["meta.json"] = mustJSON(meta, true)
	return out, nil
}

func removeString(s []string, x string) []string {
	for i, v := range s {
		if v == x {
			return append(s[:i:i], s[i+1:]...)
		}
	}
	return s
}

func scriptletSig(s []string) string { return strings.Join(s, "\x01") }

func mustJSON(v any, indentTop bool) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err)
	}
	raw := bytes.TrimSpace(buf.Bytes())
	// Arrays de reglas: una por línea, legible y sin el coste de indentar todo.
	if indentTop && len(raw) > 2 && raw[0] == '[' {
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) == nil {
			var b bytes.Buffer
			b.WriteString("[\n")
			for i, it := range items {
				b.Write(it)
				if i < len(items)-1 {
					b.WriteByte(',')
				}
				b.WriteByte('\n')
			}
			b.WriteString("]\n")
			return b.Bytes()
		}
	}
	if indentTop {
		var b bytes.Buffer
		json.Indent(&b, raw, "", " ")
		return b.Bytes()
	}
	return raw
}

// genericCSS: grupos de 20 selectores por regla. Un selector que Chrome no
// entienda anula su regla entera, así que los grupos son pequeños.
func genericCSS(sels []string) []byte {
	var b bytes.Buffer
	b.WriteString("/* TurboBLOCK: ocultación genérica (generado, no editar) */\n")
	for i := 0; i < len(sels); i += 20 {
		end := min(i+20, len(sels))
		b.WriteString(strings.Join(sels[i:end], ",\n"))
		b.WriteString("{display:none!important}\n")
	}
	return b.Bytes()
}

// siteBase: las dos últimas etiquetas del dominio ("www.youtube.com" → "youtube.com").
// Los scriptlets de un mismo sitio van al mismo grupo.
func siteBase(h string) string {
	if isIP(h) {
		return h
	}
	i := strings.LastIndexByte(h, '.')
	if i <= 0 {
		return h
	}
	if j := strings.LastIndexByte(h[:i], '.'); j >= 0 {
		return h[j+1:]
	}
	return h
}

func fnv32(s string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// bucketScriptlets reparte los dominios en grupos de unos 48 KB de datos. Chrome
// solo inyecta en una web los grupos que tienen su dominio, así que ninguna página
// carga los datos de miles de sitios que no son ella.
func bucketScriptlets(data map[string][][]string) []map[string][][]string {
	if len(data) == 0 {
		return nil
	}
	total := 0
	for k, l := range data {
		total += len(k) + 4
		for _, s := range l {
			total += len(scriptletSig(s)) + 6
		}
	}
	const target = 48 << 10
	n := max(1, (total+target-1)/target)
	out := make([]map[string][][]string, n)
	for i := range out {
		out[i] = map[string][][]string{}
	}
	for k, l := range data {
		out[fnv32(siteBase(k))%uint32(n)][k] = l
	}
	return out
}

// scriptletFile genera el script del mundo MAIN: datos + biblioteca, todo dentro
// de una función para no dejar nada a la vista de la página. Formato compacto:
//
//	S = [[nombre, args...], ...]      scriptlets distintos (ordenados)
//	K = {dominio: [índices en S]}
//	X = {dominio: [índices en S]}     excepciones (#@#+js); -1 = todos
func scriptletFile(cat string, data map[string][][]string, except map[string][]string, lib *scriptletLib) []byte {
	sigs := map[string][]string{}
	bases := map[string]bool{}
	for k, l := range data {
		bases[siteBase(k)] = true
		for _, s := range l {
			sigs[scriptletSig(s)] = s
		}
	}
	keys := make([]string, 0, len(sigs))
	for k := range sigs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	idx := make(map[string]int, len(keys))
	S := make([][]string, len(keys))
	for i, k := range keys {
		idx[k] = i
		S[i] = sigs[k]
	}
	K := map[string][]int{}
	for k, l := range data {
		var ids []int
		for _, s := range l {
			ids = append(ids, idx[scriptletSig(s)])
		}
		sort.Ints(ids)
		K[k] = dedupeInts(ids)
	}
	X := map[string][]int{}
	for d, list := range except {
		if !bases[siteBase(d)] && !strings.HasSuffix(d, ".*") {
			continue
		}
		var ids []int
		for _, sig := range list {
			if sig == "*" {
				ids = append(ids, -1)
			} else if i, ok := idx[sig]; ok {
				ids = append(ids, i)
			}
		}
		if len(ids) > 0 {
			sort.Ints(ids)
			X[d] = dedupeInts(ids)
		}
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "/* TurboBLOCK: scriptlets de %q (generado, no editar) */\n(function () {\n'use strict';\nconst S = ", cat)
	b.Write(mustJSON(S, false))
	b.WriteString(";\nconst K = ")
	b.Write(mustJSON(K, false))
	b.WriteString(";\nconst X = ")
	b.Write(mustJSON(X, false))
	b.WriteString(";\n")
	names := make([]string, len(S))
	for i, s := range S {
		names[i] = s[0]
	}
	b.WriteString(lib.build(names))
	b.WriteString("\n})();\n")
	return b.Bytes()
}

func dedupeInts(s []int) []int {
	out := s[:0]
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
}

// ---------- escribir en disco ----------

// writeRules deja los archivos en dir/rules. Primero todos los temporales y
// después los renombres: la carpeta nunca queda con medio trabajo.
func writeRules(extDir string, c *compiled) error {
	dir := filepath.Join(extDir, "rules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	names := make([]string, 0, len(c.Files))
	for n := range c.Files {
		if n != "meta.json" {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	names = append(names, "meta.json") // el último: dice que lo demás ya está
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n+".tmp"), c.Files[n], 0o644); err != nil {
			return err
		}
	}
	for _, n := range names {
		p := filepath.Join(dir, n)
		if err := os.Rename(p+".tmp", p); err != nil {
			return err
		}
	}
	// Archivos de una compilación anterior que ya no existen (p. ej. un grupo menos).
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if _, ok := c.Files[e.Name()]; !ok && !e.IsDir() {
				os.Remove(filepath.Join(dir, e.Name()))
			}
		}
	}
	return nil
}

func readMeta(extDir string) *Meta {
	raw, err := os.ReadFile(filepath.Join(extDir, "rules", "meta.json"))
	if err != nil {
		return nil
	}
	var m Meta
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	return &m
}

// CompileOptions para compileTo.
type CompileOptions struct {
	Fetch FetchOptions
	// Lists: si no está vacío, en vez de las listas de verdad se usan estos archivos
	// ("categoría=ruta"). Lo usan las pruebas.
	Lists []string
	// Extra: filtros que se añaden a las listas de verdad ("categoría=ruta").
	Extra []string
}

// compileTo descarga lo que haga falta, compila y escribe en extDir/rules si algo
// ha cambiado. Devuelve la meta resultante y si se escribió algo.
func compileTo(ctx context.Context, extDir string, o CompileOptions) (*Meta, bool, error) {
	lib, err := os.ReadFile(filepath.Join(extDir, "content", "scriptlets.js"))
	if err != nil {
		return nil, false, fmt.Errorf("no encuentro la extensión en %s: %w", extDir, err)
	}
	var lists map[string]*ListText
	if len(o.Lists) > 0 {
		lists, err = localLists(o.Lists)
	} else {
		lists, _, err = fetchAll(ctx, o.Fetch, extDir)
	}
	if err != nil {
		return nil, false, err
	}
	extra, err := localLists(o.Extra)
	if err != nil {
		return nil, false, err
	}
	for id, lt := range extra {
		if cur := lists[id]; cur != nil {
			cp := *cur
			cp.Text += "\n" + lt.Text
			lists[id] = &cp
		} else {
			lists[id] = lt
		}
	}
	c, err := compileLists(lists, string(lib))
	if err != nil {
		return nil, false, err
	}
	if old := readMeta(extDir); old != nil && old.Build == c.Meta.Build {
		return old, false, nil
	}
	if err := writeRules(extDir, c); err != nil {
		return nil, false, err
	}
	return c.Meta, true, nil
}

// localLists: "ads=ruta.txt" → una lista falsa en esa categoría.
func localLists(specs []string) (map[string]*ListText, error) {
	out := map[string]*ListText{}
	for _, s := range specs {
		cat, p, ok := strings.Cut(s, "=")
		c := categoryByID(cat)
		if !ok || c == nil {
			return nil, fmt.Errorf("lista no válida %q (formato categoría=ruta)", s)
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		// Se añaden a una lista de confianza de la categoría (pueden usar todos los scriptlets).
		src := c.Sources[0]
		for _, x := range c.Sources {
			if x.Trusted {
				src = x
				break
			}
		}
		src.Title = filepath.Base(p)
		src.Trusted = true // listas locales (pruebas): pueden usar todos los scriptlets
		lt := out[src.ID]
		if lt == nil {
			lt = &ListText{Source: src, Fetched: time.Now()}
			out[src.ID] = lt
		}
		lt.Text += "\n" + string(raw)
	}
	return out, nil
}

// genericKey: la primera clase o id del primer compuesto del selector (".ad" en
// "div.ad > img"). Si el elemento con esa clase o id no está en la página, el
// selector no puede coincidir, así que basta con mirar las clases e ids que hay.
// "" si el selector no tiene ninguna (p. ej. "a[href*=ads]"): va a la hoja fija.
func genericKey(sel string) string {
	if strings.ContainsAny(sel, `\,`) {
		return ""
	}
	depth := 0
	for i := 0; i < len(sel); i++ {
		c := sel[i]
		switch {
		case c == '[' || c == '(':
			depth++
		case c == ']' || c == ')':
			depth--
		case depth > 0:
		case c == ' ' || c == '>' || c == '+' || c == '~':
			return "" // fin del primer compuesto sin clase ni id
		case (c == '.' || c == '#') && i+1 < len(sel):
			j := i + 1
			for j < len(sel) && (sel[j] == '-' || sel[j] == '_' || sel[j] >= '0' && sel[j] <= '9' || sel[j] >= 'a' && sel[j] <= 'z' || sel[j] >= 'A' && sel[j] <= 'Z') {
				j++
			}
			if j > i+1 && !(sel[i+1] >= '0' && sel[i+1] <= '9') {
				return sel[i:j]
			}
			return ""
		}
	}
	return ""
}
