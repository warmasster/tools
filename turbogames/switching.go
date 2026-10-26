package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"turbogames/internal/steam"
)

// SwitchRequest pide entrar en Steam con una cuenta.
type SwitchRequest struct {
	Name    string `json:"name"`
	Offline bool   `json:"offline"` // entrar sin conexión
	AppID   uint32 `json:"appid"`   // abrir este juego al entrar (0 = ninguno)
}

// SwitchResult es cómo acabó el cambio.
type SwitchResult struct {
	OK        bool   `json:"ok"`
	Already   bool   `json:"already"`   // ya estaba dentro con esa cuenta
	Other     string `json:"other"`     // Steam entró con otra cuenta (la elegiste en Steam)
	Canceled  bool   `json:"canceled"`  // dejaste de esperar
	TimedOut  bool   `json:"timedOut"`  // Steam no terminó de entrar
	NeedLogin bool   `json:"needLogin"` // Steam pidió la contraseña
}

// SwitchProgress se manda a la interfaz en cada paso ("switch").
type SwitchProgress struct {
	Name   string `json:"name"`
	Step   string `json:"step"` // close, prepare, open, login, password, game, done
	Detail string `json:"detail,omitempty"`
}

func (a *App) beginSwitch() bool {
	a.switchMu.Lock()
	defer a.switchMu.Unlock()
	if a.switching {
		return false
	}
	a.switching = true
	return true
}

// busySwitching: hay un cambio de cuenta en marcha. Abrir Steam o un juego a la vez
// lo cruzaría (Steam entraría con la cuenta vieja y el cambio fallaría).
func (a *App) busySwitching() bool {
	a.switchMu.Lock()
	defer a.switchMu.Unlock()
	return a.switching
}

func (a *App) endSwitch() {
	a.switchMu.Lock()
	a.switching = false
	a.waitCancel = nil
	a.switchMu.Unlock()
}

// CancelWait deja de esperar a que Steam termine de entrar (Steam sigue a lo suyo).
func (a *App) CancelWait() {
	a.switchMu.Lock()
	if a.waitCancel != nil {
		a.waitCancel()
	}
	a.switchMu.Unlock()
}

