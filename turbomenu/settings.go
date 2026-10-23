package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/windows"
)

// Settings son las preferencias que se guardan entre usos.
type Settings struct {
	VTKey     string              `json:"vtKey"`     // clave gratuita de VirusTotal
	Recent    map[string][]string `json:"recent"`    // formatos usados por tipo ("video" → ["mp4","gif"])
	Prewarm   bool                `json:"prewarm"`   // dejar TurboMENU preparado al iniciar sesión
	Modern    bool                `json:"modern"`    // también en el menú nuevo de Windows 11
	IdleMins  int                 `json:"idleMins"`  // minutos que se queda preparado tras cerrar una ventana
	LastVideo string              `json:"lastVideo"` // última opción de «Reducir» para vídeo
	LastImage int                 `json:"lastImage"` // última calidad de imagen
}

var (
	setMu    sync.Mutex
	settings = loadSettings()
)

func defaultSettings() Settings {
	return Settings{Recent: map[string][]string{}, IdleMins: 15}
}

// dataDir: %APPDATA%\TurboMENU (ajustes).
func dataDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	d := filepath.Join(base, "TurboMENU")
	os.MkdirAll(d, 0o755)
	return d
}

// cacheDir: %LOCALAPPDATA%\TurboMENU (motores descargados, caché del navegador interno).
func cacheDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	d := filepath.Join(base, "TurboMENU")
	os.MkdirAll(d, 0o755)
	return d
}

func binDir() string {
	d := filepath.Join(cacheDir(), "bin")
	os.MkdirAll(d, 0o755)
	return d
}

func downloadsDir() string {
	if p, err := windows.KnownFolderPath(windows.FOLDERID_Downloads, 0); err == nil && p != "" {
		return p
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Downloads")
}

func loadSettings() Settings {
	s := defaultSettings()
	if b, err := os.ReadFile(filepath.Join(dataDir(), "settings.json")); err == nil {
		json.Unmarshal(b, &s)
	}
	if s.Recent == nil {
		s.Recent = map[string][]string{}
	}
	if s.IdleMins <= 0 || s.IdleMins > 240 {
		s.IdleMins = 15
	}
	return s
}

func getSettings() Settings {
	setMu.Lock()
	defer setMu.Unlock()
	return settings
}

// updateSettings aplica f y guarda en disco de forma atómica.
func updateSettings(f func(s *Settings)) error {
	setMu.Lock()
	// Releer por si otra ventana (otro proceso) guardó algo mientras tanto.
	settings = loadSettings()
	f(&settings)
	b, _ := json.MarshalIndent(settings, "", "  ")
	setMu.Unlock()
	p := filepath.Join(dataDir(), "settings.json")
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// rememberFormat sube un formato al principio de los recientes de ese tipo.
func rememberFormat(kind, id string) {
	updateSettings(func(s *Settings) {
		l := []string{id}
		for _, v := range s.Recent[kind] {
			if !strings.EqualFold(v, id) && len(l) < 4 {
				l = append(l, v)
			}
		}
		s.Recent[kind] = l
	})
}
