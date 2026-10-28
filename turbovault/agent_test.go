package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"turbovault/internal/vault"
)

const testMaster = "caballo-grapa-bateria-correcta-7"

var clipMu sync.Mutex
var clipText string

func init() {
	vault.SetParamsForTests()
	clipSet = func(text string, secret bool) (uint32, error) {
		clipMu.Lock()
		clipText = text
		clipMu.Unlock()
		return 1, nil
	}
	clipClear = func(uint32) {}
}

func newTestAgent(t *testing.T) *Agent {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TURBOVAULT_DIR", dir)
	// Sin mirar los navegadores de este PC: un perfil de prueba con la extensión
	// original cargada (si no aparece en ningún perfil, no recibe contraseñas).
	fakeBrowser(t, dir)
	settingsMu.Lock()
	settings = nil
	settingsMu.Unlock()
	a := newAgent("", "")
	return a
}

// fakeBrowser deja en dir un perfil de navegador con la extensión instalada cargada.
func fakeBrowser(t *testing.T, dir string) {
	t.Helper()
	ext, err := installExtension()
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, "navegador")
	os.MkdirAll(filepath.Join(base, "Default"), 0o700)
	prefs, _ := json.Marshal(map[string]any{"extensions": map[string]any{"settings": map[string]any{extensionID: map[string]any{"path": ext}}}})
	if err := os.WriteFile(filepath.Join(base, "Default", "Preferences"), prefs, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TURBOVAULT_BROWSERS", base)
	extCheckMu.Lock()
	extCheckAt, extCheckRes = map[string]time.Time{}, map[string]string{}
	extCheckMu.Unlock()
}

func app() *connState { return &connState{peer: peer{Role: roleApp}} }
func ext() *connState { return &connState{peer: peer{Role: roleExt, Browser: "Chrome"}} }

func do(t *testing.T, a *Agent, cs *connState, cmd string, args any) (json.RawMessage, error) {
	t.Helper()
	var raw json.RawMessage
	if args != nil {
		raw, _ = json.Marshal(args)
	}
	out, err := a.dispatch(cs, cmd, raw)
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(out)
	return b, nil
}

func must(t *testing.T, a *Agent, cs *connState, cmd string, args any, out any) {
	t.Helper()
	raw, err := do(t, a, cs, cmd, args)
	if err != nil {
		t.Fatalf("%s: %v", cmd, err)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
	}
}

func code(err error) string {
	var ae *apiError
	if errors.As(err, &ae) {
		return ae.Code
	}
	if errors.Is(err, vault.ErrLocked) {
		return "locked"
	}
	if errors.Is(err, vault.ErrWrong) {
		return "wrong"
	}
	if err == nil {
		return ""
	}
	return "error:" + err.Error()
}

func TestCreateRequiresStrongMaster(t *testing.T) {
	a := newTestAgent(t)
	if _, err := do(t, a, app(), "create", map[string]string{"master": "Fernando1999!"}); code(err) != "weak" {
		t.Fatalf("maestra débil aceptada: %v", err)
	}
	must(t, a, app(), "create", map[string]string{"master": testMaster}, nil)
	if !a.v.Unlocked() {
		t.Fatal("tras crear debe quedar abierta")
	}
	a.mu.Lock()
	left := time.Until(a.expires)
	a.mu.Unlock()
	if left < 9*time.Minute || left > 10*time.Minute {
		t.Fatalf("sesión de %v", left)
	}
}

func TestSessionIsFixedAndExpires(t *testing.T) {
	a := newTestAgent(t)
	must(t, a, app(), "create", map[string]string{"master": testMaster}, nil)
	a.mu.Lock()
	exp := a.expires
	a.mu.Unlock()
	// Usarla no alarga la sesión.
	must(t, a, app(), "items", nil, nil)
	a.mu.Lock()
	if !a.expires.Equal(exp) {
		t.Fatal("la sesión se ha alargado con el uso")
	}
	// Forzar que ya haya caducado (como tras suspender el equipo).
	a.expires = time.Now().Add(-time.Second)
	a.mu.Unlock()
	if _, err := do(t, a, app(), "items", nil); code(err) != "locked" {
		t.Fatalf("caducada: %v", err)
	}
	if a.v.Unlocked() {
		t.Fatal("no se cerró al caducar")
	}
}

