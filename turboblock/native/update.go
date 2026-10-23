package main

// Actualización de las listas en la extensión ya instalada: descarga lo que haya
// caducado (peticiones condicionales, casi siempre 304), recompila y escribe en
// rules/ de cada carpeta desde la que Chrome tiene cargado TurboBLOCK. La
// extensión, al ver una huella nueva, se recarga sola.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type UpdateResult struct {
	Changed bool     `json:"changed"`
	Build   string   `json:"build,omitempty"`
	Built   string   `json:"built,omitempty"`
	Dirs    []string `json:"dirs,omitempty"`
	Error   string   `json:"error,omitempty"`
}

var updateMu sync.Mutex

// extensionDirs: la copia instalada y las carpetas desde las que está cargada en
// los navegadores (p. ej. la del repositorio, en desarrollo).
func extensionDirs() []string {
	var out []string
	seen := map[string]bool{}
	for _, d := range append([]string{ExtensionDir()}, chromeExtensionPaths()...) {
		d = filepath.Clean(d)
		k := strings.ToLower(d)
		if seen[k] {
			continue
		}
		seen[k] = true
		if _, err := os.Stat(filepath.Join(d, "content", "scriptlets.js")); err == nil {
			out = append(out, d)
		}
	}
	return out
}

func updateAll(ctx context.Context, force bool, _ string) UpdateResult {
	updateMu.Lock()
	defer updateMu.Unlock()
	dirs := extensionDirs()
	if len(dirs) == 0 {
		return UpdateResult{Error: "no encuentro la extensión instalada; vuelve a abrir TurboBLOCK.exe"}
	}
	o := CompileOptions{Fetch: FetchOptions{CacheDir: ListsDir(), Force: force}}
	var res UpdateResult
	for _, d := range dirs {
		meta, changed, err := compileTo(ctx, d, o)
		if err != nil {
			logf("actualizar %s: %v", d, err)
			if res.Error == "" {
				res.Error = err.Error()
			}
			continue
		}
		// Ya descargado: el resto de carpetas solo recompilan.
		o.Fetch.Offline = true
		res.Build = meta.Build
		res.Built = meta.Built.Format("2006-01-02T15:04:05Z")
		if changed {
			res.Changed = true
			res.Dirs = append(res.Dirs, d)
			logf("listas actualizadas en %s (huella %s)", d, meta.Build)
		}
	}
	if res.Build != "" {
		res.Error = ""
	}
	return res
}
