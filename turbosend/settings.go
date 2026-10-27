package main

import (
	"encoding/json"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"unicode"

	"golang.org/x/sys/windows"
)

// Settings son las preferencias persistentes de la app.
type Settings struct {
	HostName           string  `json:"hostName"`
	Port               int     `json:"port"`
	Tunnel             bool    `json:"tunnel"`
	UPnP               bool    `json:"upnp"`
	ShowDirectOnTunnel bool    `json:"showDirectOnTunnel"`
	ForceDirect        bool    `json:"forceDirect"`
	Connections        int     `json:"connections"`
	RecvDir            string  `json:"recvDir"`
	AutoAccept         bool    `json:"autoAccept"`
	AutoLimitGB        float64 `json:"autoLimitGB"` // 0 = sin límite
	Sounds             bool    `json:"sounds"`
	// LANLink: enlace para la misma red (Wi-Fi de casa). Sin él, ni UPnP ni directo
	// forzado, TurboSEND solo escucha en este PC (el túnel de Cloudflare llega por ahí).
	LANLink bool `json:"lanLink"`
	// Dominio propio (túnel con nombre de Cloudflare). Vacío = túnel temporal.
	TunnelToken string `json:"tunnelToken"`
	TunnelHost  string `json:"tunnelHost"`
	// Rev: versión de los ajustes guardados, para ponerlos al día (ver migrate).
	Rev int `json:"rev"`
}

const settingsRev = 1

// migrate pone al día los ajustes guardados por versiones anteriores.
func (s *Settings) migrate() {
	if s.Rev < 1 {
		// Hasta la 2.1.1 se ofrecía por defecto el enlace directo a quien entraba por
		// Cloudflare, y ese enlace lleva tu IP pública. Ahora se activa a propósito.
		s.ShowDirectOnTunnel = false
	}
	s.Rev = settingsRev
}

// needsNetwork: hay que escuchar en todas las redes (directo o red local). Si no,
// basta con este PC, que es por donde llega el túnel de Cloudflare.
func (s Settings) needsNetwork() bool { return s.UPnP || s.ForceDirect || s.LANLink }

func defaultSettings() Settings {
	return Settings{
		HostName:           defaultHostName(),
		Port:               47831,
		Tunnel:             true,
		UPnP:               true,
		ShowDirectOnTunnel: false,
		LANLink:            true,
		Connections:        4,
		RecvDir:            filepath.Join(downloadsDir(), "TurboSEND"),
		Sounds:             true,
	}
}

func (s *Settings) normalize() {
	if s.Port < 1024 || s.Port > 65535 {
		s.Port = 47831
	}
	if s.Connections < 1 {
		s.Connections = 1
	}
	if s.Connections > 16 {
		s.Connections = 16
	}
	s.HostName = strings.TrimSpace(s.HostName)
	if s.HostName == "" {
		s.HostName = defaultHostName()
	}
	if len([]rune(s.HostName)) > 40 {
		s.HostName = string([]rune(s.HostName)[:40])
	}
	if strings.TrimSpace(s.RecvDir) == "" {
		s.RecvDir = filepath.Join(downloadsDir(), "TurboSEND")
	}
	if s.AutoLimitGB < 0 {
		s.AutoLimitGB = 0
	}
	s.TunnelToken = strings.TrimSpace(s.TunnelToken)
	h := strings.TrimSpace(s.TunnelHost)
	h = strings.TrimPrefix(strings.TrimPrefix(h, "https://"), "http://")
	s.TunnelHost = strings.TrimRight(h, "/")
}

func defaultHostName() string {
	if u, err := user.Current(); err == nil {
		n := u.Username
		if i := strings.LastIndex(n, `\`); i >= 0 {
			n = n[i+1:]
		}
		if n != "" {
			r := []rune(n)
			r[0] = unicode.ToUpper(r[0])
			return string(r)
		}
	}
	return "Yo"
}

func downloadsDir() string {
	if p, err := windows.KnownFolderPath(windows.FOLDERID_Downloads, 0); err == nil && p != "" {
		return p
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Downloads")
}

// dataDir devuelve %APPDATA%\EnviadorV2, creándolo si hace falta.
func dataDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "EnviadorV2")
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
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
