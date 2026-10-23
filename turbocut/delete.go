package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

type Failure struct {
	Path   string   `json:"path"`
	Reason string   `json:"reason"`
	Apps   []string `json:"apps,omitempty"`
}

type DeleteResult struct {
	Freed   int64     `json:"freed"`
	Deleted int       `json:"deleted"`
	Kept    int       `json:"kept"` // elementos que no se pudieron borrar (en uso, sin permiso…)
	Failed  []Failure `json:"failed"`
	Removed []int32   `json:"removed"` // elementos que ya no existen
	Info    ScanInfo  `json:"info"`
}

// Delete borra elementos del escaneo: a la papelera o para siempre.
func (a *App) Delete(ids []int32, permanent bool) (*DeleteResult, error) {
	a.opMu.Lock()
	defer a.opMu.Unlock()
	a.mu.RLock()
	if a.tree == nil {
		a.mu.RUnlock()
		return nil, errNoScan
	}
	ids = a.topmost(ids)
	a.mu.RUnlock()
	if len(ids) == 0 {
		return nil, errors.New("No hay nada seleccionado")
	}
	return a.deleteIDs(ids, permanent, true)
}

// Clean borra los grupos de limpieza elegidos.
func (a *App) Clean(groups []string) (*DeleteResult, error) {
	a.opMu.Lock()
	defer a.opMu.Unlock()
	a.mu.Lock()
	if a.tree == nil {
		a.mu.Unlock()
		return nil, errNoScan
	}
	want := map[string]bool{}
	for _, g := range groups {
		want[g] = true
	}
	var perm, bin []int32
	var bins []string
	var binSize int64
	for _, c := range a.candidates() {
		if !want[c.ID] {
			continue
		}
		switch {
		case c.special == "papelera":
			bins = append(bins, c.roots...)
			binSize += c.Size
		case c.Permanent:
			perm = append(perm, c.targets...)
		default:
			bin = append(bin, c.targets...)
		}
	}
	perm, bin = a.topmost(perm), a.topmost(bin)
	a.mu.Unlock()

	total := &DeleteResult{}
	for _, root := range bins {
		runtime.EventsEmit(a.ctx, "busy", "Vaciando la papelera…")
		if err := emptyRecycleBin(root); err == nil {
			total.Freed += binSize
			total.Deleted++
		} else {
			total.Failed = append(total.Failed, Failure{Path: root + "$Recycle.Bin", Reason: "No se pudo vaciar la papelera"})
		}
	}
	for _, part := range []struct {
		ids  []int32
		perm bool
	}{{perm, true}, {bin, false}} {
		if len(part.ids) == 0 {
			continue
		}
		r, err := a.deleteIDs(part.ids, part.perm, false)
		if err != nil {
			return nil, err
		}
		total.Freed += r.Freed
		total.Deleted += r.Deleted
		total.Kept += r.Kept
		total.Failed = append(total.Failed, r.Failed...)
		total.Removed = append(total.Removed, r.Removed...)
	}
	a.mu.Lock()
	a.afterChange()
	total.Info = a.info
	a.mu.Unlock()
	return total, nil
}

// topmost quita los elementos que ya están dentro de otro elegido (con a.mu cogido).
func (a *App) topmost(ids []int32) []int32 {
	t := a.tree
	set := map[int32]bool{}
	for _, id := range ids {
		if t.alive(id) && id != t.Root {
			set[id] = true
		}
	}
	var out []int32
	for id := range set {
		inside := false
		for p := t.Nodes[id].Parent; p != none; p = t.Nodes[p].Parent {
			if set[p] {
				inside = true
				break
			}
			if p == t.Root {
				break
			}
		}
		if !inside {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (a *App) deleteIDs(ids []int32, permanent, detail bool) (*DeleteResult, error) {
	a.mu.RLock()
	t := a.tree
	m := a.measureFn()
	paths := make([]string, len(ids))
	sizes := make([]int64, len(ids))
	for i, id := range ids {
		paths[i] = t.pathOf(id)
		sizes[i] = m(&t.Nodes[id])
	}
	a.mu.RUnlock()

	if permanent {
		runtime.EventsEmit(a.ctx, "busy", "Borrando…")
		removeAllParallel(paths)
	} else {
		runtime.EventsEmit(a.ctx, "busy", "Enviando a la papelera…")
		done := make(chan error, 1)
		go func() {
			goruntime.LockOSThread()
			defer goruntime.UnlockOSThread()
			_ = windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE)
			defer windows.CoUninitialize()
			done <- toRecycleBin(paths)
		}()
		// Aunque falle o se cancele en el aviso de Windows, abajo se mira qué llegó a borrarse.
		<-done
	}
	runtime.EventsEmit(a.ctx, "busy", "Actualizando…")
	defer runtime.EventsEmit(a.ctx, "busy", "")

	res := &DeleteResult{Failed: []Failure{}, Removed: []int32{}}
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, id := range ids {
		if !t.alive(id) {
			continue
		}
		if _, err := os.Lstat(paths[i]); errors.Is(err, fs.ErrNotExist) {
			res.Freed += sizes[i]
			res.Deleted++
			res.Removed = append(res.Removed, id)
			t.remove(id)
			continue
		}
		// Queda algo: se vuelve a leer para tener los tamaños reales.
		before := sizes[i]
		after := int64(0)
		if t.Nodes[id].dir() {
			nid := refreshSubtree(t, id)
			after = m(&t.Nodes[nid])
		} else {
			after = before
		}
		res.Freed += before - after
		res.Kept++
		if detail {
			f := Failure{Path: paths[i], Reason: "No se pudo borrar (en uso o sin permiso)"}
			f.Apps = lockingApps(remainingFiles(paths[i], 200))
			if len(f.Apps) > 0 {
				f.Reason = "Lo tiene abierto otro programa"
			}
			res.Failed = append(res.Failed, f)
		}
	}
	a.afterChange()
	res.Info = a.info
	return res, nil
}

// removeAllParallel borra para siempre varias rutas a la vez.
func removeAllParallel(paths []string) {
	ch := make(chan string)
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range ch {
				if err := os.RemoveAll(p); err != nil {
					clearReadOnly(p)
					_ = os.RemoveAll(p)
				}
			}
		}()
	}
	for _, p := range paths {
		ch <- p
	}
	close(ch)
	wg.Wait()
}

func clearReadOnly(root string) {
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil {
			if info, e := d.Info(); e == nil && info.Mode().Perm()&0o200 == 0 {
				_ = os.Chmod(p, 0o666)
			}
		}
		return nil
	})
}

// remainingFiles lista hasta max archivos que siguen dentro de una ruta.
func remainingFiles(root string, max int) []string {
	st, err := os.Lstat(root)
	if err != nil {
		return nil
	}
	if !st.IsDir() {
		return []string{root}
	}
	var out []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			out = append(out, p)
			if len(out) >= max {
				return filepath.SkipAll
			}
		}
		return nil
	})
	return out
}
