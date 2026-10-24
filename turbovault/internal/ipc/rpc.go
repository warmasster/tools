package ipc

import (
	"encoding/json"
	"sync"
	"time"
)

// Message es el sobre de todo lo que va por la tubería: peticiones (Cmd),
// respuestas (mismo ID, OK/Data/Error) y avisos (Event).
type Message struct {
	ID    int64           `json:"id,omitempty"`
	Cmd   string          `json:"cmd,omitempty"`
	Args  json.RawMessage `json:"args,omitempty"`
	OK    bool            `json:"ok,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
	Code  string          `json:"code,omitempty"`
	Event string          `json:"event,omitempty"`
}

// CallError es un error devuelto por el otro lado.
type CallError struct {
	Code string
	Msg  string
}

func (e *CallError) Error() string { return e.Msg }

// Client hace peticiones y recibe avisos por una conexión.
type Client struct {
	c       *Conn
	mu      sync.Mutex
	next    int64
	pending map[int64]chan Message
	onEvent func(Message)
	done    chan struct{}
	// Rejected: el otro lado rechazó la conexión (se puede leer tras Done).
	Rejected bool
}

// NewClient empieza a leer de la conexión. onEvent recibe los avisos.
func NewClient(c *Conn, onEvent func(Message)) *Client {
	cl := &Client{c: c, pending: map[int64]chan Message{}, onEvent: onEvent, done: make(chan struct{})}
	go cl.loop()
	return cl
}

func (cl *Client) loop() {
	defer func() {
		cl.mu.Lock()
		for id, ch := range cl.pending {
			ch <- Message{ID: id, Error: "se perdió la conexión con TurboVAULT", Code: "disconnected"}
			delete(cl.pending, id)
		}
		cl.mu.Unlock()
		close(cl.done)
	}()
	for {
		var m Message
		if err := cl.c.Read(&m); err != nil {
			return
		}
		if m.Event != "" {
			if m.Event == "rejected" {
				cl.mu.Lock()
				cl.Rejected = true
				cl.mu.Unlock()
				return
			}
			if cl.onEvent != nil {
				cl.onEvent(m)
			}
			continue
		}
		cl.mu.Lock()
		ch := cl.pending[m.ID]
		delete(cl.pending, m.ID)
		cl.mu.Unlock()
		if ch != nil {
			ch <- m
		}
	}
}

// Done se cierra cuando se pierde la conexión.
func (cl *Client) Done() <-chan struct{} { return cl.done }

// Close cierra la conexión.
func (cl *Client) Close() { cl.c.Close() }

// Call manda una petición y espera la respuesta (como mucho timeout).
func (cl *Client) Call(cmd string, args any, out any, timeout time.Duration) error {
	var raw json.RawMessage
	if args != nil {
		b, err := json.Marshal(args)
		if err != nil {
			return err
		}
		raw = b
	}
	ch := make(chan Message, 1)
	cl.mu.Lock()
	select {
	case <-cl.done:
		cl.mu.Unlock()
		return &CallError{Code: "disconnected", Msg: "se perdió la conexión con TurboVAULT"}
	default:
	}
	cl.next++
	id := cl.next
	cl.pending[id] = ch
	cl.mu.Unlock()
	if err := cl.c.Write(Message{ID: id, Cmd: cmd, Args: raw}); err != nil {
		cl.mu.Lock()
		delete(cl.pending, id)
		cl.mu.Unlock()
		return &CallError{Code: "disconnected", Msg: "se perdió la conexión con TurboVAULT"}
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case m := <-ch:
		if !m.OK {
			if m.Error == "" {
				m.Error = "error desconocido"
			}
			return &CallError{Code: m.Code, Msg: m.Error}
		}
		if out != nil && len(m.Data) > 0 {
			return json.Unmarshal(m.Data, out)
		}
		return nil
	case <-t.C:
		cl.mu.Lock()
		delete(cl.pending, id)
		cl.mu.Unlock()
		return &CallError{Code: "timeout", Msg: "TurboVAULT no responde"}
	}
}
