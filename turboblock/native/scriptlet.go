package main

// Scriptlets: pequeñas funciones que se ejecutan en la página antes que sus
// scripts (mundo MAIN, document_start) para desactivar anuncios, antiadblock y
// ventanas emergentes que no se pueden parar bloqueando peticiones. Las
// implementaciones están en content/scriptlets.js; aquí solo se traducen los
// nombres (y sus alias de uBlock Origin) al nombre canónico.

import "strings"

var scriptletAliases = map[string]string{
	"set-constant": "set-constant", "set": "set-constant",
	"abort-on-property-read": "abort-on-property-read", "aopr": "abort-on-property-read",
	"abort-on-property-write": "abort-on-property-write", "aopw": "abort-on-property-write",
	"abort-current-script": "abort-current-script", "acs": "abort-current-script",
	"abort-current-inline-script": "abort-current-script", "acis": "abort-current-script",
	"abort-on-stack-trace": "abort-on-stack-trace", "aost": "abort-on-stack-trace",
	"addEventListener-defuser": "addEventListener-defuser", "aeld": "addEventListener-defuser",
	"prevent-addEventListener": "addEventListener-defuser",
	"no-setTimeout-if":         "no-setTimeout-if", "nostif": "no-setTimeout-if",
	"prevent-setTimeout": "no-setTimeout-if", "setTimeout-defuser": "no-setTimeout-if",
	"no-setInterval-if": "no-setInterval-if", "nosiif": "no-setInterval-if",
	"prevent-setInterval": "no-setInterval-if", "setInterval-defuser": "no-setInterval-if",
	"no-requestAnimationFrame-if": "no-requestAnimationFrame-if", "norafif": "no-requestAnimationFrame-if",
	"prevent-requestAnimationFrame": "no-requestAnimationFrame-if",
	"nano-setInterval-booster":      "nano-setInterval-booster", "nano-sib": "nano-setInterval-booster",
	"nano-setTimeout-booster": "nano-setTimeout-booster", "nano-stb": "nano-setTimeout-booster",
	"no-window-open-if": "no-window-open-if", "nowoif": "no-window-open-if",
	"prevent-window-open": "no-window-open-if", "window.open-defuser": "no-window-open-if",
	"remove-attr": "remove-attr", "ra": "remove-attr",
	"remove-class": "remove-class", "rc": "remove-class",
	"remove-node-text": "remove-node-text", "rmnt": "remove-node-text",
	"no-fetch-if": "no-fetch-if", "prevent-fetch": "no-fetch-if",
	"no-xhr-if": "no-xhr-if", "prevent-xhr": "no-xhr-if",
	"json-prune":                "json-prune",
	"json-prune-fetch-response": "json-prune-fetch-response", "jpfr": "json-prune-fetch-response",
	"json-prune-xhr-response": "json-prune-xhr-response", "jpxr": "json-prune-xhr-response",
	"set-local-storage-item":   "set-local-storage-item",
	"set-session-storage-item": "set-session-storage-item",
	"remove-cookie":            "remove-cookie", "cookie-remover": "remove-cookie",
	"set-cookie": "set-cookie",
	"nowebrtc":   "nowebrtc",
	"noeval":     "noeval",
	"noeval-if":  "noeval-if", "prevent-eval-if": "noeval-if",
	"popads-dummy": "popads-dummy",
	"bab-defuser":  "bab-defuser", "nobab": "bab-defuser",
	"fuckadblock-defuser": "fuckadblock-defuser", "nofab": "fuckadblock-defuser",
	"href-sanitizer":  "href-sanitizer",
	"prevent-refresh": "prevent-refresh", "refresh-defuser": "prevent-refresh",
	"disable-newtab-links": "disable-newtab-links",
	"window-close-if":      "window-close-if", "close-window": "window-close-if",
	"xml-prune":         "xml-prune",
	"m3u-prune":         "m3u-prune",
	"set-attr":          "set-attr",
	"set-cookie-reload": "set-cookie-reload",
}

// Solo para listas de uBlock Origin (pueden reescribir respuestas o pulsar botones).
var trustedScriptlets = map[string]string{
	"trusted-set-constant": "trusted-set-constant", "trusted-set": "trusted-set-constant",
	"trusted-replace-xhr-response":   "trusted-replace-xhr-response",
	"trusted-replace-fetch-response": "trusted-replace-fetch-response", "trusted-rpfr": "trusted-replace-fetch-response",
	"trusted-set-local-storage-item":   "trusted-set-local-storage-item",
	"trusted-set-cookie":               "trusted-set-cookie",
	"trusted-click-element":            "trusted-click-element",
	"trusted-set-session-storage-item": "trusted-set-session-storage-item",
	"trusted-set-attr":                 "trusted-set-attr",
	"trusted-prevent-xhr":              "trusted-prevent-xhr",
	"trusted-prevent-fetch":            "trusted-prevent-fetch",
	"trusted-prevent-dom-bypass":       "trusted-prevent-dom-bypass",
	"trusted-replace-argument":         "trusted-replace-argument",
	"replace-node-text":                "replace-node-text", "rpnt": "replace-node-text",
	"trusted-replace-node-text": "replace-node-text", "trusted-rpnt": "replace-node-text",
	// Propios de TurboBLOCK (solo en filters/turboblock.txt).
	"tb-video-ads": "tb-video-ads",
}

func canonicalScriptlet(name string, trusted bool) string {
	name = strings.TrimSuffix(strings.TrimSpace(name), ".js")
	if c, ok := scriptletAliases[name]; ok {
		return c
	}
	if trusted {
		return trustedScriptlets[name]
	}
	return ""
}
