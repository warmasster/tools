package main

// La cámara virtual es una DLL en C (camera/) que va dentro de TurboCAM.exe. Se saca a
// %LOCALAPPDATA%\TurboCAM\camera\<huella>\ y se registra en HKCU, sin administrador,
// como dispositivo de captura de vídeo de DirectShow (en 64 y en 32 bits, para que la
// vean también las apps de 32 bits).
//
// Cada versión va en su propia carpeta: si una app tiene cargada la DLL vieja, el
// archivo está bloqueado y no se podría sustituir. Las carpetas viejas se borran
// cuando ya nadie las usa.

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

//go:embed camera/bin/turbocam64.dll
var camDLL64 []byte

//go:embed camera/bin/turbocam32.dll
var camDLL32 []byte

const (
	camCLSID      = "{F5716436-EE88-4461-A891-B532EE167F6C}" // también en camera/turbocam.c
	camName       = "TurboCAM"
	videoCategory = "{860BB310-5D01-11D0-BD3B-00A0C911CE86}" // CLSID_VideoInputDeviceCategory
)

func camRoot() string { return filepath.Join(cacheDir(), "camera") }

// camHash identifica esta versión de las DLL.
func camHash() string {
	h := sha256.New()
	h.Write(camDLL64)
	h.Write(camDLL32)
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// installCamera deja las DLL en su sitio y registra la cámara. Se puede llamar
// siempre: si ya está todo bien, no toca nada.
func installCamera() error {
	dir := filepath.Join(camRoot(), camHash())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	dll64 := filepath.Join(dir, "turbocam64.dll")
	dll32 := filepath.Join(dir, "turbocam32.dll")
	for _, f := range []struct {
		path string
		data []byte
	}{{dll64, camDLL64}, {dll32, camDLL32}} {
		if err := writeIfDifferent(f.path, f.data); err != nil {
			return fmt.Errorf("no se pudo guardar la cámara virtual: %w", err)
		}
	}
	if err := registerCamera(registry.WOW64_64KEY, dll64); err != nil {
		return fmt.Errorf("no se pudo registrar la cámara virtual: %w", err)
	}
	// Sin la de 32 bits la cámara funciona igual en casi todo: no es un error grave.
	registerCamera(registry.WOW64_32KEY, dll32)
	if nativeCamInstalled() {
		// Con la cámara nativa de Windows 11, que ya sale en todas las listas (también
		// en las de DirectShow), esta se esconde: si no, habría dos «TurboCAM».
		hideCamera()
	}
	removeOldCameras(dir)
	return nil
}

func writeIfDifferent(path string, data []byte) error {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return nil
	}
	tmp := path + ".part"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func removeOldCameras(keep string) {
	entries, _ := os.ReadDir(camRoot())
	for _, e := range entries {
		p := filepath.Join(camRoot(), e.Name())
		if e.IsDir() && !strings.EqualFold(p, keep) {
			os.RemoveAll(p) // si alguna app la tiene cargada, falla y se intenta otro día
		}
	}
}

func registerCamera(view uint32, dll string) error {
	set := func(path string, values func(k registry.Key) error) error {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE|view)
		if err != nil {
			return err
		}
		defer k.Close()
		return values(k)
	}
	base := `Software\Classes\CLSID\`
	if err := set(base+camCLSID, func(k registry.Key) error {
		return k.SetStringValue("", camName)
	}); err != nil {
		return err
	}
	if err := set(base+camCLSID+`\InprocServer32`, func(k registry.Key) error {
		if err := k.SetStringValue("", dll); err != nil {
			return err
		}
		return k.SetStringValue("ThreadingModel", "Both")
	}); err != nil {
		return err
	}
	return set(base+videoCategory+`\Instance\`+camCLSID, func(k registry.Key) error {
		if err := k.SetStringValue("FriendlyName", camName); err != nil {
			return err
		}
		if err := k.SetStringValue("CLSID", camCLSID); err != nil {
			return err
		}
		return k.SetBinaryValue("FilterData", filterData())
	})
}

// uninstallCamera quita el registro de las dos vistas y borra las DLL que se pueda.
func uninstallCamera() {
	for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
		deleteTree(view, `Software\Classes\CLSID\`+videoCategory+`\Instance\`+camCLSID)
		deleteTree(view, `Software\Classes\CLSID\`+camCLSID)
	}
	os.RemoveAll(camRoot())
}

// hideCamera la quita de la lista de cámaras de DirectShow (sigue registrada).
func hideCamera() {
	for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
		deleteTree(view, `Software\Classes\CLSID\`+videoCategory+`\Instance\`+camCLSID)
	}
}

// cameraRegistered dice si la cámara de 64 bits está registrada y su DLL existe.
func cameraRegistered() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\CLSID\`+camCLSID+`\InprocServer32`,
		registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return false
	}
	defer k.Close()
	p, _, err := k.GetStringValue("")
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

func deleteTree(view uint32, path string) {
	if k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.ENUMERATE_SUB_KEYS|view); err == nil {
		subs, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, s := range subs {
			deleteTree(view, path+`\`+s)
		}
	}
	deleteKeyEx(registry.CURRENT_USER, path, view)
}

// ---------- FilterData ----------
//
// Es la descripción binaria del filtro (REGFILTER2, versión 2) que guarda
// IFilterMapper2::RegisterFilter. Como esa función escribe en HKLM, aquí se
// construye a mano: una patilla de salida de vídeo, de la categoría «captura».

var (
	guidVideo   = mustGUID("{73646976-0000-0010-8000-00AA00389B71}") // MEDIATYPE_Video
	guidNull    = mustGUID("{00000000-0000-0000-0000-000000000000}") // cualquier subtipo
	guidCapture = mustGUID("{FB6C4281-0353-11D1-905F-0000C0CC16BA}") // PIN_CATEGORY_CAPTURE
)

const (
	meritDoNotUse    = 0x00200000 // como las demás cámaras: no la elige nadie solo
	regPinFlagOutput = 0x8
)

func filterData() []byte {
	var b bytes.Buffer
	w := func(v uint32) { binary.Write(&b, binary.LittleEndian, v) }
	// Cabecera, patilla («0pi3»), su categoría y su único tipo («0ty3»); detrás, los GUID.
	const mainSize = 16 + 24 + 4 + 16
	offCategory, offMajor, offMinor := uint32(mainSize), uint32(mainSize+16), uint32(mainSize+32)
	w(2)
	w(meritDoNotUse)
	w(1) // patillas
	w(0)
	b.WriteString("0pi3")
	w(regPinFlagOutput)
	w(0) // instancias
	w(1) // tipos
	w(0) // medios
	w(1) // tiene categoría
	w(offCategory)
	b.WriteString("0ty3")
	w(0)
	w(offMajor)
	w(offMinor)
	b.Write(guidCapture)
	b.Write(guidVideo)
	b.Write(guidNull)
	return b.Bytes()
}

// mustGUID pasa "{XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX}" a los 16 bytes de memoria.
func mustGUID(s string) []byte {
	g, err := parseGUID(s)
	if err != nil {
		panic(err)
	}
	return g
}

func parseGUID(s string) ([]byte, error) {
	s = strings.Trim(s, "{}")
	raw, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	if err != nil || len(raw) != 16 || len(s) != 36 {
		return nil, errors.New("GUID no válido: " + s)
	}
	out := make([]byte, 16)
	binary.LittleEndian.PutUint32(out[0:], binary.BigEndian.Uint32(raw[0:]))
	binary.LittleEndian.PutUint16(out[4:], binary.BigEndian.Uint16(raw[4:]))
	binary.LittleEndian.PutUint16(out[6:], binary.BigEndian.Uint16(raw[6:]))
	copy(out[8:], raw[8:])
	return out, nil
}
