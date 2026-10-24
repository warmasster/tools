package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"runtime/debug"
	"strings"
	"time"

	"turbovault/internal/csvio"
	"turbovault/internal/gen"
	"turbovault/internal/match"
	"turbovault/internal/totp"
	"turbovault/internal/vault"
	"turbovault/internal/winsec"
)

// Órdenes que admite el puente del navegador. Todo lo demás es solo para la
// ventana de TurboVAULT.
var extCmds = map[string]bool{
	"hello": true, "state": true, "matches": true, "fill": true, "totp": true, "check": true,
	"save": true, "never": true, "generate": true, "copy": true, "search": true, "pairRequest": true,
	"lock": true, "strength": true, "copyText": true,
}

// Órdenes del navegador que tocan datos de la bóveda: solo si la extensión
// cargada es la original.
var needsSecretOK = map[string]bool{
	"matches": true, "fill": true, "totp": true, "check": true, "save": true, "copy": true, "search": true, "pairRequest": true,
}

// Órdenes que funcionan con la bóveda cerrada.
var lockedOK = map[string]bool{
	"hello": true, "state": true, "create": true, "unlock": true, "lock": true, "generate": true,
	"strength": true, "suggest": true, "settings": true, "setSettings": true, "pairRequest": true, "checkBackup": true,
	"restore": true, "backupTargets": true, "addBackupTarget": true, "removeBackupTarget": true,
	"backupNow": true, "quit": true, "copyText": true,
}

func decode(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return errCode("invalid", "Petición mal formada.")
	}
	return nil
}

func (a *Agent) dispatch(cs *connState, cmd string, raw json.RawMessage) (any, error) {
	if cs.peer.Role == roleLauncher {
		if cmd != "openWindow" {
			return nil, errDenied
		}
		var p struct{ Mode string }
		decode(raw, &p)
		return nil, a.openWindow(p.Mode)
	}
	if cs.peer.Role != roleApp && cs.peer.Role != roleExt {
		return nil, errDenied
	}
	ext := cs.peer.Role == roleExt
	if ext && !extCmds[cmd] {
		return nil, errDenied
	}
	if !lockedOK[cmd] {
		if a.expired() || !a.v.Unlocked() {
			return nil, errLocked
		}
		if ext && cmd != "pairRequest" && !cs.isPaired() && !a.checkPaired(cs) {
			return nil, errUnpaired
		}
	}
	if ext {
		if needsSecretOK[cmd] {
			bad := extensionModified(cs.peer.UserData)
			if cs.peer.LoadExt {
				bad = "el navegador carga extensiones por línea de órdenes"
			}
			if bad != "" {
				a.warnExtension(bad)
				return nil, errCode("tampered", "La extensión de TurboVAULT ha sido modificada ("+bad+"). Por seguridad no recibe contraseñas: reinstálala desde TurboVAULT → Ajustes → Navegador.")
			}
		}
		return a.extCmd(cs, cmd, raw)
	}
	return a.appCmd(cmd, raw)
}

// ======================= ventana =======================

type itemArgs struct {
	ID    string        `json:"id"`
	Trash bool          `json:"trash"`
	On    bool          `json:"on"`
	Item  vault.Item    `json:"item"`
	Sec   *vault.Secret `json:"secret"`
	Field string        `json:"field"`
}

type TOTPInfo struct {
	Code   string `json:"code"`
	Left   int    `json:"left"`
	Period int    `json:"period"`
}

func totpOf(secret string) (*TOTPInfo, error) {
	if secret == "" {
		return nil, nil
	}
	k, err := totp.Parse(secret)
	if err != nil {
		return nil, err
	}
	code, left := k.Code(time.Now())
	return &TOTPInfo{Code: code, Left: left, Period: k.Period}, nil
}

