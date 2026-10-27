package main

// Qué programas externos hay en este PC. La interfaz desactiva lo que dependa de algo
// que falte y explica qué hace falta.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

type Deps struct {
	TurboSEND    string `json:"turbosend"`    // ruta de TurboSEND.exe, "" si no está (o es antiguo)
	TurboSENDOld bool   `json:"turbosendOld"` // instalado, pero sin la vía de control (anterior a 2.1)
	TurboSETUP   string `json:"turbosetup"`   // para ofrecer instalar lo que falte
	FFmpeg       string `json:"ffmpeg"`       // motor de vídeo y audio
	Office       string `json:"office"`       // "Microsoft Office", "LibreOffice" o ""
	Cloudflared  string `json:"cloudflared"`  // túnel (si no está, se descarga al usarlo)
	VirusTotal   bool   `json:"virustotal"`   // hay clave guardada
	KnownLookup  string `json:"knownLookup"`  // "", "yes" o "no": consultar huellas en CIRCL
}

func detectDeps() Deps {
	d := Deps{
		TurboSEND:   turboTool("turbosend", "TurboSEND"),
		TurboSETUP:  turboSetupPath(),
		VirusTotal:  vtKey() != "",
		KnownLookup: getSettings().KnownLookup,
	}
	if d.TurboSEND != "" && versionLess(turboVersion("turbosend"), "2.1.0") {
		d.TurboSEND, d.TurboSENDOld = "", true
	}
	d.FFmpeg = ffmpegPath()
	d.Office = officeEngine()
	d.Cloudflared = cloudflaredPath()
	return d
}

func turboRoot() string {
	base := os.Getenv("LOCALAPPDATA")
	return filepath.Join(base, "Programs", "Turbo")
}

// turboTool busca una herramienta Turbo instalada por TurboSETUP (su entrada de
// desinstalación) o, si no, en su carpeta de siempre.
func turboTool(id, name string) string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Uninstall\Turbo.`+id, registry.QUERY_VALUE)
	if err == nil {
		exe, _, _ := k.GetStringValue("DisplayIcon")
		k.Close()
		if exe != "" && fileExists(exe) {
			return exe
		}
	}
	p := filepath.Join(turboRoot(), name, name+".exe")
	if fileExists(p) {
		return p
	}
	return ""
}

func turboSetupPath() string {
	p := filepath.Join(turboRoot(), "TurboSETUP.exe")
	if fileExists(p) {
		return p
	}
	return ""
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// cloudflaredPath reutiliza el de TurboSEND si existe; si no, el nuestro o el del sistema.
func cloudflaredPath() string {
	cands := []string{filepath.Join(binDir(), "cloudflared.exe")}
	if cfg, err := os.UserConfigDir(); err == nil {
		cands = append(cands, filepath.Join(cfg, "EnviadorV2", "bin", "cloudflared.exe"))
	}
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
		cands = append(cands, filepath.Join(os.Getenv(env), "cloudflared", "cloudflared.exe"))
	}
	for _, c := range cands {
		if fileExists(c) {
			return c
		}
	}
	if p, err := exec.LookPath("cloudflared"); err == nil {
		return p
	}
	return ""
}

// OpenTurboSETUP abre el instalador para que el usuario añada lo que falta.
func (a *App) OpenTurboSETUP() bool {
	p := turboSetupPath()
	if p == "" {
		a.OpenURL("https://github.com/warmasster/tools/releases/download/apps/TurboSETUP.exe")
		return false
	}
	exec.Command(p).Start()
	return true
}

func lowerExt(p string) string { return strings.TrimPrefix(strings.ToLower(filepath.Ext(p)), ".") }
