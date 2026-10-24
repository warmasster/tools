package steam

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows/registry"

	"turbogames/internal/vdf"
)

// ErrRunning: hay que cerrar Steam antes de tocar sus archivos (al salir los
// reescribe y desharía el cambio).
var ErrRunning = errors.New("Steam sigue abierto")

// Shutdown cierra Steam: primero se lo pide (steam.exe -shutdown, que guarda
// todo y cierra la sesión limpia) y, si no ha cerrado en el tiempo dado, lo
// cierra a la fuerza.
func (in *Install) Shutdown(timeout time.Duration) error {
	if len(steamPIDs(true)) == 0 {
		// pueden quedar restos (steamwebhelper) de un cierre anterior
		for _, pid := range steamPIDs(false) {
			killPID(pid)
		}
		return nil
	}
	cmd := exec.Command(in.Exe, "-shutdown")
	cmd.Dir = in.Dir
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Start()
	if cmd.Process != nil {
		go cmd.Wait()
	}
	if waitGone(true, timeout) {
		// el resto de procesos suele irse solo un momento después
		if !waitGone(false, 4*time.Second) {
			for _, pid := range steamPIDs(false) {
				killPID(pid)
			}
		}
		return nil
	}
	for _, pid := range steamPIDs(false) {
		killPID(pid)
	}
	if !waitGone(false, 6*time.Second) {
		return errors.New("no se ha podido cerrar Steam")
	}
	return nil
}

// Running dice si steam.exe está abierto.
func (in *Install) Running() bool { return runningHook() }

// Se pueden cambiar en las pruebas para no tocar el Steam ni el registro de verdad.
var (
	runningHook   = func() bool { return len(steamPIDs(true)) > 0 }
	autoLoginHook = setAutoLoginUser
	autoLoginGet  = currentAutoLogin
)

// PrepareLogin deja todo listo para que, al abrir Steam, entre directamente
// con esa cuenta. Steam tiene que estar cerrado.
//
// En loginusers.vdf la cuenta queda como la de inicio automático (AutoLogin, y
// también MostRecent/AllowAutoLogin si el archivo es de una versión antigua) y
// las demás no. En el registro, AutoLoginUser apunta a ella.
func (in *Install) PrepareLogin(account string, offline bool) error {
	if in.Running() {
		return ErrRunning
	}
	if err := in.editLoginUsers(func(u *vdf.Node, isTarget bool) {
		if isTarget {
			u.Set("RememberPassword", "1")
			u.Set("AutoLogin", "1")
			u.SetIfPresent("MostRecent", "1")
			u.SetIfPresent("AllowAutoLogin", "1")
			if offline {
				u.Set("WantsOfflineMode", "1")
				u.Set("SkipOfflineModeWarning", "1")
			} else {
				u.SetIfPresent("WantsOfflineMode", "0")
				u.SetIfPresent("SkipOfflineModeWarning", "0")
			}
			return
		}
		u.SetIfPresent("AutoLogin", "0")
		u.SetIfPresent("MostRecent", "0")
	}, account); err != nil {
		return err
	}
	return autoLoginHook(account)
}

// Forget hace que Steam olvide una cuenta en este PC: la quita de
// loginusers.vdf y borra su sesión guardada de local.vdf. Steam tiene que
// estar cerrado. La cuenta no se toca en los servidores de Steam.
func (in *Install) Forget(account string) error {
	if in.Running() {
		return ErrRunning
	}
	p := in.LoginUsersPath()
	if root, err := vdf.ParseFile(p); err == nil {
		users := root.Get("users")
		changed := false
		if users != nil {
			for _, u := range append([]*vdf.Node(nil), users.Children...) {
				if strings.EqualFold(u.Str("AccountName"), account) {
					users.Remove(u.Key)
					changed = true
				}
			}
		}
		if changed {
			in.backup(p)
			if err := vdf.WriteFile(p, root); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	lp := LocalVDF()
	if root, err := vdf.ParseFile(lp); err == nil {
		if cc := root.Path(connectCachePath...); cc != nil {
			n := 0
			for _, k := range tokenKeyVariants(account) {
				n += cc.Remove(k)
			}
			if n > 0 {
				in.backup(lp)
				if err := vdf.WriteFile(lp, root); err != nil {
					return err
				}
			}
		}
	}

	if strings.EqualFold(autoLoginGet(), account) {
		return autoLoginHook("")
	}
	return nil
}

func (in *Install) editLoginUsers(fn func(u *vdf.Node, isTarget bool), account string) error {
	p := in.LoginUsersPath()
	root, err := vdf.ParseFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Steam aún no recuerda ninguna cuenta
		}
		return err
	}
	users := root.Get("users")
	if users == nil {
		return nil
	}
	for _, u := range users.Children {
		if !u.IsObject() {
			continue
		}
		fn(u, account != "" && strings.EqualFold(u.Str("AccountName"), account))
	}
	in.backup(p)
	return vdf.WriteFile(p, root)
}

func currentAutoLogin() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, steamKey, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, _ := k.GetStringValue("AutoLoginUser")
	return v
}

func setAutoLoginUser(account string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, steamKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetStringValue("AutoLoginUser", account); err != nil {
		return err
	}
	// Las versiones antiguas de Steam también miran este valor.
	return k.SetDWordValue("RememberPassword", 1)
}

// ResetActiveUser pone a 0 la cuenta abierta en el registro. Si Steam se cerró
// a la fuerza, se queda la de antes y parecería que ya ha entrado.
func (in *Install) ResetActiveUser() error {
	if in.Running() {
		return ErrRunning
	}
	return resetActiveHook()
}

var resetActiveHook = func() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, steamKey+`\ActiveProcess`, registry.SET_VALUE)
	if err != nil {
		return nil // sin esa clave no hay nada que limpiar
	}
	defer k.Close()
	return k.SetDWordValue("ActiveUser", 0)
}

// Start abre Steam con los argumentos dados (por ejemplo "-silent").
func (in *Install) Start(args ...string) error {
	cmd := exec.Command(in.Exe, args...)
	cmd.Dir = in.Dir
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

// RunGameArgs son los argumentos para que Steam abra un juego en cuanto pueda
// (si ya está abierto se lo pasa a la ventana que hay).
func RunGameArgs(appid uint32) []string {
	return []string{"--", "steam://rungameid/" + strconv.FormatUint(uint64(appid), 10)}
}

// OpenURL le pasa a Steam una dirección steam:// (abrir la tienda, validar
// archivos…).
func (in *Install) OpenURL(u string) error {
	return in.Start("--", u)
}
