package main

// Reparto de trabajo entre procesos.
//
// El menú clásico del Explorador lanza un proceso por cada archivo seleccionado, así que
// el primero de cada acción ("líder") recoge las rutas de los demás por una tubería con
// nombre durante un par de segundos. Además, cuando se cierra una ventana el proceso no
// muere enseguida: se queda oculto unos minutos esperando trabajo, y la siguiente acción
// se abre en él al instante (sin volver a cargar la interfaz).

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type Job struct {
	Action string   `json:"action"`
	Paths  []string `json:"paths"`
	X      int32    `json:"x"`
	Y      int32    `json:"y"`
	Time   int64    `json:"time"`
}

var actions = map[string]bool{
	"home": true, "reduce": true, "convert": true, "send": true, "qr": true, "receive": true,
	"safe": true, "lock": true, "origin": true, "folder": true, "quit": true,
}

func validAction(a string) bool { return actions[a] }

// Cuánto tiempo sigue una ventana aceptando rutas de otros procesos de la misma selección.
const collectWindow = 2 * time.Second

var (
	kernel32          = windows.NewLazySystemDLL("kernel32.dll")
	procWaitNamedPipe = kernel32.NewProc("WaitNamedPipeW")
	procCreateMutex   = kernel32.NewProc("CreateMutexW")
)

func userTag() string {
	var sid uint32
	windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &sid)
	u := os.Getenv("USERNAME")
	return fmt.Sprintf("%s.%d", sanitizeTag(u), sid)
}

func sanitizeTag(s string) string {
	b := []rune{}
	for _, r := range s {
		if r == '\\' || r == '/' || r == ':' {
			r = '_'
		}
		b = append(b, r)
	}
	return string(b)
}

func pipeName(kind string) string { return `\\.\pipe\TurboMENU.` + userTag() + "." + kind }

// ---------- cliente ----------

func dialPipe(name string, timeout time.Duration) (windows.Handle, error) {
	p, _ := windows.UTF16PtrFromString(name)
	deadline := time.Now().Add(timeout)
	for {
		h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
		if err == nil {
			return h, nil
		}
		if time.Now().After(deadline) {
			return 0, err
		}
		if err == windows.ERROR_PIPE_BUSY {
			procWaitNamedPipe.Call(uintptr(unsafe.Pointer(p)), 50)
		} else {
			time.Sleep(15 * time.Millisecond)
		}
	}
}

// pipeConn lee y escribe en una tubería. Si ov es true el handle es asíncrono y cada
// operación espera como mucho ioTimeout (un cliente colgado no bloquea al servidor).
type pipeConn struct {
	h  windows.Handle
	ov bool
}

const ioTimeout = 3 * time.Second

func (c pipeConn) io(b []byte, write bool) (int, error) {
	var n uint32
	if !c.ov {
		var err error
		if write {
			err = windows.WriteFile(c.h, b, &n, nil)
		} else {
			err = windows.ReadFile(c.h, b, &n, nil)
		}
		if err == windows.ERROR_BROKEN_PIPE {
			return int(n), io.EOF
		}
		return int(n), err
	}
	ev, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(ev)
	o := windows.Overlapped{HEvent: ev}
	if write {
		err = windows.WriteFile(c.h, b, &n, &o)
	} else {
		err = windows.ReadFile(c.h, b, &n, &o)
	}
	if err == windows.ERROR_IO_PENDING {
		if ev, _ := windows.WaitForSingleObject(ev, uint32(ioTimeout/time.Millisecond)); ev != windows.WAIT_OBJECT_0 {
			windows.CancelIoEx(c.h, &o)
			windows.GetOverlappedResult(c.h, &o, &n, true)
			return 0, errors.New("tiempo de espera agotado")
		}
		err = windows.GetOverlappedResult(c.h, &o, &n, true)
	}
	if err == windows.ERROR_BROKEN_PIPE {
		return int(n), io.EOF
	}
	return int(n), err
}

func (c pipeConn) Read(b []byte) (int, error)  { return c.io(b, false) }
func (c pipeConn) Write(b []byte) (int, error) { return c.io(b, true) }

