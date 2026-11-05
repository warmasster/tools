package steam

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"turbogames/internal/vdf"
)

// WorkshopItem es un mod del Workshop tal como lo tiene Steam en este PC
// (steamapps\workshop\appworkshop_<appid>.acf de cada biblioteca).
type WorkshopItem struct {
	ID           uint64 `json:"id"`
	AppID        uint32 `json:"appid"`
	Installed    bool   `json:"installed"`
	Size         int64  `json:"size"`
	Updated      int64  `json:"updated"` // versión que hay en el disco
	Latest       int64  `json:"latest"`  // última versión que conoce Steam
	SubscribedBy uint32 `json:"subscribedBy"`
	Dir          string `json:"dir"`
	Library      string `json:"library"`
}

// WorkshopApp son los mods de un juego en este PC.
type WorkshopApp struct {
	AppID         uint32         `json:"appid"`
	Library       string         `json:"library"`
	Size          int64          `json:"size"`
	NeedsUpdate   bool           `json:"needsUpdate"`
	NeedsDownload bool           `json:"needsDownload"`
	Items         []WorkshopItem `json:"items"`
}

// Workshop lee los mods de todos los juegos en todas las bibliotecas.
func (in *Install) Workshop() map[uint32]*WorkshopApp {
	out := map[uint32]*WorkshopApp{}
	for _, lib := range in.Libraries() {
		files, _ := filepath.Glob(filepath.Join(lib.Path, "steamapps", "workshop", "appworkshop_*.acf"))
		for _, f := range files {
			w := parseWorkshopACF(f, lib.Path)
			if w == nil || out[w.AppID] != nil {
				continue
			}
			out[w.AppID] = w
		}
	}
	return out
}

func parseWorkshopACF(path, lib string) *WorkshopApp {
	root, err := vdf.ParseFile(path)
	if err != nil {
		return nil
	}
	s := root.Get("AppWorkshop")
	id, err := strconv.ParseUint(s.Str("appid"), 10, 32)
	if err != nil || id == 0 {
		return nil
	}
	w := &WorkshopApp{AppID: uint32(id), Library: lib, Items: []WorkshopItem{}}
	w.Size, _ = strconv.ParseInt(s.Str("SizeOnDisk"), 10, 64)
	w.NeedsUpdate = s.Str("NeedsUpdate") == "1"
	w.NeedsDownload = s.Str("NeedsDownload") == "1"
	content := filepath.Join(lib, "steamapps", "workshop", "content", strconv.FormatUint(id, 10))
	items := map[uint64]*WorkshopItem{}
	get := func(key string) *WorkshopItem {
		n, err := strconv.ParseUint(key, 10, 64)
		if err != nil || n == 0 {
			return nil
		}
		it := items[n]
		if it == nil {
			it = &WorkshopItem{ID: n, AppID: uint32(id), Library: lib}
			items[n] = it
		}
		return it
	}
	for _, c := range s.Get("WorkshopItemsInstalled").Kids() {
		it := get(c.Key)
		if it == nil || !c.IsObject() {
			continue
		}
		it.Installed = true
		it.Size, _ = strconv.ParseInt(c.Str("size"), 10, 64)
		it.Updated, _ = strconv.ParseInt(c.Str("timeupdated"), 10, 64)
		it.Dir = filepath.Join(content, c.Key)
	}
	for _, c := range s.Get("WorkshopItemDetails").Kids() {
		it := get(c.Key)
		if it == nil || !c.IsObject() {
			continue
		}
		if by, err := strconv.ParseUint(c.Str("subscribedby"), 10, 32); err == nil {
			it.SubscribedBy = uint32(by)
		}
		it.Latest, _ = strconv.ParseInt(c.Str("latest_timeupdated"), 10, 64)
		if it.Updated == 0 {
			it.Updated, _ = strconv.ParseInt(c.Str("timeupdated"), 10, 64)
		}
	}
	for _, it := range items {
		if it.Installed {
			if st, err := os.Stat(it.Dir); err != nil || !st.IsDir() {
				it.Installed, it.Dir = false, ""
			}
		}
		w.Items = append(w.Items, *it)
	}
	return w
}

// UserCollections lee las colecciones de la biblioteca de Steam de una cuenta
// (Favoritos, Ocultos y las que haya creado), que Steam guarda en
// userdata\<id>\config\cloudstorage\cloud-storage-namespace-1.json.
// Devuelve nombre → juegos. Las colecciones dinámicas (por filtros) no tienen
// lista de juegos y no salen.
func (in *Install) UserCollections(accountID uint32) map[string][]uint32 {
	out := map[string][]uint32{}
	p := in.Path("userdata", strconv.FormatUint(uint64(accountID), 10), "config", "cloudstorage", "cloud-storage-namespace-1.json")
	b, err := os.ReadFile(p)
	if err != nil || len(b) > 64<<20 {
		return out
	}
	var list [][]json.RawMessage
	if json.Unmarshal(b, &list) != nil {
		return out
	}
	for _, kv := range list {
		if len(kv) != 2 {
			continue
		}
		var key string
		if json.Unmarshal(kv[0], &key) != nil || !strings.HasPrefix(key, "user-collections.") {
			continue
		}
		var e struct {
			Value   string `json:"value"`
			Deleted bool   `json:"is_deleted"`
		}
		if json.Unmarshal(kv[1], &e) != nil || e.Deleted || e.Value == "" {
			continue
		}
		var c struct {
			ID      string   `json:"id"`
			Name    string   `json:"name"`
			Added   []uint32 `json:"added"`
			Removed []uint32 `json:"removed"`
		}
		if json.Unmarshal([]byte(e.Value), &c) != nil || len(c.Added) == 0 {
			continue
		}
		name := strings.TrimSpace(c.Name)
		switch c.ID {
		case "favorite":
			name = "Favoritos"
		case "hidden":
			name = "Ocultos en Steam"
		}
		if name == "" {
			continue
		}
		removed := map[uint32]bool{}
		for _, r := range c.Removed {
			removed[r] = true
		}
		for _, a := range c.Added {
			if !removed[a] {
				out[name] = append(out[name], a)
			}
		}
	}
	return out
}
