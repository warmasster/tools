package main

// Menú nuevo de Windows 11 (el que sale sin pulsar «Mostrar más opciones»).
//
// Windows solo lo enseña para programas con identidad de paquete, así que se registra un
// paquete «sparse» que apunta a la carpeta de TurboMENU.exe, firmado con un certificado
// propio creado en este PC (CN=TurboMENU) en el que solo confía este usuario. Es opcional
// y se activa desde la ventana de inicio. El Explorador lanza TurboMENU.exe como servidor
// COM (-TurboMenuCom) para preguntarle qué opciones enseñar.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"turbomenu/internal/win11menu"
)

const modernPkgName = "Turbo.TurboMENU"

func modernPackage(exe, thumb string) win11menu.Package {
	return win11menu.Package{
		Name:        modernPkgName,
		Publisher:   "CN=TurboMENU",
		DisplayName: "TurboMENU",
		Description: "Acciones rápidas en el clic derecho",
		Version:     version + ".0",
		AppID:       "TurboMENU",
		ExePath:     exe,
		CertThumb:   thumb,
	}
}

// modernMenu es el mismo menú que el clásico, con sus condiciones de visibilidad.
func modernMenu(exe string) win11menu.Menu {
	conv := []string{}
	for _, e := range convertibleExts() {
		conv = append(conv, "."+e)
	}
	files, items, bg := win11menu.Files, win11menu.Items, win11menu.Background
	single := win11menu.All(files, win11menu.Single)
	needSend := func(win11menu.Selection) string {
		if detectDeps().TurboSEND == "" {
			return "necesita TurboSEND"
		}
		return ""
	}
	ic := func(id string) string { return iconFor(id, exe) }
	return win11menu.Menu{
		Title: "Turbo",
		Icon:  ic("turbo"),
		Commands: []win11menu.Command{
			{ID: "reduce", Title: "Reducir tamaño", Icon: ic("reduce"), Show: items},
			{ID: "convert", Title: "Convertir", Icon: ic("convert"), Show: win11menu.All(files, win11menu.Exts(conv...))},
			{ID: "send", Title: "Enviar con TurboSEND", Icon: ic("send"), Show: items, Disabled: needSend, SeparatorBefore: true},
			{ID: "qr", Title: "Pasar al móvil", Icon: ic("qr"), Show: items},
			{ID: "safe", Title: "¿Es seguro?", Icon: ic("safe"), Show: single, SeparatorBefore: true},
			{ID: "lock", Title: "¿Qué lo bloquea?", Icon: ic("lock"), Show: items},
			{ID: "origin", Title: "¿De dónde salió?", Icon: ic("origin"), Show: single},
			{ID: "folder", Title: "Nueva carpeta con esto", Icon: ic("folder"), Show: items, SeparatorBefore: true},
			{ID: "paste", Title: "Pegar como archivo", Icon: ic("paste"), Show: bg},
			{ID: "receive", Title: "Recibir del móvil", Icon: ic("receive"), Show: bg},
		},
	}
}

// serveModern atiende al Explorador. Cada acción se abre en un TurboMENU normal (que a su
// vez reutiliza la ventana preparada si la hay), con todas las rutas de una vez.
func serveModern() {
	exe, _ := os.Executable()
	win11menu.Serve(modernMenu(exe), win11menu.ServeOptions{Idle: 90 * time.Second}, func(action string, sel win11menu.Selection) {
		args := []string{action}
		paths := sel.Paths
		if strings.Join(paths, "") != "" && len(strings.Join(paths, `" "`)) > 24000 {
			// Demasiadas rutas para la línea de órdenes: van en un archivo.
			f, err := os.CreateTemp("", "TurboMENU-sel-*.txt")
			if err == nil {
				f.WriteString(strings.Join(paths, "\n"))
				f.Close()
				paths = []string{"@" + f.Name()}
			}
		}
		args = append(args, paths...)
		allowForeground(-1)
		cmd := exec.Command(exe, args...)
		cmd.Dir = filepath.Dir(exe)
		if cmd.Start() == nil {
			go cmd.Wait()
		}
	})
}

// enableModernMenu crea el certificado, confía en él (solo este usuario) y registra el paquete.
func enableModernMenu(exe string) error {
	had, _ := win11menu.FindCert("CN=TurboMENU")
	thumb, err := win11menu.EnsureCert("CN=TurboMENU")
	if err != nil {
		return errors.New("No se pudo crear el certificado de TurboMENU: " + err.Error())
	}
	if err := win11menu.TrustCert(thumb); err != nil {
		return errors.New("No se pudo confiar en el certificado: " + err.Error())
	}
	if err := win11menu.Register(modernPackage(exe, thumb)); err != nil {
		if had == "" {
			// Recién creado y sin usar: no dejar un certificado de confianza suelto. Si
			// ya existía, lo usa el paquete que sigue instalado: se deja.
			win11menu.RemoveCert(thumb)
		}
		return errors.New("Windows no ha aceptado el menú nuevo: " + err.Error())
	}
	// El paquete ya sale también en «Mostrar más opciones»: fuera el clásico para no duplicar.
	unregisterMenu()
	return nil
}

func disableModernMenu() {
	win11menu.Unregister(modernPkgName)
	if thumb, err := win11menu.FindCert("CN=TurboMENU"); err == nil && thumb != "" {
		win11menu.RemoveCert(thumb)
	}
}

func registerModernMenu(exe string) {
	if getSettings().Modern {
		if enableModernMenu(exe) != nil {
			updateSettings(func(s *Settings) { s.Modern = false })
		}
	}
}

func unregisterModernMenu() {
	if getSettings().Modern {
		disableModernMenu()
	}
}

func modernMenuInstalled() bool { return getSettings().Modern }

// SetModernMenu activa o quita la entrada en el menú nuevo de Windows 11.
func (a *App) SetModernMenu(on bool) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if on {
		if err := enableModernMenu(exe); err != nil {
			registerMenu(exe) // que al menos quede el clásico
			return err
		}
		return updateSettings(func(s *Settings) { s.Modern = true })
	}
	disableModernMenu()
	updateSettings(func(s *Settings) { s.Modern = false })
	return registerMenu(exe)
}
