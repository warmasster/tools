package main

import (
	"errors"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"turbogames/internal/steam"
)

// Meta es lo que TurboGAMES guarda de cada cuenta (en cuentas.json). Nada de
// esto es secreto: las contraseñas van en la bóveda.
type Meta struct {
	Name      string   `json:"name"` // nombre de usuario de Steam (con el que se entra)
	SteamID   string   `json:"steamId,omitempty"`
	Persona   string   `json:"persona,omitempty"` // último nombre visible conocido
	Alias     string   `json:"alias,omitempty"`
	Notes     string   `json:"notes,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Pinned    bool     `json:"pinned,omitempty"`
	Hidden    bool     `json:"hidden,omitempty"`
	HasSecret bool     `json:"hasSecret,omitempty"` // tiene algo en la bóveda
	LastUsed  int64    `json:"lastUsed,omitempty"`
	Added     int64    `json:"added,omitempty"`
}

type metaStore struct {
	mu    sync.Mutex
	path  string
	items map[string]*Meta // clave: nombre en minúsculas
}

func keyOf(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

func loadMetaStore(path string) *metaStore {
	s := &metaStore{path: path, items: map[string]*Meta{}}
	var list []*Meta
	if err := loadJSON(path, &list); err == nil {
		for _, m := range list {
			if m != nil && keyOf(m.Name) != "" {
				s.items[keyOf(m.Name)] = m
			}
		}
	}
	return s
}

// saveLocked guarda el archivo; hay que tener el cerrojo.
func (s *metaStore) saveLocked() error {
	list := make([]*Meta, 0, len(s.items))
	for _, m := range s.items {
		list = append(list, m)
	}
	sort.Slice(list, func(i, j int) bool { return keyOf(list[i].Name) < keyOf(list[j].Name) })
	return saveJSON(s.path, list)
}

// update cambia (o crea) la ficha de una cuenta y guarda.
func (s *metaStore) update(name string, fn func(m *Meta)) error {
	k := keyOf(name)
	if k == "" {
		return errors.New("falta el nombre de la cuenta")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.items[k]
	if m == nil {
		m = &Meta{Name: strings.TrimSpace(name), Added: time.Now().Unix()}
		s.items[k] = m
	}
	fn(m)
	return s.saveLocked()
}

func (s *metaStore) remove(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, keyOf(name))
	return s.saveLocked()
}

func (s *metaStore) get(name string) (Meta, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.items[keyOf(name)]
	if m == nil {
		return Meta{}, false
	}
	c := *m
	c.Tags = append([]string(nil), m.Tags...)
	return c, true
}

func (s *metaStore) all() map[string]Meta {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]Meta, len(s.items))
	for k, m := range s.items {
		c := *m
		c.Tags = append([]string(nil), m.Tags...)
		out[k] = c
	}
	return out
}

// ---------- lista que ve la interfaz ----------

// Account es una cuenta tal como la enseña la interfaz: lo que sabe Steam más
// lo que guarda TurboGAMES.
type Account struct {
	Key       string   `json:"key"`
	Name      string   `json:"name"`
	SteamID   string   `json:"steamId"`
	Persona   string   `json:"persona"`
	Alias     string   `json:"alias"`
	Notes     string   `json:"notes"`
	Tags      []string `json:"tags"`
	Pinned    bool     `json:"pinned"`
	Hidden    bool     `json:"hidden"`
	InSteam   bool     `json:"inSteam"`   // Steam la recuerda (loginusers.vdf)
	HasToken  bool     `json:"hasToken"`  // sesión guardada: entra sin contraseña
	HasSecret bool     `json:"hasSecret"` // tiene datos en la bóveda
	Active    bool     `json:"active"`    // es la que está abierta en Steam ahora
	AutoLogin bool     `json:"autoLogin"` // es con la que entrará Steam al abrirse
	Offline   bool     `json:"offline"`   // la última vez entró sin conexión
	LastUsed  int64    `json:"lastUsed"`
	Added     int64    `json:"added"`
	Avatar    string   `json:"avatar"` // URL de la foto, o vacío
	Games     int      `json:"games"`
	Minutes   int64    `json:"minutes"`
	Recent    *Recent  `json:"recent,omitempty"`
}

// Recent es el último juego al que jugó una cuenta.
type Recent struct {
	AppID      uint32 `json:"appid"`
	Name       string `json:"name"`
	LastPlayed int64  `json:"lastPlayed"`
}

// buildAccounts junta loginusers.vdf, local.vdf, userdata y cuentas.json.
func (a *App) buildAccounts() ([]Account, error) {
	in := a.steam()
	metas := a.metas.all()
	var users []steam.LoginUser
	var st steam.Status
	var err error
	if in != nil {
		users, err = in.LoginUsers()
		st = in.ReadStatus()
	}
	var meta map[uint32]steam.AppMeta
	if in != nil {
		meta = in.AppInfo()
	}

	out := make([]Account, 0, len(users)+len(metas))
	seen := map[string]bool{}
	for _, u := range users {
		k := keyOf(u.AccountName)
		if seen[k] {
			continue
		}
		seen[k] = true
		m := metas[k]
		acc := Account{
			Key: k, Name: u.AccountName, SteamID: u.SteamID, Persona: u.PersonaName,
			InSteam: true, HasToken: u.HasToken, Offline: u.WantsOffline,
			AutoLogin: strings.EqualFold(st.AutoLogin, u.AccountName),
			LastUsed:  max(u.Timestamp, m.LastUsed),
		}
		acc.Active = st.Running && st.ActiveUser != 0 && steam.AccountID(u.SteamID) == st.ActiveUser
		applyMeta(&acc, m)
		out = append(out, acc)
		// recordar el nombre visible y el SteamID para cuando Steam la olvide
		if m.Name == "" || m.SteamID != u.SteamID || (u.PersonaName != "" && m.Persona != u.PersonaName) {
			_ = a.metas.update(u.AccountName, func(mm *Meta) {
				mm.SteamID = u.SteamID
				if u.PersonaName != "" {
					mm.Persona = u.PersonaName
				}
			})
		}
	}
	// cuentas que solo están en TurboGAMES (añadidas a mano u olvidadas por Steam)
	for k, m := range metas {
		if seen[k] {
			continue
		}
		acc := Account{Key: k, Name: m.Name, SteamID: m.SteamID, Persona: m.Persona, LastUsed: m.LastUsed}
		acc.AutoLogin = strings.EqualFold(st.AutoLogin, m.Name)
		acc.Active = st.Running && st.ActiveUser != 0 && m.SteamID != "" && steam.AccountID(m.SteamID) == st.ActiveUser
		applyMeta(&acc, m)
		out = append(out, acc)
	}

	for i := range out {
		acc := &out[i]
		if in != nil && acc.SteamID != "" {
			if fi, err := os.Stat(in.AvatarPath(acc.SteamID)); err == nil {
				acc.Avatar = "/img/avatar/" + acc.SteamID + "?v=" + strconv.FormatInt(fi.ModTime().Unix(), 36)
			}
			if id := steam.AccountID(acc.SteamID); id != 0 {
				var best uint32
				var bestT int64
				for appid, ua := range a.userApps(in, id) {
					m, ok := meta[appid]
					if (ok && !isPlayable(m.Type)) || isPseudoApp(appid) {
						continue
					}
					acc.Games++
					acc.Minutes += ua.Minutes
					if ua.LastPlayed > bestT && ok {
						best, bestT = appid, ua.LastPlayed
					}
				}
				if best != 0 {
					acc.Recent = &Recent{AppID: best, Name: meta[best].Name, LastPlayed: bestT}
				}
			}
		}
		if acc.Tags == nil {
			acc.Tags = []string{}
		}
	}
	return out, err
}

func applyMeta(acc *Account, m Meta) {
	acc.Alias, acc.Notes, acc.Tags = m.Alias, m.Notes, m.Tags
	acc.Pinned, acc.Hidden, acc.HasSecret, acc.Added = m.Pinned, m.Hidden, m.HasSecret, m.Added
	if acc.Persona == "" {
		acc.Persona = m.Persona
	}
	if acc.SteamID == "" {
		acc.SteamID = m.SteamID
	}
}

// isPseudoApp: apps que no son juegos de verdad aunque Steam las cuente como
// tales. 480 es Spacewar, la app de pruebas de Valve que usan muchos
// programas y juegos de fuera de Steam para tener su superposición.
func isPseudoApp(appid uint32) bool { return appid == 480 }

// isPlayable: tipos de appinfo que cuentan como juego (no herramientas, DLC…).
func isPlayable(t string) bool {
	switch t {
	case "game", "demo", "beta", "mod", "":
		return true
	}
	return false
}

// userApps lee (con caché por fecha del archivo) lo que ha jugado una cuenta.
func (a *App) userApps(in *steam.Install, id uint32) map[uint32]steam.UserApp {
	p := in.Path("userdata", strconv.FormatUint(uint64(id), 10), "config", "localconfig.vdf")
	var mod int64
	if fi, err := os.Stat(p); err == nil {
		mod = fi.ModTime().UnixNano()
	} else {
		return nil
	}
	a.uaMu.Lock()
	c, ok := a.uaCache[id]
	a.uaMu.Unlock()
	if ok && c.mod == mod {
		return c.apps
	}
	apps := in.UserApps(id)
	a.uaMu.Lock()
	a.uaCache[id] = uaEntry{mod: mod, apps: apps}
	a.uaMu.Unlock()
	return apps
}

type uaEntry struct {
	mod  int64
	apps map[uint32]steam.UserApp
}

// cleanTags quita etiquetas vacías o repetidas.
func cleanTags(tags []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range tags {
		t = strings.TrimFunc(t, func(r rune) bool { return unicode.IsSpace(r) || r == ',' || r == '#' })
		if t == "" || len([]rune(t)) > 24 {
			continue
		}
		if k := strings.ToLower(t); !seen[k] {
			seen[k] = true
			out = append(out, t)
		}
		if len(out) >= 8 {
			break
		}
	}
	return out
}

// validAccountName: los nombres de usuario de Steam son letras, números y _.
func validAccountName(n string) bool {
	if len(n) < 2 || len(n) > 64 {
		return false
	}
	for _, r := range n {
		if !(r == '_' || r == '-' || r == '.' || r == '@' || unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}
