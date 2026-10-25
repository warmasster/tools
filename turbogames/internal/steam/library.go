package steam

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"turbogames/internal/vdf"
)

// Library es una carpeta de juegos de Steam (puede haber una por disco).
type Library struct {
	Path  string   `json:"path"`
	Label string   `json:"label"`
	Apps  []uint32 `json:"-"`
}

// Libraries lee config\libraryfolders.vdf (o steamapps\libraryfolders.vdf).
// La carpeta de Steam siempre cuenta como biblioteca.
func (in *Install) Libraries() []Library {
	var out []Library
	seen := map[string]bool{}
	add := func(p, label string, apps []uint32) {
		p = filepath.Clean(p)
		k := strings.ToLower(p)
		if seen[k] {
			return
		}
		if st, err := os.Stat(filepath.Join(p, "steamapps")); err != nil || !st.IsDir() {
			return
		}
		seen[k] = true
		out = append(out, Library{Path: p, Label: label, Apps: apps})
	}
	for _, f := range []string{in.Path("config", "libraryfolders.vdf"), in.Path("steamapps", "libraryfolders.vdf")} {
		root, err := vdf.ParseFile(f)
		if err != nil {
			continue
		}
		lf := root.Get("libraryfolders")
		for _, c := range lf.Kids() {
			if !c.IsObject() {
				// formato antiguo: "1" "D:\\SteamLibrary"
				if _, err := strconv.Atoi(c.Key); err == nil && c.Value != "" {
					add(c.Value, "", nil)
				}
				continue
			}
			var apps []uint32
			for _, a := range c.Get("apps").Kids() {
				if id, err := strconv.ParseUint(a.Key, 10, 32); err == nil {
					apps = append(apps, uint32(id))
				}
			}
			add(c.Str("path"), c.Str("label"), apps)
		}
		if len(out) > 0 {
			break
		}
	}
	add(in.Dir, "", nil)
	return out
}

// InstalledApp es un juego instalado (sale de steamapps\appmanifest_<id>.acf).
type InstalledApp struct {
	AppID       uint32 `json:"appid"`
	Name        string `json:"name"`
	InstallDir  string `json:"installDir"` // ruta completa
	Library     string `json:"library"`
	SizeOnDisk  int64  `json:"sizeOnDisk"`
	LastUpdated int64  `json:"lastUpdated"`
	LastPlayed  int64  `json:"lastPlayed"`
	LastOwner   string `json:"lastOwner"` // SteamID64 de la cuenta que lo instaló o actualizó
	StateFlags  int    `json:"stateFlags"`
	BuildID     string `json:"buildId"`
}

// Installed lee los manifiestos de todas las bibliotecas.
func (in *Install) Installed() []InstalledApp {
	var out []InstalledApp
	seen := map[uint32]bool{}
	for _, lib := range in.Libraries() {
		files, _ := filepath.Glob(filepath.Join(lib.Path, "steamapps", "appmanifest_*.acf"))
		for _, f := range files {
			root, err := vdf.ParseFile(f)
			if err != nil {
				continue
			}
			s := root.Get("AppState")
			id, err := strconv.ParseUint(s.Str("appid"), 10, 32)
			if err != nil || seen[uint32(id)] {
				continue
			}
			seen[uint32(id)] = true
			a := InstalledApp{
				AppID:     uint32(id),
				Name:      s.Str("name"),
				Library:   lib.Path,
				LastOwner: s.Str("LastOwner"),
				BuildID:   s.Str("buildid"),
			}
			if d := s.Str("installdir"); d != "" && folderName(d) {
				a.InstallDir = filepath.Join(lib.Path, "steamapps", "common", d)
			}
			a.SizeOnDisk, _ = strconv.ParseInt(s.Str("SizeOnDisk"), 10, 64)
			a.LastUpdated, _ = strconv.ParseInt(s.Str("LastUpdated"), 10, 64)
			a.LastPlayed, _ = strconv.ParseInt(s.Str("LastPlayed"), 10, 64)
			a.StateFlags, _ = strconv.Atoi(s.Str("StateFlags"))
			out = append(out, a)
		}
	}
	return out
}

// UserApp es lo que una cuenta ha jugado a un juego en este PC.
type UserApp struct {
	LastPlayed int64 `json:"lastPlayed"`
	Minutes    int64 `json:"minutes"`
}

// UserApps lee userdata\<id>\config\localconfig.vdf: a qué ha jugado esa
// cuenta y cuánto (solo lo que Steam ha guardado en este PC).
func (in *Install) UserApps(accountID uint32) map[uint32]UserApp {
	out := map[uint32]UserApp{}
	p := in.Path("userdata", strconv.FormatUint(uint64(accountID), 10), "config", "localconfig.vdf")
	root, err := vdf.ParseFile(p)
	if err != nil {
		return out
	}
	apps := root.Path("UserLocalConfigStore", "Software", "Valve", "Steam", "apps")
	if apps == nil {
		apps = root.Path("UserLocalConfigStore", "Software", "Valve", "Steam", "Apps")
	}
	for _, a := range apps.Kids() {
		id, err := strconv.ParseUint(a.Key, 10, 32)
		if err != nil || !a.IsObject() {
			continue
		}
		var ua UserApp
		ua.LastPlayed, _ = strconv.ParseInt(a.Str("LastPlayed"), 10, 64)
		ua.Minutes, _ = strconv.ParseInt(a.Str("Playtime"), 10, 64)
		if ua.LastPlayed == 0 && ua.Minutes == 0 {
			continue
		}
		out[uint32(id)] = ua
	}
	return out
}

