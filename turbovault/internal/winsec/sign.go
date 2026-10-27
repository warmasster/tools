package winsec

// Firma digital (Authenticode) de un ejecutable, para comprobar que quien lanza
// el puente de la extensión es de verdad un navegador.

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modWintrust                      = windows.NewLazySystemDLL("wintrust.dll")
	procWTHelperProvDataFromState    = modWintrust.NewProc("WTHelperProvDataFromStateData")
	procWTHelperGetProvSignerFromCh  = modWintrust.NewProc("WTHelperGetProvSignerFromChain")
	procWTHelperGetProvCertFromChain = modWintrust.NewProc("WTHelperGetProvCertFromChain")
)

type providerCert struct {
	Size uint32
	Cert *windows.CertContext
}

// Signer devuelve quién firmó el archivo si la firma es válida.
func Signer(path string) (string, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	fi := &windows.WinTrustFileInfo{Size: uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})), FilePath: p}
	data := &windows.WinTrustData{
		Size:                            uint32(unsafe.Sizeof(windows.WinTrustData{})),
		UIChoice:                        windows.WTD_UI_NONE,
		RevocationChecks:                windows.WTD_REVOKE_NONE,
		UnionChoice:                     windows.WTD_CHOICE_FILE,
		StateAction:                     windows.WTD_STATEACTION_VERIFY,
		FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(fi),
		ProvFlags:                       windows.WTD_REVOCATION_CHECK_NONE | windows.WTD_CACHE_ONLY_URL_RETRIEVAL,
	}
	verr := windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	defer func() {
		data.StateAction = windows.WTD_STATEACTION_CLOSE
		windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	}()
	if verr != nil {
		return "", errors.New("sin firma válida")
	}
	prov, _, _ := procWTHelperProvDataFromState.Call(uintptr(data.StateData))
	if prov == 0 {
		return "", errors.New("sin datos de firma")
	}
	sgnr, _, _ := procWTHelperGetProvSignerFromCh.Call(prov, 0, 0, 0)
	if sgnr == 0 {
		return "", errors.New("sin firmante")
	}
	pc, _, _ := procWTHelperGetProvCertFromChain.Call(sgnr, 0)
	if pc == 0 {
		return "", errors.New("sin certificado")
	}
	cert := (*(**providerCert)(unsafe.Pointer(&pc))).Cert // memoria de WinTrust
	buf := make([]uint16, 512)
	n := windows.CertGetNameString(cert, windows.CERT_NAME_SIMPLE_DISPLAY_TYPE, 0, nil, &buf[0], uint32(len(buf)))
	if n <= 1 {
		return "", errors.New("firmante sin nombre")
	}
	return windows.UTF16ToString(buf[:n]), nil
}

// Navegadores que pueden usar la extensión, con quién firma su ejecutable.
var browsers = map[string][]string{
	"chrome.exe":  {"Google LLC"},
	"msedge.exe":  {"Microsoft Corporation"},
	"brave.exe":   {"Brave Software, Inc."},
	"vivaldi.exe": {"Vivaldi Technologies AS"},
	"opera.exe":   {"Opera Norway AS"},
}

var (
	signCacheMu sync.Mutex
	signCache   = map[string]string{}
)

func cachedSigner(path string) string {
	signCacheMu.Lock()
	s, ok := signCache[strings.ToLower(path)]
	signCacheMu.Unlock()
	if ok {
		return s
	}
	s, _ = Signer(path)
	signCacheMu.Lock()
	signCache[strings.ToLower(path)] = s
	signCacheMu.Unlock()
	return s
}

// BrowserAncestor busca, subiendo por los procesos padre de pid, un navegador
// conocido y firmado por su fabricante (Chrome lanza los puentes a través de
// cmd.exe). Devuelve su nombre ("Chrome", "Edge"...).
func BrowserAncestor(pid uint32) (string, error) {
	name, _, err := BrowserProcess(pid)
	return name, err
}

// BrowserProcess es como BrowserAncestor pero devuelve también el PID del navegador.
func BrowserProcess(pid uint32) (string, uint32, error) {
	sys32 := filepath.Join(windowsDir(), "System32")
	cur := pid
	for depth := 0; depth < 3; depth++ {
		parent, err := ParentPID(cur)
		if err != nil || parent == 0 {
			break
		}
		path, err := ProcessPath(parent)
		if err != nil {
			break
		}
		base := strings.ToLower(filepath.Base(path))
		if signers, ok := browsers[base]; ok {
			got := cachedSigner(path)
			for _, s := range signers {
				if got == s {
					return browserName(base), parent, nil
				}
			}
			return "", 0, errors.New("el navegador no tiene la firma de su fabricante: " + path)
		}
		// Solo se admite cmd.exe del sistema como intermediario.
		if base == "cmd.exe" && SamePath(filepath.Dir(path), sys32) {
			cur = parent
			continue
		}
		break
	}
	return "", 0, errors.New("la conexión no viene de un navegador conocido")
}

var procPipeServerPID = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetNamedPipeServerProcessId")

// StdioPipeServers devuelve qué proceso creó la tubería de la entrada y la de la
// salida estándar de este proceso (0 si no son tuberías).
func StdioPipeServers() (in, out uint32) {
	get := func(which uint32) uint32 {
		h, err := windows.GetStdHandle(which)
		if err != nil || h == 0 || h == windows.InvalidHandle {
			return 0
		}
		var pid uint32
		if procPipeServerPID.Find() != nil {
			return 0
		}
		if r, _, _ := procPipeServerPID.Call(uintptr(h), uintptr(unsafe.Pointer(&pid))); r == 0 {
			return 0
		}
		return pid
	}
	return get(windows.STD_INPUT_HANDLE), get(windows.STD_OUTPUT_HANDLE)
}

func browserName(exe string) string {
	switch exe {
	case "chrome.exe":
		return "Chrome"
	case "msedge.exe":
		return "Edge"
	case "brave.exe":
		return "Brave"
	case "vivaldi.exe":
		return "Vivaldi"
	case "opera.exe":
		return "Opera"
	}
	return exe
}

func windowsDir() string {
	d, err := windows.GetSystemWindowsDirectory()
	if err != nil {
		return `C:\Windows`
	}
	return d
}
