// Genera rsrc_windows_amd64.syso (icono, versión y manifiesto de Windows) para
// TurboDL.exe a partir de los PNG de la extensión. Lo usa build.ps1:
//
//	go run . -icons ../../../icons -out ../../rsrc_windows_amd64.syso -version 1.1.0
package main

import (
	"flag"
	"image"
	_ "image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/tc-hib/winres"
	"github.com/tc-hib/winres/version"
)

func main() {
	icons := flag.String("icons", "", "carpeta con icon16/32/48/128.png")
	out := flag.String("out", "rsrc_windows_amd64.syso", "archivo .syso de salida")
	ver := flag.String("version", "1.0.0", "versión")
	flag.Parse()

	var imgs []image.Image
	for _, n := range []string{"icon16.png", "icon32.png", "icon48.png", "icon128.png"} {
		f, err := os.Open(filepath.Join(*icons, n))
		if err != nil {
			log.Fatal(err)
		}
		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			log.Fatal(err)
		}
		imgs = append(imgs, img)
	}
	icon, err := winres.NewIconFromImages(imgs)
	if err != nil {
		log.Fatal(err)
	}

	rs := &winres.ResourceSet{}
	rs.SetIcon(winres.Name("APPICON"), icon)
	rs.SetManifest(winres.AppManifest{
		ExecutionLevel:      winres.AsInvoker, // sin pedir administrador
		DPIAwareness:        winres.DPIPerMonitorV2,
		UseCommonControlsV6: true,
	})
	vi := version.Info{}
	vi.SetFileVersion(*ver)
	vi.SetProductVersion(*ver)
	vi.Set(0, version.ProductName, "TurboDL")
	vi.Set(0, version.FileDescription, "TurboDL: instalador y motor nativo de descargas")
	vi.Set(0, version.OriginalFilename, "TurboDL.exe")
	rs.SetVersionInfo(vi)

	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := rs.WriteObject(f, winres.ArchAMD64); err != nil {
		log.Fatal(err)
	}
}
