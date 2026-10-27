package main

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Progress lo lee la interfaz mientras se escanea.
type Progress struct {
	Files  atomic.Int64
	Dirs   atomic.Int64
	Bytes  atomic.Int64
	Denied atomic.Int64
	Done   atomic.Int64 // para la MFT: registros leídos
	Total  atomic.Int64 // para la MFT: registros totales
	cur    atomic.Pointer[string]
}

func (p *Progress) setCur(s string) { p.cur.Store(&s) }
func (p *Progress) Cur() string {
	if s := p.cur.Load(); s != nil {
		return *s
	}
	return ""
}

const (
	fileFullDirectoryInfo = 14
	fileIDExtdDirInfo     = 19 // como el anterior, más el identificador del archivo
	reparseMountPoint     = 0xA0000003
	reparseSymlink        = 0xA000000C
	attrRecallOnData      = 0x00400000
	attrRecallOnOpen      = 0x00040000
)

type rawEntry struct {
	name  string
	attr  uint32
	tag   uint32
	size  int64
	alloc int64
	mtime uint32
	fid   uint64 // identificador del archivo en su volumen (0 = no se sabe)
}

// longPath añade el prefijo \\?\ para rutas de más de 260 caracteres.
func longPath(p string) string {
	if strings.HasPrefix(p, `\\?\`) {
		return p
	}
	if strings.HasPrefix(p, `\\`) {
		return `\\?\UNC\` + p[2:]
	}
	return `\\?\` + p
}

func joinPath(dir, name string) string {
	if strings.HasSuffix(dir, `\`) {
		return dir + name
	}
	return dir + `\` + name
}

func filetimeToUnix(ft int64) uint32 {
	if ft <= 116444736000000000 {
		return 0
	}
	s := ft/10000000 - 11644473600
	if s > 0xFFFFFFFF {
		return 0xFFFFFFFF
	}
	return uint32(s)
}

// readDir lee una carpeta entera con GetFileInformationByHandleEx, que trae
// tamaños y fechas de golpe en un búfer grande (mucho más rápido que FindNextFile).
// Con withID pide también el identificador de cada archivo (para los enlaces
// duros); si el sistema de archivos no lo da, *withID pasa a false.
func readDir(path string, buf []byte, out []rawEntry, withID *atomic.Bool) ([]rawEntry, error) {
	p, err := windows.UTF16PtrFromString(longPath(path))
	if err != nil {
		return out, err
	}
	h, err := windows.CreateFile(p, windows.FILE_LIST_DIRECTORY,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return out, err
	}
	defer windows.CloseHandle(h)
	ids := withID != nil && withID.Load()
	first := true
	for {
		class := uint32(fileFullDirectoryInfo)
		if ids {
			class = fileIDExtdDirInfo
		}
		err := windows.GetFileInformationByHandleEx(h, class, &buf[0], uint32(len(buf)))
		if err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				return out, nil
			}
			// Si el sistema de archivos no da identificadores (FAT, algunas unidades de
			// red o virtuales), cada uno contesta con un error distinto: cualquiera que no
			// sea «no hay más» o «sin permiso» en la primera lectura vale para no usarlos.
			if ids && first && !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
				withID.Store(false) // FAT, algunas unidades de red…: sin identificadores
				ids = false
				continue
			}
			return out, err
		}
		first = false
		off := 0
		for {
			e := buf[off:]
			next := le32(e, 0)
			nameLen := int(le32(e, 60))
			// FILE_FULL_DIR_INFO: etiqueta de reparación en 64 y nombre en 68.
			// FILE_ID_EXTD_DIR_INFO: etiqueta en 68, identificador (128 bits) en 72 y nombre en 88.
			tag, nameAt := le32(e, 64), 68
			var fid uint64
			if ids {
				tag, nameAt = le32(e, 68), 88
				lo, hi := le64(e, 72), le64(e, 80)
				fid = lo
				if hi != 0 { // ReFS: identificadores de 128 bits
					fid = hi*0x9E3779B97F4A7C15 ^ lo
				}
			}
			u := unsafe.Slice((*uint16)(unsafe.Pointer(&e[nameAt])), nameLen/2)
			if !(len(u) == 1 && u[0] == '.') && !(len(u) == 2 && u[0] == '.' && u[1] == '.') {
				out = append(out, rawEntry{
					name:  utf16ToString(u),
					attr:  le32(e, 56),
					tag:   tag,
					size:  int64(le64(e, 40)),
					alloc: int64(le64(e, 48)),
					mtime: filetimeToUnix(int64(le64(e, 24))),
					fid:   fid,
				})
			}
			if next == 0 {
				break
			}
			off += int(next)
		}
	}
}

func utf16ToString(u []uint16) string {
	ascii := true
	for _, c := range u {
		if c >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		b := make([]byte, len(u))
		for i, c := range u {
			b[i] = byte(c)
		}
		return unsafe.String(unsafe.SliceData(b), len(b))
	}
	return string(utf16.Decode(u))
}

func le16(b []byte, o int) uint16 { return uint16(b[o]) | uint16(b[o+1])<<8 }
func le32(b []byte, o int) uint32 {
	return uint32(b[o]) | uint32(b[o+1])<<8 | uint32(b[o+2])<<16 | uint32(b[o+3])<<24
}
func le64(b []byte, o int) uint64 { return uint64(le32(b, o)) | uint64(le32(b, o+4))<<32 }

// walkInto rellena el subárbol de dirID (una carpeta ya creada y sin hijos)
// leyendo carpetas en paralelo.
func walkInto(ctx context.Context, t *Tree, dirID int32, dirPath string, prog *Progress) error {
	type job struct {
		id   int32
		path string
	}
	var (
		qmu     sync.Mutex
		cond    = sync.NewCond(&qmu)
		queue   = []job{{dirID, dirPath}}
		active  = 0
		stopped = false
	)
	workers := runtime.NumCPU() * 2
	if workers < 8 {
		workers = 8
	}
	if workers > 48 {
		workers = 48
	}

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 128*1024)
			var ents []rawEntry
			var subs []job
			for {
				qmu.Lock()
				for len(queue) == 0 && active > 0 && !stopped {
					cond.Wait()
				}
				if stopped || len(queue) == 0 {
					stopped = true
					cond.Broadcast()
					qmu.Unlock()
					return
				}
				j := queue[len(queue)-1]
				queue = queue[:len(queue)-1]
				active++
				qmu.Unlock()

				if ctx.Err() != nil {
					qmu.Lock()
					stopped = true
					cond.Broadcast()
					qmu.Unlock()
					return
				}

				var err error
				var withID *atomic.Bool
				if t.trackLinks && !t.noFileIDs.Load() {
					withID = new(atomic.Bool)
					withID.Store(true)
				}
				ents, err = readDir(j.path, buf, ents[:0], withID)
				if withID != nil && !withID.Load() {
					t.noFileIDs.Store(true)
				}
				subs = subs[:0]
				var files, bytes int64
				t.mu.Lock()
				if err != nil && len(ents) == 0 {
					t.Nodes[j.id].Flags |= fDenied
					prog.Denied.Add(1)
				}
				for i := range ents {
					e := &ents[i]
					id := int32(len(t.Nodes))
					off, ln := t.addName(e.name)
					n := Node{Size: e.size, Alloc: e.alloc, Parent: j.id, First: none, Next: none,
						NameOff: off, NameLen: ln, MTime: e.mtime}
					if e.attr&windows.FILE_ATTRIBUTE_HIDDEN != 0 {
						n.Flags |= fHidden
					}
					if e.attr&windows.FILE_ATTRIBUTE_SYSTEM != 0 {
						n.Flags |= fSystem
					}
					if e.attr&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
						n.Flags |= fDir
						n.Size, n.Alloc = 0, 0
						link := e.attr&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 &&
							(e.tag == reparseMountPoint || e.tag == reparseSymlink)
						if link {
							n.Flags |= fLink
						} else {
							subs = append(subs, job{id, joinPath(j.path, e.name)})
						}
					} else {
						n.Ext = t.extID(e.name)
						if e.attr&(attrRecallOnData|attrRecallOnOpen|windows.FILE_ATTRIBUTE_OFFLINE) != 0 {
							n.Flags |= fCloud
						}
						files++
						bytes += e.size
						if t.trackLinks && e.fid != 0 && e.size > 0 {
							t.fids = append(t.fids, fidRef{e.fid, id})
						}
					}
					t.Nodes = append(t.Nodes, n)
					t.link(id, j.id)
				}
				t.mu.Unlock()
				prog.Files.Add(files)
				prog.Bytes.Add(bytes)
				prog.Dirs.Add(int64(len(subs)))
				if prog.Dirs.Load()%64 == 0 {
					prog.setCur(j.path)
				}

				qmu.Lock()
				queue = append(queue, subs...)
				active--
				cond.Broadcast()
				qmu.Unlock()
			}
		}()
	}
	wg.Wait()
	return ctx.Err()
}

// walkScan escanea una ruta recorriendo carpetas (no necesita administrador).
func walkScan(ctx context.Context, root string, prog *Progress) (*Tree, error) {
	t := newTree(root, 1<<16)
	t.Nodes = append(t.Nodes, Node{Parent: none, First: none, Next: none, Flags: fDir})
	t.Root = 0
	t.trackLinks = linkIDsReliable(root)
	if err := walkInto(ctx, t, 0, root, prog); err != nil {
		return nil, err
	}
	t.settleLinks(false)
	t.aggregate(t.Root)
	return t, nil
}

// linkIDsReliable: los identificadores de archivo sirven para reconocer enlaces
// duros solo en discos locales NTFS o ReFS. En unidades de red, algunos servidores
// repiten identificadores entre archivos distintos (y se contarían como uno).
func linkIDsReliable(root string) bool {
	p, err := windows.UTF16PtrFromString(volumeOf(root))
	if err != nil || windows.GetDriveType(p) == windows.DRIVE_REMOTE {
		return false
	}
	var fs [64]uint16
	if windows.GetVolumeInformation(p, nil, 0, nil, nil, nil, &fs[0], uint32(len(fs))) != nil {
		return false
	}
	name := windows.UTF16ToString(fs[:])
	return name == "NTFS" || name == "ReFS"
}

// walkSubtree lee una carpeta en un árbol aparte (para «Volver a leer» sin tener
// bloqueado el análisis mientras tanto).
func walkSubtree(ctx context.Context, path string, prog *Progress, trackLinks bool) (*Tree, error) {
	nt := newTree(path, 1<<12)
	nt.Nodes = append(nt.Nodes, Node{Parent: none, First: none, Next: none, Flags: fDir})
	nt.Root = 0
	nt.trackLinks = trackLinks
	if err := walkInto(ctx, nt, 0, path, prog); err != nil {
		return nil, err
	}
	nt.settleLinks(true)
	return nt, nil
}
