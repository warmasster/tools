//go:build !windows

package main

// Fuera de Windows solo se usa para compilar listas y para las pruebas.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func AppDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "TurboBLOCK")
}

func messageBox(title, text string, isError bool) { println(title + ": " + text) }

func launchedByDoubleClick() bool { return false }

func chromeExtensionPaths() []string { return nil }

func manifestDirs() []string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		base := filepath.Join(home, "Library", "Application Support")
		return []string{filepath.Join(base, "Google", "Chrome", "NativeMessagingHosts"), filepath.Join(base, "Chromium", "NativeMessagingHosts")}
	}
	return []string{filepath.Join(home, ".config", "google-chrome", "NativeMessagingHosts"), filepath.Join(home, ".config", "chromium", "NativeMessagingHosts")}
}

func install() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	raw, _ := json.MarshalIndent(map[string]any{
		"name": hostName, "description": "TurboBLOCK: actualiza las listas de filtros",
		"path": self, "type": "stdio", "allowed_origins": []string{"chrome-extension://" + extensionID + "/"},
	}, "", "  ")
	for _, d := range manifestDirs() {
		if os.MkdirAll(d, 0o755) == nil {
			os.WriteFile(filepath.Join(d, hostName+".json"), raw, 0o644)
		}
	}
	return fmt.Sprintf("Actualizador registrado (%s)", self), nil
}

func uninstall() (string, error) {
	for _, d := range manifestDirs() {
		os.Remove(filepath.Join(d, hostName+".json"))
	}
	os.RemoveAll(ExtensionDir())
	os.RemoveAll(ListsDir())
	return "TurboBLOCK desinstalado.", nil
}

func setup() {
	msg, err := install()
	if err != nil {
		println("Error:", err.Error())
		os.Exit(1)
	}
	dir, _ := installExtension()
	println(msg)
	if dir != "" {
		println("Extensión en " + dir + ": cárgala en chrome://extensions → Cargar descomprimida.")
	}
}
