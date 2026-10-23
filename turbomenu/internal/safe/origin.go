package safe

import (
	"bufio"
	"bytes"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

const zoneStream = ":Zone.Identifier"

// Origin es la «marca de internet» que Windows guarda junto a los archivos descargados.
type Origin struct {
	Marked      bool      `json:"marked"`                // tiene marca
	ZoneID      int       `json:"zoneId"`                // 0 equipo, 1 intranet, 2 de confianza, 3 internet, 4 restringidos
	Zone        string    `json:"zone"`                  // nombre de la zona en español
	Referrer    string    `json:"referrer,omitempty"`    // página desde la que se descargó
	Host        string    `json:"host,omitempty"`        // enlace exacto del archivo
	Site        string    `json:"site,omitempty"`        // dominio, p. ej. «github.com»
	FromArchive string    `json:"fromArchive,omitempty"` // comprimido del que se extrajo (si se sabe)
	SavedBy     string    `json:"savedBy,omitempty"`     // app que lo guardó, si la anotó
	Created     time.Time `json:"created,omitzero"`      // fecha de creación del archivo (≈ descarga)
	Raw         string    `json:"raw,omitempty"`         // contenido tal cual de la marca
	Summary     string    `json:"summary"`
}

var zoneNames = map[int]string{
	0: "Este equipo",
	1: "Red local (intranet)",
	2: "Sitios de confianza",
	3: "Internet",
	4: "Sitios restringidos",
}

var archiveExt = map[string]bool{".zip": true, ".7z": true, ".rar": true, ".cab": true, ".tar": true,
	".gz": true, ".tgz": true, ".iso": true, ".img": true, ".vhd": true, ".vhdx": true}

// ReadOrigin lee de dónde vino el archivo. Sin marca devuelve Marked=false sin error.
func ReadOrigin(path string) (*Origin, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, errOpen(err)
	}
	o := &Origin{Created: creationTime(st)}
	data, err := os.ReadFile(path + zoneStream)
	if err != nil {
		o.Summary = "Este archivo no tiene origen registrado"
		return o, nil
	}
	parseZone(o, decodeText(data))
	return o, nil
}

// decodeText admite la marca en UTF-8/ANSI o en UTF-16 (algunos programas la escriben así).
func decodeText(b []byte) string {
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE || len(b) >= 4 && b[1] == 0 && b[3] == 0 {
		if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
			b = b[2:]
		}
		u := make([]uint16, len(b)/2)
		for i := range u {
			u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
		}
		return strings.ReplaceAll(string(utf16.Decode(u)), "\x00", "")
	}
	// Algunos navegadores dejan un carácter nulo al final.
	return strings.ReplaceAll(string(bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})), "\x00", "")
}

func parseZone(o *Origin, text string) {
	o.Marked = true
	o.Raw = strings.TrimSpace(text)
	o.ZoneID = -1
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "zoneid":
			if n, err := strconv.Atoi(v); err == nil {
				o.ZoneID = n
			}
		case "referrerurl":
			o.Referrer = v
		case "hosturl":
			o.Host = v
		case "lastwriterpackagefamilyname":
			o.SavedBy = appName(v)
		}
	}
	if o.ZoneID < 0 {
		o.ZoneID = 3 // marca sin zona: Windows la trata como internet
	}
	o.Zone = zoneNames[o.ZoneID]
	if o.Zone == "" {
		o.Zone = "Zona " + strconv.Itoa(o.ZoneID)
	}
	if o.Host == "about:internet" { // marca genérica, sin dirección real
		o.Host = ""
	}
	// Al extraer de un comprimido, Windows pone como referencia la ruta del comprimido.
	if isLocalPath(o.Referrer) {
		if archiveExt[strings.ToLower(filepath.Ext(o.Referrer))] {
			o.FromArchive = o.Referrer
		}
		o.Referrer = ""
	}
	if isLocalPath(o.Host) {
		if o.FromArchive == "" && archiveExt[strings.ToLower(filepath.Ext(o.Host))] {
			o.FromArchive = o.Host
		}
		o.Host = ""
	}
	// La página (Referrer) dice más al usuario que el servidor de descarga.
	for _, u := range []string{o.Referrer, o.Host} {
		if p, err := url.Parse(u); err == nil && p.Hostname() != "" {
			o.Site = strings.TrimPrefix(p.Hostname(), "www.")
			break
		}
	}
	o.Summary = originSummary(o)
}

func isLocalPath(s string) bool {
	return len(s) >= 3 && s[1] == ':' && (s[2] == '\\' || s[2] == '/') || strings.HasPrefix(s, `\\`) ||
		strings.HasPrefix(strings.ToLower(s), "file:")
}

// appName traduce el paquete que guardó el archivo a un nombre reconocible.
func appName(pkg string) string {
	low := strings.ToLower(pkg)
	switch {
	case strings.Contains(low, "outlook"), strings.Contains(low, "windowscommunicationsapps"):
		return "Correo (Outlook)"
	case strings.Contains(low, "msteams"), strings.Contains(low, "teams"):
		return "Microsoft Teams"
	case strings.Contains(low, "whatsapp"):
		return "WhatsApp"
	case strings.Contains(low, "telegram"):
		return "Telegram"
	case strings.Contains(low, "edge"):
		return "Microsoft Edge"
	}
	if i := strings.IndexByte(pkg, '_'); i > 0 {
		pkg = pkg[:i]
	}
	return pkg
}

func originSummary(o *Origin) string {
	switch {
	case o.FromArchive != "":
		return "Se sacó del comprimido «" + filepath.Base(o.FromArchive) + "», que vino de internet"
	case o.ZoneID == 0:
		return "Marcado como creado en este equipo"
	case o.ZoneID == 1:
		return "Llegó desde la red local"
	case o.ZoneID == 2:
		return "Llegó de un sitio marcado como de confianza"
	case o.Site != "":
		return "Se descargó de " + o.Site
	case o.SavedBy != "":
		return "Llegó de internet a través de " + o.SavedBy
	}
	return "Llegó de internet, pero el programa que lo guardó no anotó la dirección"
}

func creationTime(st os.FileInfo) time.Time {
	if d, ok := st.Sys().(*syscall.Win32FileAttributeData); ok {
		return time.Unix(0, d.CreationTime.Nanoseconds())
	}
	return time.Time{}
}

// Unblock quita la marca de internet (lo que hace «Desbloquear» en Propiedades).
// Si no tenía marca no hace nada.
func Unblock(path string) error {
	p, err := windows.UTF16PtrFromString(path + zoneStream)
	if err != nil {
		return errors.New("Ruta no válida")
	}
	err = windows.DeleteFile(p)
	switch {
	case err == nil, errors.Is(err, windows.ERROR_FILE_NOT_FOUND), errors.Is(err, windows.ERROR_PATH_NOT_FOUND):
		return nil
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		return errors.New("No hay permiso para desbloquear este archivo")
	case errors.Is(err, windows.ERROR_SHARING_VIOLATION):
		return errors.New("El archivo está en uso; ciérralo y vuelve a intentarlo")
	}
	return errors.New("No se pudo quitar la marca de internet")
}
