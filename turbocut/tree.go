package main

import (
	"strings"
	"sync"
)

// Banderas de cada nodo.
const (
	fDir     uint8 = 1 << iota
	fDenied        // no se pudo leer la carpeta
	fDeleted       // borrado desde TurboCUT
	fLink          // enlace o punto de montaje: no se entra
	fHidden
	fSystem
	fCloud // archivo de OneDrive sin descargar
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
		p.Size += n.Size
		p.Alloc += n.Alloc
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
	for p := t.Nodes[id].Parent; p != none; p = t.Nodes[p].Parent {
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
	t.addToAncestors(id, -n.Size, -n.Alloc, -files)
	t.unlink(id)
	n.Flags |= fDeleted
}

func (t *Tree) alive(id int32) bool {
	if id < 0 || int(id) >= len(t.Nodes) {
		return false
	}
	for n := id; ; n = t.Nodes[n].Parent {
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
}

// pathOf construye la ruta completa de id.
func (t *Tree) pathOf(id int32) string {
	if id == t.Root {
		return t.RootPath
	}
	parts := []string{}
	for n := id; n != t.Root && n != none; n = t.Nodes[n].Parent {
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
	for n := id; n != t.Root && n != none; n = t.Nodes[n].Parent {
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
