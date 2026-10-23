package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
	"unicode/utf16"
)

// ---------- disco NTFS sintético para probar el lector de la MFT ----------

type recBuilder struct {
	b   []byte
	off int
}

func newRec(seq, flags uint16, base uint64) *recBuilder {
	b := make([]byte, 1024)
	copy(b, "FILE")
	binary.LittleEndian.PutUint16(b[4:], 0x30) // USA
	binary.LittleEndian.PutUint16(b[6:], 3)
	binary.LittleEndian.PutUint16(b[0x10:], seq)
	binary.LittleEndian.PutUint16(b[0x14:], 0x38)
	binary.LittleEndian.PutUint16(b[0x16:], flags)
	binary.LittleEndian.PutUint32(b[0x1C:], 1024)
	binary.LittleEndian.PutUint64(b[0x20:], base)
	return &recBuilder{b: b, off: 0x38}
}

func (r *recBuilder) resident(typ uint32, content []byte) {
	ln := (0x18 + len(content) + 7) &^ 7
	a := r.b[r.off:]
	binary.LittleEndian.PutUint32(a[0:], typ)
	binary.LittleEndian.PutUint32(a[4:], uint32(ln))
	binary.LittleEndian.PutUint32(a[0x10:], uint32(len(content)))
	binary.LittleEndian.PutUint16(a[0x14:], 0x18)
	copy(a[0x18:], content)
	r.off += ln
}

func (r *recBuilder) si(mtime time.Time, attr uint32) {
	c := make([]byte, 0x48)
	ft := uint64(mtime.Unix()+11644473600) * 10000000
	binary.LittleEndian.PutUint64(c[0x08:], ft)
	binary.LittleEndian.PutUint32(c[0x20:], attr)
	r.resident(0x10, c)
}

func (r *recBuilder) fn(parent uint64, pseq uint16, name string, ns uint8) {
	u := utf16.Encode([]rune(name))
	c := make([]byte, 0x42+len(u)*2)
	binary.LittleEndian.PutUint64(c[0:], parent|uint64(pseq)<<48)
	c[0x40] = byte(len(u))
	c[0x41] = ns
	for i, x := range u {
		binary.LittleEndian.PutUint16(c[0x42+i*2:], x)
	}
	r.resident(0x30, c)
}

func (r *recBuilder) dataResident(size int) { r.resident(0x80, make([]byte, size)) }

func (r *recBuilder) dataNonResident(size, alloc int64, runs []byte) {
	ln := (0x40 + len(runs) + 1 + 7) &^ 7
	a := r.b[r.off:]
	binary.LittleEndian.PutUint32(a[0:], 0x80)
	binary.LittleEndian.PutUint32(a[4:], uint32(ln))
	a[8] = 1
	binary.LittleEndian.PutUint16(a[0x20:], 0x40)
	binary.LittleEndian.PutUint64(a[0x28:], uint64(alloc))
	binary.LittleEndian.PutUint64(a[0x30:], uint64(size))
	binary.LittleEndian.PutUint64(a[0x38:], uint64(size))
	copy(a[0x40:], runs)
	r.off += ln
}

