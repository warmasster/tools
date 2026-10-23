package main

// La extensión (con las reglas ya compiladas) va dentro del ejecutable, en un zip:
// build.ps1 lo deja en ext/extension.zip antes de compilar. Las reglas son JSON
// muy repetitivo y comprimidas ocupan una décima parte. En el repositorio ext/
// solo tiene LEEME.txt.

import (
	"archive/zip"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed all:ext
var extFS embed.FS

func embeddedZip() []byte {
	raw, _ := extFS.ReadFile("ext/extension.zip")
	return raw
}

func hasEmbeddedExtension() bool { return len(embeddedZip()) > 0 }

// unzipTo extrae el zip incluido en dir.
func unzipTo(dir string) error {
	raw := embeddedZip()
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		name := filepath.FromSlash(strings.ReplaceAll(f.Name, "\\", "/"))
		if strings.Contains(name, "..") || filepath.IsAbs(name) {
			continue
		}
		dst := filepath.Join(dir, name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		if err := writeFileAtomic(dst, data); err != nil {
			return err
		}
	}
	return nil
}

// ExtensionDir es donde se deja la extensión instalada.
func ExtensionDir() string { return filepath.Join(AppDir(), "extension") }

// ListsDir guarda las listas descargadas (para pedirlas con If-None-Match).
func ListsDir() string { return filepath.Join(AppDir(), "lists") }

// installExtension copia la extensión incluida a ExtensionDir(), sustituyendo la
// anterior. Si ya había listas descargadas más nuevas que las que trae el .exe,
// recompila con ellas para no volver atrás. Devuelve "" si no la lleva dentro.
func installExtension() (string, error) {
	if !hasEmbeddedExtension() {
		return "", nil
	}
	dir := ExtensionDir()
	tmp := dir + ".new"
	os.RemoveAll(tmp)
	if err := unzipTo(tmp); err != nil {
		os.RemoveAll(tmp)
		return "", err
	}
	// Cambiar la carpeta entera de golpe (sin restos de versiones viejas).
	old := dir + ".old"
	os.RemoveAll(old)
	if _, err := os.Stat(dir); err == nil {
		if err := os.Rename(dir, old); err != nil {
			// Algo la tiene abierta: copiar encima.
			os.RemoveAll(tmp)
			return dir, unzipTo(dir)
		}
	}
	if err := os.Rename(tmp, dir); err != nil {
		os.Rename(old, dir)
		return "", err
	}
	os.RemoveAll(old)

	embedded := readMeta(dir)
	// Solo si todas las listas guardadas son más nuevas que las del .exe.
	if oldest := cacheOldest(ListsDir()); embedded != nil && !oldest.IsZero() && oldest.After(embedded.Built) {
		o := CompileOptions{Fetch: FetchOptions{CacheDir: ListsDir(), Offline: true}}
		if m, changed, err := compileTo(context.Background(), dir, o); err == nil && changed {
			logf("instalación: recompilado con las listas guardadas (%s)", m.Build)
		}
	}
	return dir, nil
}

// cacheOldest: la fecha de comprobación más antigua de las listas guardadas (cero si
// falta alguna).
func cacheOldest(dir string) time.Time {
	var oldest time.Time
	for _, c := range categories {
		for _, src := range c.Sources {
			if src.Local != "" {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, src.ID+".json"))
			if err != nil {
				return time.Time{}
			}
			var m cacheMeta
			if json.Unmarshal(raw, &m) != nil {
				return time.Time{}
			}
			if oldest.IsZero() || m.Fetched.Before(oldest) {
				oldest = m.Fetched
			}
		}
	}
	return oldest
}
