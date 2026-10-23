package main

import (
	"bytes"
	"container/heap"
	"strings"
	"time"
	"unicode"
)

// Búsqueda por nombre en todo lo analizado, con filtros. Se hace en memoria
// sobre el árbol del último análisis, así que va al instante mientras se escribe.

type FindQuery struct {
	Text    string `json:"text"`
	Scope   int32  `json:"scope"`   // carpeta donde buscar (la raíz = todo)
	Cat     int    `json:"cat"`     // -1: cualquier tipo
	Kind    string `json:"kind"`    // "", "files" o "dirs"
	MinSize int64  `json:"minSize"` // bytes (0: sin límite)
	MaxSize int64  `json:"maxSize"`
	After   int64  `json:"after"` // modificado después de (unix, 0: sin límite)
	Before  int64  `json:"before"`
	Sort    string `json:"sort"` // "size", "name" o "mtime"
}

type FindResult struct {
	Items  []Item  `json:"items"`
	Total  int     `json:"total"`  // coincidencias en total (se devuelven las primeras)
	Size   int64   `json:"size"`   // lo que suman los archivos que coinciden
	Ms     float64 `json:"ms"`     // lo que ha tardado
	Base   string  `json:"base"`   // carpeta donde se ha buscado
	Tokens int     `json:"tokens"` // palabras que se han buscado
}

const findLimit = 500

// foldInPlace pasa a minúsculas sin cambiar la longitud en bytes: ASCII y las
// letras con tilde y la Ñ (Á→á, Ñ→ñ…), que en UTF-8 ocupan lo mismo.
func foldInPlace(b []byte) {
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		} else if c == 0xC3 && i+1 < len(b) && b[i+1] >= 0x80 && b[i+1] <= 0x9E && b[i+1] != 0x97 {
			b[i+1] += 0x20
			i++
		}
	}
}

// lowerNames devuelve los nombres en minúsculas, con los mismos desplazamientos
// que t.names. Se calcula una vez y se amplía si el árbol ha crecido.
func (t *Tree) lowerNames() []byte {
	if len(t.lower) < len(t.names) {
		start := len(t.lower)
		t.lower = append(t.lower, t.names[start:]...)
		foldInPlace(t.lower[start:])
	}
	return t.lower
}

