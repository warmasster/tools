// Package prefetch aprende qué trozos de qué archivos lee un juego mientras
// carga (con los eventos de archivos de Windows) y, la próxima vez que se
// abre, los lee por delante con la prioridad de disco más baja para que ya
// estén en memoria cuando el juego los pida. Es la precarga predictiva de los
// sistemas de archivos de los 90 (Kroeger y Long), aprendida juego a juego.
// Se nota en discos duros y SSD SATA; en un NVMe casi nada.
package prefetch

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"turbomon/internal/etw"
)

// Provider es Microsoft-Windows-Kernel-File.
var Provider = windows.GUID{Data1: 0xedd08927, Data2: 0x9cc4, Data3: 0x4e65, Data4: [8]byte{0xb9, 0x70, 0xc2, 0x56, 0x0f, 0xb5, 0xc2, 0x89}}

// Keywords: abrir archivos (CREATE) y leer (READ).
const Keywords = 0x80 | 0x100

// ProcessProvider es Microsoft-Windows-Kernel-Process (arranque de procesos con la palabra clave 0x10).
var ProcessProvider = windows.GUID{Data1: 0x22fb2cd6, Data2: 0x0e7b, Data3: 0x422b, Data4: [8]byte{0xa0, 0xc7, 0x2f, 0xad, 0x1f, 0xd0, 0xe7, 0x16}}

// ParseProcessStart lee un evento de arranque de proceso: su pid y su programa (ruta del núcleo).
func ParseProcessStart(r *etw.Record) (uint32, string, bool) {
	if r.Header.ID != 1 {
		return 0, "", false
	}
	b := r.Data()
	rd := etw.NewReader(b)
	pid := rd.U32()
	if !rd.OK() {
		return 0, "", false
	}
	// Cada versión de Windows mete campos nuevos antes del nombre: se busca
	// donde empieza la ruta («\Device\…»).
	pat := []byte{'\\', 0, 'D', 0, 'e', 0, 'v', 0, 'i', 0, 'c', 0, 'e', 0, '\\', 0}
	for i := 4; i+len(pat) <= len(b); i += 2 {
		if string(b[i:i+len(pat)]) == string(pat) {
			rd := etw.NewReader(b[i:])
			img := rd.UTF16Z()
			return pid, img, rd.OK() && img != ""
		}
	}
	return 0, "", false
}

// ChunkMB es el tamaño de cada trozo.
const ChunkMB = 4

const chunk = ChunkMB << 20

// Recorder apunta lo que lee un proceso.
type Recorder struct {
	// Pid: solo los archivos que abre este proceso (el filtro de Windows por
	// proceso no vale con los eventos del núcleo).
	Pid    uint32
	mu     sync.Mutex
	objs   map[uint64]int
	files  []*fileRec
	byPath map[string]int
	dev    map[string]string // \Device\HarddiskVolume3 → C:
	sysdir string
}

type fileRec struct {
	path   string
	chunks map[uint32]bool
}

