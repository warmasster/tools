package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Las pruebas no tocan nada del usuario: ajustes y caché en una carpeta temporal y
// una memoria compartida con otro nombre (la cámara de verdad no la ve). La única
// excepción son las pruebas con TURBOCAM_REAL=1, que usan la cámara y los ajustes
// de verdad: no corren con TurboCAM abierto (le quitarían la cámara a la app que
// la esté usando) y al acabar dejan los ajustes como estaban.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "turbocam-test-")
	if err != nil {
		panic(err)
	}
	real := os.Getenv("TURBOCAM_REAL") != ""
	if !real {
		testRoot = dir
		shmName = fmt.Sprintf(`Local\TurboCAM.Test.%d`, os.Getpid())
	} else if turboCAMRunning() {
		fmt.Println("TurboCAM está abierto: ciérralo antes de las pruebas con TURBOCAM_REAL=1")
		os.RemoveAll(dir)
		os.Exit(1)
	}
	saved, savedErr := os.ReadFile(settingsPath())
	code := m.Run()
	if real {
		if savedErr == nil {
			os.WriteFile(settingsPath(), saved, 0o600)
		} else {
			os.Remove(settingsPath())
		}
	}
	os.RemoveAll(dir)
	os.Exit(code)
}

// turboCAMRunning dice si hay un TurboCAM.exe abierto.
func turboCAMRunning() bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	pe := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if strings.EqualFold(windows.UTF16ToString(pe.ExeFile[:]), "TurboCAM.exe") {
			return true
		}
	}
	return false
}
