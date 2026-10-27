package main

import (
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Banderas de cada nodo.
const (
	fDir     uint8 = 1 << iota
	fDenied        // no se pudo leer la carpeta
	fDeleted       // borrado desde TurboCUT
	fLink          // enlace o punto de montaje: no se entra
	fHidden
	fSystem
	fCloud    // archivo de OneDrive sin descargar
	fHardLink // otro nombre de un archivo que ya cuenta en otra carpeta (enlace duro)
)

const none int32 = -1

// Node ocupa 48 bytes: un disco con 3 millones de archivos cabe en ~150 MB.
type Node struct {
	Size    int64 // bytes de los archivos
	Alloc   int64 // bytes ocupados en el disco
	Parent  int32
	First   int32 // primer hijo
	Next    int32 // siguiente hermano
	Files   int32 // archivos dentro (carpetas)
	NameOff uint32
	NameLen uint16
	Ext     uint16
	Flags   uint8
	MTime   uint32 // segundos Unix
}

func (n *Node) dir() bool { return n.Flags&fDir != 0 }

// Tree guarda todo el escaneo en arrays planos (rápido y con poca memoria).
type Tree struct {
	mu       sync.Mutex // solo para construir en paralelo
	Nodes    []Node
	names    []byte
	lower    []byte // nombres en minúsculas para buscar (se crea al buscar)
	exts     []string
	extIdx   map[string]uint16
	Root     int32
	RootPath string

	// Modo normal: un archivo con varios nombres (enlaces duros, como los de
	// C:\Windows\WinSxS) cuenta una sola vez, igual que en el modo turbo. fids
	// guarda, ordenados, los archivos que tienen más de un nombre en el análisis.
	trackLinks bool
	noFileIDs  atomic.Bool // el sistema de archivos no da identificadores (FAT…)
	fids       []fidRef
	linked     map[int32]bool // los nodos que están en fids
}

func (t *Tree) indexLinks() {
	t.linked = make(map[int32]bool, len(t.fids))
	for _, r := range t.fids {
		t.linked[r.id] = true
	}
}

type fidRef struct {
	fid uint64
	id  int32
}

func sortFids(f []fidRef) {
	sort.Slice(f, func(i, j int) bool {
		if f[i].fid != f[j].fid {
			return f[i].fid < f[j].fid
		}
		return f[i].id < f[j].id
	})
}

// settleLinks decide qué nombre de cada archivo con varios cuenta su tamaño: el de
// la ruta que va antes por orden alfabético (así sale siempre el mismo y «Qué ha
// crecido» no ve cambios que no son). Con keepAll se guardan también los archivos
// de un solo nombre (para juntar una carpeta releída con el resto del análisis).
func (t *Tree) settleLinks(keepAll bool) {
	sortFids(t.fids)
	keep := t.fids[:0]
	for i := 0; i < len(t.fids); {
		j := i + 1
		for j < len(t.fids) && t.fids[j].fid == t.fids[i].fid {
			j++
		}
		if j-i > 1 {
			best, bp := i, strings.ToLower(t.pathOf(t.fids[i].id))
			for k := i + 1; k < j; k++ {
				if p := strings.ToLower(t.pathOf(t.fids[k].id)); p < bp {
					best, bp = k, p
				}
			}
			for k := i; k < j; k++ {
				if k != best {
					t.Nodes[t.fids[k].id].Flags |= fHardLink
				}
			}
		}
		if j-i > 1 || keepAll {
			keep = append(keep, t.fids[i:j]...)
		}
		i = j
	}
	t.fids = keep
	t.indexLinks()
}

// mergeLinks junta los archivos de una carpeta releída (nt, copiada a partir de
// base) con los del resto del análisis.
func (t *Tree) mergeLinks(nt *Tree, base int32) {
	if len(nt.fids) == 0 && len(t.fids) == 0 {
		return
	}
	inNt := map[uint64]int{}
	for _, r := range nt.fids {
		inNt[r.fid]++
	}
	// Fuera lo que ya no está (la carpeta vieja, archivos borrados).
	live := t.fids[:0]
	for _, r := range t.fids {
		if t.alive(r.id) {
			live = append(live, r)
		}
	}
	t.fids = live
	var add []fidRef
	for _, r := range nt.fids {
		id := base + r.id
		i := sort.Search(len(t.fids), func(k int) bool { return t.fids[k].fid >= r.fid })
		inMain, counted := false, false
		for k := i; k < len(t.fids) && t.fids[k].fid == r.fid; k++ {
			inMain = true
			if t.Nodes[t.fids[k].id].Flags&fHardLink == 0 {
				counted = true
			}
		}
		if counted {
			t.Nodes[id].Flags |= fHardLink // ya cuenta con otro nombre fuera de esta carpeta
		}
		if inMain || inNt[r.fid] > 1 {
			add = append(add, fidRef{r.fid, id})
		}
	}
	t.fids = append(t.fids, add...)
	sortFids(t.fids)
	t.indexLinks()
}

// fixLinks: si se ha quitado el nombre que contaba un archivo con varios, pasa a
// contar otro de los que quedan (el espacio sigue ocupado).
func (t *Tree) fixLinks() {
	for i := 0; i < len(t.fids); {
		j := i + 1
		for j < len(t.fids) && t.fids[j].fid == t.fids[i].fid {
			j++
		}
		counted, spare := false, int32(none)
		for k := i; k < j; k++ {
			id := t.fids[k].id
			if !t.alive(id) {
				continue
			}
			if t.Nodes[id].Flags&fHardLink == 0 {
				counted = true
			} else if spare == none {
				spare = id
			}
		}
		if !counted && spare != none {
			n := &t.Nodes[spare]
			n.Flags &^= fHardLink
			t.addToAncestors(spare, n.Size, n.Alloc, 0)
		}
		i = j
	}
}

func newTree(rootPath string, sizeHint int) *Tree {
	t := &Tree{
		Nodes:    make([]Node, 0, sizeHint),
		names:    make([]byte, 0, sizeHint*16),
		exts:     []string{""},
		extIdx:   map[string]uint16{"": 0},
		RootPath: rootPath,
	}
	return t
}

func (t *Tree) name(id int32) string {
	if id == t.Root {
		return t.RootPath
	}
	n := &t.Nodes[id]
	return string(t.names[n.NameOff : n.NameOff+uint32(n.NameLen)])
}

func (t *Tree) ext(id int32) string { return t.exts[t.Nodes[id].Ext] }

// addName copia el nombre al almacén común. Llamar con t.mu cogido si hay concurrencia.
func (t *Tree) addName(s string) (uint32, uint16) {
	if len(s) > 65535 {
		s = s[:65535]
	}
	off := uint32(len(t.names))
	t.names = append(t.names, s...)
	return off, uint16(len(s))
}

// extID devuelve el identificador de la extensión del nombre (en minúsculas).
func (t *Tree) extID(name string) uint16 {
	e := extOf(name)
	if id, ok := t.extIdx[e]; ok {
		return id
	}
	if len(t.exts) >= 65535 {
		return 0
	}
	id := uint16(len(t.exts))
	t.exts = append(t.exts, e)
	t.extIdx[e] = id
	return id
}

func extOf(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i <= 0 || i == len(name)-1 || len(name)-i > 12 {
		return ""
	}
	e := name[i+1:]
	for j := 0; j < len(e); j++ {
		c := e[j]
		if c >= 'A' && c <= 'Z' {
			b := []byte(e)
			for k := j; k < len(b); k++ {
				if b[k] >= 'A' && b[k] <= 'Z' {
					b[k] += 'a' - 'A'
				}
			}
			return string(b)
		}
		if c == ' ' {
			return ""
		}
	}
	return e
}

// link mete id como hijo de parent.
func (t *Tree) link(id, parent int32) {
	t.Nodes[id].Parent = parent
	t.Nodes[id].Next = t.Nodes[parent].First
	t.Nodes[parent].First = id
}

// unlink saca id de la lista de hijos de su padre.
func (t *Tree) unlink(id int32) {
	p := t.Nodes[id].Parent
	if p == none {
		return
	}
	if t.Nodes[p].First == id {
		t.Nodes[p].First = t.Nodes[id].Next
	} else {
		for c := t.Nodes[p].First; c != none; c = t.Nodes[c].Next {
			if t.Nodes[c].Next == id {
				t.Nodes[c].Next = t.Nodes[id].Next
				break
			}
		}
	}
	t.Nodes[id].Next = none
}

func (t *Tree) children(id int32, fn func(c int32)) {
	for c := t.Nodes[id].First; c != none; c = t.Nodes[c].Next {
		fn(c)
	}
}

// preorder recorre el subárbol de id (id incluido) sin recursión.
func (t *Tree) preorder(id int32, fn func(n int32) bool) {
	stack := []int32{id}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if !fn(n) {
			continue
		}
		for c := t.Nodes[n].First; c != none; c = t.Nodes[c].Next {
			stack = append(stack, c)
		}
	}
}

