package main

// Lectura de la sintaxis de filtros de Adblock Plus / uBlock Origin.
//
// Solo se entiende lo que se puede llevar a Chrome sin pérdida: filtros de red que
// caben en declarativeNetRequest, ocultación de elementos (CSS y procedimental) y
// los scriptlets que implementa content/scriptlets.js. El resto se descarta.

import (
	"regexp"
	"sort"
	"strings"
)

// ---------- preprocesador (!#if / !#else / !#endif) ----------

// Lo que somos para las condiciones de uBlock Origin: Chrome con Manifest V3,
// igual que uBO Lite.
var envTokens = map[string]bool{
	"env_chromium": true, "env_mv3": true, "ext_ublock": true, "ext_ubol": true,
	"cap_user_stylesheet": true, "true": true,
}

func evalCondition(expr string) bool {
	expr = strings.NewReplacer("(", " ", ")", " ").Replace(expr)
	for _, or := range strings.Split(expr, "||") {
		ok := true
		for _, and := range strings.Split(or, "&&") {
			t := strings.TrimSpace(and)
			neg := false
			for strings.HasPrefix(t, "!") {
				neg = !neg
				t = strings.TrimSpace(t[1:])
			}
			if envTokens[t] == neg {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// preprocess devuelve las líneas útiles de una lista, ya sin comentarios y con
// los bloques !#if resueltos.
func preprocess(text string) []string {
	var out []string
	type frame struct{ active, parent bool }
	var stack []frame
	active := true
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "!#if "):
			cond := evalCondition(line[5:])
			stack = append(stack, frame{active: active && cond, parent: active})
			active = active && cond
			continue
		case strings.HasPrefix(line, "!#else"):
			if n := len(stack); n > 0 {
				f := stack[n-1]
				stack[n-1].active = f.parent && !f.active
				active = stack[n-1].active
			}
			continue
		case strings.HasPrefix(line, "!#endif"):
			if n := len(stack); n > 0 {
				active = stack[n-1].parent
				stack = stack[:n-1]
			}
			continue
		}
		if !active || line[0] == '!' || line[0] == '[' {
			continue
		}
		// "# comentario" de los archivos hosts.
		if line[0] == '#' && !strings.HasPrefix(line, "##") && !strings.HasPrefix(line, "#@#") && !strings.HasPrefix(line, "#?#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// ---------- filtros de red ----------

type netFilter struct {
	raw        string
	allow      bool
	pattern    string // sintaxis urlFilter (||, |, ^, *) o expresión regular
	regex      bool
	host       string // si el filtro es exactamente ||host^
	types      []string
	notTypes   []string
	party      string // "", "firstParty", "thirdParty"
	domains    []string
	notDomains []string
	to         []string
	notTo      []string
	methods    []string
	notMethods []string
	important  bool
	matchCase  bool
	redirect   string
	removeParm string
	doc        bool // incluye la navegación principal (main_frame)
	popup      bool // se aplica a ventanas emergentes
	onlyPopup  bool // solo a ventanas emergentes
	ghide      bool // excepción: sin ocultación genérica
	shide      bool // excepción: sin ocultación específica
	badfilter  bool
	key        string // forma normalizada, para $badfilter
}

var allTypes = []string{"main_frame", "sub_frame", "stylesheet", "script", "image", "font", "object", "xmlhttprequest", "ping", "media", "websocket", "webtransport", "webbundle", "other"}

var typeNames = map[string]string{
	"script": "script", "image": "image", "stylesheet": "stylesheet", "css": "stylesheet",
	"object": "object", "object-subrequest": "object",
	"xmlhttprequest": "xmlhttprequest", "xhr": "xmlhttprequest",
	"subdocument": "sub_frame", "frame": "sub_frame",
	"ping": "ping", "beacon": "ping", "websocket": "websocket", "media": "media",
	"font": "font", "other": "other", "webtransport": "webtransport", "webbundle": "webbundle",
}

// Opciones que no se pueden expresar en declarativeNetRequest: el filtro entero se descarta.
var unsupportedOpts = map[string]bool{
	"csp": true, "header": true, "removeheader": true, "replace": true, "urlskip": true,
	"uritransform": true, "urltransform": true, "permissions": true, "cname": true,
	"ipaddress": true, "inline-script": true, "inline-font": true, "popunder": true,
	"rewrite": true, "redirect-rule": true, "genericblock": true, "webrtc": true,
	"mp4": true, "empty": true, "stealth": true, "content": true, "jsinject": true,
	"urlblock": true, "extension": true, "app": true, "network": true, "sitekey": true,
	"requestheader": true, "responseheader": true, "cookie": true, "hls": true, "jsonprune": true,
	"referrerpolicy": true, "ad": true, "elemhide-": true,
}

var (
	optStartRe = regexp.MustCompile(`^~?[a-z_0-9-]+`)
	hostOnlyRe = regexp.MustCompile(`^\|\|([a-z0-9.\-_]+)\^\|?$`)
	backrefRe  = regexp.MustCompile(`\\[1-9]`)
	labelRe    = regexp.MustCompile(`^[a-z0-9-]+$`)
	pseudoRe   = regexp.MustCompile(`:([a-z-]+)`)
)

var hostLike = regexp.MustCompile(`^[a-z0-9]([a-z0-9_-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9_-]*[a-z0-9])?)+$`)

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

// splitOptions separa "patrón$opciones". En una regex /.../$opts el $ va justo
// después de la barra final.
func splitOptions(line string) (string, string) {
	// Regex "/.../$opciones": el $ va justo después de la barra final.
	if strings.HasPrefix(line, "/") {
		if i := strings.LastIndex(line, "/$"); i > 0 && optStartRe.MatchString(line[i+2:]) {
			return line[:i+1], line[i+2:]
		}
		if strings.HasSuffix(line, "/") {
			return line, ""
		}
	}
	i := strings.LastIndexByte(line, '$')
	if i < 0 {
		return line, ""
	}
	opts := line[i+1:]
	// "$" dentro del patrón (p. ej. "/ad$1.js"): las opciones tienen que parecer opciones.
	if opts == "" || !optStartRe.MatchString(opts) {
		return line, ""
	}
	return line[:i], opts
}

// splitOptionList separa por comas que no estén escapadas ni dentro de una regex.
func splitOptionList(s string) []string {
	var out []string
	var cur strings.Builder
	inRe := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s):
			cur.WriteByte(c)
			cur.WriteByte(s[i+1])
			i++
			continue
		case c == '/' && (i == 0 || s[i-1] == '=' || s[i-1] == '|' || inRe):
			inRe = !inRe
		case c == ',' && !inRe:
			out = append(out, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	return append(out, cur.String())
}

func parseDomainList(v string, sep string) (pos, neg []string, ok bool) {
	ok = true
	hadPos := false
	for _, d := range strings.Split(v, sep) {
		d = strings.TrimSpace(strings.ToLower(d))
		if d == "" {
			continue
		}
		n := strings.HasPrefix(d, "~")
		d = strings.TrimPrefix(d, "~")
		if !n {
			hadPos = true
		}
		// Ni regex, ni comodines ("google.*"), ni dominios con acentos sin punycode.
		if strings.ContainsAny(d, "*/") || !isASCII(d) || !hostLike.MatchString(d) && !isIP(d) {
			continue
		}
		if n {
			neg = append(neg, d)
		} else {
			pos = append(pos, d)
		}
	}
	// Había dominios concretos pero ninguno se puede expresar: mejor no aplicar el
	// filtro que aplicarlo en todas partes.
	if hadPos && len(pos) == 0 {
		ok = false
	}
	return
}

var ipRe = regexp.MustCompile(`^\d{1,3}(\.\d{1,3}){3}$`)

func isIP(s string) bool { return ipRe.MatchString(s) }

func parseNetFilter(line string) *netFilter {
	f := &netFilter{raw: line}
	if strings.HasPrefix(line, "@@") {
		f.allow = true
		line = line[2:]
	}
	// Formato hosts: "0.0.0.0 dominio".
	if strings.HasPrefix(line, "0.0.0.0 ") || strings.HasPrefix(line, "127.0.0.1 ") {
		h := strings.ToLower(strings.TrimSpace(strings.Fields(line)[1]))
		if h == "localhost" || h == "0.0.0.0" || !hostLike.MatchString(h) {
			return nil
		}
		line = "||" + h + "^"
	}
	pat, opts := splitOptions(line)
	if opts != "" {
		if !f.parseOptions(opts) {
			return nil
		}
	}
	if pat == "" && opts == "" {
		return nil
	}
	if strings.HasPrefix(pat, "/") && strings.HasSuffix(pat, "/") && len(pat) > 2 {
		f.regex = true
		f.pattern = pat[1 : len(pat)-1]
		if !f.matchCase {
			f.pattern = strings.ToLower(f.pattern)
		}
		if !validRegex(f.pattern) {
			return nil
		}
	} else {
		if !isASCII(pat) || strings.ContainsAny(pat, " \t") {
			return nil
		}
		if !f.matchCase {
			pat = strings.ToLower(pat)
		}
		// Un nombre de dominio solo (listas de dominios) equivale a ||dominio^.
		if hostLike.MatchString(pat) && !strings.HasSuffix(pat, ".") && opts == "" || isIP(pat) {
			pat = "||" + pat + "^"
		}
		if strings.HasPrefix(pat, "||*") {
			pat = strings.TrimPrefix(pat, "||")
		}
		pat = strings.TrimLeft(pat, "*")
		if !strings.HasSuffix(pat, "\\*") {
			pat = strings.TrimRight(pat, "*")
		}
		if pat == "|" || pat == "||" || pat == "^" {
			pat = ""
		}
		if m := hostOnlyRe.FindStringSubmatch(pat); m != nil && (hostLike.MatchString(m[1]) || isIP(m[1])) {
			f.host = m[1]
		}
		f.pattern = pat
	}
	if f.pattern == "" && len(f.domains) == 0 && len(f.to) == 0 && !f.onlyPopup {
		// "*$third-party" y parecidos: bloquearían media web.
		if !f.allow || (len(f.notDomains) == 0 && !f.ghide && !f.shide) {
			return nil
		}
	}
	f.key = normalizedKey(pat, opts)
	return f
}

// normalizedKey: patrón + opciones ordenadas y sin badfilter. Dos filtros con el
// mismo key son "el mismo" para $badfilter.
func normalizedKey(pat, opts string) string {
	var o []string
	for _, x := range splitOptionList(opts) {
		x = strings.TrimSpace(x)
		if x != "" && x != "badfilter" {
			o = append(o, x)
		}
	}
	sort.Strings(o)
	return strings.ToLower(pat) + "$" + strings.Join(o, ",")
}

func (f *netFilter) parseOptions(opts string) bool {
	var types, notTypes []string
	for _, o := range splitOptionList(opts) {
		o = strings.TrimSpace(o)
		if o == "" || strings.Trim(o, "_") == "" {
			continue
		}
		name, val, hasVal := strings.Cut(o, "=")
		name = strings.ToLower(name)
		neg := strings.HasPrefix(name, "~")
		bare := strings.TrimPrefix(name, "~")
		if t, ok := typeNames[bare]; ok {
			if neg {
				notTypes = append(notTypes, t)
			} else {
				types = append(types, t)
			}
			continue
		}
		switch bare {
		case "third-party", "3p", "strict3p":
			if neg {
				f.party = "firstParty"
			} else {
				f.party = "thirdParty"
			}
		case "first-party", "1p", "strict1p":
			if neg {
				f.party = "thirdParty"
			} else {
				f.party = "firstParty"
			}
		case "document", "doc":
			if neg {
				continue
			}
			f.doc = true
			types = append(types, "main_frame")
		case "popup":
			if neg {
				continue
			}
			f.popup = true
		case "all":
			f.popup, f.doc = true, true
			types = append(types, allTypes...)
		case "domain", "from":
			if !hasVal {
				return false
			}
			pos, n, ok := parseDomainList(val, "|")
			if !ok {
				return false
			}
			f.domains, f.notDomains = append(f.domains, pos...), append(f.notDomains, n...)
		case "to":
			pos, n, ok := parseDomainList(val, "|")
			if !ok {
				return false
			}
			f.to, f.notTo = append(f.to, pos...), append(f.notTo, n...)
		case "denyallow":
			pos, _, _ := parseDomainList(val, "|")
			f.notTo = append(f.notTo, pos...)
		case "method":
			for _, m := range strings.Split(strings.ToLower(val), "|") {
				if strings.HasPrefix(m, "~") {
					f.notMethods = append(f.notMethods, m[1:])
				} else if m != "" {
					f.methods = append(f.methods, m)
				}
			}
		case "important":
			f.important = true
		case "match-case":
			f.matchCase = true
		case "badfilter":
			f.badfilter = true
		case "redirect":
			// "nombre:prioridad" (pero "abp-resource:blank-js" es un nombre entero).
			if i := strings.LastIndexByte(val, ':'); i > 0 && isDigits(strings.TrimPrefix(val[i+1:], "-")) {
				val = val[:i]
			}
			f.redirect = val
		case "removeparam", "queryprune":
			if !hasVal || val == "" || strings.HasPrefix(val, "/") || strings.HasPrefix(val, "~") || strings.ContainsAny(val, "|*") {
				return false
			}
			f.removeParm = val
		case "elemhide", "ehide":
			f.ghide, f.shide = true, true
		case "generichide", "ghide":
			f.ghide = true
		case "specifichide", "shide":
			f.shide = true
		case "reason", "_":
			// sin efecto
		default:
			if unsupportedOpts[bare] {
				return false
			}
			return false // opción desconocida: mejor no adivinar
		}
	}
	// Solo para ventanas emergentes: lo gestiona el bloqueador de ventanas, no DNR.
	if f.popup && len(types) == 0 && len(notTypes) == 0 {
		f.onlyPopup = true
	}
	f.types = dedupe(types)
	f.notTypes = dedupe(notTypes)
	if (f.ghide || f.shide) && !f.allow {
		return false
	}
	return true
}

// validRegex: RE2 (lo que acepta Chrome), sin construcciones de JavaScript que
// RE2 no tiene y sin expresiones enormes (Chrome las rechaza por memoria).
func validRegex(p string) bool {
	if len(p) > 220 || strings.Contains(p, "(?=") || strings.Contains(p, "(?!") || strings.Contains(p, "(?<") || !isASCII(p) {
		return false
	}
	if backrefRe.MatchString(p) {
		return false
	}
	_, err := regexp.Compile(p)
	return err == nil
}

func dedupe(s []string) []string {
	if len(s) < 2 {
		return s
	}
	seen := map[string]bool{}
	out := s[:0]
	for _, x := range s {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// ---------- filtros cosméticos ----------

type cosKind int

const (
	cosHide   cosKind = iota // selector CSS normal
	cosProc                  // selector procedimental (:has-text, :upward, :remove()...)
	cosStyle                 // selector:style(declaraciones)
	cosScript                // +js(...)
)

type cosFilter struct {
	domains    []string // incluye entidades "google.*"
	notDomains []string
	exception  bool
	kind       cosKind
	selector   string
	style      string   // para cosStyle
	scriptlet  []string // nombre canónico + argumentos
}

var cosSepRe = regexp.MustCompile(`#@?[?$%]?#`)

// Operadores procedimentales de uBO y sus alias de ABP/AdGuard.
var procAliases = strings.NewReplacer(
	":-abp-has(", ":has(", ":-abp-contains(", ":has-text(", ":contains(", ":has-text(",
	":nth-ancestor(", ":upward(", ":if(", ":has(", ":if-not(", ":not(",
)
var procOpRe = regexp.MustCompile(`:(has-text|upward|remove|style|matches-css|matches-css-before|matches-css-after|matches-attr|matches-path|min-text-length|xpath|watch-attr|remove-attr|remove-class|matches-media|matches-prop|others|-abp-properties|shadow)\(`)
var procSupported = map[string]bool{
	"has-text": true, "upward": true, "remove": true, "style": true, "matches-css": true,
	"matches-css-before": true, "matches-css-after": true, "matches-attr": true,
	"matches-path": true, "min-text-length": true, "xpath": true, "watch-attr": true,
	"remove-attr": true, "remove-class": true,
}

// parseCosmetic devuelve isCos=false si la línea no es un filtro cosmético (y
// c=nil si lo es pero no se puede usar).
func parseCosmetic(line string, trusted bool) (c *cosFilter, isCos bool) {
	loc := cosSepRe.FindStringIndex(line)
	if loc == nil {
		return nil, false
	}
	dom, sep, body := line[:loc[0]], line[loc[0]:loc[1]], strings.TrimSpace(line[loc[1]:])
	// La parte de dominios no puede tener cosas de URL: entonces es un filtro de red.
	if strings.ContainsAny(dom, "/|^$= ") || body == "" {
		return nil, false
	}
	c = &cosFilter{exception: strings.Contains(sep, "@")}
	for _, d := range strings.Split(strings.ToLower(dom), ",") {
		d = strings.TrimSpace(d)
		if d == "" || d == "*" {
			continue
		}
		n := strings.HasPrefix(d, "~")
		d = strings.TrimPrefix(d, "~")
		if !isASCII(d) || strings.HasPrefix(d, "/") {
			if !n {
				return nil, true // dominio que no sabemos tratar: no aplicarlo a todo
			}
			continue
		}
		if !(hostLike.MatchString(d) || isIP(d) || entityRe.MatchString(d) || labelRe.MatchString(d)) {
			if !n {
				return nil, true
			}
			continue
		}
		if n {
			c.notDomains = append(c.notDomains, d)
		} else {
			c.domains = append(c.domains, d)
		}
	}

	switch {
	case sep == "#%#" || sep == "#@%#" || strings.HasPrefix(body, "^"):
		return nil, true // JavaScript de AdGuard y filtrado de HTML: no
	case sep == "#$#" || sep == "#@$#":
		// Fragmentos de ABP: solo los que tienen equivalente en uBO.
		return parseABPSnippet(c, body), true
	case strings.HasPrefix(body, "+js("):
		// +js(...) tiene que ser todo el filtro (sin ":matches-path()" detrás, etc.).
		if !strings.HasSuffix(body, ")") || strings.Contains(body, "):") {
			return nil, true
		}
		c.kind = cosScript
		args := parseScriptletArgs(body[4 : len(body)-1])
		if len(args) == 0 || args[0] == "" {
			if c.exception {
				c.scriptlet = []string{"*"} // #@#+js(): ningún scriptlet en estos dominios
				return c, true
			}
			return nil, true
		}
		name := canonicalScriptlet(args[0], trusted)
		if name == "" {
			return nil, true
		}
		c.scriptlet = append([]string{name}, args[1:]...)
		return c, true
	}

	sel := procAliases.Replace(body)
	if !balanced(sel) || strings.ContainsAny(sel, "{}") && !strings.Contains(sel, ":style(") {
		return nil, true
	}
	ops := procOpRe.FindAllStringSubmatch(sel, -1)
	if len(ops) == 0 {
		if !validCSS(sel) {
			return nil, true
		}
		c.kind = cosHide
		c.selector = sel
		return c, true
	}
	for _, m := range ops {
		if !procSupported[m[1]] {
			return nil, true
		}
	}
	// "selector:style(...)" con selector normal: va directo a la hoja de estilos.
	if len(ops) == 1 && ops[0][1] == "style" && strings.HasSuffix(sel, ")") {
		i := strings.LastIndex(sel, ":style(")
		css, decl := sel[:i], sel[i+7:len(sel)-1]
		if validCSS(css) && validStyle(decl) {
			c.kind = cosStyle
			c.selector, c.style = css, decl
			return c, true
		}
		return nil, true
	}
	if strings.Contains(sel, ":style(") {
		i := strings.LastIndex(sel, ":style(")
		if !validStyle(sel[i+7 : len(sel)-1]) {
			return nil, true
		}
	}
	c.kind = cosProc
	c.selector = sel
	return c, true
}

var entityRe = regexp.MustCompile(`^([a-z0-9-]+\.)*[a-z0-9-]+\.\*$`)

func balanced(s string) bool {
	var stack []byte
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' {
			i++
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '(', '[':
			stack = append(stack, c)
		case ')', ']':
			if len(stack) == 0 {
				return false
			}
			open := stack[len(stack)-1]
			if c == ')' && open != '(' || c == ']' && open != '[' {
				return false
			}
			stack = stack[:len(stack)-1]
		}
	}
	return quote == 0 && len(stack) == 0
}

var badCSS = regexp.MustCompile(`(?i)(^[>+~]|[,>+~]\s*$|::?-(abp|moz|ms)-|:(not|has|is|where)\(\s*\)|@|\burl\(|\\$|</?\w+>|::(before|after)\b.*\S)`)

// validCSS hace una comprobación rápida: un selector inválido dentro de un grupo
// invalida toda la regla en Chrome, así que mejor descartar de más.
func validCSS(sel string) bool {
	if sel == "" || len(sel) > 1000 || !isASCII(sel) && strings.ContainsAny(sel, "{};") {
		return false
	}
	if strings.ContainsAny(sel, "{};") || badCSS.MatchString(sel) || !balanced(sel) {
		return false
	}
	// Pseudoclases que Chrome no conoce.
	for _, m := range pseudoRe.FindAllStringSubmatch(sel, -1) {
		if !knownPseudo[m[1]] {
			// Puede ser parte de un valor de atributo entre comillas.
			if !insideQuotes(sel, m[0]) {
				return false
			}
		}
	}
	return true
}

func insideQuotes(sel, frag string) bool {
	i := strings.Index(sel, frag)
	q := 0
	for j := 0; j < i; j++ {
		if sel[j] == '"' || sel[j] == '\'' {
			q++
		}
	}
	return q%2 == 1
}

var knownPseudo = map[string]bool{
	"not": true, "has": true, "is": true, "where": true, "first-child": true, "last-child": true,
	"only-child": true, "nth-child": true, "nth-last-child": true, "nth-of-type": true,
	"nth-last-of-type": true, "first-of-type": true, "last-of-type": true, "only-of-type": true,
	"empty": true, "root": true, "scope": true, "link": true, "visited": true, "hover": true,
	"focus": true, "focus-within": true, "focus-visible": true, "active": true, "checked": true,
	"disabled": true, "enabled": true, "required": true, "optional": true, "read-only": true,
	"read-write": true, "placeholder-shown": true, "target": true, "lang": true, "dir": true,
	"before": true, "after": true, "first-line": true, "first-letter": true, "defined": true,
	"any-link": true, "indeterminate": true, "default": true, "valid": true, "invalid": true,
	"in-range": true, "out-of-range": true, "fullscreen": true, "modal": true, "popover-open": true,
	"autofill": true, "picture-in-picture": true, "open": true, "closed": true,
}

func validStyle(decl string) bool {
	d := strings.ToLower(decl)
	return decl != "" && !strings.ContainsAny(decl, "{}\\<>") && !strings.Contains(d, "url(") &&
		!strings.Contains(d, "expression") && !strings.Contains(d, "image-set")
}

// parseScriptletArgs separa los argumentos de +js(a, b, c) como uBO: comas
// escapadas con "\," y argumentos opcionalmente entre comillas.
func parseScriptletArgs(s string) []string {
	var out []string
	i := 0
	for i <= len(s) {
		for i < len(s) && s[i] == ' ' {
			i++
		}
		var arg strings.Builder
		if i < len(s) && (s[i] == '\'' || s[i] == '"' || s[i] == '`') {
			q := s[i]
			j := i + 1
			for j < len(s) && s[j] != q {
				if s[j] == '\\' && j+1 < len(s) && s[j+1] == q {
					arg.WriteByte(q)
					j += 2
					continue
				}
				arg.WriteByte(s[j])
				j++
			}
			rest := s[min(j+1, len(s)):]
			k := strings.IndexByte(rest, ',')
			if strings.TrimSpace(rest) == "" || k >= 0 && strings.TrimSpace(rest[:k]) == "" {
				out = append(out, arg.String())
				if k < 0 {
					return out
				}
				i = j + 1 + k + 1
				continue
			}
			arg.Reset() // comilla que no cierra el argumento: se lee tal cual
		}
		j := i
		for j < len(s) && s[j] != ',' {
			if s[j] == '\\' && j+1 < len(s) && s[j+1] == ',' {
				arg.WriteByte(',')
				j += 2
				continue
			}
			arg.WriteByte(s[j])
			j++
		}
		out = append(out, strings.TrimSpace(arg.String()))
		if j >= len(s) {
			break
		}
		i = j + 1
	}
	return out
}

// Fragmentos de ABP con equivalente en uBO.
var abpSnippets = map[string]string{
	"abort-on-property-read": "abort-on-property-read", "abort-on-property-write": "abort-on-property-write",
	"abort-current-inline-script": "abort-current-script", "json-prune": "json-prune",
	"override-property-read": "set-constant", "cookie-remover": "remove-cookie",
	"prevent-listener": "addEventListener-defuser",
}

func parseABPSnippet(c *cosFilter, body string) *cosFilter {
	// Solo un fragmento por filtro (los encadenados con ";" se descartan).
	if strings.Contains(body, ";") || strings.Contains(body, "{") {
		return nil
	}
	f := strings.Fields(body)
	if len(f) == 0 {
		return nil
	}
	name, ok := abpSnippets[f[0]]
	if !ok {
		return nil
	}
	c.kind = cosScript
	args := f[1:]
	if name == "json-prune" {
		args = []string{strings.Join(args, " ")}
	}
	c.scriptlet = append([]string{name}, args...)
	return c
}