func TestUnlockThrottle(t *testing.T) {
	a := newTestAgent(t)
	must(t, a, app(), "create", map[string]string{"master": testMaster}, nil)
	a.lockNow("manual")
	for i := 0; i < 3; i++ {
		if _, err := do(t, a, app(), "unlock", map[string]string{"master": "no-es-esta-para-nada"}); code(err) != "wrong" {
			t.Fatalf("intento %d: %v", i, err)
		}
	}
	// Tras 3 fallos hay que esperar, aunque se acierte.
	if _, err := do(t, a, app(), "unlock", map[string]string{"master": testMaster}); code(err) != "wait" {
		t.Fatalf("debía pedir esperar: %v", err)
	}
	updateSettings(func(s *Settings) { s.WaitUntil = 0 })
	must(t, a, app(), "unlock", map[string]string{"master": testMaster}, nil)
	if getSettings().Fails != 0 {
		t.Fatal("los fallos no se reiniciaron")
	}
	var evs []vault.Event
	must(t, a, app(), "events", nil, &evs)
	found := false
	for _, e := range evs {
		if e.Kind == "fails" {
			found = true
		}
	}
	if !found {
		t.Fatal("no se apuntaron los intentos fallidos")
	}
}

func saveLogin(t *testing.T, a *Agent, title, url, user, pw string) vault.Item {
	var it vault.Item
	must(t, a, app(), "save", map[string]any{
		"item":   vault.Item{Title: title, URLs: []string{url}, Username: user},
		"secret": vault.Secret{Password: pw},
	}, &it)
	return it
}

func pairExt(t *testing.T, a *Agent, cs *connState) {
	t.Helper()
	tok := strings.Repeat("ab", 32)
	must(t, a, cs, "hello", map[string]string{"token": tok}, nil)
	var r map[string]string
	must(t, a, cs, "pairRequest", map[string]string{"token": tok}, &r)
	var st State
	must(t, a, app(), "state", nil, &st)
	if st.Pair == nil || st.Pair.Code != r["code"] || st.Pair.Code != pairCode(tok) {
		t.Fatalf("vinculación pendiente: %+v %v", st.Pair, r)
	}
	if _, err := do(t, a, app(), "approvePair", map[string]string{"code": "000 000"}); code(err) != "invalid" {
		t.Fatalf("código malo aceptado: %v", err)
	}
	// Tras un código malo hay que volver a pedirla.
	must(t, a, cs, "pairRequest", map[string]string{"token": tok}, &r)
	must(t, a, app(), "approvePair", map[string]string{"code": r["code"]}, nil)
}