// aggregate suma tamaños de abajo arriba dentro del subárbol de id.
func (t *Tree) aggregate(id int32) {
	order := make([]int32, 0, 1024)
	t.preorder(id, func(n int32) bool {
		nd := &t.Nodes[n]
		if nd.dir() {
			nd.Size, nd.Alloc, nd.Files = 0, 0, 0
		}
		order = append(order, n)
		return true
	})
	for i := len(order) - 1; i > 0; i-- {
		n := &t.Nodes[order[i]]
		p := &t.Nodes[n.Parent]
		if n.Flags&fHardLink == 0 {
			p.Size += n.Size
			p.Alloc += n.Alloc
		}
		if n.dir() {
			p.Files += n.Files
		} else {
			p.Files++
		}
		if n.MTime > p.MTime {
			p.MTime = n.MTime
		}
	}
}

// addToAncestors suma (o resta) a todas las carpetas por encima de id.
func (t *Tree) addToAncestors(id int32, size, alloc int64, files int32) {
	steps := len(t.Nodes) // tope: un árbol dañado nunca debe dejarlo en bucle
	for p := t.Nodes[id].Parent; p != none && steps > 0; p, steps = t.Nodes[p].Parent, steps-1 {
		t.Nodes[p].Size += size
		t.Nodes[p].Alloc += alloc
		t.Nodes[p].Files += files
		if p == t.Root {
			break
		}
	}
}

