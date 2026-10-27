package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---------- modelo ----------

type Member struct {
	PID      string `json:"pid"`     // id público (lo ven los demás)
	MidHash  string `json:"midHash"` // hash del secreto que guarda su navegador
	Name     string `json:"name"`
	Color    int    `json:"color"`
	Joined   int64  `json:"joined"`
	LastSeen int64  `json:"lastSeen"`
}

type InFile struct {
	Rel   string `json:"rel"`
	Size  int64  `json:"size"`
	Final string `json:"final,omitempty"` // ruta definitiva en este PC
	Have  []byte `json:"have,omitempty"`  // bloques recibidos (bitmap)
	Done  bool   `json:"done,omitempty"`
}

func (f *InFile) chunks(chunk int64) int64 { return (f.Size + chunk - 1) / chunk }
func (f *InFile) has(i int64) bool         { return int(i/8) < len(f.Have) && f.Have[i/8]&(1<<(i%8)) != 0 }
func (f *InFile) mark(i int64) {
	for int(i/8) >= len(f.Have) {
		f.Have = append(f.Have, 0)
	}
	f.Have[i/8] |= 1 << (i % 8)
}

// InBundle es un paquete de archivos que un invitado ofrece al anfitrión.
type InBundle struct {
	ID       string    `json:"id"`
	From     string    `json:"from"` // pid
	Name     string    `json:"name"`
	Kind     string    `json:"kind"` // file | multi
	Total    int64     `json:"total"`
	Chunk    int64     `json:"chunk"`
	Files    []*InFile `json:"files"`
	Status   string    `json:"status"` // offered | accepted | transferring | done | rejected | cancelled | failed
	Received int64     `json:"received"`
	Root     string    `json:"root,omitempty"` // carpeta o archivo final a mostrar
	Error    string    `json:"error,omitempty"`
	Created  int64     `json:"created"`
	Finished int64     `json:"finished,omitempty"`
}

type Message struct {
	ID     string `json:"id"`
	Time   int64  `json:"time"` // ms
	From   string `json:"from"` // "host" | pid | "system"
	Kind   string `json:"kind"` // text | out | in | system
	Text   string `json:"text,omitempty"`
	Bundle string `json:"bundle,omitempty"`
}

type Room struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	NameSet  bool        `json:"nameSet"`
	Created  int64       `json:"created"`
	Updated  int64       `json:"updated"`
	Members  []*Member   `json:"members"`
	Messages []*Message  `json:"messages"`
	In       []*InBundle `json:"in"`
	Auto     string      `json:"auto"` // "" (por defecto) | on | off
	Unread   int         `json:"unread"`
}

func (r *Room) member(pid string) *Member {
	for _, m := range r.Members {
		if m.PID == pid {
			return m
		}
	}
	return nil
}

func (r *Room) inBundle(id string) *InBundle {
	for _, b := range r.In {
		if b.ID == id {
			return b
		}
	}
	return nil
}

func hashMid(mid string) string {
	h := sha256.Sum256([]byte("enviador:" + mid))
	return hex.EncodeToString(h[:16])
}

// ---------- almacén ----------

const maxMessages = 3000

type RoomStore struct {
	mu       sync.Mutex
	dir      string
	rooms    map[string]*Room
	dirty    map[string]bool
	onChange func(roomID string)
}

func newRoomStore(base string) *RoomStore {
	s := &RoomStore{dir: filepath.Join(base, "rooms"), rooms: map[string]*Room{}, dirty: map[string]bool{}}
	os.MkdirAll(s.dir, 0o755)
	entries, _ := os.ReadDir(s.dir)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var r Room
		if loadJSON(filepath.Join(s.dir, e.Name()), &r) == nil && r.ID != "" {
			// las transferencias a medias quedan pendientes de reanudar
			for _, b := range r.In {
				// "finishing": se cerró la app justo al guardar; se vuelve a intentar.
				if b.Status == "transferring" || b.Status == "finishing" {
					b.Status = "accepted"
				}
			}
			s.rooms[r.ID] = &r
		}
	}
	go s.flushLoop()
	return s
}

func (s *RoomStore) flushLoop() {
	for range time.Tick(time.Second) {
		s.Flush()
	}
}

func (s *RoomStore) Flush() {
	s.mu.Lock()
	type job struct {
		path string
		data []byte
	}
	var jobs []job
	for id := range s.dirty {
		if r, ok := s.rooms[id]; ok {
			if b, err := json.Marshal(r); err == nil {
				jobs = append(jobs, job{filepath.Join(s.dir, id+".json"), b})
			}
		}
		delete(s.dirty, id)
	}
	s.mu.Unlock()
	for _, j := range jobs {
		tmp := j.path + ".tmp"
		if os.WriteFile(tmp, j.data, 0o644) == nil {
			os.Rename(tmp, j.path)
		}
	}
}

