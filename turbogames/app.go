package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"turbogames/internal/steam"
	"turbogames/internal/vault"
)

const version = "1.0.2"

type App struct {
	ctx context.Context

	smu      sync.Mutex
	settings Settings

	imu sync.Mutex
	in  *steam.Install // nil si no se encuentra Steam

	metas *metaStore
	vault *vault.Vault

	vmu      sync.Mutex
	lastUse  time.Time // último uso de la bóveda (para cerrarla sola)
	clipStop chan struct{}

	uaMu    sync.Mutex
	uaCache map[uint32]uaEntry

	switchMu   sync.Mutex
	switching  bool
	waitCancel context.CancelFunc

	images *imageServer
}

func NewApp() *App {
	a := &App{settings: defaultSettings(), uaCache: map[uint32]uaEntry{}}
	_ = loadJSON(filepath.Join(dataDir(), "ajustes.json"), &a.settings)
	a.settings.normalize()
	a.metas = loadMetaStore(filepath.Join(dataDir(), "cuentas.json"))
	a.vault = vault.Open(filepath.Join(dataDir(), "boveda.dat"))
	a.locate()
	a.images = newImageServer(a)
	return a
}

func (a *App) locate() {
	in, err := steam.Locate(a.getSettings().SteamDir)
	if err != nil {
		in = nil
	} else {
		in.BackupDir = filepath.Join(dataDir(), "copias")
	}
	a.imu.Lock()
	a.in = in
	a.imu.Unlock()
}

func (a *App) steam() *steam.Install {
	a.imu.Lock()
	defer a.imu.Unlock()
	return a.in
}

func (a *App) getSettings() Settings {
	a.smu.Lock()
	defer a.smu.Unlock()
	return a.settings
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	go a.watch(ctx)
}

func (a *App) emit(name string, data any) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, name, data)
	}
}

// ---------- vigilancia ----------

// watch mira cada poco si Steam ha cambiado (abierto, cerrado, otra cuenta,
// un juego) y si sus archivos de cuentas han cambiado, y se lo dice a la
// interfaz. También cierra la bóveda cuando lleva un rato sin usarse.
func (a *App) watch(ctx context.Context) {
	t := time.NewTicker(1500 * time.Millisecond)
	defer t.Stop()
	var last SteamState
	var lastFiles string
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		s := a.Status()
		if s != last {
			last = s
			a.emit("steam", s)
		}
		if in := a.steam(); in != nil {
			sig := fileSig(in.LoginUsersPath()) + "|" + fileSig(steam.LocalVDF()) + "|" + fileSig(in.Path("config", "avatarcache"))
			if lastFiles != "" && sig != lastFiles {
				a.emit("accounts", nil)
			}
			lastFiles = sig
		}
		a.autoLock()
	}
}

func fileSig(p string) string {
	fi, err := os.Stat(p)
	if err != nil {
		return "-"
	}
	return fi.ModTime().Format(time.RFC3339Nano) + "/" + itoa(fi.Size())
}

// ---------- datos para la interfaz ----------

// SteamState es el estado de Steam que ve la interfaz.
type SteamState struct {
	Found      bool   `json:"found"`
	Dir        string `json:"dir"`
	Running    bool   `json:"running"`
	ActiveID   string `json:"activeId"` // SteamID64 de la cuenta abierta
	RunningApp uint32 `json:"runningApp"`
	AutoLogin  string `json:"autoLogin"`
	Switching  bool   `json:"switching"`
}

// Status devuelve cómo está Steam ahora.
func (a *App) Status() SteamState {
	in := a.steam()
	if in == nil {
		return SteamState{}
	}
	st := in.ReadStatus()
	s := SteamState{Found: true, Dir: in.Dir, Running: st.Running, RunningApp: st.RunningApp, AutoLogin: st.AutoLogin}
	if st.ActiveUser != 0 {
		s.ActiveID = steam.SteamID64(st.ActiveUser)
	}
	a.switchMu.Lock()
	s.Switching = a.switching
	a.switchMu.Unlock()
	return s
}

