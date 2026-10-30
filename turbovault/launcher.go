package main

// Abrir la ventana. TurboVAULT.exe sin argumentos (menú Inicio, bandeja,
// extensión) no abre la ventana él mismo: le pide al agente que la abra. Así la
// única ventana con acceso a la bóveda es la que lanza el agente, protegida
// desde que nace: ningún otro programa ha tenido nunca un acceso completo a ella.

import (
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"turbovault/internal/ipc"
	"turbovault/internal/winsec"
)

var procAllowSetFg = user32.NewProc("AllowSetForegroundWindow")

func runLauncher(mode string) int {
	// Quien lanza esto suele ser el usuario (doble clic): que la ventana que
	// abra el agente pueda ponerse delante.
	procAllowSetFg.Call(uintptr(0xFFFFFFFF)) // ASFW_ANY
	conn, other, err := connectAgent(8 * time.Second)
	if err != nil {
		msg := "No se pudo arrancar TurboVAULT."
		if other != nil {
			msg = "Hay otro TurboVAULT abierto desde otra carpeta:\n" + other.Path + "\n\nCiérralo desde su icono de la bandeja y vuelve a intentarlo."
		}
		messageBox("TurboVAULT", msg, true)
		return 1
	}
	cl := ipc.NewClient(conn, nil)
	defer cl.Close()
	if err := cl.Call("openWindow", map[string]string{"mode": mode}, nil, 10*time.Second); err != nil {
		messageBox("TurboVAULT", "No se pudo abrir la ventana:\n"+err.Error(), true)
		return 1
	}
	return 0
}

// openWindow abre la ventana (o la trae delante si ya está abierta) y apunta
// su proceso como ventana legítima mientras viva.
func (a *Agent) openWindow(mode string) error {
	args := []string{"--window"}
	if mode == "unlock" || mode == "pair" || mode == "card" {
		args = append(args, "--"+mode)
	}
	pa, _ := winsec.ProcessAttributes()
	ta, _ := winsec.ThreadAttributes()
	cmd := exec.Command(a.exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags:     windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
		ProcessAttributes: pa,
		ThreadAttributes:  ta,
	}
	if err := winsec.WithNormalDefaults(cmd.Start); err != nil {
		logf("no se pudo abrir la ventana: %v", err)
		return err
	}
	pid := uint32(cmd.Process.Pid)
	a.winMu.Lock()
	a.windows[pid] = true
	a.winMu.Unlock()
	// Mientras no se recoja el proceso, su PID no se puede reutilizar.
	go func() {
		cmd.Wait()
		a.winMu.Lock()
		delete(a.windows, pid)
		a.winMu.Unlock()
	}()
	return nil
}

func (a *Agent) isWindow(pid uint32) bool {
	a.winMu.Lock()
	defer a.winMu.Unlock()
	return a.windows[pid]
}
