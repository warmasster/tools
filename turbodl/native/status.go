package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// printStatus muestra qué hay instalado (útil para comprobar una instalación).
func printStatus() {
	dir := AppDir()
	fmt.Println("TurboDL", version)
	fmt.Println("Carpeta:", dir)
	_, errExe := os.Stat(filepath.Join(dir, "turbodl-native.exe"))
	_, errMan := os.Stat(filepath.Join(dir, hostName+".json"))
	fmt.Println("Motor nativo instalado:", errExe == nil && errMan == nil)
	_, errExt := os.Stat(filepath.Join(ExtensionDir(), "manifest.json"))
	fmt.Println("Extensión copiada en", ExtensionDir()+":", errExt == nil)
	fmt.Println("Este ejecutable lleva la extensión dentro:", hasEmbeddedExtension())
	paths := chromeExtensionPaths()
	if len(paths) == 0 {
		fmt.Println("Cargada en Chrome: no")
	}
	for _, p := range paths {
		fmt.Println("Cargada en Chrome desde:", p)
	}
}
