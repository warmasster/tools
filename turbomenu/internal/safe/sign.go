package safe

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// SignStatus resume el estado de la firma digital.
type SignStatus string

const (
	SignValid      SignStatus = "valid"      // firmada y la firma es correcta
	SignUnsigned   SignStatus = "unsigned"   // sin firma
	SignTampered   SignStatus = "tampered"   // el archivo cambió después de firmarse
	SignExpired    SignStatus = "expired"    // certificado caducado y sin sello de tiempo
	SignRevoked    SignStatus = "revoked"    // certificado revocado
	SignUntrusted  SignStatus = "untrusted"  // firmada por alguien en quien Windows no confía
	SignDistrusted SignStatus = "distrusted" // certificado marcado como no fiable
	SignInvalid    SignStatus = "invalid"    // firma no válida por otro motivo
)

// Signature es el resultado de comprobar la firma digital de un archivo.
type Signature struct {
	Status      SignStatus `json:"status"`
	Signer      string     `json:"signer,omitempty"`      // p. ej. «Valve Corporation»
	Issuer      string     `json:"issuer,omitempty"`      // autoridad que emitió el certificado
	Timestamp   time.Time  `json:"timestamp,omitzero"`    // fecha del sello de tiempo
	TimestampBy string     `json:"timestampBy,omitempty"` // quién puso el sello
	ValidFrom   time.Time  `json:"validFrom,omitzero"`
	ValidTo     time.Time  `json:"validTo,omitzero"`
	Catalog     bool       `json:"catalog"`               // firmado por catálogo (archivos del sistema)
	CatalogFile string     `json:"catalogFile,omitempty"` // ruta del .cat
	WellKnown   bool       `json:"wellKnown"`             // el firmante es un editor muy conocido
	Summary     string     `json:"summary"`               // frase lista para enseñar
}

var (
	modWintrust                      = windows.NewLazySystemDLL("wintrust.dll")
	procWTHelperProvDataFromState    = modWintrust.NewProc("WTHelperProvDataFromStateData")
	procWTHelperGetProvSignerFromCh  = modWintrust.NewProc("WTHelperGetProvSignerFromChain")
	procWTHelperGetProvCertFromChain = modWintrust.NewProc("WTHelperGetProvCertFromChain")
	procCATAcquireContext2           = modWintrust.NewProc("CryptCATAdminAcquireContext2")
	procCATAcquireContext            = modWintrust.NewProc("CryptCATAdminAcquireContext")
	procCATCalcHash2                 = modWintrust.NewProc("CryptCATAdminCalcHashFromFileHandle2")
	procCATCalcHash                  = modWintrust.NewProc("CryptCATAdminCalcHashFromFileHandle")
	procCATEnumFromHash              = modWintrust.NewProc("CryptCATAdminEnumCatalogFromHash")
	procCATInfoFromContext           = modWintrust.NewProc("CryptCATCatalogInfoFromContext")
	procCATReleaseCatalog            = modWintrust.NewProc("CryptCATAdminReleaseCatalogContext")
	procCATReleaseContext            = modWintrust.NewProc("CryptCATAdminReleaseContext")
)

// Estructuras de wintrust.h (solo los campos que se usan, con el mismo orden).
type providerSigner struct {
	Size             uint32
	VerifyAsOf       windows.Filetime
	CertChainCount   uint32
	CertChain        uintptr
	SignerType       uint32
	Signer           uintptr
	Error            uint32
	CounterSigners   uint32
	CounterSignerArr *providerSigner
	ChainContext     uintptr
}

type providerCert struct {
	Size uint32
	Cert *windows.CertContext
}

type catalogInfo struct {
	Size        uint32
	CatalogFile [windows.MAX_PATH]uint16
}

type wintrustCatalogInfo struct {
	Size             uint32
	CatalogVersion   uint32
	CatalogFilePath  *uint16
	MemberTag        *uint16
	MemberFilePath   *uint16
	MemberFile       windows.Handle
	CalcFileHash     *byte
	CalcFileHashSize uint32
	CatalogContext   uintptr
	CatAdmin         uintptr
}

// Códigos HRESULT de WinVerifyTrust.
const (
	trustNoSignature      = 0x800B0100
	trustSubjectUnknown   = 0x800B0003
	trustProviderUnknown  = 0x800B0001
	trustActionUnknown    = 0x800B0002
	trustBadDigest        = 0x80096010
	trustCertExpired      = 0x800B0101
	trustCertRevoked      = 0x800B010C
	trustUntrustedRoot    = 0x800B0109
	trustChaining         = 0x800B010A
	trustUntrustedTest    = 0x800B010D
	trustExplicitDistrust = 0x800B0111
	cryptFileError        = 0x80092003
)

// Límite para buscar en catálogos archivos que no son programas (evita leer vídeos enteros).
const catalogMaxPlain = 32 << 20

