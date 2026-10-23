package win11menu

// Paquete MSIX «sparse» sin firmar, generado a mano (sin makeappx): ZIP sin
// compresión con AppxManifest.xml, los logos, AppxBlockMap.xml y [Content_Types].xml.

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// UnsignedPublisherOID: con este OID en el Publisher, Windows 11 deja instalar
// el paquete sin firma (Add-AppxPackage -AllowUnsigned).
const UnsignedPublisherOID = "OID.2.25.311729368913984317654407730594956997722=1"

// Package describe el paquete de identidad.
type Package struct {
	Name        string // identidad, p. ej. "Turbo.TurboMENU"
	Publisher   string // por defecto "CN=Turbo, " + UnsignedPublisherOID
	DisplayName string
	Description string
	Version     string // "1.0.0.0"
	AppID       string // por defecto "TurboMENU"
	ExePath     string // .exe completo; su carpeta es la ExternalLocation
	Logo        []byte // PNG (opcional)
	// CertThumb: huella de un certificado de CurrentUser\My para firmar el
	// paquete (Publisher debe ser su Subject). Vacío = sin firmar, con el OID
	// especial y -AllowUnsigned, que para paquetes con ejecutables exige admin.
	CertThumb string
	// Dónde sale la entrada; si los tres son false, salen todos.
	Files, Folders, Background bool
}

func (p *Package) defaults() {
	if p.Publisher == "" {
		p.Publisher = "CN=Turbo, " + UnsignedPublisherOID
		if p.CertThumb != "" {
			p.Publisher = "CN=TurboMENU"
		}
	}
	if p.Version == "" {
		p.Version = "1.0.0.0"
	}
	if p.AppID == "" {
		p.AppID = "TurboMENU"
	}
	if p.DisplayName == "" {
		p.DisplayName = p.AppID
	}
	if p.Description == "" {
		p.Description = p.DisplayName
	}
	if !p.Files && !p.Folders && !p.Background {
		p.Files, p.Folders, p.Background = true, true, true
	}
}

