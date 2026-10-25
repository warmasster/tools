package main

// Instalación (lo que lanza TurboSETUP con "setup" y "uninstall"):
//   - registra el puente de la extensión para Chrome, Edge, Brave… (HKCU);
//   - deja la extensión en %LOCALAPPDATA%\TurboVAULT\extension;
//   - arranque con Windows (el agente de la bandeja) y acceso directo;
//   - cierra versiones anteriores que sigan abiertas.
// La bóveda no se toca nunca, tampoco al desinstalar.

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"turbovault/internal/ipc"
	"turbovault/internal/winsec"
)

//go:embed all:extension
var extensionFS embed.FS

// extVersion es la versión de la extensión que va dentro del programa.
var extVersion = func() string {
	b, _ := extensionFS.ReadFile("extension/manifest.json")
	var m struct {
		Version string `json:"version"`
	}
	json.Unmarshal(b, &m)
	return m.Version
}()

var browserKeys = []struct{ name, key string }{
	{"Chrome", `Software\Google\Chrome\NativeMessagingHosts\`},
	{"Edge", `Software\Microsoft\Edge\NativeMessagingHosts\`},
	{"Brave", `Software\BraveSoftware\Brave-Browser\NativeMessagingHosts\`},
	{"Vivaldi", `Software\Vivaldi\NativeMessagingHosts\`},
	{"Chromium", `Software\Chromium\NativeMessagingHosts\`},
}

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

func secureDataDir() error { return winsec.SecureDir(dataDir()) }

func extensionDir() string { return filepath.Join(localDir(), "extension") }

func hostManifestPath() string { return filepath.Join(localDir(), hostName+".json") }

// registerHost escribe el manifiesto del puente y lo registra para el usuario.
func registerHost() error {
	if err := os.MkdirAll(localDir(), 0o700); err != nil {
		return err
	}
	manifest := map[string]any{
		"name":            hostName,
		"description":     "TurboVAULT: contraseñas guardadas en tu PC",
		"path":            selfExe(),
		"type":            "stdio",
		"allowed_origins": []string{"chrome-extension://" + extensionID + "/"},
	}
	raw, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(hostManifestPath(), raw, 0o644); err != nil {
		return err
	}
	ok := 0
	for _, b := range browserKeys {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, b.key+hostName, registry.SET_VALUE)
		if err != nil {
			continue
		}
		if k.SetStringValue("", hostManifestPath()) == nil {
			ok++
		}
		k.Close()
	}
	if ok == 0 {
		return fmt.Errorf("no se pudo escribir en el registro")
	}
	return nil
}

func hostRegistered() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, browserKeys[0].key+hostName, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	p, _, err := k.GetStringValue("")
	if err != nil {
		return false
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	var m struct{ Path string }
	json.Unmarshal(raw, &m)
	return winsec.SamePath(m.Path, selfExe())
}

// installExtension copia la extensión que va dentro del programa.
func installExtension() (string, error) {
	dir := extensionDir()
	tmp := dir + ".new"
	os.RemoveAll(tmp)
	err := fs.WalkDir(extensionFS, "extension", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, "extension")
		dst := filepath.Join(tmp, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := extensionFS.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		os.RemoveAll(tmp)
		return "", err
	}
	old := dir + ".old"
	os.RemoveAll(old)
	if _, err := os.Stat(dir); err == nil {
		if err := os.Rename(dir, old); err != nil {
			// Algo la tiene abierta: copiar encima.
			os.RemoveAll(tmp)
			return dir, copyTree(dir)
		}
	}
	if err := os.Rename(tmp, dir); err != nil {
		os.Rename(old, dir)
		return "", err
	}
	os.RemoveAll(old)
	return dir, nil
}

func copyTree(dir string) error {
	return fs.WalkDir(extensionFS, "extension", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(p, "extension")))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, _ := extensionFS.ReadFile(p)
		return os.WriteFile(dst, b, 0o644)
	})
}

func setAutostart(on bool) {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	if !on {
		k.DeleteValue("TurboVAULT")
		return
	}
	k.SetStringValue("TurboVAULT", fmt.Sprintf(`"%s" --agent`, selfExe()))
}

func startMenuLink() string {
	dir, err := windows.KnownFolderPath(windows.FOLDERID_Programs, 0)
	if err != nil {
		dir = filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs`)
	}
	return filepath.Join(dir, "TurboVAULT.lnk")
}

