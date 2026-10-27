package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// testRoot: en las pruebas, ajustes y caché van a una carpeta temporal.
var testRoot string

// dataDir guarda los ajustes y el certificado (%APPDATA%\TurboCAM).
func dataDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	if testRoot != "" {
		base = filepath.Join(testRoot, "config")
	}
	d := filepath.Join(base, "TurboCAM")
	os.MkdirAll(d, 0o755)
	return d
}

// cacheDir guarda lo que se puede regenerar: la cámara virtual, el navegador
// interno y cloudflared (%LOCALAPPDATA%\TurboCAM).
func cacheDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	if testRoot != "" {
		base = filepath.Join(testRoot, "cache")
	}
	d := filepath.Join(base, "TurboCAM")
	os.MkdirAll(d, 0o755)
	return d
}

func binDir() string { return filepath.Join(cacheDir(), "bin") }

func loadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func saveJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Adjust son los retoques de imagen de un móvil. Los aplica el propio móvil al
// dibujar cada fotograma; el PC solo los guarda y se los manda.
type Adjust struct {
	Facing     string  `json:"facing"`     // user | environment
	Lens       string  `json:"lens"`       // deviceId de una cámara concreta ("" = la de Facing)
	Zoom       float64 `json:"zoom"`       // 1..5
	PanX       float64 `json:"panX"`       // -1..1, solo con zoom
	PanY       float64 `json:"panY"`       // -1..1
	Brightness float64 `json:"brightness"` // 0.5..1.5
	Contrast   float64 `json:"contrast"`   // 0.5..1.5
	Saturation float64 `json:"saturation"` // 0..2
	Mirror     bool    `json:"mirror"`
	Rotate     int     `json:"rotate"` // 0, 90, 180, 270
	Fit        string  `json:"fit"`    // cover | contain
	Torch      bool    `json:"torch"`
	Res        int     `json:"res"` // alto máximo que manda el móvil (0 = el de la salida)
	FPS        int     `json:"fps"` // fotogramas por segundo del móvil (0 = los de ajustes)
}

func defaultAdjust() Adjust {
	return Adjust{Facing: "user", Zoom: 1, Brightness: 1, Contrast: 1, Saturation: 1, Mirror: false, Fit: "cover"}
}

func (a Adjust) normalized() Adjust {
	clamp := func(v, lo, hi, def float64) float64 {
		if v == 0 && def != 0 {
			return def
		}
		if v < lo {
			return lo
		}
		if v > hi {
			return hi
		}
		return v
	}
	if a.Facing != "environment" {
		a.Facing = "user"
	}
	a.Zoom = clamp(a.Zoom, 1, 5, 1)
	a.PanX = clamp(a.PanX, -1, 1, 0)
	a.PanY = clamp(a.PanY, -1, 1, 0)
	a.Brightness = clamp(a.Brightness, .5, 1.5, 1)
	a.Contrast = clamp(a.Contrast, .5, 1.5, 1)
	if a.Saturation < 0 || a.Saturation > 2 {
		a.Saturation = 1
	}
	switch a.Rotate {
	case 0, 90, 180, 270:
	default:
		a.Rotate = 0
	}
	if a.Fit != "contain" {
		a.Fit = "cover"
	}
	switch a.Res {
	case 0, 1080, 720, 540, 480, 360, 240:
	default:
		a.Res = 0
	}
	switch a.FPS {
	case 0, 10, 15, 20, 24, 30:
	default:
		a.FPS = 0
	}
	if len(a.Lens) > 200 {
		a.Lens = ""
	}
	return a
}

// Region es un trozo de la pantalla, en píxeles físicos del escritorio virtual.
type Region struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// Scene dice qué sale por la cámara.
//
//	phone   un móvil (Main)
//	screen  una región de la pantalla
//	pip     Main de fondo y Pip encima, en una esquina
type Scene struct {
	Kind      string  `json:"kind"`
	Main      string  `json:"main"` // id de móvil o "screen"
	Pip       string  `json:"pip"`  // id de móvil o "screen"
	PipCorner string  `json:"pipCorner"`
	PipSize   float64 `json:"pipSize"` // fracción del ancho, 0.15..0.5
	Region    Region  `json:"region"`
	Cursor    bool    `json:"cursor"`
}

func (s Scene) normalized() Scene {
	switch s.Kind {
	case "screen", "pip":
	default:
		s.Kind = "phone"
	}
	switch s.PipCorner {
	case "tl", "tr", "bl", "br":
	default:
		s.PipCorner = "br"
	}
	if s.PipSize < .15 || s.PipSize > .5 {
		s.PipSize = .3
	}
	if s.Region.W < 64 || s.Region.H < 64 {
		s.Region = Region{}
	}
	return s
}

// uses dice si la escena necesita esa fuente.
func (s Scene) uses(id string) bool {
	switch s.Kind {
	case "screen":
		return id == "screen"
	case "pip":
		return s.Main == id || s.Pip == id
	}
	return s.Main == id
}

// Profile guarda una escena y los retoques del móvil principal, y las apps que lo
// activan solas al abrir la cámara.
type Profile struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Scene  Scene    `json:"scene"`
	Adjust Adjust   `json:"adjust"`
	Apps   []string `json:"apps"` // ejecutables, en minúsculas
}

