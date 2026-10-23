package main

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"turbogames/internal/vault"
)

// VaultState es lo que la interfaz sabe de la bóveda (nunca su contenido).
type VaultState struct {
	Exists      bool   `json:"exists"`
	Unlocked    bool   `json:"unlocked"`
	Damaged     string `json:"damaged,omitempty"`
	LockMinutes int    `json:"lockMinutes"`
	LocksIn     int    `json:"locksIn"` // segundos hasta que se cierre sola
}

func (a *App) VaultState() VaultState {
	s := VaultState{Exists: a.vault.Exists(), Unlocked: a.vault.Unlocked(), LockMinutes: a.getSettings().LockMinutes}
	if err := a.vault.Damaged(); err != nil {
		s.Damaged = err.Error()
	}
	if s.Unlocked {
		a.vmu.Lock()
		left := time.Until(a.lastUse.Add(time.Duration(s.LockMinutes) * time.Minute))
		a.vmu.Unlock()
		s.LocksIn = max(0, int(left.Seconds()))
	}
	return s
}

func (a *App) touchVault() {
	a.vmu.Lock()
	a.lastUse = time.Now()
	a.vmu.Unlock()
}

func (a *App) autoLock() {
	if !a.vault.Unlocked() {
		return
	}
	a.vmu.Lock()
	idle := time.Since(a.lastUse)
	a.vmu.Unlock()
	if idle >= time.Duration(a.getSettings().LockMinutes)*time.Minute {
		a.vault.Lock()
		a.emit("vault", a.VaultState())
	}
}

func vaultErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, vault.ErrLocked):
		return errors.New("La bóveda está cerrada")
	case errors.Is(err, vault.ErrWrong):
		return errors.New("Contraseña maestra incorrecta")
	}
	return err
}

// VaultCreate crea la bóveda con una contraseña maestra.
func (a *App) VaultCreate(master string) (VaultState, error) {
	err := a.vault.Create(master)
	if err == nil {
		a.touchVault()
	}
	return a.VaultState(), vaultErr(err)
}

// VaultUnlock abre la bóveda.
func (a *App) VaultUnlock(master string) (VaultState, error) {
	err := a.vault.Unlock(master)
	if err == nil {
		a.touchVault()
		a.syncSecretFlags()
	}
	return a.VaultState(), vaultErr(err)
}

// VaultLock la cierra ya.
func (a *App) VaultLock() VaultState {
	a.vault.Lock()
	return a.VaultState()
}

// VaultTouch alarga el tiempo antes de que se cierre sola (la interfaz lo
// llama mientras la usas).
func (a *App) VaultTouch() VaultState {
	if a.vault.Unlocked() {
		a.touchVault()
	}
	return a.VaultState()
}

// VaultGet devuelve los datos guardados de una cuenta (para verlos o editarlos).
func (a *App) VaultGet(name string) (vault.Entry, error) {
	e, _, err := a.vault.Get(keyOf(name))
	if err == nil {
		a.touchVault()
	}
	return e, vaultErr(err)
}

// VaultSet guarda los datos de una cuenta. Si todo va vacío, se borran.
func (a *App) VaultSet(name string, e vault.Entry) error {
	if keyOf(name) == "" {
		return errors.New("falta la cuenta")
	}
	e.Password = strings.TrimRight(e.Password, "\r\n")
	e.Email = strings.TrimSpace(e.Email)
	e.Recovery = strings.ToUpper(strings.TrimSpace(e.Recovery))
	e.Notes = clip(e.Notes, 4000)
	if msg := entryError(e); msg != "" {
		return errors.New(msg)
	}
	if err := a.vault.Set(keyOf(name), e); err != nil {
		return vaultErr(err)
	}
	a.touchVault()
	return a.metas.update(name, func(m *Meta) { m.HasSecret = !e.Empty() })
}

// entryError comprueba los datos de acceso antes de guardarlos.
func entryError(e vault.Entry) string {
	if e.Email != "" {
		at := strings.LastIndexByte(e.Email, '@')
		if at < 1 || !strings.Contains(e.Email[at+1:], ".") || strings.ContainsAny(e.Email, " ,;") || strings.HasSuffix(e.Email, ".") {
			return "Ese correo no parece válido"
		}
	}
	if e.Recovery != "" && !recoveryRe.MatchString(e.Recovery) {
		return "El código de recuperación de Steam Guard es una R y 5 números, como R12345"
	}
	if e.EmailPassword != "" && e.Email == "" {
		return "Falta el correo de esa contraseña"
	}
	return ""
}

