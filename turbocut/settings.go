package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Settings struct {
	Measure     string `json:"measure"`     // "alloc" (en disco) o "size" (tamaño de los archivos)
	AlwaysAdmin bool   `json:"alwaysAdmin"` // abrir siempre en modo turbo
	History     bool   `json:"history"`     // guardar escaneos para comparar
	ShowHidden  bool   `json:"showHidden"`
	ShowDeltas  bool   `json:"showDeltas"` // flechas de crecido/bajado en la lista
	DeltaMin    int64  `json:"deltaMin"`   // MB: cambios más pequeños no se enseñan
}

func defaultSettings() Settings {
	return Settings{Measure: "alloc", History: true, ShowHidden: true, DeltaMin: 100}
}

func (s *Settings) normalize() {
	if s.Measure != "size" {
		s.Measure = "alloc"
	}
	if s.DeltaMin <= 0 {
		s.DeltaMin = 100
	}
}

func dataDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	d := filepath.Join(base, "TurboCUT")
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
