package steam

import (
	"errors"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Antes de abrir steam.exe (o pedirle que se cierre) se mira que lo firme Valve:
// si alguien cambiara la ruta de Steam en el registro por otro programa, TurboGAMES
// lo abriría con tu cuenta.

var (
	wintrust                   = windows.NewLazySystemDLL("wintrust.dll")
	procProvDataFromStateData  = wintrust.NewProc("WTHelperProvDataFromStateData")
	procGetProvSignerFromChain = wintrust.NewProc("WTHelperGetProvSignerFromChain")
)

type cryptProviderSgnr struct {
	Size         uint32
	VerifyAsOf   windows.Filetime
	CertChainLen uint32
	CertChain    *cryptProviderCert
}

type cryptProviderCert struct {
	Size uint32
	Cert *windows.CertContext
}

// ErrUnsigned: el steam.exe que hay no está firmado por Valve.
var ErrUnsigned = errors.New("el steam.exe de la carpeta de Steam no está firmado por Valve: no se abre (revisa la carpeta de Steam en Ajustes)")

var sigCache struct {
	sync.Mutex
	path string
	size int64
	mod  time.Time
	ok   bool
}

// checkSigned comprueba la firma de in.Exe. Lo que pasa se recuerda mientras el
// archivo no cambie; un fallo no (puede ser pasajero: Windows arrancando, el archivo
// ocupado un momento).
var checkSignedHook = func(exe string) error {
	st, err := os.Stat(exe)
	if err != nil {
		return err
	}
	sigCache.Lock()
	defer sigCache.Unlock()
	if sigCache.ok && sigCache.path == exe && sigCache.size == st.Size() && sigCache.mod.Equal(st.ModTime()) {
		return nil
	}
	name, err := verifiedSigner(exe)
	if err != nil || !strings.HasPrefix(name, "Valve Corp") {
		return ErrUnsigned
	}
	sigCache.path, sigCache.size, sigCache.mod, sigCache.ok = exe, st.Size(), st.ModTime(), true
	return nil
}

func (in *Install) checkSigned() error { return checkSignedHook(in.Exe) }

// CheckSigned: steam.exe está firmado por Valve (antes de empezar un cambio de cuenta).
func (in *Install) CheckSigned() error { return in.checkSigned() }

// verifiedSigner valida la firma Authenticode de un archivo y devuelve el nombre
// del firmante (el certificado que Windows ha validado, no otro de los que lleve).
func verifiedSigner(path string) (string, error) {
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
		// Sin ir a internet a por listas de revocación: sin red no se queda esperando.
		ProvFlags: windows.WTD_REVOCATION_CHECK_NONE | windows.WTD_CACHE_ONLY_URL_RETRIEVAL,
	}
	verr := windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	defer func() {
		data.StateAction = windows.WTD_STATEACTION_CLOSE
		windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	}()
	if verr != nil {
		return "", errors.New("firma digital no válida")
	}
	if procProvDataFromStateData.Find() != nil || procGetProvSignerFromChain.Find() != nil {
		return "", errors.New("no se pudo leer la firma")
	}
	prov, _, _ := procProvDataFromStateData.Call(uintptr(data.StateData))
	if prov == 0 {
		return "", errors.New("no se pudo leer la firma")
	}
	sp, _, _ := procGetProvSignerFromChain.Call(prov, 0, 0, 0)
	if sp == 0 {
		return "", errors.New("no se pudo leer la firma")
	}
	// Memoria de wintrust (no de Go), válida hasta cerrar el estado.
	sgnr := (*cryptProviderSgnr)(*(*unsafe.Pointer)(unsafe.Pointer(&sp)))
	if sgnr.CertChainLen == 0 || sgnr.CertChain == nil || sgnr.CertChain.Cert == nil {
		return "", errors.New("no se pudo leer la firma")
	}
	buf := make([]uint16, 256)
	n := windows.CertGetNameString(sgnr.CertChain.Cert, windows.CERT_NAME_SIMPLE_DISPLAY_TYPE, 0, nil, &buf[0], uint32(len(buf)))
	if n <= 1 {
		return "", errors.New("no se pudo leer la firma")
	}
	return windows.UTF16ToString(buf[:n]), nil
}
