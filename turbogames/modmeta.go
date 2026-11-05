package main

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"turbogames/internal/steamworks"
)

// histEntry es lo que se sabe de un mod al que una cuenta estuvo suscrita.
type histEntry struct {
	Title   string `json:"title,omitempty"`
	First   int64  `json:"first"`             // la primera vez que se le vio suscrito
	Last    int64  `json:"last"`              // la última
	Removed int64  `json:"removed,omitempty"` // cuándo dejó de estarlo (0 = sigue)
}

// modStore guarda en mods.json las etiquetas que pones a los mods y el
// historial de suscripciones de cada cuenta (para ver los que tuviste antes).
type modStore struct {
	mu   sync.Mutex
	path string
	Tags map[string][]string `json:"tags"` // id del mod → etiquetas
	// cuenta (SteamID64) → juego → mod → historial
	History map[string]map[string]map[string]*histEntry `json:"history"`
}

func loadModStore(path string) *modStore {
	s := &modStore{path: path}
	if err := loadJSON(path, s); err != nil && !os.IsNotExist(err) {
		os.Rename(path, path+".dañado-"+time.Now().Format("20060102-150405"))
	}
	if s.Tags == nil {
		s.Tags = map[string][]string{}
	}
	if s.History == nil {
		s.History = map[string]map[string]map[string]*histEntry{}
	}
	return s
}

func (s *modStore) saveLocked() error { return saveJSON(s.path, s) }

func (s *modStore) tags(id uint64) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.Tags[strconv.FormatUint(id, 10)]...)
}

// record apunta a qué mods está suscrita ahora la cuenta en ese juego, y marca
// como quitados los que estaban y ya no.
// title no puede usar mstore (se llama con el cerrojo cogido).
func (s *modStore) record(account string, appid uint32, ids []uint64, title func(uint64) string) {
	if account == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Unix()
	acc := s.History[account]
	if acc == nil {
		acc = map[string]map[string]*histEntry{}
		s.History[account] = acc
	}
	app := strconv.FormatUint(uint64(appid), 10)
	h := acc[app]
	if h == nil {
		h = map[string]*histEntry{}
		acc[app] = h
	}
	cur := map[string]bool{}
	for _, id := range ids {
		k := strconv.FormatUint(id, 10)
		cur[k] = true
		e := h[k]
		if e == nil {
			e = &histEntry{First: now}
			h[k] = e
		}
		if t := title(id); t != "" {
			e.Title = clip(t, 160)
		}
		e.Last, e.Removed = now, 0
	}
	for k, e := range h {
		if !cur[k] && e.Removed == 0 {
			e.Removed = now
		}
	}
	_ = s.saveLocked()
}

// history devuelve los mods de ese juego a los que la cuenta estuvo suscrita y
// ya no (los más recientes primero), y cuándo se suscribió a cada uno de los
// que tiene.
func (s *modStore) history(account string, appid uint32) (removed []histItem, since map[uint64]int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	since = map[uint64]int64{}
	h := s.History[account][strconv.FormatUint(uint64(appid), 10)]
	for k, e := range h {
		id, err := strconv.ParseUint(k, 10, 64)
		if err != nil {
			continue
		}
		if e.Removed > 0 {
			removed = append(removed, histItem{ID: id, Title: e.Title, Removed: e.Removed, First: e.First})
		} else {
			since[id] = e.First
		}
	}
	sort.Slice(removed, func(i, j int) bool { return removed[i].Removed > removed[j].Removed })
	if len(removed) > 1000 {
		removed = removed[:1000]
	}
	return removed, since
}

type histItem struct {
	ID      uint64
	Title   string
	Removed int64
	First   int64
}

// SetModTags añade y quita etiquetas tuyas a unos mods (para ordenarlos a tu
// manera: «imprescindible», «gráficos», «probar»…). Devuelve cómo quedan.
func (a *App) SetModTags(ids []uint64, add, remove []string) (map[string][]string, error) {
	add, remove = cleanTags(add), cleanTags(remove)
	drop := map[string]bool{}
	for _, t := range remove {
		drop[normTag(t)] = true
	}
	s := a.mstore
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string][]string{}
	for _, id := range dedupe(ids) {
		k := strconv.FormatUint(id, 10)
		var tags []string
		for _, t := range s.Tags[k] {
			if !drop[normTag(t)] {
				tags = append(tags, t)
			}
		}
		tags = cleanTags(append(tags, add...))
		if len(tags) == 0 {
			delete(s.Tags, k)
		} else {
			s.Tags[k] = tags
		}
		out[k] = tags
	}
	return out, s.saveLocked()
}

func normTag(t string) string { return strings.ToLower(t) }

// galleryFromItem devuelve las imágenes y vídeos que Steam ha dado de un mod.
func galleryFromItem(it steamworks.Item) []steamworks.Preview {
	var out []steamworks.Preview
	for _, p := range it.Previews {
		if p.Video != "" && validVideoID(p.Video) {
			out = append(out, steamworks.Preview{Video: p.Video})
		} else if p.URL != "" && safeImageURL(p.URL) {
			out = append(out, steamworks.Preview{URL: p.URL})
		}
	}
	return out
}