func (a *Agent) appCmd(cmd string, raw json.RawMessage) (any, error) {
	switch cmd {
	case "state":
		return a.stateFor(nil), nil

	case "create":
		var p struct{ Master string }
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		if s := gen.Check(p.Master, true); s.Score < 3 {
			return nil, errCode("weak", "Esa contraseña maestra es demasiado fácil de adivinar. "+s.Warning)
		}
		gen := a.lockGen.Load()
		s := getSettings()
		a.mu.Lock()
		a.expires = time.Now().Add(time.Duration(s.SessionMin) * time.Minute)
		a.mu.Unlock()
		if err := a.v.Create(p.Master); err != nil {
			return nil, err
		}
		debug.FreeOSMemory()
		_ = secureDataDir()
		if a.lockGen.Load() != gen {
			a.lockNow("windows")
			return nil, errCode("interrupted", "Se bloqueó el equipo mientras se creaba. La bóveda está creada: ábrela con tu contraseña maestra.")
		}
		a.mu.Lock()
		if a.timer != nil {
			a.timer.Stop()
		}
		a.timer = time.AfterFunc(time.Until(a.expires), func() { a.lockNow("tiempo") })
		a.mu.Unlock()
		a.event(vault.Event{Kind: "create", Src: "app"})
		a.broadcast()
		a.requestBackup()
		return a.stateFor(nil), nil

	case "unlock":
		var p struct{ Master string }
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		if err := a.unlock(p.Master); err != nil {
			return nil, err
		}
		return a.stateFor(nil), nil

	case "lock":
		a.lockNow("manual")
		return nil, nil

	case "quit":
		go func() { time.Sleep(100 * time.Millisecond); a.shutdown() }()
		return nil, nil

	case "items":
		var p itemArgs
		decode(raw, &p)
		return a.v.Items(p.Trash)

	case "get":
		var p itemArgs
		decode(raw, &p)
		it, sec, err := a.v.Get(p.ID)
		if err != nil {
			return nil, err
		}
		out := map[string]any{"item": it, "secret": sec}
		if t, err := totpOf(sec.TOTP); err == nil && t != nil {
			out["totp"] = t
		}
		return out, nil

	case "totp":
		var p itemArgs
		decode(raw, &p)
		_, sec, err := a.v.Get(p.ID)
		if err != nil {
			return nil, err
		}
		t, err := totpOf(sec.TOTP)
		if err != nil {
			return nil, errCode("invalid", err.Error())
		}
		return t, nil

	case "save":
		var p itemArgs
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		it, err := a.saveItem(p.Item, p.Sec, "app")
		if err != nil {
			return nil, err
		}
		return it, nil

	case "delete", "untrash", "purge", "fav":
		var p itemArgs
		decode(raw, &p)
		var err error
		switch cmd {
		case "delete":
			err = a.v.Delete(p.ID)
		case "untrash":
			err = a.v.Restore(p.ID)
		case "purge":
			err = a.v.Purge(p.ID)
		case "fav":
			err = a.v.SetFav(p.ID, p.On)
		}
		if err == nil {
			a.broadcast()
			a.requestBackup()
		}
		return nil, err

	case "emptyTrash":
		n, err := a.v.EmptyTrash()
		if err == nil {
			a.requestBackup()
		}
		return n, err

	case "copy":
		var p itemArgs
		decode(raw, &p)
		return a.copyField(p.ID, p.Field, "", "app")

	case "copyText":
		var p struct {
			Text   string
			Secret bool
		}
		decode(raw, &p)
		if p.Secret {
			secs, err := a.copySecret(p.Text)
			return map[string]int{"clearIn": secs}, err
		}
		_, err := clipSet(p.Text, false)
		return nil, err

	case "generate":
		var p gen.Options
		if len(raw) > 0 {
			if err := decode(raw, &p); err != nil {
				return nil, err
			}
			updateSettings(func(s *Settings) { s.Gen = p })
		} else {
			p = getSettings().Gen
		}
		return generate(p)

	case "suggest":
		// Frase para la contraseña maestra (no cambia las opciones guardadas).
		return generate(gen.Options{Mode: "words", Words: 5, Sep: "-", Capitalize: true, Number: true})

	case "strength":
		var p struct {
			Password string
			Master   bool
			Inputs   []string
		}
		decode(raw, &p)
		return gen.Check(p.Password, p.Master, p.Inputs...), nil

	case "health":
		return a.health()

	case "breach":
		return a.breachCheck()

	case "events":
		return a.v.Events()

	case "clearEvents":
		return nil, a.v.ClearEvents()

	case "never":
		return a.v.Never()

	case "setNever":
		var p struct {
			Host string
			On   bool
		}
		decode(raw, &p)
		return nil, a.v.SetNever(p.Host, p.On)

	case "pairs":
		return a.v.Pairs()

	case "unpair":
		var p struct{ Hash string }
		decode(raw, &p)
		if err := a.v.RemovePair(p.Hash); err != nil {
			return nil, err
		}
		a.event(vault.Event{Kind: "unpair", Src: "app"})
		a.mu.Lock()
		for _, cs := range a.conns {
			if cs.hash() == p.Hash {
				cs.setPair(p.Hash, false)
			}
		}
		a.mu.Unlock()
		a.broadcast()
		return nil, nil

	case "approvePair", "rejectPair":
		var p struct{ Code string }
		decode(raw, &p)
		a.mu.Lock()
		pend := a.pending
		a.pending = nil
		a.mu.Unlock()
		if pend == nil {
			return nil, errCode("notfound", "No hay ninguna vinculación pendiente.")
		}
		if cmd == "approvePair" {
			if p.Code != pend.Code {
				return nil, errCode("invalid", "El código no coincide.")
			}
			if err := a.v.AddPair(pend.Hash, pend.Browser); err != nil {
				return nil, err
			}
			a.event(vault.Event{Kind: "pair", Title: pend.Browser, Src: "app"})
		}
		a.broadcast()
		return nil, nil

	case "settings":
		s := getSettings()
		s.Fails, s.WaitUntil = 0, 0
		return s, nil

	case "setSettings":
		var p struct {
			SessionMin  *int
			ClipSeconds *int
			NotifyFill  *bool
			Autostart   *bool
			HideCapture *bool
			BreachCheck *bool
		}
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		s := updateSettings(func(s *Settings) {
			if p.SessionMin != nil {
				s.SessionMin = *p.SessionMin
			}
			if p.ClipSeconds != nil {
				s.ClipSeconds = *p.ClipSeconds
			}
			if p.NotifyFill != nil {
				s.NotifyFill = *p.NotifyFill
			}
			if p.Autostart != nil {
				s.Autostart = *p.Autostart
			}
			if p.HideCapture != nil {
				s.HideCapture = *p.HideCapture
			}
			if p.BreachCheck != nil {
				s.BreachCheck = *p.BreachCheck
			}
		})
		if p.Autostart != nil {
			setAutostart(*p.Autostart)
		}
		a.broadcast()
		return s, nil

	case "verifyMaster":
		var p struct{ Master string }
		decode(raw, &p)
		return nil, a.masterTry(func() error { return a.v.VerifyMaster(p.Master) })

	case "changeMaster":
		var p struct{ Old, New string }
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		if s := gen.Check(p.New, true); s.Score < 3 {
			return nil, errCode("weak", "La contraseña maestra nueva es demasiado fácil de adivinar. "+s.Warning)
		}
		if err := a.masterTry(func() error { return a.v.ChangeMaster(p.Old, p.New) }); err != nil {
			return nil, err
		}
		a.event(vault.Event{Kind: "master", Src: "app"})
		a.requestBackup()
		return nil, nil

	case "import":
		var p struct {
			Path string
			Wipe bool
		}
		decode(raw, &p)
		return a.importCSV(p.Path, p.Wipe)

	case "export":
		var p struct{ Path, Master string }
		decode(raw, &p)
		return a.exportCSV(p.Path, p.Master)

	case "backupTargets":
		return a.backupStatus(), nil

	case "addBackupTarget":
		var p struct {
			Path string
			Keep int
		}
		decode(raw, &p)
		return a.addBackupTarget(p.Path, p.Keep)

	case "removeBackupTarget":
		var p struct{ ID string }
		decode(raw, &p)
		updateSettings(func(s *Settings) {
			out := s.Backups[:0]
			for _, b := range s.Backups {
				if b.ID != p.ID {
					out = append(out, b)
				}
			}
			s.Backups = out
		})
		return a.backupStatus(), nil

	case "backupNow":
		var p struct{ Path string }
		decode(raw, &p)
		if p.Path == "" {
			a.runBackups(true)
			return a.backupStatus(), nil
		}
		file, err := a.backupTo(p.Path, 0)
		if err == nil {
			a.event(vault.Event{Kind: "backup", Title: file, Src: "app"})
		}
		return file, err

	case "checkBackup":
		var p struct{ Path, Master string }
		decode(raw, &p)
		if !vault.LooksLikeVault(p.Path) {
			return nil, errCode("invalid", "Ese archivo no es una copia de TurboVAULT.")
		}
		if winsec.SamePath(p.Path, vaultPath()) {
			return nil, errCode("invalid", "Esa es la bóveda que ya estás usando.")
		}
		if p.Master == "" {
			return nil, nil
		}
		var info vault.FileInfo
		err := a.masterTry(func() error {
			var e error
			info, e = vault.CheckFile(p.Path, p.Master)
			return e
		})
		if errors.Is(err, vault.ErrWrong) {
			return nil, errCode("wrong", "La contraseña maestra de esa copia no es esa.")
		}
		return info, err

	case "restore":
		var p struct{ Path, Master string }
		decode(raw, &p)
		if winsec.SamePath(p.Path, vaultPath()) {
			return nil, errCode("invalid", "Esa es la bóveda que ya estás usando.")
		}
		var saved string
		gen := a.lockGen.Load()
		err := a.masterTry(func() error {
			var e error
			saved, e = a.v.ReplaceWith(p.Path, p.Master)
			return e
		})
		if errors.Is(err, vault.ErrWrong) {
			return nil, errCode("wrong", "La contraseña maestra de esa copia no es esa.")
		}
		if err != nil {
			return nil, err
		}
		a.lockNow("restaurar")
		_ = secureDataDir()
		updateSettings(func(s *Settings) { s.MaxRev = 0 }) // la restaurada es la buena desde ahora
		// Solo se abre si nada más la ha cerrado mientras se restauraba.
		if err := a.unlockGen(p.Master, gen+1); err == nil {
			a.event(vault.Event{Kind: "restore", Title: filepath.Base(p.Path), Src: "app"})
		}
		return map[string]string{"saved": saved}, nil
	}
	return nil, errCode("invalid", "Orden desconocida: "+cmd)
}