// SwitchTo cierra Steam, lo prepara para entrar con la cuenta pedida y lo
// vuelve a abrir. Espera a que la cuenta esté dentro (o a que Steam pida la
// contraseña) y va contando los pasos con el evento "switch".
func (a *App) SwitchTo(r SwitchRequest) (SwitchResult, error) {
	var res SwitchResult
	in := a.steam()
	if in == nil {
		return res, steam.ErrNotFound
	}
	name := strings.TrimSpace(r.Name)
	if name == "" {
		return res, errors.New("falta la cuenta")
	}
	if !a.knownAccount(name) {
		return res, errors.New("Esa cuenta ya no está en la lista")
	}
	if !a.beginSwitch() {
		return res, errors.New("Ya se está cambiando de cuenta")
	}
	defer a.endSwitch()
	a.emit("steam", a.Status())
	defer func() { a.emit("steam", a.Status()) }()

	// datos de la cuenta: SteamID (para saber cuándo ha entrado) y si tiene sesión
	var target uint32
	hasToken := false
	if users, err := in.LoginUsers(); err == nil {
		for _, u := range users {
			if strings.EqualFold(u.AccountName, name) {
				name = u.AccountName
				target = steam.AccountID(u.SteamID)
				hasToken = u.HasToken
			}
		}
	}
	if target == 0 {
		if m, ok := a.metas.get(name); ok {
			target = steam.AccountID(m.SteamID)
		}
	}
	progress := func(step, detail string) {
		a.emit("switch", SwitchProgress{Name: name, Step: step, Detail: detail})
	}

	st := in.ReadStatus()
	// ya está dentro con esa cuenta: no hace falta reiniciar nada
	if st.Running && target != 0 && st.ActiveUser == target && !r.Offline {
		if r.AppID != 0 {
			progress("game", "")
			if err := in.Start(steam.RunGameArgs(r.AppID)...); err != nil {
				return res, err
			}
		}
		_ = a.metas.update(name, func(m *Meta) { m.LastUsed = time.Now().Unix() })
		progress("done", "")
		res.OK, res.Already = true, true
		return res, nil
	}

	if st.Running {
		progress("close", "")
		if err := in.Shutdown(20 * time.Second); err != nil {
			return res, err
		}
	}
	progress("prepare", "")
	if err := in.PrepareLogin(name, r.Offline); err != nil {
		return res, err
	}
	_ = in.ResetActiveUser()
	progress("open", "")
	args := a.steamArgs()
	if r.AppID != 0 {
		args = append(args, steam.RunGameArgs(r.AppID)...)
	}
	if err := in.Start(args...); err != nil {
		return res, err
	}
	_ = a.metas.update(name, func(m *Meta) { m.LastUsed = time.Now().Unix(); m.Hidden = false })
	a.emit("accounts", nil)

	// esperar a que entre
	progress("login", "")
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	a.switchMu.Lock()
	a.waitCancel = cancel
	a.switchMu.Unlock()

	start := time.Now()
	askAfter := 7 * time.Second // sin sesión guardada, Steam pedirá la contraseña enseguida
	if hasToken {
		askAfter = 30 * time.Second // con sesión, si tarda tanto es que ha caducado
	}
	asked := false
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				res.TimedOut = true
			} else {
				res.Canceled = true
			}
			res.NeedLogin = asked
			return res, nil
		case <-time.After(500 * time.Millisecond):
		}
		s := in.ReadStatus()
		if s.ActiveUser != 0 {
			if target == 0 || s.ActiveUser == target {
				if target == 0 {
					_ = a.metas.update(name, func(m *Meta) { m.SteamID = steam.SteamID64(s.ActiveUser) })
				}
				res.OK = true
				break
			}
			res.Other = a.nameOf(s.ActiveUser)
			progress("done", "")
			return res, nil
		}
		if !asked && s.Running && time.Since(start) > askAfter {
			asked = true
			progress("password", "")
		}
	}
	if r.AppID != 0 {
		progress("game", "")
	}
	progress("done", "")
	a.emit("accounts", nil)
	if a.getSettings().AfterSwitch == "minimizar" && a.ctx != nil {
		runtime.WindowMinimise(a.ctx)
	}
	return res, nil
}

// knownAccount dice si la cuenta está en Steam o en TurboGAMES.
func (a *App) knownAccount(name string) bool {
	if _, ok := a.metas.get(name); ok {
		return true
	}
	if in := a.steam(); in != nil {
		users, _ := in.LoginUsers()
		for _, u := range users {
			if strings.EqualFold(u.AccountName, name) {
				return true
			}
		}
	}
	return false
}

// nameOf busca el nombre de una cuenta por su ID.
func (a *App) nameOf(id uint32) string {
	if in := a.steam(); in != nil {
		if users, err := in.LoginUsers(); err == nil {
			for _, u := range users {
				if steam.AccountID(u.SteamID) == id {
					if u.PersonaName != "" {
						return u.PersonaName
					}
					return u.AccountName
				}
			}
		}
	}
	return steam.SteamID64(id)
}

// PlayGame abre un juego. Si se indica una cuenta y no es la que está abierta,
// primero cambia a ella.
func (a *App) PlayGame(appid uint32, account string) (SwitchResult, error) {
	in := a.steam()
	if in == nil {
		return SwitchResult{}, steam.ErrNotFound
	}
	if appid == 0 {
		return SwitchResult{}, errors.New("falta el juego")
	}
	if account == "" {
		if a.busySwitching() {
			return SwitchResult{}, errors.New("Espera a que termine el cambio de cuenta")
		}
		if !in.Running() {
			if err := in.Start(append(a.steamArgs(), steam.RunGameArgs(appid)...)...); err != nil {
				return SwitchResult{}, err
			}
		} else if err := in.Start(steam.RunGameArgs(appid)...); err != nil {
			return SwitchResult{}, err
		}
		return SwitchResult{OK: true, Already: true}, nil
	}
	return a.SwitchTo(SwitchRequest{Name: account, AppID: appid})
}