func TestExtensionRules(t *testing.T) {
	a := newTestAgent(t)
	must(t, a, app(), "create", map[string]string{"master": testMaster}, nil)
	gh := saveLogin(t, a, "GitHub", "https://github.com", "fer", "pw-github-123456")
	saveLogin(t, a, "Banco", "https://banco.es", "yo", "pw-banco-123456")

	e := ext()
	// Sin vincular no hay nada.
	must(t, a, e, "hello", map[string]string{"token": strings.Repeat("cd", 32)}, nil)
	if _, err := do(t, a, e, "matches", map[string]string{"url": "https://github.com/login"}); code(err) != "unpaired" {
		t.Fatalf("sin vincular: %v", err)
	}
	// Órdenes solo de la ventana.
	for _, cmd := range []string{"items", "get", "export", "events", "approvePair", "setSettings", "changeMaster", "unpair"} {
		if _, err := do(t, a, e, cmd, nil); code(err) != "denied" {
			t.Fatalf("%s desde el navegador: %v", cmd, err)
		}
	}
	e = ext()
	pairExt(t, a, e)

	var ms []MatchItem
	must(t, a, e, "matches", map[string]string{"url": "https://github.com/login"}, &ms)
	if len(ms) != 1 || ms[0].ID != gh.ID || ms[0].Username != "fer" {
		t.Fatalf("coincidencias: %+v", ms)
	}
	raw, _ := json.Marshal(ms)
	if strings.Contains(string(raw), "pw-github") {
		t.Fatal("matches enseña la contraseña")
	}
	// Una web falsa no recibe nada.
	must(t, a, e, "matches", map[string]string{"url": "https://github.com.evil.net/login"}, &ms)
	if len(ms) != 0 {
		t.Fatalf("phishing: %+v", ms)
	}
	if _, err := do(t, a, e, "fill", map[string]string{"id": gh.ID, "url": "https://evil.net"}); code(err) != "denied" {
		t.Fatalf("relleno en otra web: %v", err)
	}
	if _, err := do(t, a, e, "fill", map[string]string{"id": gh.ID, "url": "http://github.com"}); code(err) != "denied" {
		t.Fatalf("relleno en http: %v", err)
	}
	var f map[string]string
	must(t, a, e, "fill", map[string]string{"id": gh.ID, "url": "https://github.com/login"}, &f)
	if f["password"] != "pw-github-123456" || f["username"] != "fer" {
		t.Fatalf("relleno: %v", f)
	}
	// Guardar desde el navegador.
	var c map[string]string
	must(t, a, e, "check", map[string]string{"url": "https://github.com/session", "username": "fer", "password": "pw-github-123456"}, &c)
	if c["status"] != "same" {
		t.Fatalf("check igual: %v", c)
	}
	must(t, a, e, "check", map[string]string{"url": "https://github.com/session", "username": "fer", "password": "otra-nueva-XYZ-987"}, &c)
	if c["status"] != "update" || c["id"] != gh.ID {
		t.Fatalf("check cambio: %v", c)
	}
	must(t, a, e, "check", map[string]string{"url": "https://nueva.com/login", "username": "x", "password": "y"}, &c)
	if c["status"] != "new" {
		t.Fatalf("check nueva: %v", c)
	}
	var saved MatchItem
	must(t, a, e, "save", map[string]string{"url": "https://nueva.com/login", "username": "x", "password": "y-larga-123"}, &saved)
	if saved.Title != "nueva.com" {
		t.Fatalf("guardada: %+v", saved)
	}
	// No se puede usar save para cambiar la contraseña de otra web.
	if _, err := do(t, a, e, "save", map[string]string{"url": "https://evil.net", "id": gh.ID, "password": "robada"}); code(err) != "denied" {
		t.Fatalf("save cruzado: %v", err)
	}
	must(t, a, e, "never", map[string]string{"url": "https://pesada.com/login"}, nil)
	must(t, a, e, "check", map[string]string{"url": "https://pesada.com", "username": "a", "password": "b"}, &c)
	if c["status"] != "never" {
		t.Fatalf("never: %v", c)
	}
	// La actividad apunta el relleno con su web.
	var evs []vault.Event
	must(t, a, app(), "events", nil, &evs)
	okFill := false
	for _, ev := range evs {
		if ev.Kind == "fill" && ev.Origin == "https://github.com" && ev.Src == "Chrome" {
			okFill = true
		}
	}
	if !okFill {
		t.Fatalf("actividad sin el relleno: %+v", evs)
	}
}

func TestRateLimitLocks(t *testing.T) {
	a := newTestAgent(t)
	must(t, a, app(), "create", map[string]string{"master": testMaster}, nil)
	gh := saveLogin(t, a, "GitHub", "https://github.com", "fer", "pw-github-123456")
	e := ext()
	pairExt(t, a, e)
	var last error
	for i := 0; i < 40; i++ {
		_, last = do(t, a, e, "fill", map[string]string{"id": gh.ID, "url": "https://github.com"})
		if last != nil {
			break
		}
	}
	if code(last) != "ratelimit" {
		t.Fatalf("sin límite: %v", last)
	}
	if a.v.Unlocked() {
		t.Fatal("tras el abuso la bóveda debe cerrarse")
	}
}

