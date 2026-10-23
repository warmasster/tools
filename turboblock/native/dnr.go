package main

// Traducción de filtros de red a reglas de declarativeNetRequest (DNR).
//
// La optimización principal: todos los filtros "||dominio^" con las mismas
// opciones se juntan en una sola regla con requestDomains. Chrome comprueba esa
// lista con una tabla hash, así que miles de dominios cuestan lo mismo que uno, y
// el número de reglas baja muchísimo (las listas de dominios enteras quedan en 1).

import (
	"encoding/json"
	"sort"
	"strings"
)

const (
	prioRemoveParam = 5
	prioBlock       = 10
	prioRedirect    = 15 // un filtro $redirect gana a un bloqueo normal
	prioAllow       = 20 // una excepción gana a ambos
	prioImportant   = 30 // $important gana a las excepciones
	prioImpRedirect = 35
	maxRegexRules   = 900 // Chrome admite 1000 por ruleset
	AllowIDBase     = 500000
)

type dnrRule struct {
	ID        int          `json:"id"`
	Priority  int          `json:"priority"`
	Action    dnrAction    `json:"action"`
	Condition dnrCondition `json:"condition"`
}

type dnrAction struct {
	Type     string       `json:"type"`
	Redirect *dnrRedirect `json:"redirect,omitempty"`
}

type dnrRedirect struct {
	ExtensionPath     string        `json:"extensionPath,omitempty"`
	RegexSubstitution string        `json:"regexSubstitution,omitempty"`
	Transform         *dnrTransform `json:"transform,omitempty"`
}

type dnrTransform struct {
	QueryTransform *dnrQueryTransform `json:"queryTransform,omitempty"`
}

type dnrQueryTransform struct {
	RemoveParams []string `json:"removeParams"`
}

type dnrCondition struct {
	URLFilter                string   `json:"urlFilter,omitempty"`
	RegexFilter              string   `json:"regexFilter,omitempty"`
	IsURLFilterCaseSensitive bool     `json:"isUrlFilterCaseSensitive,omitempty"`
	RequestDomains           []string `json:"requestDomains,omitempty"`
	ExcludedRequestDomains   []string `json:"excludedRequestDomains,omitempty"`
	InitiatorDomains         []string `json:"initiatorDomains,omitempty"`
	ExcludedInitiatorDomains []string `json:"excludedInitiatorDomains,omitempty"`
	ResourceTypes            []string `json:"resourceTypes,omitempty"`
	ExcludedResourceTypes    []string `json:"excludedResourceTypes,omitempty"`
	RequestMethods           []string `json:"requestMethods,omitempty"`
	ExcludedRequestMethods   []string `json:"excludedRequestMethods,omitempty"`
	DomainType               string   `json:"domainType,omitempty"`
}

