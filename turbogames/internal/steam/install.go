// Package steam lee y cambia lo que Steam guarda en este PC: cuentas, sesiones
// guardadas, bibliotecas y juegos. No usa internet ni la API de Steam.
package steam

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const steamKey = `Software\Valve\Steam`

// Install es una instalación de Steam.
type Install struct {
	Dir string // C:\Program Files (x86)\Steam
	Exe string // ...\steam.exe

	// BackupDir: dónde guardar una copia de loginusers.vdf y local.vdf antes
	// de cambiarlos. Vacío = sin copias.
	BackupDir string
}

// ErrNotFound: Steam no está instalado (o no se encuentra).
var ErrNotFound = errors.New("no se encuentra Steam en este PC")

// Locate busca Steam: primero la carpeta indicada (si hay), luego el registro
// del usuario, luego el de la máquina y por último la ruta de siempre.
func Locate(override string) (*Install, error) {
	var cands []string
	if override != "" {
		cands = append(cands, override)
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, steamKey, registry.QUERY_VALUE); err == nil {
		if p, _, err := k.GetStringValue("SteamPath"); err == nil {
			cands = append(cands, p)
		}
		if p, _, err := k.GetStringValue("SteamExe"); err == nil {
			cands = append(cands, filepath.Dir(p))
		}
		k.Close()
	}
	for _, sub := range []string{`SOFTWARE\WOW6432Node\Valve\Steam`, `SOFTWARE\Valve\Steam`} {
		if k, err := registry.OpenKey(registry.LOCAL_MACHINE, sub, registry.QUERY_VALUE); err == nil {
			if p, _, err := k.GetStringValue("InstallPath"); err == nil {
				cands = append(cands, p)
			}
			k.Close()
		}
	}
	if pf := os.Getenv("ProgramFiles(x86)"); pf != "" {
		cands = append(cands, filepath.Join(pf, "Steam"))
	}
	cands = append(cands, `C:\Program Files (x86)\Steam`, `C:\Program Files\Steam`)
	for _, c := range cands {
		dir := filepath.Clean(filepath.FromSlash(c))
		exe := filepath.Join(dir, "steam.exe")
		if st, err := os.Stat(exe); err == nil && !st.IsDir() {
			return &Install{Dir: fixCase(dir), Exe: fixCase(exe)}, nil
		}
	}
	return nil, ErrNotFound
}

// fixCase devuelve la ruta con las mayúsculas reales (el registro la guarda en
// minúsculas: "c:/program files (x86)/steam").
func fixCase(p string) string {
	vol := filepath.VolumeName(p)
	parts := strings.Split(strings.TrimPrefix(p[len(vol):], `\`), `\`)
	cur := strings.ToUpper(vol) + `\`
	for _, part := range parts {
		if part == "" {
			continue
		}
		ents, err := os.ReadDir(cur)
		if err != nil {
			return p
		}
		name := part
		for _, e := range ents {
			if strings.EqualFold(e.Name(), part) {
				name = e.Name()
				break
			}
		}
		cur = filepath.Join(cur, name)
	}
	return cur
}

// Path une rutas dentro de la carpeta de Steam.
func (in *Install) Path(parts ...string) string {
	return filepath.Join(append([]string{in.Dir}, parts...)...)
}

// LocalVDF es el archivo donde Steam guarda las sesiones (cifradas) de cada cuenta.
func LocalVDF() string { return localVDFHook() }

var localVDFHook = defaultLocalVDF

func defaultLocalVDF() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		if d, err := os.UserCacheDir(); err == nil {
			base = d
		}
	}
	return filepath.Join(base, "Steam", "local.vdf")
}

// ---------- estado ----------

// Status es lo que está pasando ahora mismo con Steam.
type Status struct {
	Running    bool   `json:"running"`
	PID        uint32 `json:"pid"`
	ActiveUser uint32 `json:"activeUser"` // ID de cuenta (32 bits) con la sesión abierta; 0 = ninguna
	RunningApp uint32 `json:"runningApp"` // juego abierto desde Steam; 0 = ninguno
	AutoLogin  string `json:"autoLogin"`  // cuenta con la que entrará la próxima vez
}

// ReadStatus mira los procesos y el registro.
func (in *Install) ReadStatus() Status {
	var s Status
	for _, p := range processes() {
		if strings.EqualFold(p.name, "steam.exe") {
			s.Running, s.PID = true, p.pid
			break
		}
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, steamKey, registry.QUERY_VALUE); err == nil {
		s.AutoLogin, _, _ = k.GetStringValue("AutoLoginUser")
		if v, _, err := k.GetIntegerValue("RunningAppID"); err == nil {
			s.RunningApp = uint32(v)
		}
		k.Close()
	}
	if s.Running {
		if k, err := registry.OpenKey(registry.CURRENT_USER, steamKey+`\ActiveProcess`, registry.QUERY_VALUE); err == nil {
			if v, _, err := k.GetIntegerValue("ActiveUser"); err == nil {
				s.ActiveUser = uint32(v)
			}
			k.Close()
		}
	} else {
		s.RunningApp = 0
	}
	return s
}

// ---------- identificadores ----------

const steamID64Base = 76561197960265728

// AccountID pasa un SteamID64 ("7656119…") al ID de cuenta de 32 bits que usa
// la carpeta userdata.
func AccountID(steamID64 string) uint32 {
	var v uint64
	for _, c := range steamID64 {
		if c < '0' || c > '9' {
			return 0
		}
		v = v*10 + uint64(c-'0')
	}
	if v < steamID64Base {
		return 0
	}
	return uint32(v - steamID64Base)
}

// SteamID64 hace lo contrario que AccountID.
func SteamID64(accountID uint32) string {
	return uitoa(uint64(accountID) + steamID64Base)
}

func uitoa(v uint64) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}
