package ipc

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func testName(t *testing.T) string {
	return Name(fmt.Sprintf("test.%d.%d", windows.GetCurrentProcessId(), time.Now().UnixNano()))
}

func echoServer(t *testing.T, name string, verify Verify) *Server {
	s, err := Listen(name, verify, func(c *Conn) {
		for {
			var m Message
			if err := c.Read(&m); err != nil {
				return
			}
			switch m.Cmd {
			case "echo":
				c.Write(Message{ID: m.ID, OK: true, Data: m.Args})
			case "event":
				c.Write(Message{Event: "ping", Data: json.RawMessage(`1`)})
				c.Write(Message{ID: m.ID, OK: true})
			case "slow":
				time.Sleep(300 * time.Millisecond)
				c.Write(Message{ID: m.ID, OK: true})
			default:
				c.Write(Message{ID: m.ID, Error: "no", Code: "bad"})
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCallsAndEvents(t *testing.T) {
	name := testName(t)
	pid := windows.GetCurrentProcessId()
	s := echoServer(t, name, func(p uint32) (any, error) {
		if p != pid {
			return nil, errors.New("pid raro")
		}
		return "yo", nil
	})
	defer s.Close()

	c, err := Dial(name, time.Second, func(p uint32) error {
		if p != pid {
			return errors.New("servidor raro")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	events := 0
	cl := NewClient(c, func(m Message) { mu.Lock(); events++; mu.Unlock() })
	defer cl.Close()

	var out map[string]string
	if err := cl.Call("echo", map[string]string{"a": "ñ"}, &out, time.Second); err != nil || out["a"] != "ñ" {
		t.Fatalf("%v %v", err, out)
	}
	big := strings.Repeat("x", 300<<10) // más que el búfer de la tubería
	var bigOut string
	if err := cl.Call("echo", big, &bigOut, 5*time.Second); err != nil || bigOut != big {
		t.Fatalf("mensaje grande: %v %d", err, len(bigOut))
	}
	if err := cl.Call("event", nil, nil, time.Second); err != nil {
		t.Fatal(err)
	}
	var ce *CallError
	if err := cl.Call("otra", nil, nil, time.Second); !errors.As(err, &ce) || ce.Code != "bad" {
		t.Fatalf("error: %v", err)
	}
	// Varias a la vez.
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var n int
			if err := cl.Call("echo", i, &n, 5*time.Second); err != nil || n != i {
				t.Errorf("concurrente %d: %v %d", i, err, n)
			}
		}(i)
	}
	wg.Wait()
	mu.Lock()
	if events != 1 {
		t.Fatalf("avisos: %d", events)
	}
	mu.Unlock()
	if len(s.Conns()) != 1 {
		t.Fatalf("conexiones: %d", len(s.Conns()))
	}
}

func TestRejectedClient(t *testing.T) {
	name := testName(t)
	s := echoServer(t, name, func(uint32) (any, error) { return nil, errors.New("no te conozco") })
	defer s.Close()
	c, err := Dial(name, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	cl := NewClient(c, nil)
	if err := cl.Call("echo", 1, nil, time.Second); err == nil {
		t.Fatal("debía rechazarse")
	}
	select {
	case <-cl.Done():
	case <-time.After(time.Second):
		t.Fatal("la conexión rechazada no se cerró")
	}
}

func TestServerVerification(t *testing.T) {
	name := testName(t)
	s := echoServer(t, name, nil)
	defer s.Close()
	if _, err := Dial(name, time.Second, func(uint32) error { return errors.New("no") }); !errors.Is(err, ErrPeerRejected) {
		t.Fatalf("%v", err)
	}
}

func TestSquatting(t *testing.T) {
	name := testName(t)
	s := echoServer(t, name, nil)
	defer s.Close()
	if _, err := Listen(name, nil, func(*Conn) {}); err != ErrBusy {
		t.Fatalf("un segundo servidor con el mismo nombre: %v", err)
	}
}

func TestCloseWakesClient(t *testing.T) {
	name := testName(t)
	s := echoServer(t, name, nil)
	c, _ := Dial(name, time.Second, nil)
	cl := NewClient(c, nil)
	done := make(chan error, 1)
	go func() { done <- cl.Call("slow", nil, nil, 5*time.Second) }()
	time.Sleep(50 * time.Millisecond)
	s.Close()
	select {
	case <-cl.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("el cliente no se enteró del cierre")
	}
	<-done
}
