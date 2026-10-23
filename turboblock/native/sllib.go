package main

// content/scriptlets.js está partida en bloques "//# nombre [needs=a,b]" … "//# end".
// Cada archivo de scriptlets lleva "core", los bloques que usa (con sus
// dependencias) y "run": nada más.

import (
	"fmt"
	"regexp"
	"strings"
)

type libBlock struct {
	name  string
	needs []string
	code  string
}

type scriptletLib struct {
	blocks map[string]*libBlock
	order  []string
}

var libBlockRe = regexp.MustCompile(`(?ms)^//# (\S+)(?: needs=(\S+))?\r?\n(.*?)^//# end\r?$`)

func parseScriptletLib(src string) (*scriptletLib, error) {
	l := &scriptletLib{blocks: map[string]*libBlock{}}
	for _, m := range libBlockRe.FindAllStringSubmatch(src, -1) {
		b := &libBlock{name: m[1], code: m[3]}
		if m[2] != "" {
			b.needs = strings.Split(m[2], ",")
		}
		l.blocks[b.name] = b
		l.order = append(l.order, b.name)
	}
	for _, n := range []string{"core", "run"} {
		if l.blocks[n] == nil {
			return nil, fmt.Errorf("content/scriptlets.js no tiene el bloque %q", n)
		}
	}
	for _, b := range l.blocks {
		for _, n := range b.needs {
			if l.blocks[n] == nil {
				return nil, fmt.Errorf("el scriptlet %q necesita %q, que no existe", b.name, n)
			}
		}
	}
	return l, nil
}

func (l *scriptletLib) has(name string) bool {
	b := l.blocks[name]
	return b != nil && !strings.HasPrefix(name, "_") && name != "core" && name != "run"
}

// build devuelve el código para estos scriptlets, en el orden del archivo.
func (l *scriptletLib) build(names []string) string {
	want := map[string]bool{"core": true, "run": true}
	var add func(string)
	add = func(n string) {
		if want[n] || l.blocks[n] == nil {
			return
		}
		want[n] = true
		for _, d := range l.blocks[n].needs {
			add(d)
		}
	}
	for _, n := range names {
		add(n)
	}
	var b strings.Builder
	for _, n := range l.order {
		if want[n] && n != "run" {
			b.WriteString(l.blocks[n].code)
		}
	}
	b.WriteString(l.blocks["run"].code)
	return b.String()
}
