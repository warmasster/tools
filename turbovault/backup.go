package main

// Copias de seguridad automáticas: cada vez que cambia la bóveda (y cada
// minuto, por si se acaba de conectar el disco) se deja una copia del archivo
// cifrado en las carpetas elegidas. El archivo ya va cifrado: la copia sirve sin
// TurboVAULT abierto y no enseña nada sin la contraseña maestra.
//
// Las carpetas de discos externos se recuerdan por el número de serie del
// volumen, así que se encuentran aunque Windows le dé otra letra al USB.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"turbovault/internal/vault"
)

const backupPrefix = "TurboVAULT-"

func (a *Agent) requestBackup() {
	select {
	case a.backupCh <- struct{}{}:
	default:
	}
}

func (a *Agent) backupLoop() {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-a.quit:
			return
		case <-a.backupCh:
			time.Sleep(2 * time.Second) // juntar varios cambios seguidos
			a.runBackups(false)
		case <-tick.C:
			a.runBackups(false)
		}
	}
}

func fileHash(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:8])
}

// runBackups copia la bóveda a cada destino que no tenga ya la última versión.
func (a *Agent) runBackups(force bool) {
	s := getSettings()
	if len(s.Backups) == 0 || !a.v.Exists() {
		return
	}
	raw, err := a.v.Bytes()
	if err != nil {
		return
	}
	hash := fileHash(raw)
	for _, t := range s.Backups {
		if !force && t.LastError == "" && t.LastHash == hash {
			continue
		}
		dir, ok := resolveTarget(t)
		if !ok {
			if t.LastError != "no conectado" {
				updateTarget(t.ID, func(b *BackupTarget) { b.LastError = "no conectado" })
			}
			continue
		}
		_, err := a.writeBackup(dir, raw, t.Keep)
		updateTarget(t.ID, func(b *BackupTarget) {
			if err != nil {
				b.LastError = err.Error()
				return
			}
			b.LastError = ""
			b.LastCopy = time.Now().UnixMilli()
			b.LastHash = hash
		})
		if err == nil {
			logf("copia en %s", dir)
		}
	}
	a.broadcast()
}

func updateTarget(id string, fn func(*BackupTarget)) {
	updateSettings(func(s *Settings) {
		for i := range s.Backups {
			if s.Backups[i].ID == id {
				fn(&s.Backups[i])
			}
		}
	})
}

// writeBackup deja una copia con la fecha y borra las más viejas (keep>0).
func (a *Agent) writeBackup(dir string, raw []byte, keep int) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := backupPrefix + time.Now().Format("2006-01-02_15-04-05") + ".tvault"
	path := filepath.Join(dir, name)
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	_, werr := f.Write(raw)
	serr := f.Sync()
	cerr := f.Close()
	if werr != nil || serr != nil || cerr != nil {
		os.Remove(tmp)
		return "", errors.Join(werr, serr, cerr)
	}
	// Comprobar que lo escrito se lee bien antes de darlo por bueno.
	if back, err := os.ReadFile(tmp); err != nil || fileHash(back) != fileHash(raw) {
		os.Remove(tmp)
		return "", errors.New("la copia no se leyó igual que el original")
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if keep > 0 {
		pruneBackups(dir, keep)
	}
	return path, nil
}

func pruneBackups(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() && strings.HasPrefix(n, backupPrefix) && strings.HasSuffix(n, ".tvault") {
			names = append(names, n)
		}
	}
	sort.Strings(names) // la fecha va en el nombre: orden alfabético = cronológico
	for len(names) > keep {
		os.Remove(filepath.Join(dir, names[0]))
		names = names[1:]
	}
}

// backupTo hace una copia puntual en una carpeta.
func (a *Agent) backupTo(dir string, keep int) (string, error) {
	dir = filepath.Clean(strings.TrimSpace(dir))
	if !filepath.IsAbs(dir) || strings.HasPrefix(dir, `\\`) || driveKind(dir) == "red" {
		return "", errCode("invalid", "Elige una carpeta de un disco o un USB (las de red no se admiten).")
	}
	raw, err := a.v.Bytes()
	if err != nil {
		return "", err
	}
	return a.writeBackup(dir, raw, keep)
}

