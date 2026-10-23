package safe

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// DefenderResult es el resultado del análisis con Microsoft Defender.
type DefenderResult struct {
	Available bool          `json:"available"`         // false si Defender no está activo
	Clean     bool          `json:"clean"`             // true si no encontró nada
	Threats   []string      `json:"threats,omitempty"` // p. ej. «Virus:DOS/EICAR_Test_File»
	Duration  time.Duration `json:"duration"`
	Summary   string        `json:"summary"`
}

var threatRe = regexp.MustCompile(`(?m)^\s*Threat\s*:\s*(.+?)\s*$`)

// ScanDefender analiza el archivo con el Microsoft Defender del equipo, sin tocarlo
// (no lo pone en cuarentena). Si Defender no está activo devuelve Available=false.
func ScanDefender(ctx context.Context, path string) (*DefenderResult, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, errOpen(err)
	}
	exe := mpCmdRun()
	if exe == "" || !defenderRunning() {
		return &DefenderResult{Summary: "Microsoft Defender no está activo en este equipo (quizá uses otro antivirus)"}, nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	start := time.Now()
	cmd := exec.CommandContext(ctx, exe, "-Scan", "-ScanType", "3", "-File", abs, "-DisableRemediation")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	res := &DefenderResult{Available: true, Duration: time.Since(start)}
	code := 0
	if runErr != nil {
		var ee *exec.ExitError
		if !errors.As(runErr, &ee) {
			return nil, errors.New("No se pudo ejecutar Microsoft Defender")
		}
		code = ee.ExitCode()
	}
	return parseDefender(res, code, out.String())
}

// parseDefender interpreta el código de salida y el texto de MpCmdRun.
func parseDefender(res *DefenderResult, code int, text string) (*DefenderResult, error) {
	for _, m := range threatRe.FindAllStringSubmatch(text, -1) {
		res.Threats = append(res.Threats, m[1])
	}
	switch {
	case code == 2 || len(res.Threats) > 0:
		if len(res.Threats) == 0 {
			res.Threats = []string{"Amenaza sin nombre"}
		}
		res.Summary = "Microsoft Defender lo detecta como " + res.Threats[0]
	case code == 0:
		res.Clean = true
		res.Summary = "Microsoft Defender no encuentra nada"
	default:
		low := strings.ToLower(text)
		// 0x800106ba: el servicio no está disponible (otro antivirus lo sustituye).
		if strings.Contains(low, "0x800106ba") || strings.Contains(low, "not running") || strings.Contains(low, "disabled") {
			res.Available = false
			res.Summary = "Microsoft Defender no está activo en este equipo (quizá uses otro antivirus)"
			return res, nil
		}
		return nil, errors.New("Microsoft Defender no pudo analizar el archivo")
	}
	return res, nil
}

// mpCmdRun busca la versión más reciente de MpCmdRun.exe.
func mpCmdRun() string {
	if pd := os.Getenv("ProgramData"); pd != "" {
		dirs, _ := filepath.Glob(filepath.Join(pd, "Microsoft", "Windows Defender", "Platform", "*"))
		sort.Slice(dirs, func(i, j int) bool { return versionLess(filepath.Base(dirs[i]), filepath.Base(dirs[j])) })
		for i := len(dirs) - 1; i >= 0; i-- {
			if exe := filepath.Join(dirs[i], "MpCmdRun.exe"); fileExists(exe) {
				return exe
			}
		}
	}
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		pf = `C:\Program Files`
	}
	if exe := filepath.Join(pf, "Windows Defender", "MpCmdRun.exe"); fileExists(exe) {
		return exe
	}
	return ""
}

// versionLess compara versiones tipo «4.18.26080.4-0» número a número.
func versionLess(a, b string) bool {
	split := func(s string) []int {
		var out []int
		for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == '.' || r == '-' }) {
			n := 0
			for _, c := range p {
				if c < '0' || c > '9' {
					break
				}
				n = n*10 + int(c-'0')
			}
			out = append(out, n)
		}
		return out
	}
	x, y := split(a), split(b)
	for i := 0; i < len(x) && i < len(y); i++ {
		if x[i] != y[i] {
			return x[i] < y[i]
		}
	}
	return len(x) < len(y)
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// defenderRunning mira si el servicio WinDefend está en marcha.
func defenderRunning() bool {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return true // no se puede saber: se intenta igualmente
	}
	defer windows.CloseServiceHandle(scm)
	name, _ := windows.UTF16PtrFromString("WinDefend")
	svc, err := windows.OpenService(scm, name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST)
	}
	defer windows.CloseServiceHandle(svc)
	var st windows.SERVICE_STATUS
	if err := windows.QueryServiceStatus(svc, &st); err != nil {
		return true
	}
	return st.CurrentState == windows.SERVICE_RUNNING
}
