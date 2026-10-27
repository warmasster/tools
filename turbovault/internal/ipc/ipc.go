// Package ipc es la tubería con nombre por la que hablan los procesos de
// TurboVAULT (el agente que guarda la bóveda abierta, la ventana y el puente
// del navegador).
//
// Protecciones:
//   - Solo el usuario actual puede abrir la tubería (DACL protegida) y no se
//     admiten conexiones por red.
//   - El servidor crea la primera instancia con FILE_FLAG_FIRST_PIPE_INSTANCE:
//     si otro programa ya tenía ese nombre, falla en vez de compartirlo.
//   - Cada lado comprueba qué proceso hay al otro lado (por su PID) antes de
//     hablar, con la función que se le pase.
//   - El cliente se conecta con SECURITY_IDENTIFICATION: un servidor falso no
//     podría hacerse pasar por el usuario con sus credenciales.
package ipc

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"turbovault/internal/winsec"
)

const (
	maxMsg            = 8 << 20
	pipeAccessDuplex  = 0x00000003
	pipeRejectRemote  = 0x00000008
	firstPipeInstance = 0x00080000
	fileFlagOverlap   = 0x40000000
	securitySQOS      = 0x00100000 // SECURITY_SQOS_PRESENT
	securityIdent     = 0x00010000 // SECURITY_IDENTIFICATION
	errPipeConnected  = windows.Errno(535)
	writeTimeout      = 5 * time.Second
)

var (
	kernel32          = windows.NewLazySystemDLL("kernel32.dll")
	procWaitNamedPipe = kernel32.NewProc("WaitNamedPipeW")
	procGetClientPID  = kernel32.NewProc("GetNamedPipeClientProcessId")
	procGetServerPID  = kernel32.NewProc("GetNamedPipeServerProcessId")
	ErrBusy           = errors.New("otro programa ya usa la tubería de TurboVAULT")
	ErrClosed         = errors.New("conexión cerrada")
	errTimeout        = errors.New("tiempo de espera agotado")
	errTooBig         = errors.New("mensaje demasiado grande")
	ErrPeerRejected   = errors.New("el otro lado de la conexión no es TurboVAULT")
)

// Name devuelve el nombre de la tubería del usuario actual.
func Name(suffix string) string {
	sid, err := winsec.UserSID()
	if err != nil {
		sid = "x"
	}
	n := `\\.\pipe\TurboVAULT.` + sid
	if suffix != "" {
		n += "." + suffix
	}
	return n
}

// ---------- conexión ----------

// Conn es un extremo de la tubería. Los mensajes van con su longitud delante.
type Conn struct {
	h       windows.Handle
	closeEv windows.Handle
	wmu     sync.Mutex
	rmu     sync.Mutex
	once    sync.Once
	PID     uint32 // proceso del otro lado
	Info    any    // lo que devolvió la verificación
}

func newConn(h windows.Handle, pid uint32) *Conn {
	ev, _ := windows.CreateEvent(nil, 1, 0, nil)
	return &Conn{h: h, closeEv: ev, PID: pid}
}

func (c *Conn) do(b []byte, write bool, timeout time.Duration) (int, error) {
	ev, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(ev)
	o := windows.Overlapped{HEvent: ev}
	var n uint32
	if write {
		err = windows.WriteFile(c.h, b, &n, &o)
	} else {
		err = windows.ReadFile(c.h, b, &n, &o)
	}
	if err == windows.ERROR_IO_PENDING {
		wait := uint32(windows.INFINITE)
		if timeout > 0 {
			wait = uint32(timeout / time.Millisecond)
		}
		r, _ := windows.WaitForMultipleObjects([]windows.Handle{ev, c.closeEv}, false, wait)
		if r != windows.WAIT_OBJECT_0 {
			windows.CancelIoEx(c.h, &o)
			windows.GetOverlappedResult(c.h, &o, &n, true)
			if r == windows.WAIT_OBJECT_0+1 {
				return 0, ErrClosed
			}
			return 0, errTimeout
		}
		err = windows.GetOverlappedResult(c.h, &o, &n, true)
	}
	if err == windows.ERROR_BROKEN_PIPE || err == windows.ERROR_PIPE_NOT_CONNECTED || err == windows.ERROR_NO_DATA {
		return int(n), io.EOF
	}
	return int(n), err
}