func writeMsg(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], uint32(len(b)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func readMsg(r io.Reader, v any) error {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return err
	}
	n := binary.LittleEndian.Uint32(hdr[:])
	if n > 64<<20 {
		return errors.New("mensaje demasiado grande")
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// sendJob entrega el trabajo por una tubería y espera la confirmación.
func sendJob(name string, timeout time.Duration, job *Job) bool {
	h, err := dialPipe(name, timeout)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	c := pipeConn{h: h}
	if writeMsg(c, job) != nil {
		return false
	}
	var ack struct {
		OK  bool `json:"ok"`
		PID int  `json:"pid"`
	}
	if readMsg(c, &ack) != nil || !ack.OK {
		return false
	}
	return true
}

// handoff intenta que otro proceso de TurboMENU se encargue del trabajo.
// Devuelve true si ya está entregado y este proceso debe terminar.
func handoff(job *Job) bool {
	if job.Action == "home" {
		return false
	}
	collector := pipeName("collect." + job.Action)
	// 1) ¿Hay una ventana de esta acción recogiendo la selección ahora mismo?
	if sendJob(collector, 0, job) {
		return true
	}
	// 2) Elección de líder entre los procesos lanzados a la vez para la misma selección.
	mn, _ := windows.UTF16PtrFromString(`Local\TurboMENU.lead.` + userTag() + "." + job.Action)
	h, _, err := procCreateMutex.Call(0, 1, uintptr(unsafe.Pointer(mn)))
	if h != 0 && err == windows.ERROR_ALREADY_EXISTS {
		windows.CloseHandle(windows.Handle(h))
		if sendJob(collector, collectWindow, job) {
			return true
		}
		return false
	}
	leadMutex = windows.Handle(h)
	// 3) Somos el líder: ¿hay un TurboMENU preparado esperando trabajo?
	allowForeground(-1) // que pueda ponerse delante (nosotros venimos del Explorador)
	if sendJob(pipeName("idle"), 0, job) {
		return true // el mutex se libera al salir del proceso
	}
	return false
}

var leadMutex windows.Handle

func closeHandle(h windows.Handle) { windows.CloseHandle(h) }

// releaseLead suelta el mutex de líder (cuando ya no se recogen más rutas).
func releaseLead() {
	if leadMutex != 0 {
		windows.CloseHandle(leadMutex)
		leadMutex = 0
	}
}

// ---------- servidor ----------

const (
	pipeAccessDuplex = 0x00000003
	pipeRejectRemote = 0x00000008
	pipeUnlimited    = 255
	fileFlagOverlap  = 0x40000000
	errPipeConnected = windows.Errno(535)
)

type pipeServer struct {
	name   string
	handle func(job *Job) bool
	stopEv windows.Handle
	once   sync.Once
	done   chan struct{}
}

// servePipe atiende conexiones en la tubería hasta stop(). handle decide si acepta el
// trabajo. Cuando vuelve, la tubería ya existe y los clientes pueden conectarse.
func servePipe(name string, handle func(job *Job) bool) *pipeServer {
	ev, _ := windows.CreateEvent(nil, 1, 0, nil)
	s := &pipeServer{name: name, handle: handle, stopEv: ev, done: make(chan struct{})}
	ready := make(chan struct{})
	go s.loop(ready)
	<-ready
	return s
}

func (s *pipeServer) loop(ready chan struct{}) {
	defer close(s.done)
	p, _ := windows.UTF16PtrFromString(s.name)
	first := true
	for {
		h, err := windows.CreateNamedPipe(p, pipeAccessDuplex|fileFlagOverlap,
			windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT|pipeRejectRemote,
			pipeUnlimited, 64<<10, 64<<10, 0, nil)
		if first {
			first = false
			close(ready)
		}
		if err != nil {
			return
		}
		cev, _ := windows.CreateEvent(nil, 1, 0, nil)
		o := windows.Overlapped{HEvent: cev}
		err = windows.ConnectNamedPipe(h, &o)
		connected := err == nil || err == errPipeConnected
		if err == windows.ERROR_IO_PENDING {
			r, _ := windows.WaitForMultipleObjects([]windows.Handle{cev, s.stopEv}, false, windows.INFINITE)
			if r == windows.WAIT_OBJECT_0 {
				var n uint32
				connected = windows.GetOverlappedResult(h, &o, &n, false) == nil
			} else {
				windows.CancelIoEx(h, &o)
				var n uint32
				windows.GetOverlappedResult(h, &o, &n, true)
				windows.CloseHandle(cev)
				windows.CloseHandle(h)
				return
			}
		}
		windows.CloseHandle(cev)
		if !connected {
			windows.CloseHandle(h)
			if s.isStopped() {
				return
			}
			continue
		}
		go s.serveConn(h)
	}
}

func (s *pipeServer) isStopped() bool {
	r, _ := windows.WaitForSingleObject(s.stopEv, 0)
	return r == windows.WAIT_OBJECT_0
}

func (s *pipeServer) serveConn(h windows.Handle) {
	defer windows.CloseHandle(h)
	c := pipeConn{h: h, ov: true}
	var job Job
	if readMsg(c, &job) != nil {
		return
	}
	ok := false
	if !s.isStopped() && validAction(job.Action) {
		ok = s.handle(&job)
	}
	writeMsg(c, map[string]any{"ok": ok, "pid": os.Getpid()})
	windows.FlushFileBuffers(h)
	windows.DisconnectNamedPipe(h)
}

// stop deja de aceptar conexiones nuevas.
func (s *pipeServer) stop() {
	s.once.Do(func() {
		windows.SetEvent(s.stopEv)
		select {
		case <-s.done:
		case <-time.After(time.Second):
		}
	})
}

// quitIdle cierra el TurboMENU que esté preparado en segundo plano (al desinstalar).
func quitIdle() {
	sendJob(pipeName("idle"), 300*time.Millisecond, &Job{Action: "quit"})
}