// createShortcut crea un .lnk con WScript.Shell (COM), que viene con Windows.
func createShortcut(lnk, target, description string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		if oe, ok := err.(*ole.OleError); !ok || oe.Code() != 1 {
			return err
		}
	}
	defer ole.CoUninitialize()
	unk, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return err
	}
	defer unk.Release()
	shell, err := unk.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return err
	}
	defer shell.Release()
	v, err := oleutil.CallMethod(shell, "CreateShortcut", lnk)
	if err != nil {
		return err
	}
	sc := v.ToIDispatch()
	defer sc.Release()
	for prop, val := range map[string]string{
		"TargetPath":       target,
		"WorkingDirectory": filepath.Dir(target),
		"Description":      description,
		"IconLocation":     target + ",0",
	} {
		if _, err := oleutil.PutProperty(sc, prop, val); err != nil {
			return err
		}
	}
	_, err = oleutil.CallMethod(sc, "Save")
	return err
}

// stopOthers cierra los demás procesos de TurboVAULT (agente y ventanas de una
// versión anterior), para que arranque la nueva.
func stopOthers() {
	// Primero con educación: pedir al agente que se cierre.
	if c, err := ipc.Dial(pipeName(), 300*time.Millisecond, nil); err == nil {
		cl := ipc.NewClient(c, nil)
		cl.Call("quit", nil, nil, 2*time.Second)
		cl.Close()
		time.Sleep(300 * time.Millisecond)
	}
	self := windows.GetCurrentProcessId()
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		name := strings.ToLower(windows.UTF16ToString(e.ExeFile[:]))
		if e.ProcessID == self || !strings.HasPrefix(name, "turbovault") {
			continue
		}
		path, err := winsec.ProcessPath(e.ProcessID)
		if err != nil {
			continue
		}
		args, _ := winsec.CommandLine(e.ProcessID)
		if len(args) > 1 && strings.HasPrefix(strings.ToLower(args[1]), "chrome-extension://") {
			continue // los puentes los cierra Chrome
		}
		if strings.HasPrefix(strings.ToLower(filepath.Base(path)), "turbovault") {
			killProcess(e.ProcessID)
		}
	}
}

func messageBox(title, text string, isErr bool) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	flags := uint32(windows.MB_ICONINFORMATION | windows.MB_SETFOREGROUND)
	if isErr {
		flags = windows.MB_ICONERROR | windows.MB_SETFOREGROUND
	}
	windows.MessageBox(0, m, t, flags)
}

func cliSetup() int {
	stopOthers()
	if err := secureDataDir(); err != nil {
		logf("setup: carpeta: %v", err)
	}
	os.MkdirAll(localDir(), 0o700)
	if err := registerHost(); err != nil {
		messageBox("TurboVAULT", "No se pudo registrar la extensión:\n"+err.Error(), true)
		return 1
	}
	extDir, err := installExtension()
	if err != nil {
		messageBox("TurboVAULT", "No se pudo copiar la extensión:\n"+err.Error(), true)
		return 1
	}
	setAutostart(getSettings().Autostart)
	if err := createShortcut(startMenuLink(), selfExe(), "Tus contraseñas, cifradas en tu PC"); err != nil {
		logf("setup: acceso directo: %v", err)
	}
	spawnDetached("--agent")

	loaded := chromeExtensionPaths()
	already := false
	for _, p := range loaded {
		if winsec.SamePath(p, extDir) {
			already = true
		}
	}
	if !already {
		setClipboard(extDir, false)
		opened := openInChrome("chrome://extensions/")
		where := "En la pestaña de extensiones que se acaba de abrir:"
		if !opened {
			where = "Abre Chrome y escribe chrome://extensions en la barra de direcciones. Luego:"
		}
		messageBox("TurboVAULT · último paso", where+`

  1. Activa «Modo de desarrollador» (arriba a la derecha).
  2. Pulsa «Cargar descomprimida».
  3. Pega la ruta en la barra de la ventana (Ctrl+V; ya está copiada) y pulsa «Seleccionar carpeta».

La extensión está en:
`+extDir+`

Chrome no deja que un programa instale extensiones por su cuenta: este paso se hace una sola vez. Después, al abrir TurboVAULT por primera vez, crearás tu contraseña maestra.`, false)
	} else {
		messageBox("TurboVAULT", "TurboVAULT actualizado.\n\nLa extensión de Chrome se pone al día sola. Si tenías la 1.0.0, esta vez pulsa el botón de recargar (↻) de TurboVAULT en chrome://extensions o reinicia Chrome.", false)
	}
	spawnDetached()
	fmt.Println("TurboVAULT instalado. Está en la bandeja, junto a la flechita de la barra de tareas.")
	return 0
}

