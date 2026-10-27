package main

import (
	"fmt"
	"os"
	"testing"
)

// Las pruebas no tocan nada del usuario: ajustes y caché en una carpeta temporal y
// una memoria compartida con otro nombre (la cámara de verdad no la ve). La única
// excepción es TestRealVirtualCamera, que solo corre con TURBOCAM_REAL=1.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "turbocam-test-")
	if err != nil {
		panic(err)
	}
	if os.Getenv("TURBOCAM_REAL") == "" {
		testRoot = dir
		shmName = fmt.Sprintf(`Local\TurboCAM.Test.%d`, os.Getpid())
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
