package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

// Búsqueda de archivos duplicados sobre el último análisis:
//  1. se agrupan los archivos por tamaño (sin leer nada del disco);
//  2. se quitan los enlaces duros (son el mismo archivo con dos nombres);
//  3. se compara un trozo del principio y otro del final;
//  4. a los que siguen empatados se les lee el contenido entero (SHA-256).

type DupeOpts struct {
	Scope         int32 `json:"scope"`
	MinSize       int64 `json:"minSize"`       // bytes
	IncludeSystem bool  `json:"includeSystem"` // Windows, Archivos de programa, AppData…
}

type DupeGroup struct {
	Size   int64  `json:"size"`
	Wasted int64  `json:"wasted"` // lo que se libera dejando una sola copia
	Files  []Item `json:"files"`
}

type DupeResult struct {
	Groups   []DupeGroup `json:"groups"`
	Wasted   int64       `json:"wasted"`
	Copies   int         `json:"copies"` // archivos que sobran
	Checked  int         `json:"checked"`
	Skipped  int         `json:"skipped"` // no se pudieron leer
	Seconds  float64     `json:"seconds"`
	Base     string      `json:"base"`
	MinSize  int64       `json:"minSize"`
	Capped   bool        `json:"capped"`
	ScanWhen int64       `json:"scanWhen"`
}

type DupeProgress struct {
	Phase      string `json:"phase"` // "tamaños", "principio", "contenido"
	Done       int64  `json:"done"`
	Total      int64  `json:"total"`
	Bytes      int64  `json:"bytes"`
	BytesTotal int64  `json:"bytesTotal"`
}

const (
	dupePartial   = 64 << 10 // bytes que se leen del principio y del final
	dupeMaxGroups = 3000
)

// Carpetas que no se miran salvo que se pida: duplicados "normales" de Windows
// y de los programas, que no conviene tocar.
var (
	dupeSkipAnywhere = map[string]bool{"node_modules": true, ".git": true, "$recycle.bin": true,
		"system volume information": true, "appdata": true, "windowsapps": true, "winsxs": true}
	dupeSkipAtRoot = map[string]bool{"windows": true, "program files": true, "program files (x86)": true,
		"programdata": true, "recovery": true, "$windows.~bt": true, "$windows.~ws": true, "windows.old": true, "msocache": true}
)

type dupeCand struct {
	id   int32
	size int64
	path string
	key  string
}

