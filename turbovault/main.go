// TurboVAULT: gestor de contraseñas local con extensión para Chrome.
//
//	TurboVAULT.exe                 pedir al agente que abra la ventana (y arrancarlo si no está)
//	TurboVAULT.exe --window        la ventana (solo la lanza el agente)
//	TurboVAULT.exe --agent         el agente de la bandeja (arranca con Windows)
//	TurboVAULT.exe --unlock        ventana pequeña para abrir la bóveda
//	TurboVAULT.exe --pair          ventana para vincular un navegador
//	TurboVAULT.exe --window --card ventana para confirmar una tarjeta (solo la abre el agente)
//	TurboVAULT.exe setup|uninstall lo que usa TurboSETUP
//	TurboVAULT.exe chrome-extension://…  puente de la extensión (lo lanza Chrome)
package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed all:frontend
var assets embed.FS

//go:embed wails.json
var wailsJSON []byte

// version sale de wails.json (info.productVersion), que es lo que publica el workflow.
var version = func() string {
	var w struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	if json.Unmarshal(wailsJSON, &w) == nil && w.Info.ProductVersion != "" {
		return w.Info.ProductVersion
	}
	return "0.0.0"
}()

const (
	hostName    = "com.turbovault.native"
	extensionID = "pchkjmmbookdihnaekobdmkmimknihbo" // fijado con "key" en extension/manifest.json
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 && strings.HasPrefix(strings.ToLower(args[0]), "chrome-extension://") {
		os.Exit(runHost(args[0]))
	}
	mode := ""
	if len(args) > 0 {
		switch strings.ToLower(args[0]) {
		case "--agent":
			os.Exit(runAgent())
		case "setup":
			os.Exit(cliSetup())
		case "uninstall":
			os.Exit(cliUninstall())
		case "version", "--version":
			fmt.Println("TurboVAULT", version)
			return
		case "--window":
			// La ventana de verdad: solo la abre el agente.
			if len(args) > 1 {
				mode = strings.TrimPrefix(strings.ToLower(args[1]), "--")
			}
			runUI(mode)
			return
		case "--unlock":
			mode = "unlock"
		case "--pair":
			mode = "pair"
		}
	}
	os.Exit(runLauncher(mode))
}

// logf escribe en el registro técnico (nunca secretos), que se recorta solo.
func logf(format string, a ...any) {
	dir := localDir()
	os.MkdirAll(dir, 0o700)
	p := filepath.Join(dir, "turbovault.log")
	if st, err := os.Stat(p); err == nil && st.Size() > 1<<20 {
		os.Rename(p, p+".1")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s [%d] %s\n", time.Now().Format("2006-01-02 15:04:05"), os.Getpid(), fmt.Sprintf(format, a...))
}
