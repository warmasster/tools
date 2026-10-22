package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"
)

// Todo va en %LOCALAPPDATA%\Programs\Turbo: cada herramienta en su carpeta y,
// al lado, una copia de TurboSETUP para actualizar y desinstalar.
func rootDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base, _ = os.UserCacheDir()
	}
	return filepath.Join(base, "Programs", "Turbo")
}

func toolDir(name string) string { return filepath.Join(rootDir(), name) }
func setupPath() string          { return filepath.Join(rootDir(), "TurboSETUP.exe") }

// La caché del navegador interno va en %TEMP%: así la carpeta de instalación
// solo tiene programas y se puede borrar entera sin esperar a WebView2.
func webviewDir() string { return filepath.Join(os.TempDir(), "TurboSETUP-webview") }

const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\`

func regKeyName(id string) string { return uninstallKey + "Turbo." + id }

type Installed struct {
	Name    string
	Version string
	Kind    string
	Exe     string
}

// listInstalled lee las herramientas instaladas de las entradas de desinstalación.
func listInstalled() map[string]Installed {
	out := map[string]Installed{}
	k, err := registry.OpenKey(registry.CURRENT_USER, uninstallKey, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return out
	}
	names, _ := k.ReadSubKeyNames(-1)
	k.Close()
	for _, n := range names {
		if !strings.HasPrefix(n, "Turbo.") {
			continue
		}
		sk, err := registry.OpenKey(registry.CURRENT_USER, uninstallKey+n, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		var in Installed
		in.Name, _, _ = sk.GetStringValue("DisplayName")
		in.Version, _, _ = sk.GetStringValue("DisplayVersion")
		in.Kind, _, _ = sk.GetStringValue("TurboKind")
		in.Exe, _, _ = sk.GetStringValue("DisplayIcon")
		sk.Close()
		if in.Exe != "" {
			if _, err := os.Stat(in.Exe); err != nil {
				continue // la borraron a mano
			}
		}
		out[strings.TrimPrefix(n, "Turbo.")] = in
	}
	return out
}

// ---------- descargar ----------

type progressFunc func(done, total int64)

// download baja un archivo de la Release a dst, comprobando tamaño y SHA-256.
func download(ctx context.Context, a Asset, dst string, onProgress progressFunc) error {
	req, err := httpNewRequest(ctx, downloadBase()+"/"+a.Asset)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return context.Canceled
		}
		return fmt.Errorf("no se pudo conectar con GitHub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("GitHub respondió %s al pedir %s", resp.Status, a.Asset)
	}
	total := a.Size
	if total <= 0 {
		total = resp.ContentLength
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	h := sha256.New()
	buf := make([]byte, 256<<10)
	var done int64
	last := time.Time{}
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				f.Close()
				os.Remove(dst)
				return err
			}
			h.Write(buf[:n])
			done += int64(n)
			if time.Since(last) > 80*time.Millisecond {
				onProgress(done, total)
				last = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			os.Remove(dst)
			if ctx.Err() != nil {
				return context.Canceled
			}
			return fmt.Errorf("la descarga se cortó: %w", rerr)
		}
	}
	onProgress(done, total)
	if err := f.Close(); err != nil {
		os.Remove(dst)
		return err
	}
	if a.Size > 0 && done != a.Size {
		os.Remove(dst)
		return fmt.Errorf("la descarga llegó incompleta (%d de %d bytes)", done, a.Size)
	}
	if a.SHA256 != "" && !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), a.SHA256) {
		os.Remove(dst)
		return errors.New("el archivo descargado no coincide con el de GitHub (SHA-256 distinto)")
	}
	return nil
}

// replaceFile pone src en dst aunque dst esté abierto: Windows deja renombrar un
// .exe en uso, así que el viejo se aparta a .old y se borra cuando se pueda.
// Devuelve true si el viejo seguía abierto.
func replaceFile(src, dst string) (inUse bool, err error) {
	old := dst + ".old"
	os.Remove(old)
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, old); err != nil {
			return false, err
		}
	}
	if err := os.Rename(src, dst); err != nil {
		os.Rename(old, dst)
		return false, err
	}
	if _, err := os.Stat(old); err == nil {
		inUse = os.Remove(old) != nil
	}
	return inUse, nil
}

type InstallResult struct {
	Message string `json:"message"`
	Restart bool   `json:"restart"`
}

func installTool(ctx context.Context, t *Tool, desktop bool, onProgress progressFunc) (*InstallResult, error) {
	dir := toolDir(t.Name)
	exe := filepath.Join(dir, t.Asset.Asset)
	part := exe + ".part"
	if err := download(ctx, t.Asset, part, onProgress); err != nil {
		return nil, err
	}
	inUse, err := replaceFile(part, exe)
	if err != nil {
		os.Remove(part)
		return nil, fmt.Errorf("no se pudo guardar %s: %w", exe, err)
	}

	res := &InstallResult{Message: t.Name + " " + t.Version + " instalado"}
	switch t.Kind {
	case "setup":
		// TurboDL instala su motor y deja la extensión lista; enseña sus propios avisos.
		out, err := runHidden(exe, "setup")
		if err != nil {
			return nil, fmt.Errorf("el instalador de %s falló: %v %s", t.Name, err, out)
		}
	default:
		if inUse {
			res.Restart = true
			res.Message = t.Name + " actualizado. Ciérralo y vuelve a abrirlo para usar la versión nueva."
		}
		if err := createShortcut(startMenuLink(t.Name), exe, t.Summary); err != nil {
			return nil, fmt.Errorf("no se pudo crear el acceso directo: %w", err)
		}
		if desktop {
			createShortcut(desktopLink(t.Name), exe, t.Summary)
		}
	}

	if err := writeUninstallEntry(t, exe); err != nil {
		return nil, fmt.Errorf("no se pudo registrar la instalación: %w", err)
	}
	ensureSetupCopy()
	return res, nil
}

func writeUninstallEntry(t *Tool, exe string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, regKeyName(t.ID), registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	var size uint32
	if st, err := os.Stat(exe); err == nil {
		size = uint32(st.Size() / 1024)
	}
	k.SetStringValue("DisplayName", t.Name)
	k.SetStringValue("DisplayVersion", t.Version)
	k.SetStringValue("DisplayIcon", exe)
	k.SetStringValue("Publisher", "Fernando")
	k.SetStringValue("InstallLocation", filepath.Dir(exe))
	k.SetStringValue("URLInfoAbout", "https://github.com/"+repo)
	k.SetStringValue("InstallDate", time.Now().Format("20060102"))
	k.SetStringValue("UninstallString", fmt.Sprintf(`"%s" --uninstall %s`, setupPath(), t.ID))
	k.SetStringValue("TurboKind", t.Kind)
	k.SetDWordValue("EstimatedSize", size)
	k.SetDWordValue("NoModify", 1)
	k.SetDWordValue("NoRepair", 1)
	return nil
}

// ensureSetupCopy deja una copia de TurboSETUP en la carpeta de instalación, con su
// acceso directo, para poder actualizar y desinstalar sin volver a descargarlo.
func ensureSetupCopy() {
	self, err := os.Executable()
	if err != nil {
		return
	}
	dst := setupPath()
	if !strings.EqualFold(filepath.Clean(self), filepath.Clean(dst)) {
		tmp := dst + ".new"
		if err := copyFile(self, tmp); err == nil {
			if _, err := replaceFile(tmp, dst); err != nil {
				os.Remove(tmp)
			}
		}
	}
	createShortcut(startMenuLink("TurboSETUP"), dst, "Instala y actualiza las herramientas Turbo")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}

// ---------- desinstalar ----------

func uninstallTool(id string) (string, error) {
	in, ok := listInstalled()[id]
	if !ok {
		return "", errors.New("no está instalada")
	}
	name := in.Name
	if name == "" {
		name = id
	}
	dir := ""
	if in.Exe != "" {
		dir = filepath.Dir(in.Exe)
	}
	msg := name + " desinstalado"

	if in.Kind == "setup" && in.Exe != "" {
		out, err := runHidden(in.Exe, "uninstall")
		if err != nil {
			return "", fmt.Errorf("no se pudo desinstalar %s: %v %s", name, err, out)
		}
		if s := strings.TrimSpace(out); s != "" {
			msg = s
		}
	} else if in.Exe != "" {
		// Si está abierto no se puede borrar: mejor avisar antes de tocar nada.
		probe := in.Exe + ".del"
		if err := os.Rename(in.Exe, probe); err != nil {
			if !os.IsNotExist(err) {
				return "", fmt.Errorf("%s está abierto. Ciérralo y vuelve a intentarlo", name)
			}
		} else if err := os.Remove(probe); err != nil {
			os.Rename(probe, in.Exe)
			return "", fmt.Errorf("%s está abierto. Ciérralo y vuelve a intentarlo", name)
		}
	}

	os.Remove(startMenuLink(name))
	os.Remove(desktopLink(name))
	registry.DeleteKey(registry.CURRENT_USER, regKeyName(id))
	if dir != "" && strings.HasPrefix(strings.ToLower(dir), strings.ToLower(rootDir())) {
		os.RemoveAll(dir)
	}
	if len(listInstalled()) == 0 {
		os.Remove(startMenuLink("TurboSETUP"))
	}
	return msg, nil
}

// removeEverythingLater borra la carpeta Turbo entera cuando este proceso termine
// (se usa al quitar la última herramienta: no deja nada atrás).
func removeEverythingLater() {
	root := rootDir()
	if _, err := os.Stat(root); err != nil {
		return
	}
	cmd := exec.Command("cmd", "/c", fmt.Sprintf(`ping -n 4 127.0.0.1 >nul & rmdir /s /q "%s"`, root))
	hideWindow(cmd)
	cmd.Start()
}

// cleanupOldFiles borra los .old que dejaron actualizaciones de programas que estaban abiertos.
func cleanupOldFiles() {
	matches, _ := filepath.Glob(filepath.Join(rootDir(), "*.old"))
	more, _ := filepath.Glob(filepath.Join(rootDir(), "*", "*.old"))
	for _, m := range append(matches, more...) {
		os.Remove(m)
	}
}

func cliUninstall(id string) int {
	in, ok := listInstalled()[id]
	if !ok {
		registry.DeleteKey(registry.CURRENT_USER, regKeyName(id))
		messageBox("TurboSETUP", "Esa herramienta ya no está instalada.", mbInfo)
		return 0
	}
	name := in.Name
	if name == "" {
		name = id
	}
	if !confirmBox("Desinstalar "+name, "¿Quieres desinstalar "+name+"?\n\nSe borra el programa y sus accesos directos. Tus ajustes y archivos no se tocan.") {
		return 1
	}
	msg, err := uninstallTool(id)
	if err != nil {
		messageBox("Desinstalar "+name, err.Error(), mbError)
		return 1
	}
	if len(listInstalled()) == 0 {
		removeEverythingLater()
	}
	messageBox("Desinstalar "+name, msg+".", mbInfo)
	return 0
}

func sortedTools(tv []ToolView) {
	sort.SliceStable(tv, func(i, j int) bool { return tv[i].Available && !tv[j].Available })
}