func (a *App) StartDupes(o DupeOpts) error {
	a.dupeMu.Lock()
	if a.dupeCancel != nil {
		a.dupeMu.Unlock()
		return errors.New("Ya se están buscando duplicados")
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.dupeCancel = cancel
	a.dupeMu.Unlock()

	go func() {
		defer func() {
			a.dupeMu.Lock()
			a.dupeCancel = nil
			a.dupeMu.Unlock()
			cancel()
		}()
		res, err := a.findDupes(ctx, o, func(p DupeProgress) { runtime.EventsEmit(a.ctx, "dupes", p) })
		switch {
		case ctx.Err() != nil:
			runtime.EventsEmit(a.ctx, "dupesdone", map[string]any{"cancelled": true})
		case err != nil:
			runtime.EventsEmit(a.ctx, "dupesdone", map[string]any{"error": err.Error()})
		default:
			runtime.EventsEmit(a.ctx, "dupesdone", map[string]any{"result": res})
		}
	}()
	return nil
}

func (a *App) CancelDupes() {
	a.dupeMu.Lock()
	if a.dupeCancel != nil {
		a.dupeCancel()
	}
	a.dupeMu.Unlock()
}

// collectDupeCands agrupa por tamaño (con a.mu cogido para leer el árbol).
func (a *App) collectDupeCands(o DupeOpts) ([][]dupeCand, string, int64, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	t := a.tree
	if t == nil {
		return nil, "", 0, errNoScan
	}
	scope := o.Scope
	if !t.alive(scope) || !t.Nodes[scope].dir() {
		scope = t.Root
	}
	minSize := o.MinSize
	if minSize < 1 {
		minSize = 1
	}
	rootIsDrive := strings.EqualFold(strings.TrimRight(t.RootPath, `\`)+`\`, volumeOf(t.RootPath))
	bySize := map[int64][]int32{}
	t.preorder(scope, func(n int32) bool {
		nd := &t.Nodes[n]
		if nd.dir() {
			if n == scope {
				return true
			}
			if nd.Flags&fLink != 0 {
				return false
			}
			if !o.IncludeSystem {
				name := strings.ToLower(t.name(n))
				if dupeSkipAnywhere[name] || rootIsDrive && nd.Parent == t.Root && dupeSkipAtRoot[name] {
					return false
				}
			}
			return true
		}
		if nd.Size < minSize || nd.Flags&fCloud != 0 || !o.IncludeSystem && nd.Flags&fSystem != 0 && nd.Flags&fHidden != 0 {
			return false
		}
		bySize[nd.Size] = append(bySize[nd.Size], n)
		return false
	})
	var groups [][]dupeCand
	for size, ids := range bySize {
		if len(ids) < 2 {
			continue
		}
		g := make([]dupeCand, len(ids))
		for i, id := range ids {
			g[i] = dupeCand{id: id, size: size, path: t.pathOf(id)}
		}
		groups = append(groups, g)
	}
	return groups, t.pathOf(scope), a.info.When, nil
}

// regroup separa cada grupo según la clave calculada y deja los que tienen 2 o más.
func regroup(groups [][]dupeCand) [][]dupeCand {
	var out [][]dupeCand
	for _, g := range groups {
		by := map[string][]dupeCand{}
		for _, c := range g {
			if c.key != "" {
				by[c.key] = append(by[c.key], c)
			}
		}
		for _, sub := range by {
			if len(sub) > 1 {
				out = append(out, sub)
			}
		}
	}
	return out
}

func (a *App) findDupes(ctx context.Context, o DupeOpts, progress func(DupeProgress)) (*DupeResult, error) {
	start := time.Now()
	progress(DupeProgress{Phase: "tamaños"})
	groups, base, when, err := a.collectDupeCands(o)
	if err != nil {
		return nil, err
	}
	res := &DupeResult{Base: base, MinSize: o.MinSize, ScanWhen: when, Groups: []DupeGroup{}}

	var skipped atomic.Int64
	// Paso 2 y 3: identificador del archivo (para quitar enlaces duros) + principio y final.
	var total int64
	for _, g := range groups {
		total += int64(len(g))
	}
	var done atomic.Int64
	stop := tick(func() { progress(DupeProgress{Phase: "principio", Done: done.Load(), Total: total}) })
	parallelFor(ctx, groups, 8, func(g []dupeCand) {
		seen := map[string]bool{}
		for i := range g {
			if ctx.Err() != nil {
				return
			}
			id, sum, err := partialHash(g[i].path, g[i].size)
			done.Add(1)
			if err != nil {
				skipped.Add(1)
				continue
			}
			if seen[id] {
				continue // enlace duro: es el mismo archivo
			}
			seen[id] = true
			g[i].key = sum
		}
	})
	stop()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	groups = regroup(groups)

	// Paso 4: contenido entero (si el archivo es mayor que lo ya leído).
	var bytesTotal int64
	var files int64
	for _, g := range groups {
		if g[0].size > 2*dupePartial {
			bytesTotal += g[0].size * int64(len(g))
			files += int64(len(g))
		}
	}
	var bytesDone atomic.Int64
	done.Store(0)
	stop = tick(func() {
		progress(DupeProgress{Phase: "contenido", Done: done.Load(), Total: files, Bytes: bytesDone.Load(), BytesTotal: bytesTotal})
	})
	parallelFor(ctx, groups, 4, func(g []dupeCand) {
		if g[0].size <= 2*dupePartial {
			return // ya se ha leído entero
		}
		for i := range g {
			if ctx.Err() != nil {
				return
			}
			sum, err := fullHash(ctx, g[i].path, &bytesDone)
			done.Add(1)
			if err != nil {
				skipped.Add(1)
				g[i].key = ""
				continue
			}
			g[i].key = sum
		}
	})
	stop()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	groups = regroup(groups)

	// Resultado, con los datos actuales del árbol.
	a.mu.RLock()
	t := a.tree
	m := func(n *Node) int64 { return n.Size }
	for _, g := range groups {
		dg := DupeGroup{Size: g[0].size}
		for _, c := range g {
			if t == nil || !t.alive(c.id) {
				continue
			}
			it := makeItem(t, c.id, m)
			it.Path = c.path[:max(0, strings.LastIndexByte(c.path, '\\'))]
			dg.Files = append(dg.Files, it)
		}
		if len(dg.Files) < 2 {
			continue
		}
		sort.Slice(dg.Files, func(i, j int) bool { return dg.Files[i].MTime < dg.Files[j].MTime })
		dg.Wasted = dg.Size * int64(len(dg.Files)-1)
		res.Groups = append(res.Groups, dg)
	}
	a.mu.RUnlock()
	sort.Slice(res.Groups, func(i, j int) bool { return res.Groups[i].Wasted > res.Groups[j].Wasted })
	if len(res.Groups) > dupeMaxGroups {
		res.Groups = res.Groups[:dupeMaxGroups]
		res.Capped = true
	}
	for _, g := range res.Groups {
		res.Wasted += g.Wasted
		res.Copies += len(g.Files) - 1
	}
	res.Checked = int(total)
	res.Skipped = int(skipped.Load())
	res.Seconds = time.Since(start).Seconds()
	return res, nil
}

// tick llama a fn cada 150 ms hasta que se llama a la función que devuelve.
func tick(fn func()) func() {
	quit := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(150 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-quit:
				fn()
				return
			case <-t.C:
				fn()
			}
		}
	}()
	return func() { close(quit); wg.Wait() }
}

func parallelFor(ctx context.Context, groups [][]dupeCand, workers int, fn func([]dupeCand)) {
	ch := make(chan []dupeCand)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for g := range ch {
				fn(g)
			}
		}()
	}
	for _, g := range groups {
		if ctx.Err() != nil {
			break
		}
		ch <- g
	}
	close(ch)
	wg.Wait()
}

func openForHash(path string) (*os.File, string, error) {
	f, err := os.Open(longPath(path))
	if err != nil {
		return nil, "", err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info); err != nil {
		f.Close()
		return nil, "", err
	}
	id := fmt.Sprintf("%x:%x:%x", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow)
	return f, id, nil
}

// partialHash devuelve el identificador del archivo y el SHA-256 del principio y el final.
func partialHash(path string, size int64) (string, string, error) {
	f, id, err := openForHash(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	h := sha256.New()
	buf := make([]byte, dupePartial)
	n, err := io.ReadFull(f, buf[:min(int64(len(buf)), size)])
	if err != nil {
		return "", "", err
	}
	h.Write(buf[:n])
	if size > dupePartial {
		off := max(int64(n), size-dupePartial)
		m, err := f.ReadAt(buf[:size-off], off)
		if err != nil && !errors.Is(err, io.EOF) {
			return "", "", err
		}
		h.Write(buf[:m])
	}
	return id, string(h.Sum(nil)), nil
}

func fullHash(ctx context.Context, path string, done *atomic.Int64) (string, error) {
	f, _, err := openForHash(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	buf := make([]byte, 1<<20)
	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		n, err := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
			done.Add(int64(n))
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return string(h.Sum(nil)), nil
}