func TestUnpairStopsAccess(t *testing.T) {
	a := newTestAgent(t)
	must(t, a, app(), "create", map[string]string{"master": testMaster}, nil)
	e := ext()
	pairExt(t, a, e)
	var pairs []vault.Pair
	must(t, a, app(), "pairs", nil, &pairs)
	if len(pairs) != 1 || pairs[0].Name != "Chrome" {
		t.Fatalf("pairs: %+v", pairs)
	}
	must(t, a, app(), "unpair", map[string]string{"hash": pairs[0].Hash}, nil)
	if _, err := do(t, a, e, "matches", map[string]string{"url": "https://x.com"}); code(err) != "unpaired" {
		t.Fatalf("tras desvincular: %v", err)
	}
}

func TestCopyAndClipboard(t *testing.T) {
	a := newTestAgent(t)
	must(t, a, app(), "create", map[string]string{"master": testMaster}, nil)
	it := saveLogin(t, a, "X", "https://x.com", "user", "secreto-copiado-1")
	var r map[string]int
	must(t, a, app(), "copy", map[string]string{"id": it.ID, "field": "password"}, &r)
	clipMu.Lock()
	got := clipText
	clipMu.Unlock()
	if got != "secreto-copiado-1" || r["clearIn"] != 20 {
		t.Fatalf("copiado %q, %v", got, r)
	}
}

func TestImportExport(t *testing.T) {
	a := newTestAgent(t)
	must(t, a, app(), "create", map[string]string{"master": testMaster}, nil)
	saveLogin(t, a, "a.com", "https://a.com", "u", "p1")
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "Contraseñas de Chrome.csv")
	os.WriteFile(csvPath, []byte("name,url,username,password,note\na.com,https://a.com/,u,p1,\nb.com,https://b.com/login,v,p2,nota\n"), 0o600)
	var r ImportResult
	must(t, a, app(), "import", map[string]any{"path": csvPath, "wipe": true}, &r)
	if r.Added != 1 || r.Dupes != 1 || !r.Wiped {
		t.Fatalf("importación: %+v", r)
	}
	if _, err := os.Stat(csvPath); !os.IsNotExist(err) {
		t.Fatal("el CSV no se borró")
	}
	out := filepath.Join(dir, "export.csv")
	if _, err := do(t, a, app(), "export", map[string]string{"path": out, "master": "mala-mala-mala-mala"}); code(err) != "wrong" {
		t.Fatalf("exportar con maestra mala: %v", err)
	}
	must(t, a, app(), "export", map[string]string{"path": out, "master": testMaster}, nil)
	b, _ := os.ReadFile(out)
	if !strings.Contains(string(b), "p2") || !strings.Contains(string(b), "nota") {
		t.Fatalf("exportación: %s", b)
	}
}

func TestBackupAndRestore(t *testing.T) {
	a := newTestAgent(t)
	must(t, a, app(), "create", map[string]string{"master": testMaster}, nil)
	saveLogin(t, a, "antes", "https://a.com", "u", "p1")
	dest := t.TempDir()
	must(t, a, app(), "addBackupTarget", map[string]any{"path": dest, "keep": 2}, nil)
	es, _ := os.ReadDir(dest)
	if len(es) != 1 {
		t.Fatalf("copias tras añadir destino: %d", len(es))
	}
	// Sin cambios no se repite.
	a.runBackups(false)
	es, _ = os.ReadDir(dest)
	if len(es) != 1 {
		t.Fatalf("copia repetida sin cambios: %d", len(es))
	}
	first := filepath.Join(dest, es[0].Name())
	for i := 0; i < 3; i++ {
		time.Sleep(1100 * time.Millisecond) // el nombre lleva los segundos
		saveLogin(t, a, "x", "https://x.com", "u", "p")
		a.runBackups(false)
	}
	es, _ = os.ReadDir(dest)
	if len(es) != 2 {
		t.Fatalf("debía guardar solo las 2 últimas: %d", len(es))
	}
	_ = first
	var st []TargetStatus
	must(t, a, app(), "backupTargets", nil, &st)
	if len(st) != 1 || !st[0].Connected || st[0].Copies != 2 || st[0].LastError != "" {
		t.Fatalf("estado: %+v", st)
	}
	// Restaurar la más vieja que queda.
	old := filepath.Join(dest, es[0].Name())
	var info vault.FileInfo
	must(t, a, app(), "checkBackup", map[string]string{"path": old, "master": testMaster}, &info)
	if info.Items < 2 {
		t.Fatalf("info: %+v", info)
	}
	must(t, a, app(), "restore", map[string]string{"path": old, "master": testMaster}, nil)
	if !a.v.Unlocked() {
		t.Fatal("tras restaurar con su maestra queda abierta")
	}
}

