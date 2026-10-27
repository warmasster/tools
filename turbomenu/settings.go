package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Settings son las preferencias que se guardan entre usos.
type Settings struct {
	// VTKey: la clave de VirusTotal tal como la guardaban las versiones de antes (en
	// claro). Al arrancar pasa a VTKeyEnc, cifrada con DPAPI (solo este usuario de
	// Windows en este PC puede leerla).
	VTKey    string `json:"vtKey,omitempty"`
	VTKeyEnc string `json:"vtKeyEnc,omitempty"`
	// KnownLookup: preguntar a CIRCL si el archivo es conocido (se manda su huella SHA-1).
	// "" = aún no se ha preguntado, "yes" o "no".
	KnownLookup string              `json:"knownLookup,omitempty"`
	Recent      map[string][]string `json:"recent"`    // formatos usados por tipo ("video" → ["mp4","gif"])
	Prewarm     bool                `json:"prewarm"`   // dejar TurboMENU preparado al iniciar sesión
	Modern      bool                `json:"modern"`    // también en el menú nuevo de Windows 11
	IdleMins    int                 `json:"idleMins"`  // minutos que se queda preparado tras cerrar una ventana
	LastVideo   string              `json:"lastVideo"` // última opción de «Reducir» para vídeo
	LastImage   int                 `json:"lastImage"` // última calidad de imagen
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
	s, _ := readSettings()
	return s
}

// readSettings lee los ajustes; ok es false si el archivo existe pero no se puede
// leer (entonces no se guarda encima: se perderían la clave y lo demás).
func readSettings() (Settings, bool) {
	s := defaultSettings()
	ok := true
	if b, err := os.ReadFile(filepath.Join(dataDir(), "settings.json")); err == nil {
		if json.Unmarshal(b, &s) != nil {
			s, ok = defaultSettings(), false
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		ok = false
	}
	if s.Recent == nil {
		s.Recent = map[string][]string{}
	}
	if s.IdleMins <= 0 || s.IdleMins > 240 {
		s.IdleMins = 15
	}
	return s, ok
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
	cur, ok := readSettings()
	for i := 0; !ok && i < 5; i++ {
		time.Sleep(60 * time.Millisecond) // otro proceso lo está cambiando justo ahora
		cur, ok = readSettings()
	}
	if !ok {
		p := filepath.Join(dataDir(), "settings.json")
		if b, err := os.ReadFile(p); err != nil || json.Valid(b) {
			setMu.Unlock()
			return errors.New("no se pueden leer los ajustes guardados; no se guarda encima para no perderlos")
		}
		// Dañado: se aparta (no se borra) y se sigue con los de por defecto.
		os.Rename(p, p+".dañado-"+time.Now().Format("20060102-150405"))
	}
	settings = cur
	f(&settings)
	b, _ := json.MarshalIndent(settings, "", "  ")
	setMu.Unlock()
	p := filepath.Join(dataDir(), "settings.json")
	// Un temporal por proceso: dos ventanas guardando a la vez no se mezclan.
	tmp := fmt.Sprintf("%s.%d.tmp", p, os.Getpid())
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// vtKey devuelve la clave de VirusTotal ("" si no hay o no se puede descifrar).
func vtKey() string {
	s := getSettings()
	if s.VTKeyEnc != "" {
		k, err := unprotect(s.VTKeyEnc)
		if err != nil {
			return ""
		}
		return k
	}
	return s.VTKey
}

// setVTKey guarda la clave cifrada (vacía = quitarla).
func setVTKey(key string) error {
	enc := ""
	if key != "" {
		var err error
		if enc, err = protect(key); err != nil {
			return err
		}
	}
	return updateSettings(func(s *Settings) { s.VTKey, s.VTKeyEnc = "", enc })
}

// migrateSettings pasa a cifrada la clave que las versiones de antes guardaban en claro.
func migrateSettings() {
	if getSettings().VTKey == "" {
		return
	}
	updateSettings(func(s *Settings) {
		if s.VTKey == "" {
			return
		}
		if enc, err := protect(s.VTKey); err == nil {
			s.VTKeyEnc, s.VTKey = enc, ""
		}
	})
}

var dpapiEntropy = []byte("TurboMENU VirusTotal")

func protect(plain string) (string, error) {
	in := windows.DataBlob{Size: uint32(len(plain)), Data: unsafe.SliceData([]byte(plain))}
	ent := windows.DataBlob{Size: uint32(len(dpapiEntropy)), Data: &dpapiEntropy[0]}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, &ent, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return "", err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return base64.StdEncoding.EncodeToString(unsafe.Slice(out.Data, out.Size)), nil
}

func unprotect(enc string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil || len(raw) == 0 {
		return "", errors.New("clave dañada")
	}
	in := windows.DataBlob{Size: uint32(len(raw)), Data: &raw[0]}
	ent := windows.DataBlob{Size: uint32(len(dpapiEntropy)), Data: &dpapiEntropy[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, &ent, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return "", err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return string(unsafe.Slice(out.Data, out.Size)), nil
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