func xmlEsc(s string) string {
	var b bytes.Buffer
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ExeManifestElement es el elemento <msix> que debe llevar el manifiesto
// embebido del .exe (dentro de <assembly>) para tener la identidad del paquete.
func (p Package) ExeManifestElement() string {
	p.defaults()
	return fmt.Sprintf(`<msix xmlns="urn:schemas-microsoft-com:msix.v1" publisher="%s" packageName="%s" applicationId="%s"/>`,
		xmlEsc(p.Publisher), xmlEsc(p.Name), xmlEsc(p.AppID))
}

// Manifest genera AppxManifest.xml.
func (p Package) Manifest() string {
	p.defaults()
	exe := filepath.Base(p.ExePath)
	items := CLSIDItems.String()
	bg := CLSIDBackground.String()
	trim := func(s string) string { return strings.Trim(s, "{}") }
	var types strings.Builder
	if p.Files {
		fmt.Fprintf(&types, "            <desktop5:ItemType Type=\"*\"><desktop5:Verb Id=\"TurboFiles\" Clsid=\"%s\"/></desktop5:ItemType>\n", trim(items))
	}
	if p.Folders {
		fmt.Fprintf(&types, "            <desktop5:ItemType Type=\"Directory\"><desktop5:Verb Id=\"TurboDirs\" Clsid=\"%s\"/></desktop5:ItemType>\n", trim(items))
	}
	if p.Background {
		fmt.Fprintf(&types, "            <desktop5:ItemType Type=\"Directory\\Background\"><desktop5:Verb Id=\"TurboBack\" Clsid=\"%s\"/></desktop5:ItemType>\n", trim(bg))
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10"
  xmlns:uap="http://schemas.microsoft.com/appx/manifest/uap/windows10"
  xmlns:uap10="http://schemas.microsoft.com/appx/manifest/uap/windows10/10"
  xmlns:desktop4="http://schemas.microsoft.com/appx/manifest/desktop/windows10/4"
  xmlns:desktop5="http://schemas.microsoft.com/appx/manifest/desktop/windows10/5"
  xmlns:com="http://schemas.microsoft.com/appx/manifest/com/windows10"
  xmlns:rescap="http://schemas.microsoft.com/appx/manifest/foundation/windows10/restrictedcapabilities"
  IgnorableNamespaces="uap uap10 desktop4 desktop5 com rescap">
  <Identity Name="%[1]s" Publisher="%[2]s" Version="%[3]s" ProcessorArchitecture="neutral"/>
  <Properties>
    <DisplayName>%[4]s</DisplayName>
    <PublisherDisplayName>%[4]s</PublisherDisplayName>
    <Logo>logo.png</Logo>
    <uap10:AllowExternalContent>true</uap10:AllowExternalContent>
  </Properties>
  <Resources>
    <Resource Language="es-es"/>
  </Resources>
  <Dependencies>
    <TargetDeviceFamily Name="Windows.Desktop" MinVersion="10.0.22000.0" MaxVersionTested="10.0.26200.0"/>
  </Dependencies>
  <Capabilities>
    <rescap:Capability Name="runFullTrust"/>
    <rescap:Capability Name="unvirtualizedResources"/>
  </Capabilities>
  <Applications>
    <Application Id="%[5]s" Executable="%[6]s" uap10:TrustLevel="mediumIL" uap10:RuntimeBehavior="win32App">
      <uap:VisualElements AppListEntry="none" DisplayName="%[4]s" Description="%[7]s" BackgroundColor="transparent" Square150x150Logo="logo.png" Square44x44Logo="logo.png"/>
      <Extensions>
        <desktop4:Extension Category="windows.fileExplorerContextMenus">
          <desktop4:FileExplorerContextMenus>
%[8]s          </desktop4:FileExplorerContextMenus>
        </desktop4:Extension>
        <com:Extension Category="windows.comServer">
          <com:ComServer>
            <com:ExeServer Executable="%[6]s" Arguments="%[9]s" DisplayName="%[4]s">
              <com:Class Id="%[10]s" DisplayName="%[4]s"/>
              <com:Class Id="%[11]s" DisplayName="%[4]s (fondo)"/>
            </com:ExeServer>
          </com:ComServer>
        </com:Extension>
      </Extensions>
    </Application>
  </Applications>
</Package>
`, xmlEsc(p.Name), xmlEsc(p.Publisher), xmlEsc(p.Version), xmlEsc(p.DisplayName),
		xmlEsc(p.AppID), xmlEsc(exe), xmlEsc(p.Description), types.String(), ServerArg,
		trim(items), trim(bg))
}

// defaultLogo: PNG de 44x44 con el degradado violeta-cian.
func defaultLogo() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 44, 44))
	for y := 0; y < 44; y++ {
		for x := 0; x < 44; x++ {
			t := float64(x+y) / 86
			img.Set(x, y, color.NRGBA{uint8(124 + t*(34-124)), uint8(58 + t*(211-58)), uint8(237 + t*(238-237)), 255})
		}
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

// WriteMSIX escribe el paquete .msix sin firmar.
func (p Package) WriteMSIX(w io.Writer) error {
	p.defaults()
	return WriteMSIXManifest(w, p.Manifest(), p.Logo)
}

// WriteMSIXManifest escribe un .msix sin firmar con un manifiesto dado.
func WriteMSIXManifest(w io.Writer, manifest string, logo []byte) error {
	if len(logo) == 0 {
		logo = defaultLogo()
	}
	files := []struct {
		name string
		data []byte
	}{
		{"AppxManifest.xml", []byte(manifest)},
		{"logo.png", logo},
	}

	// Mapa de bloques: SHA-256 de cada bloque de 64 KB (sin comprimir).
	var bm strings.Builder
	bm.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	bm.WriteString(`<BlockMap xmlns="http://schemas.microsoft.com/appx/2010/blockmap" HashMethod="http://www.w3.org/2001/04/xmlenc#sha256">` + "\n")
	for _, f := range files {
		fmt.Fprintf(&bm, `<File Name="%s" Size="%d" LfhSize="%d">`, f.name, len(f.data), 30+len(f.name))
		for off := 0; off < len(f.data) || off == 0; off += 65536 {
			end := min(off+65536, len(f.data))
			h := sha256.Sum256(f.data[off:end])
			fmt.Fprintf(&bm, `<Block Hash="%s"/>`, base64.StdEncoding.EncodeToString(h[:]))
			if end == len(f.data) {
				break
			}
		}
		bm.WriteString("</File>\n")
	}
	bm.WriteString("</BlockMap>\n")

	ct := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="png" ContentType="image/png"/>` +
		`<Override PartName="/AppxManifest.xml" ContentType="application/vnd.ms-appx.manifest+xml"/>` +
		`<Override PartName="/AppxBlockMap.xml" ContentType="application/vnd.ms-appx.blockmap+xml"/>` +
		`</Types>`

	all := append(files, struct {
		name string
		data []byte
	}{"AppxBlockMap.xml", []byte(bm.String())}, struct {
		name string
		data []byte
	}{"[Content_Types].xml", []byte(ct)})

	zw := zip.NewWriter(w)
	for _, f := range all {
		// Sin compresión ni descriptor de datos: la cabecera local mide 30+nombre,
		// como dice LfhSize en el mapa de bloques.
		fw, err := zw.CreateRaw(&zip.FileHeader{
			Name:               f.name,
			CreatorVersion:     45,
			ReaderVersion:      20,
			Method:             zip.Store,
			CRC32:              crc32.ChecksumIEEE(f.data),
			CompressedSize64:   uint64(len(f.data)),
			UncompressedSize64: uint64(len(f.data)),
		})
		if err != nil {
			return err
		}
		if _, err := fw.Write(f.data); err != nil {
			return err
		}
	}
	return zw.Close()
}

// Register crea el .msix en una carpeta temporal y lo instala para el usuario
// actual con la carpeta del .exe como ExternalLocation. Sin admin ni modo desarrollador.
func Register(p Package) error {
	p.defaults()
	if p.Name == "" || p.ExePath == "" {
		return fmt.Errorf("faltan Name o ExePath")
	}
	dir, err := os.MkdirTemp("", "turbomenu-msix")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	msix := filepath.Join(dir, "TurboMENU.msix")
	f, err := os.Create(msix)
	if err != nil {
		return err
	}
	if err := p.WriteMSIX(f); err != nil {
		f.Close()
		return err
	}
	f.Close()
	flags := "-AllowUnsigned"
	if p.CertThumb != "" {
		if err := SignMSIX(msix, p.CertThumb); err != nil {
			return err
		}
		flags = ""
	}
	ext := filepath.Dir(p.ExePath)
	return powershell(fmt.Sprintf(
		"Add-AppxPackage -Path '%s' -ExternalLocation '%s' %s -ForceUpdateFromAnyVersion -ForceApplicationShutdown",
		psq(msix), psq(ext), flags))
}

// Unregister quita el paquete (si está).
func Unregister(name string) error {
	return powershell(fmt.Sprintf(
		"Get-AppxPackage -Name '%s' | Remove-AppxPackage", psq(name)))
}

// IsRegistered dice si el paquete está instalado y devuelve su versión.
func IsRegistered(name string) (bool, string) {
	out, err := psOut(fmt.Sprintf("(Get-AppxPackage -Name '%s').Version", psq(name)))
	v := strings.TrimSpace(out)
	return err == nil && v != "", v
}

func psq(s string) string { return strings.ReplaceAll(s, "'", "''") }

func psOut(script string) (string, error) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command",
		"$ErrorActionPreference='Stop'; $ProgressPreference='SilentlyContinue'; "+script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func powershell(script string) error {
	_, err := psOut(script)
	return err
}
