package main

import (
	"container/heap"
	"context"
	"errors"
	"os"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const version = "1.0"

type App struct {
	ctx context.Context

	smu      sync.Mutex
	settings Settings

	mu    sync.RWMutex // protege tree, info, prev y cands
	tree  *Tree
	info  ScanInfo
	prev  *Snapshot
	cands []*Candidate

	opMu     sync.Mutex // una operación que cambia el árbol cada vez
	scanMu   sync.Mutex
	cancel   context.CancelFunc
	scanning bool

	startScan string
}

type ScanInfo struct {
	Root     string  `json:"root"`
	Mode     string  `json:"mode"` // "mft" o "walk"
	Note     string  `json:"note,omitempty"`
	Seconds  float64 `json:"seconds"`
	Files    int64   `json:"files"`
	Dirs     int64   `json:"dirs"`
	Denied   int64   `json:"denied"`
	Total    int64   `json:"total"` // tamaño de la unidad
	Free     int64   `json:"free"`
	Scanned  int64   `json:"scanned"`
	IsDrive  bool    `json:"isDrive"`
	When     int64   `json:"when"`
	PrevTime int64   `json:"prevTime"`
	RootID   int32   `json:"rootId"`
}

func NewApp() *App {
	a := &App{settings: defaultSettings()}
	_ = loadJSON(dataDir()+`\ajustes.json`, &a.settings)
	a.settings.normalize()
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		if args[i] == "--scan" && i+1 < len(args) {
			a.startScan = args[i+1]
			i++
		} else if !strings.HasPrefix(args[i], "-") && a.startScan == "" {
			a.startScan = args[i]
		}
	}
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	runtime.OnFileDrop(ctx, func(x, y int, paths []string) {
		if len(paths) == 0 {
			return
		}
		p := paths[0]
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			p = p[:strings.LastIndexByte(p, '\\')]
		}
		runtime.EventsEmit(ctx, "dropped", p)
	})
}

func (a *App) getSettings() Settings {
	a.smu.Lock()
	defer a.smu.Unlock()
	return a.settings
}

func (a *App) measureFn() func(*Node) int64 {
	if a.getSettings().Measure == "size" {
		return func(n *Node) int64 { return n.Size }
	}
	return func(n *Node) int64 { return n.Alloc }
}

// ---------- información general ----------

type Info struct {
	Version    string     `json:"version"`
	Admin      bool       `json:"admin"`
	Drives     []Drive    `json:"drives"`
	Settings   Settings   `json:"settings"`
	Categories []Category `json:"categories"`
	StartScan  string     `json:"startScan"`
	Scan       *ScanInfo  `json:"scan"`
}

func (a *App) GetInfo() Info {
	in := Info{Version: version, Admin: isAdmin(), Drives: listDrives(), Settings: a.getSettings(),
		Categories: categories, StartScan: a.startScan}
	a.startScan = ""
	a.mu.RLock()
	if a.tree != nil {
		s := a.info
		in.Scan = &s
	}
	a.mu.RUnlock()
	return in
}

func (a *App) Drives() []Drive { return listDrives() }

func (a *App) SaveSettings(s Settings) Settings {
	s.normalize()
	a.smu.Lock()
	a.settings = s
	a.smu.Unlock()
	_ = saveJSON(dataDir()+`\ajustes.json`, s)
	a.mu.Lock()
	a.cands = nil
	if a.tree != nil {
		a.info.Scanned = a.measureFn()(&a.tree.Nodes[a.tree.Root])
	}
	a.mu.Unlock()
	return s
}

func (a *App) PickFolder() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Elige la carpeta que quieres analizar"})
}

func (a *App) RestartAdmin(path string) error {
	args := []string{}
	if path != "" {
		args = append(args, "--scan", path)
	}
	if err := relaunchAsAdmin(args); err != nil {
		return errors.New("No se ha concedido el permiso de administrador")
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		runtime.Quit(a.ctx)
	}()
	return nil
}

// ---------- escaneo ----------