func TestStateHidesPairFromExt(t *testing.T) {
	a := newTestAgent(t)
	must(t, a, app(), "create", map[string]string{"master": testMaster}, nil)
	e := ext()
	tok := strings.Repeat("ef", 32)
	must(t, a, e, "hello", map[string]string{"token": tok}, nil)
	must(t, a, e, "pairRequest", map[string]string{"token": tok}, nil)
	var st State
	must(t, a, e, "state", nil, &st)
	if st.Pair != nil {
		t.Fatal("la extensión no debe ver el código desde el agente")
	}
	// Con un token que no es el suyo no puede pedir vinculación.
	if _, err := do(t, a, e, "pairRequest", map[string]string{"token": strings.Repeat("00", 32)}); code(err) != "invalid" {
		t.Fatalf("token ajeno: %v", err)
	}
}

func TestAllMasterChecksAreThrottled(t *testing.T) {
	a := newTestAgent(t)
	must(t, a, app(), "create", map[string]string{"master": testMaster}, nil)
	out := filepath.Join(t.TempDir(), "x.csv")
	for i := 0; i < 3; i++ {
		if _, err := do(t, a, app(), "export", map[string]string{"path": out, "master": "no-es-la-maestra-ni-de-lejos"}); code(err) != "wrong" {
			t.Fatalf("intento %d: %v", i, err)
		}
	}
	if _, err := do(t, a, app(), "verifyMaster", map[string]string{"master": testMaster}); code(err) != "wait" {
		t.Fatalf("tras 3 fallos exportando también hay que esperar: %v", err)
	}
	updateSettings(func(s *Settings) { s.WaitUntil = 0 })
	if _, err := do(t, a, app(), "checkBackup", map[string]string{"path": vaultPath(), "master": testMaster}); code(err) != "invalid" {
		t.Fatalf("la propia bóveda no se puede usar como copia: %v", err)
	}
}

func TestUnlockAbortedByLock(t *testing.T) {
	a := newTestAgent(t)
	must(t, a, app(), "create", map[string]string{"master": testMaster}, nil)
	a.lockNow("manual")
	// Windows se bloquea justo mientras Argon2id trabaja: la bóveda no se queda abierta.
	testAfterUnlock = func(a *Agent) { a.lockNow("windows") }
	defer func() { testAfterUnlock = nil }()
	if _, err := do(t, a, app(), "unlock", map[string]string{"master": testMaster}); code(err) != "interrupted" {
		t.Fatalf("debía interrumpirse: %v", err)
	}
	if a.v.Unlocked() {
		t.Fatal("se quedó abierta con Windows bloqueado")
	}
	testAfterUnlock = nil
	must(t, a, app(), "unlock", map[string]string{"master": testMaster}, nil)
}

