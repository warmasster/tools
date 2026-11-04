// TurboMON: FPS, sensores y overlay para jugar, y una revisión de que el PC
// está configurado para rendir lo que debe.
//
//	TurboMON.exe                  la ventana (arranca el agente si no está)
//	TurboMON.exe --agent          el agente de la bandeja (arranca con Windows)
//	TurboMON.exe setup|uninstall  lo que usa TurboSETUP
//	TurboMON.exe setup-admin …    lo que se hace con permiso de administrador
package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"turbomon/internal/noise"
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

func main() {
	args := os.Args[1:]
	if len(args) > 0 {
		switch strings.ToLower(args[0]) {
		case "--agent":
			os.Exit(runAgent())
		case "--noise-probe":
			os.Exit(noise.RunProbe(args[1:], os.Stdout))
		case "setup":
			os.Exit(cliSetup())
		case "uninstall":
			os.Exit(cliUninstall())
		case "setup-admin", "uninstall-admin":
			res := ""
			if len(args) > 1 {
				res = args[1]
			}
			os.Exit(cliAdmin(strings.ToLower(args[0]), res))
		case "version", "--version":
			fmt.Println("TurboMON", version)
			return
		}
	}
	runUI()
}