// CheckSignature comprueba la firma Authenticode del archivo y, si no la tiene,
// si Windows lo reconoce por catálogo. No consulta revocaciones por red.
func CheckSignature(ctx context.Context, path string) (*Signature, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, errOpen(err)
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, errors.New("Ruta no válida")
	}
	fi := &windows.WinTrustFileInfo{Size: uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})), FilePath: p}
	sig, code := verify(windows.WTD_CHOICE_FILE, unsafe.Pointer(fi))
	if code == cryptFileError {
		return nil, errors.New("No se pudo leer el archivo para comprobar la firma")
	}
	if sig.Status == SignUnsigned && ctx.Err() == nil && (st.Size() <= catalogMaxPlain || isPE(path)) {
		if cs := checkCatalog(path); cs != nil {
			sig = cs
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sig.WellKnown = sig.Status == SignValid && wellKnownPublisher(sig.Signer)
	sig.Summary = signSummary(sig)
	return sig, nil
}

// verify llama a WinVerifyTrust y saca el firmante real de los datos de estado.
func verify(choice uint32, info unsafe.Pointer) (*Signature, uint32) {
	data := &windows.WinTrustData{
		Size:                            uint32(unsafe.Sizeof(windows.WinTrustData{})),
		UIChoice:                        windows.WTD_UI_NONE,
		RevocationChecks:                windows.WTD_REVOKE_NONE,
		UnionChoice:                     choice,
		StateAction:                     windows.WTD_STATEACTION_VERIFY,
		FileOrCatalogOrBlobOrSgnrOrCert: info,
		ProvFlags:                       windows.WTD_REVOCATION_CHECK_NONE | windows.WTD_CACHE_ONLY_URL_RETRIEVAL,
	}
	verr := windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	defer func() {
		data.StateAction = windows.WTD_STATEACTION_CLOSE
		windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	}()

	var code uint32
	if verr != nil {
		var errno windows.Errno
		if errors.As(verr, &errno) {
			code = uint32(errno)
		} else {
			code = 0xFFFFFFFF
		}
	}
	sig := &Signature{Status: statusFromCode(code)}
	if sig.Status != SignUnsigned && data.StateData != 0 {
		fillSigner(sig, data.StateData)
	}
	return sig, code
}

func statusFromCode(code uint32) SignStatus {
	switch code {
	case 0:
		return SignValid
	case trustNoSignature, trustSubjectUnknown, trustProviderUnknown, trustActionUnknown, cryptFileError:
		return SignUnsigned
	case trustBadDigest:
		return SignTampered
	case trustCertExpired:
		return SignExpired
	case trustCertRevoked:
		return SignRevoked
	case trustUntrustedRoot, trustChaining, trustUntrustedTest:
		return SignUntrusted
	case trustExplicitDistrust:
		return SignDistrusted
	}
	return SignInvalid
}

// fillSigner lee el certificado del firmante (y del sello de tiempo) desde WinTrust.
func fillSigner(sig *Signature, state windows.Handle) {
	prov, _, _ := procWTHelperProvDataFromState.Call(uintptr(state))
	if prov == 0 {
		return
	}
	sgnr, _, _ := procWTHelperGetProvSignerFromCh.Call(prov, 0, 0, 0)
	if sgnr == 0 {
		return
	}
	s := *(**providerSigner)(unsafe.Pointer(&sgnr)) // memoria de WinTrust, no de Go
	if cert := signerCert(sgnr); cert != nil {
		sig.Signer = certName(cert, 0)
		sig.Issuer = certName(cert, windows.CERT_NAME_ISSUER_FLAG)
		sig.ValidFrom = time.Unix(0, cert.CertInfo.NotBefore.Nanoseconds())
		sig.ValidTo = time.Unix(0, cert.CertInfo.NotAfter.Nanoseconds())
	}
	if s.CounterSigners > 0 && s.CounterSignerArr != nil {
		// Con sello de tiempo, WinTrust verifica a la fecha del sello.
		sig.Timestamp = time.Unix(0, s.VerifyAsOf.Nanoseconds())
		if cert := signerCert(uintptr(unsafe.Pointer(s.CounterSignerArr))); cert != nil {
			sig.TimestampBy = certName(cert, 0)
		}
	}
}

func signerCert(sgnr uintptr) *windows.CertContext {
	pc, _, _ := procWTHelperGetProvCertFromChain.Call(sgnr, 0)
	if pc == 0 {
		return nil
	}
	return (*(**providerCert)(unsafe.Pointer(&pc))).Cert
}

func certName(cert *windows.CertContext, flags uint32) string {
	buf := make([]uint16, 512)
	n := windows.CertGetNameString(cert, windows.CERT_NAME_SIMPLE_DISPLAY_TYPE, flags, nil, &buf[0], uint32(len(buf)))
	if n <= 1 {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// checkCatalog busca el archivo en los catálogos de Windows y verifica el catálogo.
// Devuelve nil si no está en ninguno.
func checkCatalog(path string) *Signature {
	h, err := windows.CreateFile(windows.StringToUTF16Ptr(path), windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(h)

	// Catálogos modernos con SHA-256 y, si no, los antiguos con SHA-1.
	for _, alg := range []string{"SHA256", "SHA1"} {
		if sig := catalogLookup(path, h, alg); sig != nil {
			return sig
		}
	}
	return nil
}

func catalogLookup(path string, file windows.Handle, alg string) *Signature {
	var admin uintptr
	if procCATAcquireContext2.Find() == nil {
		a, _ := windows.UTF16PtrFromString(alg)
		r, _, _ := procCATAcquireContext2.Call(uintptr(unsafe.Pointer(&admin)), 0, uintptr(unsafe.Pointer(a)), 0, 0)
		if r == 0 {
			return nil
		}
	} else {
		if alg != "SHA1" {
			return nil
		}
		r, _, _ := procCATAcquireContext.Call(uintptr(unsafe.Pointer(&admin)), 0, 0)
		if r == 0 {
			return nil
		}
	}
	defer procCATReleaseContext.Call(admin, 0)

	if _, err := windows.Seek(file, 0, 0); err != nil {
		return nil
	}
	hash := make([]byte, 64)
	size := uint32(len(hash))
	var r uintptr
	if procCATCalcHash2.Find() == nil {
		r, _, _ = procCATCalcHash2.Call(admin, uintptr(file), uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&hash[0])), 0)
	} else {
		r, _, _ = procCATCalcHash.Call(uintptr(file), uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&hash[0])), 0)
	}
	if r == 0 || size == 0 {
		return nil
	}
	hash = hash[:size]

	cat, _, _ := procCATEnumFromHash.Call(admin, uintptr(unsafe.Pointer(&hash[0])), uintptr(size), 0, 0)
	if cat == 0 {
		return nil
	}
	defer procCATReleaseCatalog.Call(admin, cat, 0)

	ci := catalogInfo{Size: uint32(unsafe.Sizeof(catalogInfo{}))}
	if r, _, _ := procCATInfoFromContext.Call(cat, uintptr(unsafe.Pointer(&ci)), 0); r == 0 {
		return nil
	}
	tag, _ := windows.UTF16PtrFromString(strings.ToUpper(fmt.Sprintf("%x", hash)))
	member, _ := windows.UTF16PtrFromString(path)
	wci := &wintrustCatalogInfo{
		Size:             uint32(unsafe.Sizeof(wintrustCatalogInfo{})),
		CatalogFilePath:  &ci.CatalogFile[0],
		MemberTag:        tag,
		MemberFilePath:   member,
		MemberFile:       file,
		CalcFileHash:     &hash[0],
		CalcFileHashSize: size,
		CatAdmin:         admin,
	}
	sig, _ := verify(windows.WTD_CHOICE_CATALOG, unsafe.Pointer(wci))
	if sig.Status == SignUnsigned {
		return nil
	}
	sig.Catalog = true
	sig.CatalogFile = windows.UTF16ToString(ci.CatalogFile[:])
	return sig
}

