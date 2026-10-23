package main

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// turboVersion lee la versión instalada de una herramienta Turbo ("" si no se sabe).
func turboVersion(id string) string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Uninstall\Turbo.`+id, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, _ := k.GetStringValue("DisplayVersion")
	return v
}

// versionLess compara versiones tipo 2.0.0. Una versión desconocida no se considera vieja.
func versionLess(a, b string) bool {
	if a == "" {
		return false
	}
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		var x, y int
		if i < len(pa) {
			fmt.Sscan(pa[i], &x)
		}
		if i < len(pb) {
			fmt.Sscan(pb[i], &y)
		}
		if x != y {
			return x < y
		}
	}
	return false
}
