package steam

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"unicode/utf16"

	"turbogames/internal/vdf"
)

// appinfo.vdf es la caché binaria donde Steam guarda los datos de cada juego que
// conoce (los de todas las cuentas que han entrado en este PC). De ahí sacamos
// el nombre y el tipo de los juegos que no están instalados.
//
// Formato (versiones 27, 28 y 29):
//
//	uint32 magic, uint32 universo, [int64 posición de la tabla de cadenas (v29)]
//	por cada app: uint32 appid (0 = fin), uint32 tamaño, uint32 estado,
//	  uint32 actualizado, uint64 token, 20 bytes sha1, uint32 cambio,
//	  [20 bytes sha1 del binario (v28+)], KeyValues binario
//	(v29) tabla de cadenas: uint32 número, cadenas terminadas en 0
const (
	appInfoV27 = 0x07564427
	appInfoV28 = 0x07564428
	appInfoV29 = 0x07564429
)

// AppMeta son los datos de un juego que nos interesan.
type AppMeta struct {
	Name     string
	Type     string // "game", "demo", "tool", "application", "dlc", "music"…
	Icon     string // hash del icono pequeño (clienticon)
	Assets   map[string]string
	Parent   uint32 // demos y DLC: el juego principal
	OSList   string
	Released int64
}

// ParseAppInfo lee appinfo.vdf y devuelve el bloque "common" de cada app.
func ParseAppInfo(path string) (map[uint32]AppMeta, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseAppInfo(b)
}

type binReader struct {
	b       []byte
	pos     int
	strings []string // v29: las claves son índices en esta tabla
}

var errShort = errors.New("appinfo: archivo cortado")

func (r *binReader) u32() (uint32, error) {
	if r.pos+4 > len(r.b) {
		return 0, errShort
	}
	v := binary.LittleEndian.Uint32(r.b[r.pos:])
	r.pos += 4
	return v, nil
}

func (r *binReader) cstr() (string, error) {
	for i := r.pos; i < len(r.b); i++ {
		if r.b[i] == 0 {
			s := string(r.b[r.pos:i])
			r.pos = i + 1
			return s, nil
		}
	}
	return "", errShort
}

func (r *binReader) key() (string, error) {
	if r.strings == nil {
		return r.cstr()
	}
	i, err := r.u32()
	if err != nil {
		return "", err
	}
	if int(i) >= len(r.strings) {
		return "", fmt.Errorf("appinfo: índice de cadena %d fuera de la tabla", i)
	}
	return r.strings[i], nil
}

func parseAppInfo(b []byte) (map[uint32]AppMeta, error) {
	if len(b) < 8 {
		return nil, errShort
	}
	magic := binary.LittleEndian.Uint32(b)
	r := &binReader{b: b, pos: 8}
	switch magic {
	case appInfoV27, appInfoV28:
	case appInfoV29:
		if len(b) < 16 {
			return nil, errShort
		}
		off := int64(binary.LittleEndian.Uint64(b[8:]))
		r.pos = 16
		if off <= 16 || off >= int64(len(b)) {
			return nil, errors.New("appinfo: tabla de cadenas no válida")
		}
		t := &binReader{b: b, pos: int(off)}
		n, err := t.u32()
		if err != nil {
			return nil, err
		}
		if n > 10_000_000 {
			return nil, errors.New("appinfo: tabla de cadenas no válida")
		}
		r.strings = make([]string, 0, n)
		for i := uint32(0); i < n; i++ {
			s, err := t.cstr()
			if err != nil {
				return nil, err
			}
			r.strings = append(r.strings, s)
		}
	default:
		return nil, fmt.Errorf("appinfo: versión desconocida %#x", magic)
	}

	out := make(map[uint32]AppMeta, 1024)
	for {
		appid, err := r.u32()
		if err != nil {
			return out, err
		}
		if appid == 0 {
			return out, nil
		}
		size, err := r.u32()
		if err != nil {
			return out, err
		}
		end := r.pos + int(size)
		if end > len(r.b) || end < r.pos {
			return out, errShort
		}
		// cabecera fija antes del KeyValues
		hdr := 4 + 4 + 8 + 20 + 4
		if magic != appInfoV27 {
			hdr += 20
		}
		r.pos += hdr
		if r.pos > end {
			return out, errShort
		}
		root := vdf.NewObject("")
		if err := r.readObject(root, end, 0); err == nil {
			if m, ok := metaFrom(root); ok {
				out[appid] = m
			}
		}
		r.pos = end
	}
}

