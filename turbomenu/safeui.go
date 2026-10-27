package main

// «¿Es seguro?» y «¿De dónde salió?» para la interfaz.

import (
	"errors"
	"strings"
	"time"

	"turbomenu/internal/safe"
)

// StartSafe analiza el archivo y va mandando resultados con el evento "safe".
func (a *App) StartSafe(path string) {
	ctx := a.newTask("safe")
	gen := a.gen
	// La huella solo sale a CIRCL si lo has aceptado.
	opt := safe.Options{NoKnown: getSettings().KnownLookup != "yes"}
	if k := vtKey(); k != "" {
		opt.VT = safe.NewVTClient(k)
	}
	var last time.Time
	opt.OnProgress = func(done, total int64) {
		if time.Since(last) > 150*time.Millisecond && a.gen == gen {
			last = time.Now()
			a.emit("safe-progress", map[string]int64{"done": done, "total": total})
		}
	}
	go func() {
		defer a.endTask("safe")
		safe.Analyze(ctx, path, opt, func(u safe.Update) {
			if a.gen == gen && ctx.Err() == nil {
				a.emit("safe", u)
			}
		})
	}()
}

// SetVTKey guarda la clave gratuita de VirusTotal (vacía = quitarla).
func (a *App) SetVTKey(key string) error {
	key = strings.TrimSpace(key)
	if key != "" && !safe.ValidVTKey(key) {
		return errors.New("Esa clave no parece válida: son 64 letras y números")
	}
	return setVTKey(key)
}

// SetKnownLookup guarda si se puede consultar la huella en CIRCL ("yes" o "no").
func (a *App) SetKnownLookup(v string) error {
	if v != "yes" && v != "no" {
		v = ""
	}
	return updateSettings(func(s *Settings) { s.KnownLookup = v })
}

// VTUpload sube el archivo a VirusTotal (el usuario ya ha dado permiso) y espera el análisis.
func (a *App) VTUpload(path string) (*safe.VTReport, error) {
	k := vtKey()
	if k == "" {
		return nil, errors.New("Falta la clave de VirusTotal")
	}
	ctx := a.newTask("vt")
	defer a.endTask("vt")
	return safe.NewVTClient(k).UploadAndWait(ctx, path, func(u safe.VTUpload) { a.emit("vt-upload", u) })
}

// GetOrigin lee de dónde se descargó el archivo.
func (a *App) GetOrigin(path string) (*safe.Origin, error) { return safe.ReadOrigin(path) }

// RemoveMark quita la marca de «descargado de internet».
func (a *App) RemoveMark(path string) error { return safe.Unblock(path) }
