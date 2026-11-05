package main

import (
	"os"
	"strconv"
	"sync"
	"time"
)

// GameMeta es lo que TurboGAMES guarda de cada juego (en juegos.json): por ahora,
// sus etiquetas.
type GameMeta struct {
	Tags []string `json:"tags,omitempty"`
}

type gameMetaStore struct {
	mu    sync.Mutex
	path  string
	items map[string]*GameMeta // clave: appid en texto
}

func loadGameMetaStore(path string) *gameMetaStore {
	s := &gameMetaStore{path: path, items: map[string]*GameMeta{}}
	if err := loadJSON(path, &s.items); err != nil && !os.IsNotExist(err) {
		os.Rename(path, path+".dañado-"+time.Now().Format("20060102-150405"))
	}
	if s.items == nil {
		s.items = map[string]*GameMeta{}
	}
	return s
}

func (s *gameMetaStore) tags(appid uint32) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m := s.items[strconv.FormatUint(uint64(appid), 10)]; m != nil {
		return append([]string{}, m.Tags...)
	}
	return []string{}
}

// SetGameTags guarda las etiquetas de un juego.
func (a *App) SetGameTags(appid uint32, tags []string) ([]string, error) {
	tags = cleanTags(tags)
	s := a.gmeta
	s.mu.Lock()
	defer s.mu.Unlock()
	k := strconv.FormatUint(uint64(appid), 10)
	if len(tags) == 0 {
		delete(s.items, k)
	} else {
		s.items[k] = &GameMeta{Tags: tags}
	}
	return tags, saveJSON(s.path, s.items)
}
