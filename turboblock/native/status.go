package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// printStatus muestra qué hay instalado (útil para comprobar una instalación).
func printStatus() {
	dir := AppDir()
	fmt.Println("TurboBLOCK", version)
	fmt.Println("Carpeta:", dir)
	_, errExe := os.Stat(filepath.Join(dir, "turboblock-native.exe"))
	_, errMan := os.Stat(filepath.Join(dir, hostName+".json"))
	fmt.Println("Actualizador instalado:", errExe == nil && errMan == nil)
	fmt.Println("Este ejecutable lleva la extensión dentro:", hasEmbeddedExtension())
	paths := chromeExtensionPaths()
	if len(paths) == 0 {
		fmt.Println("Cargada en el navegador: no")
	}
	for _, p := range paths {
		fmt.Println("Cargada en el navegador desde:", p)
	}
	for _, d := range extensionDirs() {
		if m := readMeta(d); m != nil {
			fmt.Printf("Reglas en %s: huella %s, compiladas %s\n", d, m.Build, m.Built.Local().Format("2006-01-02 15:04"))
			printMeta(m)
		}
	}
}