func volumeOf(path string) (root string, serial uint32, label string, ok bool) {
	root = filepath.VolumeName(path)
	if root == "" {
		return "", 0, "", false
	}
	root += `\`
	p, _ := windows.UTF16PtrFromString(root)
	lab := make([]uint16, 261)
	var fsFlags, maxLen uint32
	if err := windows.GetVolumeInformation(p, &lab[0], uint32(len(lab)), &serial, &maxLen, &fsFlags, nil, 0); err != nil {
		return root, 0, "", false
	}
	return root, serial, windows.UTF16ToString(lab), true
}

// resolveTarget encuentra la carpeta de un destino aunque el disco haya
// cambiado de letra. ok=false si el disco no está conectado.
func resolveTarget(t BackupTarget) (string, bool) {
	root, serial, _, ok := volumeOf(t.Path)
	if ok && (t.Serial == 0 || serial == t.Serial) {
		return t.Path, true
	}
	if t.Serial == 0 || root == "" {
		return "", false
	}
	rest := strings.TrimPrefix(t.Path, filepath.VolumeName(t.Path))
	for c := 'D'; c <= 'Z'; c++ {
		drive := string(c) + ":"
		if _, s, _, ok := volumeOf(drive + `\`); ok && s == t.Serial {
			return drive + rest, true
		}
	}
	return "", false
}

// driveKind dice qué tipo de disco es (para avisar si la copia se queda en el
// mismo disco del sistema, que no protege si se pierde o se rompe el PC).
func driveKind(path string) string {
	root := filepath.VolumeName(path)
	if root == "" {
		return "red"
	}
	p, _ := windows.UTF16PtrFromString(root + `\`)
	switch windows.GetDriveType(p) {
	case windows.DRIVE_REMOVABLE:
		return "usb"
	case windows.DRIVE_REMOTE:
		return "red"
	case windows.DRIVE_FIXED:
		sys := os.Getenv("SystemDrive")
		if strings.EqualFold(root, sys) {
			return "sistema"
		}
		return "disco"
	}
	return "otro"
}

func (a *Agent) addBackupTarget(path string, keep int) (any, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if !filepath.IsAbs(path) {
		return nil, errCode("invalid", "Elige una carpeta.")
	}
	if strings.HasPrefix(path, `\\`) || driveKind(path) == "red" {
		return nil, errCode("invalid", "Las carpetas de red no se admiten: usa un disco o un USB.")
	}
	if strings.HasPrefix(strings.ToLower(path), strings.ToLower(dataDir())) {
		return nil, errCode("invalid", "Elige una carpeta fuera de la de TurboVAULT (mejor en otro disco).")
	}
	if keep <= 0 {
		keep = 10
	}
	_, serial, label, _ := volumeOf(path)
	t := BackupTarget{ID: vault.NewID()[:8], Path: path, Label: label, Serial: serial, Keep: keep}
	if k := driveKind(path); k != "usb" && k != "disco" {
		t.Serial = 0 // solo los discos que pueden cambiar de letra se buscan por número de serie
	}
	updateSettings(func(s *Settings) { s.Backups = append(s.Backups, t) })
	a.runBackups(true)
	return a.backupStatus(), nil
}

// TargetStatus es un destino de copia para la ventana.
type TargetStatus struct {
	BackupTarget
	Connected bool   `json:"connected"`
	Kind      string `json:"kind"`
	Current   string `json:"current"` // ruta con la letra de ahora
	Copies    int    `json:"copies"`
}

func (a *Agent) backupStatus() []TargetStatus {
	out := []TargetStatus{}
	for _, t := range getSettings().Backups {
		ts := TargetStatus{BackupTarget: t, Kind: driveKind(t.Path)}
		if dir, ok := resolveTarget(t); ok {
			ts.Connected, ts.Current = true, dir
			if es, err := os.ReadDir(dir); err == nil {
				for _, e := range es {
					if strings.HasPrefix(e.Name(), backupPrefix) && strings.HasSuffix(e.Name(), ".tvault") {
						ts.Copies++
					}
				}
			}
		}
		out = append(out, ts)
	}
	return out
}