func generate(o gen.Options) (map[string]any, error) {
	r, err := gen.Generate(o)
	if err != nil {
		return nil, errCode("invalid", err.Error())
	}
	return map[string]any{"password": r.Password, "bits": r.Bits, "strength": gen.Check(r.Password, false)}, nil
}

func normalizeURLs(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, u := range in {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if !strings.Contains(u, "://") {
			u = "https://" + u
		}
		if seen[strings.ToLower(u)] {
			continue
		}
		seen[strings.ToLower(u)] = true
		out = append(out, u)
	}
	return out
}

// saveItem valida y guarda una entrada.
func (a *Agent) saveItem(it vault.Item, sec *vault.Secret, src string) (vault.Item, error) {
	it.URLs = normalizeURLs(it.URLs)
	it.Title = strings.TrimSpace(it.Title)
	it.Username = strings.TrimSpace(it.Username)
	if it.Title == "" {
		if len(it.URLs) > 0 {
			it.Title = match.Host(it.URLs[0])
		}
		if it.Title == "" {
			it.Title = "Sin título"
		}
	}
	if sec != nil {
		sec.TOTP = strings.TrimSpace(sec.TOTP)
		if sec.TOTP != "" {
			if _, err := totp.Parse(sec.TOTP); err != nil {
				return vault.Item{}, errCode("invalid", err.Error())
			}
		}
		if sec.Password != "" {
			inputs := []string{it.Username}
			for _, u := range it.URLs {
				inputs = append(inputs, match.Host(u))
			}
			it.Score = gen.Check(sec.Password, false, inputs...).Score
		}
		var fields []vault.Field
		for _, f := range sec.Fields {
			if strings.TrimSpace(f.Name) != "" || f.Value != "" {
				fields = append(fields, f)
			}
		}
		sec.Fields = fields
	}
	isNew := it.ID == ""
	saved, err := a.v.Put(it, sec)
	if err != nil {
		return vault.Item{}, err
	}
	kind := "edit"
	if isNew {
		kind = "add"
	}
	a.event(vault.Event{Kind: kind, Item: saved.ID, Title: saved.Title, Src: src})
	a.broadcast()
	a.requestBackup()
	return saved, nil
}

