package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"
)

// Claves donde cada navegador basado en Chromium busca los hosts de Native Messaging.
var browserKeys = []struct{ name, key string }{
	{"Chrome", `Software\Google\Chrome\NativeMessagingHosts\`},
	{"Edge", `Software\Microsoft\Edge\NativeMessagingHosts\`},
	{"Brave", `Software\BraveSoftware\Brave-Browser\NativeMessagingHosts\`},
	{"Chromium", `Software\Chromium\NativeMessagingHosts\`},
}

// install copia el ejecutable a %LOCALAPPDATA%\TurboBLOCK, escribe el manifiesto
// del host y lo registra para el usuario actual (sin permisos de administrador).
func install() (string, error) {
	dir := AppDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	target := filepath.Join(dir, "turboblock-native.exe")
	if !strings.EqualFold(filepath.Clean(self), filepath.Clean(target)) {
		if err := placeExe(self, target); err != nil {
			return "", fmt.Errorf("no se pudo copiar el programa a %s: %w", dir, err)
		}
	}
	manifest := map[string]any{
		"name":            hostName,
		"description":     "TurboBLOCK: actualiza las listas de filtros",
		"path":            target,
		"type":            "stdio",
		"allowed_origins": []string{"chrome-extension://" + extensionID + "/"},
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
	return fmt.Sprintf("TurboBLOCK instalado en %s\nActualizador registrado para: %s", dir, strings.Join(done, ", ")), nil
}

func uninstall() (string, error) {
	for _, b := range browserKeys {
		registry.DeleteKey(registry.CURRENT_USER, b.key+hostName)
	}
	dir := AppDir()
	os.RemoveAll(ExtensionDir())
	os.RemoveAll(ExtensionDir() + ".old")
	os.RemoveAll(ListsDir())
	os.Remove(filepath.Join(dir, hostName+".json"))
	os.Remove(filepath.Join(dir, "native.log"))
	os.Remove(filepath.Join(dir, "native.log.1"))
	removeOld(dir)
	os.Remove(filepath.Join(dir, "turboblock-native.exe.new"))
	if err := os.Remove(filepath.Join(dir, "turboblock-native.exe")); err != nil && !os.IsNotExist(err) {
		return "Registro borrado. El programa sigue en uso; bórralo a mano de " + dir + " al cerrar Chrome.", nil
	}
	os.Remove(dir)
	return "TurboBLOCK desinstalado. Quita también la extensión en chrome://extensions.", nil
}

// removeOld borra los .old que dejaron actualizaciones con el host abierto.
func removeOld(dir string) {
	olds, _ := filepath.Glob(filepath.Join(dir, "*.old"))
	for _, o := range olds {
		os.Remove(o)
	}
}

// placeExe copia src a target sin dejar nunca el host roto: primero copia a
// .new y solo entonces aparta el viejo (Chrome puede tenerlo abierto, pero
// Windows deja renombrarlo) y pone el nuevo; si algo falla, vuelve el viejo.
func placeExe(src, target string) error {
	tmp := target + ".new"
	if err := copyFile(src, tmp); err != nil {
		os.Remove(tmp)
		return err
	}
	old := target + ".old"
	if os.Remove(old) != nil {
		if _, err := os.Stat(old); err == nil {
			old = fmt.Sprintf("%s.%d.old", target, time.Now().UnixNano()) // el .old anterior sigue abierto
		}
	}
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, old); err != nil {
			os.Remove(tmp)
			return err
		}
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Rename(old, target)
		os.Remove(tmp)
		return err
	}
	return nil
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
