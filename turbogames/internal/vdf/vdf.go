// Package vdf lee y escribe el formato de texto KeyValues de Valve (los .vdf y
// .acf de Steam).
//
// Guarda el orden de las claves y los valores tal cual (bytes incluidos), para
// que reescribir un archivo que Steam lee no cambie nada que no se haya tocado.
// Las claves se buscan sin distinguir mayúsculas, como hace Steam.
package vdf

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Node es una clave con un valor de texto o con hijos.
type Node struct {
	Key      string
	Value    string
	Children []*Node
	obj      bool
}

// NewObject crea un nodo con hijos.
func NewObject(key string) *Node { return &Node{Key: key, obj: true} }

// IsObject dice si el nodo tiene hijos (aunque esté vacío).
func (n *Node) IsObject() bool { return n != nil && n.obj }

// Kids devuelve los hijos (nil si el nodo no existe), para poder recorrer sin
// comprobar antes.
func (n *Node) Kids() []*Node {
	if n == nil {
		return nil
	}
	return n.Children
}

// Get devuelve el primer hijo con esa clave, o nil.
func (n *Node) Get(key string) *Node {
	if n == nil {
		return nil
	}
	for _, c := range n.Children {
		if strings.EqualFold(c.Key, key) {
			return c
		}
	}
	return nil
}

// Path baja por varias claves seguidas.
func (n *Node) Path(keys ...string) *Node {
	for _, k := range keys {
		n = n.Get(k)
		if n == nil {
			return nil
		}
	}
	return n
}

// Str devuelve el valor de texto de un hijo ("" si no existe o es un objeto).
func (n *Node) Str(key string) string {
	c := n.Get(key)
	if c == nil || c.obj {
		return ""
	}
	return c.Value
}

// Has dice si existe el hijo.
func (n *Node) Has(key string) bool { return n.Get(key) != nil }

// Set pone el valor de un hijo, creándolo al final si no existe.
func (n *Node) Set(key, value string) {
	if c := n.Get(key); c != nil {
		c.Value, c.Children, c.obj = value, nil, false
		return
	}
	n.obj = true
	n.Children = append(n.Children, &Node{Key: key, Value: value})
}

// SetIfPresent cambia el valor solo si el hijo ya existe.
func (n *Node) SetIfPresent(key, value string) {
	if c := n.Get(key); c != nil && !c.obj {
		c.Value = value
	}
}

// Add añade un hijo al final (aunque ya haya otro con la misma clave).
func (n *Node) Add(c *Node) {
	n.obj = true
	n.Children = append(n.Children, c)
}

// Remove quita todos los hijos con esa clave y dice cuántos quitó.
func (n *Node) Remove(key string) int {
	if n == nil {
		return 0
	}
	out := n.Children[:0]
	removed := 0
	for _, c := range n.Children {
		if strings.EqualFold(c.Key, key) {
			removed++
			continue
		}
		out = append(out, c)
	}
	for i := len(out); i < len(n.Children); i++ {
		n.Children[i] = nil
	}
	n.Children = out
	return removed
}

// ---------- lectura ----------

// ErrSyntax indica que el archivo no tiene el formato esperado.
var ErrSyntax = errors.New("vdf: formato no válido")

type tokKind int

const (
	tEOF tokKind = iota
	tStr
	tOpen
	tClose
	tCond
)

type lexer struct {
	b   []byte
	pos int
}