// UserPersona devuelve el nombre visible que guardó una cuenta en su
// localconfig.vdf (para cuentas que Steam ya no recuerda).
func (in *Install) UserPersona(accountID uint32) string {
	p := in.Path("userdata", strconv.FormatUint(uint64(accountID), 10), "config", "localconfig.vdf")
	root, err := vdf.ParseFile(p)
	if err != nil {
		return ""
	}
	return root.Path("UserLocalConfigStore", "friends").Str("PersonaName")
}

// UserDirs devuelve los ID de cuenta que tienen carpeta en userdata.
func (in *Install) UserDirs() []uint32 {
	ents, _ := os.ReadDir(in.Path("userdata"))
	var out []uint32
	for _, e := range ents {
		if id, err := strconv.ParseUint(e.Name(), 10, 32); err == nil && e.IsDir() && id != 0 {
			out = append(out, uint32(id))
		}
	}
	return out
}

// ---------- appinfo con caché ----------

var appInfoCache struct {
	sync.Mutex
	path string
	mod  int64
	size int64
	data map[uint32]AppMeta
}

// AppInfo devuelve los datos de appcache\appinfo.vdf. Solo lo vuelve a leer
// si el archivo ha cambiado.
func (in *Install) AppInfo() map[uint32]AppMeta {
	p := in.Path("appcache", "appinfo.vdf")
	st, err := os.Stat(p)
	if err != nil {
		return map[uint32]AppMeta{}
	}
	c := &appInfoCache
	c.Lock()
	defer c.Unlock()
	if c.data != nil && c.path == p && c.mod == st.ModTime().UnixNano() && c.size == st.Size() {
		return c.data
	}
	m, err := ParseAppInfo(p)
	if m == nil {
		m = map[uint32]AppMeta{}
	}
	if err != nil && len(m) == 0 && c.data != nil {
		return c.data // a medio escribir: mejor lo anterior
	}
	c.path, c.mod, c.size, c.data = p, st.ModTime().UnixNano(), st.Size(), m
	return m
}

// ---------- imágenes ----------

// Nombres de archivo de cada imagen en la caché de la biblioteca, por si
// appinfo no dice la ruta exacta.
var assetFiles = map[string][]string{
	"cover":  {"library_600x900.jpg", "library_capsule.jpg", "library_600x900_2x.jpg"},
	"header": {"header.jpg", "library_header.jpg"},
	"hero":   {"library_hero.jpg"},
	"logo":   {"logo.png"},
}

var assetKeys = map[string][]string{
	"cover":  {"library_capsule"},
	"header": {"header", "library_header"},
	"hero":   {"library_hero"},
	"logo":   {"library_logo"},
}

// AssetRel devuelve la ruta relativa (como "hash/library_capsule.jpg") de una
// imagen según appinfo, o el nombre de siempre si appinfo no la tiene.
func AssetRel(meta AppMeta, kind string) []string {
	var out []string
	for _, k := range assetKeys[kind] {
		if v := meta.Assets[k]; v != "" && SafeRel(v) {
			out = append(out, v)
		}
	}
	return append(out, assetFiles[kind]...)
}

// folderName: installdir es el nombre de una sola carpeta dentro de
// steamapps\common (puede llevar apóstrofos, acentos…), nunca una ruta.
func folderName(d string) bool {
	d = strings.TrimSpace(d)
	return d != "" && d != "." && d != ".." && !strings.ContainsAny(d, `/\:`)
}

// SafeRel: una ruta relativa que viene de los archivos de Steam (appinfo.vdf, un
// appmanifest) y se une a una carpeta o a una URL. Solo letras, números y . _ - /,
// sin «..»: así no puede salirse de la carpeta ni cambiar la dirección.
func SafeRel(rel string) bool {
	if rel == "" || len(rel) > 260 || strings.HasPrefix(rel, "/") {
		return false
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	for _, r := range rel {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' || r == '/' || r == ' ') {
			return false
		}
	}
	return true
}

// LocalImage busca una imagen del juego en appcache\librarycache\<appid>.
// kind: cover (600x900), header (460x215), hero (fondo ancho), logo, icon.
func (in *Install) LocalImage(appid uint32, kind string, meta AppMeta) string {
	dir := in.Path("appcache", "librarycache", strconv.FormatUint(uint64(appid), 10))
	if kind == "icon" {
		if meta.Icon != "" && SafeRel(meta.Icon) && !strings.Contains(meta.Icon, "/") {
			p := filepath.Join(dir, meta.Icon+".jpg")
			if fileExists(p) {
				return p
			}
		}
		return ""
	}
	for _, rel := range AssetRel(meta, kind) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if fileExists(p) {
			return p
		}
	}
	// Steam también guarda algunas en subcarpetas con un hash como nombre
	names := assetFiles[kind]
	subs, _ := os.ReadDir(dir)
	for _, s := range subs {
		if !s.IsDir() {
			continue
		}
		for _, n := range names {
			p := filepath.Join(dir, s.Name(), n)
			if fileExists(p) {
				return p
			}
		}
	}
	return ""
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir() && st.Size() > 0
}
