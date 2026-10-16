package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// Claves donde cada navegador basado en Chromium busca los hosts de Native Messaging.
var browserKeys = []struct{ name, key string }{
	{"Chrome", `Software\Google\Chrome\NativeMessagingHosts\`},
	{"Edge", `Software\Microsoft\Edge\NativeMessagingHosts\`},
	{"Brave", `Software\BraveSoftware\Brave-Browser\NativeMessagingHosts\`},
	{"Chromium", `Software\Chromium\NativeMessagingHosts\`},
}

// install copia el ejecutable a %LOCALAPPDATA%\TurboDL, escribe el manifiesto del host
// y lo registra para el usuario actual (sin permisos de administrador).
func install(extraIDs []string) (string, error) {
	dir := AppDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	target := filepath.Join(dir, "turbodl-native.exe")
	if !strings.EqualFold(filepath.Clean(self), filepath.Clean(target)) {
		// Si Chrome tiene el host abierto el .exe está bloqueado, pero sí se puede
		// renombrar: lo apartamos y copiamos el nuevo.
		os.Remove(target + ".old")
		os.Rename(target, target+".old")
		if err := copyFile(self, target); err != nil {
			return "", fmt.Errorf("no se pudo copiar el programa a %s: %w", dir, err)
		}
	}

	origins := []string{"chrome-extension://" + extensionID + "/"}
	for _, id := range extraIDs {
		if id = strings.TrimSpace(id); id != "" && id != extensionID {
			origins = append(origins, "chrome-extension://"+id+"/")
		}
	}
	manifest := map[string]any{
		"name":            hostName,
		"description":     "TurboDL: motor nativo de descargas",
		"path":            target,
		"type":            "stdio",
		"allowed_origins": origins,
	}
	raw, _ := json.MarshalIndent(manifest, "", "  ")
	manifestPath := filepath.Join(dir, hostName+".json")
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		return "", err
	}

	var done []string
	for _, b := range browserKeys {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, b.key+hostName, registry.SET_VALUE)
		if err != nil {
			continue
		}
		if k.SetStringValue("", manifestPath) == nil {
			done = append(done, b.name)
		}
		k.Close()
	}
	if len(done) == 0 {
		return "", fmt.Errorf("no se pudo escribir en el registro")
	}
	return fmt.Sprintf("Motor nativo instalado en %s\nRegistrado para: %s", dir, strings.Join(done, ", ")), nil
}

func uninstall() (string, error) {
	for _, b := range browserKeys {
		registry.DeleteKey(registry.CURRENT_USER, b.key+hostName)
	}
	dir := AppDir()
	os.RemoveAll(ExtensionDir())
	os.Remove(filepath.Join(dir, hostName+".json"))
	os.Remove(filepath.Join(dir, "turbodl-native.exe.old"))
	if err := os.Remove(filepath.Join(dir, "turbodl-native.exe")); err != nil && !os.IsNotExist(err) {
		return "Registro borrado. El programa sigue en uso; bórralo a mano de " + dir + " al cerrar Chrome.", nil
	}
	return "Motor nativo desinstalado. Quita también TurboDL en chrome://extensions.", nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
