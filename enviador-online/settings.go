package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings son las preferencias persistentes de la app.
type Settings struct {
	Port               int  `json:"port"`
	Tunnel             bool `json:"tunnel"`
	UPnP               bool `json:"upnp"`
	ShowDirectOnTunnel bool `json:"showDirectOnTunnel"`
	ForceDirect        bool `json:"forceDirect"`
	Connections        int  `json:"connections"`
}

func defaultSettings() Settings {
	return Settings{
		Port:               47821,
		Tunnel:             true,
		UPnP:               true,
		ShowDirectOnTunnel: true,
		ForceDirect:        false,
		Connections:        4,
	}
}

func (s *Settings) normalize() {
	if s.Port < 1024 || s.Port > 65535 {
		s.Port = 47821
	}
	if s.Connections < 1 {
		s.Connections = 1
	}
	if s.Connections > 16 {
		s.Connections = 16
	}
}

// dataDir devuelve %APPDATA%\Enviador, creándolo si hace falta.
func dataDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "Enviador")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

func loadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// saveJSON escribe de forma atómica (archivo temporal + rename).
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
