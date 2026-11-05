package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"turbogames/internal/steam"
	"turbogames/internal/steamworks"
)

// ---------- caché de datos de los mods ----------

// wsEntry son los datos de un mod que ya se han pedido.
type wsEntry struct {
	Item  steamworks.Item `json:"item"`
	At    int64           `json:"at"`    // cuándo se pidieron
	Votes bool            `json:"votes"` // vinieron de Steam con votos y dependencias
}

type wsAuthor struct {
	Name string `json:"name"`
	At   int64  `json:"at"`
}

// wsCache guarda en %APPDATA%\TurboGAMES\cache\workshop.json los nombres, autores,
// imágenes y votos de los mods para no pedirlos cada vez.
type wsCache struct {
	mu      sync.Mutex
	path    string
	Items   map[uint64]*wsEntry  `json:"items"`
	Authors map[string]*wsAuthor `json:"authors"`
}

func loadWsCache(path string) *wsCache {
	c := &wsCache{path: path}
	_ = loadJSON(path, c)
	if c.Items == nil {
		c.Items = map[uint64]*wsEntry{}
	}
	if c.Authors == nil {
		c.Authors = map[string]*wsAuthor{}
	}
	return c
}

func (c *wsCache) save() {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = os.MkdirAll(filepath.Dir(c.path), 0o755)
	_ = saveJSON(c.path, c)
}

func (c *wsCache) get(id uint64) (wsEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.Items[id]
	if e == nil {
		return wsEntry{}, false
	}
	return *e, true
}

func (c *wsCache) put(items []steamworks.Item, votes bool) {
	c.mu.Lock()
	now := time.Now().Unix()
	for _, it := range items {
		if it.ID == 0 {
			continue
		}
		old := c.Items[it.ID]
		if it.Missing && old != nil && !old.Item.Missing {
			// ya no existe o se ha vuelto privado: se queda el nombre que tenía
			old.Item.Missing, old.At = true, now
			continue
		}
		if !votes && old != nil && old.Votes {
			// la web no da votos ni dependencias: se guardan los que había
			it.VotesUp, it.VotesDown, it.Score, it.Children = old.Item.VotesUp, old.Item.VotesDown, old.Item.Score, old.Item.Children
		}
		c.Items[it.ID] = &wsEntry{Item: it, At: now, Votes: votes || (old != nil && old.Votes)}
	}
	c.mu.Unlock()
}

func (c *wsCache) author(id string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if a := c.Authors[id]; a != nil {
		return a.Name
	}
	return ""
}

// ---------- la web de Steam (sin clave) ----------

var webClient = &http.Client{Timeout: 25 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		h := req.URL.Hostname()
		if len(via) >= 5 || req.URL.Scheme != "https" || !(h == "steamcommunity.com" || h == "api.steampowered.com") {
			return http.ErrUseLastResponse
		}
		return nil
	}}

// flexNum lee un número que la API de Steam a veces manda como texto.
type flexNum int64

func (n *flexNum) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*n = 0
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		f, ferr := strconv.ParseFloat(s, 64)
		if ferr != nil {
			return nil
		}
		v = int64(f)
	}
	*n = flexNum(v)
	return nil
}