func (r *recBuilder) done() []byte {
	binary.LittleEndian.PutUint32(r.b[r.off:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(r.b[0x18:], uint32(r.off+8))
	// update sequence: el final de cada bloque de 512 se guarda en la USA
	const usn = 0x0042
	binary.LittleEndian.PutUint16(r.b[0x30:], usn)
	for i := 1; i <= 2; i++ {
		end := i*512 - 2
		copy(r.b[0x30+i*2:], r.b[end:end+2])
		binary.LittleEndian.PutUint16(r.b[end:], usn)
	}
	return r.b
}

// buildImage crea un volumen con una MFT de 64 registros partida en dos trozos.
func buildImage(t *testing.T) []byte {
	const cluster = 4096
	img := make([]byte, 28*cluster)
	copy(img[3:], "NTFS    ")
	binary.LittleEndian.PutUint16(img[0x0B:], 512)
	img[0x0D] = 8
	binary.LittleEndian.PutUint64(img[0x30:], 4)
	img[0x40] = 0xF6 // registros de 1024 bytes

	put := func(r int, rec []byte) {
		var phys int
		if r < 32 {
			phys = 4*cluster + r*1024
		} else {
			phys = 20*cluster + (r-32)*1024
		}
		copy(img[phys:], rec)
	}
	now := time.Now()
	old := now.Add(-90 * 24 * time.Hour)

	r0 := newRec(1, 1, 0)
	r0.fn(5, 5, "$MFT", 3)
	// 8 clústeres en el LCN 4 y otros 8 en el LCN 20 (+16)
	r0.dataNonResident(64*1024, 64*1024, []byte{0x11, 8, 4, 0x11, 8, 16, 0})
	put(0, r0.done())

	root := newRec(5, 3, 0)
	root.si(now, 0)
	root.fn(5, 5, ".", 3)
	put(5, root.done())

	users := newRec(1, 3, 0)
	users.si(now, 0)
	users.fn(5, 5, "Users", 3)
	put(16, users.done())

	fer := newRec(1, 3, 0)
	fer.si(now, 0)
	fer.fn(16, 1, "Fernando", 3)
	put(17, fer.done())

	a := newRec(1, 1, 0)
	a.si(old, 0)
	a.fn(17, 1, "Vídeo.MKV", 1)
	a.dataNonResident(5000, 8192, []byte{0x11, 2, 0x30, 0})
	put(18, a.done())

	b := newRec(1, 1, 0)
	b.si(now, 2) // oculto
	b.fn(17, 1, "b.txt", 3)
	b.dataResident(100)
	put(19, b.done())

	big := newRec(1, 1, 0)
	big.si(now, 0)
	big.fn(16, 1, "big.iso", 3)
	put(20, big.done())
	bigExt := newRec(1, 1, 20) // registro de extensión con los datos
	bigExt.dataNonResident(10<<20, 10<<20, []byte{0x21, 0x00, 0x0A, 0x40, 0})
	put(21, bigExt.done())

	dos := newRec(1, 1, 0)
	dos.si(now, 0)
	dos.fn(5, 5, "BIGFIL~1.ISO", 2)
	dos.fn(5, 5, "Big File.iso", 1)
	dos.dataNonResident(1<<20, 1<<20, []byte{0x11, 1, 0x50, 0})
	put(22, dos.done())

	orphan := newRec(1, 1, 0)
	orphan.fn(30, 1, "huerfano.txt", 3)
	orphan.dataResident(10)
	put(23, orphan.done())

	stale := newRec(1, 1, 0)
	stale.fn(16, 9, "padre-reciclado.txt", 3)
	stale.dataResident(10)
	put(24, stale.done())

	unused := newRec(1, 0, 0)
	unused.fn(5, 5, "borrado.txt", 3)
	unused.dataResident(10)
	put(25, unused.done())

	c := newRec(1, 1, 0)
	c.si(now, 0)
	c.fn(17, 1, "c.dat", 3)
	c.dataNonResident(4096, 4096, []byte{0x11, 1, 0x60, 0})
	put(40, c.done()) // en el segundo trozo de la MFT

	return img
}

func memVolume(t *testing.T) *mftVolume {
	v := &mftVolume{r: bytes.NewReader(buildImage(t))}
	if err := v.init(); err != nil {
		t.Fatal(err)
	}
	return v
}

func childNames(tr *Tree, id int32) []string {
	var out []string
	tr.children(id, func(c int32) { out = append(out, tr.name(c)) })
	sort.Strings(out)
	return out
}

func TestMFTSynthetic(t *testing.T) {
	v := memVolume(t)
	if v.records != 64 || v.recordSize != 1024 || v.cluster != 4096 || len(v.runs) != 2 {
		t.Fatalf("volumen mal leído: %+v", v)
	}
	tr, err := mftScanVolume(context.Background(), v, `C:\`, `C:\`, &Progress{})
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(childNames(tr, tr.Root)); got != "[Big File.iso Users]" {
		t.Fatalf("raíz: %s", got)
	}
	users := tr.findPath("users")
	if got := fmt.Sprint(childNames(tr, users)); got != "[Fernando big.iso]" {
		t.Fatalf("Users: %s", got)
	}
	fer := tr.findPath(`Users\Fernando`)
	if got := fmt.Sprint(childNames(tr, fer)); got != "[Vídeo.MKV b.txt c.dat]" {
		t.Fatalf("Fernando: %s", got)
	}
	vid := tr.findPath(`users\fernando\vídeo.mkv`)
	if n := tr.Nodes[vid]; n.Size != 5000 || n.Alloc != 8192 || tr.ext(vid) != "mkv" {
		t.Fatalf("vídeo: %+v ext=%q", n, tr.ext(vid))
	}
	if n := tr.Nodes[tr.findPath(`users\fernando\b.txt`)]; n.Size != 100 || n.Flags&fHidden == 0 {
		t.Fatalf("b.txt: %+v", n)
	}
	if n := tr.Nodes[tr.findPath(`users\big.iso`)]; n.Size != 10<<20 {
		t.Fatalf("big.iso (datos en extensión): %+v", n)
	}
	wantSize := int64(5000 + 100 + 4096 + 10<<20 + 1<<20)
	if r := tr.Nodes[tr.Root]; r.Size != wantSize || r.Files != 5 {
		t.Fatalf("total %d (quería %d), archivos %d", r.Size, wantSize, r.Files)
	}
	if p := tr.pathOf(vid); p != `C:\Users\Fernando\Vídeo.MKV` {
		t.Fatalf("ruta: %s", p)
	}

	// Raíz en una subcarpeta.
	v2 := memVolume(t)
	sub, err := mftScanVolume(context.Background(), v2, `C:\`, `C:\Users\Fernando`, &Progress{})
	if err != nil {
		t.Fatal(err)
	}
	if r := sub.Nodes[sub.Root]; r.Size != 5000+100+4096 || r.Files != 3 {
		t.Fatalf("subcarpeta: %+v", r)
	}
	if p := sub.pathOf(sub.findPath("c.dat")); p != `C:\Users\Fernando\c.dat` {
		t.Fatalf("ruta en subcarpeta: %s", p)
	}
}

func TestDecodeRuns(t *testing.T) {
	runs := decodeRuns([]byte{0x21, 0x10, 0x00, 0x01, 0x11, 0x08, 0xF0, 0x01, 0x04, 0})
	if len(runs) != 3 {
		t.Fatalf("runs: %+v", runs)
	}
	if runs[0] != (run{0, 256, 16}) || runs[1] != (run{16, 240, 8}) || runs[2] != (run{24, -1, 4}) {
		t.Fatalf("runs: %+v", runs)
	}
}

// ---------- recorrido de carpetas ----------

func writeFile(t *testing.T, p string, n int) {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, make([]byte, n), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWalk(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), 1000)
	writeFile(t, filepath.Join(dir, "sub", "b.mp4"), 3000)
	writeFile(t, filepath.Join(dir, "sub", "deep", "c.ZIP"), 5000)
	writeFile(t, filepath.Join(dir, "sub", "deep", "ñandú café.bin"), 7)
	os.MkdirAll(filepath.Join(dir, "vacía"), 0o755)

	tr, err := walkScan(context.Background(), dir, &Progress{})
	if err != nil {
		t.Fatal(err)
	}
	r := tr.Nodes[tr.Root]
	if r.Size != 9007 || r.Files != 4 {
		t.Fatalf("raíz: %+v", r)
	}
	deep := tr.findPath(`sub\deep`)
	if tr.Nodes[deep].Size != 5007 {
		t.Fatalf("deep: %+v", tr.Nodes[deep])
	}
	c := tr.findPath(`sub\deep\c.zip`)
	if tr.ext(c) != "zip" || categoryOf(tr.ext(c)) != 5 {
		t.Fatalf("extensión: %q", tr.ext(c))
	}
	if p := tr.pathOf(tr.findPath(`sub\deep\ñandú café.bin`)); p != filepath.Join(dir, "sub", "deep", "ñandú café.bin") {
		t.Fatalf("ruta: %s", p)
	}

	// Quitar un archivo descuenta su tamaño de todas las carpetas de arriba.
	tr.remove(c)
	if tr.Nodes[tr.Root].Size != 4007 || tr.Nodes[tr.Root].Files != 3 || tr.Nodes[deep].Size != 7 {
		t.Fatalf("tras quitar: raíz %+v deep %+v", tr.Nodes[tr.Root], tr.Nodes[deep])
	}

	// Releer una carpeta tras añadir un archivo.
	writeFile(t, filepath.Join(dir, "sub", "nuevo.dat"), 2000)
	sub := tr.findPath("sub")
	nid := refreshSubtree(tr, sub)
	if tr.Nodes[nid].Size != 3000+5007+2000 || tr.Nodes[tr.Root].Size != 1000+3000+5007+2000 || tr.Nodes[tr.Root].Files != 5 {
		t.Fatalf("tras releer: sub %+v raíz %+v", tr.Nodes[nid], tr.Nodes[tr.Root])
	}
	if tr.alive(sub) || !tr.alive(nid) || tr.findPath("sub") != nid {
		t.Fatal("el nodo viejo debería estar muerto y el nuevo enlazado")
	}
}

func TestMatchPath(t *testing.T) {
	if !matchPath(`c:\users\x\appdata\local\google\chrome\user data\default\cache`, `c:\users\x\appdata\local\google\chrome\user data\*\cache`) {
		t.Fatal("debería encajar")
	}
	if matchPath(`c:\users\x\appdata\local\temp\sub`, `c:\users\x\appdata\local\temp`) {
		t.Fatal("no debería encajar")
	}
}

func TestCleanupFindsNodeModules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "proyecto", "node_modules", "lib", "index.js"), 2<<20)
	writeFile(t, filepath.Join(dir, "proyecto", "src", "app.js"), 10)
	writeFile(t, filepath.Join(dir, "juego", "target", "CACHEDIR.TAG"), 10)
	writeFile(t, filepath.Join(dir, "juego", "target", "debug", "juego.exe"), 3<<20)
	writeFile(t, filepath.Join(dir, "otro", "target", "no-es-cargo.txt"), 3<<20)
	old := time.Now().Add(-60 * 24 * time.Hour)
	for _, p := range []string{"proyecto/node_modules/lib/index.js", "proyecto/node_modules/lib", "proyecto/node_modules",
		"juego/target/CACHEDIR.TAG", "juego/target/debug/juego.exe", "juego/target/debug", "juego/target"} {
		os.Chtimes(filepath.Join(dir, p), old, old)
	}
	tr, err := walkScan(context.Background(), dir, &Progress{})
	if err != nil {
		t.Fatal(err)
	}
	var node, rust *Candidate
	for _, c := range findCandidates(tr, func(n *Node) int64 { return n.Size }) {
		if c.ID == "node" {
			node = c
		}
		if c.ID == "rust" {
			rust = c
		}
	}
	if rust == nil || rust.Size != 3<<20+10 || len(rust.targets) != 1 {
		t.Fatalf("target de Rust: %+v", rust)
	}
	if node == nil || node.Size != 2<<20 || len(node.targets) != 1 {
		t.Fatalf("node_modules: %+v", node)
	}
}

// Escaneo real, solo si se pide: TURBOCUT_SCAN=C:\ go test -run Real -v
func TestRealScan(t *testing.T) {
	root := os.Getenv("TURBOCUT_SCAN")
	if root == "" {
		t.Skip("pon TURBOCUT_SCAN para escanear de verdad")
	}
	root = cleanRoot(root)
	for _, mode := range []string{"walk", "mft"} {
		if mode == "mft" && !canUseMFT(root) {
			t.Log("MFT: hace falta administrador y NTFS")
			continue
		}
		prog := &Progress{}
		start := time.Now()
		var tr *Tree
		var err error
		if mode == "mft" {
			tr, err = mftScan(context.Background(), root, prog)
		} else {
			tr, err = walkScan(context.Background(), root, prog)
		}
		if err != nil {
			t.Fatal(mode, err)
		}
		r := tr.Nodes[tr.Root]
		t.Logf("%s: %.2fs · %d archivos · %.2f GB (%.2f GB en disco) · %d sin acceso · %d nodos",
			mode, time.Since(start).Seconds(), r.Files, float64(r.Size)/(1<<30), float64(r.Alloc)/(1<<30), prog.Denied.Load(), len(tr.Nodes))
		var kids []int32
		tr.children(tr.Root, func(c int32) { kids = append(kids, c) })
		sort.Slice(kids, func(i, j int) bool { return tr.Nodes[kids[i]].Alloc > tr.Nodes[kids[j]].Alloc })
		for i, c := range kids {
			if i >= 8 {
				break
			}
			t.Logf("   %-40s %8.2f GB", tr.name(c), float64(tr.Nodes[c].Alloc)/(1<<30))
		}
		start = time.Now()
		cands := findCandidates(tr, func(n *Node) int64 { return n.Alloc })
		t.Logf("   limpieza (%.0f ms):", time.Since(start).Seconds()*1000)
		for _, c := range cands {
			t.Logf("     [%s] %-45s %8.2f GB  %d", c.Level, c.Title, float64(c.Size)/(1<<30), c.Count)
		}
	}
}

func TestPruneSnapshots(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 35; i++ {
		p := filepath.Join(dir, fmt.Sprintf("%02d.json", i))
		writeFile(t, p, 10)
		ts := time.Now().Add(time.Duration(i) * time.Minute)
		os.Chtimes(p, ts, ts)
	}
	pruneSnapshots(dir, 30)
	ents, _ := os.ReadDir(dir)
	if len(ents) != 30 {
		t.Fatalf("quedan %d", len(ents))
	}
	if _, err := os.Stat(filepath.Join(dir, "00.json")); err == nil {
		t.Fatal("debería haber borrado el más antiguo")
	}
	if _, err := os.Stat(filepath.Join(dir, "34.json")); err != nil {
		t.Fatal("no debería borrar el más nuevo")
	}
}

func testApp(t *testing.T, dir string) *App {
	tr, err := walkScan(context.Background(), dir, &Progress{})
	if err != nil {
		t.Fatal(err)
	}
	s := defaultSettings()
	s.Measure = "size"
	return &App{settings: s, tree: tr}
}

func TestFind(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Fotos", "Vacaciones Málaga.JPG"), 3000)
	writeFile(t, filepath.Join(dir, "Fotos", "vacaciones-2.png"), 100)
	writeFile(t, filepath.Join(dir, "Vídeos", "Vacaciones.mp4"), 9000)
	writeFile(t, filepath.Join(dir, "notas.txt"), 10)
	a := testApp(t, dir)
	names := func(q FindQuery) []string {
		r, err := a.Find(q)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, it := range r.Items {
			out = append(out, it.Name)
		}
		return out
	}
	all := FindQuery{Scope: a.tree.Root, Cat: -1}
	q := all
	q.Text = "VACACIONES"
	if got := fmt.Sprint(names(q)); got != "[Vacaciones.mp4 Vacaciones Málaga.JPG vacaciones-2.png]" {
		t.Fatalf("por tamaño: %s", got)
	}
	q.Text = "vacaciones málaga" // dos palabras y una tilde en mayúsculas/minúsculas
	if got := fmt.Sprint(names(q)); got != "[Vacaciones Málaga.JPG]" {
		t.Fatalf("dos palabras: %s", got)
	}
	q.Text = `"vacaciones m"`
	if got := fmt.Sprint(names(q)); got != "[Vacaciones Málaga.JPG]" {
		t.Fatalf("comillas: %s", got)
	}
	q = all
	q.Text = "vacaciones .mp4"
	if got := fmt.Sprint(names(q)); got != "[Vacaciones.mp4]" {
		t.Fatalf("extensión: %s", got)
	}
	q = all
	q.Cat, q.MinSize = 2, 1000 // imágenes de 1000 bytes o más
	if got := fmt.Sprint(names(q)); got != "[Vacaciones Málaga.JPG]" {
		t.Fatalf("tipo y tamaño: %s", got)
	}
	q = all
	q.Kind, q.Text = "dirs", "o"
	if got := fmt.Sprint(names(q)); got != "[Vídeos Fotos]" {
		t.Fatalf("carpetas: %s", got)
	}
	q = all
	q.Sort = "name"
	if got := fmt.Sprint(names(q)); got != "[Fotos notas.txt Vacaciones Málaga.JPG vacaciones-2.png Vacaciones.mp4 Vídeos]" {
		t.Fatalf("por nombre: %s", got)
	}
}

func TestDupes(t *testing.T) {
	dir := t.TempDir()
	content := make([]byte, 300<<10)
	for i := range content {
		content[i] = byte(i * 7)
	}
	put := func(rel string, b []byte) {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put("a/foto.raw", content)
	put("b/copia de foto.raw", content)
	put("c/otra copia.raw", content)
	casi := append([]byte(nil), content...)
	casi[150<<10] ^= 1 // mismo tamaño, principio y final iguales, distinto por dentro
	put("d/casi igual.raw", casi)
	if err := os.Link(filepath.Join(dir, "a", "foto.raw"), filepath.Join(dir, "a", "enlace duro.raw")); err != nil {
		t.Fatal(err)
	}
	put("e/pequeño1.txt", []byte("hola"))
	put("e/pequeño2.txt", []byte("hola"))
	put("node_modules/x/foto.raw", content) // se salta por defecto

	a := testApp(t, dir)
	res, err := a.findDupes(context.Background(), DupeOpts{Scope: a.tree.Root, MinSize: 1 << 10}, func(DupeProgress) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Groups) != 1 {
		t.Fatalf("grupos: %+v", res.Groups)
	}
	g := res.Groups[0]
	var got []string
	for _, f := range g.Files {
		got = append(got, f.Name)
	}
	sort.Strings(got)
	// el enlace duro y foto.raw son el mismo archivo: solo cuenta uno de los dos
	if len(got) != 3 || got[0] != "copia de foto.raw" || got[2] != "otra copia.raw" {
		t.Fatalf("archivos: %v", got)
	}
	if g.Wasted != 2*int64(len(content)) || res.Wasted != g.Wasted {
		t.Fatalf("liberable: %d", g.Wasted)
	}
}