// copyField copia al portapapeles un campo de una entrada.
func (a *Agent) copyField(id, field, origin, src string) (any, error) {
	it, sec, err := a.v.Get(id)
	if err != nil {
		return nil, err
	}
	var text string
	secret := true
	switch {
	case field == "password":
		text = sec.Password
	case field == "username":
		text, secret = it.Username, false
	case field == "notes":
		text = sec.Notes
	case field == "totp":
		t, err := totpOf(sec.TOTP)
		if err != nil || t == nil {
			return nil, errCode("invalid", "Esta entrada no tiene 2FA.")
		}
		text = t.Code
	case strings.HasPrefix(field, "field:"):
		var i int
		fmt.Sscanf(field, "field:%d", &i)
		if i < 0 || i >= len(sec.Fields) {
			return nil, errCode("notfound", "Ese campo no existe.")
		}
		text, secret = sec.Fields[i].Value, sec.Fields[i].Hidden
	case strings.HasPrefix(field, "history:"):
		var i int
		fmt.Sscanf(field, "history:%d", &i)
		if i < 0 || i >= len(sec.History) {
			return nil, errCode("notfound", "No existe.")
		}
		text = sec.History[i].Password
	default:
		return nil, errCode("invalid", "Campo desconocido.")
	}
	if text == "" {
		return nil, errCode("invalid", "Está vacío.")
	}
	if !secret {
		_, err := clipSet(text, false)
		return map[string]int{"clearIn": 0}, err
	}
	secs, err := a.copySecret(text)
	if err != nil {
		return nil, err
	}
	if field == "password" || field == "totp" {
		a.v.Touch(id)
		a.event(vault.Event{Kind: "copy", Item: id, Title: it.Title, Origin: origin, Src: src})
	}
	return map[string]int{"clearIn": secs}, nil
}

