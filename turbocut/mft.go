package main

// Lectura directa de la MFT (Master File Table) de NTFS, como hace WizTree:
// en vez de abrir carpeta por carpeta se lee el índice del disco de un tirón
// y se reconstruye el árbol en memoria. Necesita permisos de administrador.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/sys/windows"
)

var errNotNTFS = errors.New("la unidad no es NTFS")

type run struct {
	vcn, lcn, n int64 // lcn < 0: hueco (sparse)
}

type mftVolume struct {
	r           io.ReaderAt
	closeFn     func()
	sector      int64
	cluster     int64
	recordSize  int64
	runs        []run
	records     int64
	initRecords int64
}

func openVolume(root string) (*mftVolume, error) {
	letter := strings.TrimRight(root, `\`)
	if len(letter) != 2 || letter[1] != ':' {
		return nil, fmt.Errorf("ruta de unidad no válida: %s", root)
	}
	p, _ := windows.UTF16PtrFromString(`\\.\` + letter)
	h, err := windows.CreateFile(p, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_SEQUENTIAL_SCAN, 0)
	if err != nil {
		return nil, err
	}
	v := &mftVolume{r: handleReader{h}, closeFn: func() { windows.CloseHandle(h) }}
	if err := v.init(); err != nil {
		v.close()
		return nil, err
	}
	return v, nil
}

func (v *mftVolume) close() {
	if v.closeFn != nil {
		v.closeFn()
	}
}

// handleReader lee de la unidad abierta en crudo (\.\C:).
type handleReader struct{ h windows.Handle }

func (r handleReader) ReadAt(buf []byte, off int64) (int, error) {
	var ov windows.Overlapped
	ov.Offset = uint32(off)
	ov.OffsetHigh = uint32(off >> 32)
	var n uint32
	err := windows.ReadFile(r.h, buf, &n, &ov)
	return int(n), err
}

func (v *mftVolume) readAt(buf []byte, off int64) error {
	n, err := v.r.ReadAt(buf, off)
	if err != nil && !(errors.Is(err, io.EOF) && n == len(buf)) {
		return err
	}
	if n != len(buf) {
		return fmt.Errorf("lectura corta: %d de %d", n, len(buf))
	}
	return nil
}

func (v *mftVolume) init() error {
	boot := make([]byte, 4096)
	if err := v.readAt(boot, 0); err != nil {
		return err
	}
	if string(boot[3:11]) != "NTFS    " {
		return errNotNTFS
	}
	v.sector = int64(le16(boot, 0x0B))
	spc := int64(boot[0x0D])
	if spc > 0x80 {
		spc = 1 << (256 - spc)
	}
	v.cluster = v.sector * spc
	mftLCN := int64(le64(boot, 0x30))
	cpr := int8(boot[0x40])
	if cpr < 0 {
		v.recordSize = 1 << uint(-cpr)
	} else {
		v.recordSize = int64(cpr) * v.cluster
	}
	if v.sector < 512 || v.cluster < v.sector || v.recordSize < 256 || v.recordSize > 65536 {
		return errors.New("sector de arranque NTFS extraño")
	}

	// El registro 0 es el propio $MFT: de ahí sacamos dónde está en el disco.
	rec := make([]byte, alignUp(v.recordSize, v.sector))
	if err := v.readAt(rec, mftLCN*v.cluster); err != nil {
		return err
	}
	rec = rec[:v.recordSize]
	if !fixup(rec) {
		return errors.New("registro $MFT dañado")
	}
	var dataSize, initSize int64
	found := false
	forAttrs(rec, func(typ uint32, a []byte) bool {
		if typ == 0x80 && a[9] == 0 && a[8] == 1 {
			dataSize = int64(le64(a, 0x30))
			initSize = int64(le64(a, 0x38))
			v.runs = decodeRuns(a[le16(a, 0x20):])
			found = true
			return false
		}
		return true
	})
	if !found || len(v.runs) == 0 {
		return errors.New("no se encontró el $DATA de la MFT")
	}
	var clusters int64
	for _, r := range v.runs {
		clusters += r.n
	}
	if clusters*v.cluster < dataSize {
		// La MFT está tan fragmentada que sus trozos siguen en otros registros.
		return errors.New("MFT muy fragmentada")
	}
	v.records = dataSize / v.recordSize
	v.initRecords = initSize / v.recordSize
	return nil
}

func alignUp(n, a int64) int64 { return (n + a - 1) / a * a }

// fixup aplica la "update sequence" que NTFS escribe al final de cada bloque de 512 bytes.
func fixup(rec []byte) bool {
	if len(rec) < 48 || string(rec[0:4]) != "FILE" {
		return false
	}
	usaOff := int(le16(rec, 4))
	usaCount := int(le16(rec, 6))
	if usaCount == 0 || usaOff+usaCount*2 > len(rec) || (usaCount-1)*512 > len(rec) {
		return false
	}
	usn := le16(rec, usaOff)
	for i := 1; i < usaCount; i++ {
		end := i*512 - 2
		if le16(rec, end) != usn {
			return false
		}
		rec[end] = rec[usaOff+i*2]
		rec[end+1] = rec[usaOff+i*2+1]
	}
	return true
}

// forAttrs recorre los atributos de un registro FILE ya corregido.
func forAttrs(rec []byte, fn func(typ uint32, a []byte) bool) {
	off := int(le16(rec, 0x14))
	used := int(le32(rec, 0x18))
	if used > len(rec) {
		used = len(rec)
	}
	for off+16 <= used {
		typ := le32(rec, off)
		if typ == 0xFFFFFFFF {
			return
		}
		ln := int(le32(rec, off+4))
		if ln < 16 || off+ln > used {
			return
		}
		if !fn(typ, rec[off:off+ln]) {
			return
		}
		off += ln
	}
}

// residentContent devuelve el contenido de un atributo residente.
func residentContent(a []byte) []byte {
	if a[8] != 0 || len(a) < 0x18 {
		return nil
	}
	ln := int(le32(a, 0x10))
	off := int(le16(a, 0x14))
	if off+ln > len(a) {
		return nil
	}
	return a[off : off+ln]
}

func decodeRuns(b []byte) []run {
	var runs []run
	var vcn, lcn int64
	for i := 0; i < len(b); {
		h := b[i]
		if h == 0 {
			break
		}
		ls, os := int(h&0x0F), int(h>>4)
		i++
		if ls == 0 || ls > 8 || os > 8 || i+ls+os > len(b) {
			break
		}
		var n int64
		for k := ls - 1; k >= 0; k-- {
			n = n<<8 | int64(b[i+k])
		}
		i += ls
		r := run{vcn: vcn, n: n, lcn: -1}
		if os > 0 {
			var d int64
			for k := os - 1; k >= 0; k-- {
				d = d<<8 | int64(b[i+k])
			}
			if b[i+os-1]&0x80 != 0 { // signo
				d -= 1 << (8 * uint(os))
			}
			lcn += d
			r.lcn = lcn
		}
		i += os
		runs = append(runs, r)
		vcn += n
	}
	return runs
}

// readVirtual lee bytes de la MFT a partir de un desplazamiento lógico.
func (v *mftVolume) readVirtual(buf []byte, voff int64) error {
	for len(buf) > 0 {
		vcn := voff / v.cluster
		var r *run
		for i := range v.runs {
			if vcn >= v.runs[i].vcn && vcn < v.runs[i].vcn+v.runs[i].n {
				r = &v.runs[i]
				break
			}
		}
		if r == nil {
			return errors.New("fuera de la MFT")
		}
		inRun := voff - r.vcn*v.cluster
		avail := r.n*v.cluster - inRun
		n := int64(len(buf))
		if n > avail {
			n = avail
		}
		if r.lcn < 0 {
			clear(buf[:n])
		} else if err := v.readAt(buf[:n], r.lcn*v.cluster+inRun); err != nil {
			return err
		}
		buf = buf[n:]
		voff += n
	}
	return nil
}

// Datos auxiliares por registro mientras se reconstruye el árbol.
type mftAux struct {
	pseq   uint16 // secuencia esperada del padre
	seq    uint16
	prio   uint8 // calidad del nombre elegido (0 = sin nombre)
	worker uint8
	parent uint32
}

type extItem struct {
	base    uint32
	parent  uint32
	pseq    uint16
	nameOff uint32
	nameLen uint16
	prio    uint8
	worker  uint8
	size    int64
	alloc   int64
	hasData bool
}

func namePrio(ns uint8) uint8 {
	switch ns {
	case 1, 3: // Win32, Win32+DOS
		return 3
	case 0: // POSIX
		return 2
	default: // solo DOS (8.3)
		return 1
	}
}

func appendUTF16(dst []byte, u []byte) []byte {
	for i := 0; i+1 < len(u); i += 2 {
		c := rune(le16(u, i))
		if c < 0x80 {
			dst = append(dst, byte(c))
			continue
		}
		if c >= 0xD800 && c < 0xDC00 && i+3 < len(u) {
			c2 := rune(le16(u, i+2))
			if c2 >= 0xDC00 && c2 < 0xE000 {
				c = (c-0xD800)<<10 + (c2 - 0xDC00) + 0x10000
				i += 2
			}
		}
		dst = utf8.AppendRune(dst, c)
	}
	return dst
}

// mftScan lee la MFT entera de la unidad y devuelve el árbol con raíz en root
// (que puede ser la unidad, "C:\", o una carpeta dentro de ella).
// safeMFTScan es mftScan, pero una MFT dañada que haga fallar la lectura se
// convierte en error (y se usa el modo normal) en vez de cerrar TurboCUT.
func safeMFTScan(ctx context.Context, root string, prog *Progress) (t *Tree, err error) {
	defer func() {
		if r := recover(); r != nil {
			t, err = nil, fmt.Errorf("la MFT tiene datos que no se entienden: %v", r)
		}
	}()
	return mftScan(ctx, root, prog)
}

func mftScan(ctx context.Context, root string, prog *Progress) (*Tree, error) {
	vol := volumeOf(root)
	v, err := openVolume(vol)
	if err != nil {
		return nil, err
	}
	defer v.close()
	return mftScanVolume(ctx, v, vol, root, prog)
}

func mftScanVolume(ctx context.Context, v *mftVolume, vol, root string, prog *Progress) (*Tree, error) {
	n := v.records
	prog.Total.Store(v.initRecords)
	t := newTree(vol, 0)
	t.Nodes = make([]Node, n)
	aux := make([]mftAux, n)

	workers := runtime.NumCPU()
	if workers > 32 {
		workers = 32
	}
	arenas := make([][]byte, workers)
	extsW := make([][]extItem, workers)

	type chunk struct {
		buf   []byte
		first int64
		count int64
	}
	const perChunk = 4096
	chunkBytes := alignUp(perChunk*v.recordSize, v.sector)
	pool := make(chan []byte, workers+2)
	for i := 0; i < workers+2; i++ {
		pool <- make([]byte, chunkBytes)
	}
	work := make(chan chunk, workers)
	var readErr error

	go func() {
		defer close(work)
		for first := int64(0); first < v.initRecords; first += perChunk {
			if ctx.Err() != nil {
				return
			}
			cnt := int64(perChunk)
			if first+cnt > v.initRecords {
				cnt = v.initRecords - first
			}
			buf := <-pool
			if err := v.readVirtual(buf[:alignUp(cnt*v.recordSize, v.sector)], first*v.recordSize); err != nil {
				readErr = err
				return
			}
			work <- chunk{buf, first, cnt}
		}
	}()

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			arena := make([]byte, 0, 1<<20)
			var exts []extItem
			for c := range work {
				for i := int64(0); i < c.count; i++ {
					r := c.first + i
					rec := c.buf[i*v.recordSize : (i+1)*v.recordSize]
					arena, exts = parseRecord(rec, uint32(r), uint8(w), &t.Nodes[r], &aux[r], arena, exts)
				}
				prog.Done.Add(c.count)
				pool <- c.buf
			}
			arenas[w] = arena
			extsW[w] = exts
		}(w)
	}
	wg.Wait()
	if readErr != nil {
		return nil, readErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Une los nombres de todos los hilos en un solo almacén.
	bases := make([]uint32, workers)
	total := 0
	for w, a := range arenas {
		bases[w] = uint32(total)
		total += len(a)
	}
	t.names = make([]byte, 0, total)
	for _, a := range arenas {
		t.names = append(t.names, a...)
	}
	for r := range t.Nodes {
		if aux[r].prio > 0 {
			t.Nodes[r].NameOff += bases[aux[r].worker]
		}
	}

	// Registros de extensión: atributos que no cabían en el registro base.
	for _, exts := range extsW {
		for _, e := range exts {
			if int64(e.base) >= n {
				continue
			}
			nd, ax := &t.Nodes[e.base], &aux[e.base]
			if e.prio > ax.prio {
				ax.prio, ax.parent, ax.pseq = e.prio, e.parent, e.pseq
				nd.NameOff, nd.NameLen = e.nameOff+bases[e.worker], e.nameLen
			}
			if e.hasData {
				if e.size > nd.Size {
					nd.Size = e.size
				}
				if e.alloc > nd.Alloc {
					nd.Alloc = e.alloc
				}
			}
		}
	}

	// Enlaza cada registro con su carpeta padre.
	const rootRec = 5
	for r := range t.Nodes {
		// Parent también: un registro que no se enlaza (padre borrado, metadatos de
		// NTFS…) se quedaría con Parent = 0, y el registro 0 apunta a sí mismo.
		t.Nodes[r].First, t.Nodes[r].Next, t.Nodes[r].Parent = none, none, none
	}
	t.Nodes[rootRec].Parent = none
	var files, bytes int64
	for r := int64(16); r < n; r++ {
		nd, ax := &t.Nodes[r], &aux[r]
		if ax.prio == 0 || nd.Flags&fDeleted != 0 {
			continue
		}
		p := int64(ax.parent)
		if p == r || p >= n || (p < 16 && p != rootRec) || !t.Nodes[p].dir() || aux[p].prio == 0 && p != rootRec {
			continue
		}
		if ax.pseq != 0 && aux[p].seq != ax.pseq {
			continue // el padre se borró y su registro se reutilizó
		}
		if nd.dir() {
			nd.Size, nd.Alloc = 0, 0
		} else {
			nd.Ext = t.extID(t.name(int32(r)))
			files++
			bytes += nd.Size
		}
		t.link(int32(r), int32(p))
	}
	prog.Files.Store(files)
	prog.Bytes.Store(bytes)

	t.Root = rootRec
	t.Nodes[rootRec].Flags = fDir
	if rel := strings.TrimPrefix(root, vol); rel != "" {
		id := t.findPath(rel)
		if id == none || !t.Nodes[id].dir() {
			return nil, fmt.Errorf("no se encontró %s en la MFT", root)
		}
		t.Root = id
		t.Nodes[id].Parent = none
		t.Nodes[id].Next = none
	}
	t.RootPath = root
	t.aggregate(t.Root)
	return t, nil
}

// parseRecord lee un registro FILE y rellena su nodo (o apunta lo que aporte a su registro base).
func parseRecord(rec []byte, r uint32, w uint8, nd *Node, ax *mftAux, arena []byte, exts []extItem) ([]byte, []extItem) {
	nd.Flags = fDeleted // por defecto: registro libre
	if !fixup(rec) {
		return arena, exts
	}
	flags := le16(rec, 0x16)
	if flags&1 == 0 {
		return arena, exts
	}
	base := uint32(le64(rec, 0x20) & 0xFFFFFFFFFFFF)
	ext := extItem{base: base, worker: w}

	var size, alloc int64
	var hasData bool
	var mtime uint32
	var fattr uint32
	var prio uint8
	var parent uint32
	var pseq uint16
	var nameOff uint32
	var nameLen uint16
	var reparseTag uint32
	var hasTag bool

	forAttrs(rec, func(typ uint32, a []byte) bool {
		switch typ {
		case 0x10: // $STANDARD_INFORMATION
			if c := residentContent(a); len(c) >= 0x24 {
				mtime = filetimeToUnix(int64(le64(c, 0x08)))
				fattr = le32(c, 0x20)
			}
		case 0x30: // $FILE_NAME
			c := residentContent(a)
			if len(c) < 0x42 {
				break
			}
			nl := int(c[0x40])
			if 0x42+nl*2 > len(c) {
				break
			}
			p := namePrio(c[0x41])
			if p <= prio {
				break
			}
			prio = p
			ref := le64(c, 0)
			parent = uint32(ref & 0xFFFFFFFFFFFF)
			pseq = uint16(ref >> 48)
			off := len(arena)
			arena = appendUTF16(arena, c[0x42:0x42+nl*2])
			nameOff = uint32(off)
			nameLen = uint16(len(arena) - off)
		case 0xC0: // $REPARSE_POINT: los 4 primeros bytes dicen de qué tipo es
			if c := residentContent(a); len(c) >= 4 {
				reparseTag, hasTag = le32(c, 0), true
			}
		case 0x80: // $DATA sin nombre (el contenido del archivo)
			if a[9] != 0 {
				break
			}
			if a[8] == 0 {
				size = int64(le32(a, 0x10))
				hasData = true
			} else if len(a) >= 0x40 && le64(a, 0x10) == 0 {
				alloc = int64(le64(a, 0x28))
				size = int64(le64(a, 0x30))
				// Comprimidos o con huecos: el tamaño real en disco va aparte.
				if le16(a, 0x0C)&0x8001 != 0 && len(a) >= 0x48 && le16(a, 0x20) >= 0x48 {
					alloc = int64(le64(a, 0x40))
				}
				hasData = true
			}
		}
		return true
	})

	if base != 0 && base != r {
		if prio > 0 || hasData {
			ext.parent, ext.pseq, ext.prio = parent, pseq, prio
			ext.nameOff, ext.nameLen = nameOff, nameLen
			ext.size, ext.alloc, ext.hasData = size, alloc, hasData
			exts = append(exts, ext)
		}
		return arena, exts
	}

	nd.Flags = 0
	if flags&2 != 0 {
		nd.Flags |= fDir
	}
	if fattr&windows.FILE_ATTRIBUTE_HIDDEN != 0 {
		nd.Flags |= fHidden
	}
	if fattr&windows.FILE_ATTRIBUTE_SYSTEM != 0 {
		nd.Flags |= fSystem
	}
	// Enlace solo si es una unión o un enlace simbólico (como en el modo normal). Las
	// carpetas de OneDrive y otras con punto de reanálisis son carpetas de verdad, con
	// sus archivos dentro: antes no salían en Buscar, Duplicados ni Limpieza.
	if fattr&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 && flags&2 != 0 &&
		(!hasTag || reparseTag == reparseMountPoint || reparseTag == reparseSymlink) {
		nd.Flags |= fLink
	}
	if fattr&(attrRecallOnData|attrRecallOnOpen|windows.FILE_ATTRIBUTE_OFFLINE) != 0 {
		nd.Flags |= fCloud
	}
	nd.Size, nd.Alloc, nd.MTime = size, alloc, mtime
	nd.NameOff, nd.NameLen = nameOff, nameLen
	ax.seq = le16(rec, 0x10)
	ax.prio, ax.parent, ax.pseq, ax.worker = prio, parent, pseq, w
	return arena, exts
}