// Device es lo que se recuerda de cada móvil.
type Device struct {
	Name   string `json:"name"`
	Model  string `json:"model"` // para reconocerlo si entra por otro enlace
	Adjust Adjust `json:"adjust"`
}

type Hotkeys struct {
	Photo  string `json:"photo"`
	Freeze string `json:"freeze"`
	Cover  string `json:"cover"`
}

type Settings struct {
	Token      string             `json:"token"` // secreto del enlace del móvil
	Port       int                `json:"port"`
	Resolution string             `json:"resolution"` // auto | 1080 | 720 | 540 | 480 | 360
	FPS        int                `json:"fps"`        // 15 | 24 | 30
	UPnP       bool               `json:"upnp"`
	CFToken    string             `json:"cfToken"` // túnel con dominio propio (opcional)
	CFHost     string             `json:"cfHost"`
	Link       string             `json:"link"` // internet | direct | lan: qué QR se enseña
	Scene      Scene              `json:"scene"`
	Profiles   []Profile          `json:"profiles"`
	Devices    map[string]*Device `json:"devices"`
	Hotkeys    Hotkeys            `json:"hotkeys"`
	PhotoDir   string             `json:"photoDir"`
	Idle       bool               `json:"idle"`     // pedir menos fotogramas al móvil si nadie mira
	OnTop      bool               `json:"onTop"`    // modo ventana: siempre encima
	Welcomed   bool               `json:"welcomed"` // ya vio la explicación inicial
}

func defaultSettings() Settings {
	return Settings{
		Resolution: "auto",
		FPS:        30,
		UPnP:       true,
		Link:       "internet",
		Scene:      Scene{Kind: "phone"}.normalized(),
		Devices:    map[string]*Device{},
		Hotkeys:    Hotkeys{Photo: "Ctrl+Alt+P", Freeze: "Ctrl+Alt+F", Cover: "Ctrl+Alt+T"},
		Idle:       true,
		OnTop:      true,
	}
}

var (
	settingsMu sync.Mutex
	settings   *Settings
)

func settingsPath() string { return filepath.Join(dataDir(), "settings.json") }

// getSettings devuelve una copia de los ajustes (se cargan la primera vez).
func getSettings() Settings {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	if settings == nil {
		s := defaultSettings()
		loadJSON(settingsPath(), &s)
		s = s.normalized()
		settings = &s
		saveJSON(settingsPath(), settings) // guarda el secreto nuevo si no lo había
	}
	return settings.clone()
}

// updateSettings cambia los ajustes y los guarda.
func updateSettings(f func(s *Settings)) Settings {
	getSettings()
	settingsMu.Lock()
	defer settingsMu.Unlock()
	f(settings)
	n := settings.normalized()
	*settings = n
	saveJSON(settingsPath(), settings)
	return settings.clone()
}

func (s Settings) clone() Settings {
	c := s
	c.Devices = make(map[string]*Device, len(s.Devices))
	for k, v := range s.Devices {
		d := *v
		c.Devices[k] = &d
	}
	c.Profiles = make([]Profile, len(s.Profiles))
	for i, p := range s.Profiles {
		p.Apps = append([]string(nil), p.Apps...)
		c.Profiles[i] = p
	}
	return c
}

func (s Settings) normalized() Settings {
	if len(s.Token) < 32 {
		s.Token = randHex(16)
	}
	switch s.Resolution {
	case "1080", "720", "540", "480", "360":
	default:
		s.Resolution = "auto"
	}
	switch s.FPS {
	case 15, 24, 30:
	default:
		s.FPS = 30
	}
	switch s.Link {
	case "direct", "lan":
	default:
		s.Link = "internet"
	}
	s.CFHost = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(s.CFHost, "https://"), "http://"), "/"))
	s.CFToken = strings.TrimSpace(s.CFToken)
	s.Scene = s.Scene.normalized()
	if s.Devices == nil {
		s.Devices = map[string]*Device{}
	}
	for k, d := range s.Devices {
		if d == nil || k == "" || len(k) > 40 {
			delete(s.Devices, k)
			continue
		}
		d.Adjust = d.Adjust.normalized()
	}
	seen := map[string]bool{}
	var profiles []Profile
	for _, p := range s.Profiles {
		if p.ID == "" || seen[p.ID] {
			continue
		}
		seen[p.ID] = true
		p.Name = strings.TrimSpace(p.Name)
		if p.Name == "" {
			p.Name = "Perfil"
		}
		p.Scene = p.Scene.normalized()
		p.Adjust = p.Adjust.normalized()
		var apps []string
		for _, a := range p.Apps {
			if a = strings.ToLower(strings.TrimSpace(a)); a != "" {
				apps = append(apps, a)
			}
		}
		p.Apps = apps
		profiles = append(profiles, p)
	}
	s.Profiles = profiles
	return s
}

func photoDir() string {
	if d := getSettings().PhotoDir; d != "" {
		return d
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "Pictures", "TurboCAM")
	}
	return filepath.Join(dataDir(), "Fotos")
}