func (l *lexer) next() (tokKind, string, error) {
	for l.pos < len(l.b) {
		c := l.b[l.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			l.pos++
		case c == '/' && l.pos+1 < len(l.b) && l.b[l.pos+1] == '/':
			for l.pos < len(l.b) && l.b[l.pos] != '\n' {
				l.pos++
			}
		case c == '{':
			l.pos++
			return tOpen, "", nil
		case c == '}':
			l.pos++
			return tClose, "", nil
		case c == '[':
			// condicional de plataforma, como [$WIN32]: se ignora
			end := bytes.IndexByte(l.b[l.pos:], ']')
			if end < 0 {
				return tEOF, "", ErrSyntax
			}
			s := string(l.b[l.pos : l.pos+end+1])
			l.pos += end + 1
			return tCond, s, nil
		case c == '"':
			l.pos++
			var sb strings.Builder
			for {
				if l.pos >= len(l.b) {
					return tEOF, "", fmt.Errorf("%w: comillas sin cerrar", ErrSyntax)
				}
				c := l.b[l.pos]
				if c == '"' {
					l.pos++
					return tStr, sb.String(), nil
				}
				if c == '\\' && l.pos+1 < len(l.b) {
					switch l.b[l.pos+1] {
					case '\\':
						sb.WriteByte('\\')
						l.pos += 2
						continue
					case '"':
						sb.WriteByte('"')
						l.pos += 2
						continue
					case 'n':
						sb.WriteByte('\n')
						l.pos += 2
						continue
					case 't':
						sb.WriteByte('\t')
						l.pos += 2
						continue
					}
				}
				sb.WriteByte(c)
				l.pos++
			}
		default:
			// texto sin comillas: hasta un espacio, llave o comilla
			start := l.pos
			for l.pos < len(l.b) {
				c := l.b[l.pos]
				if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '{' || c == '}' || c == '"' {
					break
				}
				l.pos++
			}
			return tStr, string(l.b[start:l.pos]), nil
		}
	}
	return tEOF, "", nil
}

// Parse lee un documento entero. Devuelve un nodo raíz sin clave cuyos hijos son
// las claves de primer nivel (normalmente una sola, como "users").
func Parse(b []byte) (*Node, error) {
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	l := &lexer{b: b}
	root := NewObject("")
	if err := parseInto(l, root, 0); err != nil {
		return nil, err
	}
	return root, nil
}

func parseInto(l *lexer, parent *Node, depth int) error {
	if depth > 64 {
		return fmt.Errorf("%w: demasiado anidado", ErrSyntax)
	}
	for {
		k, key, err := l.next()
		if err != nil {
			return err
		}
		switch k {
		case tEOF:
			if depth > 0 {
				return fmt.Errorf("%w: falta cerrar una llave", ErrSyntax)
			}
			return nil
		case tClose:
			if depth == 0 {
				return fmt.Errorf("%w: llave de cierre de más", ErrSyntax)
			}
			return nil
		case tCond:
			continue
		case tOpen:
			return fmt.Errorf("%w: llave sin clave", ErrSyntax)
		}
		// key leída: ahora viene un valor o una llave
		k, val, err := l.next()
		if err != nil {
			return err
		}
		for k == tCond {
			k, val, err = l.next()
			if err != nil {
				return err
			}
		}
		switch k {
		case tStr:
			parent.Children = append(parent.Children, &Node{Key: key, Value: val})
			// un condicional detrás del valor: se salta
			save := l.pos
			if k2, _, err := l.next(); err != nil || k2 != tCond {
				l.pos = save
			}
		case tOpen:
			child := NewObject(key)
			if err := parseInto(l, child, depth+1); err != nil {
				return err
			}
			parent.Children = append(parent.Children, child)
		default:
			return fmt.Errorf("%w: falta el valor de %q", ErrSyntax, key)
		}
	}
}

// ParseFile lee un archivo .vdf.
func ParseFile(path string) (*Node, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

// ---------- escritura ----------

func quote(s string) string {
	if !strings.ContainsAny(s, "\\\"\n\t") {
		return `"` + s + `"`
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`)
	return `"` + r.Replace(s) + `"`
}

// Marshal escribe el documento con el mismo estilo que Steam (tabuladores).
func (n *Node) Marshal() []byte {
	var b bytes.Buffer
	for _, c := range n.Children {
		write(&b, c, 0)
	}
	return b.Bytes()
}

func write(b *bytes.Buffer, n *Node, depth int) {
	ind := strings.Repeat("\t", depth)
	if n.obj {
		b.WriteString(ind + quote(n.Key) + "\n" + ind + "{\n")
		for _, c := range n.Children {
			write(b, c, depth+1)
		}
		b.WriteString(ind + "}\n")
		return
	}
	b.WriteString(ind + quote(n.Key) + "\t\t" + quote(n.Value) + "\n")
}

// WriteFile guarda el documento sin dejarlo a medias: escribe a un temporal y
// lo cambia por el original.
func WriteFile(path string, root *Node) error {
	tmp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".turbogames.tmp")
	if err := os.WriteFile(tmp, root.Marshal(), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
