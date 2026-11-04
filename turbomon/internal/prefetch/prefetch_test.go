package prefetch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDOSPath(t *testing.T) {
	r := NewRecorder()
	if len(r.dev) == 0 {
		t.Fatal("sin volúmenes")
	}
	for d, l := range r.dev {
		if l == "C:" {
			got := r.DOSPath(d + `\Juegos\x.pak`)
			if !strings.EqualFold(got, `C:\Juegos\x.pak`) {
				t.Fatalf("%q", got)
			}
			return
		}
	}
	t.Fatal("no está C:")
}

func TestMergeAndWarm(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "datos.pak")
	os.WriteFile(f, make([]byte, 9<<20), 0o644)
	a := Plan{Files: []PlanFile{{Path: f, Chunks: []uint32{0, 2}}}, Runs: 1}
	b := Plan{Files: []PlanFile{{Path: strings.ToUpper(f), Chunks: []uint32{1}}}, Runs: 1}
	m := Merge(a, b, 1<<30)
	if len(m.Files) != 1 || len(m.Files[0].Chunks) != 3 || m.Runs != 2 {
		t.Fatalf("%+v", m)
	}
	n := Warm(context.Background(), m)
	if n != 9<<20 {
		t.Fatalf("leídos %d", n)
	}
}
