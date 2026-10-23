// proto: prueba del menú moderno de Windows 11 con servidor COM fuera de proceso.
//
//	proto install <carpeta>   copia el exe a <carpeta>, le mete el manifiesto con <msix> y registra el paquete
//	proto uninstall           quita el paquete
//	proto msix <archivo>      solo escribe el .msix
//	proto -TurboMenuCom       (lo lanza el sistema) servidor COM
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"turbomenu/internal/win11menu"
)

var pkg = win11menu.Package{
	Name:        "Turbo.TurboMENU.Proto",
	DisplayName: "TurboMENU Proto",
	AppID:       "TurboMENU",
}

func logPath() string { return filepath.Join(os.TempDir(), "turbomenu-proto.log") }

func main() {
	if win11menu.IsServerLaunch(os.Args[1:]) {
		serve()
		return
	}
	if len(os.Args) < 2 {
		fmt.Println("uso: proto install <carpeta> | uninstall | msix <archivo>")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "install":
		err = install(os.Args[2])
	case "uninstall":
		err = win11menu.Unregister(pkg.Name)
	case "msix":
		pkg.ExePath = `C:\x\TurboMENU.exe`
		var f *os.File
		if f, err = os.Create(os.Args[2]); err == nil {
			err = pkg.WriteMSIX(f)
			f.Close()
		}
	case "msixfrom": // msixfrom <manifiesto> <salida>
		var b []byte
		if b, err = os.ReadFile(os.Args[2]); err == nil {
			var f *os.File
			if f, err = os.Create(os.Args[3]); err == nil {
				err = win11menu.WriteMSIXManifest(f, string(b), nil)
				f.Close()
			}
		}
	case "ctx":
		err = ctx(os.Args[2:])
	case "classic-reg": // classic-reg <carpeta>
		os.MkdirAll(os.Args[2], 0o755)
		self, _ := os.Executable()
		dst := filepath.Join(os.Args[2], "TurboMENU.exe")
		if err = copyFile(self, dst); err == nil {
			err = classicReg(dst)
		}
	case "classic-unreg":
		classicUnreg()
	case "cleanup": // deja el sistema como estaba
		classicUnreg()
		win11menu.Unregister(pkg.Name)
		powershellCleanCert()
	case "sign": // sign <msix> <huella>
		err = win11menu.SignMSIX(os.Args[2], os.Args[3])
	case "excmd":
		err = excmd(os.Args[2:])
	case "cocreate":
		err = cocreate()
	case "manifest":
		pkg.ExePath = `C:\x\TurboMENU.exe`
		fmt.Println(pkg.Manifest())
	}
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}

