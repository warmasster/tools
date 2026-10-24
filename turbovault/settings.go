package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"turbovault/internal/gen"
)

// dataDir es la carpeta de TurboVAULT (%APPDATA%\TurboVAULT). Solo la puede
// abrir el usuario. TURBOVAULT_DIR la cambia (pruebas).
func dataDir() string {
	if d := testEnv("TURBOVAULT_DIR"); d != "" {
		return d
	}
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "TurboVAULT")
}

func vaultPath() string    { return filepath.Join(dataDir(), "boveda.tvault") }
func settingsPath() string { return filepath.Join(dataDir(), "ajustes.json") }

// localDir es para lo que no hace falta que viaje con el perfil (la extensión,
// la caché del navegador interno).
func localDir() string {
	if d := testEnv("TURBOVAULT_DIR"); d != "" {
		return filepath.Join(d, "local")
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "TurboVAULT")
}

// BackupTarget es una carpeta (normalmente en un disco externo o USB) donde se
// deja una copia cifrada cada vez que cambia la bóveda.
type BackupTarget struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	Label     string `json:"label"`  // nombre del disco
	Serial    uint32 `json:"serial"` // número de serie del volumen, para encontrarlo aunque cambie de letra
	Keep      int    `json:"keep"`   // cuántas copias guardar
	LastCopy  int64  `json:"lastCopy"`
	LastHash  string `json:"lastHash"`
	LastError string `json:"lastError,omitempty"`
}

// Settings son los ajustes que no son secretos (ajustes.json). Si alguien los
// toca a mano, al leerlos se recortan a valores seguros.
type Settings struct {
	SessionMin   int            `json:"sessionMin"`   // vida de la sesión desde que se abre
	ClipSeconds  int            `json:"clipSeconds"`  // borrar el portapapeles tras copiar
	NotifyFill   bool           `json:"notifyFill"`   // aviso en la bandeja al rellenar
	Autostart    bool           `json:"autostart"`    // arrancar con Windows
	HideCapture  bool           `json:"hideCapture"`  // ventana fuera de capturas
	Gen          gen.Options    `json:"gen"`          // últimas opciones del generador
	Backups      []BackupTarget `json:"backups"`      // copias automáticas
	Fails        int            `json:"fails"`        // intentos fallidos seguidos
	WaitUntil    int64          `json:"waitUntil"`    // no aceptar otro intento hasta (ms)
	BreachCheck  bool           `json:"breachCheck"`  // se ha aceptado consultar filtraciones
	LastBreachAt int64          `json:"lastBreachAt"` // última comprobación
	MaxRev       int64          `json:"maxRev"`       // versión más alta de la bóveda vista (avisa si vuelve atrás)
}

func defaultSettings() Settings {
	return Settings{SessionMin: 10, ClipSeconds: 20, Autostart: true, HideCapture: true, Gen: gen.Default()}
}

var allowedSessions = []int{1, 2, 5, 10, 15, 30, 60}

func (s *Settings) sanitize() {
	ok := false
	for _, v := range allowedSessions {
		if s.SessionMin == v {
			ok = true
		}
	}
	if !ok {
		s.SessionMin = 10
	}
	if s.ClipSeconds < 5 || s.ClipSeconds > 120 {
		s.ClipSeconds = 20
	}
	if s.Gen.Mode == "" {
		s.Gen = gen.Default()
	}
	if s.Fails < 0 {
		s.Fails = 0
	}
	for i := range s.Backups {
		if s.Backups[i].Keep < 1 || s.Backups[i].Keep > 100 {
			s.Backups[i].Keep = 10
		}
	}
}

var (
	settingsMu sync.Mutex
	settings   *Settings
)

func getSettings() Settings {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	if settings == nil {
		s := defaultSettings()
		if b, err := os.ReadFile(settingsPath()); err == nil {
			_ = json.Unmarshal(b, &s)
		}
		s.sanitize()
		settings = &s
	}
	c := *settings
	c.Backups = append([]BackupTarget(nil), settings.Backups...)
	return c
}

// updateSettings cambia los ajustes y los guarda.
func updateSettings(fn func(s *Settings)) Settings {
	getSettings()
	settingsMu.Lock()
	defer settingsMu.Unlock()
	fn(settings)
	settings.sanitize()
	if b, err := json.MarshalIndent(settings, "", "  "); err == nil {
		os.MkdirAll(dataDir(), 0o700)
		tmp := settingsPath() + ".tmp"
		if os.WriteFile(tmp, b, 0o600) == nil {
			os.Rename(tmp, settingsPath())
		}
	}
	c := *settings
	c.Backups = append([]BackupTarget(nil), settings.Backups...)
	return c
}
