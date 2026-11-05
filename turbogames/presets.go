package main

import (
	"bytes"
	"compress/flate"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// PresetMod es un mod de un preset (con su nombre, por si luego ya no existe).
type PresetMod struct {
	ID    uint64 `json:"id"`
	Title string `json:"title"`
}

// Preset es una lista de mods de un juego que se puede activar de golpe y
// compartir por texto.
type Preset struct {
	ID      string      `json:"id"`
	AppID   uint32      `json:"appid"`
	Game    string      `json:"game"`
	Name    string      `json:"name"`
	Author  string      `json:"author"` // quien lo hizo
	From    string      `json:"from"`   // quien te lo pasó ("" = tuyo)
	Tags    []string    `json:"tags"`
	Notes   string      `json:"notes"`
	Mods    []PresetMod `json:"mods"`
	Created int64       `json:"created"`
	Updated int64       `json:"updated"`
	Auto    bool        `json:"auto"`   // copia automática antes de quitar mods
	Source  string      `json:"source"` // propio, pegado, archivo, arma3, coleccion, copia
}

const (
	maxPresetMods = 5000
	maxAutoCopies = 5 // copias automáticas que se guardan por juego
)

type presetStore struct {
	mu    sync.Mutex
	path  string
	items []*Preset
}

func loadPresetStore(path string) *presetStore {
	s := &presetStore{path: path}
	if err := loadJSON(path, &s.items); err != nil && !os.IsNotExist(err) {
		// dañado: se aparta en vez de pisarlo
		os.Rename(path, path+".dañado-"+time.Now().Format("20060102-150405"))
		s.items = nil
	}
	return s
}

func (s *presetStore) saveLocked() error {
	if s.items == nil {
		s.items = []*Preset{}
	}
	return saveJSON(s.path, s.items)
}

func copyPreset(p *Preset) Preset {
	c := *p
	c.Tags = append([]string{}, p.Tags...)
	c.Mods = append([]PresetMod{}, p.Mods...)
	return c
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Presets devuelve todos los presets guardados (los más nuevos primero).
func (a *App) Presets() []Preset {
	s := a.presets
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Preset, 0, len(s.items))
	for _, p := range s.items {
		out = append(out, copyPreset(p))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Updated > out[j].Updated })
	return out
}

func cleanPreset(p *Preset) error {
	p.Name = clip(strings.TrimSpace(p.Name), 80)
	p.Author = clip(strings.TrimSpace(p.Author), 64)
	p.From = clip(strings.TrimSpace(p.From), 64)
	p.Notes = clip(strings.TrimSpace(p.Notes), 4000)
	p.Game = clip(strings.TrimSpace(p.Game), 120)
	p.Tags = cleanTags(p.Tags)
	if p.Tags == nil {
		p.Tags = []string{}
	}
	if p.Name == "" {
		return errors.New("Ponle un nombre al preset")
	}
	if p.AppID == 0 {
		return errors.New("Falta el juego del preset")
	}
	seen := map[uint64]bool{}
	mods := make([]PresetMod, 0, len(p.Mods))
	for _, m := range p.Mods {
		if m.ID == 0 || seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		m.Title = clip(strings.TrimSpace(m.Title), 160)
		mods = append(mods, m)
	}
	if len(mods) == 0 {
		return errors.New("El preset no tiene ningún mod")
	}
	if len(mods) > maxPresetMods {
		return fmt.Errorf("Como mucho %d mods por preset", maxPresetMods)
	}
	p.Mods = mods
	return nil
}

// SavePreset guarda un preset (si no tiene ID, es nuevo).
func (a *App) SavePreset(p Preset) (Preset, error) {
	if err := cleanPreset(&p); err != nil {
		return p, err
	}
	if p.Game == "" {
		p.Game = a.GameName(p.AppID)
	}
	s := a.presets
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Unix()
	p.Updated = now
	if p.ID == "" {
		p.ID, p.Created = newID(), now
		if p.Source == "" {
			p.Source = "propio"
		}
		s.items = append(s.items, &p)
	} else {
		found := false
		for i, old := range s.items {
			if old.ID == p.ID {
				p.Created, p.Auto = old.Created, old.Auto && p.Auto
				s.items[i] = &p
				found = true
				break
			}
		}
		if !found {
			p.Created = now
			s.items = append(s.items, &p)
		}
	}
	return copyPreset(&p), s.saveLocked()
}

// DeletePreset borra un preset.
func (a *App) DeletePreset(id string) error {
	s := a.presets
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.items {
		if p.ID == id {
			s.items = append(s.items[:i], s.items[i+1:]...)
			return s.saveLocked()
		}
	}
	return errors.New("Ese preset ya no existe")
}

