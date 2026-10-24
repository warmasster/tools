package winsec

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// skipIfDebugger salta las pruebas de "nadie puede leer este proceso" cuando
// quien prueba es el sistema o tiene activo el privilegio de depuración (como
// en los equipos de GitHub Actions): ahí Windows deja abrir cualquier proceso
// por diseño, y la protección no aplica.
func skipIfDebugger(t *testing.T) {
	t.Helper()
	tok := windows.GetCurrentProcessToken()
	if u, err := tok.GetTokenUser(); err == nil && u.User.Sid.IsWellKnown(windows.WinLocalSystemSid) {
		t.Skip("se ejecuta como SYSTEM")
	}
	var luid windows.LUID
	name, _ := windows.UTF16PtrFromString("SeDebugPrivilege")
	if windows.LookupPrivilegeValue(nil, name, &luid) != nil {
		return
	}
	var n uint32
	windows.GetTokenInformation(tok, windows.TokenPrivileges, nil, 0, &n)
	if n == 0 {
		return
	}
	buf := make([]byte, n)
	if windows.GetTokenInformation(tok, windows.TokenPrivileges, &buf[0], n, &n) != nil {
		return
	}
	tp := (*windows.Tokenprivileges)(unsafe.Pointer(&buf[0]))
	for _, p := range tp.AllPrivileges() {
		if p.Luid == luid && p.Attributes&windows.SE_PRIVILEGE_ENABLED != 0 {
			t.Skip("el privilegio de depuración está activo: Windows deja abrir cualquier proceso")
		}
	}
}
