package main

// La extensión va dentro del ejecutable: build.ps1 copia sus archivos a ext/ antes de
// compilar. Así un único TurboDL.exe instala el motor y deja la extensión lista para
// cargar en Chrome. En el repositorio ext/ solo tiene LEEME.txt (compilado sin
// build.ps1, el .exe instala únicamente el motor).

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed all:ext
var extFS embed.FS

func hasEmbeddedExtension() bool {
	_, err := fs.Stat(extFS, "ext/manifest.json")
	return err == nil
}

// ExtensionDir es donde se deja la extensión instalada.
func ExtensionDir() string { return filepath.Join(AppDir(), "extension") }

// installExtension copia la extensión incluida a ExtensionDir() (sustituyendo la
// versión anterior). Devuelve "" si este ejecutable no la lleva dentro.
func installExtension() (string, error) {
	if !hasEmbeddedExtension() {
		return "", nil
	}
	dir := ExtensionDir()
	// Borrar antes para que no queden archivos de versiones viejas.
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	err := fs.WalkDir(extFS, "ext", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel("ext", filepath.FromSlash(p))
		if d.Name() == "LEEME.txt" {
			return nil
		}
		dst := filepath.Join(dir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		raw, err := extFS.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, raw, 0o644)
	})
	if err != nil {
		return "", err
	}
	return dir, nil
}
