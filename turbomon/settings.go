package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// dataDir: %LOCALAPPDATA%\TurboMON (ajustes, partidas, registro). TURBOMON_DIR lo cambia (pruebas).
func dataDir() string {
	if d := os.Getenv("TURBOMON_DIR"); d != "" {
		return d
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "TurboMON")
}

// OverlaySettings: qué enseña el overlay y dónde.
type OverlaySettings struct {
	On       bool   `json:"on"`
	FPSOnly  bool   `json:"fpsOnly"`  // solo el contador de FPS
	Corner   string `json:"corner"`   // ti, td, bi, bd (arriba/abajo, izquierda/derecha)
	Scale    int    `json:"scale"`    // % (75-200)
	Opacity  int    `json:"opacity"`  // % (40-100)
	GameOnly bool   `json:"gameOnly"` // solo con un juego delante
	Low1     bool   `json:"low1"`
	FrameMs  bool   `json:"frameMs"`
	CPU      bool   `json:"cpu"`
	CPUTemp  bool   `json:"cpuTemp"`
	GPU      bool   `json:"gpu"`
	GPUTemp  bool   `json:"gpuTemp"`
	RAM      bool   `json:"ram"`
	VRAM     bool   `json:"vram"`
	Clock    bool   `json:"clock"`
	Power    bool   `json:"power"`
	Ping     bool   `json:"ping"`
	Hotkey   bool   `json:"hotkey"` // Ctrl+Alt+O
}

// Settings son los ajustes de TurboMON.
type Settings struct {
	Overlay      OverlaySettings `json:"overlay"`
	Sessions     bool            `json:"sessions"`     // guardar las partidas
	TurboMode    bool            `json:"turboMode"`    // modo juego Turbo
	TurboPlan    bool            `json:"turboPlan"`    // · plan de energía al máximo durante la partida
	TurboPrio    bool            `json:"turboPrio"`    // · más prioridad para el juego
	TurboStandby bool            `json:"turboStandby"` // · soltar la memoria en espera si queda poca
	WifiNoScan   bool            `json:"wifiNoScan"`   // apagar la búsqueda de redes wifi mientras se juega
	Governor     bool            `json:"governor"`     // ahorro inteligente: bajar la frecuencia si sobra (experimental)
	Welcomed     bool            `json:"welcomed"`
}

func defaultSettings() Settings {
	return Settings{
		Overlay: OverlaySettings{
			Corner: "ti", Scale: 100, Opacity: 88, Low1: true, CPU: true, CPUTemp: true,
			GPU: true, GPUTemp: true, RAM: true, VRAM: true, Hotkey: true,
		},
		Sessions: true, TurboPlan: true, TurboPrio: true, TurboStandby: true,
	}
}

var (
	setMu sync.Mutex
	set   *Settings
)

func settingsPath() string { return filepath.Join(dataDir(), "ajustes.json") }

func getSettings() Settings {
	setMu.Lock()
	defer setMu.Unlock()
	if set == nil {
		s := defaultSettings()
		if b, err := os.ReadFile(settingsPath()); err == nil {
			json.Unmarshal(b, &s)
		}
		s.normalize()
		set = &s
	}
	return *set
}

func (s *Settings) normalize() {
	o := &s.Overlay
	switch o.Corner {
	case "ti", "td", "bi", "bd":
	default:
		o.Corner = "ti"
	}
	if o.Scale < 75 || o.Scale > 200 {
		o.Scale = 100
	}
	if o.Opacity < 40 || o.Opacity > 100 {
		o.Opacity = 88
	}
}

func saveSettings(s Settings) error {
	s.normalize()
	setMu.Lock()
	set = &s
	setMu.Unlock()
	os.MkdirAll(dataDir(), 0o755)
	b, _ := json.MarshalIndent(s, "", "  ")
	tmp := settingsPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, settingsPath())
}

func updateSettings(f func(*Settings)) Settings {
	s := getSettings()
	f(&s)
	saveSettings(s)
	return getSettings()
}