// parseFind separa el texto en palabras ("entre comillas" vale como una) y
// extensiones (".mp4", "*.mp4" o "ext:mp4,mkv").
func parseFind(text string) (words [][]byte, exts map[string]bool) {
	var parts []string
	var cur strings.Builder
	quoted := false
	flush := func() {
		if cur.Len() > 0 {
			parts = append(parts, cur.String())
			cur.Reset()
		}
	}
	for _, r := range text {
		switch {
		case r == '"':
			quoted = !quoted
			if !quoted {
				flush()
			}
		case unicode.IsSpace(r) && !quoted:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	for _, p := range parts {
		lp := strings.ToLower(p)
		var list []string
		switch {
		case strings.HasPrefix(lp, "ext:"):
			list = strings.Split(lp[4:], ",")
		case strings.HasPrefix(lp, "*.") && !strings.ContainsAny(lp[2:], ".*"):
			list = []string{lp[2:]}
		case strings.HasPrefix(lp, ".") && len(lp) > 1 && !strings.ContainsAny(lp[1:], ".*"):
			list = []string{lp[1:]}
		}
		if list != nil {
			if exts == nil {
				exts = map[string]bool{}
			}
			for _, e := range list {
				if e = strings.TrimPrefix(strings.TrimSpace(e), "."); e != "" {
					exts[e] = true
				}
			}
			continue
		}
		w := []byte(p)
		foldInPlace(w)
		words = append(words, w)
	}
	return words, exts
}

// topK guarda los k mejores según less (el peor arriba del montón para echarlo).
type topK struct {
	ids  []int32
	less func(a, b int32) bool // a va antes que b
	k    int
}

func (h *topK) Len() int           { return len(h.ids) }
func (h *topK) Less(i, j int) bool { return h.less(h.ids[j], h.ids[i]) }
func (h *topK) Swap(i, j int)      { h.ids[i], h.ids[j] = h.ids[j], h.ids[i] }
func (h *topK) Push(x any)         { h.ids = append(h.ids, x.(int32)) }
func (h *topK) Pop() any {
	n := len(h.ids) - 1
	x := h.ids[n]
	h.ids = h.ids[:n]
	return x
}
func (h *topK) offer(id int32) {
	if len(h.ids) < h.k {
		heap.Push(h, id)
	} else if h.less(id, h.ids[0]) {
		h.ids[0] = id
		heap.Fix(h, 0)
	}
}

func (a *App) Find(q FindQuery) (*FindResult, error) {
	start := time.Now()
	a.mu.Lock() // lowerNames puede ampliar el almacén de nombres
	defer a.mu.Unlock()
	if a.tree == nil {
		return nil, errNoScan
	}
	t := a.tree
	scope := q.Scope
	if !t.alive(scope) || !t.Nodes[scope].dir() {
		scope = t.Root
	}
	m := a.measureFn()
	showHidden := a.getSettings().ShowHidden
	words, exts := parseFind(q.Text)
	lower := t.lowerNames()

	less := func(x, y int32) bool { return m(&t.Nodes[x]) > m(&t.Nodes[y]) }
	switch q.Sort {
	case "name":
		less = func(x, y int32) bool {
			nx, ny := &t.Nodes[x], &t.Nodes[y]
			c := bytes.Compare(lower[nx.NameOff:nx.NameOff+uint32(nx.NameLen)], lower[ny.NameOff:ny.NameOff+uint32(ny.NameLen)])
			return c < 0 || c == 0 && x < y
		}
	case "mtime":
		less = func(x, y int32) bool { return t.Nodes[x].MTime > t.Nodes[y].MTime }
	}
	top := &topK{less: less, k: findLimit}
	res := &FindResult{Base: t.pathOf(scope), Tokens: len(words) + len(exts)}

	t.preorder(scope, func(n int32) bool {
		nd := &t.Nodes[n]
		isDir := nd.dir()
		if n == scope {
			return true
		}
		if !showHidden && nd.Flags&(fHidden|fSystem) != 0 {
			return false
		}
		ok := true
		switch {
		case q.Kind == "files" && isDir, q.Kind == "dirs" && !isDir:
			ok = false
		case (exts != nil || q.Cat >= 0) && isDir:
			ok = false
		}
		if ok && !isDir && (exts != nil || q.Cat >= 0) {
			e := t.exts[nd.Ext]
			if exts != nil && !exts[e] || q.Cat >= 0 && int(categoryOf(e)) != q.Cat {
				ok = false
			}
		}
		if ok && (q.MinSize > 0 || q.MaxSize > 0) {
			s := m(nd)
			if s < q.MinSize || q.MaxSize > 0 && s > q.MaxSize {
				ok = false
			}
		}
		if ok && (q.After > 0 && int64(nd.MTime) < q.After || q.Before > 0 && int64(nd.MTime) > q.Before) {
			ok = false
		}
		if ok && len(words) > 0 {
			name := lower[nd.NameOff : nd.NameOff+uint32(nd.NameLen)]
			for _, w := range words {
				if !bytes.Contains(name, w) {
					ok = false
					break
				}
			}
		}
		if ok {
			res.Total++
			if !isDir {
				res.Size += m(nd)
			}
			top.offer(n)
		}
		return isDir && nd.Flags&fLink == 0
	})

	ids := top.ids
	sortIDs(ids, less)
	res.Items = make([]Item, 0, len(ids))
	parents := map[int32]string{}
	for _, id := range ids {
		it := makeItem(t, id, m)
		p := t.Nodes[id].Parent
		path, ok := parents[p]
		if !ok {
			path = t.pathOf(p)
			parents[p] = path
		}
		it.Path = path
		res.Items = append(res.Items, it)
	}
	res.Ms = float64(time.Since(start).Microseconds()) / 1000
	return res, nil
}

func sortIDs(ids []int32, less func(a, b int32) bool) {
	// inserción binaria: son como mucho findLimit elementos
	for i := 1; i < len(ids); i++ {
		x := ids[i]
		lo, hi := 0, i
		for lo < hi {
			mid := (lo + hi) / 2
			if less(x, ids[mid]) {
				hi = mid
			} else {
				lo = mid + 1
			}
		}
		copy(ids[lo+1:i+1], ids[lo:i])
		ids[lo] = x
	}
}
