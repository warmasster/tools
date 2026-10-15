//go:build !windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// En Linux y macOS el host se registra con un manifiesto en una carpeta conocida.
func manifestDirs() []string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		base := filepath.Join(home, "Library", "Application Support")
		return []string{
			filepath.Join(base, "Google", "Chrome", "NativeMessagingHosts"),
			filepath.Join(base, "Chromium", "NativeMessagingHosts"),
		}
	}
	return []string{
		filepath.Join(home, ".config", "google-chrome", "NativeMessagingHosts"),
		filepath.Join(home, ".config", "chromium", "NativeMessagingHosts"),
	}
}

func install(extraIDs []string) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	origins := []string{"chrome-extension://" + extensionID + "/"}
	for _, id := range extraIDs {
		origins = append(origins, "chrome-extension://"+id+"/")
	}
	raw, _ := json.MarshalIndent(map[string]any{
		"name": hostName, "description": "TurboDL: motor nativo de descargas",
		"path": self, "type": "stdio", "allowed_origins": origins,
	}, "", "  ")
	for _, d := range manifestDirs() {
		if os.MkdirAll(d, 0o755) == nil {
			os.WriteFile(filepath.Join(d, hostName+".json"), raw, 0o644)
		}
	}
	return fmt.Sprintf("Motor nativo registrado (%s)", self), nil
}

func uninstall() (string, error) {
	for _, d := range manifestDirs() {
		os.Remove(filepath.Join(d, hostName+".json"))
	}
	return "Motor nativo desinstalado.", nil
}