// isPE indica si el archivo empieza como un programa de Windows (MZ).
func isPE(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	b := make([]byte, 2)
	n, _ := f.Read(b)
	return n == 2 && b[0] == 'M' && b[1] == 'Z'
}

// Editores grandes y conocidos: su firma válida pesa más en el veredicto.
var wellKnown = []string{
	"Microsoft", "Google", "Apple", "Mozilla", "Valve", "Adobe", "Intel", "NVIDIA",
	"Advanced Micro Devices", "AMD", "Oracle", "Amazon", "Meta Platforms", "Facebook",
	"Dropbox", "Zoom", "Spotify", "Discord", "Epic Games", "Electronic Arts", "Ubisoft",
	"Riot Games", "Blizzard", "Logitech", "Realtek", "Dell", "HP Inc", "Hewlett",
	"Lenovo", "ASUSTeK", "Samsung", "Sony", "Igor Pavlov", "VideoLAN", "Brave Software",
	"Opera Norway", "Telegram", "WhatsApp", "Cloudflare", "GitHub", "JetBrains",
	"Notepad++", "Python Software Foundation", "Node.js Foundation", "OpenJS Foundation",
	"Docker", "Corsair", "Razer", "Malwarebytes", "ESET", "Kaspersky", "Bitdefender",
	"Avast", "AVG", "Norton", "Gen Digital", "Symantec", "McAfee",
}

func wellKnownPublisher(name string) bool {
	n := strings.ToLower(name)
	for _, w := range wellKnown {
		if strings.HasPrefix(n, strings.ToLower(w)) {
			return true
		}
	}
	return false
}

func signSummary(s *Signature) string {
	who := s.Signer
	if who == "" {
		who = "un editor desconocido"
	}
	switch s.Status {
	case SignValid:
		if s.Catalog {
			return "Archivo de Windows firmado por " + who
		}
		return "Firmado por " + who
	case SignUnsigned:
		return "Sin firma digital"
	case SignTampered:
		return "La firma está rota: el archivo se modificó después de firmarlo"
	case SignExpired:
		return "Firmado por " + who + ", pero el certificado caducó"
	case SignRevoked:
		return "Firmado por " + who + ", pero ese certificado fue anulado"
	case SignUntrusted:
		return "Firmado por " + who + ", pero Windows no confía en ese certificado"
	case SignDistrusted:
		return "Firmado con un certificado que Windows marca como no fiable"
	}
	return "La firma digital no es válida"
}