func TestExtCopyLimited(t *testing.T) {
	a := newTestAgent(t)
	must(t, a, app(), "create", map[string]string{"master": testMaster}, nil)
	var note vault.Item
	must(t, a, app(), "save", map[string]any{"item": vault.Item{Title: "nota", Type: vault.TypeNote}, "secret": vault.Secret{Notes: "secreta"}}, &note)
	it := saveLogin(t, a, "X", "https://x.com", "u", "pw-1234567")
	e := ext()
	pairExt(t, a, e)
	if _, err := do(t, a, e, "copy", map[string]string{"id": note.ID, "field": "notes"}); code(err) != "denied" {
		t.Fatalf("copiar una nota desde el navegador: %v", err)
	}
	if _, err := do(t, a, e, "copy", map[string]string{"id": it.ID, "field": "history:0"}); code(err) != "denied" {
		t.Fatalf("copiar historial desde el navegador: %v", err)
	}
	must(t, a, e, "copy", map[string]string{"id": it.ID, "field": "password", "url": "https://x.com"}, nil)
}

// Si la extensión no aparece en ningún perfil no se puede comprobar: no recibe
// contraseñas (antes se daba por buena).
func TestExtensionNotFoundIsRefused(t *testing.T) {
	a := newTestAgent(t)
	t.Setenv("TURBOVAULT_BROWSERS", t.TempDir()) // ningún perfil con la extensión
	extCheckMu.Lock()
	extCheckAt, extCheckRes = map[string]time.Time{}, map[string]string{}
	extCheckMu.Unlock()
	must(t, a, app(), "create", map[string]string{"master": testMaster}, nil)
	if _, err := do(t, a, ext(), "pairRequest", map[string]string{"hash": "x"}); err == nil || !strings.Contains(err.Error(), "no encuentra la extensión") {
		t.Fatalf("debería negarse: %v", err)
	}
}

// «No la encuentro» no es «la han cambiado»: ni alarma ni aviso en la ventana.
func TestExtensionNotFoundIsNoAlarm(t *testing.T) {
	a := newTestAgent(t)
	t.Setenv("TURBOVAULT_BROWSERS", t.TempDir())
	extCheckMu.Lock()
	extCheckAt, extCheckRes, extCheckWarn, extCheckBad = map[string]time.Time{}, map[string]string{}, false, ""
	extCheckMu.Unlock()
	must(t, a, app(), "create", map[string]string{"master": testMaster}, nil)
	do(t, a, ext(), "pairRequest", map[string]string{"hash": "x"})
	extCheckMu.Lock()
	warned, bad := extCheckWarn, extCheckBad
	extCheckMu.Unlock()
	if warned || bad != "" {
		t.Fatalf("no debería avisar de extensión modificada (aviso=%v, %q)", warned, bad)
	}
}

// Cambiar la duración en Ajustes se aplica a la sesión abierta; con la bóveda
// cerrada solo se guarda, y no se aceptan duraciones fuera de la lista.
func TestSessionDurationAppliesNow(t *testing.T) {
	a := newTestAgent(t)
	must(t, a, app(), "create", map[string]string{"master": testMaster}, nil)
	must(t, a, app(), "setSettings", map[string]int{"sessionMin": 240}, nil)
	a.mu.Lock()
	left := time.Until(a.expires)
	a.mu.Unlock()
	if left < 239*time.Minute || left > 240*time.Minute {
		t.Fatalf("tras pedir 4 h quedan %v", left)
	}
	must(t, a, app(), "setSettings", map[string]int{"sessionMin": 2}, nil)
	a.mu.Lock()
	left = time.Until(a.expires)
	a.mu.Unlock()
	if left < time.Minute || left > 2*time.Minute {
		t.Fatalf("tras pedir 2 min quedan %v", left)
	}
	must(t, a, app(), "lock", nil, nil)
	must(t, a, app(), "setSettings", map[string]int{"sessionMin": 30}, nil)
	a.mu.Lock()
	zero := a.expires.IsZero()
	a.mu.Unlock()
	if !zero || a.v.Unlocked() {
		t.Fatal("con la bóveda cerrada, cambiar la duración no debe abrir ninguna sesión")
	}
	if getSettings().SessionMin != 30 {
		t.Fatalf("no se guardó: %d", getSettings().SessionMin)
	}
	must(t, a, app(), "setSettings", map[string]int{"sessionMin": 10000}, nil)
	if getSettings().SessionMin != 10 {
		t.Fatalf("duración fuera de la lista aceptada: %d", getSettings().SessionMin)
	}
}
