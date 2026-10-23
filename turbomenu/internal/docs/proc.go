package docs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

// hiddenCmd prepara un proceso hijo sin ventana que muere con el contexto.
func hiddenCmd(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW,
	}
	return cmd
}

// psQuote escribe s como literal de PowerShell entre comillas simples.
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// psPrelude prepara la consola y las ayudas para esperar operaciones WinRT.
const psPrelude = `
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
try { [Console]::OutputEncoding = [Text.Encoding]::UTF8 } catch {}
`

const psWinRT = `
Add-Type -AssemblyName System.Runtime.WindowsRuntime
$__ext = [System.WindowsRuntimeSystemExtensions]
$__op = $__ext.GetMethods() | ? { $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation` + "`" + `1' } | select -First 1
$__act = $__ext.GetMethods() | ? { $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncAction' } | select -First 1
function AwaitOp($op, [Type]$t) { $k = $__op.MakeGenericMethod($t).Invoke($null, @($op)); $null = $k.Wait(-1); $k.Result }
function AwaitAct($a) { $k = $__act.Invoke($null, @($a)); $null = $k.Wait(-1) }
function RAS($s) { [System.IO.WindowsRuntimeStreamExtensions]::AsRandomAccessStream($s) }
`

// runPS ejecuta un script de PowerShell oculto. Cada línea de la salida
// estándar se pasa a onLine (si no es nil). Devuelve la salida de error si falla.
func runPS(ctx context.Context, script string, onLine func(string)) error {
	full := psPrelude + script
	u := utf16.Encode([]rune(full))
	b := make([]byte, len(u)*2)
	for i, c := range u {
		b[2*i] = byte(c)
		b[2*i+1] = byte(c >> 8)
	}
	enc := base64.StdEncoding.EncodeToString(b)
	cmd := hiddenCmd(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive",
		"-ExecutionPolicy", "Bypass", "-EncodedCommand", enc)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return uerr(err, "No se ha podido iniciar PowerShell")
	}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	var lastErr string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "ERR ") {
			lastErr = strings.TrimPrefix(line, "ERR ")
		}
		if onLine != nil && line != "" {
			onLine(line)
		}
	}
	err = cmd.Wait()
	if ctx.Err() != nil {
		return ErrCanceled
	}
	if err != nil || lastErr != "" {
		msg := strings.TrimSpace(stderr.String())
		if lastErr != "" {
			msg = lastErr
		}
		return &psError{msg: msg, cause: err}
	}
	return nil
}

// psError guarda el mensaje técnico de PowerShell (no se enseña tal cual).
type psError struct {
	msg   string
	cause error
}

func (e *psError) Error() string { return "powershell: " + e.msg }
func (e *psError) Unwrap() error { return e.cause }

// psMessage devuelve el texto técnico de un error de PowerShell (o "").
func psMessage(err error) string {
	var pe *psError
	if errors.As(err, &pe) {
		return pe.msg
	}
	return ""
}
