package steam

import (
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"turbogames/internal/vdf"
)

// fakeSteam monta una carpeta de Steam de mentira y cambia los ganchos para
// que nada toque el registro ni el Steam de verdad.
func fakeSteam(t *testing.T) (*Install, *string) {
	t.Helper()
	dir := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(dir, "config", "avatarcache"), 0o755))
	must(os.MkdirAll(filepath.Join(dir, "steamapps", "common", "Juego Uno"), 0o755))
	must(os.MkdirAll(filepath.Join(dir, "userdata", "1000", "config"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "steam.exe"), []byte("MZ"), 0o644))

	id1, id2 := SteamID64(1000), SteamID64(2000)
	must(os.WriteFile(filepath.Join(dir, "config", "loginusers.vdf"), []byte(`"users"
{
	"`+id1+`"
	{
		"AccountName"		"primera"
		"PersonaName"		"La Primera"
		"RememberPassword"		"1"
		"WantsOfflineMode"		"0"
		"SkipOfflineModeWarning"		"0"
		"AutoLogin"		"1"
		"timestamp"		"200"
	}
	"`+id2+`"
	{
		"AccountName"		"Segunda"
		"PersonaName"		"Dos"
		"RememberPassword"		"1"
		"AutoLogin"		"0"
		"MostRecent"		"0"
		"timestamp"		"100"
	}
}
`), 0o644))
	must(os.WriteFile(filepath.Join(dir, "config", "libraryfolders.vdf"), []byte(`"libraryfolders"
{
	"0"
	{
		"path"		"`+strings.ReplaceAll(dir, `\`, `\\`)+`"
		"label"		""
		"apps"
		{
			"10"		"123"
		}
	}
}
`), 0o644))
	must(os.WriteFile(filepath.Join(dir, "steamapps", "appmanifest_10.acf"), []byte(`"AppState"
{
	"appid"		"10"
	"name"		"Juego Uno"
	"installdir"		"Juego Uno"
	"SizeOnDisk"		"123"
	"LastOwner"		"`+id1+`"
}
`), 0o644))
	must(os.WriteFile(filepath.Join(dir, "userdata", "1000", "config", "localconfig.vdf"), []byte(`"UserLocalConfigStore"
{
	"Software" { "Valve" { "Steam" { "apps" {
		"10" { "LastPlayed" "1700000000" "Playtime" "90" }
		"20" { "cloud" { "x" "y" } }
	} } } }
}
`), 0o644))
	local := filepath.Join(dir, "local.vdf")
	must(os.WriteFile(local, []byte(fmt.Sprintf(`"MachineUserConfigStore"
{
	"Software" { "Valve" { "Steam" { "ConnectCache" {
		"%x1"		"cifrado"
	} } } }
}
`, crc32.ChecksumIEEE([]byte("primera")))), 0o644))

	auto := new(string)
	oldRun, oldAuto, oldGet, oldLocal := runningHook, autoLoginHook, autoLoginGet, localVDFHook
	autoLoginGet = func() string { return *auto }
	runningHook = func() bool { return false }
	autoLoginHook = func(a string) error { *auto = a; return nil }
	localVDFHook = func() string { return local }
	t.Cleanup(func() { runningHook, autoLoginHook, autoLoginGet, localVDFHook = oldRun, oldAuto, oldGet, oldLocal })

	in, err := Locate(dir)
	if err != nil {
		t.Fatal(err)
	}
	in.BackupDir = filepath.Join(dir, "copias")
	return in, auto
}

func TestIDs(t *testing.T) {
	if got := SteamID64(1606894720); got != "76561199567160448" {
		t.Fatal(got)
	}
	if AccountID("76561199567160448") != 1606894720 || AccountID("abc") != 0 || AccountID("12") != 0 {
		t.Fatal("AccountID mal")
	}
}

func TestLoginUsers(t *testing.T) {
	in, _ := fakeSteam(t)
	users, err := in.LoginUsers()
	if err != nil || len(users) != 2 {
		t.Fatalf("%v %+v", err, users)
	}
	if users[0].AccountName != "primera" || !users[0].HasToken || !users[0].AutoLogin {
		t.Fatalf("primera mal: %+v", users[0])
	}
	if users[1].HasToken {
		t.Fatal("la segunda no tiene sesión guardada")
	}
}

func TestPrepareLoginAndForget(t *testing.T) {
	in, auto := fakeSteam(t)
	if err := in.PrepareLogin("segunda", true); err != nil {
		t.Fatal(err)
	}
	if *auto != "segunda" {
		t.Fatalf("AutoLoginUser = %q", *auto)
	}
	root, _ := vdf.ParseFile(in.LoginUsersPath())
	u1 := root.Path("users", SteamID64(1000))
	u2 := root.Path("users", SteamID64(2000))
	if u1.Str("AutoLogin") != "0" || u2.Str("AutoLogin") != "1" || u2.Str("MostRecent") != "1" || u2.Str("WantsOfflineMode") != "1" {
		t.Fatalf("loginusers mal:\n%s", root.Marshal())
	}
	if u1.Has("MostRecent") {
		t.Fatal("no debe añadir MostRecent donde no estaba")
	}
	if ents, _ := os.ReadDir(in.BackupDir); len(ents) == 0 {
		t.Fatal("no hizo copia de seguridad")
	}

	runningHook = func() bool { return true }
	if err := in.PrepareLogin("primera", false); err != ErrRunning {
		t.Fatalf("con Steam abierto debe negarse: %v", err)
	}
	runningHook = func() bool { return false }

	*auto = "primera"
	if err := in.Forget("PRIMERA"); err != nil {
		t.Fatal(err)
	}
	users, _ := in.LoginUsers()
	if len(users) != 1 || users[0].AccountName != "Segunda" {
		t.Fatalf("no la olvidó: %+v", users)
	}
	if HasToken(TokenKeys(), "primera") {
		t.Fatal("no borró la sesión guardada")
	}
	if *auto != "" {
		t.Fatal("debe quitar el inicio automático de la cuenta olvidada")
	}
}

func TestLibrary(t *testing.T) {
	in, _ := fakeSteam(t)
	libs := in.Libraries()
	if len(libs) != 1 {
		t.Fatalf("bibliotecas: %+v", libs)
	}
	apps := in.Installed()
	if len(apps) != 1 || apps[0].Name != "Juego Uno" || apps[0].SizeOnDisk != 123 || !strings.HasSuffix(apps[0].InstallDir, `common\Juego Uno`) {
		t.Fatalf("instalados: %+v", apps)
	}
	ua := in.UserApps(1000)
	if len(ua) != 1 || ua[10].Minutes != 90 {
		t.Fatalf("jugados: %+v", ua)
	}
	if ids := in.UserDirs(); len(ids) != 1 || ids[0] != 1000 {
		t.Fatalf("userdata: %v", ids)
	}
}

// Contra el Steam de este PC, sin cambiar nada:
//
//	$env:TURBOGAMES_REAL = "1"; go test -run Real -v ./internal/steam/
func TestRealReadOnly(t *testing.T) {
	if os.Getenv("TURBOGAMES_REAL") == "" {
		t.Skip("pon TURBOGAMES_REAL=1 para leer el Steam de este PC")
	}
	in, err := Locate("")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Steam en %s · %+v", in.Dir, in.ReadStatus())
	users, err := in.LoginUsers()
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		t.Logf("cuenta %s (%s) sesión=%v auto=%v", u.PersonaName, u.SteamID, u.HasToken, u.AutoLogin)
	}
	meta := in.AppInfo()
	t.Logf("appinfo: %d apps", len(meta))
	for _, a := range in.Installed() {
		t.Logf("instalado %d %s · %d MB · portada %q", a.AppID, a.Name, a.SizeOnDisk>>20, in.LocalImage(a.AppID, "cover", meta[a.AppID]))
	}
}