// Recursos de resources/ por los que se cambian scripts e imágenes bloqueados
// (nombres de uBlock Origin). Sin sustituto, el filtro bloquea sin más.
var redirectResources = map[string]string{
	"noop.js": "noop.js", "noopjs": "noop.js", "abp-resource:blank-js": "noop.js",
	"noop.html": "noop.html", "noopframe": "noop.html", "abp-resource:blank-html": "noop.html",
	"noop.txt": "noop.txt", "nooptext": "noop.txt", "empty": "noop.txt",
	"noop.css": "noop.css", "abp-resource:blank-css": "noop.css",
	"noop.json": "noop.json",
	"1x1.gif":   "1x1.gif", "1x1-transparent.gif": "1x1.gif", "abp-resource:1x1-transparent-gif": "1x1.gif",
	"2x2.png": "2x2.png", "2x2-transparent.png": "2x2.png", "abp-resource:2x2-transparent-png": "2x2.png",
	"3x2.png": "3x2.png", "3x2-transparent.png": "3x2.png", "abp-resource:3x2-transparent-png": "3x2.png",
	"32x32.png": "32x32.png", "32x32-transparent.png": "32x32.png", "abp-resource:32x32-transparent-png": "32x32.png",
	"google-analytics_analytics.js": "google-analytics_analytics.js", "google-analytics.com/analytics.js": "google-analytics_analytics.js",
	"googletagmanager_gtm.js": "googletagmanager_gtm.js", "googletagmanager.com/gtm.js": "googletagmanager_gtm.js",
	"google-analytics_ga.js": "google-analytics_analytics.js", "google-analytics.com/ga.js": "google-analytics_analytics.js",
	"googletagservices_gpt.js": "googletagservices_gpt.js", "googletagservices.com/gpt.js": "googletagservices_gpt.js",
	"googlesyndication_adsbygoogle.js": "googlesyndication_adsbygoogle.js", "googlesyndication.com/adsbygoogle.js": "googlesyndication_adsbygoogle.js",
	// Vídeo: el SDK de anuncios de Google y respuestas VAST/VMAP vacías.
	"google-ima.js": "google-ima.js", "google-ima3": "google-ima.js", "imasdk.googleapis.com/js/sdkloader/ima3.js": "google-ima.js",
	"noopvast-2.0": "noop-vast2.xml", "noop-vast2.xml": "noop-vast2.xml",
	"noopvast-3.0": "noop-vast3.xml", "noop-vast3.xml": "noop-vast3.xml",
	"noopvast-4.0": "noop-vast4.xml", "noop-vast4.xml": "noop-vast4.xml",
	"noopvmap-1.0": "noop-vmap1.xml", "noop-vmap1.0.xml": "noop-vmap1.xml", "noop-vmap1.xml": "noop-vmap1.xml",
	"noopmp3-0.1s": "noop-0.1s.mp3", "noop-0.1s.mp3": "noop-0.1s.mp3", "abp-resource:blank-mp3": "noop-0.1s.mp3",
	// Detectores de bloqueadores y otros scripts que las webs esperan.
	"fingerprint2.js": "fingerprint2.js", "fingerprint3.js": "fingerprint2.js",
	"fuckadblock.js-3.2.0": "fuckadblock.js", "nobab.js": "fuckadblock.js", "nobab2.js": "fuckadblock.js",
	"prebid-ads.js": "prebid-ads.js", "amazon_apstag.js": "amazon_apstag.js",
	"noopjson": "noop.json", "none": "noop.txt",
}

var validMethods = map[string]bool{"connect": true, "delete": true, "get": true, "head": true, "options": true, "patch": true, "post": true, "put": true, "other": true}

// netResult: lo que sale de los filtros de red de una categoría.
type netResult struct {
	Rules      []dnrRule
	Regex      int
	PopupHosts []string // dominios cuyas ventanas emergentes se cierran
	PopupAllow []string
	Ghide      []string // dominios sin ocultación genérica
	Shide      []string // dominios sin ocultación específica
}