func (c *Conn) readFull(b []byte) error {
	for len(b) > 0 {
		n, err := c.do(b, false, 0)
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}
		b = b[n:]
	}
	return nil
}

// ReadRaw lee un mensaje.
func (c *Conn) ReadRaw() ([]byte, error) {
	c.rmu.Lock()
	defer c.rmu.Unlock()
	var hdr [4]byte
	if err := c.readFull(hdr[:]); err != nil {
		return nil, err
	}
	n := binary.LittleEndian.Uint32(hdr[:])
	if n > maxMsg {
		return nil, errTooBig
	}
	b := make([]byte, n)
	if err := c.readFull(b); err != nil {
		return nil, err
	}
	return b, nil
}

// WriteRaw escribe un mensaje.
func (c *Conn) WriteRaw(b []byte) error {
	if len(b) > maxMsg {
		return errTooBig
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	buf := make([]byte, 4+len(b))
	binary.LittleEndian.PutUint32(buf, uint32(len(b)))
	copy(buf[4:], b)
	for len(buf) > 0 {
		n, err := c.do(buf, true, writeTimeout)
		if err != nil {
			return err
		}
		buf = buf[n:]
	}
	return nil
}

// Read lee un mensaje JSON. El búfer se borra después (puede llevar la
// contraseña maestra o una contraseña).
func (c *Conn) Read(v any) error {
	b, err := c.ReadRaw()
	if err != nil {
		return err
	}
	err = json.Unmarshal(b, v)
	for i := range b {
		b[i] = 0
	}
	return err
}

// Write escribe un mensaje JSON.
func (c *Conn) Write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.WriteRaw(b)
}

// Close cierra la conexión (y despierta a quien esté leyendo).
func (c *Conn) Close() error {
	c.once.Do(func() {
		windows.SetEvent(c.closeEv)
		windows.CancelIoEx(c.h, nil)
		c.wmu.Lock()
		c.rmu.Lock()
		windows.CloseHandle(c.h)
		c.rmu.Unlock()
		c.wmu.Unlock()
		windows.CloseHandle(c.closeEv)
	})
	return nil
}

// ---------- servidor ----------

// Verify decide si se acepta al proceso del otro lado. Lo que devuelva queda en
// Conn.Info.
type Verify func(pid uint32) (any, error)

// Server atiende conexiones.
type Server struct {
	name   string
	sa     *windows.SecurityAttributes
	verify Verify
	handle func(*Conn)
	stopEv windows.Handle
	done   chan struct{}
	once   sync.Once
	mu     sync.Mutex
	conns  map[*Conn]bool
}

func securityAttrs() (*windows.SecurityAttributes, error) {
	sid, err := winsec.UserSID()
	if err != nil {
		return nil, err
	}
	sd, err := windows.SecurityDescriptorFromString(fmt.Sprintf("D:P(A;;GA;;;%s)", sid))
	if err != nil {
		return nil, err
	}
	return &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}, nil
}

func (s *Server) create(first bool) (windows.Handle, error) {
	p, _ := windows.UTF16PtrFromString(s.name)
	mode := uint32(pipeAccessDuplex | fileFlagOverlap)
	if first {
		mode |= firstPipeInstance
	}
	return windows.CreateNamedPipe(p, mode,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT|pipeRejectRemote,
		windows.PIPE_UNLIMITED_INSTANCES, 64<<10, 64<<10, 0, s.sa)
}

// Listen empieza a atender en la tubería. handle se llama en una goroutine por
// cada conexión aceptada y debe volver cuando la conexión se cierre.
func Listen(name string, verify Verify, handle func(*Conn)) (*Server, error) {
	sa, err := securityAttrs()
	if err != nil {
		return nil, err
	}
	ev, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, err
	}
	s := &Server{name: name, sa: sa, verify: verify, handle: handle, stopEv: ev, done: make(chan struct{}), conns: map[*Conn]bool{}}
	h, err := s.create(true)
	if err != nil {
		windows.CloseHandle(ev)
		if err == windows.ERROR_ACCESS_DENIED || err == windows.ERROR_PIPE_BUSY {
			return nil, ErrBusy
		}
		return nil, err
	}
	go s.loop(h)
	return s, nil
}

