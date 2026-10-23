package main

// Comprimir en ZIP (para carpetas y archivos que no se pueden reducir por dentro).

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/flate"
)


// Lo que ya viene comprimido se guarda tal cual: recomprimirlo solo gasta tiempo.
var storedExt = map[string]bool{
	"zip": true, "rar": true, "7z": true, "gz": true, "xz": true, "bz2": true, "zst": true,
	"jpg": true, "jpeg": true, "png": true, "gif": true, "webp": true, "heic": true, "avif": true,
	"mp4": true, "mkv": true, "mov": true, "webm": true, "avi": true, "m4v": true,
	"mp3": true, "m4a": true, "aac": true, "ogg": true, "opus": true, "flac": true,
	"docx": true, "xlsx": true, "pptx": true, "odt": true, "pdf": true, "jar": true, "apk": true, "iso": true,
}

func zipName(files []FileInfo) string {
	if len(files) == 1 {
		n := files[0].Name
		if !files[0].IsDir {
			n = strings.TrimSuffix(n, filepath.Ext(n))
		}
		return n + ".zip"
	}
	if d := filepath.Base(files[0].Dir); d != "" && !strings.HasSuffix(d, `\`) {
		return d + ".zip"
	}
	return "Archivos.zip"
}

type zipEntryIn struct {
	path, name string
	size       int64
	mod        time.Time
	dir        bool
}

func makeZip(ctx context.Context, files []FileInfo, pr progressFn) (string, int64, error) {
	var list []zipEntryIn
	var total int64
	for _, f := range files {
		if !f.IsDir {
			st, err := os.Stat(f.Path)
			if err != nil {
				continue
			}
			list = append(list, zipEntryIn{path: f.Path, name: f.Name, size: st.Size(), mod: st.ModTime()})
			total += st.Size()
			continue
		}
		root := filepath.Base(f.Path)
		filepath.WalkDir(f.Path, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(f.Path, p)
			name := filepath.ToSlash(filepath.Join(root, rel))
			info, e := d.Info()
			if e != nil {
				return nil
			}
			if d.IsDir() {
				list = append(list, zipEntryIn{name: name + "/", dir: true, mod: info.ModTime()})
				return nil
			}
			if d.Type().IsRegular() {
				list = append(list, zipEntryIn{path: p, name: name, size: info.Size(), mod: info.ModTime()})
				total += info.Size()
			}
			return nil
		})
	}
	if len(list) == 0 {
		return "", 0, errors.New("No hay nada que comprimir")
	}
	out := uniquePath(files[0].Dir, zipName(files))
	part := out + ".part"
	fh, err := os.Create(part)
	if err != nil {
		return "", 0, errors.New("No se puede escribir en esa carpeta")
	}
	zw := zip.NewWriter(fh)
	// El compresor de klauspost es varias veces más rápido que el estándar.
	zw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, 5)
	})
	var done int64
	start := time.Now()
	last := time.Time{}
	buf := make([]byte, 1<<20)
	fail := func(e error) (string, int64, error) {
		zw.Close()
		fh.Close()
		os.Remove(part)
		return "", 0, e
	}
	for _, e := range list {
		if ctx.Err() != nil {
			return fail(ctx.Err())
		}
		h := &zip.FileHeader{Name: e.name, Modified: e.mod}
		if e.dir {
			if _, err := zw.CreateHeader(h); err != nil {
				return fail(err)
			}
			continue
		}
		h.Method = zip.Deflate
		if storedExt[lowerExt(e.name)] {
			h.Method = zip.Store
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			return fail(err)
		}
		src, err := os.Open(e.path)
		if err != nil {
			continue // en uso o sin permiso: se salta
		}
		for {
			n, rerr := src.Read(buf)
			if n > 0 {
				if _, werr := w.Write(buf[:n]); werr != nil {
					src.Close()
					return fail(errors.New("No se pudo escribir el ZIP (¿disco lleno?)"))
				}
				done += int64(n)
				if time.Since(last) > 150*time.Millisecond {
					last = time.Now()
					frac := float64(done) / float64(max(total, 1))
					eta := -1.0
					if frac > 0.03 {
						el := time.Since(start).Seconds()
						eta = el/frac - el
					}
					pr(frac, 0, eta, "Comprimiendo")
				}
			}
			if rerr != nil {
				break
			}
			if ctx.Err() != nil {
				src.Close()
				return fail(ctx.Err())
			}
		}
		src.Close()
	}
	if err := zw.Close(); err != nil {
		return fail(err)
	}
	fh.Close()
	if err := os.Rename(part, out); err != nil {
		os.Remove(part)
		return "", 0, err
	}
	return out, total, nil
}
