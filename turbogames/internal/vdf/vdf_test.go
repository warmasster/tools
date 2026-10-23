package vdf

import (
	"strings"
	"testing"
)

const loginUsers = `"users"
{
	"76561199567160448"
	{
		"AccountName"		"cuenta_uno"
		"PersonaName"		"Nombre \"raro\" ñ"
		"RememberPassword"		"1"
		"WantsOfflineMode"		"0"
		"SkipOfflineModeWarning"		"0"
		"AutoLogin"		"1"
		"timestamp"		"1792793960"
	}
	"76561199009090340"
	{
		"AccountName"		"cuenta_dos"
		"PersonaName"		"Otra"
		"AutoLogin"		"0"
	}
}
`

func TestRoundTrip(t *testing.T) {
	root, err := Parse([]byte(loginUsers))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(root.Marshal()); got != loginUsers {
		t.Fatalf("no queda igual al reescribir:\n%s", got)
	}
	u := root.Path("Users", "76561199567160448")
	if u == nil || u.Str("accountname") != "cuenta_uno" {
		t.Fatal("no encuentra la cuenta sin distinguir mayúsculas")
	}
	if u.Str("PersonaName") != `Nombre "raro" ñ` {
		t.Fatalf("escapes mal leídos: %q", u.Str("PersonaName"))
	}
}

func TestEdit(t *testing.T) {
	root, _ := Parse([]byte(loginUsers))
	users := root.Get("users")
	u := users.Get("76561199009090340")
	u.Set("AutoLogin", "1")
	u.Set("RememberPassword", "1")
	u.SetIfPresent("MostRecent", "1")
	if u.Has("MostRecent") {
		t.Fatal("SetIfPresent no debe crear claves")
	}
	if n := users.Remove("76561199567160448"); n != 1 {
		t.Fatalf("Remove quitó %d", n)
	}
	out := string(root.Marshal())
	if strings.Contains(out, "cuenta_uno") || !strings.Contains(out, "\"RememberPassword\"\t\t\"1\"") {
		t.Fatalf("edición mal escrita:\n%s", out)
	}
}

func TestOddSyntax(t *testing.T) {
	src := "// comentario\n\"a\" { \"b\" \"1\" [$WIN32] c 2 \"path\" \"C:\\\\Juegos\\\\X\" \"vacio\" { } }"
	root, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	a := root.Get("a")
	if a.Str("b") != "1" || a.Str("c") != "2" || a.Str("path") != `C:\Juegos\X` {
		t.Fatalf("mal leído: %+v", a.Children)
	}
	if v := a.Get("vacio"); v == nil || !v.IsObject() || len(v.Children) != 0 {
		t.Fatal("objeto vacío mal leído")
	}
	for _, bad := range []string{`"a" {`, `"a" { } }`, `"a"`, `"a" "sin cerrar`} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("debería fallar: %q", bad)
		}
	}
}