// autoBackup guarda lo que hay suscrito ahora como copia automática (antes de
// quitar mods), y deja solo las últimas de cada juego.
func (a *App) autoBackup(appid uint32, ids []uint64) (Preset, error) {
	mods := make([]PresetMod, 0, len(ids))
	for _, id := range ids {
		mods = append(mods, PresetMod{ID: id, Title: a.modFrom(id).Title})
	}
	sort.Slice(mods, func(i, j int) bool { return strings.ToLower(mods[i].Title) < strings.ToLower(mods[j].Title) })
	p := Preset{AppID: appid, Game: a.GameName(appid), Name: "Copia automática · " + time.Now().Format("02/01/2006 15:04"),
		Notes: "Lo que tenías suscrito antes de quitar mods. TurboGAMES guarda las " + strconv.Itoa(maxAutoCopies) + " últimas de cada juego.",
		Mods:  mods, Auto: true, Source: "copia", Tags: []string{}}
	saved, err := a.SavePreset(p)
	if err != nil {
		return saved, err
	}
	s := a.presets
	s.mu.Lock()
	defer s.mu.Unlock()
	var autos []*Preset
	for _, q := range s.items {
		if q.Auto && q.AppID == appid {
			autos = append(autos, q)
		}
	}
	if len(autos) > maxAutoCopies {
		sort.Slice(autos, func(i, j int) bool { return autos[i].Created > autos[j].Created })
		drop := map[string]bool{}
		for _, q := range autos[maxAutoCopies:] {
			drop[q.ID] = true
		}
		kept := s.items[:0]
		for _, q := range s.items {
			if !drop[q.ID] {
				kept = append(kept, q)
			}
		}
		s.items = kept
		_ = s.saveLocked()
	}
	return saved, nil
}

// ---------- compartir por texto ----------

// codeData es lo que va dentro del código TGP1 (comprimido): sin los nombres de
// los mods para que quepa en un mensaje; se vuelven a pedir al pegarlo.
type codeData struct {
	V     int      `json:"v"`
	App   uint32   `json:"a"`
	Game  string   `json:"g,omitempty"`
	Name  string   `json:"n"`
	By    string   `json:"b,omitempty"`
	Tags  []string `json:"t,omitempty"`
	Notes string   `json:"o,omitempty"`
	IDs   []uint64 `json:"m"`
}

const codePrefix = "TGP1:"

func encodeCode(p Preset) string {
	d := codeData{V: 1, App: p.AppID, Game: p.Game, Name: p.Name, By: p.Author, Tags: p.Tags, Notes: clip(p.Notes, 500)}
	for _, m := range p.Mods {
		d.IDs = append(d.IDs, m.ID)
	}
	b, _ := json.Marshal(d)
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, flate.BestCompression)
	w.Write(b)
	w.Close()
	return codePrefix + base64.RawURLEncoding.EncodeToString(buf.Bytes())
}

var codeRe = regexp.MustCompile(`TGP1:([A-Za-z0-9_-]{8,})`)

func decodeCode(text string) (codeData, bool) {
	var d codeData
	// el código puede llegar partido en varias líneas por el chat
	m := codeRe.FindStringSubmatch(strings.Join(strings.Fields(text), ""))
	if m == nil {
		return d, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(m[1])
	if err != nil {
		return d, false
	}
	b, err := io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(raw)), 4<<20))
	if err != nil || json.Unmarshal(b, &d) != nil || d.V != 1 || len(d.IDs) == 0 {
		return d, false
	}
	return d, true
}

func modURL(id uint64) string {
	return "https://steamcommunity.com/sharedfiles/filedetails/?id=" + strconv.FormatUint(id, 10)
}

// PresetText es el texto para mandar un preset por un chat: la lista que se lee
// bien y, al final, el código para pegarlo en TurboGAMES. Con full=false, solo
// una línea y el código (para chats con poco espacio).
func (a *App) PresetText(id string, full bool) (string, error) {
	p, ok := a.presetByID(id)
	if !ok {
		return "", errors.New("Ese preset ya no existe")
	}
	if p.Game == "" {
		p.Game = a.GameName(p.AppID)
	}
	var b strings.Builder
	head := fmt.Sprintf("Preset de TurboGAMES: «%s»", p.Name)
	if p.Game != "" {
		head += " · " + p.Game
	}
	head += fmt.Sprintf(" · %d mods", len(p.Mods))
	b.WriteString(head + "\n")
	if full {
		if p.Author != "" {
			b.WriteString("De: " + p.Author + "\n")
		}
		if len(p.Tags) > 0 {
			b.WriteString("Etiquetas: #" + strings.Join(p.Tags, " #") + "\n")
		}
		if p.Notes != "" {
			b.WriteString("Notas: " + p.Notes + "\n")
		}
		b.WriteString("\n")
		for i, m := range p.Mods {
			t := m.Title
			if t == "" {
				t = "Mod " + strconv.FormatUint(m.ID, 10)
			}
			fmt.Fprintf(&b, "%d. %s — %s\n", i+1, t, modURL(m.ID))
		}
		b.WriteString("\nPara cargarlo en TurboGAMES: copia todo este texto y pulsa «Pegar preset» en la pestaña Mods.\n")
	}
	b.WriteString(encodeCode(p))
	return b.String(), nil
}

