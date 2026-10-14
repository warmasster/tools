package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestZip64 genera un ZIP con un archivo de más de 4 GB y comprueba que el tamaño
// calculado de antemano coincide y que el resultado se lee bien.
func TestZip64(t *testing.T) {
	if testing.Short() {
		t.Skip("crea un ZIP de más de 4 GB")
	}
	dir := t.TempDir()
	big := filepath.Join(dir, "grande.bin")
	f, _ := os.Create(big)
	f.Truncate(4<<30 + 12345)
	f.Close()
	small := filepath.Join(dir, "peque.txt")
	os.WriteFile(small, []byte("hola"), 0o644)

	files := []ShareFile{
		{Path: small, Rel: "c/peque.txt", Size: 4},
		{Path: big, Rel: "c/grande.bin", Size: 4<<30 + 12345},
		{Path: small, Rel: "c/después.txt", Size: 4},
	}
	entries, size := zipLayout(files)
	out := filepath.Join(dir, "out.zip")
	of, _ := os.Create(out)
	if err := writeZip(of, entries, make([]byte, 1<<20)); err != nil {
		t.Fatal(err)
	}
	of.Close()
	st, _ := os.Stat(out)
	if st.Size() != size {
		t.Fatalf("tamaño real %d != calculado %d", st.Size(), size)
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, zf := range zr.File {
		rc, err := zf.Open()
		if err != nil {
			t.Fatal(zf.Name, err)
		}
		n, err := io.Copy(io.Discard, rc) // comprueba también el CRC
		rc.Close()
		if err != nil || uint64(n) != zf.UncompressedSize64 {
			t.Fatalf("%s: %v (%d bytes)", zf.Name, err, n)
		}
	}
	if zr.File[2].Name != "c/después.txt" {
		t.Fatal("nombre UTF-8 mal", zr.File[2].Name)
	}
}

func TestZipSmallLayout(t *testing.T) {
	dir := t.TempDir()
	var files []ShareFile
	for i := 0; i < 70000; i++ { // más de 65535 entradas -> ZIP64 al final
		files = append(files, ShareFile{Path: filepath.Join(dir, "x"), Rel: "a/b.txt", Size: 0})
	}
	os.WriteFile(filepath.Join(dir, "x"), nil, 0o644)
	entries, size := zipLayout(files)
	cw := &countWriter{w: io.Discard}
	if err := writeZip(cw, entries, make([]byte, 4096)); err != nil {
		t.Fatal(err)
	}
	if cw.n != size {
		t.Fatalf("real %d != calculado %d", cw.n, size)
	}
}
