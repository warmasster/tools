// TurboBLOCK.exe: instalador de la extensión, compilador de listas de filtros y
// host de Native Messaging (com.turboblock.native) que las mantiene al día.
//
//	TurboBLOCK.exe                      (doble clic) instalar y dejar la extensión lista
//	TurboBLOCK.exe setup | uninstall    lo que usa TurboSETUP
//	TurboBLOCK.exe update [--force]     descargar listas nuevas y recompilar
//	TurboBLOCK.exe compile --ext DIR    compilar las listas en DIR/rules (build.ps1)
//	TurboBLOCK.exe status
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	hostName    = "com.turboblock.native"
	extensionID = "bdgliednephldclmchggpdnahalgdcif" // fijado con "key" en manifest.json
)

// version la pone build.ps1 a partir de manifest.json.
var version = "dev"

func main() {
	args := os.Args[1:]
	// Chrome pasa el origen de la extensión como primer argumento.
	if len(args) > 0 && strings.HasPrefix(args[0], "chrome-extension://") {
		runHost()
		return
	}
	if len(args) == 0 {
		if launchedByDoubleClick() {
			setup()
			return
		}
		usage()
		return
	}
	switch args[0] {
	case "setup":
		setup()
	case "install", "instalar":
		msg, err := install()
		if err != nil {
			fail(err)
		}
		fmt.Println(msg)
		if dir, err := installExtension(); err != nil {
			fail(fmt.Errorf("no se pudo copiar la extensión: %w", err))
		} else if dir != "" {
			fmt.Println("Extensión en", dir)
			fmt.Println("Cárgala una vez en chrome://extensions → Modo de desarrollador → Cargar descomprimida.")
		}
	case "uninstall", "desinstalar":
		msg, err := uninstall()
		if err != nil {
			fail(err)
		}
		fmt.Println(msg)
	case "update", "actualizar":
		force := hasFlag(args[1:], "--force")
		res := updateAll(context.Background(), force, "")
		if res.Error != "" {
			fail(fmt.Errorf("%s", res.Error))
		}
		if res.Changed {
			fmt.Println("Listas actualizadas:", strings.Join(res.Dirs, ", "))
		} else {
			fmt.Println("Las listas ya estaban al día.")
		}
	case "compile", "compilar":
		os.Exit(cliCompile(args[1:]))
	case "status", "estado":
		printStatus()
	case "version", "--version":
		fmt.Println("TurboBLOCK", version)
	default:
		usage()
		os.Exit(2)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}

func hasFlag(args []string, f string) bool {
	for _, a := range args {
		if a == f {
			return true
		}
	}
	return false
}

func usage() {
	fmt.Print(`TurboBLOCK ` + version + `

Uso:
  TurboBLOCK.exe (doble clic)       instalar y dejar la extensión lista para Chrome
  TurboBLOCK.exe install            lo mismo desde la terminal
  TurboBLOCK.exe uninstall          quitarlo (la extensión se quita en Chrome)
  TurboBLOCK.exe update [--force]   descargar las listas nuevas y recompilarlas
  TurboBLOCK.exe status             ver qué está instalado y dónde
  TurboBLOCK.exe compile --ext DIR [--cache DIR] [--offline] [--force] [--list|--extra cat=archivo]...
                                    compilar las listas en DIR\rules
`)
}

func cliCompile(args []string) int {
	o := CompileOptions{}
	ext := ""
	for i := 0; i < len(args); i++ {
		next := func() string {
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "Falta el valor de %s\n", args[i])
				os.Exit(2)
			}
			i++
			return args[i]
		}
		switch args[i] {
		case "--ext":
			ext = next()
		case "--cache":
			o.Fetch.CacheDir = next()
		case "--offline":
			o.Fetch.Offline = true
		case "--force":
			o.Fetch.Force = true
		case "--list":
			o.Lists = append(o.Lists, next())
		case "--extra":
			o.Extra = append(o.Extra, next())
		default:
			fmt.Fprintf(os.Stderr, "Opción desconocida: %s\n", args[i])
			return 2
		}
	}
	if ext == "" {
		usage()
		return 2
	}
	if o.Fetch.CacheDir == "" {
		o.Fetch.CacheDir = ListsDir()
	}
	t0 := time.Now()
	meta, changed, err := compileTo(context.Background(), ext, o)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	printMeta(meta)
	state := "sin cambios"
	if changed {
		state = "escrito en " + filepath.Join(ext, "rules")
	}
	fmt.Printf("Compilado en %.1f s (%s). Huella %s\n", time.Since(t0).Seconds(), state, meta.Build)
	return 0
}

func printMeta(m *Meta) {
	for _, c := range categories {
		cm := m.Categories[c.ID]
		if cm == nil {
			continue
		}
		fmt.Printf("  %-11s %6d reglas de red (de %d filtros) · %6d genéricos · %6d por dominio · %5d scriptlets\n",
			c.ID, cm.Network, cm.Filters, cm.Generic, cm.Specific, cm.Scriptlets)
	}
}
