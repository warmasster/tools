package main

// Ventana de inicio (TurboMENU.exe sin argumentos) y utilidades comunes.

import (
	"errors"
	"os"
	"time"
)

type HomeInfo struct {
	Installed bool   `json:"installed"`
	Modern    bool   `json:"modern"` // también en el menú nuevo de Windows 11
	Prewarm   bool   `json:"prewarm"`
	IdleMins  int    `json:"idleMins"` // minutos que se queda preparado tras cerrar
	Version   string `json:"version"`
	Exe       string `json:"exe"`
	Deps      Deps   `json:"deps"`
}

func (a *App) HomeInfo() HomeInfo {
	exe, _ := os.Executable()
	s := getSettings()
	return HomeInfo{Installed: menuInstalled(exe) || modernMenuInstalled(), Modern: modernMenuInstalled(), Prewarm: s.Prewarm, IdleMins: s.IdleMins,
		Version: version, Exe: exe, Deps: detectDeps()}
}

func (a *App) InstallMenu() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if getSettings().Modern {
		registerModernMenu(exe)
		if getSettings().Modern {
			return nil
		}
	}
	return registerMenu(exe)
}

func (a *App) UninstallMenu() error {
	unregisterMenu()
	unregisterModernMenu()
	return nil
}

func (a *App) SetPrewarm(on bool) error {
	setPrewarm(on)
	return updateSettings(func(s *Settings) { s.Prewarm = on })
}

// SetIdleMins elige cuánto se queda preparado tras cerrar una ventana (para reabrir al instante).
func (a *App) SetIdleMins(m int) error {
	switch m {
	case 5, 15, 30, 60:
	default:
		return errors.New("Valor no válido")
	}
	return updateSettings(func(s *Settings) { s.IdleMins = m })
}

// SetJob cambia el trabajo de la ventana (p. ej. «Enviar» con el archivo ya reducido).
func (a *App) SetJob(action string, paths []string) (JobView, error) {
	if !validAction(action) || action == "quit" {
		return JobView{}, errors.New("Acción desconocida")
	}
	a.mu.Lock()
	x, y := int32(0), int32(0)
	if a.job != nil {
		x, y = a.job.X, a.job.Y
	}
	a.job = &Job{Action: action, Paths: paths, X: x, Y: y, Time: time.Now().UnixMilli()}
	a.gen++
	a.mu.Unlock()
	a.stopQR()
	return a.GetJob(), nil
}
