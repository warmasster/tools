package main

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Snapshot guarda el tamaño de las carpetas grandes de un escaneo para poder
// decir la próxima vez qué ha crecido.
type Snapshot struct {
	Root  string           `json:"root"`
	Time  int64            `json:"time"`
	Total int64            `json:"total"`
	Dirs  map[string]int64 `json:"dirs"` // ruta relativa en minúsculas -> bytes en disco
	// Mode: "mft" o "walk" (vacío en los de antes de la 1.1.2, cuando el modo normal
	// contaba varias veces los archivos con varios nombres).
	Mode string `json:"mode,omitempty"`
}

// growthNote avisa si el análisis anterior se hizo de otra forma: los enlaces
// duros pueden caer en otra carpeta y salir como cambios que no son.
func growthNote(prev *Snapshot, mode string) string {
	name := map[string]string{"mft": "turbo", "walk": "normal"}
	switch {
	case prev == nil:
		return ""
	case prev.Mode != "" && prev.Mode != mode:
		return "El análisis anterior se hizo en modo " + name[prev.Mode] + " y este en modo " + name[mode] +
			": los archivos con varios nombres (enlaces duros, como los de C:\\Windows) pueden contar en otra carpeta, así que algunas diferencias en carpetas del sistema no son reales."
	case prev.Mode == "" && mode == "walk":
		return "El análisis anterior es de una versión de TurboCUT que contaba varias veces los archivos con varios nombres (enlaces duros): lo que haya bajado en carpetas del sistema como C:\\Windows puede no ser real."
	}
	return ""
}

const (
	snapMinSize  = 32 << 20 // carpetas de 32 MB o más
	snapMaxDepth = 9
)

func snapPath(root string) string {
	h := sha1.Sum([]byte(strings.ToLower(root)))
	d := filepath.Join(dataDir(), "historial")
	_ = os.MkdirAll(d, 0o755)
	return filepath.Join(d, hex.EncodeToString(h[:8])+".json")
}

func loadSnapshot(root string) *Snapshot {
	var s Snapshot
	if err := loadJSON(snapPath(root), &s); err != nil || s.Dirs == nil {
		return nil
	}
	return &s
}

func makeSnapshot(t *Tree) *Snapshot {
	s := &Snapshot{Root: t.RootPath, Time: time.Now().Unix(), Total: t.Nodes[t.Root].Alloc, Dirs: map[string]int64{}}
	type item struct {
		id    int32
		path  string
		depth int
	}
	stack := []item{{t.Root, "", 0}}
	for len(stack) > 0 {
		it := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if it.id != t.Root {
			s.Dirs[it.path] = t.Nodes[it.id].Alloc
		}
		if it.depth >= snapMaxDepth {
			continue
		}
		t.children(it.id, func(c int32) {
			n := &t.Nodes[c]
			if !n.dir() || n.Alloc < snapMinSize {
				return
			}
			p := strings.ToLower(t.name(c))
			if it.path != "" {
				p = it.path + `\` + p
			}
			stack = append(stack, item{c, p, it.depth + 1})
		})
	}
	return s
}

// Como mucho se guardan los últimos maxSnapshots análisis (uno por unidad o carpeta,
// cada uno de unos 100 KB), así el historial nunca pasa de unos pocos MB.
const maxSnapshots = 30

func saveSnapshot(s *Snapshot) error {
	if err := saveJSON(snapPath(s.Root), s); err != nil {
		return err
	}
	pruneSnapshots(filepath.Dir(snapPath(s.Root)), maxSnapshots)
	return nil
}

// pruneSnapshots borra los análisis guardados más antiguos si hay más de max.
func pruneSnapshots(dir string, max int) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type f struct {
		path string
		mod  time.Time
	}
	var files []f
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if info, err := e.Info(); err == nil {
			files = append(files, f{filepath.Join(dir, e.Name()), info.ModTime()})
		}
	}
	if len(files) <= max {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	for _, old := range files[max:] {
		_ = os.Remove(old.path)
	}
}

type Growth struct {
	ID    int32  `json:"id"` // -1 si la carpeta ya no existe
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	Delta int64  `json:"delta"`
	Gone  bool   `json:"gone,omitempty"`
	New   bool   `json:"new,omitempty"`
}

// growth compara el árbol actual con el escaneo anterior y se queda con las
// carpetas más concretas que explican cada cambio.
func growth(t *Tree, prev *Snapshot, measure func(*Node) int64, minDelta int64) []Growth {
	if prev == nil {
		return nil
	}
	cur := makeSnapshot(t)
	var all []Growth
	for p, sz := range cur.Dirs {
		old, ok := prev.Dirs[p]
		if !ok {
			// carpeta nueva: solo si su padre ya existía (si no, ya lo cuenta el padre)
			parent := ""
			if i := strings.LastIndexByte(p, '\\'); i >= 0 {
				parent = p[:i]
			}
			if _, pok := prev.Dirs[parent]; !pok && parent != "" {
				continue
			}
			if sz >= minDelta {
				all = append(all, Growth{Path: p, Size: sz, Delta: sz, New: true})
			}
			continue
		}
		if d := sz - old; d >= minDelta || d <= -minDelta {
			all = append(all, Growth{Path: p, Size: sz, Delta: d})
		}
	}
	for p, old := range prev.Dirs {
		if _, ok := cur.Dirs[p]; ok {
			continue
		}
		if t.findPath(p) == none && old >= snapMinSize && old >= minDelta {
			all = append(all, Growth{Path: p, Size: 0, Delta: -old, Gone: true})
		}
	}
	sort.Slice(all, func(i, j int) bool { return abs64(all[i].Delta) > abs64(all[j].Delta) })
	if len(all) > 400 {
		all = all[:400]
	}
	// Quita una carpeta si un hijo suyo explica ya el 75 % de su cambio.
	keep := make([]bool, len(all))
	for i := range all {
		keep[i] = true
		for j := range all {
			if i == j || !strings.HasPrefix(all[j].Path, all[i].Path+`\`) {
				continue
			}
			if (all[j].Delta > 0) == (all[i].Delta > 0) && abs64(all[j].Delta)*4 >= abs64(all[i].Delta)*3 {
				keep[i] = false
				break
			}
		}
	}
	var out []Growth
	for i, g := range all {
		if !keep[i] {
			continue
		}
		g.ID = none
		if !g.Gone {
			g.ID = t.findPath(g.Path)
			if g.ID != none {
				g.Size = measure(&t.Nodes[g.ID])
			}
		}
		out = append(out, g)
		if len(out) >= 40 {
			break
		}
	}
	return out
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