// buildDNR convierte los filtros de red de una categoría. bad contiene las claves
// anuladas con $badfilter (en cualquier lista).
func buildDNR(filters []*netFilter, bad map[string]bool, strictDoc bool) netResult {
	var res netResult
	var rules []*dnrRule
	groups := map[string]*dnrRule{} // reglas que agrupan dominios
	seen := map[string]bool{}       // reglas sueltas ya emitidas
	var docHosts, docImportant []string
	popupSeen := map[string]bool{}

	add := func(r *dnrRule, host string) {
		if host != "" {
			k := ruleKey(r)
			if g, ok := groups[k]; ok {
				g.Condition.RequestDomains = append(g.Condition.RequestDomains, host)
				return
			}
			r.Condition.RequestDomains = []string{host}
			groups[k] = r
			rules = append(rules, r)
			return
		}
		k := ruleKey(r)
		if seen[k] {
			return
		}
		if r.Condition.RegexFilter != "" {
			if res.Regex >= maxRegexRules {
				return
			}
			res.Regex++
		}
		seen[k] = true
		rules = append(rules, r)
	}

	for _, f := range filters {
		if f.badfilter || bad[f.key] {
			continue
		}
		// Excepciones solo cosméticas.
		if f.allow && (f.ghide || f.shide) {
			if f.host != "" {
				if f.ghide {
					res.Ghide = append(res.Ghide, f.host)
				}
				if f.shide {
					res.Shide = append(res.Shide, f.host)
				}
			}
			continue
		}
		if f.popup && f.host != "" && len(f.domains) == 0 {
			if f.allow {
				res.PopupAllow = append(res.PopupAllow, f.host)
			} else if !popupSeen[f.host] {
				popupSeen[f.host] = true
				res.PopupHosts = append(res.PopupHosts, f.host)
			}
		}
		if f.onlyPopup {
			continue
		}

		types := f.types
		// Bloqueo de la navegación a un dominio entero: página de aviso con la opción
		// de continuar (en vez de la página de error de Chrome).
		blocksDoc := !f.allow && f.host != "" && len(f.to) == 0 && len(f.domains) == 0 && len(f.notDomains) == 0 && f.party == "" &&
			f.redirect == "" && f.removeParm == "" && (f.doc || strictDoc && len(types) == 0 && len(f.notTypes) == 0)
		if blocksDoc {
			if f.important {
				docImportant = append(docImportant, f.host)
			} else {
				docHosts = append(docHosts, f.host)
			}
			if f.doc {
				types = without(types, "main_frame")
				if len(types) == 0 {
					continue
				}
			}
		}

		r := &dnrRule{}
		c := &r.Condition
		if f.regex {
			c.RegexFilter = f.pattern
		} else if f.host == "" || len(f.to) > 0 {
			c.URLFilter = f.pattern
		}
		c.IsURLFilterCaseSensitive = f.matchCase && (c.URLFilter != "" || c.RegexFilter != "")
		c.InitiatorDomains = sortedHosts(f.domains)
		c.ExcludedInitiatorDomains = sortedHosts(f.notDomains)
		if len(f.to) > 0 {
			c.RequestDomains = sortedHosts(f.to)
		}
		c.ExcludedRequestDomains = sortedHosts(f.notTo)
		c.DomainType = f.party
		for _, m := range f.methods {
			if validMethods[m] {
				c.RequestMethods = append(c.RequestMethods, m)
			}
		}
		for _, m := range f.notMethods {
			if validMethods[m] {
				c.ExcludedRequestMethods = append(c.ExcludedRequestMethods, m)
			}
		}
		if len(f.methods) > 0 && len(c.RequestMethods) == 0 {
			continue
		}

		switch {
		case f.removeParm != "":
			if f.allow {
				continue
			}
			r.Priority = prioRemoveParam
			r.Action = dnrAction{Type: "redirect", Redirect: &dnrRedirect{Transform: &dnrTransform{
				QueryTransform: &dnrQueryTransform{RemoveParams: []string{f.removeParm}}}}}
			if len(types) == 0 {
				types = []string{"main_frame", "sub_frame", "xmlhttprequest", "script", "image", "ping", "other"}
			}
		case f.allow:
			r.Priority = prioAllow
			r.Action.Type = "allow"
			if f.doc {
				// @@...$document: la página entera queda sin filtrar.
				ar := *r
				ar.Action = dnrAction{Type: "allowAllRequests"}
				ar.Condition.ResourceTypes = []string{"main_frame", "sub_frame"}
				ar.Condition.ExcludedResourceTypes = nil
				add(&ar, hostIfGroupable(f))
				types = without(without(types, "main_frame"), "sub_frame")
				if len(types) == 0 {
					continue
				}
			}
		case f.redirect != "":
			file, ok := redirectResources[f.redirect]
			if ok {
				r.Priority = prioRedirect
				if f.important {
					r.Priority = prioImpRedirect
				}
				r.Action = dnrAction{Type: "redirect", Redirect: &dnrRedirect{ExtensionPath: "/resources/" + file}}
			} else {
				r.Priority = prioBlock
				if f.important {
					r.Priority = prioImportant
				}
				r.Action.Type = "block"
			}
		default:
			r.Priority = prioBlock
			if f.important {
				r.Priority = prioImportant
			}
			r.Action.Type = "block"
		}
		c.ResourceTypes = sortedCopy(types)
		c.ExcludedResourceTypes = sortedCopy(f.notTypes)
		if len(c.ResourceTypes) > 0 && len(c.ExcludedResourceTypes) > 0 {
			c.ResourceTypes = minus(c.ResourceTypes, c.ExcludedResourceTypes)
			c.ExcludedResourceTypes = nil
			if len(c.ResourceTypes) == 0 {
				continue
			}
		}
		add(r, hostIfGroupable(f))
	}

	for _, d := range []struct {
		hosts []string
		prio  int
	}{{docHosts, prioBlock}, {docImportant, prioImportant}} {
		if len(d.hosts) == 0 {
			continue
		}
		rules = append(rules, &dnrRule{
			Priority: d.prio,
			Action: dnrAction{Type: "redirect", Redirect: &dnrRedirect{
				RegexSubstitution: "chrome-extension://" + extensionID + "/blocked.html#\\0"}},
			Condition: dnrCondition{RegexFilter: "^.+$", RequestDomains: d.hosts, ResourceTypes: []string{"main_frame"}},
		})
		res.Regex++
	}

	// Los ids de las excepciones empiezan en AllowIDBase: la extensión solo cuenta
	// como bloqueos las reglas por debajo (getMatchedRules no dice el tipo).
	res.Rules = make([]dnrRule, 0, len(rules))
	nb, na := 0, 0
	for _, r := range rules {
		if len(r.Condition.RequestDomains) > 0 {
			r.Condition.RequestDomains = compactHosts(r.Condition.RequestDomains)
		}
		if r.Action.Type == "allow" || r.Action.Type == "allowAllRequests" {
			na++
			r.ID = AllowIDBase + na
		} else {
			nb++
			r.ID = nb
		}
		res.Rules = append(res.Rules, *r)
	}
	res.PopupHosts = compactHosts(res.PopupHosts)
	res.Ghide = compactHosts(res.Ghide)
	res.Shide = compactHosts(res.Shide)
	return res
}