// NewRecorder prepara un registro.
func NewRecorder() *Recorder {
	return &Recorder{objs: map[uint64]int{}, byPath: map[string]int{}, dev: dosDevices(), sysdir: strings.ToLower(os.Getenv("SystemRoot")) + `\`}
}

// dosDevices relaciona los nombres de volumen del núcleo con las letras.
func dosDevices() map[string]string {
	out := map[string]string{}
	buf := make([]uint16, 512)
	for c := 'A'; c <= 'Z'; c++ {
		name, _ := windows.UTF16PtrFromString(string(c) + ":")
		n, err := windows.QueryDosDevice(name, &buf[0], uint32(len(buf)))
		if err != nil || n == 0 {
			continue
		}
		out[strings.ToLower(windows.UTF16ToString(buf))] = string(c) + ":"
	}
	return out
}

// DOSPath pasa «\Device\HarddiskVolume3\Juegos\x.pak» a «C:\Juegos\x.pak».
func (r *Recorder) DOSPath(p string) string {
	lp := strings.ToLower(p)
	for d, letter := range r.dev {
		if strings.HasPrefix(lp, d+`\`) {
			return letter + p[len(d):]
		}
	}
	return ""
}

// Handle recibe un evento de Kernel-File.
func (r *Recorder) Handle(rec *etw.Record) {
	switch rec.Header.ID {
	case 12: // Create
		// Solo los archivos que abre el juego; las lecturas de esos archivos
		// cuentan las haga quien las haga (Windows lee por adelantado en su nombre).
		if r.Pid != 0 && rec.Header.ProcessID != r.Pid {
			return
		}
		rd := etw.NewReader(rec.Data())
		rd.Ptr() // Irp
		obj := rd.Ptr()
		rd.Skip(16)
		name := rd.UTF16Z()
		if !rd.OK() || name == "" {
			return
		}
		p := r.DOSPath(name)
		lp := strings.ToLower(p)
		if p == "" || strings.HasPrefix(lp, r.sysdir) || strings.HasSuffix(lp, `\`) || strings.Contains(lp, `$`) {
			return
		}
		r.mu.Lock()
		i, ok := r.byPath[lp]
		if !ok {
			i = len(r.files)
			r.files = append(r.files, &fileRec{path: p, chunks: map[uint32]bool{}})
			r.byPath[lp] = i
		}
		r.objs[obj] = i
		r.mu.Unlock()
	case 15: // Read
		rd := etw.NewReader(rec.Data())
		off := rd.U64()
		rd.Ptr() // Irp
		obj := rd.Ptr()
		rd.Ptr() // FileKey
		rd.U32() // hilo
		size := rd.U32()
		if !rd.OK() || size == 0 {
			return
		}
		r.mu.Lock()
		if i, ok := r.objs[obj]; ok {
			f := r.files[i]
			for c := off / chunk; c <= (off+uint64(size)-1)/chunk; c++ {
				f.chunks[uint32(c)] = true
			}
		}
		r.mu.Unlock()
	}
}

// PlanFile son los trozos de un archivo que hay que leer.
type PlanFile struct {
	Path   string   `json:"path"`
	Chunks []uint32 `json:"chunks"`
}

// Plan es lo que se ha aprendido de un juego.
type Plan struct {
	Files []PlanFile `json:"files"`
	Bytes uint64     `json:"bytes"`
	Runs  int        `json:"runs"`
}

// Plan devuelve lo aprendido (en el orden en que el juego lo pidió), hasta maxBytes.
func (r *Recorder) Plan(maxBytes uint64) Plan {
	r.mu.Lock()
	defer r.mu.Unlock()
	var p Plan
	for _, f := range r.files {
		if len(f.chunks) == 0 {
			continue
		}
		cs := make([]uint32, 0, len(f.chunks))
		for c := range f.chunks {
			cs = append(cs, c)
		}
		sort.Slice(cs, func(i, j int) bool { return cs[i] < cs[j] })
		if p.Bytes+uint64(len(cs))*chunk > maxBytes {
			break
		}
		p.Bytes += uint64(len(cs)) * chunk
		p.Files = append(p.Files, PlanFile{Path: f.path, Chunks: cs})
	}
	p.Runs = 1
	return p
}

// Merge junta lo aprendido antes con lo nuevo (lo nuevo manda en el orden).
func Merge(old, nw Plan, maxBytes uint64) Plan {
	out := Plan{Runs: old.Runs + nw.Runs}
	seen := map[string]map[uint32]bool{}
	var order []string
	paths := map[string]string{}
	for _, src := range []Plan{nw, old} {
		for _, f := range src.Files {
			k := strings.ToLower(f.Path)
			if seen[k] == nil {
				seen[k] = map[uint32]bool{}
				order = append(order, k)
				paths[k] = f.Path
			}
			for _, c := range f.Chunks {
				seen[k][c] = true
			}
		}
	}
	for _, k := range order {
		cs := make([]uint32, 0, len(seen[k]))
		for c := range seen[k] {
			cs = append(cs, c)
		}
		sort.Slice(cs, func(i, j int) bool { return cs[i] < cs[j] })
		if out.Bytes+uint64(len(cs))*chunk > maxBytes {
			break
		}
		out.Bytes += uint64(len(cs)) * chunk
		out.Files = append(out.Files, PlanFile{Path: paths[k], Chunks: cs})
	}
	return out
}

// Load lee un plan guardado.
func Load(path string) (Plan, bool) {
	var p Plan
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, &p) != nil {
		return p, false
	}
	return p, true
}

// Save guarda un plan.
func (p Plan) Save(path string) error {
	b, _ := json.Marshal(p)
	return os.WriteFile(path, b, 0o644)
}

var pSetThreadPrio = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetThreadPriority")

// Warm lee los trozos del plan con la prioridad de disco más baja (así el
// juego siempre va primero) para que queden en la caché de Windows. Devuelve
// los bytes leídos.
func Warm(ctx context.Context, p Plan) uint64 {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	th := windows.CurrentThread()
	pSetThreadPrio.Call(uintptr(th), 0x00010000)       // THREAD_MODE_BACKGROUND_BEGIN
	defer pSetThreadPrio.Call(uintptr(th), 0x00020000) // THREAD_MODE_BACKGROUND_END
	buf := make([]byte, 1<<20)
	var done uint64
	for _, f := range p.Files {
		if ctx.Err() != nil {
			break
		}
		name, err := windows.UTF16PtrFromString(f.Path)
		if err != nil {
			continue
		}
		h, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
		if err != nil {
			continue
		}
		for _, c := range f.Chunks {
			if ctx.Err() != nil {
				break
			}
			for k := 0; k < ChunkMB; k++ {
				off := int64(c)*chunk + int64(k)<<20
				ov := windows.Overlapped{Offset: uint32(off), OffsetHigh: uint32(off >> 32)}
				var n uint32
				if windows.ReadFile(h, buf, &n, &ov) != nil || n == 0 {
					break
				}
				done += uint64(n)
				if n < uint32(len(buf)) {
					break
				}
			}
		}
		windows.CloseHandle(h)
	}
	runtime.KeepAlive(unsafe.Pointer(&buf[0]))
	return done
}