// touch marca el chat como modificado (debe llamarse con s.mu bloqueado).
func (s *RoomStore) touch(r *Room) {
	r.Updated = time.Now().UnixMilli()
	s.dirty[r.ID] = true
}

func (s *RoomStore) changed(id string) {
	if s.onChange != nil {
		s.onChange(id)
	}
}

// With ejecuta f con el chat bloqueado. Si f devuelve true, se guarda y se notifica.
func (s *RoomStore) With(id string, f func(r *Room) bool) bool {
	s.mu.Lock()
	r, ok := s.rooms[id]
	mod := false
	if ok {
		mod = f(r)
		if mod {
			s.touch(r)
		}
	}
	s.mu.Unlock()
	if mod {
		s.changed(id)
	}
	return ok
}

func (s *RoomStore) Exists(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.rooms[id]
	return ok
}

func (s *RoomStore) Create(name string) *Room {
	now := time.Now().UnixMilli()
	r := &Room{ID: newID(), Name: name, Created: now, Updated: now}
	s.mu.Lock()
	s.rooms[r.ID] = r
	s.dirty[r.ID] = true
	s.mu.Unlock()
	s.changed(r.ID)
	return r
}

func (s *RoomStore) Delete(id string) {
	s.mu.Lock()
	delete(s.rooms, id)
	delete(s.dirty, id)
	s.mu.Unlock()
	os.Remove(filepath.Join(s.dir, id+".json"))
	s.changed(id)
}

func (s *RoomStore) IDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	type kv struct {
		id string
		t  int64
	}
	var l []kv
	for id, r := range s.rooms {
		l = append(l, kv{id, r.Updated})
	}
	sort.Slice(l, func(i, j int) bool { return l[i].t > l[j].t })
	out := make([]string, len(l))
	for i, x := range l {
		out[i] = x.id
	}
	return out
}

func newMessage(from, kind string) *Message {
	return &Message{ID: newID(), Time: time.Now().UnixMilli(), From: from, Kind: kind}
}

// Post añade un mensaje y devuelve una copia.
func (s *RoomStore) Post(roomID string, m *Message, unread bool) (Message, bool) {
	var cp Message
	ok := s.With(roomID, func(r *Room) bool {
		r.Messages = append(r.Messages, m)
		if len(r.Messages) > maxMessages {
			r.Messages = r.Messages[len(r.Messages)-maxMessages:]
		}
		if unread {
			r.Unread++
		}
		cp = *m
		return true
	})
	return cp, ok
}

var memberColors = 10

// Join registra (o reconoce) a un invitado a partir del secreto de su navegador.
func (s *RoomStore) Join(roomID, mid, name string) (*Member, bool, error) {
	name = cleanName(name)
	if len(mid) < 16 || len(mid) > 128 {
		return nil, false, errors.New("identificador no válido")
	}
	if name == "" {
		return nil, false, errors.New("pon un nombre")
	}
	h := hashMid(mid)
	var out Member
	isNew := false
	ok := s.With(roomID, func(r *Room) bool {
		for _, m := range r.Members {
			if m.MidHash == h {
				m.LastSeen = time.Now().UnixMilli()
				if m.Name != name {
					m.Name = name
				}
				out = *m
				return true
			}
		}
		if len(r.Members) >= 50 {
			return false
		}
		m := &Member{PID: newID()[:10], MidHash: h, Name: name, Color: len(r.Members) % memberColors, Joined: time.Now().UnixMilli(), LastSeen: time.Now().UnixMilli()}
		r.Members = append(r.Members, m)
		if !r.NameSet && len(r.Members) == 1 && (r.Name == "" || r.Name == "Chat nuevo") {
			r.Name = name
		}
		out = *m
		isNew = true
		return true
	})
	if !ok {
		return nil, false, errors.New("chat no encontrado o lleno")
	}
	if out.PID == "" {
		return nil, false, errors.New("el chat está lleno")
	}
	return &out, isNew, nil
}

// MemberByMid devuelve el invitado de ese secreto (o nil).
func (s *RoomStore) MemberByMid(roomID, mid string) *Member {
	if mid == "" {
		return nil
	}
	h := hashMid(mid)
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rooms[roomID]
	if !ok {
		return nil
	}
	for _, m := range r.Members {
		if m.MidHash == h {
			cp := *m
			return &cp
		}
	}
	return nil
}

func (s *RoomStore) MemberName(roomID, pid string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.rooms[roomID]; ok {
		if m := r.member(pid); m != nil {
			return m.Name
		}
	}
	return ""
}

func cleanName(n string) string {
	n = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 32 || r == 0x7f || invisibleMark(r) {
			return -1
		}
		return r
	}, n))
	if len([]rune(n)) > 32 {
		n = string([]rune(n)[:32])
	}
	return n
}
