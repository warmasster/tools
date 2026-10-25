package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// reservedWinName: nombres que Windows reserva para dispositivos (CON, NUL, COM1…),
// también con extensión («aux.min.js»). Crear un archivo así falla o escribe en el
// dispositivo.
func reservedWinName(name string) bool {
	base := strings.ToLower(strings.TrimRight(strings.SplitN(name, ".", 2)[0], " "))
	switch base {
	case "con", "prn", "aux", "nul", "conin$", "conout$":
		return true
	}
	if len(base) >= 4 && (strings.HasPrefix(base, "com") || strings.HasPrefix(base, "lpt")) {
		switch base[3:] {
		case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "¹", "²", "³":
			return true
		}
	}
	return false
}

// invisibleMark: marcas de dirección y de anchura cero. Sirven para disfrazar
// nombres («foto\u202Egpj.exe» se ve como «fotoexe.jpg»), así que se quitan.
func invisibleMark(r rune) bool {
	return r == 0x200B || r == 0x200E || r == 0x200F || r >= 0x202A && r <= 0x202E || r >= 0x2066 && r <= 0x2069 || r == 0xFEFF
}

// markFromInternet pone la marca «descargado de Internet» que pone Windows a las
// descargas: lo que llega del móvil de otra persona pasa por SmartScreen al abrirlo.
func markFromInternet(path string) {
	os.WriteFile(path+":Zone.Identifier", []byte("[ZoneTransfer]\r\nZoneId=3\r\n"), 0o644)
}

// createExclusive crea un archivo nuevo con ese nombre (o «nombre (2)»…) sin
// pisar nunca uno que exista, aunque otro proceso elija el mismo a la vez.
func createExclusive(dir, name string) (string, *os.File, error) {
	for i := 0; i < 50; i++ {
		p := uniquePath(dir, name)
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			return p, f, nil
		}
		if !os.IsExist(err) {
			return "", nil, err
		}
	}
	return "", nil, errors.New("no hay un nombre libre en " + filepath.Base(dir))
}
