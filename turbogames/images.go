package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"turbogames/internal/steam"
)

// imageServer sirve a la interfaz las fotos de perfil y las imágenes de los
// juegos. Primero mira lo que Steam ya tiene en su caché; si no está, lo baja
// de los servidores de imágenes de Steam una vez y lo guarda en
// %APPDATA%\TurboGAMES\cache\img.
//
//	/img/avatar/<steamid64>
//	/img/app/<appid>/<cover|header|hero|logo|icon>
type imageServer struct {
	app    *App
	dir    string
	client *http.Client
	sem    chan struct{}

	mu       sync.Mutex
	missing  map[string]time.Time // lo que no existe, para no pedirlo cada vez
	inflight map[string]*sync.WaitGroup
}

func newImageServer(a *App) *imageServer {
	return &imageServer{
		app: a,
		dir: filepath.Join(dataDir(), "cache", "img"),
		client: &http.Client{Timeout: 15 * time.Second,
			// Solo los servidores de imágenes de Steam, siempre por https.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 || req.URL.Scheme != "https" || !strings.HasSuffix(req.URL.Hostname(), ".steamstatic.com") {
					return http.ErrUseLastResponse
				}
				return nil
			}},
		sem:      make(chan struct{}, 6),
		missing:  map[string]time.Time{},
		inflight: map[string]*sync.WaitGroup{},
	}
}

var imgKinds = map[string]bool{"cover": true, "header": true, "hero": true, "logo": true, "icon": true}

func (s *imageServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 || parts[0] != "img" {
		http.NotFound(w, r)
		return
	}
	in := s.app.steam()
	switch {
	case parts[1] == "avatar" && len(parts) == 3:
		if in == nil || steam.AccountID(parts[2]) == 0 {
			http.NotFound(w, r)
			return
		}
		serveFile(w, r, in.AvatarPath(parts[2]), "max-age=300")
	case parts[1] == "app" && len(parts) == 4 && imgKinds[parts[3]]:
		id, err := strconv.ParseUint(parts[2], 10, 32)
		if err != nil || id == 0 {
			http.NotFound(w, r)
			return
		}
		p := s.find(in, uint32(id), parts[3])
		if p == "" {
			w.Header().Set("Cache-Control", "max-age=600")
			http.NotFound(w, r)
			return
		}
		serveFile(w, r, p, "max-age=86400")
	default:
		http.NotFound(w, r)
	}
}

func serveFile(w http.ResponseWriter, r *http.Request, p, cache string) {
	f, err := os.Open(p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	switch strings.ToLower(filepath.Ext(p)) {
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".webp":
		w.Header().Set("Content-Type", "image/webp")
	}
	w.Header().Set("Cache-Control", cache)
	io.Copy(w, f)
}

// find devuelve la ruta local de la imagen, bajándola si hace falta.
func (s *imageServer) find(in *steam.Install, appid uint32, kind string) string {
	var meta steam.AppMeta
	if in != nil {
		meta = in.AppInfo()[appid]
		if p := in.LocalImage(appid, kind, meta); p != "" {
			return p
		}
	}
	key := strconv.FormatUint(uint64(appid), 10) + "_" + kind
	for _, ext := range []string{".jpg", ".png"} {
		p := filepath.Join(s.dir, key+ext)
		if fi, err := os.Stat(p); err == nil && fi.Size() > 0 {
			return p
		}
	}

	s.mu.Lock()
	if t, ok := s.missing[key]; ok && time.Since(t) < 30*time.Minute {
		s.mu.Unlock()
		return ""
	}
	if wg, ok := s.inflight[key]; ok {
		s.mu.Unlock()
		wg.Wait()
		return s.cached(key)
	}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	s.inflight[key] = wg
	s.mu.Unlock()

	p := s.download(appid, kind, meta, key)

	s.mu.Lock()
	delete(s.inflight, key)
	if p == "" {
		s.missing[key] = time.Now()
	}
	s.mu.Unlock()
	wg.Done()
	return p
}

func (s *imageServer) cached(key string) string {
	for _, ext := range []string{".jpg", ".png"} {
		p := filepath.Join(s.dir, key+ext)
		if fi, err := os.Stat(p); err == nil && fi.Size() > 0 {
			return p
		}
	}
	return ""
}

func (s *imageServer) download(appid uint32, kind string, meta steam.AppMeta, key string) string {
	id := strconv.FormatUint(uint64(appid), 10)
	var urls []string
	if kind == "icon" {
		if meta.Icon == "" || !steam.SafeRel(meta.Icon) || strings.Contains(meta.Icon, "/") {
			return ""
		}
		urls = []string{"https://cdn.cloudflare.steamstatic.com/steamcommunity/public/images/apps/" + id + "/" + meta.Icon + ".jpg"}
	} else {
		for _, rel := range steam.AssetRel(meta, kind) {
			urls = append(urls,
				"https://shared.cloudflare.steamstatic.com/store_item_assets/steam/apps/"+id+"/"+rel,
				"https://cdn.cloudflare.steamstatic.com/steam/apps/"+id+"/"+rel)
		}
	}
	s.sem <- struct{}{}
	defer func() { <-s.sem }()
	for _, u := range urls {
		resp, err := s.client.Get(u)
		if err != nil {
			continue
		}
		ct := resp.Header.Get("Content-Type")
		if resp.StatusCode != 200 || !strings.HasPrefix(ct, "image/") {
			resp.Body.Close()
			continue
		}
		ext := ".jpg"
		if strings.Contains(ct, "png") {
			ext = ".png"
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 12<<20))
		resp.Body.Close()
		if err != nil || len(data) == 0 {
			continue
		}
		_ = os.MkdirAll(s.dir, 0o755)
		p := filepath.Join(s.dir, key+ext)
		tmp := p + ".tmp"
		if os.WriteFile(tmp, data, 0o644) == nil && os.Rename(tmp, p) == nil {
			return p
		}
		os.Remove(tmp)
	}
	return ""
}