// remove quita id del árbol y descuenta su tamaño de las carpetas de arriba.
func (t *Tree) remove(id int32) {
	n := &t.Nodes[id]
	files := n.Files
	if !n.dir() {
		files = 1
	}
	size, alloc := n.Size, n.Alloc
	if n.Flags&fHardLink != 0 {
		size, alloc = 0, 0 // no contaba: su tamaño va en otro nombre
	}
	t.addToAncestors(id, -size, -alloc, -files)
	t.unlink(id)
	n.Flags |= fDeleted
	// Solo si lo quitado puede ser el nombre que contaba un archivo con varios.
	if len(t.fids) > 0 && (n.dir() || t.linked[id]) {
		t.fixLinks()
	}
}

func (t *Tree) alive(id int32) bool {
	if id < 0 || int(id) >= len(t.Nodes) {
		return false
	}
	for n, steps := id, len(t.Nodes); steps > 0; n, steps = t.Nodes[n].Parent, steps-1 {
		if t.Nodes[n].Flags&fDeleted != 0 {
			return false
		}
		if n == t.Root {
			return true
		}
		if t.Nodes[n].Parent == none {
			return false
		}
	}
	return false // más pasos que nodos: hay un ciclo
}

// pathOf construye la ruta completa de id.
func (t *Tree) pathOf(id int32) string {
	if id == t.Root {
		return t.RootPath
	}
	parts := []string{}
	for n := id; n != t.Root && n != none && len(parts) < len(t.Nodes); n = t.Nodes[n].Parent {
		parts = append(parts, t.name(n))
	}
	var b strings.Builder
	b.WriteString(t.RootPath)
	for i := len(parts) - 1; i >= 0; i-- {
		if s := b.String(); !strings.HasSuffix(s, `\`) {
			b.WriteByte('\\')
		}
		b.WriteString(parts[i])
	}
	return b.String()
}

// relPath es la ruta desde la raíz del escaneo, en minúsculas (para el historial).
func (t *Tree) relPath(id int32) string {
	if id == t.Root {
		return ""
	}
	parts := []string{}
	for n := id; n != t.Root && n != none && len(parts) < len(t.Nodes); n = t.Nodes[n].Parent {
		parts = append(parts, t.name(n))
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.ToLower(strings.Join(parts, `\`))
}

// findPath busca una ruta (relativa a la raíz) sin distinguir mayúsculas.
func (t *Tree) findPath(rel string) int32 {
	cur := t.Root
	for _, part := range strings.Split(strings.Trim(rel, `\`), `\`) {
		if part == "" {
			continue
		}
		next := none
		for c := t.Nodes[cur].First; c != none; c = t.Nodes[c].Next {
			if strings.EqualFold(t.name(c), part) {
				next = c
				break
			}
		}
		if next == none {
			return none
		}
		cur = next
	}
	return cur
}
