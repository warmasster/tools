package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// En el modo normal un archivo con dos nombres (enlace duro) cuenta una vez, como
// en el modo turbo; al releer o quitar uno de sus nombres se sigue contando bien.
func TestWalkHardLinks(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a", "datos.bin"), 4000)
	writeFile(t, filepath.Join(dir, "otro.txt"), 10)
	os.MkdirAll(filepath.Join(dir, "b"), 0o755)
	if err := os.Link(filepath.Join(dir, "a", "datos.bin"), filepath.Join(dir, "b", "copia.bin")); err != nil {
		t.Skip("este disco no admite enlaces duros:", err)
	}
	tr, err := walkScan(context.Background(), dir, &Progress{})
	if err != nil {
		t.Fatal(err)
	}
	if tr.noFileIDs.Load() {
		t.Skip("el sistema de archivos no da identificadores")
	}
	root := func() int64 { return tr.Nodes[tr.Root].Size }
	if root() != 4010 || tr.Nodes[tr.Root].Files != 3 {
		t.Fatalf("raíz: %d bytes, %d archivos (quería 4010 y 3)", root(), tr.Nodes[tr.Root].Files)
	}
	a, b := tr.findPath("a"), tr.findPath("b")
	// Cuenta el nombre de la ruta que va antes: a\datos.bin.
	if tr.Nodes[a].Size != 4000 || tr.Nodes[b].Size != 0 {
		t.Fatalf("a=%d b=%d", tr.Nodes[a].Size, tr.Nodes[b].Size)
	}
	if tr.Nodes[tr.findPath(`b\copia.bin`)].Flags&fHardLink == 0 {
		t.Fatal(`b\copia.bin debería estar marcado como enlace`)
	}

	// Releer la carpeta del nombre que cuenta: el archivo sigue contando una vez.
	na := refreshSubtree(tr, a)
	if root() != 4010 || tr.Nodes[na].Size != 4000 {
		t.Fatalf("tras releer a: raíz %d, a %d", root(), tr.Nodes[na].Size)
	}
	// Releer la otra: igual.
	nb := refreshSubtree(tr, tr.findPath("b"))
	if root() != 4010 || tr.Nodes[nb].Size != 0 {
		t.Fatalf("tras releer b: raíz %d, b %d", root(), tr.Nodes[nb].Size)
	}

	// Quitar el nombre que cuenta: el espacio sigue ocupado, pasa a contar el otro.
	tr.remove(tr.findPath(`a\datos.bin`))
	if root() != 4010 || tr.Nodes[tr.findPath("b")].Size != 4000 {
		t.Fatalf("tras quitar datos.bin: raíz %d, b %d", root(), tr.Nodes[tr.findPath("b")].Size)
	}
	// Y quitar el último: ahora sí baja.
	tr.remove(tr.findPath(`b\copia.bin`))
	if root() != 10 {
		t.Fatalf("tras quitar los dos: raíz %d", root())
	}
}

func TestGrowthNote(t *testing.T) {
	if growthNote(&Snapshot{Mode: "mft"}, "walk") == "" {
		t.Error("modos distintos deberían avisar")
	}
	if growthNote(&Snapshot{Mode: "walk"}, "walk") != "" {
		t.Error("mismo modo: sin aviso")
	}
	if growthNote(&Snapshot{}, "walk") == "" {
		t.Error("un análisis normal de antes de esta versión debería avisar")
	}
	if growthNote(&Snapshot{}, "mft") != "" {
		t.Error("el modo turbo contaba igual que ahora")
	}
}