// ======================= importar y exportar =======================

type ImportResult struct {
	Source  string `json:"source"`
	Added   int    `json:"added"`
	Dupes   int    `json:"dupes"`
	Skipped int    `json:"skipped"`
	Wiped   bool   `json:"wiped"`
}

func (a *Agent) importCSV(path string, wipe bool) (*ImportResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errCode("invalid", "No se pudo abrir el archivo.")
	}
	res, err := csvio.Read(f)
	f.Close()
	if err != nil {
		return nil, errCode("invalid", err.Error())
	}
	have := map[string]bool{}
	key := func(url, user, pw string) string {
		return strings.ToLower(match.Host(url)) + "\x00" + strings.ToLower(user) + "\x00" + pw
	}
	if err := a.v.Each(func(it vault.Item, s vault.Secret) {
		u := ""
		if len(it.URLs) > 0 {
			u = it.URLs[0]
		}
		have[key(u, it.Username, s.Password)] = true
	}); err != nil {
		return nil, err
	}
	out := &ImportResult{Source: res.Source, Skipped: res.Skipped}
	var entries []vault.Entry
	for _, r := range res.Records {
		k := key(r.URL, r.Username, r.Password)
		if have[k] {
			out.Dupes++
			continue
		}
		have[k] = true
		it := vault.Item{Title: r.Title, Username: r.Username}
		if r.URL != "" {
			it.URLs = normalizeURLs([]string{r.URL})
		}
		if it.Title == "" && len(it.URLs) > 0 {
			it.Title = match.Host(it.URLs[0])
		}
		if it.Title == "" {
			it.Title = "Importada"
		}
		sec := &vault.Secret{Password: r.Password, Notes: r.Notes}
		if r.TOTP != "" {
			if _, err := totp.Parse(r.TOTP); err == nil {
				sec.TOTP = r.TOTP
			} else {
				sec.Notes = strings.TrimSpace(sec.Notes + "\n2FA: " + r.TOTP)
			}
		}
		if r.Password == "" && r.URL == "" && r.Username == "" {
			it.Type = vault.TypeNote
		}
		if r.Password != "" {
			it.Score = gen.Check(r.Password, false, r.Username, match.Host(r.URL)).Score
		}
		entries = append(entries, vault.Entry{Item: it, Secret: sec})
	}
	if len(entries) > 0 {
		if _, err := a.v.PutMany(entries); err != nil {
			return nil, err
		}
	}
	out.Added = len(entries)
	a.event(vault.Event{Kind: "import", Title: fmt.Sprintf("%d de %s", out.Added, res.Source), Src: "app"})
	if wipe {
		out.Wiped = wipeFile(path) == nil
	}
	a.broadcast()
	a.requestBackup()
	return out, nil
}

