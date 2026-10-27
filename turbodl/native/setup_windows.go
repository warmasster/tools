package main

// Instalación "de doble clic": motor nativo + extensión + guía para cargarla en
// el navegador. Chrome (y Edge, y Brave) en Windows no deja que un programa instale
// extensiones que no vienen de su tienda (ni por línea de comandos desde la versión
// 137), así que lo más que se puede hacer es dejarlo todo preparado: la extensión
// copiada en una carpeta fija, su ruta en el portapapeles y la página de extensiones
// del navegador abierta.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
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
		messageBox("TurboDL", msg+"\n\nRecarga la extensión en la página de extensiones del navegador para que lo use.", false)
		return
	}

	loaded := chromeExtensionPaths()
	for _, p := range loaded {
		if strings.EqualFold(filepath.Clean(p), filepath.Clean(extDir)) {
			messageBox("TurboDL", "TurboDL actualizado.\n\nEn la página de extensiones del navegador (chrome://extensions, edge://extensions…) pulsa el botón de recargar (↻) de TurboDL para usar la versión nueva.", false)
			return
		}
	}
	if len(loaded) > 0 {
		messageBox("TurboDL", fmt.Sprintf("Motor nativo instalado.\n\nYa tienes la extensión cargada desde:\n%s\n\nRecárgala en la página de extensiones del navegador (↻) para que use el motor.", loaded[0]), false)
		return
	}

	setClipboard(extDir)
	b, opened := openExtensionsPage()
	messageBox("TurboDL · último paso", loadSteps(b, opened)+`

La extensión está en:
`+extDir+`

El navegador no permite que un programa instale extensiones por su cuenta: este paso hay que darlo a mano una sola vez. Las próximas versiones se instalan con un doble clic y un ↻.`, false)
}

// Carpetas de perfiles de los navegadores donde se registra el puente.
func browserDataDirs() []string {
	la := os.Getenv("LOCALAPPDATA")
	return []string{
		filepath.Join(la, "Google", "Chrome", "User Data"),
		filepath.Join(la, "Microsoft", "Edge", "User Data"),
		filepath.Join(la, "BraveSoftware", "Brave-Browser", "User Data"),
		filepath.Join(la, "Chromium", "User Data"),
	}
}

// chromeExtensionPaths devuelve desde qué carpetas está cargada TurboDL en los
// perfiles de los navegadores (vacío si no está). Solo lee su configuración.
func chromeExtensionPaths() []string {
	var out []string
	for _, base := range browserDataDirs() {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
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
	}
	return out
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
