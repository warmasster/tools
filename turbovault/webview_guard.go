//go:build !e2e

package main

import (
	"os"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/options"
)

// guardWebView borra las variables de entorno con las que se puede cambiar el
// navegador interno de la ventana (abrir un puerto de depuración, usar otro
// WebView2, otra carpeta de datos…). Un programa malicioso podría lanzar
// TurboVAULT con ellas para espiar la ventana.
func guardWebView() {
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(strings.ToUpper(k), "WEBVIEW2_") {
			os.Unsetenv(k)
		}
	}
}

func webviewOptions(*options.App) {}

func e2eHooks(*App) {}

// testEnv lee una variable de entorno de pruebas (TURBOVAULT_DIR,
// TURBOVAULT_PIPE). En TurboVAULT.exe normal no se hace caso: así nadie puede
// lanzarlo con otra carpeta de datos u otra tubería.
func testEnv(name string) string {
	if testing.Testing() {
		return os.Getenv(name)
	}
	return ""
}