func (a *App) presetByID(id string) (Preset, bool) {
	s := a.presets
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.items {
		if p.ID == id {
			return copyPreset(p), true
		}
	}
	return Preset{}, false
}

// ---------- leer un preset pegado o de un archivo ----------

// PresetDraft es un preset leído de un texto, sin guardar todavía.
type PresetDraft struct {
	Preset   Preset   `json:"preset"`
	Missing  int      `json:"missing"`  // mods que ya no existen o son privados
	Others   int      `json:"others"`   // mods de otros juegos que se han dejado fuera
	Local    int      `json:"local"`    // mods locales (no del Workshop) que no se pueden incluir
	Warnings []string `json:"warnings"` // avisos para enseñar
	Size     int64    `json:"size"`
}

var (
	reModLink  = regexp.MustCompile(`(?i)(?:filedetails/?\?(?:[^\s"'<>]*?&)?id=|CommunityFilePage/)(\d{3,20})`)
	reBareID   = regexp.MustCompile(`^\s*(?:\d+[.)]\s*)?(\d{6,20})\s*$`)
	reArmaMeta = regexp.MustCompile(`(?i)<meta\s+name="arma:(\w+)"\s+content="([^"]*)"`)
	reArmaRow  = regexp.MustCompile(`(?is)<tr[^>]*data-type="ModContainer"[^>]*>(.*?)</tr>`)
	reArmaName = regexp.MustCompile(`(?is)data-type="DisplayName"[^>]*>(.*?)</td>`)
	reHeadName = regexp.MustCompile(`«([^»]{1,80})»`)
	reHeadFrom = regexp.MustCompile(`(?m)^De:\s*(.{1,64})$`)
)

// ParsePreset lee un preset de un texto: un código TGP1 de TurboGAMES, un
// preset exportado del lanzador de Arma 3 (.html) o cualquier texto con enlaces
// o números de mods del Workshop (también colecciones, que se abren).
func (a *App) ParsePreset(text string) (PresetDraft, error) {
	return a.parsePreset(text, "pegado")
}

