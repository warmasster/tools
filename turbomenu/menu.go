package main

// Entrada «Turbo» del menú contextual clásico (registro de HKCU, no necesita
// administrador). En Windows 11 sale en «Mostrar más opciones» y con Mayús+clic derecho.

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const classesKey = `Software\Classes\`
const verbName = "TurboMENU"

type menuVerb struct {
	ID        string // acción (y nombre del icono)
	Title     string
	Arg       string // %1 para elementos, %V para el fondo de una carpeta
	AppliesTo string // condición AQS opcional
	Needs     string // dependencia: si falta, la entrada no se registra
}

// Extensiones que «Convertir» sabe tratar.
func convertibleExts() []string {
	var out []string
	for e, k := range kindByExt {
		switch k {
		case "video", "audio", "image", "pdf", "doc":
			out = append(out, e)
		}
	}
	sort.Strings(out)
	return out
}

func appliesToExts(exts []string) string {
	parts := make([]string, len(exts))
	for i, e := range exts {
		parts[i] = "System.FileExtension:=." + e
	}
	return strings.Join(parts, " OR ")
}

// menuLayout: dónde va cada submenú y qué lleva.
func menuLayout() map[string][]menuVerb {
	conv := appliesToExts(convertibleExts())
	return map[string][]menuVerb{
		`*`: {
			{ID: "reduce", Title: "Reducir tamaño", Arg: "%1"},
			{ID: "convert", Title: "Convertir", Arg: "%1", AppliesTo: conv},
			{ID: "send", Title: "Enviar con TurboSEND", Arg: "%1", Needs: "turbosend"},
			{ID: "qr", Title: "Pasar al móvil", Arg: "%1"},
			{ID: "safe", Title: "¿Es seguro?", Arg: "%1"},
			{ID: "lock", Title: "¿Qué lo bloquea?", Arg: "%1"},
			{ID: "origin", Title: "¿De dónde salió?", Arg: "%1"},
			{ID: "folder", Title: "Nueva carpeta con esto", Arg: "%1"},
		},
		`Directory`: {
			{ID: "reduce", Title: "Reducir tamaño", Arg: "%1"},
			{ID: "send", Title: "Enviar con TurboSEND", Arg: "%1", Needs: "turbosend"},
			{ID: "qr", Title: "Pasar al móvil", Arg: "%1"},
			{ID: "lock", Title: "¿Qué lo bloquea?", Arg: "%1"},
			{ID: "folder", Title: "Nueva carpeta con esto", Arg: "%1"},
		},
		`Directory\Background`: {
			{ID: "paste", Title: "Pegar como archivo", Arg: "%V"},
			{ID: "receive", Title: "Recibir del móvil", Arg: "%V"},
		},
		`DesktopBackground`: {
			{ID: "paste", Title: "Pegar como archivo", Arg: "%V"},
			{ID: "receive", Title: "Recibir del móvil", Arg: "%V"},
		},
		`Drive`: {
			{ID: "lock", Title: "¿Qué lo bloquea? (para expulsar)", Arg: "%1"},
		},
	}
}

func iconDir() string { return filepath.Join(cacheDir(), "icons") }

//go:embed menuicons/*.ico
var menuIcons embed.FS

// writeMenuIcons deja los iconos del menú en disco (el registro necesita rutas).
func writeMenuIcons() {
	os.MkdirAll(iconDir(), 0o755)
	ents, _ := menuIcons.ReadDir("menuicons")
	for _, e := range ents {
		b, _ := menuIcons.ReadFile("menuicons/" + e.Name())
		dst := filepath.Join(iconDir(), e.Name())
		if old, err := os.ReadFile(dst); err == nil && string(old) == string(b) {
			continue
		}
		os.WriteFile(dst, b, 0o644)
	}
}

// registerMenu escribe (o reescribe) todas las entradas.
func registerMenu(exe string) error {
	writeMenuIcons()
	deps := detectDeps()
	for root, verbs := range menuLayout() {
		base := classesKey + root + `\shell\` + verbName
		deleteTree(registry.CURRENT_USER, base)
		k, _, err := registry.CreateKey(registry.CURRENT_USER, base, registry.ALL_ACCESS)
		if err != nil {
			return fmt.Errorf("no se pudo escribir en el registro: %w", err)
		}
		k.SetStringValue("MUIVerb", "Turbo")
		k.SetStringValue("Icon", iconFor("turbo", exe))
		k.SetStringValue("SubCommands", "")
		k.Close()
		n := 0
		for _, v := range verbs {
			if v.Needs == "turbosend" && deps.TurboSEND == "" {
				continue
			}
			n++
			vk, _, err := registry.CreateKey(registry.CURRENT_USER, fmt.Sprintf(`%s\shell\%02d%s`, base, n, v.ID), registry.ALL_ACCESS)
			if err != nil {
				return err
			}
			vk.SetStringValue("MUIVerb", v.Title)
			vk.SetStringValue("Icon", iconFor(v.ID, exe))
			vk.SetStringValue("MultiSelectModel", "Player")
			if v.AppliesTo != "" {
				vk.SetStringValue("AppliesTo", v.AppliesTo)
			}
			if v.ID == "folder" || v.ID == "send" {
				// separador visual antes de los grupos
				vk.SetDWordValue("CommandFlags", 0x20) // ECF_SEPARATORBEFORE
			}
			ck, _, err := registry.CreateKey(registry.CURRENT_USER, fmt.Sprintf(`%s\shell\%02d%s\command`, base, n, v.ID), registry.ALL_ACCESS)
			if err != nil {
				vk.Close()
				return err
			}
			ck.SetStringValue("", fmt.Sprintf(`"%s" %s "%s"`, exe, v.ID, v.Arg))
			ck.Close()
			vk.Close()
		}
	}
	shellChanged()
	return nil
}

func unregisterMenu() {
	for root := range menuLayout() {
		deleteTree(registry.CURRENT_USER, classesKey+root+`\shell\`+verbName)
	}
	shellChanged()
}

// menuInstalled dice si las entradas están y apuntan a este ejecutable.
func menuInstalled(exe string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, classesKey+`*\shell\`+verbName+`\shell\01reduce\command`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, _ := k.GetStringValue("")
	return strings.Contains(strings.ToLower(v), strings.ToLower(exe))
}

// syncMenu rehace el menú si ha cambiado algo (otra ruta del .exe, TurboSEND instalado
// o quitado…). Se llama en segundo plano cada vez que se abre una ventana.
func syncMenu() {
	exe, err := os.Executable()
	if err != nil || !menuInstalled(exe) {
		return // no lo registramos si nadie lo instaló (p. ej. al probar el .exe suelto)
	}
	hasSend := false
	if k, err := registry.OpenKey(registry.CURRENT_USER, classesKey+`*\shell\`+verbName+`\shell`, registry.ENUMERATE_SUB_KEYS); err == nil {
		names, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, n := range names {
			if strings.HasSuffix(n, "send") {
				hasSend = true
			}
		}
	}
	if hasSend != (detectDeps().TurboSEND != "") {
		registerMenu(exe)
	}
}

func iconFor(id, exe string) string {
	p := filepath.Join(iconDir(), id+".ico")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return exe + ",0"
}

func deleteTree(root registry.Key, path string) {
	k, err := registry.OpenKey(root, path, registry.ENUMERATE_SUB_KEYS)
	if err == nil {
		names, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, n := range names {
			deleteTree(root, path+`\`+n)
		}
	}
	registry.DeleteKey(root, path)
}

func shellChanged() {
	procSHChangeNotify.Call(0x08000000, 0, 0, 0) // SHCNE_ASSOCCHANGED
}

// ---------- órdenes de TurboSETUP ----------

func cliSetup() int {
	exe, err := os.Executable()
	if err != nil {
		return 1
	}
	// Al actualizar, el TurboMENU viejo que queda preparado en segundo plano seguiría
	// atendiendo el clic derecho hasta 15 min: se cierra para que entre el nuevo.
	quitIdle()
	if getSettings().Modern {
		registerModernMenu(exe) // si falla, vuelve al clásico
	}
	if !getSettings().Modern {
		if err := registerMenu(exe); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	setPrewarm(getSettings().Prewarm)
	if getSettings().Prewarm {
		// Se cerró el preparado viejo: el nuevo queda listo ya, no al volver a iniciar sesión.
		// (Sin salida estándar: no retiene la tubería de quien lanzó «setup».)
		if cmd := exec.Command(exe, "--idle"); cmd.Start() == nil {
			cmd.Process.Release()
		}
	}
	fmt.Println("TurboMENU instalado: busca «Turbo» en el clic derecho.")
	return 0
}

func cliUninstall() int {
	unregisterMenu()
	// Siempre, aunque el ajuste diga que está apagado: si un intento de activarlo
	// falló a medias, o se borraron los ajustes, quedarían el paquete y el certificado.
	disableModernMenu()
	setPrewarm(false)
	// Cierra cualquier TurboMENU preparado en segundo plano.
	quitIdle()
	os.RemoveAll(cacheDir())
	fmt.Println("TurboMENU desinstalado.")
	return 0
}

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// setPrewarm activa o quita el arranque oculto al iniciar sesión.
func setPrewarm(on bool) {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	if !on {
		k.DeleteValue("TurboMENU")
		return
	}
	exe, _ := os.Executable()
	k.SetStringValue("TurboMENU", fmt.Sprintf(`"%s" --idle`, exe))
}