func postForm(u string, form url.Values, out any) error {
	req, _ := http.NewRequest("POST", u, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "TurboGAMES")
	resp, err := webClient.Do(req)
	if err != nil {
		return errors.New("No hay conexión con Steam")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Steam ha respondido %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(out)
}

// webDetails pide los datos de unos mods a la API pública de Steam
// (ISteamRemoteStorage/GetPublishedFileDetails, que no pide clave). No trae
// votos ni dependencias: eso solo lo da Steam abierto.
func webDetails(ids []uint64) ([]steamworks.Item, error) {
	var out []steamworks.Item
	for len(ids) > 0 {
		n := min(len(ids), 100)
		chunk := ids[:n]
		ids = ids[n:]
		form := url.Values{"itemcount": {strconv.Itoa(n)}}
		for i, id := range chunk {
			form.Set(fmt.Sprintf("publishedfileids[%d]", i), strconv.FormatUint(id, 10))
		}
		var r struct {
			Response struct {
				Details []struct {
					ID          flexNum `json:"publishedfileid"`
					Result      int     `json:"result"`
					Creator     string  `json:"creator"`
					App         flexNum `json:"consumer_app_id"`
					Size        flexNum `json:"file_size"`
					Preview     string  `json:"preview_url"`
					Title       string  `json:"title"`
					Description string  `json:"description"`
					Created     flexNum `json:"time_created"`
					Updated     flexNum `json:"time_updated"`
					Visibility  flexNum `json:"visibility"`
					Banned      flexNum `json:"banned"`
					Subs        flexNum `json:"subscriptions"`
					Favs        flexNum `json:"favorited"`
					Tags        []struct {
						Tag string `json:"tag"`
					} `json:"tags"`
				} `json:"publishedfiledetails"`
			} `json:"response"`
		}
		if err := postForm("https://api.steampowered.com/ISteamRemoteStorage/GetPublishedFileDetails/v1/", form, &r); err != nil {
			return out, err
		}
		for _, d := range r.Response.Details {
			it := steamworks.Item{ID: uint64(d.ID), Tags: []string{}, Children: []uint64{}}
			if d.Result != 1 {
				it.Missing = true
				out = append(out, it)
				continue
			}
			it.AppID = uint32(d.App)
			it.Title = strings.TrimSpace(d.Title)
			it.Description = d.Description
			it.Owner = d.Creator
			it.Created, it.Updated = int64(d.Created), int64(d.Updated)
			it.Visibility = int32(d.Visibility)
			it.Banned = d.Banned != 0
			it.Size = int64(d.Size)
			it.Preview = d.Preview
			it.Subscriptions, it.Favorites = uint64(d.Subs), uint64(d.Favs)
			for _, t := range d.Tags {
				if t.Tag != "" {
					it.Tags = append(it.Tags, t.Tag)
				}
			}
			out = append(out, it)
		}
	}
	return out, nil
}

// Se pueden cambiar en las pruebas para no ir a internet.
var (
	webDetailsFn     = webDetails
	webCollectionsFn = webCollections
)

// webCollections dice qué ids son colecciones y qué mods tienen dentro.
func webCollections(ids []uint64) (map[uint64][]uint64, error) {
	out := map[uint64][]uint64{}
	for len(ids) > 0 {
		n := min(len(ids), 100)
		chunk := ids[:n]
		ids = ids[n:]
		form := url.Values{"collectioncount": {strconv.Itoa(n)}}
		for i, id := range chunk {
			form.Set(fmt.Sprintf("publishedfileids[%d]", i), strconv.FormatUint(id, 10))
		}
		var r struct {
			Response struct {
				Details []struct {
					ID       flexNum `json:"publishedfileid"`
					Result   int     `json:"result"`
					Children []struct {
						ID       flexNum `json:"publishedfileid"`
						Order    int     `json:"sortorder"`
						FileType int     `json:"filetype"`
					} `json:"children"`
				} `json:"collectiondetails"`
			} `json:"response"`
		}
		if err := postForm("https://api.steampowered.com/ISteamRemoteStorage/GetCollectionDetails/v1/", form, &r); err != nil {
			return out, err
		}
		for _, d := range r.Response.Details {
			if d.Result != 1 || len(d.Children) == 0 {
				continue
			}
			kids := d.Children
			sort.SliceStable(kids, func(i, j int) bool { return kids[i].Order < kids[j].Order })
			var list []uint64
			for _, k := range kids {
				if k.ID != 0 {
					list = append(list, uint64(k.ID))
				}
			}
			out[uint64(d.ID)] = list
		}
	}
	return out, nil
}

var xmlNameRe = regexp.MustCompile(`<steamID><!\[CDATA\[(.*?)\]\]></steamID>`)

// fillAuthors busca el nombre visible de los autores que aún no se conocen (en
// segundo plano, unos pocos cada vez) y avisa a la interfaz.
func (a *App) fillAuthors(ids []string) {
	var need []string
	seen := map[string]bool{}
	a.wcache.mu.Lock()
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if au := a.wcache.Authors[id]; au == nil || (au.Name == "" && time.Now().Unix()-au.At > 86400) {
			need = append(need, id)
		}
	}
	a.wcache.mu.Unlock()
	if len(need) == 0 {
		return
	}
	if len(need) > 60 {
		need = need[:60]
	}
	go func() {
		sem := make(chan struct{}, 4)
		var wg sync.WaitGroup
		for _, id := range need {
			if _, err := strconv.ParseUint(id, 10, 64); err != nil {
				continue
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(id string) {
				defer func() { <-sem; wg.Done() }()
				req, _ := http.NewRequest("GET", "https://steamcommunity.com/profiles/"+id+"/?xml=1", nil)
				req.Header.Set("User-Agent", "TurboGAMES")
				name := ""
				if resp, err := webClient.Do(req); err == nil {
					b, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
					resp.Body.Close()
					if m := xmlNameRe.FindSubmatch(b); m != nil {
						name = strings.TrimSpace(html.UnescapeString(string(m[1])))
					}
				}
				a.wcache.mu.Lock()
				a.wcache.Authors[id] = &wsAuthor{Name: clip(name, 64), At: time.Now().Unix()}
				a.wcache.mu.Unlock()
			}(id)
		}
		wg.Wait()
		a.wcache.save()
		a.emit("mods-meta", nil)
	}()
}

// ---------- lo que ve la interfaz ----------

// Mod es un mod tal como lo enseña la interfaz.
type Mod struct {
	ID            uint64   `json:"id"`
	AppID         uint32   `json:"appid"`
	Title         string   `json:"title"`
	Author        string   `json:"author"`
	Owner         string   `json:"owner"`
	Preview       bool     `json:"preview"`
	Size          int64    `json:"size"`
	Updated       int64    `json:"updated"`
	Tags          []string `json:"tags"`
	VotesUp       uint32   `json:"votesUp"`
	VotesDown     uint32   `json:"votesDown"`
	Score         float32  `json:"score"`
	HasVotes      bool     `json:"hasVotes"`
	Subscriptions uint64   `json:"subscriptions"`
	Comments      uint64   `json:"comments"`
	Children      []uint64 `json:"children"`
	Missing       bool     `json:"missing"`
	Banned        bool     `json:"banned"`
	Collection    bool     `json:"collection"`
	Known         bool     `json:"known"`   // tenemos sus datos
	MyTags        []string `json:"myTags"`  // etiquetas que le has puesto tú
	Summary       string   `json:"summary"` // el principio de la descripción, sin formato
	Since         int64    `json:"since"`   // desde cuándo está suscrito (lo que ha visto TurboGAMES)
	Removed       int64    `json:"removed"` // cuándo se quitó (en el historial)

	Subscribed   bool    `json:"subscribed"`   // la cuenta abierta en Steam
	SubscribedBy string  `json:"subscribedBy"` // SteamID64 de quien se suscribió en este PC
	Installed    bool    `json:"installed"`
	Dir          string  `json:"dir"`
	DiskSize     int64   `json:"diskSize"`
	NeedsUpdate  bool    `json:"needsUpdate"`
	Downloading  bool    `json:"downloading"`
	Pending      bool    `json:"pending"`
	Progress     float64 `json:"progress"` // 0 a 1 mientras se descarga
}

// ModList son los mods de un juego.
type ModList struct {
	AppID     uint32 `json:"appid"`
	Mods      []Mod  `json:"mods"`
	Live      bool   `json:"live"`      // lo de «suscrito» viene de Steam ahora mismo
	LiveError string `json:"liveError"` // por qué no se ha podido preguntar a Steam
	Account   string `json:"account"`   // SteamID64 de la cuenta abierta en Steam
	DiskSize  int64  `json:"diskSize"`
	History   []Mod  `json:"history"` // los que tuvo antes esta cuenta y ya no
	NotOwned  bool   `json:"notOwned"`
	Pending   int    `json:"pending"` // mods cuyos datos se están pidiendo en segundo plano
}

func (a *App) modFrom(id uint64) Mod {
	m := Mod{ID: id, Tags: []string{}, Children: []uint64{}}
	if e, ok := a.wcache.get(id); ok {
		it := e.Item
		m.Known = true
		m.AppID, m.Title, m.Owner, m.Size, m.Updated = it.AppID, it.Title, it.Owner, it.Size, it.Updated
		m.Preview = it.Preview != ""
		m.Tags, m.Children = nonNil(it.Tags), nonNilU(it.Children)
		m.VotesUp, m.VotesDown, m.Score, m.HasVotes = it.VotesUp, it.VotesDown, it.Score, e.Votes
		m.Subscriptions, m.Comments = it.Subscriptions, it.Comments
		m.Missing, m.Banned, m.Collection = it.Missing, it.Banned, it.Collection
		m.Author = a.wcache.author(it.Owner)
		m.Summary = summary(it.Description)
	}
	if a.mstore != nil {
		m.MyTags = a.mstore.tags(id)
	}
	return m
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nonNilU(s []uint64) []uint64 {
	if s == nil {
		return []uint64{}
	}
	return s
}

// ensureDetails pide los datos de los mods que no están en la caché (o están
// viejos). Con Steam abierto se piden a Steam (traen votos y dependencias); si
// no, a la web.
func (a *App) ensureDetails(appid uint32, ids []uint64, live bool, maxAge time.Duration) {
	need := a.needDetails(ids, live, maxAge)
	if len(need) == 0 {
		return
	}
	if live && appid != 0 {
		var items []steamworks.Item
		if err := a.wsCall(appid, wsRequest{Op: "details", IDs: need}, 90*time.Second, &items); err == nil {
			a.wcache.put(items, true)
			a.wcache.save()
			return
		}
	}
	if items, err := webDetailsFn(need); err == nil || len(items) > 0 {
		a.wcache.put(items, false)
		a.wcache.save()
	}
}

// needDetails dice de qué mods no hay datos (o son viejos, o les faltan los
// votos si se puede preguntar a Steam).
func (a *App) needDetails(ids []uint64, live bool, maxAge time.Duration) []uint64 {
	var need []uint64
	now := time.Now().Unix()
	for _, id := range ids {
		e, ok := a.wcache.get(id)
		if !ok || now-e.At > int64(maxAge.Seconds()) || (live && !e.Votes && !e.Item.Missing) {
			need = append(need, id)
		}
	}
	return need
}

// fillDetails pide en segundo plano, de 50 en 50, los datos de unos mods, y
// avisa a la interfaz con "mods-meta" después de cada tanda para que los vaya
// enseñando. Para si se cambia de juego.
func (a *App) fillDetails(appid uint32, ids []uint64, live bool) int {
	a.fillMu.Lock()
	if a.filling == nil {
		a.filling = map[uint64]bool{}
	}
	var todo []uint64
	for _, id := range ids {
		if !a.filling[id] {
			a.filling[id] = true
			todo = append(todo, id)
		}
	}
	a.fillMu.Unlock()
	if len(todo) == 0 {
		return len(ids)
	}
	go func() {
		defer func() {
			a.fillMu.Lock()
			for _, id := range todo {
				delete(a.filling, id)
			}
			a.fillMu.Unlock()
		}()
		for len(todo) > 0 {
			if a.modsApp.Load() != appid {
				return
			}
			n := min(len(todo), 50)
			chunk := todo[:n]
			todo = todo[n:]
			got := false
			if live {
				var items []steamworks.Item
				if a.wsCall(appid, wsRequest{Op: "details", IDs: chunk}, 60*time.Second, &items) == nil {
					a.wcache.put(items, true)
					got = true
				}
			}
			if !got {
				if items, err := webDetailsFn(chunk); err == nil || len(items) > 0 {
					a.wcache.put(items, false)
				}
			}
			a.wcache.save()
			var owners []string
			for _, id := range chunk {
				if e, ok := a.wcache.get(id); ok {
					owners = append(owners, e.Item.Owner)
				}
			}
			a.fillAuthors(owners)
			a.emit("mods-meta", map[string]any{"appid": appid, "ids": chunk, "left": len(todo)})
		}
	}()
	return len(ids)
}

// ModsCached devuelve lo que se sabe ya de unos mods, sin pedir nada (para
// refrescar la lista cuando llegan datos en segundo plano).
func (a *App) ModsCached(ids []uint64) []Mod {
	if len(ids) > 5000 {
		ids = ids[:5000]
	}
	out := make([]Mod, 0, len(ids))
	for _, id := range ids {
		out = append(out, a.modFrom(id))
	}
	return out
}

// Mods devuelve los mods de un juego: los que tiene Steam en este PC y, si
// live, a los que está suscrita ahora la cuenta abierta en Steam (aunque el
// juego no esté instalado).
func (a *App) Mods(appid uint32, live bool) (ModList, error) {
	l := ModList{AppID: appid, Mods: []Mod{}, History: []Mod{}}
	a.modsApp.Store(appid)
	in := a.steam()
	if in == nil {
		return l, steam.ErrNotFound
	}
	st := in.ReadStatus()
	if st.ActiveUser != 0 {
		l.Account = steam.SteamID64(st.ActiveUser)
	}
	local := map[uint64]steam.WorkshopItem{}
	if w := in.Workshop()[appid]; w != nil {
		for _, it := range w.Items {
			local[it.ID] = it
		}
	}
	var liveStates map[uint64]steamworks.ItemState
	if live {
		var states []steamworks.ItemState
		if err := a.wsCall(appid, wsRequest{Op: "subscribed"}, 30*time.Second, &states); err != nil {
			l.LiveError = err.Error()
			l.NotOwned = errors.Is(err, errNotOwned) || err.Error() == steamworks.ErrNotOwned.Error()
		} else {
			l.Live = true
			liveStates = map[uint64]steamworks.ItemState{}
			for _, s := range states {
				liveStates[s.ID] = s
			}
		}
	}
	ids := make([]uint64, 0, len(local)+len(liveStates))
	for id := range local {
		ids = append(ids, id)
	}
	for id := range liveStates {
		if _, ok := local[id]; !ok {
			ids = append(ids, id)
		}
	}
	// lo que falte llega después, por tandas (evento "mods-meta")
	pending := map[uint64]bool{}
	if need := a.needDetails(ids, l.Live, 24*time.Hour); len(need) > 0 {
		for _, id := range need {
			pending[id] = true
		}
	}
	var owners []string
	for _, id := range ids {
		m := a.modFrom(id)
		if m.AppID == 0 {
			m.AppID = appid
		}
		if it, ok := local[id]; ok {
			m.Installed, m.Dir, m.DiskSize = it.Installed, it.Dir, it.Size
			if it.SubscribedBy != 0 {
				m.SubscribedBy = steam.SteamID64(it.SubscribedBy)
			}
			m.NeedsUpdate = it.Installed && it.Latest > it.Updated
			if !l.Live && it.SubscribedBy != 0 && it.SubscribedBy == st.ActiveUser {
				m.Subscribed = true
			}
			l.DiskSize += it.Size
		}
		if s, ok := liveStates[id]; ok {
			m.Subscribed = s.Flags&steamworks.StateSubscribed != 0
			if s.Flags&steamworks.StateInstalled != 0 {
				m.Installed = true
				if s.Folder != "" {
					m.Dir = s.Folder
				}
				if s.Size > 0 {
					m.DiskSize = s.Size
				}
			}
			m.NeedsUpdate = s.Flags&steamworks.StateNeedsUpdate != 0
			m.Downloading = s.Flags&steamworks.StateDownloading != 0
			m.Pending = s.Flags&steamworks.StateDownloadPending != 0
			if s.Total > 0 {
				m.Progress = float64(s.Downloaded) / float64(s.Total)
			}
			if m.SubscribedBy == "" && m.Subscribed && l.Account != "" {
				m.SubscribedBy = l.Account
			}
		}
		if m.Title == "" {
			m.Title = "Mod " + strconv.FormatUint(id, 10)
		}
		owners = append(owners, m.Owner)
		l.Mods = append(l.Mods, m)
	}
	sort.Slice(l.Mods, func(i, j int) bool {
		return strings.ToLower(l.Mods[i].Title) < strings.ToLower(l.Mods[j].Title)
	})
	// historial: a qué estaba suscrita esta cuenta antes
	if l.Live && l.Account != "" {
		var subs []uint64
		titles := map[uint64]string{}
		for _, m := range l.Mods {
			if m.Subscribed {
				subs = append(subs, m.ID)
				titles[m.ID] = m.Title
			}
		}
		// los nombres se sacan antes: record tiene el cerrojo de mstore
		a.mstore.record(l.Account, appid, subs, func(id uint64) string { return titles[id] })
	}
	if l.Account != "" {
		removed, since := a.mstore.history(l.Account, appid)
		for i := range l.Mods {
			l.Mods[i].Since = since[l.Mods[i].ID]
		}
		var hids []uint64
		for _, h := range removed {
			hids = append(hids, h.ID)
		}
		for _, id := range a.needDetails(hids, false, 7*24*time.Hour) {
			pending[id] = true
		}
		for _, h := range removed {
			m := a.modFrom(h.ID)
			if m.Title == "" {
				m.Title = firstNonEmpty(h.Title, "Mod "+strconv.FormatUint(h.ID, 10))
			}
			m.Removed, m.Since = h.Removed, h.First
			owners = append(owners, m.Owner)
			l.History = append(l.History, m)
		}
	}
	a.fillAuthors(owners)
	if len(pending) > 0 {
		need := make([]uint64, 0, len(pending))
		for id := range pending {
			need = append(need, id)
		}
		l.Pending = a.fillDetails(appid, need, l.Live)
	}
	return l, nil
}

var errNotOwned = steamworks.ErrNotOwned

// ModDetails devuelve los datos de unos mods (para los presets: nombres,
// tamaños y si existen). No pregunta a Steam si no está ya conectado.
func (a *App) ModDetails(appid uint32, ids []uint64) []Mod {
	if len(ids) > 5000 {
		ids = ids[:5000]
	}
	a.ensureDetails(appid, ids, false, 7*24*time.Hour)
	out := make([]Mod, 0, len(ids))
	var owners []string
	for _, id := range ids {
		m := a.modFrom(id)
		owners = append(owners, m.Owner)
		out = append(out, m)
	}
	a.fillAuthors(owners)
	return out
}

// ---------- ficha de un mod ----------

// ModComment es un comentario del Workshop.
type ModComment struct {
	Author string `json:"author"`
	Text   string `json:"text"`
	Time   int64  `json:"time"`
}

// ModInfoData es todo lo que enseña la ficha de un mod.
type ModInfoData struct {
	Mod         Mod                  `json:"mod"`
	Description string               `json:"description"`
	PreviewURL  string               `json:"previewUrl"`
	Required    []Mod                `json:"required"` // mods que necesita
	Stars       int                  `json:"stars"`    // 0 a 5 (la valoración de la página; -1 = sin valorar)
	Ratings     int                  `json:"ratings"`
	CommentList []ModComment         `json:"commentList"`
	CommentsN   int                  `json:"commentsN"`
	PageError   string               `json:"pageError"`
	Gallery     []steamworks.Preview `json:"gallery"` // imágenes y vídeos
}

var (
	reStars    = regexp.MustCompile(`/(\d|not-yet)[-_]star_large\.png`)
	reRatings  = regexp.MustCompile(`class="numRatings"[^>]*>\s*([\d.,\s]+)`)
	reReqBlock = regexp.MustCompile(`(?s)id="RequiredItems"(.*?)</div>\s*</div>\s*</div>`)
	reReqItem  = regexp.MustCompile(`(?s)filedetails/\?id=(\d+)[^>]*>\s*<div class="requiredItem">\s*(.*?)\s*</div>`)
	reComment  = regexp.MustCompile(`(?s)class="commentthread_comment_author".*?<bdi>(.*?)</bdi>.*?data-timestamp="(\d+)".*?class="commentthread_comment_text"[^>]*>(.*?)</div>`)
	reTags     = regexp.MustCompile(`<[^>]+>`)
	reBR       = regexp.MustCompile(`(?i)<br\s*/?>`)
)

func htmlText(s string) string {
	s = reBR.ReplaceAllString(s, "\n")
	s = reTags.ReplaceAllString(s, "")
	return strings.TrimSpace(html.UnescapeString(s))
}

// ModInfo trae la ficha completa de un mod: descripción, mods que necesita,
// valoración, imágenes y vídeos, y los últimos comentarios (de su página del
// Workshop). Con live, primero se lo pregunta a Steam (si ya está conectado o
// se puede conectar).
func (a *App) ModInfo(appid uint32, id uint64, live bool) (ModInfoData, error) {
	var d ModInfoData
	if id == 0 {
		return d, errors.New("falta el mod")
	}
	if live && appid != 0 {
		if e, ok := a.wcache.get(id); !ok || !e.Item.HasPreviews || time.Now().Unix()-e.At > 3600 {
			var items []steamworks.Item
			if a.wsCall(appid, wsRequest{Op: "details", IDs: []uint64{id}}, 30*time.Second, &items) == nil {
				a.wcache.put(items, true)
				a.wcache.save()
			}
		}
	}
	a.ensureDetails(appid, []uint64{id}, false, 6*time.Hour)
	d.Mod = a.modFrom(id)
	fromSteam := false
	if e, ok := a.wcache.get(id); ok {
		d.Description = e.Item.Description
		d.PreviewURL = e.Item.Preview
		if e.Item.HasPreviews {
			d.Gallery, fromSteam = galleryFromItem(e.Item), true
		}
	}
	d.Stars = -1
	req, _ := http.NewRequest("GET", "https://steamcommunity.com/sharedfiles/filedetails/?id="+strconv.FormatUint(id, 10), nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) TurboGAMES")
	req.Header.Set("Accept-Language", "es-ES,es;q=0.9")
	resp, err := webClient.Do(req)
	if err != nil {
		d.PageError = "No se pudo abrir su página del Workshop"
	} else {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		page := string(b)
		if m := reStars.FindStringSubmatch(page); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil {
				d.Stars = n
			}
		}
		if m := reRatings.FindStringSubmatch(page); m != nil {
			d.Ratings, _ = strconv.Atoi(strings.NewReplacer(".", "", ",", "", " ", "", " ", "").Replace(m[1]))
		}
		var req []uint64
		if blk := reReqBlock.FindStringSubmatch(page); blk != nil {
			for _, m := range reReqItem.FindAllStringSubmatch(blk[1], 200) {
				if n, err := strconv.ParseUint(m[1], 10, 64); err == nil && n != id {
					req = append(req, n)
				}
			}
		}
		if len(req) == 0 {
			req = d.Mod.Children
		}
		if !fromSteam {
			d.Gallery = pageGallery(page)
		}
		if len(req) > 0 {
			d.Required = a.ModDetails(appid, req)
		}
	}
	// Los comentarios no vienen en la página: la web los pide aparte. Si el
	// perfil del autor es privado, Steam no los da sin iniciar sesión.
	if d.Mod.Owner != "" {
		var r struct {
			Success bool    `json:"success"`
			Total   flexNum `json:"total_count"`
			HTML    string  `json:"comments_html"`
		}
		u := "https://steamcommunity.com/comment/PublishedFile_Public/render/" + d.Mod.Owner + "/" + strconv.FormatUint(id, 10) + "/"
		if postForm(u, url.Values{"start": {"0"}, "count": {"10"}}, &r) == nil && r.Success {
			d.CommentsN = int(r.Total)
			for _, m := range reComment.FindAllStringSubmatch(r.HTML, 10) {
				t, _ := strconv.ParseInt(m[2], 10, 64)
				d.CommentList = append(d.CommentList, ModComment{Author: clip(htmlText(m[1]), 64), Text: clip(htmlText(m[3]), 1500), Time: t})
			}
		}
	}
	if d.CommentsN == 0 {
		d.CommentsN = int(d.Mod.Comments)
	}
	if d.Required == nil {
		d.Required = []Mod{}
	}
	if d.Gallery == nil {
		d.Gallery = []steamworks.Preview{}
	}
	if d.CommentList == nil {
		d.CommentList = []ModComment{}
	}
	a.fillAuthors([]string{d.Mod.Owner})
	return d, nil
}

// ---------- buscar en el Workshop ----------

// ModPage es una página de resultados de búsqueda.
type ModPage struct {
	Mods   []Mod  `json:"mods"`
	Total  uint32 `json:"total"`
	Cursor string `json:"cursor"`
}

var searchSorts = map[string]int{
	"relevance": steamworks.QueryByText,
	"trend":     steamworks.QueryByTrend, "votes": steamworks.QueryByVote, "subs": steamworks.QueryBySubscribers,
	"recent": steamworks.QueryByDate, "updated": steamworks.QueryByLastUpdated,
}

// SearchQuery son los filtros de una búsqueda en el Workshop.
type SearchQuery struct {
	Text    string   `json:"text"`
	Sort    string   `json:"sort"` // relevance, trend, votes, subs, recent, updated
	Days    int      `json:"days"` // para trend: populares de los últimos N días
	Cursor  string   `json:"cursor"`
	Tags    []string `json:"tags"`
	Exclude []string `json:"exclude"`
}

// ModsSearch busca mods de un juego en el Workshop (necesita Steam abierto).
func (a *App) ModsSearch(appid uint32, q SearchQuery) (ModPage, error) {
	text, sortBy, cursor, tags := q.Text, q.Sort, q.Cursor, q.Tags
	if strings.TrimSpace(text) == "" && sortBy == "relevance" {
		sortBy = "trend"
	}
	excl := q.Exclude
	if len(excl) > 10 {
		excl = excl[:10]
	}
	p := ModPage{Mods: []Mod{}}
	s, ok := searchSorts[sortBy]
	if !ok {
		s = steamworks.QueryByTrend
	}
	var page steamworks.Page
	if len(tags) > 10 {
		tags = tags[:10]
	}
	for i := range tags {
		tags[i] = clip(tags[i], 64)
	}
	if err := a.wsCall(appid, wsRequest{Op: "search", Text: clip(text, 200), Sort: s, Cursor: cursor, Tags: tags, Excl: excl, Days: q.Days}, 45*time.Second, &page); err != nil {
		return p, err
	}
	a.wcache.put(page.Items, true)
	a.wcache.save()
	ids := make([]uint64, 0, len(page.Items))
	for _, it := range page.Items {
		ids = append(ids, it.ID)
	}
	var states []steamworks.ItemState
	sub := map[uint64]steamworks.ItemState{}
	if a.wsCall(appid, wsRequest{Op: "states", IDs: ids}, 20*time.Second, &states) == nil {
		for _, s := range states {
			sub[s.ID] = s
		}
	}
	var owners []string
	for _, it := range page.Items {
		if it.Missing {
			continue
		}
		m := a.modFrom(it.ID)
		if s, ok := sub[it.ID]; ok {
			m.Subscribed = s.Flags&steamworks.StateSubscribed != 0
			m.Installed = s.Flags&steamworks.StateInstalled != 0
		}
		owners = append(owners, m.Owner)
		p.Mods = append(p.Mods, m)
	}
	p.Total, p.Cursor = page.Total, page.Cursor
	a.fillAuthors(owners)
	return p, nil
}

// ---------- suscribir y quitar ----------

// ModsChangeResult dice cómo ha ido un cambio de suscripciones.
type ModsChangeResult struct {
	Subscribed   int          `json:"subscribed"`
	Unsubscribed int          `json:"unsubscribed"`
	Failed       []ModFailure `json:"failed"`
	Canceled     bool         `json:"canceled"`
	Backup       string       `json:"backup"` // preset con lo que había antes, si se ha quitado algo
}

// ModFailure es un mod que no se ha podido cambiar.
type ModFailure struct {
	ID    uint64 `json:"id"`
	Title string `json:"title"`
	Error string `json:"error"`
}

// ModsChange suscribe la cuenta abierta en Steam a unos mods y le quita otros,
// con varios a la vez en marcha (como hace el propio Steam con «cancelar todas
// las suscripciones»), contando el progreso con el evento "mods-progress". Lo
// que falle por algo pasajero se vuelve a intentar una vez. Si va a quitar algo, antes guarda un preset
// automático con lo que había.
func (a *App) ModsChange(appid uint32, subscribe, unsubscribe []uint64) (ModsChangeResult, error) {
	r := ModsChangeResult{Failed: []ModFailure{}}
	if !a.modsMu.TryLock() {
		return r, errors.New("Ya hay un cambio de mods en marcha")
	}
	defer a.modsMu.Unlock()
	a.modsCancel.Store(false)
	subscribe, unsubscribe = dedupe(subscribe), dedupe(unsubscribe)
	total := len(subscribe) + len(unsubscribe)
	if total == 0 {
		return r, nil
	}
	if total > 5000 {
		return r, errors.New("Demasiados mods de una vez")
	}
	var before []steamworks.ItemState
	if err := a.wsCall(appid, wsRequest{Op: "subscribed"}, 30*time.Second, &before); err != nil {
		return r, err
	}
	if len(unsubscribe) > 0 && len(before) > 0 {
		ids := make([]uint64, 0, len(before))
		for _, s := range before {
			ids = append(ids, s.ID)
		}
		if p, err := a.autoBackup(appid, ids); err == nil {
			r.Backup = p.Name
		}
	}
	done := 0
	title := func(id uint64) string {
		if t := a.modFrom(id).Title; t != "" {
			return t
		}
		return "Mod " + strconv.FormatUint(id, 10)
	}
	// batch manda una tanda al proceso del Workshop (varios mods a la vez en
	// marcha) y devuelve los que han fallado por algo que puede ser pasajero
	batch := func(sub bool, ids []uint64, last bool) []uint64 {
		op := "unsubscribeMany"
		if sub {
			op = "subscribeMany"
		}
		seen := map[uint64]bool{}
		var retry []uint64
		finish := func(id uint64, errMsg string) {
			if seen[id] {
				return
			}
			seen[id] = true
			transient := errMsg != "" && (strings.Contains(errMsg, "a tiempo") || strings.Contains(errMsg, "conexión") || strings.Contains(errMsg, "no ha aceptado"))
			switch {
			case errMsg == "":
				if sub {
					r.Subscribed++
				} else {
					r.Unsubscribed++
				}
			case transient && !last:
				retry = append(retry, id)
				return // se vuelve a intentar: todavía no cuenta
			default:
				r.Failed = append(r.Failed, ModFailure{ID: id, Title: title(id), Error: errMsg})
			}
			done++
			a.emit("mods-progress", map[string]any{"appid": appid, "done": done, "total": total, "title": title(id), "op": strings.TrimSuffix(op, "Many")})
		}
		err := a.wsCallEx(appid, wsRequest{Op: op, IDs: ids}, 60*time.Second, nil, func(p wsProgress) { finish(p.ID, p.Error) }, a.modsCancel.Load)
		if a.modsCancel.Load() {
			r.Canceled = true
		}
		for _, id := range ids {
			if seen[id] {
				continue
			}
			switch {
			case err != nil && !last:
				retry = append(retry, id)
			case err != nil:
				finish(id, err.Error())
			}
			// sin error y sin respuesta: se paró antes de mandarlo
		}
		return retry
	}
	a.emit("mods-progress", map[string]any{"appid": appid, "done": 0, "total": total, "title": title(firstID(unsubscribe, subscribe)), "op": opName(len(unsubscribe) > 0)})
	if len(unsubscribe) > 0 {
		if again := batch(false, unsubscribe, false); len(again) > 0 && !r.Canceled {
			time.Sleep(time.Second)
			batch(false, again, true)
		}
	}
	if len(subscribe) > 0 && !r.Canceled {
		if again := batch(true, subscribe, false); len(again) > 0 && !r.Canceled {
			time.Sleep(time.Second)
			batch(true, again, true)
		}
	}
	a.emit("mods-progress", map[string]any{"appid": appid, "done": total, "total": total, "finished": true})
	return r, nil
}

func firstID(lists ...[]uint64) uint64 {
	for _, l := range lists {
		if len(l) > 0 {
			return l[0]
		}
	}
	return 0
}

func opName(unsub bool) string {
	if unsub {
		return "unsubscribe"
	}
	return "subscribe"
}

// ModsCancel para un cambio de suscripciones en marcha (no se mandan más; los
// que ya van, terminan).
func (a *App) ModsCancel() { a.modsCancel.Store(true) }

func dedupe(ids []uint64) []uint64 {
	seen := map[uint64]bool{}
	out := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if id != 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// OpenModInSteam abre la página de un mod dentro de Steam (para suscribirse a
// mano cuando no se puede desde aquí).
func (a *App) OpenModInSteam(id uint64) error {
	if id == 0 {
		return errors.New("falta el mod")
	}
	in := a.steam()
	if in == nil {
		return steam.ErrNotFound
	}
	return in.OpenURL("steam://url/CommunityFilePage/" + strconv.FormatUint(id, 10))
}

// ModPreviewURL es la imagen de un mod (la sirve imageServer en /img/mod/<id>).
func (a *App) modPreviewURL(id uint64) string {
	if e, ok := a.wcache.get(id); ok {
		return e.Item.Preview
	}
	return ""
}

var (
	reVideoID   = regexp.MustCompile(`^[A-Za-z0-9_-]{6,20}$`)
	rePageVideo = regexp.MustCompile(`YOUTUBE_VIDEO_ID:\s*"([A-Za-z0-9_-]{6,20})"`)
	rePageImage = regexp.MustCompile(`https://images\.steamusercontent\.com/ugc/\d+/[0-9A-Fa-f]+/`)
)

func validVideoID(v string) bool { return reVideoID.MatchString(v) }

// safeImageURL: solo imágenes de los servidores de Steam, por https.
func safeImageURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && modImageHost(u)
}

// pageGallery saca de la página pública de un mod sus vídeos y capturas (la
// zona de arriba, antes de la descripción, para no coger otras imágenes).
func pageGallery(page string) []steamworks.Preview {
	out := []steamworks.Preview{}
	start := strings.Index(page, `id="highlight_player_area"`)
	if start < 0 {
		return out
	}
	area := page[start:]
	if i := strings.Index(area, `class="workshopItemDescription"`); i > 0 {
		area = area[:i]
	}
	for _, m := range rePageVideo.FindAllStringSubmatch(page, 40) {
		out = append(out, steamworks.Preview{Video: m[1]})
	}
	seen := map[string]bool{}
	for _, u := range rePageImage.FindAllString(area, 80) {
		if !seen[u] {
			seen[u] = true
			out = append(out, steamworks.Preview{URL: u})
		}
	}
	return out
}

var (
	reBBTag   = regexp.MustCompile(`\[/?[a-zA-Z0-9*]+(=[^\]]*)?\]`)
	reBBMedia = regexp.MustCompile(`(?is)\[(img|previewyoutube)[^\]]*\].*?\[/(img|previewyoutube)\]`)
	reSpaces  = regexp.MustCompile(`\s+`)
)

// summary saca el principio de una descripción del Workshop (que viene con
// BBCode), en texto plano, para enseñarlo en la lista.
func summary(desc string) string {
	if desc == "" {
		return ""
	}
	if len(desc) > 4000 {
		desc = desc[:4000]
	}
	t := reBBMedia.ReplaceAllString(desc, " ")
	t = reBBTag.ReplaceAllString(t, " ")
	t = strings.TrimSpace(reSpaces.ReplaceAllString(t, " "))
	return clip(strings.ToValidUTF8(t, ""), 240)
}
