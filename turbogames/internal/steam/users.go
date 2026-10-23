package steam

import (
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"turbogames/internal/vdf"
)

// LoginUser es una cuenta que Steam recuerda en config\loginusers.vdf.
type LoginUser struct {
	SteamID      string `json:"steamId"`
	AccountName  string `json:"accountName"`
	PersonaName  string `json:"personaName"`
	Remember     bool   `json:"remember"`
	AutoLogin    bool   `json:"autoLogin"`
	WantsOffline bool   `json:"wantsOffline"`
	Timestamp    int64  `json:"timestamp"`
	HasToken     bool   `json:"hasToken"` // hay una sesión guardada: entra sin contraseña
}

// LoginUsersPath es config\loginusers.vdf.
func (in *Install) LoginUsersPath() string { return in.Path("config", "loginusers.vdf") }

// AvatarPath es la foto de perfil que Steam guarda de cada cuenta que ha entrado.
func (in *Install) AvatarPath(steamID string) string {
	return in.Path("config", "avatarcache", steamID+".png")
}

// LoginUsers lee las cuentas que recuerda Steam, con la más reciente primero.
func (in *Install) LoginUsers() ([]LoginUser, error) {
	root, err := vdf.ParseFile(in.LoginUsersPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	tokens := TokenKeys()
	var out []LoginUser
	for _, u := range root.Get("users").Kids() {
		if !u.IsObject() || AccountID(u.Key) == 0 {
			continue
		}
		lu := LoginUser{
			SteamID:      u.Key,
			AccountName:  u.Str("AccountName"),
			PersonaName:  u.Str("PersonaName"),
			Remember:     u.Str("RememberPassword") == "1",
			AutoLogin:    u.Str("AutoLogin") == "1" || u.Str("MostRecent") == "1",
			WantsOffline: u.Str("WantsOfflineMode") == "1",
		}
		lu.Timestamp, _ = strconv.ParseInt(u.Str("Timestamp"), 10, 64)
		if lu.AccountName == "" {
			continue
		}
		lu.HasToken = HasToken(tokens, lu.AccountName)
		out = append(out, lu)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Timestamp > out[j].Timestamp })
	return out, nil
}

// ---------- sesiones guardadas ----------

// Steam guarda la sesión de cada cuenta en local.vdf, dentro de
// MachineUserConfigStore\Software\Valve\Steam\ConnectCache, con la clave
// crc32(nombre de la cuenta en minúsculas) en hexadecimal seguido de "1".
// El valor va cifrado y no lo tocamos; solo miramos si está.
var connectCachePath = []string{"MachineUserConfigStore", "Software", "Valve", "Steam", "ConnectCache"}

// tokenKeyVariants son las formas en que puede aparecer la clave de una cuenta.
func tokenKeyVariants(account string) []string {
	var out []string
	for _, name := range []string{strings.ToLower(account), account} {
		c := crc32.ChecksumIEEE([]byte(name))
		out = append(out, fmt.Sprintf("%x1", c), fmt.Sprintf("%08x1", c))
	}
	return out
}

// TokenKeys lee las claves de ConnectCache (en minúsculas).
func TokenKeys() map[string]bool {
	keys := map[string]bool{}
	root, err := vdf.ParseFile(LocalVDF())
	if err != nil {
		return keys
	}
	for _, c := range root.Path(connectCachePath...).Kids() {
		if c.Value != "" {
			keys[strings.ToLower(c.Key)] = true
		}
	}
	return keys
}

// HasToken dice si la cuenta tiene una sesión guardada.
func HasToken(keys map[string]bool, account string) bool {
	for _, k := range tokenKeyVariants(account) {
		if keys[k] {
			return true
		}
	}
	return false
}

// ---------- copias de seguridad ----------

// backup copia un archivo a la carpeta de copias antes de tocarlo. Guarda las
// 10 últimas de cada archivo.
func (in *Install) backup(path string) {
	if in.BackupDir == "" {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	_ = os.MkdirAll(in.BackupDir, 0o755)
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, filepath.Ext(base)) + "-" + time.Now().Format("20060102-150405") + filepath.Ext(base)
	_ = os.WriteFile(filepath.Join(in.BackupDir, name), b, 0o644)
	prefix := strings.TrimSuffix(base, filepath.Ext(base)) + "-"
	ents, _ := os.ReadDir(in.BackupDir)
	var mine []string
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), prefix) {
			mine = append(mine, e.Name())
		}
	}
	sort.Strings(mine)
	for len(mine) > 10 {
		os.Remove(filepath.Join(in.BackupDir, mine[0]))
		mine = mine[1:]
	}
}
