package main

// Comprueba que la extensión que tiene cargada el navegador es exactamente la
// que va dentro de TurboVAULT. La extensión se carga "descomprimida" desde una
// carpeta del usuario, así que un programa malicioso podría cambiar sus
// archivos para pedir contraseñas a escondidas. Si no coincide, el navegador no
// recibe ningún secreto hasta que se reinstale desde Ajustes.

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	extCheckMu   sync.Mutex
	extCheckAt   = map[string]time.Time{}
	extCheckRes  = map[string]string{}
	extCheckBad  string // último problema encontrado (para avisar en la ventana)
	extCheckWarn bool
)

// extensionModified devuelve qué archivo no coincide ("" si todo está bien),
// mirando los perfiles normales de los navegadores y, si el navegador usa otra
// carpeta de perfiles, también esa. Se recalcula como mucho cada 15 s.
func extensionModified(userData string) string {
	extCheckMu.Lock()
	defer extCheckMu.Unlock()
	key := strings.ToLower(userData)
	if t, ok := extCheckAt[key]; ok && time.Since(t) < 15*time.Second {
		return extCheckRes[key]
	}
	bases := browserDataDirs()
	if userData != "" {
		bases = append(bases, userData)
	}
	bad := ""
	for _, dir := range chromeExtensionPathsIn(bases) {
		if b := compareExtension(dir); b != "" {
			bad = b
			break
		}
	}
	extCheckAt[key], extCheckRes[key] = time.Now(), bad
	if bad != "" || key == "" {
		extCheckBad = bad
	}
	return bad
}

func compareExtension(dir string) string {
	want := map[string][]byte{}
	fs.WalkDir(extensionFS, "extension", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := extensionFS.ReadFile(p)
			want[strings.TrimPrefix(p, "extension/")] = b
		}
		return nil
	})
	seen := 0
	bad := ""
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || bad != "" {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "_metadata" {
				return filepath.SkipDir // lo crea Chrome
			}
			return nil
		}
		w, ok := want[rel]
		if !ok {
			bad = rel + " (sobra)"
			return nil
		}
		got, err := os.ReadFile(p)
		if err != nil || !bytes.Equal(normalizeEOL(got), normalizeEOL(w)) {
			bad = rel
			return nil
		}
		seen++
		return nil
	})
	if bad == "" && seen != len(want) {
		bad = "faltan archivos"
	}
	return bad
}

// Git puede cambiar los finales de línea de la copia del repositorio.
func normalizeEOL(b []byte) []byte { return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")) }
