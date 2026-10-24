package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"turbovault/internal/ipc"
	"turbovault/internal/winsec"
)

// pipeName es la tubería del agente. TURBOVAULT_PIPE la cambia (pruebas).
func pipeName() string { return ipc.Name(testEnv("TURBOVAULT_PIPE")) }

func selfExe() string {
	exe, _ := os.Executable()
	return exe
}

// spawnDetached lanza otra copia de este programa sin consola y fuera del
// trabajo del proceso padre (Chrome cierra los suyos al desconectar).
func spawnDetached(args ...string) error {
	flags := uint32(windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP)
	// Nacen ya protegidos: nadie tiene un acceso completo al proceso antes de
	// que se proteja él mismo.
	pa, _ := winsec.ProcessAttributes()
	ta, _ := winsec.ThreadAttributes()
	start := func(extra uint32) error {
		cmd := exec.Command(selfExe(), args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags | extra, ProcessAttributes: pa, ThreadAttributes: ta}
		if err := cmd.Start(); err != nil {
			return err
		}
		cmd.Process.Release()
		return nil
	}
	if err := start(windows.CREATE_BREAKAWAY_FROM_JOB); err == nil {
		return nil
	}
	return start(0)
}

// spawnUI abre la ventana (o la trae delante si ya está abierta).
func spawnUI(args ...string) {
	if err := spawnDetached(args...); err != nil {
		logf("no se pudo abrir la ventana: %v", err)
	}
}

// verifyAgent comprueba que el servidor de la tubería es este mismo programa
// (el mismo archivo, no solo la misma ruta).
func verifyAgent(pid uint32) error {
	path, id, err := winsec.ProcessFileID(pid)
	if err != nil {
		return err
	}
	self, err := winsec.FileIDOf(selfExe())
	if err != nil {
		return err
	}
	if !winsec.SamePath(path, selfExe()) || id != self {
		return fmt.Errorf("lo atiende otro programa: %s", path)
	}
	return nil
}

// otherAgent describe un agente de otra carpeta (otra versión de TurboVAULT).
type otherAgent struct {
	PID  uint32
	Path string
}

var errOtherAgent = errors.New("hay otro TurboVAULT abierto desde otra carpeta")

// connectAgent se conecta al agente y, si no está en marcha, lo arranca.
func connectAgent(timeout time.Duration) (*ipc.Conn, *otherAgent, error) {
	name := pipeName()
	var other *otherAgent
	verify := func(pid uint32) error {
		err := verifyAgent(pid)
		if err != nil {
			path, _ := winsec.ProcessPath(pid)
			other = &otherAgent{PID: pid, Path: path}
		}
		return err
	}
	c, err := ipc.Dial(name, 300*time.Millisecond, verify)
	if err == nil {
		return c, nil, nil
	}
	if other != nil {
		return nil, other, errOtherAgent
	}
	if err := spawnDetached("--agent"); err != nil {
		return nil, nil, err
	}
	c, err = ipc.Dial(name, timeout, verify)
	if other != nil {
		return nil, other, errOtherAgent
	}
	return c, nil, err
}

// killProcess cierra un proceso (los agentes de TurboVAULT dejan cerrarse).
func killProcess(pid uint32) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	if err := windows.TerminateProcess(h, 0); err != nil {
		return err
	}
	windows.WaitForSingleObject(h, 3000)
	return nil
}