// wipeFile sobrescribe un archivo con ceros antes de borrarlo (en un SSD no hay
// garantía total, pero no se deja el texto tal cual).
func wipeFile(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err == nil {
		zero := make([]byte, 64<<10)
		for left := st.Size(); left > 0; {
			n := int64(len(zero))
			if left < n {
				n = left
			}
			if _, err := f.Write(zero[:n]); err != nil {
				break
			}
			left -= n
		}
		f.Sync()
		f.Close()
	}
	return os.Remove(path)
}

func (a *Agent) exportCSV(path, master string) (any, error) {
	if err := a.masterTry(func() error { return a.v.VerifyMaster(master) }); err != nil {
		if errors.Is(err, vault.ErrWrong) {
			return nil, errCode("wrong", "Contraseña maestra incorrecta.")
		}
		return nil, err
	}
	var recs []csvio.Record
	err := a.v.Each(func(it vault.Item, s vault.Secret) {
		u := ""
		if len(it.URLs) > 0 {
			u = it.URLs[0]
		}
		recs = append(recs, csvio.Record{Title: it.Title, URL: u, Username: it.Username, Password: s.Password, Notes: s.Notes, TOTP: s.TOTP})
	})
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, errCode("invalid", "No se pudo crear el archivo.")
	}
	werr := csvio.Write(f, recs)
	cerr := f.Close()
	for i := range recs {
		recs[i] = csvio.Record{}
	}
	if werr != nil || cerr != nil {
		os.Remove(path)
		return nil, errCode("error", "No se pudo escribir el archivo.")
	}
	a.event(vault.Event{Kind: "export", Title: filepath.Base(path), Src: "app"})
	return len(recs), nil
}

// ======================= navegador =======================

type extArgs struct {
	Token    string      `json:"token"`
	URL      string      `json:"url"`
	ID       string      `json:"id"`
	Field    string      `json:"field"`
	Q        string      `json:"q"`
	Title    string      `json:"title"`
	Username string      `json:"username"`
	Password string      `json:"password"`
	Gen      gen.Options `json:"gen"`
}

// MatchItem es lo que ve la extensión de una entrada: nunca el secreto.
type MatchItem struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Username string `json:"username"`
	Host     string `json:"host"`
	HasTOTP  bool   `json:"hasTotp"`
	HasPass  bool   `json:"hasPassword"`
	Exact    bool   `json:"exact"`
}

func toMatch(it vault.Item, page string) MatchItem {
	m := MatchItem{ID: it.ID, Title: it.Title, Username: it.Username, HasTOTP: it.HasTOTP, HasPass: it.HasPassword}
	for _, u := range it.URLs {
		if h := match.Host(u); h != "" && m.Host == "" {
			m.Host = h
		}
		if page != "" && match.Score(u, page) == 2 {
			m.Exact = true
			m.Host = match.Host(u)
		}
	}
	return m
}

func itemMatches(it vault.Item, page string) bool {
	for _, u := range it.URLs {
		if match.Matches(u, page) {
			return true
		}
	}
	return false
}

