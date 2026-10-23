package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings son los ajustes (en %APPDATA%\TurboGAMES\ajustes.json).
type Settings struct {
	SteamDir    string `json:"steamDir"`    // vacío = buscarlo solo
	StartSilent bool   `json:"startSilent"` // abrir Steam minimizado en la bandeja
	LockMinutes int    `json:"lockMinutes"` // cerrar la bóveda tras estos minutos sin usarla
	ClipSeconds int    `json:"clipSeconds"` // borrar del portapapeles lo copiado de la bóveda
	AfterSwitch string `json:"afterSwitch"` // "nada" o "minimizar"
}

func defaultSettings() Settings {
	return Settings{LockMinutes: 5, ClipSeconds: 20, AfterSwitch: "nada"}
}

func (s *Settings) normalize() {
	switch s.LockMinutes {
	case 1, 5, 15, 60:
	default:
		s.LockMinutes = 5
	}
	switch s.ClipSeconds {
	case 10, 20, 45, 90:
	default:
		s.ClipSeconds = 20
	}
	if s.AfterSwitch != "minimizar" {
		s.AfterSwitch = "nada"
	}
}

func dataDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	d := filepath.Join(base, "TurboGAMES")
	_ = os.MkdirAll(d, 0o755)
	return d
}

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
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