var recoveryRe = regexp.MustCompile(`^R[0-9]{5}$`)

// VaultChangeMaster cambia la contraseña maestra.
func (a *App) VaultChangeMaster(old, newMaster string) (VaultState, error) {
	if old == newMaster {
		return a.VaultState(), errors.New("La nueva contraseña maestra es igual que la actual")
	}
	err := a.vault.ChangeMaster(old, newMaster)
	if err == nil {
		a.touchVault()
	}
	return a.VaultState(), vaultErr(err)
}

// VaultDestroy borra la bóveda entera.
func (a *App) VaultDestroy(master string) (VaultState, error) {
	if err := a.vault.Destroy(master); err != nil {
		return a.VaultState(), vaultErr(err)
	}
	for k := range a.metas.all() {
		_ = a.metas.update(k, func(m *Meta) { m.HasSecret = false })
	}
	return a.VaultState(), nil
}

// syncSecretFlags pone bien la marca "tiene contraseña" de cada cuenta al
// abrir la bóveda (por si el archivo vino de otra copia).
func (a *App) syncSecretFlags() {
	names, err := a.vault.Names()
	if err != nil {
		return
	}
	has := map[string]bool{}
	for _, n := range names {
		has[n] = true
	}
	changed := false
	for k, m := range a.metas.all() {
		if m.HasSecret != has[k] {
			v := has[k]
			_ = a.metas.update(m.Name, func(mm *Meta) { mm.HasSecret = v })
			changed = true
		}
	}
	for n := range has {
		if _, ok := a.metas.get(n); !ok {
			_ = a.metas.update(n, func(mm *Meta) { mm.HasSecret = true })
			changed = true
		}
	}
	if changed {
		a.emit("accounts", nil)
	}
}

// ---------- portapapeles ----------

// CopyField copia al portapapeles un dato de una cuenta: "name" (el usuario)
// o, de la bóveda, "password", "email", "emailPassword" o "recovery". Lo de
// la bóveda no entra en el historial del portapapeles de Windows ni en la nube
// y se borra solo al rato.
func (a *App) CopyField(name, field string) (int, error) {
	if field == "name" {
		user := name
		if m, ok := a.metas.get(name); ok && m.Name != "" {
			user = m.Name
		}
		return 0, a.copyText(user, false)
	}
	e, _, err := a.vault.Get(keyOf(name))
	if err != nil {
		return 0, vaultErr(err)
	}
	a.touchVault()
	var v string
	switch field {
	case "password":
		v = e.Password
	case "email":
		v = e.Email
	case "emailPassword":
		v = e.EmailPassword
	case "recovery":
		v = e.Recovery
	default:
		return 0, errors.New("campo desconocido")
	}
	if v == "" {
		return 0, errors.New("No hay nada guardado ahí")
	}
	secs := a.getSettings().ClipSeconds
	return secs, a.copyText(v, true)
}

// CopyText copia un texto normal (rutas, SteamID…).
func (a *App) CopyText(s string) error { return a.copyText(s, false) }

func (a *App) copyText(s string, secret bool) error {
	seq, err := setClipboard(s, secret)
	if err != nil {
		return errors.New("No se pudo copiar: " + err.Error())
	}
	a.vmu.Lock()
	if a.clipStop != nil {
		close(a.clipStop)
		a.clipStop = nil
	}
	if secret {
		stop := make(chan struct{})
		a.clipStop = stop
		d := time.Duration(a.getSettings().ClipSeconds) * time.Second
		go func() {
			select {
			case <-stop:
			case <-time.After(d):
				clearClipboardIf(seq)
				a.emit("clipCleared", nil)
			}
		}()
	}
	a.vmu.Unlock()
	return nil
}

// shutdown: al cerrar la app, si lo último copiado era un secreto, se borra.
func (a *App) shutdownClip() {
	a.vmu.Lock()
	stop := a.clipStop
	a.clipStop = nil
	a.vmu.Unlock()
	if stop != nil {
		close(stop)
		clearClipboardIf(lastSecretSeq())
	}
	a.vault.Lock()
}
