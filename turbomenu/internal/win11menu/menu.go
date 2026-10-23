// Package win11menu añade un submenú al menú contextual moderno de Windows 11.
//
// Funciona con un paquete MSIX «sparse» sin firmar (solo identidad) que declara
// un servidor COM fuera de proceso: el propio .exe, lanzado por el sistema con
// ServerArg, implementa IExplorerCommand en Go puro (sin cgo).
package win11menu

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// Selection es lo que el usuario ha pulsado con el botón derecho.
type Selection struct {
	// Rutas de los elementos seleccionados. En el fondo de una carpeta, la carpeta.
	Paths []string
	// Dirs[i] indica si Paths[i] es una carpeta.
	Dirs []bool
	// Background: clic en el fondo de una carpeta (no sobre un elemento).
	Background bool
	// NonFS: algún elemento no es del sistema de archivos (Este equipo, Papelera…).
	NonFS bool
}

// Command es una entrada del submenú.
type Command struct {
	ID      string // acción que recibe el callback de Serve
	Title   string
	Icon    string // "C:\\ruta\\icono.ico" o "C:\\ruta\\app.exe,0"; vacío = sin icono
	Tooltip string
	// Show decide si se ve para esta selección; nil = siempre.
	Show func(Selection) bool
	// Disabled devuelve "" si está activo o un texto (p. ej. "necesita TurboSEND")
	// que se añade al título entre paréntesis, y entonces sale desactivado.
	Disabled func(Selection) string
	// Sub convierte la entrada en un submenú (anidado).
	Sub []Command
	// SeparatorBefore pone una línea antes de la entrada.
	SeparatorBefore bool
}

// Menu es la entrada raíz («Turbo») con su submenú.
type Menu struct {
	Title    string
	Icon     string
	Commands []Command
}

// Predicados de ayuda para Command.Show.

// Files: solo archivos (ninguna carpeta), y no en el fondo.
func Files(s Selection) bool {
	if s.Background || len(s.Paths) == 0 {
		return false
	}
	for _, d := range s.Dirs {
		if d {
			return false
		}
	}
	return true
}

// Folders: solo carpetas seleccionadas (no el fondo).
func Folders(s Selection) bool {
	if s.Background || len(s.Paths) == 0 {
		return false
	}
	for _, d := range s.Dirs {
		if !d {
			return false
		}
	}
	return true
}

// Items: cualquier selección de elementos (archivos o carpetas), no el fondo.
func Items(s Selection) bool { return !s.Background && len(s.Paths) > 0 }

// Background: clic en el fondo de una carpeta.
func Background(s Selection) bool { return s.Background }

// Single: exactamente un elemento seleccionado.
func Single(s Selection) bool { return !s.Background && len(s.Paths) == 1 }

// Exts: todos son archivos con alguna de estas extensiones (".jpg", ".png"…).
func Exts(exts ...string) func(Selection) bool {
	set := map[string]bool{}
	for _, e := range exts {
		set[strings.ToLower(e)] = true
	}
	return func(s Selection) bool {
		if !Files(s) {
			return false
		}
		for _, p := range s.Paths {
			if !set[strings.ToLower(filepath.Ext(p))] {
				return false
			}
		}
		return true
	}
}

// All combina predicados (todos deben cumplirse).
func All(fs ...func(Selection) bool) func(Selection) bool {
	return func(s Selection) bool {
		for _, f := range fs {
			if !f(s) {
				return false
			}
		}
		return true
	}
}

// Any combina predicados (basta uno).
func Any(fs ...func(Selection) bool) func(Selection) bool {
	return func(s Selection) bool {
		for _, f := range fs {
			if f(s) {
				return true
			}
		}
		return false
	}
}

func isDir(p string) bool {
	u, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return false
	}
	a, err := windows.GetFileAttributes(u)
	return err == nil && a&windows.FILE_ATTRIBUTE_DIRECTORY != 0
}

func (c *Command) visible(s Selection) bool {
	if c.Show != nil && !c.Show(s) {
		return false
	}
	if len(c.Sub) > 0 {
		return anyVisible(c.Sub, s)
	}
	return true
}

func anyVisible(cs []Command, s Selection) bool {
	for i := range cs {
		if cs[i].visible(s) {
			return true
		}
	}
	return false
}