func (a *App) parsePreset(text, source string) (PresetDraft, error) {
	var d PresetDraft
	d.Warnings = []string{}
	if len(text) > 8<<20 {
		return d, errors.New("Ese texto es demasiado largo")
	}
	p := Preset{Tags: []string{}, Source: source}
	var ids []uint64
	titles := map[uint64]string{}
	if c, ok := decodeCode(text); ok {
		p.AppID, p.Game, p.Name, p.Author, p.Tags, p.Notes = c.App, c.Game, c.Name, c.By, nonNil(c.Tags), c.Notes
		ids = c.IDs
	} else if strings.Contains(text, `arma:Type`) || strings.Contains(text, `data-type="ModContainer"`) {
		// preset exportado del lanzador de Arma 3
		p.AppID, p.Source = 107410, "arma3"
		for _, m := range reArmaMeta.FindAllStringSubmatch(text, -1) {
			if strings.EqualFold(m[1], "PresetName") {
				p.Name = html.UnescapeString(m[2])
			}
		}
		for _, row := range reArmaRow.FindAllStringSubmatch(text, -1) {
			lm := reModLink.FindStringSubmatch(row[1])
			if lm == nil {
				d.Local++
				continue
			}
			id, err := strconv.ParseUint(lm[1], 10, 64)
			if err != nil {
				continue
			}
			ids = append(ids, id)
			if nm := reArmaName.FindStringSubmatch(row[1]); nm != nil {
				titles[id] = htmlText(nm[1])
			}
		}
		if d.Local > 0 {
			d.Warnings = append(d.Warnings, fmt.Sprintf("%d mods locales del lanzador no son del Workshop y no se pueden incluir.", d.Local))
		}
	} else {
		for _, m := range reModLink.FindAllStringSubmatch(text, -1) {
			if id, err := strconv.ParseUint(m[1], 10, 64); err == nil {
				ids = append(ids, id)
			}
		}
		for _, line := range strings.Split(text, "\n") {
			if m := reBareID.FindStringSubmatch(line); m != nil {
				if id, err := strconv.ParseUint(m[1], 10, 64); err == nil {
					ids = append(ids, id)
				}
			}
		}
		if m := reHeadName.FindStringSubmatch(text); m != nil {
			p.Name = strings.TrimSpace(m[1])
		}
		if m := reHeadFrom.FindStringSubmatch(text); m != nil {
			p.Author = strings.TrimSpace(m[1])
		}
	}
	ids = dedupe(ids)
	if len(ids) == 0 {
		return d, errors.New("No he encontrado ningún mod en ese texto. Pega un preset de TurboGAMES, enlaces del Workshop o sus números.")
	}
	if len(ids) > maxPresetMods {
		return d, fmt.Errorf("Son demasiados mods (más de %d)", maxPresetMods)
	}
	// las colecciones del Workshop se cambian por los mods que llevan
	if cols, err := webCollectionsFn(ids); err == nil && len(cols) > 0 {
		var out []uint64
		for _, id := range ids {
			if kids, ok := cols[id]; ok {
				out = append(out, kids...)
				if p.Name == "" {
					if items, err := webDetailsFn([]uint64{id}); err == nil && len(items) == 1 {
						p.Name = items[0].Title
					}
				}
				if p.Source == "pegado" {
					p.Source = "coleccion"
				}
				continue
			}
			out = append(out, id)
		}
		ids = dedupe(out)
	}
	mods := a.ModDetails(p.AppID, ids)
	// el juego: el que diga el código o el de la mayoría de los mods
	if p.AppID == 0 {
		count := map[uint32]int{}
		for _, m := range mods {
			if m.AppID != 0 {
				count[m.AppID]++
			}
		}
		best := 0
		for app, n := range count {
			if n > best {
				p.AppID, best = app, n
			}
		}
	}
	for _, m := range mods {
		if m.Known && !m.Missing && m.AppID != 0 && p.AppID != 0 && m.AppID != p.AppID {
			d.Others++
			continue
		}
		t := m.Title
		if t == "" {
			t = titles[m.ID]
		}
		if m.Missing || !m.Known {
			d.Missing++
		}
		d.Size += m.Size
		p.Mods = append(p.Mods, PresetMod{ID: m.ID, Title: t})
	}
	if p.AppID == 0 {
		return d, errors.New("No he podido saber de qué juego son esos mods (¿hay conexión a internet?)")
	}
	if n := a.GameName(p.AppID); n != "" {
		p.Game = n
	}
	if p.Name == "" {
		p.Name = "Preset de " + firstNonEmpty(p.Game, "mods")
	}
	if d.Others > 0 {
		d.Warnings = append(d.Warnings, fmt.Sprintf("%d mods son de otro juego y se han dejado fuera.", d.Others))
	}
	if d.Missing > 0 {
		d.Warnings = append(d.Warnings, fmt.Sprintf("%d mods ya no existen, son privados o no se han podido comprobar.", d.Missing))
	}
	if p.Mods == nil {
		p.Mods = []PresetMod{}
	}
	d.Preset = p
	return d, nil
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// ImportPresetFile deja elegir un archivo (un .txt con un preset, o un .html
// exportado del lanzador de Arma 3) y lo lee.
func (a *App) ImportPresetFile() (PresetDraft, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Elige un preset",
		Filters: []runtime.FileFilter{{DisplayName: "Presets (*.txt, *.html)", Pattern: "*.txt;*.html;*.htm"}, {DisplayName: "Todos los archivos", Pattern: "*.*"}},
	})
	if err != nil || path == "" {
		return PresetDraft{}, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return PresetDraft{}, err
	}
	if st.Size() > 8<<20 {
		return PresetDraft{}, errors.New("Ese archivo es demasiado grande para ser un preset")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return PresetDraft{}, err
	}
	return a.parsePreset(string(b), "archivo")
}

// ExportPresetFile guarda un preset como archivo de texto (el mismo texto que se
// copia para un chat).
func (a *App) ExportPresetFile(id string) (bool, error) {
	p, ok := a.presetByID(id)
	if !ok {
		return false, errors.New("Ese preset ya no existe")
	}
	text, err := a.PresetText(id, true)
	if err != nil {
		return false, err
	}
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return '_'
		}
		return r
	}, p.Name)
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Guardar el preset",
		DefaultFilename: name + ".txt",
		Filters:         []runtime.FileFilter{{DisplayName: "Texto (*.txt)", Pattern: "*.txt"}},
	})
	if err != nil || path == "" {
		return false, err
	}
	if !strings.HasSuffix(strings.ToLower(path), ".txt") {
		path += ".txt"
	}
	return true, os.WriteFile(path, []byte(text), 0o644)
}
