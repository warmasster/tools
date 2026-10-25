package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Todo se descarga de una única Release de GitHub: la de la etiqueta "apps".
// Dentro están TurboSETUP.exe, el .exe de cada herramienta y manifest.json,
// que genera GitHub Actions (.github/scripts/release.ps1).
const (
	repo       = "warmasster/tools"
	releaseTag = "apps"
)

// downloadBase se puede cambiar con TURBOSETUP_BASE para probar con un servidor local.
func downloadBase() string {
	if b := strings.TrimRight(os.Getenv("TURBOSETUP_BASE"), "/"); b != "" {
		return b
	}
	return "https://github.com/" + repo + "/releases/download/" + releaseTag
}

type Asset struct {
	Version string `json:"version"`
	Asset   string `json:"asset"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
}

type Tool struct {
	Asset
	ID      string `json:"id"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
	// "app": se instala y se abre (acceso directo en el menú Inicio).
	// "setup": el .exe es su propio instalador (TurboDL); se ejecuta con "setup".
	Kind string `json:"kind"`
	// Open: una herramienta "setup" que además tiene ventana (TurboVAULT,
	// TurboMENU): se puede abrir desde aquí.
	Open bool   `json:"open,omitempty"`
	Dir  string `json:"dir"`
}

type Manifest struct {
	Updated string `json:"updated"`
	Setup   Asset  `json:"setup"`
	Tools   []Tool `json:"tools"`
}

func (m *Manifest) find(id string) *Tool {
	if m == nil {
		return nil
	}
	for i := range m.Tools {
		if m.Tools[i].ID == id {
			return &m.Tools[i]
		}
	}
	return nil
}

var httpClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	},
}

func httpGet(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "TurboSETUP/"+version)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, fmt.Errorf("GitHub respondió %s", resp.Status)
	}
	return resp, nil
}

func fetchManifest() (*Manifest, error) {
	resp, err := httpGet(downloadBase() + "/manifest.json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("el catálogo no es válido: %w", err)
	}
	return &m, nil
}

// newer dice si la versión a es más nueva que b ("1.10.0" > "1.9.2").
func newer(a, b string) bool {
	pa, pb := strings.Split(strings.TrimPrefix(a, "v"), "."), strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x > y
		}
	}
	return false
}

// ---------- lo que ve la ventana ----------

type ToolView struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Summary          string `json:"summary"`
	Kind             string `json:"kind"`
	Open             bool   `json:"open"`
	Version          string `json:"version"`
	Size             int64  `json:"size"`
	Available        bool   `json:"available"`
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installedVersion"`
	Update           bool   `json:"update"`
	URL              string `json:"url"`
}

type CatalogView struct {
	Tools       []ToolView `json:"tools"`
	Error       string     `json:"error"`
	Version     string     `json:"version"`
	SetupLatest string     `json:"setupLatest"`
	SetupUpdate bool       `json:"setupUpdate"`
	Root        string     `json:"root"`
	Repo        string     `json:"repo"`
}

func buildView(m *Manifest, fetchErr error) *CatalogView {
	v := &CatalogView{Version: version, Root: rootDir(), Repo: "https://github.com/" + repo}
	if fetchErr != nil {
		v.Error = fetchErr.Error()
	}
	installed := listInstalled()
	seen := map[string]bool{}
	if m != nil {
		v.SetupLatest = m.Setup.Version
		v.SetupUpdate = m.Setup.Asset != "" && newer(m.Setup.Version, version)
		for _, t := range m.Tools {
			tv := ToolView{
				ID: t.ID, Name: t.Name, Summary: t.Summary, Kind: t.Kind, Open: t.Open,
				Version: t.Version, Size: t.Size, Available: true,
				URL: "https://github.com/" + repo + "/tree/main/" + t.Dir,
			}
			if in, ok := installed[t.ID]; ok {
				tv.Installed, tv.InstalledVersion = true, in.Version
				tv.Update = newer(t.Version, in.Version)
			}
			seen[t.ID] = true
			v.Tools = append(v.Tools, tv)
		}
	}
	// Instaladas que ya no están en el catálogo (o sin conexión): que se puedan abrir y quitar.
	for id, in := range installed {
		if !seen[id] {
			v.Tools = append(v.Tools, ToolView{
				ID: id, Name: in.Name, Kind: in.Kind, Installed: true, InstalledVersion: in.Version,
				URL: "https://github.com/" + repo,
			})
		}
	}
	return v
}