func hostIfGroupable(f *netFilter) string {
	if f.host != "" && len(f.to) == 0 && !f.regex {
		return f.host
	}
	return ""
}

func ruleKey(r *dnrRule) string {
	raw, _ := json.Marshal(r)
	return string(raw)
}

func without(s []string, x string) []string {
	var out []string
	for _, v := range s {
		if v != x {
			out = append(out, v)
		}
	}
	return out
}

func minus(a, b []string) []string {
	var out []string
	for _, v := range a {
		if !contains(b, v) {
			out = append(out, v)
		}
	}
	return out
}

func contains(s []string, x string) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}

func sortedCopy(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	out := append([]string(nil), s...)
	sort.Strings(out)
	return dedupe(out)
}

func sortedHosts(s []string) []string { return sortedCopy(s) }

// compactHosts ordena, quita repetidos y quita subdominios cuyo dominio padre ya
// está en la lista (requestDomains ya incluye los subdominios).
func compactHosts(hosts []string) []string {
	if len(hosts) == 0 {
		return nil
	}
	set := make(map[string]bool, len(hosts))
	for _, h := range hosts {
		set[h] = true
	}
	out := make([]string, 0, len(set))
	for h := range set {
		covered := false
		for p := h; ; {
			i := strings.IndexByte(p, '.')
			if i < 0 {
				break
			}
			p = p[i+1:]
			if set[p] {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, h)
		}
	}
	sort.Strings(out)
	return out
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
