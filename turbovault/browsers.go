package main

// Navegadores basados en Chromium donde se puede cargar la extensión. El puente se
// registra también para Edge y Brave, así que el asistente abre la página de
// extensiones del que uses (el predeterminado, si es uno de estos) con los pasos
// de ese navegador, que no llama igual a los botones.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

type browser struct {
	Name   string `json:"name"`   // Chrome, Edge…
	Exe    string `json:"-"`      // ruta del .exe
	Page   string `json:"page"`   // su página de extensiones
	Load   string `json:"load"`   // el botón para cargar una carpeta
	Toggle string `json:"toggle"` // dónde está «Modo de desarrollador»
}

type browserDef struct {
	name, exe, page, load, toggle string
	progID                        string   // prefijo del ProgId del navegador predeterminado
	paths                         []string // relativas a Archivos de programa y a LOCALAPPDATA
}

var browserDefs = []browserDef{
	{"Chrome", "chrome.exe", "chrome://extensions/", "Cargar descomprimida", "arriba a la derecha", "ChromeHTML", []string{`Google\Chrome\Application\chrome.exe`}},
	{"Edge", "msedge.exe", "edge://extensions/", "Cargar desempaquetada", "en la columna de la izquierda", "MSEdgeHTM", []string{`Microsoft\Edge\Application\msedge.exe`}},
	{"Brave", "brave.exe", "brave://extensions/", "Cargar descomprimida", "arriba a la derecha", "BraveHTML", []string{`BraveSoftware\Brave-Browser\Application\brave.exe`}},
	{"Vivaldi", "vivaldi.exe", "vivaldi://extensions/", "Cargar descomprimida", "arriba a la derecha", "VivaldiHTM", []string{`Vivaldi\Application\vivaldi.exe`}},
}

// defaultChrome: lo que se enseña si no se encuentra ningún navegador.
var defaultChrome = browser{Name: "Chrome", Page: "chrome://extensions/", Load: "Cargar descomprimida", Toggle: "arriba a la derecha"}

func findBrowserExe(d browserDef) string {
	key := `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\` + d.exe
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		if k, err := registry.OpenKey(root, key, registry.QUERY_VALUE); err == nil {
			p, _, err := k.GetStringValue("")
			k.Close()
			p = strings.Trim(p, `"`)
			if err == nil && p != "" {
				if _, err := os.Stat(p); err == nil {
					return p
				}
			}
		}
	}
	for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")} {
		if base == "" {
			continue
		}
		for _, rel := range d.paths {
			p := filepath.Join(base, rel)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

// defaultBrowserProgID: el navegador predeterminado para https (vacío si no se sabe).
func defaultBrowserProgID() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\Shell\Associations\UrlAssociations\https\UserChoice`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, _ := k.GetStringValue("ProgId")
	return v
}

// preferredBrowsers: los instalados, con el predeterminado primero y luego Chrome.
func preferredBrowsers() []browser {
	def := defaultBrowserProgID()
	var first, rest []browser
	for _, d := range browserDefs {
		exe := findBrowserExe(d)
		if exe == "" {
			continue
		}
		b := browser{Name: d.name, Exe: exe, Page: d.page, Load: d.load, Toggle: d.toggle}
		if def != "" && strings.HasPrefix(def, d.progID) {
			first = append(first, b)
		} else {
			rest = append(rest, b)
		}
	}
	return append(first, rest...)
}

// openExtensionsPage abre la página de extensiones del navegador que se use.
// Devuelve cuál, y false si no se pudo abrir ninguno.
func openExtensionsPage() (browser, bool) {
	for _, b := range preferredBrowsers() {
		if exec.Command(b.Exe, b.Page).Start() == nil {
			return b, true
		}
	}
	return defaultChrome, false
}

// loadSteps: los pasos para cargar una carpeta en ese navegador.
func loadSteps(b browser, opened bool) string {
	where := "En la página de extensiones de " + b.Name + " que se acaba de abrir:"
	if !opened {
		where = "Abre tu navegador (Chrome, Edge, Brave o Vivaldi) y escribe chrome://extensions (edge://extensions en Edge) en la barra de direcciones. Luego:"
	}
	return where + `

  1. Activa «Modo de desarrollador» (` + b.Toggle + `).
  2. Pulsa «` + b.Load + `».
  3. Pega la ruta en la barra de la ventana (Ctrl+V; ya está copiada) y pulsa «Seleccionar carpeta».`
}