func cliUninstall() int {
	stopOthers()
	for _, b := range browserKeys {
		registry.DeleteKey(registry.CURRENT_USER, b.key+hostName)
	}
	setAutostart(false)
	os.Remove(startMenuLink())
	os.Remove(hostManifestPath())
	os.RemoveAll(extensionDir())
	os.RemoveAll(filepath.Join(localDir(), "webview"))
	fmt.Println("TurboVAULT desinstalado. Tu bóveda sigue en " + dataDir() + " (cifrada). Quita también la extensión en chrome://extensions.")
	return 0
}

// ---------- Chrome ----------

func browserDataDirs() []string {
	// Pruebas: los perfiles de los navegadores del PC no se miran.
	if d, ok := testEnvSet("TURBOVAULT_BROWSERS"); ok {
		return filepath.SplitList(d)
	}
	la := os.Getenv("LOCALAPPDATA")
	return []string{
		filepath.Join(la, "Google", "Chrome", "User Data"),
		filepath.Join(la, "Microsoft", "Edge", "User Data"),
		filepath.Join(la, "BraveSoftware", "Brave-Browser", "User Data"),
		filepath.Join(la, "Chromium", "User Data"),
	}
}

// chromeExtensionPaths devuelve desde qué carpetas está cargada la extensión en
// los perfiles de los navegadores (solo lee su configuración).
func chromeExtensionPaths() []string { return chromeExtensionPathsIn(browserDataDirs()) }

func chromeExtensionPathsIn(bases []string) []string {
	var out []string
	for _, base := range bases {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			for _, f := range []string{"Secure Preferences", "Preferences"} {
				raw, err := os.ReadFile(filepath.Join(base, e.Name(), f))
				if err != nil {
					continue
				}
				var prefs struct {
					Extensions struct {
						Settings map[string]struct {
							Path string `json:"path"`
						} `json:"settings"`
					} `json:"extensions"`
				}
				if json.Unmarshal(raw, &prefs) != nil {
					continue
				}
				if s, ok := prefs.Extensions.Settings[extensionID]; ok && s.Path != "" && filepath.IsAbs(s.Path) {
					out = append(out, s.Path)
				}
			}
		}
	}
	return out
}

func findChrome() string {
	const key = `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\chrome.exe`
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		if k, err := registry.OpenKey(root, key, registry.QUERY_VALUE); err == nil {
			p, _, err := k.GetStringValue("")
			k.Close()
			if err == nil && p != "" {
				return p
			}
		}
	}
	for _, p := range []string{
		filepath.Join(os.Getenv("ProgramFiles"), `Google\Chrome\Application\chrome.exe`),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), `Google\Chrome\Application\chrome.exe`),
		filepath.Join(os.Getenv("LOCALAPPDATA"), `Google\Chrome\Application\chrome.exe`),
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func openInChrome(url string) bool {
	chrome := findChrome()
	if chrome == "" {
		return false
	}
	return exec.Command(chrome, url).Start() == nil
}
