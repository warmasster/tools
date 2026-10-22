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
func readDir(path string, buf []byte, out []rawEntry) ([]rawEntry, error) {
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
	for {
		err := windows.GetFileInformationByHandleEx(h, fileFullDirectoryInfo, &buf[0], uint32(len(buf)))
		if err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				return out, nil
			}
			return out, err
		}
		off := 0
		for {
			e := buf[off:]
			next := le32(e, 0)
			nameLen := int(le32(e, 60))
			u := unsafe.Slice((*uint16)(unsafe.Pointer(&e[68])), nameLen/2)
			if !(len(u) == 1 && u[0] == '.') && !(len(u) == 2 && u[0] == '.' && u[1] == '.') {
				out = append(out, rawEntry{
					name:  utf16ToString(u),
					attr:  le32(e, 56),
					tag:   le32(e, 64),
					size:  int64(le64(e, 40)),
					alloc: int64(le64(e, 48)),
					mtime: filetimeToUnix(int64(le64(e, 24))),
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
				ents, err = readDir(j.path, buf, ents[:0])
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
	if err := walkInto(ctx, t, 0, root, prog); err != nil {
		return nil, err
	}
	t.aggregate(t.Root)
	return t, nil
}
