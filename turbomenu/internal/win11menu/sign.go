package win11menu

// Firma local del .msix con un certificado autofirmado creado en el propio PC
// (CurrentUser\My) y SignerSignEx2 de mssign32.dll, que viene con Windows.
// Así no hace falta signtool ni compilador. Para que Add-AppxPackage acepte el
// paquete, la parte pública del certificado debe estar en CurrentUser\TrustedPeople.

import (
	"encoding/hex"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modMssign32       = windows.NewLazySystemDLL("mssign32.dll")
	procSignerSignEx2 = modMssign32.NewProc("SignerSignEx2")
	procSignerFreeCtx = modMssign32.NewProc("SignerFreeSignerContext")
)

type signerFileInfo struct {
	cbSize   uint32
	fileName *uint16
	hFile    windows.Handle
}

type signerSubjectInfo struct {
	cbSize   uint32
	pdwIndex *uint32
	choice   uint32 // SIGNER_SUBJECT_FILE = 1
	file     *signerFileInfo
}

type signerCertStoreInfo struct {
	cbSize     uint32
	cert       *windows.CertContext
	certPolicy uint32 // SIGNER_CERT_POLICY_CHAIN = 2
	store      windows.Handle
}

type signerCert struct {
	cbSize uint32
	choice uint32 // SIGNER_CERT_STORE = 2
	store  *signerCertStoreInfo
	hwnd   uintptr
}

type signerSignatureInfo struct {
	cbSize          uint32
	algidHash       uint32 // CALG_SHA_256
	attrChoice      uint32 // SIGNER_NO_ATTR
	attr            uintptr
	authenticated   uintptr
	unauthenticated uintptr
}

type signerSignEx2Params struct {
	flags         uint32
	subject       *signerSubjectInfo
	cert          *signerCert
	sigInfo       *signerSignatureInfo
	provider      uintptr
	tsFlags       uint32
	tsOid         uintptr
	tsURL         uintptr
	cryptAttrs    uintptr
	sipData       uintptr
	signerContext *uintptr
	cryptoPolicy  uintptr
	reserved      uintptr
}

type appxSipClientData struct {
	params   *signerSignEx2Params
	sipState *iface
}

// findCert busca por huella (SHA-1 en hexadecimal) en CurrentUser\My.
func findCert(thumb string) (windows.Handle, *windows.CertContext, error) {
	thumb = strings.ReplaceAll(strings.ToLower(thumb), " ", "")
	if len(thumb) != 40 {
		return 0, nil, fmt.Errorf("huella no válida: %q", thumb)
	}
	hash, err := hex.DecodeString(thumb)
	if err != nil {
		return 0, nil, err
	}
	my, _ := windows.UTF16PtrFromString("MY")
	store, err := windows.CertOpenStore(windows.CERT_STORE_PROV_SYSTEM, 0, 0,
		windows.CERT_SYSTEM_STORE_CURRENT_USER|windows.CERT_STORE_OPEN_EXISTING_FLAG, uintptr(unsafe.Pointer(my)))
	if err != nil {
		return 0, nil, err
	}
	blob := windows.CryptHashBlob{Size: 20, Data: &hash[0]}
	cert, err := windows.CertFindCertificateInStore(store, windows.X509_ASN_ENCODING|windows.PKCS_7_ASN_ENCODING,
		0, windows.CERT_FIND_HASH, unsafe.Pointer(&blob), nil)
	if err != nil {
		windows.CertCloseStore(store, 0)
		return 0, nil, fmt.Errorf("no está el certificado %s en CurrentUser\\My: %w", thumb, err)
	}
	return store, cert, nil
}