// InitData es todo lo que necesita la interfaz al arrancar.
type InitData struct {
	Version  string     `json:"version"`
	Steam    SteamState `json:"steam"`
	Settings Settings   `json:"settings"`
	Vault    VaultState `json:"vault"`
	Accounts []Account  `json:"accounts"`
	Error    string     `json:"error,omitempty"`
}

// Init es la primera llamada de la interfaz.
func (a *App) Init() InitData {
	d := InitData{Version: version, Steam: a.Status(), Settings: a.getSettings(), Vault: a.VaultState()}
	acc, err := a.buildAccounts()
	d.Accounts = acc
	if err != nil {
		d.Error = "No se pudieron leer las cuentas de Steam: " + err.Error()
	}
	return d
}

// Accounts devuelve la lista de cuentas.
func (a *App) Accounts() ([]Account, error) {
	return a.buildAccounts()
}

// GameName devuelve el nombre de un juego por su número.
func (a *App) GameName(appid uint32) string {
	if in := a.steam(); in != nil {
		if m, ok := in.AppInfo()[appid]; ok {
			return m.Name
		}
	}
	return ""
}

// ---------- fichas de cuenta ----------

// MetaPatch son los cambios que se pueden hacer a la ficha de una cuenta.
type MetaPatch struct {
	Alias  *string   `json:"alias"`
	Notes  *string   `json:"notes"`
	Tags   *[]string `json:"tags"`
	Pinned *bool     `json:"pinned"`
	Hidden *bool     `json:"hidden"`
}

// UpdateAccount guarda alias, notas, etiquetas, favorita u oculta.
func (a *App) UpdateAccount(name string, p MetaPatch) error {
	return a.metas.update(name, func(m *Meta) {
		if p.Alias != nil {
			m.Alias = clip(strings.TrimSpace(*p.Alias), 40)
		}
		if p.Notes != nil {
			m.Notes = clip(strings.TrimSpace(*p.Notes), 2000)
		}
		if p.Tags != nil {
			m.Tags = cleanTags(*p.Tags)
		}
		if p.Pinned != nil {
			m.Pinned = *p.Pinned
		}
		if p.Hidden != nil {
			m.Hidden = *p.Hidden
		}
	})
}

// AddManualAccount añade una cuenta que Steam todavía no recuerda (para tener
// su contraseña a mano y entrar con ella).
// Si ya estaba pero oculta, vuelve a la lista (Restored).
func (a *App) AddManualAccount(name string) (AddResult, error) {
	name = strings.TrimSpace(name)
	if msg := accountNameError(name); msg != "" {
		return AddResult{}, errors.New(msg)
	}
	accs, _ := a.buildAccounts()
	for _, acc := range accs {
		if acc.Key == keyOf(name) {
			if !acc.Hidden {
				return AddResult{}, errors.New("Esa cuenta ya está en la lista")
			}
			return AddResult{Restored: true}, a.metas.update(acc.Name, func(m *Meta) { m.Hidden = false })
		}
	}
	return AddResult{}, a.metas.update(name, func(m *Meta) { m.Hidden = false })
}

// AddResult dice cómo acabó añadir una cuenta.
type AddResult struct {
	Restored bool `json:"restored"` // ya estaba, oculta, y vuelve a la lista
}

// CheckAccountName comprueba un nombre de usuario mientras se escribe.
func (a *App) CheckAccountName(name string) string { return accountNameError(strings.TrimSpace(name)) }

// ForgetOptions dice qué borrar al olvidar una cuenta.
type ForgetOptions struct {
	Steam bool `json:"steam"` // que Steam también la olvide en este PC
	Vault bool `json:"vault"` // borrar sus datos de la bóveda
}

