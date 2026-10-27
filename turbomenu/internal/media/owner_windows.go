package media

import (
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// publicSafe: en la raíz de C: cualquier usuario del PC puede crear carpetas, y lo que
// hay dentro hereda que todos puedan cambiarlo. Un FFmpeg de ahí solo se usa si la
// carpeta y sus ejecutables son tuyos (o de los administradores o del sistema) y
// nadie más puede escribir en ellos ni sustituirlos.
func publicSafe(dir string) bool {
	paths := []string{dir, filepath.Join(dir, "ffmpeg.exe"), filepath.Join(dir, "ffprobe.exe")}
	if filepath.Base(dir) == "bin" {
		paths = append(paths, filepath.Dir(dir))
	}
	for _, p := range paths {
		if !trustedOwner(p) || othersCanWrite(p) {
			return false
		}
	}
	return true
}

func trusted(sid *windows.SID) bool {
	if sid == nil {
		return false
	}
	if sid.IsWellKnown(windows.WinBuiltinAdministratorsSid) || sid.IsWellKnown(windows.WinLocalSystemSid) ||
		sid.IsWellKnown(windows.WinCreatorOwnerSid) {
		return true
	}
	s := sid.String()
	// TrustedInstaller y OWNER RIGHTS.
	if s == "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464" || s == "S-1-3-4" {
		return true
	}
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	return err == nil && windows.EqualSid(sid, u.User.Sid)
}

func trustedOwner(path string) bool {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return false
	}
	owner, _, err := sd.Owner()
	return err == nil && trusted(owner)
}

// Permisos con los que otro podría cambiar o sustituir el archivo o la carpeta.
const writeMask = 0x2 | 0x4 | 0x40 | // escribir / añadir (crear archivos y subcarpetas) / borrar hijos
	0x10000 | 0x40000 | 0x80000 | // DELETE, WRITE_DAC, WRITE_OWNER
	0x40000000 | 0x10000000 // GENERIC_WRITE, GENERIC_ALL

// othersCanWrite: alguien que no es de confianza tiene permiso de escritura.
func othersCanWrite(path string) bool {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return true
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return true
	}
	if dacl == nil {
		return true // sin lista de permisos: todos pueden todo
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(dacl, i, &ace) != nil || ace == nil {
			return true
		}
		const inheritOnly = 0x08 // no se aplica al propio objeto
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags&inheritOnly != 0 {
			continue
		}
		if uint32(ace.Mask)&writeMask == 0 {
			continue
		}
		if !trusted((*windows.SID)(unsafe.Pointer(&ace.SidStart))) {
			return true
		}
	}
	return false
}