// SignMSIX firma path (.msix) con el certificado de huella thumb de CurrentUser\My.
// El Subject del certificado debe ser igual al Publisher del manifiesto.
func SignMSIX(path, thumb string) error {
	store, cert, err := findCert(thumb)
	if err != nil {
		return err
	}
	defer windows.CertCloseStore(store, 0)
	defer windows.CertFreeCertificateContext(cert)

	name, _ := windows.UTF16PtrFromString(path)
	var index uint32
	file := signerFileInfo{fileName: name}
	file.cbSize = uint32(unsafe.Sizeof(file))
	subj := signerSubjectInfo{pdwIndex: &index, choice: 1, file: &file}
	subj.cbSize = uint32(unsafe.Sizeof(subj))
	csi := signerCertStoreInfo{cert: cert, certPolicy: 2}
	csi.cbSize = uint32(unsafe.Sizeof(csi))
	sc := signerCert{choice: 2, store: &csi}
	sc.cbSize = uint32(unsafe.Sizeof(sc))
	si := signerSignatureInfo{algidHash: 0x800c}
	si.cbSize = uint32(unsafe.Sizeof(si))
	var ctx uintptr
	params := signerSignEx2Params{subject: &subj, cert: &sc, sigInfo: &si, signerContext: &ctx}
	sip := appxSipClientData{params: &params}
	params.sipData = uintptr(unsafe.Pointer(&sip))

	r, _, _ := procSignerSignEx2.Call(0, uintptr(unsafe.Pointer(&subj)), uintptr(unsafe.Pointer(&sc)),
		uintptr(unsafe.Pointer(&si)), 0, 0, 0, 0, 0, uintptr(unsafe.Pointer(&sip)),
		uintptr(unsafe.Pointer(&ctx)), 0, 0)
	if sip.sipState != nil {
		sip.sipState.release()
	}
	if ctx != 0 {
		procSignerFreeCtx.Call(ctx)
	}
	if r != 0 {
		return fmt.Errorf("SignerSignEx2: 0x%08X", uint32(r))
	}
	return nil
}

// EnsureCert devuelve la huella de un certificado de firma autofirmado con ese
// Subject en CurrentUser\My, creándolo si no existe (sin admin).
func EnsureCert(subject string) (string, error) {
	out, err := psOut(fmt.Sprintf(`$c = Get-ChildItem Cert:\CurrentUser\My | Where-Object { $_.Subject -eq '%[1]s' -and $_.HasPrivateKey -and $_.NotAfter -gt (Get-Date).AddDays(30) } | Select-Object -First 1
if (-not $c) { $c = New-SelfSignedCertificate -Type Custom -Subject '%[1]s' -KeyUsage DigitalSignature -FriendlyName 'TurboMENU' -CertStoreLocation Cert:\CurrentUser\My -NotAfter (Get-Date).AddYears(20) -TextExtension @('2.5.29.37={text}1.3.6.1.5.5.7.3.3', '2.5.29.19={text}') }
$c.Thumbprint`, psq(subject)))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// TrustCert copia la parte pública del certificado a CurrentUser\TrustedPeople.
func TrustCert(thumb string) error {
	return powershell(fmt.Sprintf(`$c = Get-Item 'Cert:\CurrentUser\My\%[1]s'
$s = New-Object System.Security.Cryptography.X509Certificates.X509Store('TrustedPeople','CurrentUser')
$s.Open('ReadWrite'); $s.Add([System.Security.Cryptography.X509Certificates.X509Certificate2]::new($c.RawData)); $s.Close()`, psq(thumb)))
}

// RemoveCert borra el certificado de CurrentUser\My y de CurrentUser\TrustedPeople.
func RemoveCert(thumb string) error {
	return powershell(fmt.Sprintf(`Remove-Item -ErrorAction SilentlyContinue 'Cert:\CurrentUser\TrustedPeople\%[1]s', 'Cert:\CurrentUser\My\%[1]s'`, psq(thumb)))
}

// FindCert devuelve la huella del certificado con ese Subject (o "").
func FindCert(subject string) (string, error) {
	out, err := psOut(fmt.Sprintf(`(Get-ChildItem Cert:\CurrentUser\My | Where-Object { $_.Subject -eq '%s' } | Select-Object -First 1).Thumbprint`, psq(subject)))
	return strings.TrimSpace(out), err
}
