package main

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestRealFindDupes(t *testing.T) {
	root := os.Getenv("TURBOCUT_SCAN")
	if root == "" {
		t.Skip()
	}
	a := testApp(t, cleanRoot(root))
	for _, q := range []string{"a", "vacaciones", "selene v4", ".mp4", "ext:gguf,safetensors"} {
		r, _ := a.Find(FindQuery{Scope: a.tree.Root, Cat: -1, Text: q})
		r2, _ := a.Find(FindQuery{Scope: a.tree.Root, Cat: -1, Text: q, Sort: "name"})
		t.Logf("buscar %-22q %7d resultados  %6.1f ms (por nombre %6.1f ms)", q, r.Total, r.Ms, r2.Ms)
	}
	if os.Getenv("TURBOCUT_DUPES") == "" {
		return
	}
	start := time.Now()
	res, err := a.findDupes(context.Background(), DupeOpts{Scope: a.tree.Root, MinSize: 10 << 20}, func(DupeProgress) {})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("duplicados (>=10 MB): %d grupos, %d copias, %.2f GB liberables, %d revisados, %d sin leer, %.1f s",
		len(res.Groups), res.Copies, float64(res.Wasted)/(1<<30), res.Checked, res.Skipped, time.Since(start).Seconds())
	for i, g := range res.Groups {
		if i >= 6 {
			break
		}
		t.Logf("   %8.1f MB x%d  %s / %s", float64(g.Size)/(1<<20), len(g.Files), g.Files[0].Path, g.Files[0].Name)
	}
}
