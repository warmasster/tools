package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Deshacer «Nueva carpeta con esto» no pisa lo que haya aparecido después en el
// sitio de origen, y solo trabaja con lo que hizo MakeFolder.
func TestUndoFolderDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	a1 := filepath.Join(dir, "informe.docx")
	a2 := filepath.Join(dir, "notas.txt")
	os.WriteFile(a1, []byte("viejo"), 0o644)
	os.WriteFile(a2, []byte("notas"), 0o644)

	app := NewApp(&Job{Action: "folder", Paths: []string{a1, a2}}, false)
	res, err := app.MakeFolder("Trabajo")
	if err != nil || len(res.Moved) != 2 {
		t.Fatalf("MakeFolder: %v %+v", err, res)
	}
	// Mientras tanto, el usuario guarda un informe nuevo donde estaba el viejo.
	os.WriteFile(a1, []byte("nuevo"), 0o644)

	// Lo que mande la página no cuenta: aunque pida mover otra cosa, se usa lo guardado.
	err = app.UndoFolder(FolderResult{Moved: []string{filepath.Join(dir, "otro")}, From: []string{a2}})
	if err == nil {
		t.Error("debería avisar de que un archivo no se pudo devolver")
	}
	if b, _ := os.ReadFile(a1); string(b) != "nuevo" {
		t.Errorf("Deshacer ha pisado el archivo nuevo: %q", b)
	}
	if b, _ := os.ReadFile(a2); string(b) != "notas" {
		t.Errorf("notas.txt no ha vuelto a su sitio: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(res.Folder, "informe.docx")); string(b) != "viejo" {
		t.Errorf("el informe viejo debería seguir en la carpeta nueva: %q", b)
	}
	if err := app.UndoFolder(FolderResult{}); err == nil {
		t.Error("un segundo Deshacer no debería hacer nada")
	}
}
