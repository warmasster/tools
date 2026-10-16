package main

// Instalación "de doble clic": motor nativo + extensión + guía para cargarla en
// Chrome. Chrome en Windows no deja que un programa instale extensiones que no
// vienen de la Chrome Web Store (ni por línea de comandos desde la versión 137), así
// que lo más que se puede hacer es dejarlo todo preparado: la extensión copiada en
// una carpeta fija, su ruta en el portapapeles y chrome://extensions abierto.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// setup es lo que hace el doble clic en TurboDL.exe.
func setup() {
	msg, err := install(nil)
	if err != nil {
		messageBox("TurboDL", "No se pudo instalar el motor nativo:\n"+err.Error(), true)
		os.Exit(1)
	}
	extDir, err := installExtension()
	if err != nil {
		messageBox("TurboDL", "No se pudo copiar la extensión:\n"+err.Error(), true)
		os.Exit(1)
	}
	if extDir == "" { // ejecutable sin la extensión dentro: solo el motor
		messageBox("TurboDL", msg+"\n\nRecarga la extensión en chrome://extensions para que lo use.", false)
		return
	}

	loaded := chromeExtensionPaths()
	for _, p := range loaded {
		if strings.EqualFold(filepath.Clean(p), filepath.Clean(extDir)) {
			messageBox("TurboDL", "TurboDL actualizado.\n\nEn chrome://extensions pulsa el botón de recargar (↻) de TurboDL para usar la versión nueva.", false)
			return
		}
	}
	if len(loaded) > 0 {
		messageBox("TurboDL", fmt.Sprintf("Motor nativo instalado.\n\nYa tienes la extensión cargada desde:\n%s\n\nRecárgala en chrome://extensions (↻) para que use el motor.", loaded[0]), false)
		return
	}

	setClipboard(extDir)
	opened := openInChrome("chrome://extensions/")
	where := "En la pestaña de extensiones que se acaba de abrir:"
	if !opened {
		where = "Abre Chrome y escribe chrome://extensions en la barra de direcciones. Luego:"
	}
	messageBox("TurboDL · último paso", where+`

  1. Activa «Modo de desarrollador» (arriba a la derecha).
  2. Pulsa «Cargar descomprimida».
  3. Pega la ruta en la barra de la ventana (Ctrl+V; ya está copiada) y pulsa «Seleccionar carpeta».

La extensión está en:
`+extDir+`

Chrome no permite que un programa instale extensiones por su cuenta: este paso hay que darlo a mano una sola vez. Las próximas versiones se instalan con un doble clic y un ↻.`, false)
}

// chromeExtensionPaths devuelve desde qué carpetas está cargada TurboDL en los
// perfiles de Chrome (vacío si no está). Solo lee la configuración de los perfiles.
func chromeExtensionPaths() []string {
	base := filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "User Data")
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		for _, f := range []string{"Secure Preferences", "Preferences"} {
			raw, err := os.ReadFile(filepath.Join(base, e.Name(), f))
			if err != nil {
				continue
			}
			var prefs struct {
				Extensions struct {
					Settings map[string]struct {
						Path string `json:"path"`
					} `json:"settings"`
				} `json:"extensions"`
			}
			if json.Unmarshal(raw, &prefs) != nil {
				continue
			}
			if s, ok := prefs.Extensions.Settings[extensionID]; ok && s.Path != "" {
				out = append(out, s.Path)
			}
		}
	}
	return out
}

func findChrome() string {
	const key = `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\chrome.exe`
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		if k, err := registry.OpenKey(root, key, registry.QUERY_VALUE); err == nil {
			p, _, err := k.GetStringValue("")
			k.Close()
			if err == nil && p != "" {
				return p
			}
		}
	}
	for _, p := range []string{
		filepath.Join(os.Getenv("ProgramFiles"), `Google\Chrome\Application\chrome.exe`),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), `Google\Chrome\Application\chrome.exe`),
		filepath.Join(os.Getenv("LOCALAPPDATA"), `Google\Chrome\Application\chrome.exe`),
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// openInChrome abre una URL (también chrome://) en Chrome; si ya está abierto, en
// una pestaña nueva.
func openInChrome(url string) bool {
	chrome := findChrome()
	if chrome == "" {
		return false
	}
	return exec.Command(chrome, url).Start() == nil
}

func setClipboard(text string) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	u16, err := windows.UTF16FromString(text)
	if err != nil {
		return
	}
	if r, _, _ := user32.NewProc("OpenClipboard").Call(0); r == 0 {
		return
	}
	defer user32.NewProc("CloseClipboard").Call()
	user32.NewProc("EmptyClipboard").Call()
	const gmemMoveable, cfUnicodeText = 0x0002, 13
	size := uintptr(len(u16) * 2)
	h, _, _ := kernel32.NewProc("GlobalAlloc").Call(gmemMoveable, size)
	if h == 0 {
		return
	}
	p, _, _ := kernel32.NewProc("GlobalLock").Call(h)
	if p == 0 {
		return
	}
	kernel32.NewProc("RtlMoveMemory").Call(p, uintptr(unsafe.Pointer(&u16[0])), size)
	kernel32.NewProc("GlobalUnlock").Call(h)
	user32.NewProc("SetClipboardData").Call(cfUnicodeText, h)
}
