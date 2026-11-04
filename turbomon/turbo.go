package main

// Modo juego Turbo: mientras dura una partida, el plan de energía de más
// rendimiento, algo más de prioridad para el juego y, si se acaba la memoria
// libre, soltar la memoria «en espera». Al acabar se deja todo como estaba
// (también si TurboMON se cerró a mitad: lo arregla al volver a arrancar).
//
// Lo que NO hace, a propósito: vaciar la memoria de los programas («limpiar la
// RAM» de otras herramientas). Windows la vuelve a cargar enseguida y lo único
// que se consigue son tirones.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"turbomon/internal/checks"
	"turbomon/internal/sensors"
)

var procNtSetSysInfo = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtSetSystemInformation")

type turboState struct {
	prevPlan *windows.GUID
}

func planBackupPath() string { return filepath.Join(dataDir(), "plan-anterior.txt") }

func (a *Agent) turboStart(pid uint32) *turboState {
	s := getSettings()
	ts := &turboState{}
	if s.TurboPlan {
		if cur, err := checks.ActivePlan(); err == nil {
			if best, ok := checks.BestPlan(); ok && best.GUID != cur.GUID {
				if checks.SetPlan(best.GUID) == nil {
					g := cur.GUID
					ts.prevPlan = &g
					os.WriteFile(planBackupPath(), []byte(g.String()), 0o644)
					logf("modo Turbo: plan «%s» (antes «%s»)", best.Name, cur.Name)
				}
			}
		}
	}
	if s.TurboPrio {
		if h, err := windows.OpenProcess(windows.PROCESS_SET_INFORMATION, false, pid); err == nil {
			windows.SetPriorityClass(h, windows.ABOVE_NORMAL_PRIORITY_CLASS)
			windows.CloseHandle(h)
		}
	}
	return ts
}

func (a *Agent) turboEnd(ts *turboState) {
	if ts.prevPlan != nil {
		checks.SetPlan(*ts.prevPlan)
		os.Remove(planBackupPath())
	}
}

// restorePlan deja el plan de antes si TurboMON se cerró en mitad de una partida.
func restorePlan() {
	b, err := os.ReadFile(planBackupPath())
	if err != nil {
		return
	}
	if g, err := windows.GUIDFromString(strings.TrimSpace(string(b))); err == nil {
		checks.SetPlan(g)
	}
	os.Remove(planBackupPath())
}

// turboTick: durante la partida, si queda muy poca memoria libre y mucha en
// espera, se suelta (como mucho cada 5 minutos).
func (a *Agent) turboTick(c *liveSession, snap *sensors.Snapshot) {
	if c.turbo == nil || snap == nil || !snap.Full || !a.elevated || !getSettings().TurboStandby {
		return
	}
	if time.Since(c.lastPurg) < 5*time.Minute {
		return
	}
	if snap.Mem.Free < 1<<30 && snap.Mem.Standby > 2<<30 && snap.Mem.Load >= 75 {
		c.lastPurg = time.Now()
		go func() {
			if _, err := a.purgeStandby(); err == nil {
				c.purges++
			}
		}()
	}
}

// purgeStandby suelta la memoria en espera (la caché de archivos que Windows
// guarda por si acaso). Necesita ser administrador.
func (a *Agent) purgeStandby() (uint64, error) {
	if !a.elevated {
		return 0, errors.New("hace falta permiso de administrador")
	}
	if err := enablePrivilege("SeProfileSingleProcessPrivilege"); err != nil {
		return 0, err
	}
	before := sensors.StandbyBytes()
	cmd := uint32(4)                                                       // MemoryPurgeStandbyList
	r, _, _ := procNtSetSysInfo.Call(80, uintptr(unsafe.Pointer(&cmd)), 4) // SystemMemoryListInformation
	if r != 0 {
		return 0, errors.New("Windows no ha dejado liberarla")
	}
	after := sensors.StandbyBytes()
	if before > after {
		return before - after, nil
	}
	return 0, nil
}

func enablePrivilege(name string) error {
	var tok windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &tok); err != nil {
		return err
	}
	defer tok.Close()
	var luid windows.LUID
	p, _ := windows.UTF16PtrFromString(name)
	if err := windows.LookupPrivilegeValue(nil, p, &luid); err != nil {
		return err
	}
	tp := windows.Tokenprivileges{PrivilegeCount: 1}
	tp.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
	return windows.AdjustTokenPrivileges(tok, false, &tp, 0, nil, nil)
}
