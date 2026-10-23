package main

// «¿Qué lo bloquea?» y miniaturas para la interfaz.

import (
	"encoding/base64"
	"errors"
	"strings"
	"sync"

	"turbomenu/internal/lock"
	"turbomenu/internal/shellx"
)

type LockProc struct {
	lock.Process
	Icon string `json:"icon"`
}

type LockView struct {
	Procs    []LockProc `json:"procs"`
	Warnings []string   `json:"warnings"`
}

// LockScan busca qué programas usan los archivos del trabajo actual.
func (a *App) LockScan() (*LockView, error) {
	job := a.currentJob()
	if job == nil {
		return nil, errors.New("No hay nada seleccionado")
	}
	ctx := a.newTask("lock")
	defer a.endTask("lock")
	rep, err := lock.Find(ctx, job.Paths)
	if err != nil {
		return nil, err
	}
	v := &LockView{Procs: []LockProc{}, Warnings: []string{}}
	var wg sync.WaitGroup
	v.Procs = make([]LockProc, len(rep.Processes))
	for i, p := range rep.Processes {
		v.Procs[i] = LockProc{Process: p}
		if p.Hint != "" && len(p.Files) == 0 {
			v.Procs[i].Files = nil
		}
		if p.Exe != "" {
			wg.Add(1)
			go func(i int, exe string) {
				defer wg.Done()
				if b, err := shellx.Icon(exe, 32); err == nil {
					v.Procs[i].Icon = "data:image/png;base64," + base64.StdEncoding.EncodeToString(b)
				}
			}(i, p.Exe)
		}
	}
	wg.Wait()
	// El aviso de programas de administrador solo si no hemos encontrado a nadie.
	if rep.Note != "" && (len(rep.Processes) == 0 || rep.Partial) {
		v.Warnings = append(v.Warnings, rep.Note)
	}
	return v, nil
}

type LockResult struct {
	OK       bool   `json:"ok"`
	Status   string `json:"status"`
	Message  string `json:"message"`
	CanForce bool   `json:"canForce"`
}

// LockRelease libera lo que bloquea un proceso (con force lo termina).
func (a *App) LockRelease(pid int, force bool) LockResult {
	job := a.currentJob()
	if job == nil {
		return LockResult{Message: "No hay nada seleccionado"}
	}
	ctx := a.newTask("release")
	defer a.endTask("release")
	r := lock.Release(ctx, pid, job.Paths, force)
	res := LockResult{Status: string(r.Status), Message: r.Message}
	switch r.Status {
	case "closed", "released", "gone":
		res.OK = !r.StillLocked
		if r.StillLocked {
			res.CanForce = false
		}
	case "needsForce":
		res.Status = "unsaved"
		res.CanForce = true
	default:
		res.CanForce = false
	}
	return res
}

// LockAfter borra (a la papelera), mueve o expulsa una vez liberado.
func (a *App) LockAfter(op, dest string) error {
	job := a.currentJob()
	if job == nil {
		return errors.New("No hay nada seleccionado")
	}
	switch op {
	case "delete":
		err := lock.Recycle(job.Paths)
		if errors.Is(err, lock.ErrNoRecycleBin) {
			return errors.New("Esta unidad no tiene papelera: bórralo desde el Explorador si estás seguro")
		}
		return err
	case "move":
		return lock.Move(job.Paths, dest)
	case "eject":
		for _, p := range job.Paths {
			if len(p) >= 2 && p[1] == ':' {
				return lock.Eject(strings.ToUpper(p[:2]))
			}
		}
		return errors.New("No es una unidad")
	}
	return errors.New("Acción desconocida")
}

var thumbCache sync.Map

// Thumb devuelve la miniatura (o el icono) del archivo como data URL.
func (a *App) Thumb(path string, size int) string {
	if size <= 0 || size > 512 {
		size = 96
	}
	b, err := shellx.Thumbnail(path, size)
	if err != nil || len(b) == 0 {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b)
}