// secretGate cuenta una petición que revela un secreto. Si el navegador pide
// demasiados de golpe (lo normal es uno cada mucho), se cierra la bóveda: algo
// raro está pasando.
func (a *Agent) secretGate(cs *connState) error {
	if a.secrets.allow(15, 150) {
		return nil
	}
	a.event(vault.Event{Kind: "abuse", Title: "Demasiadas peticiones desde " + cs.peer.Browser, Src: "navegador"})
	a.lockNow("abuso")
	if a.tray != nil {
		a.tray.notify("TurboVAULT se ha cerrado", "El navegador ha pedido demasiadas contraseñas seguidas. Si no has sido tú, revisa tus extensiones.")
	}
	return errCode("ratelimit", "Demasiadas peticiones: TurboVAULT se ha cerrado por seguridad.")
}

func (a *Agent) extCmd(cs *connState, cmd string, raw json.RawMessage) (any, error) {
	var p extArgs
	if err := decode(raw, &p); err != nil {
		return nil, err
	}
	switch cmd {
	case "hello":
		if len(p.Token) >= 32 && len(p.Token) <= 256 {
			cs.setPair(pairHash(p.Token), false)
		}
		return a.stateFor(cs), nil

	case "state":
		return a.stateFor(cs), nil

	case "lock":
		a.lockNow("navegador")
		return nil, nil

	case "strength":
		return gen.Check(p.Password, false, p.Username, match.Host(p.URL)), nil

	case "generate":
		o := getSettings().Gen
		if p.Gen.Mode != "" {
			o = p.Gen
		}
		return generate(o)

	case "pairRequest":
		h := cs.hash()
		if h == "" || len(p.Token) < 32 || pairHash(p.Token) != h {
			return nil, errCode("invalid", "Falta el código de la extensión.")
		}
		code := pairCode(p.Token)
		a.mu.Lock()
		a.pending = &pairRequest{Hash: h, Code: code, Browser: cs.peer.Browser, Created: time.Now()}
		a.mu.Unlock()
		a.broadcast()
		return map[string]string{"code": code}, nil

	case "matches":
		if !a.lookups.allow(120, 2000) {
			return nil, errCode("ratelimit", "Demasiadas peticiones.")
		}
		if match.Origin(p.URL) == "" {
			return []MatchItem{}, nil
		}
		items, err := a.v.Items(false)
		if err != nil {
			return nil, err
		}
		out := []MatchItem{}
		for _, it := range items {
			if it.Type == vault.TypeLogin && itemMatches(it, p.URL) {
				out = append(out, toMatch(it, p.URL))
			}
		}
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].Exact != out[j].Exact {
				return out[i].Exact
			}
			return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
		})
		return out, nil

	case "search":
		if !a.lookups.allow(120, 2000) {
			return nil, errCode("ratelimit", "Demasiadas peticiones.")
		}
		q := strings.ToLower(strings.TrimSpace(p.Q))
		items, err := a.v.Items(false)
		if err != nil {
			return nil, err
		}
		out := []MatchItem{}
		for _, it := range items {
			if it.Type != vault.TypeLogin || len(q) < 2 {
				continue
			}
			hay := strings.ToLower(it.Title + " " + it.Username + " " + strings.Join(it.URLs, " "))
			if strings.Contains(hay, q) {
				out = append(out, toMatch(it, ""))
				if len(out) >= 30 {
					break
				}
			}
		}
		return out, nil

	case "fill", "totp":
		origin := match.Origin(p.URL)
		if origin == "" {
			return nil, errCode("denied", "Solo se rellena en webs http o https.")
		}
		it, sec, err := a.v.Get(p.ID)
		if err != nil {
			return nil, err
		}
		if it.Deleted != 0 || !itemMatches(it, p.URL) {
			return nil, errCode("denied", "Esa entrada no es de esta web.")
		}
		if err := a.secretGate(cs); err != nil {
			return nil, err
		}
		out := map[string]any{}
		if cmd == "fill" {
			out["username"], out["password"] = it.Username, sec.Password
			a.v.Touch(it.ID)
			a.event(vault.Event{Kind: "fill", Item: it.ID, Title: it.Title, Origin: origin, Src: cs.peer.Browser})
			if getSettings().NotifyFill && a.tray != nil {
				a.tray.notify("Contraseña rellenada", it.Title+" en "+match.Host(p.URL))
			}
		}
		if t, err := totpOf(sec.TOTP); err == nil && t != nil {
			out["totp"] = t.Code
			if cmd == "totp" {
				a.event(vault.Event{Kind: "totp", Item: it.ID, Title: it.Title, Origin: origin, Src: cs.peer.Browser})
			}
		} else if cmd == "totp" {
			return nil, errCode("invalid", "Esta entrada no tiene 2FA.")
		}
		return out, nil

	case "copy":
		it, _, err := a.v.Get(p.ID)
		if err != nil {
			return nil, err
		}
		// Desde el menú se puede copiar de otra web (búsqueda), pero solo lo
		// básico de un inicio de sesión vivo, y cuenta para el límite.
		if it.Deleted != 0 || it.Type != vault.TypeLogin || (p.Field != "username" && p.Field != "password" && p.Field != "totp") {
			return nil, errDenied
		}
		if err := a.secretGate(cs); err != nil {
			return nil, err
		}
		return a.copyField(p.ID, p.Field, match.Origin(p.URL), cs.peer.Browser)

	case "check":
		// ¿Hay que ofrecer guardar lo que se ha escrito en esta web?
		host := match.Host(p.URL)
		if host == "" || p.Password == "" {
			return map[string]string{"status": "skip"}, nil
		}
		if !a.checks.allow(20, 200) {
			return nil, errCode("ratelimit", "Demasiadas peticiones.")
		}
		never, _ := a.v.Never()
		for _, h := range never {
			if h == host {
				return map[string]string{"status": "never"}, nil
			}
		}
		items, err := a.v.Items(false)
		if err != nil {
			return nil, err
		}
		var update *vault.Item
		for i, it := range items {
			if it.Type != vault.TypeLogin || !itemMatches(it, p.URL) {
				continue
			}
			sameUser := strings.EqualFold(it.Username, p.Username)
			if !sameUser && p.Username != "" {
				continue
			}
			_, sec, err := a.v.Get(it.ID)
			if err != nil {
				continue
			}
			if sec.Password == p.Password {
				return map[string]string{"status": "same", "id": it.ID}, nil
			}
			if sameUser && update == nil {
				update = &items[i]
			}
		}
		if update != nil {
			return map[string]string{"status": "update", "id": update.ID, "title": update.Title}, nil
		}
		return map[string]string{"status": "new", "title": host}, nil

	case "save":
		origin := match.Origin(p.URL)
		if origin == "" || p.Password == "" {
			return nil, errCode("invalid", "Nada que guardar.")
		}
		if err := a.secretGate(cs); err != nil {
			return nil, err
		}
		if p.ID != "" {
			it, sec, err := a.v.Get(p.ID)
			if err != nil {
				return nil, err
			}
			if !itemMatches(it, p.URL) {
				return nil, errCode("denied", "Esa entrada no es de esta web.")
			}
			sec.Password = p.Password
			if p.Username != "" {
				it.Username = p.Username
			}
			saved, err := a.saveItem(it, &sec, cs.peer.Browser)
			return toMatch(saved, p.URL), err
		}
		title := strings.TrimSpace(p.Title)
		if title == "" {
			title = match.Host(p.URL)
		}
		saved, err := a.saveItem(vault.Item{Type: vault.TypeLogin, Title: title, URLs: []string{origin}, Username: p.Username},
			&vault.Secret{Password: p.Password}, cs.peer.Browser)
		return toMatch(saved, p.URL), err

	case "copyText":
		// Copiar una contraseña recién generada en el menú de la extensión.
		if !cs.isPaired() && !a.checkPaired(cs) {
			return nil, errUnpaired
		}
		if p.Password == "" || len(p.Password) > 512 {
			return nil, errCode("invalid", "Nada que copiar.")
		}
		secs, err := a.copySecret(p.Password)
		return map[string]int{"clearIn": secs}, err

	case "never":
		host := match.Host(p.URL)
		if host == "" {
			return nil, errCode("invalid", "Web no válida.")
		}
		return nil, a.v.SetNever(host, true)
	}
	return nil, errDenied
}