type ProgressEvent struct {
	Mode    string  `json:"mode"`
	Files   int64   `json:"files"`
	Dirs    int64   `json:"dirs"`
	Bytes   int64   `json:"bytes"`
	Done    int64   `json:"done"`
	Total   int64   `json:"total"`
	Cur     string  `json:"cur"`
	Seconds float64 `json:"seconds"`
}

func (a *App) Scan(path string) error {
	root := cleanRoot(path)
	st, err := os.Stat(root)
	if err != nil || !st.IsDir() {
		return errors.New("No se puede abrir esa carpeta")
	}
	a.scanMu.Lock()
	if a.scanning {
		a.scanMu.Unlock()
		return errors.New("Ya hay un escaneo en marcha")
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.scanning, a.cancel = true, cancel
	a.scanMu.Unlock()

	go func() {
		defer func() {
			a.scanMu.Lock()
			a.scanning, a.cancel = false, nil
			a.scanMu.Unlock()
			cancel()
		}()
		a.opMu.Lock()
		defer a.opMu.Unlock()

		prog := &Progress{}
		mode := "walk"
		if canUseMFT(root) {
			mode = "mft"
		}
		start := time.Now()
		stopTick := make(chan struct{})
		var modeMu sync.Mutex
		go func() {
			t := time.NewTicker(100 * time.Millisecond)
			defer t.Stop()
			for {
				select {
				case <-stopTick:
					return
				case <-t.C:
					modeMu.Lock()
					m := mode
					modeMu.Unlock()
					runtime.EventsEmit(a.ctx, "progress", ProgressEvent{Mode: m, Files: prog.Files.Load(), Dirs: prog.Dirs.Load(),
						Bytes: prog.Bytes.Load(), Done: prog.Done.Load(), Total: prog.Total.Load(), Cur: prog.Cur(), Seconds: time.Since(start).Seconds()})
				}
			}
		}()

		var t *Tree
		note := ""
		if mode == "mft" {
			t, err = mftScan(ctx, root, prog)
			if err != nil && ctx.Err() == nil {
				note = "No se pudo leer la MFT (" + err.Error() + "); se ha usado el modo normal."
				modeMu.Lock()
				mode = "walk"
				modeMu.Unlock()
				prog = &Progress{}
				t, err = walkScan(ctx, root, prog)
			}
		} else {
			t, err = walkScan(ctx, root, prog)
		}
		close(stopTick)
		if err != nil {
			if ctx.Err() != nil {
				runtime.EventsEmit(a.ctx, "scanfail", map[string]any{"cancelled": true})
			} else {
				runtime.EventsEmit(a.ctx, "scanfail", map[string]any{"error": err.Error()})
			}
			return
		}

		var dirs int64
		t.preorder(t.Root, func(n int32) bool {
			if t.Nodes[n].dir() {
				dirs++
				return true
			}
			return false
		})
		vol := volumeOf(root)
		total, free := diskSpace(vol)
		info := ScanInfo{Root: root, Mode: mode, Note: note, Seconds: time.Since(start).Seconds(),
			Files: int64(t.Nodes[t.Root].Files), Dirs: dirs - 1, Denied: prog.Denied.Load(),
			Total: total, Free: free, Scanned: a.measureFn()(&t.Nodes[t.Root]),
			IsDrive: strings.EqualFold(strings.TrimRight(root, `\`)+`\`, vol), When: time.Now().Unix(), RootID: t.Root}

		var prev *Snapshot
		if a.getSettings().History {
			prev = loadSnapshot(root)
			_ = saveSnapshot(makeSnapshot(t))
		}
		if prev != nil {
			info.PrevTime = prev.Time
		}

		a.mu.Lock()
		a.tree, a.info, a.prev, a.cands = t, info, prev, nil
		a.mu.Unlock()
		runtime.EventsEmit(a.ctx, "scanned", info)
		go func() { time.Sleep(time.Second); debug.FreeOSMemory() }()
	}()
	return nil
}

func (a *App) CancelScan() {
	a.scanMu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	a.scanMu.Unlock()
}

// ---------- lectura del árbol ----------

type Item struct {
	ID    int32  `json:"id"`
	Name  string `json:"name"`
	Dir   bool   `json:"dir"`
	Size  int64  `json:"size"`
	Files int32  `json:"files"`
	MTime uint32 `json:"mtime"`
	Cat   uint8  `json:"cat"`
	Ext   string `json:"ext,omitempty"`
	Flags uint8  `json:"flags"`
	Delta *int64 `json:"delta,omitempty"`
	New   bool   `json:"new,omitempty"`
	Path  string `json:"path,omitempty"`
}

func makeItem(t *Tree, id int32, measure func(*Node) int64) Item {
	n := &t.Nodes[id]
	it := Item{ID: id, Name: t.name(id), Dir: n.dir(), Size: measure(n), Files: n.Files, MTime: n.MTime, Flags: n.Flags}
	if !it.Dir {
		it.Ext = t.exts[n.Ext]
		it.Cat = categoryOf(it.Ext)
	}
	return it
}

type Crumb struct {
	ID   int32  `json:"id"`
	Name string `json:"name"`
}

type Listing struct {
	Node     Item    `json:"node"`
	Path     string  `json:"path"`
	Crumbs   []Crumb `json:"crumbs"`
	Items    []Item  `json:"items"`
	More     int     `json:"more"`
	MoreSize int64   `json:"moreSize"`
}

var errNoScan = errors.New("Todavía no hay ningún escaneo")

func (a *App) treeFor(id int32) (*Tree, error) {
	if a.tree == nil {
		return nil, errNoScan
	}
	if !a.tree.alive(id) {
		return nil, errors.New("Ese elemento ya no existe")
	}
	return a.tree, nil
}

func (a *App) Listing(id int32) (*Listing, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	t, err := a.treeFor(id)
	if err != nil {
		return nil, err
	}
	m := a.measureFn()
	st := a.getSettings()
	showHidden := st.ShowHidden
	minDelta := st.DeltaMin << 20
	l := &Listing{Node: makeItem(t, id, m), Path: t.pathOf(id)}
	for n := id; n != none; n = t.Nodes[n].Parent {
		l.Crumbs = append([]Crumb{{n, t.name(n)}}, l.Crumbs...)
		if n == t.Root {
			break
		}
	}
	t.children(id, func(c int32) {
		if !showHidden && t.Nodes[c].Flags&(fHidden|fSystem) != 0 {
			return
		}
		it := makeItem(t, c, m)
		if a.prev != nil && it.Dir && st.ShowDeltas {
			rel := t.relPath(c)
			if old, ok := a.prev.Dirs[rel]; ok {
				if d := t.Nodes[c].Alloc - old; d >= minDelta || d <= -minDelta {
					it.Delta = &d
				}
			} else if t.Nodes[c].Alloc >= snapMinSize && t.Nodes[c].Alloc >= minDelta {
				parentRel := t.relPath(id)
				if _, pok := a.prev.Dirs[parentRel]; pok || id == t.Root {
					it.New = true
				}
			}
		}
		l.Items = append(l.Items, it)
	})
	sort.Slice(l.Items, func(i, j int) bool {
		if l.Items[i].Size != l.Items[j].Size {
			return l.Items[i].Size > l.Items[j].Size
		}
		return l.Items[i].Name < l.Items[j].Name
	})
	const maxItems = 2500
	if len(l.Items) > maxItems {
		for _, it := range l.Items[maxItems:] {
			l.MoreSize += it.Size
		}
		l.More = len(l.Items) - maxItems
		l.Items = l.Items[:maxItems]
	}
	return l, nil
}

// ---------- mapa (treemap) ----------

type TM struct {
	I int32  `json:"i"`
	S int64  `json:"s"`
	K uint8  `json:"k,omitempty"`
	D bool   `json:"d,omitempty"`
	N string `json:"n,omitempty"`
	C []*TM  `json:"c,omitempty"`
}

type tmHeap []*TM

func (h tmHeap) Len() int           { return len(h) }
func (h tmHeap) Less(i, j int) bool { return h[i].S > h[j].S }
func (h tmHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *tmHeap) Push(x any)        { *h = append(*h, x.(*TM)) }
func (h *tmHeap) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}

// Treemap devuelve el árbol recortado para dibujar: solo lo que se vería (área > ~20 px²).
func (a *App) Treemap(id int32, area float64) (*TM, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	t, err := a.treeFor(id)
	if err != nil {
		return nil, err
	}
	m := a.measureFn()
	showHidden := a.getSettings().ShowHidden
	root := &TM{I: id, S: m(&t.Nodes[id]), D: t.Nodes[id].dir(), N: t.name(id)}
	if !root.D || root.S <= 0 {
		return root, nil
	}
	if area < 10000 {
		area = 10000
	}
	threshold := float64(root.S) * 20 / area
	h := &tmHeap{root}
	count := 1
	const maxNodes = 7000
	for h.Len() > 0 && count < maxNodes {
		d := heap.Pop(h).(*TM)
		var kids []*TM
		var rest int64 = d.S
		t.children(d.I, func(c int32) {
			n := &t.Nodes[c]
			if !showHidden && n.Flags&(fHidden|fSystem) != 0 {
				return
			}
			s := m(n)
			if float64(s) < threshold || s <= 0 {
				return
			}
			k := &TM{I: c, S: s, D: n.dir(), N: t.name(c)}
			if !k.D {
				k.K = categoryOf(t.exts[n.Ext])
			}
			kids = append(kids, k)
			rest -= s
		})
		sort.Slice(kids, func(i, j int) bool { return kids[i].S > kids[j].S })
		if float64(rest) >= threshold {
			kids = append(kids, &TM{I: none, S: rest})
		}
		d.C = kids
		count += len(kids)
		for _, k := range kids {
			if k.D && k.I != none {
				heap.Push(h, k)
			}
		}
	}
	return root, nil
}

// ---------- archivos más grandes, tipos y búsqueda ----------

type itemHeap struct {
	ids  []int32
	size []int64
}

func (h *itemHeap) Len() int           { return len(h.ids) }
func (h *itemHeap) Less(i, j int) bool { return h.size[i] < h.size[j] }
func (h *itemHeap) Swap(i, j int) {
	h.ids[i], h.ids[j] = h.ids[j], h.ids[i]
	h.size[i], h.size[j] = h.size[j], h.size[i]
}
func (h *itemHeap) Push(x any) {
	p := x.([2]int64)
	h.ids = append(h.ids, int32(p[0]))
	h.size = append(h.size, p[1])
}
func (h *itemHeap) Pop() any {
	n := len(h.ids) - 1
	x := [2]int64{int64(h.ids[n]), h.size[n]}
	h.ids, h.size = h.ids[:n], h.size[:n]
	return x
}

func (h *itemHeap) offer(id int32, s int64, max int) {
	if h.Len() < max {
		heap.Push(h, [2]int64{int64(id), s})
	} else if s > h.size[0] {
		h.ids[0], h.size[0] = id, s
		heap.Fix(h, 0)
	}
}

func (a *App) sortedItems(t *Tree, h *itemHeap, base int32, m func(*Node) int64) []Item {
	out := make([]Item, 0, h.Len())
	for _, id := range h.ids {
		it := makeItem(t, id, m)
		it.Path = relTo(t.pathOf(t.Nodes[id].Parent), t.pathOf(base))
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Size > out[j].Size })
	return out
}

func relTo(p, base string) string {
	if strings.EqualFold(p, base) {
		return ""
	}
	b := strings.TrimRight(base, `\`) + `\`
	if len(p) >= len(b) && strings.EqualFold(p[:len(b)], b) {
		return p[len(b):]
	}
	return p
}

// TopFiles devuelve los archivos más grandes del subárbol (cat < 0: de cualquier tipo).
func (a *App) TopFiles(id int32, cat int) ([]Item, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	t, err := a.treeFor(id)
	if err != nil {
		return nil, err
	}
	m := a.measureFn()
	h := &itemHeap{}
	t.preorder(id, func(n int32) bool {
		nd := &t.Nodes[n]
		if nd.dir() {
			return true
		}
		if cat < 0 || int(categoryOf(t.exts[nd.Ext])) == cat {
			h.offer(n, m(nd), 300)
		}
		return false
	})
	return a.sortedItems(t, h, id, m), nil
}

func containsFold(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		ok := true
		for j := 0; j < len(sub); j++ {
			c := s[i+j]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != sub[j] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func (a *App) Search(id int32, q string) ([]Item, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	t, err := a.treeFor(id)
	if err != nil {
		return nil, err
	}
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return []Item{}, nil
	}
	m := a.measureFn()
	h := &itemHeap{}
	t.preorder(id, func(n int32) bool {
		nd := &t.Nodes[n]
		if n != id {
			name := t.names[nd.NameOff : nd.NameOff+uint32(nd.NameLen)]
			if containsFold(unsafeString(name), q) {
				h.offer(n, m(nd), 500)
			}
		}
		return nd.dir()
	})
	return a.sortedItems(t, h, id, m), nil
}

type CatStat struct {
	Cat   uint8 `json:"cat"`
	Size  int64 `json:"size"`
	Files int64 `json:"files"`
}

type ExtStat struct {
	Ext   string `json:"ext"`
	Cat   uint8  `json:"cat"`
	Size  int64  `json:"size"`
	Files int64  `json:"files"`
}

type TypesResult struct {
	Cats []CatStat `json:"cats"`
	Exts []ExtStat `json:"exts"`
}

func (a *App) Types(id int32) (*TypesResult, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	t, err := a.treeFor(id)
	if err != nil {
		return nil, err
	}
	m := a.measureFn()
	cats := make([]CatStat, len(categories))
	for i := range cats {
		cats[i].Cat = uint8(i)
	}
	exts := make([]ExtStat, len(t.exts))
	t.preorder(id, func(n int32) bool {
		nd := &t.Nodes[n]
		if nd.dir() {
			return true
		}
		s := m(nd)
		e := &exts[nd.Ext]
		e.Size += s
		e.Files++
		return false
	})
	r := &TypesResult{}
	for i := range exts {
		if exts[i].Files == 0 {
			continue
		}
		exts[i].Ext = t.exts[i]
		exts[i].Cat = categoryOf(t.exts[i])
		c := &cats[exts[i].Cat]
		c.Size += exts[i].Size
		c.Files += exts[i].Files
		r.Exts = append(r.Exts, exts[i])
	}
	for _, c := range cats {
		if c.Files > 0 {
			r.Cats = append(r.Cats, c)
		}
	}
	sort.Slice(r.Cats, func(i, j int) bool { return r.Cats[i].Size > r.Cats[j].Size })
	sort.Slice(r.Exts, func(i, j int) bool { return r.Exts[i].Size > r.Exts[j].Size })
	if len(r.Exts) > 40 {
		r.Exts = r.Exts[:40]
	}
	return r, nil
}

// ---------- limpieza e historial ----------

func (a *App) candidates() []*Candidate {
	if a.cands == nil && a.tree != nil {
		a.cands = findCandidates(a.tree, a.measureFn())
	}
	return a.cands
}

func (a *App) Cleanup() ([]*Candidate, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.tree == nil {
		return nil, errNoScan
	}
	return a.candidates(), nil
}

type GrowthResult struct {
	PrevTime int64    `json:"prevTime"`
	Items    []Growth `json:"items"`
}

func (a *App) Growth() (*GrowthResult, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.tree == nil {
		return nil, errNoScan
	}
	r := &GrowthResult{Items: []Growth{}}
	if a.prev == nil {
		return r, nil
	}
	r.PrevTime = a.prev.Time
	alloc := func(n *Node) int64 { return n.Alloc }
	r.Items = growth(a.tree, a.prev, alloc, a.getSettings().DeltaMin<<20)
	for i := range r.Items {
		g := &r.Items[i]
		if g.ID != none {
			g.Path = a.tree.pathOf(g.ID)
		} else {
			g.Path = joinPath(strings.TrimRight(a.tree.RootPath, `\`), g.Path)
		}
	}
	return r, nil
}

// ---------- acciones sobre un elemento ----------

func (a *App) pathOf(id int32) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	t, err := a.treeFor(id)
	if err != nil {
		return "", err
	}
	return t.pathOf(id), nil
}

func (a *App) Reveal(id int32) error {
	p, err := a.pathOf(id)
	if err != nil {
		return err
	}
	return revealInExplorer(p)
}

func (a *App) RevealPath(p string) error { return revealInExplorer(p) }

func (a *App) Open(id int32) error {
	p, err := a.pathOf(id)
	if err != nil {
		return err
	}
	return shellOpen(p)
}

func (a *App) CopyPath(id int32) (string, error) {
	p, err := a.pathOf(id)
	if err != nil {
		return "", err
	}
	return p, runtime.ClipboardSetText(a.ctx, p)
}

type RefreshResult struct {
	Info ScanInfo `json:"info"`
	ID   int32    `json:"id"` // la carpeta releída tiene un identificador nuevo
}

// Refresh vuelve a leer una carpeta (recorriéndola) y actualiza los totales.
func (a *App) Refresh(id int32) (*RefreshResult, error) {
	a.opMu.Lock()
	defer a.opMu.Unlock()
	runtime.EventsEmit(a.ctx, "busy", "Actualizando carpeta…")
	defer runtime.EventsEmit(a.ctx, "busy", "")
	a.mu.Lock()
	defer a.mu.Unlock()
	t, err := a.treeFor(id)
	if err != nil {
		return nil, err
	}
	if !t.Nodes[id].dir() {
		return nil, errors.New("Solo se pueden actualizar carpetas")
	}
	nid := t.Nodes[id].Parent
	if _, err := os.Stat(t.pathOf(id)); err != nil {
		if id == t.Root {
			return nil, errors.New("La carpeta analizada ya no existe")
		}
		t.remove(id)
	} else {
		nid = refreshSubtree(t, id)
	}
	a.afterChange()
	return &RefreshResult{Info: a.info, ID: nid}, nil
}

// Locate busca una ruta en el escaneo; si ya no está, devuelve la carpeta más cercana por encima.
func (a *App) Locate(path string) (int32, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.tree == nil {
		return none, errNoScan
	}
	t := a.tree
	rel := relTo(cleanRoot(path), t.RootPath)
	if rel == cleanRoot(path) && !strings.EqualFold(cleanRoot(path), t.RootPath) {
		return t.Root, nil // fuera del escaneo
	}
	parts := strings.Split(rel, `\`)
	for len(parts) > 0 {
		if id := t.findPath(strings.Join(parts, `\`)); id != none {
			return id, nil
		}
		parts = parts[:len(parts)-1]
	}
	return t.Root, nil
}

// refreshSubtree sustituye el subárbol de id por uno recién leído del disco.
func refreshSubtree(t *Tree, id int32) int32 {
	path := t.pathOf(id)
	old := t.Nodes[id]
	nid := int32(len(t.Nodes))
	t.Nodes = append(t.Nodes, Node{Parent: old.Parent, First: none, Next: none, NameOff: old.NameOff, NameLen: old.NameLen,
		Flags: old.Flags &^ fDenied, MTime: old.MTime})
	_ = walkInto(context.Background(), t, nid, path, &Progress{})
	t.aggregate(nid)
	n := &t.Nodes[nid]
	if id == t.Root {
		t.Nodes[id].Flags |= fDeleted
		t.Root = nid
		n.Parent = none
		return nid
	}
	p := old.Parent
	t.unlink(id)
	t.Nodes[id].Flags |= fDeleted
	t.link(nid, p)
	t.addToAncestors(nid, n.Size-old.Size, n.Alloc-old.Alloc, n.Files-old.Files)
	return nid
}

// afterChange recalcula lo que depende del árbol (con a.mu cogido).
func (a *App) afterChange() {
	t := a.tree
	a.cands = nil
	a.info.RootID = t.Root
	a.info.Files = int64(t.Nodes[t.Root].Files)
	a.info.Scanned = a.measureFn()(&t.Nodes[t.Root])
	a.info.Total, a.info.Free = diskSpace(volumeOf(a.info.Root))
}

func (a *App) Summary() (*ScanInfo, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.tree == nil {
		return nil, errNoScan
	}
	s := a.info
	return &s, nil
}

func unsafeString(b []byte) string { return unsafe.String(unsafe.SliceData(b), len(b)) }
