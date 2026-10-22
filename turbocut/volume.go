package main

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

type Drive struct {
	Path  string `json:"path"`
	Label string `json:"label"`
	FS    string `json:"fs"`
	Kind  string `json:"kind"` // fijo, usb, red, cd
	Total int64  `json:"total"`
	Free  int64  `json:"free"`
}

func listDrives() []Drive {
	mask, _ := windows.GetLogicalDrives()
	var out []Drive
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		p, _ := windows.UTF16PtrFromString(root)
		kind := ""
		switch windows.GetDriveType(p) {
		case windows.DRIVE_FIXED:
			kind = "fijo"
		case windows.DRIVE_REMOVABLE:
			kind = "usb"
		case windows.DRIVE_REMOTE:
			kind = "red"
		default:
			continue
		}
		d := Drive{Path: root, Kind: kind}
		d.Label, d.FS = volumeLabel(root)
		d.Total, d.Free = diskSpace(root)
		if d.Total == 0 {
			continue
		}
		out = append(out, d)
	}
	return out
}

func volumeLabel(root string) (label, fs string) {
	p, _ := windows.UTF16PtrFromString(root)
	lb := make([]uint16, 261)
	fb := make([]uint16, 261)
	if err := windows.GetVolumeInformation(p, &lb[0], uint32(len(lb)), nil, nil, nil, &fb[0], uint32(len(fb))); err != nil {
		return "", ""
	}
	return windows.UTF16ToString(lb), windows.UTF16ToString(fb)
}

func diskSpace(root string) (total, free int64) {
	p, _ := windows.UTF16PtrFromString(root)
	var avail, tot, tfree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &avail, &tot, &tfree); err != nil {
		return 0, 0
	}
	return int64(tot), int64(tfree)
}

// volumeOf devuelve la raíz de la unidad de una ruta: "C:\".
func volumeOf(path string) string {
	v := filepath.VolumeName(path)
	if v == "" {
		return path
	}
	return v + `\`
}

// cleanRoot normaliza la ruta elegida: "c:" -> "C:\", sin barra final en carpetas.
func cleanRoot(p string) string {
	p = filepath.Clean(strings.TrimSpace(p))
	if len(p) == 2 && p[1] == ':' {
		p += `\`
	}
	if len(p) >= 2 && p[1] == ':' {
		p = strings.ToUpper(p[:1]) + p[1:]
	}
	return p
}

func isAdmin() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// canUseMFT: unidad local NTFS y permisos de administrador.
func canUseMFT(root string) bool {
	if !isAdmin() {
		return false
	}
	vol := volumeOf(root)
	if len(vol) != 3 {
		return false
	}
	p, _ := windows.UTF16PtrFromString(vol)
	t := windows.GetDriveType(p)
	if t != windows.DRIVE_FIXED && t != windows.DRIVE_REMOVABLE {
		return false
	}
	_, fs := volumeLabel(vol)
	return fs == "NTFS"
}