func install(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	self, _ := os.Executable()
	dst := filepath.Join(dir, "TurboMENU.exe")
	if err := copyFile(self, dst); err != nil {
		return err
	}
	if os.Getenv("PROTO_SIGN") != "" {
		thumb, err := win11menu.EnsureCert("CN=TurboMENU")
		if err != nil {
			return err
		}
		fmt.Println("certificado", thumb)
		pkg.CertThumb = thumb
		if os.Getenv("PROTO_TRUST") != "" {
			if err := win11menu.TrustCert(thumb); err != nil {
				return err
			}
		}
	}
	if os.Getenv("PROTO_NO_MSIX_ELEMENT") == "" {
		man := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly manifestVersion="1.0" xmlns="urn:schemas-microsoft-com:asm.v1">
  <assemblyIdentity type="win32" name="Turbo.TurboMENU" version="1.0.0.0" processorArchitecture="*"/>
  ` + pkg.ExeManifestElement() + `
</assembly>`
		if err := embedManifest(dst, man); err != nil {
			return fmt.Errorf("manifiesto: %w", err)
		}
	}
	pkg.ExePath = dst
	t := time.Now()
	err := win11menu.Register(pkg)
	fmt.Println("registro en", time.Since(t))
	return err
}

func copyFile(a, b string) error {
	in, err := os.Open(a)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(b)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// embedManifest mete RT_MANIFEST #1 con UpdateResourceW (sin herramientas externas).
func embedManifest(exe, xml string) error {
	k := windows.NewLazySystemDLL("kernel32.dll")
	begin, upd, end := k.NewProc("BeginUpdateResourceW"), k.NewProc("UpdateResourceW"), k.NewProc("EndUpdateResourceW")
	p, _ := windows.UTF16PtrFromString(exe)
	h, _, e := begin.Call(uintptr(unsafe.Pointer(p)), 0)
	if h == 0 {
		return e
	}
	data := []byte(xml)
	if r, _, e := upd.Call(h, 24, 1, 0x0409, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data))); r == 0 {
		end.Call(h, 1)
		return e
	}
	if r, _, e := end.Call(h, 0); r == 0 {
		return e
	}
	return nil
}

func serve() {
	log := win11menu.FileLog(logPath())
	exe, _ := os.Executable()
	ico := exe + ",0"
	m := win11menu.Menu{
		Title: "Turbo",
		Icon:  `C:\Windows\System32\shell32.dll,-16802`,
		Commands: []win11menu.Command{
			{ID: "reducir", Title: "Reducir tamaño", Icon: `C:\Windows\System32\imageres.dll,-5205`,
				Show: win11menu.Exts(".jpg", ".jpeg", ".png", ".mp4", ".pdf")},
			{ID: "convertir", Title: "Convertir", Show: win11menu.Files, Sub: []win11menu.Command{
				{ID: "conv-png", Title: "A PNG", Show: win11menu.Exts(".jpg", ".webp")},
				{ID: "conv-jpg", Title: "A JPG", Show: win11menu.Exts(".png", ".webp")},
				{ID: "conv-pdf", Title: "A PDF"},
			}},
			{ID: "enviar", Title: "Enviar con TurboSEND", Icon: ico, Show: win11menu.Items,
				Disabled: func(win11menu.Selection) string { return "necesita TurboSEND" }},
			{ID: "seguro", Title: "¿Es seguro?", Show: win11menu.All(win11menu.Files, win11menu.Single), SeparatorBefore: true},
			{ID: "nueva-carpeta", Title: "Nueva carpeta con esto", Show: win11menu.Items},
			{ID: "pegar", Title: "Pegar como archivo", Show: win11menu.Background},
		},
	}
	err := win11menu.Serve(m, win11menu.ServeOptions{Idle: 20 * time.Second, Log: log},
		func(action string, sel win11menu.Selection) {
			log(fmt.Sprintf("ACCIÓN %s bg=%v rutas=%s", action, sel.Background, strings.Join(sel.Paths, " | ")))
		})
	if err != nil {
		log("error: " + err.Error())
	}
}

// Verbos de prueba del menú clásico con DelegateExecute.
var testKeys = []string{
	`Software\Classes\*\shell\TurboProto`,
	`Software\Classes\Directory\Background\shell\TurboProto`,
}

func classicReg(exe string) error {
	if err := win11menu.RegisterExecuteServer(exe); err != nil {
		return err
	}
	set := func(path string, vals map[string]string) error {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
		if err != nil {
			return err
		}
		defer k.Close()
		for n, v := range vals {
			if err := k.SetStringValue(n, v); err != nil {
				return err
			}
		}
		return nil
	}
	de := map[string]string{"DelegateExecute": win11menu.DelegateExecuteValue()}
	items := []struct{ parent, id, title, applies string }{
		{testKeys[0], "reducir", "Reducir tamaño", `System.FileExtension:=.jpg OR System.FileExtension:=.png`},
		{testKeys[0], "enviar", "Enviar con TurboSEND", ""},
		{testKeys[1], "pegar", "Pegar como archivo", ""},
	}
	for _, p := range testKeys {
		if err := set(p, map[string]string{"MUIVerb": "Turbo", "SubCommands": ""}); err != nil {
			return err
		}
	}
	for _, it := range items {
		v := map[string]string{"MUIVerb": it.title, "MultiSelectModel": "Player"}
		if it.applies != "" {
			v["AppliesTo"] = it.applies
		}
		if err := set(it.parent+`\shell\`+it.id, v); err != nil {
			return err
		}
		if err := set(it.parent+`\shell\`+it.id+`\command`, de); err != nil {
			return err
		}
	}
	return nil
}

func classicUnreg() {
	for _, k := range append(testKeys,
		`Software\Classes\*\shell\TurboMENU`, `Software\Classes\Directory\shell\TurboMENU`,
		`Software\Classes\Directory\Background\shell\TurboMENU`,
		`Software\Classes\CLSID\`+win11menu.CLSIDItems.String(),
		`Software\Classes\CLSID\`+win11menu.CLSIDBackground.String()) {
		win11menu.DeleteKeyTree(registry.CURRENT_USER, k)
	}
	win11menu.UnregisterExecuteServer()
}

func powershellCleanCert() {
	if thumb, err := win11menu.FindCert("CN=TurboMENU"); err == nil && thumb != "" {
		win11menu.RemoveCert(thumb)
	}
}