func (s *Server) loop(h windows.Handle) {
	defer close(s.done)
	for {
		if h == 0 {
			var err error
			h, err = s.create(false)
			if err != nil {
				if s.stopped() {
					return
				}
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}
		cev, _ := windows.CreateEvent(nil, 1, 0, nil)
		o := windows.Overlapped{HEvent: cev}
		err := windows.ConnectNamedPipe(h, &o)
		connected := err == nil || err == errPipeConnected
		if err == windows.ERROR_IO_PENDING {
			r, _ := windows.WaitForMultipleObjects([]windows.Handle{cev, s.stopEv}, false, windows.INFINITE)
			var n uint32
			if r == windows.WAIT_OBJECT_0 {
				connected = windows.GetOverlappedResult(h, &o, &n, false) == nil
			} else {
				windows.CancelIoEx(h, &o)
				windows.GetOverlappedResult(h, &o, &n, true)
				windows.CloseHandle(cev)
				windows.CloseHandle(h)
				return
			}
		}
		windows.CloseHandle(cev)
		if !connected {
			windows.CloseHandle(h)
			h = 0
			if s.stopped() {
				return
			}
			continue
		}
		var pid uint32
		procGetClientPID.Call(uintptr(h), uintptr(unsafe.Pointer(&pid)))
		c := newConn(h, pid)
		h = 0
		go s.serve(c)
	}
}

func (s *Server) serve(c *Conn) {
	if s.verify != nil {
		info, err := s.verify(c.PID)
		if err != nil {
			c.Write(map[string]any{"event": "rejected", "error": err.Error()})
			c.Close()
			return
		}
		c.Info = info
	}
	s.mu.Lock()
	if s.stopped() {
		s.mu.Unlock()
		c.Close()
		return
	}
	s.conns[c] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.conns, c)
		s.mu.Unlock()
		c.Close()
	}()
	s.handle(c)
}

func (s *Server) stopped() bool {
	r, _ := windows.WaitForSingleObject(s.stopEv, 0)
	return r == windows.WAIT_OBJECT_0
}

// Conns devuelve las conexiones abiertas (para avisar a todos de un cambio).
func (s *Server) Conns() []*Conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Conn, 0, len(s.conns))
	for c := range s.conns {
		out = append(out, c)
	}
	return out
}

// Close deja de atender y cierra todas las conexiones.
func (s *Server) Close() {
	s.once.Do(func() {
		s.mu.Lock()
		windows.SetEvent(s.stopEv)
		conns := make([]*Conn, 0, len(s.conns))
		for c := range s.conns {
			conns = append(conns, c)
		}
		s.mu.Unlock()
		for _, c := range conns {
			c.Close()
		}
		<-s.done
	})
}

// ---------- cliente ----------

// Dial se conecta a la tubería. verifyServer comprueba el proceso servidor.
func Dial(name string, timeout time.Duration, verifyServer func(pid uint32) error) (*Conn, error) {
	p, _ := windows.UTF16PtrFromString(name)
	deadline := time.Now().Add(timeout)
	var h windows.Handle
	for {
		var err error
		h, err = windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING,
			fileFlagOverlap|securitySQOS|securityIdent, 0)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		if err == windows.ERROR_PIPE_BUSY {
			procWaitNamedPipe.Call(uintptr(unsafe.Pointer(p)), 100)
		} else {
			time.Sleep(25 * time.Millisecond)
		}
	}
	var pid uint32
	procGetServerPID.Call(uintptr(h), uintptr(unsafe.Pointer(&pid)))
	c := newConn(h, pid)
	if verifyServer != nil {
		if err := verifyServer(pid); err != nil {
			c.Close()
			return nil, fmt.Errorf("%w: %v", ErrPeerRejected, err)
		}
	}
	return c, nil
}