// readObject lee un bloque de KeyValues binario hasta su marca de fin.
func (r *binReader) readObject(parent *vdf.Node, end, depth int) error {
	if depth > 32 {
		return errors.New("appinfo: demasiado anidado")
	}
	for r.pos < end {
		t := r.b[r.pos]
		r.pos++
		if t == 0x08 || t == 0x0B {
			return nil
		}
		k, err := r.key()
		if err != nil {
			return err
		}
		switch t {
		case 0x00:
			child := vdf.NewObject(k)
			if err := r.readObject(child, end, depth+1); err != nil {
				return err
			}
			parent.Add(child)
		case 0x01:
			s, err := r.cstr()
			if err != nil {
				return err
			}
			parent.Add(&vdf.Node{Key: k, Value: s})
		case 0x02, 0x04, 0x06:
			v, err := r.u32()
			if err != nil {
				return err
			}
			parent.Add(&vdf.Node{Key: k, Value: strconv.FormatInt(int64(int32(v)), 10)})
		case 0x03:
			v, err := r.u32()
			if err != nil {
				return err
			}
			parent.Add(&vdf.Node{Key: k, Value: strconv.FormatFloat(float64(math.Float32frombits(v)), 'g', -1, 32)})
		case 0x05:
			var u []uint16
			for {
				if r.pos+2 > end {
					return errShort
				}
				c := binary.LittleEndian.Uint16(r.b[r.pos:])
				r.pos += 2
				if c == 0 {
					break
				}
				u = append(u, c)
			}
			parent.Add(&vdf.Node{Key: k, Value: string(utf16.Decode(u))})
		case 0x07, 0x0A:
			if r.pos+8 > end {
				return errShort
			}
			v := binary.LittleEndian.Uint64(r.b[r.pos:])
			r.pos += 8
			s := strconv.FormatUint(v, 10)
			if t == 0x0A {
				s = strconv.FormatInt(int64(v), 10)
			}
			parent.Add(&vdf.Node{Key: k, Value: s})
		default:
			return fmt.Errorf("appinfo: tipo %#x desconocido", t)
		}
	}
	return nil
}

func metaFrom(root *vdf.Node) (AppMeta, bool) {
	c := root.Path("appinfo", "common")
	if c == nil {
		return AppMeta{}, false
	}
	m := AppMeta{
		Name:   strings.TrimSpace(c.Str("name")),
		Type:   strings.ToLower(c.Str("type")),
		Icon:   c.Str("clienticon"),
		OSList: c.Str("oslist"),
	}
	if m.Icon == "" {
		m.Icon = c.Str("icon")
	}
	if p, err := strconv.ParseUint(c.Str("parent"), 10, 32); err == nil {
		m.Parent = uint32(p)
	}
	m.Released, _ = strconv.ParseInt(c.Str("steam_release_date"), 10, 64)
	// rutas de las imágenes de la biblioteca (pueden llevar un hash delante)
	if la := c.Get("library_assets_full"); la != nil {
		m.Assets = map[string]string{}
		for _, a := range la.Children {
			img := a.Path("image", "english")
			if img == nil {
				if im := a.Get("image"); im != nil && len(im.Children) > 0 {
					img = im.Children[0]
				}
			}
			if img != nil && !img.IsObject() && img.Value != "" {
				m.Assets[strings.ToLower(a.Key)] = img.Value
			}
		}
	}
	if h := c.Path("header_image", "english"); h != nil && h.Value != "" {
		if m.Assets == nil {
			m.Assets = map[string]string{}
		}
		m.Assets["header"] = h.Value
	}
	return m, m.Name != ""
}
