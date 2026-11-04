package main

// Programas que arrancan con Windows: saber si uno lo hace y desactivarlo como
// el Administrador de tareas (no se borra nada: se marca como desactivado en
// «StartupApproved» y se puede volver a activar desde allí).

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

type startupEntry struct {
	root     registry.Key
	approved string
	name     string
	folder   bool // acceso directo en la carpeta Inicio
}

func startupEntriesFor(exe string) []startupEntry {
	exe = strings.ToLower(exe)
	var out []startupEntry
	for _, s := range []struct {
		root     registry.Key
		run      string
		approved string
	}{
		{registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`},
		{registry.LOCAL_MACHINE, `Software\Microsoft\Windows\CurrentVersion\Run`, `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`},
		{registry.LOCAL_MACHINE, `Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Run`, `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run32`},
	} {
		k, err := registry.OpenKey(s.root, s.run, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		names, _ := k.ReadValueNames(-1)
		for _, n := range names {
			v, _, _ := k.GetStringValue(n)
			if strings.Contains(strings.ToLower(v), exe) {
				out = append(out, startupEntry{root: s.root, approved: s.approved, name: n})
			}
		}
		k.Close()
	}
	dir := filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs\Startup`)
	ents, _ := os.ReadDir(dir)
	base := strings.TrimSuffix(exe, ".exe")
	for _, e := range ents {
		if strings.Contains(strings.ToLower(e.Name()), base) {
			out = append(out, startupEntry{root: registry.CURRENT_USER, approved: `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\StartupFolder`, name: e.Name(), folder: true})
		}
	}
	return out
}

// startupEnabled dice si el programa arranca solo con Windows.
func startupEnabled(exe string) bool {
	for _, e := range startupEntriesFor(exe) {
		k, err := registry.OpenKey(e.root, e.approved, registry.QUERY_VALUE)
		if err != nil {
			return true
		}
		b, _, err := k.GetBinaryValue(e.name)
		k.Close()
		if err != nil || len(b) == 0 || b[0]&1 == 0 {
			return true
		}
	}
	return false
}

// disableStartup lo desactiva (los de todos los usuarios necesitan permiso de administrador).
func disableStartup(exe string) error {
	entries := startupEntriesFor(exe)
	if len(entries) == 0 {
		return errors.New("ese programa no arranca con Windows desde las listas normales (puede ser un servicio o una tarea programada)")
	}
	var lastErr error
	done := 0
	for _, e := range entries {
		k, _, err := registry.CreateKey(e.root, e.approved, registry.SET_VALUE)
		if err != nil {
			lastErr = errors.New("hace falta permiso de administrador para este (arranca para todos los usuarios)")
			continue
		}
		// 0x03 + 11 bytes: desactivado (como lo deja el Administrador de tareas).
		err = k.SetBinaryValue(e.name, []byte{3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
		k.Close()
		if err != nil {
			lastErr = err
			continue
		}
		done++
	}
	if done == 0 && lastErr != nil {
		return lastErr
	}
	return nil
}
