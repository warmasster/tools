package main

import (
	"sort"
	"strings"

	"golang.org/x/sys/windows"

	"turbogames/internal/steam"
)

// GamePlayer es una cuenta que ha jugado a un juego en este PC.
type GamePlayer struct {
	Key        string `json:"key"` // cuenta en TurboGAMES ("" si no la tenemos)
	Name       string `json:"name"`
	Persona    string `json:"persona"`
	SteamID    string `json:"steamId"`
	Minutes    int64  `json:"minutes"`
	LastPlayed int64  `json:"lastPlayed"`
}

// Game es un juego de la biblioteca.
type Game struct {
	AppID      uint32       `json:"appid"`
	Name       string       `json:"name"`
	Type       string       `json:"type"`
	Installed  bool         `json:"installed"`
	Dir        string       `json:"dir"`
	Library    string       `json:"library"`
	Size       int64        `json:"size"`
	Updated    int64        `json:"updated"`
	LastPlayed int64        `json:"lastPlayed"`
	Minutes    int64        `json:"minutes"`
	Owner      string       `json:"owner"` // SteamID64 de quien lo instaló
	OwnerKey   string       `json:"ownerKey"`
	Players    []GamePlayer `json:"players"`
	Running    bool         `json:"running"`
	Released   int64        `json:"released"`
}

// LibraryInfo es una carpeta de juegos con el espacio de su disco.
type LibraryInfo struct {
	Path  string `json:"path"`
	Label string `json:"label"`
	Games int    `json:"games"`
	Size  int64  `json:"size"`
	Free  int64  `json:"free"`
	Total int64  `json:"total"`
}

// GamesData es la biblioteca entera.
type GamesData struct {
	Games     []Game        `json:"games"`
	Libraries []LibraryInfo `json:"libraries"`
}

// Games junta los juegos instalados y los que ha jugado cada cuenta.
func (a *App) Games() (GamesData, error) {
	var d GamesData
	d.Games, d.Libraries = []Game{}, []LibraryInfo{}
	in := a.steam()
	if in == nil {
		return d, steam.ErrNotFound
	}
	meta := in.AppInfo()
	st := in.ReadStatus()

	// cuentas conocidas por su ID de 32 bits
	type who struct{ key, name, persona, steamID string }
	people := map[uint32]who{}
	if users, err := in.LoginUsers(); err == nil {
		for _, u := range users {
			people[steam.AccountID(u.SteamID)] = who{keyOf(u.AccountName), u.AccountName, u.PersonaName, u.SteamID}
		}
	}
	for k, m := range a.metas.all() {
		id := steam.AccountID(m.SteamID)
		if id == 0 {
			continue
		}
		if _, ok := people[id]; !ok {
			people[id] = who{k, m.Name, m.Persona, m.SteamID}
		}
	}
	for _, id := range in.UserDirs() {
		if _, ok := people[id]; !ok {
			people[id] = who{persona: in.UserPersona(id), steamID: steam.SteamID64(id)}
		}
	}

	games := map[uint32]*Game{}
	get := func(id uint32) *Game {
		g := games[id]
		if g == nil {
			m := meta[id]
			g = &Game{AppID: id, Name: m.Name, Type: m.Type, Released: m.Released, Players: []GamePlayer{}}
			games[id] = g
		}
		return g
	}

	libs := map[string]*LibraryInfo{}
	for _, l := range in.Libraries() {
		li := &LibraryInfo{Path: l.Path, Label: l.Label}
		if p, err := windows.UTF16PtrFromString(l.Path); err == nil {
			var free, total, totalFree uint64
			if windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree) == nil {
				li.Free, li.Total = int64(free), int64(total)
			}
		}
		libs[strings.ToLower(l.Path)] = li
		d.Libraries = append(d.Libraries, *li)
	}

	for _, ia := range in.Installed() {
		m, known := meta[ia.AppID]
		if known && (m.Type == "tool" || m.Type == "config") {
			continue // redistribuibles, Proton, configuraciones…
		}
		g := get(ia.AppID)
		if g.Name == "" {
			g.Name = ia.Name
		}
		g.Installed, g.Dir, g.Library, g.Size, g.Updated = true, ia.InstallDir, ia.Library, ia.SizeOnDisk, ia.LastUpdated
		g.LastPlayed = max(g.LastPlayed, ia.LastPlayed)
		g.Owner = ia.LastOwner
		if w, ok := people[steam.AccountID(ia.LastOwner)]; ok {
			g.OwnerKey = w.key
		}
		if li := libs[strings.ToLower(ia.Library)]; li != nil {
			li.Games++
			li.Size += ia.SizeOnDisk
		}
	}

	for id, w := range people {
		for appid, ua := range a.userApps(in, id) {
			m, known := meta[appid]
			if !known || !isPlayable(m.Type) || isPseudoApp(appid) {
				continue // sin nombre o que no es un juego (configuraciones, herramientas)
			}
			g := get(appid)
			g.Players = append(g.Players, GamePlayer{Key: w.key, Name: w.name, Persona: w.persona, SteamID: w.steamID, Minutes: ua.Minutes, LastPlayed: ua.LastPlayed})
			g.Minutes += ua.Minutes
			g.LastPlayed = max(g.LastPlayed, ua.LastPlayed)
		}
	}

	for _, g := range games {
		if g.Name == "" {
			continue
		}
		g.Running = st.RunningApp != 0 && st.RunningApp == g.AppID
		sort.Slice(g.Players, func(i, j int) bool {
			if g.Players[i].LastPlayed != g.Players[j].LastPlayed {
				return g.Players[i].LastPlayed > g.Players[j].LastPlayed
			}
			return g.Players[i].Minutes > g.Players[j].Minutes
		})
		d.Games = append(d.Games, *g)
	}
	sort.Slice(d.Games, func(i, j int) bool {
		if d.Games[i].LastPlayed != d.Games[j].LastPlayed {
			return d.Games[i].LastPlayed > d.Games[j].LastPlayed
		}
		return strings.ToLower(d.Games[i].Name) < strings.ToLower(d.Games[j].Name)
	})
	for i := range d.Libraries {
		if li := libs[strings.ToLower(d.Libraries[i].Path)]; li != nil {
			d.Libraries[i] = *li
		}
	}
	return d, nil
}
