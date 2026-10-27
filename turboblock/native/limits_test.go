package main

import (
	"strings"
	"testing"
)

// Una lista que no es de uBlock Origin no puede usar scriptlets que escriben
// cookies o cambian enlaces; las de uBO sí.
func TestUntrustedScriptlets(t *testing.T) {
	text := "web.com##+js(set-cookie, consent, 1)\nweb.com##+js(href-sanitizer, a)\nweb.com##+js(aopr, foo)\n"
	untrusted := compileText(t, "ads", text) // EasyList
	js := string(untrusted.Files["ads.sl0.js"])
	if strings.Contains(js, "set-cookie") || strings.Contains(js, "href-sanitizer") {
		t.Error("EasyList no debería poder usar set-cookie ni href-sanitizer")
	}
	if !strings.Contains(js, "abort-on-property-read") {
		t.Error("los scriptlets que solo desactivan cosas siguen valiendo")
	}

	c := categoryByID("ads")
	var ubo Source
	for _, s := range c.Sources {
		if s.Trusted {
			ubo = s
			break
		}
	}
	trusted, err := compileLists(map[string]*ListText{ubo.ID: {Source: ubo, Text: text}}, lib(t))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(trusted.Files["ads.sl0.js"]), "set-cookie") {
		t.Error("las listas de uBO sí pueden usar set-cookie")
	}
}

// $removeparam nunca quita parámetros de inicio de sesión, pagos o enlaces firmados.
func TestRemoveparamSensitive(t *testing.T) {
	c := compileText(t, "privacy", "||web.com^$removeparam=utm_source\n||web.com^$removeparam=code\n||web.com^$removeparam=access_token\n||web.com^$removeparam=X-Amz-Signature\n")
	var params []string
	for _, r := range rules(t, c, "privacy") {
		if r.Action.Redirect != nil && r.Action.Redirect.Transform != nil {
			params = append(params, r.Action.Redirect.Transform.QueryTransform.RemoveParams...)
		}
	}
	if strings.Join(params, ",") != "utm_source" {
		t.Errorf("parámetros que se quitan: %v (solo debería ser utm_source)", params)
	}
	for _, p := range []string{"code", "state", "id_token", "SAMLResponse", "redirect_uri", "authuser", "sessionid"} {
		if !sensitiveParam(p) {
			t.Errorf("%s debería estar protegido", p)
		}
	}
	for _, p := range []string{"utm_campaign", "fbclid", "gclid", "mc_eid"} {
		if sensitiveParam(p) {
			t.Errorf("%s se puede quitar", p)
		}
	}
}

// El aviso de malware gana a las excepciones de otras categorías, pero no a las de
// las propias listas de malware.
func TestMalwarePriority(t *testing.T) {
	mal := compileText(t, "malware", "||malo.com^\n@@||falso-positivo.com^\n")
	var doc, allow int
	for _, r := range rules(t, mal, "malware") {
		if r.Action.Redirect != nil && strings.Contains(r.Action.Redirect.RegexSubstitution, "blocked.html") {
			doc = r.Priority
		}
		if r.Action.Type == "allow" {
			allow = r.Priority
		}
	}
	ads := compileText(t, "ads", "@@||malo.com^$document\n")
	other := 0
	for _, r := range rules(t, ads, "ads") {
		if r.Action.Type == "allowAllRequests" {
			other = r.Priority
		}
	}
	if doc == 0 || allow == 0 || other == 0 {
		t.Fatalf("faltan reglas: aviso %d, excepción de malware %d, excepción de anuncios %d", doc, allow, other)
	}
	if !(doc > other) {
		t.Errorf("una excepción de anuncios (%d) no debería poder con el aviso de malware (%d)", other, doc)
	}
	if !(allow > doc) {
		t.Errorf("las excepciones de las listas de malware (%d) deben poder con su aviso (%d)", allow, doc)
	}
}
