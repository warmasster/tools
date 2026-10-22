package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type ShareFile struct {
	Path    string `json:"path"`    // ruta absoluta en este PC
	Rel     string `json:"rel"`     // ruta relativa que ve el receptor (con "/")
	Size    int64  `json:"size"`    // bytes
	ModTime int64  `json:"modTime"` // unix
}

type Share struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Kind      string      `json:"kind"` // "file" | "multi"
	Files     []ShareFile `json:"files"`
	Total     int64       `json:"total"`
	Created   int64       `json:"created"`
	Downloads int         `json:"downloads"`
	Sources   []string    `json:"sources"` // lo que se arrastró originalmente
	RoomID    string      `json:"roomId"`
}

type ShareStore struct {
	mu     sync.RWMutex
	path   string
	shares map[string]*Share
	order  []string
}

func newShareStore(dir string) *ShareStore {
	s := &ShareStore{path: filepath.Join(dir, "bundles.json"), shares: map[string]*Share{}}
	var list []*Share
	if loadJSON(s.path, &list) == nil {
		for _, sh := range list {
			if sh == nil || sh.ID == "" {
				continue
			}
			s.shares[sh.ID] = sh
			s.order = append(s.order, sh.ID)
		}
	}
	return s
}

func (s *ShareStore) saveLocked() {
	list := make([]*Share, 0, len(s.order))
	for _, id := range s.order {
		list = append(list, s.shares[id])
	}
	_ = saveJSON(s.path, list)
}

func (s *ShareStore) Get(id string) *Share {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.shares[id]
}

// List devuelve los envíos del más reciente al más antiguo.
func (s *ShareStore) List() []*Share {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Share, 0, len(s.order))
	for i := len(s.order) - 1; i >= 0; i-- {
		out = append(out, s.shares[s.order[i]])
	}
	return out
}

func (s *ShareStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.shares[id]; !ok {
		return
	}
	delete(s.shares, id)
	for i, v := range s.order {
		if v == id {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	s.saveLocked()
}

func (s *ShareStore) Rename(id, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sh, ok := s.shares[id]; ok {
		name = strings.TrimSpace(name)
		if name != "" {
			sh.Name = name
			s.saveLocked()
		}
	}
}

func (s *ShareStore) IncDownloads(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sh, ok := s.shares[id]; ok {
		sh.Downloads++
		s.saveLocked()
	}
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// DeleteRoom borra todos los envíos de un chat.
func (s *ShareStore) DeleteRoom(roomID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	keep := s.order[:0]
	for _, id := range s.order {
		if sh := s.shares[id]; sh != nil && sh.RoomID == roomID {
			delete(s.shares, id)
			continue
		}
		keep = append(keep, id)
	}
	s.order = keep
	s.saveLocked()
}

// Add registra un envío ya construido (importación desde la v1).
func (s *ShareStore) Add(sh *Share) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shares[sh.ID] = sh
	s.order = append(s.order, sh.ID)
	s.saveLocked()
}

// Create construye un envío a partir de archivos y/o carpetas.
func (s *ShareStore) Create(paths []string, roomID string) (*Share, error) {
	var files []ShareFile
	var sources []string
	used := map[string]int{}
	uniq := func(rel string) string {
		key := strings.ToLower(rel)
		n := used[key]
		used[key] = n + 1
		if n == 0 {
			return rel
		}
		ext := path.Ext(rel)
		return fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(rel, ext), n+1, ext)
	}

	dirs := 0
	for _, p := range paths {
		p = filepath.Clean(strings.TrimSpace(p))
		if p == "" || p == "." {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		st, err := os.Stat(abs)
		if err != nil {
			continue
		}
		sources = append(sources, abs)
		if !st.IsDir() {
			files = append(files, ShareFile{Path: abs, Rel: uniq(st.Name()), Size: st.Size(), ModTime: st.ModTime().Unix()})
			continue
		}
		dirs++
		root := filepath.Base(abs)
		if root == "" || root == string(filepath.Separator) || strings.HasSuffix(root, ":\\") {
			root = "carpeta"
		}
		root = uniq(root)
		_ = filepath.WalkDir(abs, func(fp string, d fs.DirEntry, err error) error {
			if err != nil {
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.Type()&fs.ModeSymlink != 0 || !d.Type().IsRegular() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			rel, err := filepath.Rel(abs, fp)
			if err != nil {
				return nil
			}
			files = append(files, ShareFile{
				Path:    fp,
				Rel:     root + "/" + filepath.ToSlash(rel),
				Size:    info.Size(),
				ModTime: info.ModTime().Unix(),
			})
			return nil
		})
	}
	if len(files) == 0 {
		return nil, errors.New("no hay archivos que enviar (¿carpeta vacía o sin permisos?)")
	}

	sh := &Share{ID: newID(), Created: time.Now().Unix(), Files: files, Sources: sources, RoomID: roomID}
	for _, f := range files {
		sh.Total += f.Size
	}
	switch {
	case len(files) == 1 && dirs == 0:
		sh.Kind = "file"
		sh.Name = files[0].Rel
	case len(sources) == 1 && dirs == 1:
		sh.Kind = "multi"
		sh.Name = filepath.Base(sources[0])
	default:
		sh.Kind = "multi"
		sh.Name = fmt.Sprintf("%d archivos", len(files))
	}
	if sh.Kind == "multi" {
		sort.SliceStable(sh.Files, func(i, j int) bool {
			return strings.ToLower(sh.Files[i].Rel) < strings.ToLower(sh.Files[j].Rel)
		})
	}

	s.mu.Lock()
	s.shares[sh.ID] = sh
	s.order = append(s.order, sh.ID)
	s.saveLocked()
	s.mu.Unlock()
	return sh, nil
}
