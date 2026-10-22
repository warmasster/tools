package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

const inChunk = 16 << 20 // Cloudflare corta peticiones de más de 100 MB

type offerFile struct {
	Rel  string `json:"rel"`
	Size int64  `json:"size"`
}

var reservedNames = map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true}

// safeComponent limpia un trozo de ruta para que sea válido y no salga de su carpeta.
func safeComponent(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, s)
	s = strings.TrimRight(strings.TrimSpace(s), ". ")
	if s == "" || s == "." || s == ".." {
		s = "_"
	}
	base := strings.ToUpper(strings.SplitN(s, ".", 2)[0])
	if reservedNames[base] {
		s = "_" + s
	}
	if len([]rune(s)) > 180 {
		s = string([]rune(s)[:180])
	}
	return s
}

func safeRel(rel string) []string {
	var out []string
	for _, p := range strings.Split(strings.ReplaceAll(rel, `\`, "/"), "/") {
		if p == "" || p == "." || p == ".." {
			continue
		}
		out = append(out, safeComponent(p))
	}
	if len(out) == 0 {
		out = []string{"archivo"}
	}
	return out
}

// uniqueName devuelve un nombre libre dentro de dir ("foto (2).jpg" si ya existe).
func uniqueName(dir, name string, taken map[string]bool) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for n := 1; n < 10000; n++ {
		cand := name
		if n > 1 {
			cand = fmt.Sprintf("%s (%d)%s", base, n, ext)
		}
		if taken[strings.ToLower(cand)] {
			continue
		}
		p := filepath.Join(dir, cand)
		if _, err := os.Stat(p); err == nil {
			continue
		}
		if _, err := os.Stat(p + ".turbosend-part"); err == nil {
			continue
		}
		return cand
	}
	return name
}

func freeSpace(dir string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return 0, err
	}
	return free, nil
}

// markFromInternet añade la marca "descargado de Internet" que pone Windows a las descargas.
func markFromInternet(path string) {
	os.WriteFile(path+":Zone.Identifier", []byte("[ZoneTransfer]\r\nZoneId=3\r\n"), 0o644)
}

// Offer registra lo que un invitado quiere enviar.
func (a *App) Offer(roomID string, m *Member, files []offerFile) (*InBundle, error) {
	if len(files) == 0 || len(files) > 100000 {
		return nil, errors.New("lista de archivos no válida")
	}
	b := &InBundle{ID: newID(), From: m.PID, Chunk: inChunk, Status: "offered", Created: time.Now().UnixMilli()}
	tops := map[string]bool{}
	for _, f := range files {
		if f.Size < 0 || f.Size > 1<<44 {
			return nil, errors.New("tamaño no válido")
		}
		parts := safeRel(f.Rel)
		tops[parts[0]] = true
		b.Files = append(b.Files, &InFile{Rel: strings.Join(parts, "/"), Size: f.Size})
		b.Total += f.Size
	}
	switch {
	case len(b.Files) == 1 && !strings.Contains(b.Files[0].Rel, "/"):
		b.Kind, b.Name = "file", b.Files[0].Rel
	case len(tops) == 1:
		for t := range tops {
			b.Kind, b.Name = "multi", t
		}
	default:
		b.Kind, b.Name = "multi", fmt.Sprintf("%d archivos", len(b.Files))
	}

	auto := false
	a.rooms.With(roomID, func(r *Room) bool {
		set := a.getSettings()
		switch r.Auto {
		case "on":
			auto = true
		case "off":
			auto = false
		default:
			auto = set.AutoAccept
		}
		if auto && set.AutoLimitGB > 0 && float64(b.Total) > set.AutoLimitGB*(1<<30) {
			auto = false
		}
		r.In = append(r.In, b)
		return true
	})
	msg := newMessage(m.PID, "in")
	msg.Bundle = b.ID
	a.post(roomID, msg, true)
	if auto {
		if err := a.AcceptIn(roomID, b.ID); err != nil {
			a.notice(roomID, "No se pudo aceptar automáticamente: "+err.Error())
		}
	}
	return b, nil
}

// AcceptIn prepara los archivos de destino y pide al invitado que empiece a enviar.
func (a *App) AcceptIn(roomID, bid string) error {
	set := a.getSettings()
	var err error
	var from string
	a.rooms.With(roomID, func(r *Room) bool {
		b := r.inBundle(bid)
		if b == nil {
			err = errors.New("ese envío ya no existe")
			return false
		}
		if b.Status != "offered" && b.Status != "failed" {
			return false
		}
		from = b.From
		base := filepath.Join(set.RecvDir, safeComponent(r.Name))
		if e := os.MkdirAll(base, 0o755); e != nil {
			err = fmt.Errorf("no se pudo crear la carpeta de recepción: %v", e)
			return false
		}
		if free, e := freeSpace(base); e == nil && free < uint64(b.Total-b.Received)+(256<<20) {
			err = fmt.Errorf("no hay espacio suficiente en el disco (hacen falta %s)", humanBytes(b.Total))
			return false
		}
		if b.Root == "" { // primera vez: elegimos nombres finales libres
			taken := map[string]bool{}
			tops := map[string]string{}
			for _, f := range b.Files {
				parts := strings.Split(f.Rel, "/")
				top, ok := tops[parts[0]]
				if !ok {
					top = uniqueName(base, parts[0], taken)
					taken[strings.ToLower(top)] = true
					tops[parts[0]] = top
				}
				parts[0] = top
				f.Final = filepath.Join(append([]string{base}, parts...)...)
			}
			if b.Kind == "file" || len(tops) == 1 {
				for _, t := range tops {
					b.Root = filepath.Join(base, t)
				}
			} else {
				b.Root = base
			}
		}
		for _, f := range b.Files {
			if f.Done {
				continue
			}
			if e := os.MkdirAll(filepath.Dir(f.Final), 0o755); e != nil {
				err = e
				return false
			}
			part := f.Final + ".turbosend-part"
			fh, e := os.OpenFile(part, os.O_CREATE|os.O_WRONLY, 0o644)
			if e != nil {
				err = e
				return false
			}
			if st, _ := fh.Stat(); st != nil && st.Size() != f.Size {
				fh.Truncate(f.Size)
			}
			fh.Close()
		}
		b.Status = "accepted"
		b.Error = ""
		return true
	})
	if err != nil {
		return err
	}
	a.bundleChanged(roomID, bid)
	a.hub.SendTo(roomID, from, "upload", map[string]string{"bundle": bid})
	a.maybeFinish(roomID, bid) // por si solo había archivos vacíos
	return nil
}

func (a *App) RejectIn(roomID, bid string) {
	a.setInStatus(roomID, bid, "rejected", "", true)
}

// CancelIn cancela un envío entrante y borra lo recibido a medias.
func (a *App) CancelIn(roomID, bid string) {
	a.setInStatus(roomID, bid, "cancelled", "", true)
}

func (a *App) setInStatus(roomID, bid, status, errMsg string, cleanup bool) {
	var parts []string
	a.rooms.With(roomID, func(r *Room) bool {
		b := r.inBundle(bid)
		if b == nil || b.Status == "done" {
			return false
		}
		b.Status, b.Error = status, errMsg
		if cleanup {
			for _, f := range b.Files {
				if f.Final != "" && !f.Done {
					parts = append(parts, f.Final+".turbosend-part")
				}
			}
		}
		return true
	})
	for _, p := range parts {
		os.Remove(p)
	}
	a.tracker.Finish("in|" + bid)
	a.bundleChanged(roomID, bid)
}

// receiveChunk guarda un bloque ya verificado.
func (a *App) receiveChunk(roomID, bid string, fi int, off int64, data []byte) error {
	var final string
	var err error
	a.rooms.With(roomID, func(r *Room) bool {
		b := r.inBundle(bid)
		if b == nil || (b.Status != "accepted" && b.Status != "transferring") {
			err = errors.New("este envío no está aceptado")
			return false
		}
		if fi < 0 || fi >= len(b.Files) {
			err = errors.New("archivo no válido")
			return false
		}
		f := b.Files[fi]
		if off%b.Chunk != 0 || off >= f.Size {
			err = errors.New("bloque no válido")
			return false
		}
		want := b.Chunk
		if f.Size-off < want {
			want = f.Size - off
		}
		if int64(len(data)) != want {
			err = errors.New("tamaño de bloque incorrecto")
			return false
		}
		final = f.Final
		return false
	})
	if err != nil {
		return err
	}
	fh, e := os.OpenFile(final+".turbosend-part", os.O_WRONLY, 0o644)
	if e != nil {
		return fmt.Errorf("no se pudo abrir el archivo de destino: %v", e)
	}
	_, e = fh.WriteAt(data, off)
	fh.Close()
	if e != nil {
		return fmt.Errorf("no se pudo escribir en el disco: %v", e)
	}
	first := false
	a.rooms.With(roomID, func(r *Room) bool {
		b := r.inBundle(bid)
		if b == nil {
			return false
		}
		f := b.Files[fi]
		ci := off / b.Chunk
		if !f.has(ci) {
			f.mark(ci)
			b.Received += int64(len(data))
		}
		if b.Status == "accepted" {
			b.Status = "transferring"
			first = true
		}
		return true
	})
	if first {
		a.bundleChanged(roomID, bid)
	}
	a.maybeFinish(roomID, bid)
	return nil
}

// maybeFinish renombra los archivos cuando han llegado todos los bloques.
func (a *App) maybeFinish(roomID, bid string) {
	var moves []*InFile
	complete := false
	a.rooms.With(roomID, func(r *Room) bool {
		b := r.inBundle(bid)
		if b == nil || (b.Status != "accepted" && b.Status != "transferring") {
			return false
		}
		for _, f := range b.Files {
			if f.Done {
				continue
			}
			n := f.chunks(b.Chunk)
			for i := int64(0); i < n; i++ {
				if !f.has(i) {
					return false
				}
			}
		}
		complete = true
		b.Status = "finishing" // evita que dos bloques que acaban a la vez lo cierren dos veces
		for _, f := range b.Files {
			if !f.Done {
				moves = append(moves, f)
			}
		}
		return true
	})
	if !complete {
		return
	}
	var ferr error
	for _, f := range moves {
		part := f.Final + ".turbosend-part"
		dst := f.Final
		if _, err := os.Stat(dst); err == nil { // alguien creó ese nombre mientras tanto
			dst = filepath.Join(filepath.Dir(dst), uniqueName(filepath.Dir(dst), filepath.Base(dst), nil))
		}
		if err := os.Rename(part, dst); err != nil {
			ferr = err
			continue
		}
		markFromInternet(dst)
		f.Final = dst
	}
	a.rooms.With(roomID, func(r *Room) bool {
		b := r.inBundle(bid)
		if b == nil {
			return false
		}
		for _, f := range b.Files {
			for _, m := range moves {
				if m == f && ferr == nil {
					f.Done = true
				}
			}
			if f.Done {
				f.Have = nil
			}
		}
		if ferr != nil {
			b.Status, b.Error = "failed", "No se pudo guardar: "+ferr.Error()
		} else {
			b.Status = "done"
			b.Finished = time.Now().UnixMilli()
			r.Unread++
		}
		return true
	})
	a.tracker.Finish("in|" + bid)
	a.bundleChanged(roomID, bid)
	if ferr == nil {
		a.notify(roomID, "in-done", bid)
	}
}

// missingChunks dice qué bloques faltan (para reanudar).
func (a *App) missingChunks(roomID, bid, pid string) (map[string]any, error) {
	var out map[string]any
	var err error
	a.rooms.With(roomID, func(r *Room) bool {
		b := r.inBundle(bid)
		if b == nil || b.From != pid {
			err = errors.New("envío no encontrado")
			return false
		}
		files := []map[string]any{}
		for i, f := range b.Files {
			if f.Done {
				continue
			}
			var miss []int64
			n := f.chunks(b.Chunk)
			for c := int64(0); c < n; c++ {
				if !f.has(c) {
					miss = append(miss, c)
				}
			}
			files = append(files, map[string]any{"i": i, "missing": miss})
		}
		out = map[string]any{"status": b.Status, "chunk": b.Chunk, "files": files}
		return false
	})
	return out, err
}

func humanBytes(n int64) string {
	f := float64(n)
	u := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for f >= 1024 && i < len(u)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", n)
	}
	return strings.Replace(fmt.Sprintf("%.1f %s", f, u[i]), ".", ",", 1)
}
