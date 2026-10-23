package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func lib(t *testing.T) string {
	raw, err := os.ReadFile("../content/scriptlets.js")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func compileText(t *testing.T, cat, text string) *compiled {
	t.Helper()
	c := categoryByID(cat)
	lists := map[string]*ListText{c.Sources[0].ID: {Source: c.Sources[0], Text: text}}
	out, err := compileLists(lists, lib(t))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func rules(t *testing.T, c *compiled, cat string) []dnrRule {
	var r []dnrRule
	if err := json.Unmarshal(c.Files[cat+".json"], &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestDominiosAgrupados(t *testing.T) {
	c := compileText(t, "ads", "||a.com^\n||b.com^\n||sub.a.com^\n||c.com^$third-party\nd.com\n0.0.0.0 e.com\n")
	r := rules(t, c, "ads")
	if len(r) != 2 {
		t.Fatalf("esperaba 2 reglas (una por grupo de opciones), hay %d: %+v", len(r), r)
	}
	got := strings.Join(r[0].Condition.RequestDomains, ",")
	if got != "a.com,b.com,d.com,e.com" {
		t.Errorf("dominios del primer grupo: %s", got)
	}
	if r[1].Condition.DomainType != "thirdParty" {
		t.Errorf("segundo grupo sin thirdParty: %+v", r[1])
	}
}

func TestOpciones(t *testing.T) {
	c := compileText(t, "ads", strings.Join([]string{
		"/banner/*/ad.$script,image,domain=x.com|~y.x.com",
		"@@||ok.com/ads.js$script",
		"||imp.com^$important",
		"||ga.com/analytics.js$script,redirect=google-analytics.com/analytics.js",
		"||x.com^$removeparam=utm_source",
		"||nope.com^$csp=script-src 'none'",
		"||pop.com^$popup",
		"/regex[0-9]+/$xhr",
		"/(?!lookahead)/",
		"||bad.com^$badfilter",
		"||bad.com^",
	}, "\n"))
	r := rules(t, c, "ads")
	find := func(pred func(dnrRule) bool) *dnrRule {
		for i := range r {
			if pred(r[i]) {
				return &r[i]
			}
		}
		return nil
	}
	if x := find(func(r dnrRule) bool { return r.Condition.URLFilter == "/banner/*/ad." }); x == nil ||
		strings.Join(x.Condition.ResourceTypes, ",") != "image,script" || x.Condition.InitiatorDomains[0] != "x.com" || x.Condition.ExcludedInitiatorDomains[0] != "y.x.com" {
		t.Errorf("filtro con opciones mal: %+v", x)
	}
	if x := find(func(r dnrRule) bool { return r.Action.Type == "allow" }); x == nil || x.Priority != prioAllow {
		t.Errorf("excepción mal: %+v", x)
	}
	if x := find(func(r dnrRule) bool { return r.Priority == prioImportant }); x == nil {
		t.Error("falta $important")
	}
	if x := find(func(r dnrRule) bool { return r.Action.Redirect != nil && r.Action.Redirect.ExtensionPath != "" }); x == nil ||
		x.Action.Redirect.ExtensionPath != "/resources/google-analytics_analytics.js" {
		t.Errorf("redirect mal: %+v", x)
	}
	if x := find(func(r dnrRule) bool { return r.Action.Redirect != nil && r.Action.Redirect.Transform != nil }); x == nil {
		t.Error("falta $removeparam")
	}
	if find(func(r dnrRule) bool {
		return strings.Contains(strings.Join(r.Condition.RequestDomains, ","), "nope.com")
	}) != nil {
		t.Error("$csp no se puede expresar: debía descartarse")
	}
	if find(func(r dnrRule) bool {
		return strings.Contains(strings.Join(r.Condition.RequestDomains, ","), "pop.com")
	}) != nil {
		t.Error("$popup no debe ir a DNR")
	}
	if find(func(r dnrRule) bool { return r.Condition.RegexFilter == "regex[0-9]+" }) == nil {
		t.Error("falta la regex")
	}
	if find(func(r dnrRule) bool { return strings.Contains(r.Condition.RegexFilter, "lookahead") }) != nil {
		t.Error("una regex con (?! no vale en Chrome")
	}
	if find(func(r dnrRule) bool {
		return strings.Contains(strings.Join(r.Condition.RequestDomains, ","), "bad.com")
	}) != nil {
		t.Error("$badfilter no anuló el filtro")
	}
	var cf cosmeticFile
	json.Unmarshal(c.Files["ads.cosmetic.json"], &cf)
	if len(cf.Popup) != 1 || cf.Popup[0] != "pop.com" {
		t.Errorf("lista de ventanas emergentes: %v", cf.Popup)
	}
}

func TestMalwareBloqueaNavegacion(t *testing.T) {
	c := compileText(t, "malware", "||malo.com^\n")
	r := rules(t, c, "malware")
	var doc *dnrRule
	for i := range r {
		if r[i].Action.Redirect != nil && r[i].Action.Redirect.RegexSubstitution != "" {
			doc = &r[i]
		}
	}
	if doc == nil || doc.Condition.RequestDomains[0] != "malo.com" || !strings.Contains(doc.Action.Redirect.RegexSubstitution, extensionID+"/blocked.html") {
		t.Fatalf("falta la página de aviso: %+v", r)
	}
}

func TestCosmeticos(t *testing.T) {
	c := compileText(t, "ads", strings.Join([]string{
		"##.anuncio",
		"##div.caja > .ad",
		"##a[href*=\"ads\"]",
		"##.tiene-excepcion",
		"web.com#@#.tiene-excepcion",
		"web.com,~sub.web.com##.solo-web",
		"web.com##.x:has-text(Publi)",
		"web.com##.y:style(opacity: 0 !important)",
		"web.com##.z:style(background: url(x))",
		"##.raro:matches-media(screen)",
		"google.*##.ent",
	}, "\n"))
	var cf cosmeticFile
	if err := json.Unmarshal(c.Files["ads.cosmetic.json"], &cf); err != nil {
		t.Fatal(err)
	}
	if cf.Gen[".anuncio"] == nil || cf.Gen[".caja"] == nil {
		t.Errorf("genéricos por clase: %v", cf.Gen)
	}
	css := string(c.Files["ads.css"])
	if !strings.Contains(css, `a[href*="ads"]`) || strings.Contains(css, ".anuncio") {
		t.Errorf("hoja fija: %s", css)
	}
	if len(cf.Gx) != 1 || cf.Gx[0][0] != ".tiene-excepcion" {
		t.Errorf("genérico con excepción: %v", cf.Gx)
	}
	w := cf.Spec["web.com"]
	if w == nil || len(w.Hide) != 1 || len(w.Proc) != 1 || len(w.CSS) != 1 || len(w.Unhide) != 1 {
		t.Fatalf("web.com: %+v", w)
	}
	if s := cf.Spec["sub.web.com"]; s == nil || s.Unhide[0] != ".solo-web" {
		t.Errorf("~sub.web.com: %+v", s)
	}
	if cf.Spec["google.*"] == nil {
		t.Error("entidad google.*")
	}
}

func TestScriptlets(t *testing.T) {
	c := compileText(t, "ads", strings.Join([]string{
		`web.com##+js(set, a.b, true)`,
		`web.com##+js(aopr, foo)`,
		`otra.com##+js(set, a.b, true)`,
		`web.com##+js(trusted-set, x, 1)`, // no es lista de uBO: fuera
		`web.com##+js(inexistente, 1)`,
		`google.*##+js(nowoif)`,
		`sub.web.com#@#+js(aopr, foo)`,
	}, "\n"))
	js := string(c.Files["ads.sl0.js"])
	for _, want := range []string{`["abort-on-property-read","foo"]`, `["set-constant","a.b","true"]`, `R['set-constant']`, `R['abort-on-property-read']`} {
		if !strings.Contains(js, want) {
			t.Errorf("falta %s en ads.sl0.js", want)
		}
	}
	if strings.Contains(js, "trusted-set-constant") || strings.Contains(js, "inexistente") || strings.Contains(js, "R['json-prune']") {
		t.Error("ads.sl0.js lleva cosas que no debe")
	}
	if !strings.Contains(js, `"sub.web.com":[`) {
		t.Error("falta la excepción de sub.web.com")
	}
	if _, ok := c.Files["ads.sla0.js"]; !ok {
		t.Error("falta el archivo de entidades")
	}
	if c.Meta.Categories["ads"].SlBuckets[0][0] != "otra.com" {
		t.Errorf("dominios del grupo: %v", c.Meta.Categories["ads"].SlBuckets)
	}
}

func TestPreprocesador(t *testing.T) {
	text := "!#if env_firefox\n||ff.com^\n!#else\n||chrome.com^\n!#endif\n!#if !env_mobile && env_chromium\n||movil.com^\n!#endif\n"
	got := strings.Join(preprocess(text), ",")
	if got != "||chrome.com^,||movil.com^" {
		t.Errorf("preprocess: %s", got)
	}
}

func TestArgumentosScriptlet(t *testing.T) {
	got := parseScriptletArgs(`set, a.b, '{"x":1, "y":2}', c\,d`)
	want := []string{"set", "a.b", `{"x":1, "y":2}`, "c,d"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("%q", got)
	}
}

func TestListasReales(t *testing.T) {
	if testing.Short() {
		t.Skip("sin red")
	}
	if _, err := os.Stat(".cache/easylist.txt"); err != nil {
		t.Skip("sin caché de listas")
	}
	lists, _, err := fetchAll(t.Context(), FetchOptions{CacheDir: ".cache", Offline: true}, "..")
	if err != nil {
		t.Fatal(err)
	}
	c, err := compileLists(lists, lib(t))
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, cm := range c.Meta.Categories {
		total += cm.Network
	}
	if total > 30000 {
		t.Errorf("%d reglas: más que el mínimo garantizado de Chrome", total)
	}
}