// ForgetAccount quita una cuenta de la lista (y, si se pide, de Steam y de la
// bóveda).
func (a *App) ForgetAccount(name string, o ForgetOptions) error {
	in := a.steam()
	// Se comprueba antes de tocar nada, pero sus datos se borran al final: si cerrar
	// Steam o editar sus archivos falla, la cuenta sigue ahí y sus datos también.
	if o.Vault && a.vault.Exists() && !a.vault.Unlocked() {
		return errors.New("Desbloquea la bóveda para borrar sus datos")
	}
	inSteam := false
	if in != nil {
		users, _ := in.LoginUsers()
		for _, u := range users {
			if strings.EqualFold(u.AccountName, name) {
				inSteam = true
			}
		}
	}
	if o.Steam && in != nil {
		if !a.beginSwitch() {
			return errors.New("Espera a que termine el cambio de cuenta")
		}
		defer a.endSwitch()
		wasRunning := in.Running()
		prevAuto := in.ReadStatus().AutoLogin
		if wasRunning {
			if err := in.Shutdown(15 * time.Second); err != nil {
				return err
			}
		}
		if err := in.Forget(name); err != nil {
			if wasRunning {
				_ = in.Start(a.steamArgs()...) // que no se quede Steam cerrado
			}
			return err
		}
		if wasRunning {
			args := a.steamArgs()
			if prevAuto != "" && !strings.EqualFold(prevAuto, name) {
				_ = in.PrepareLogin(prevAuto, false)
			}
			_ = in.Start(args...)
		}
		inSteam = false
	}
	if o.Vault && a.vault.Exists() {
		if err := a.vault.Set(keyOf(name), vault.Entry{}); err != nil {
			if errors.Is(err, vault.ErrLocked) {
				return errors.New("Desbloquea la bóveda para borrar sus datos")
			}
			return err
		}
		a.touchVault()
		_ = a.metas.update(name, func(m *Meta) { m.HasSecret = false })
	}
	if inSteam {
		// Steam la sigue recordando: solo se oculta para que no vuelva a salir
		return a.metas.update(name, func(m *Meta) { m.Hidden = true })
	}
	return a.metas.remove(name)
}

// ---------- ajustes ----------

func (a *App) GetSettings() Settings { return a.getSettings() }

func (a *App) SaveSettings(s Settings) (SteamState, error) {
	s.normalize()
	a.smu.Lock()
	changedDir := a.settings.SteamDir != s.SteamDir
	a.settings = s
	err := saveJSON(filepath.Join(dataDir(), "ajustes.json"), s)
	a.smu.Unlock()
	if changedDir {
		a.locate()
	}
	return a.Status(), err
}

// PickSteamDir deja elegir la carpeta de Steam a mano.
func (a *App) PickSteamDir() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Elige la carpeta donde está Steam (la que tiene steam.exe)"})
	if err != nil || dir == "" {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(dir, "steam.exe")); err != nil {
		return "", errors.New("En esa carpeta no está steam.exe")
	}
	return dir, nil
}

// ---------- abrir cosas ----------

// OpenFolder abre una carpeta en el Explorador.
func (a *App) OpenFolder(path string) error {
	if path == "" {
		return errors.New("no hay carpeta")
	}
	st, err := os.Stat(path)
	if err != nil {
		return errors.New("Esa carpeta ya no existe")
	}
	if !st.IsDir() {
		// explorer.exe con un archivo lo abriría (o lo ejecutaría si es un .exe)
		return errors.New("Eso no es una carpeta")
	}
	cmd := exec.Command("explorer.exe", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

var steamURLRe = regexp.MustCompile(`^steam://(install|validate|nav/games/details)/\d{1,10}$|^steam://open/main$`)

// SteamURL abre una dirección steam:// (tienda, propiedades, verificar…).
func (a *App) SteamURL(u string) error {
	// Solo lo que usa la ventana: instalar, verificar y ver un juego, o abrir Steam.
	// steam://run/…//argumentos, uninstall, exit… no pasan.
	if !steamURLRe.MatchString(u) {
		return errors.New("dirección no válida")
	}
	in := a.steam()
	if in == nil {
		return steam.ErrNotFound
	}
	if a.busySwitching() {
		return errors.New("Espera a que termine el cambio de cuenta")
	}
	return in.OpenURL(u)
}

// OpenSteam abre Steam si está cerrado, o lo trae al frente.
func (a *App) OpenSteam() error {
	in := a.steam()
	if in == nil {
		return steam.ErrNotFound
	}
	if a.busySwitching() {
		return errors.New("Espera a que termine el cambio de cuenta")
	}
	if in.Running() {
		return in.OpenURL("steam://open/main")
	}
	return in.Start(a.steamArgs()...)
}

func (a *App) steamArgs() []string {
	if a.getSettings().StartSilent {
		return []string{"-silent"}
	}
	return nil
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
