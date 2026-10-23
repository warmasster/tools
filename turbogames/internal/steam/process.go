package steam

import (
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type proc struct {
	pid  uint32
	name string
}

// processes lista los procesos abiertos (nombre del .exe y PID).
func processes() []proc {
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(h)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	var out []proc
	for err = windows.Process32First(h, &pe); err == nil; err = windows.Process32Next(h, &pe) {
		out = append(out, proc{pid: pe.ProcessID, name: windows.UTF16ToString(pe.ExeFile[:])})
	}
	return out
}

// Procesos de Steam que hay que cerrar para cambiar de cuenta. steamservice.exe
// no: es un servicio del sistema y no guarda nada de la cuenta.
var steamProcs = []string{"steam.exe", "steamwebhelper.exe", "gameoverlayui.exe", "gameoverlayui64.exe", "steamerrorreporter.exe", "steamerrorreporter64.exe"}

func isSteamProc(name string) bool {
	for _, n := range steamProcs {
		if strings.EqualFold(n, name) {
			return true
		}
	}
	return false
}

func steamPIDs(onlyMain bool) []uint32 {
	var out []uint32
	for _, p := range processes() {
		if onlyMain && !strings.EqualFold(p.name, "steam.exe") {
			continue
		}
		if isSteamProc(p.name) {
			out = append(out, p.pid)
		}
	}
	return out
}

func killPID(pid uint32) {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	if windows.TerminateProcess(h, 1) == nil {
		windows.WaitForSingleObject(h, 3000)
	}
}

// waitGone espera a que no quede ningún proceso de Steam (o solo el principal,
// si onlyMain) o a que se acabe el tiempo.
func waitGone(onlyMain bool, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for {
		if len(steamPIDs(onlyMain)) == 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(250 * time.Millisecond)
	}
}
