package win11menu

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func TestMSIX(t *testing.T) {
	p := Package{Name: "Turbo.Prueba", ExePath: `C:\x\TurboMENU.exe`}
	var b bytes.Buffer
	if err := p.WriteMSIX(&b); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b.Bytes()), int64(b.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	var names []string
	for _, f := range zr.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = data
		names = append(names, f.Name)
		if f.Method != zip.Store || f.Flags&0x8 != 0 {
			t.Errorf("%s: debe ir sin comprimir y sin descriptor", f.Name)
		}
	}
	if got := strings.Join(names, ","); got != "AppxManifest.xml,logo.png,AppxBlockMap.xml,[Content_Types].xml" {
		t.Fatalf("orden de archivos: %s", got)
	}
	var bm struct {
		Files []struct {
			Name   string `xml:"Name,attr"`
			Size   int    `xml:"Size,attr"`
			Blocks []struct {
				Hash string `xml:"Hash,attr"`
			} `xml:"Block"`
		} `xml:"File"`
	}
	if err := xml.Unmarshal(files["AppxBlockMap.xml"], &bm); err != nil {
		t.Fatal(err)
	}
	for _, f := range bm.Files {
		h := sha256.Sum256(files[f.Name])
		if len(f.Blocks) != 1 || f.Blocks[0].Hash != base64.StdEncoding.EncodeToString(h[:]) || f.Size != len(files[f.Name]) {
			t.Errorf("mapa de bloques mal para %s", f.Name)
		}
	}
	m := string(files["AppxManifest.xml"])
	for _, s := range []string{UnsignedPublisherOID, ServerArg, `Type="*"`, `Directory\Background`, strings.Trim(CLSIDItems.String(), "{}")} {
		if !strings.Contains(m, s) {
			t.Errorf("falta %q en el manifiesto", s)
		}
	}
	if err := xml.Unmarshal([]byte(m), new(struct{})); err != nil {
		t.Errorf("manifiesto no es XML válido: %v", err)
	}
	if !strings.Contains(p.ExeManifestElement(), `packageName="Turbo.Prueba"`) {
		t.Error("elemento msix")
	}
}

func TestPredicados(t *testing.T) {
	fotos := Selection{Paths: []string{`C:\a.JPG`, `C:\b.png`}, Dirs: []bool{false, false}}
	mezcla := Selection{Paths: []string{`C:\a.jpg`, `C:\d`}, Dirs: []bool{false, true}}
	fondo := Selection{Paths: []string{`C:\d`}, Dirs: []bool{true}, Background: true}
	img := Exts(".jpg", ".png")
	cases := []struct {
		name string
		f    func(Selection) bool
		s    Selection
		want bool
	}{
		{"exts", img, fotos, true},
		{"exts mezcla", img, mezcla, false},
		{"files", Files, mezcla, false},
		{"items", Items, mezcla, true},
		{"fondo", Background, fondo, true},
		{"items fondo", Items, fondo, false},
		{"single", Single, fotos, false},
		{"all", All(Files, Single), Selection{Paths: []string{"x"}, Dirs: []bool{false}}, true},
	}
	for _, c := range cases {
		if got := c.f(c.s); got != c.want {
			t.Errorf("%s = %v", c.name, got)
		}
	}
	sub := Command{Sub: []Command{{Show: Folders}}}
	if sub.visible(fotos) {
		t.Error("un submenú sin hijos visibles debe ocultarse")
	}
}
