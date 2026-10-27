package main

// Puente de Native Messaging: Chrome lanza "TurboVAULT.exe chrome-extension://…"
// cuando la extensión se conecta, y este proceso pasa los mensajes al agente
// por la tubería. No guarda nada: el agente es quien decide qué contestar y
// sabe que la conexión viene de un navegador (lo comprueba él mismo).

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"

	"turbovault/internal/ipc"
	"turbovault/internal/winsec"
)

const maxChromeMsg = 1 << 20

var stdoutMu sync.Mutex

func writeChrome(raw []byte) {
	if len(raw) > maxChromeMsg {
		raw, _ = json.Marshal(map[string]any{"ok": false, "error": "Respuesta demasiado grande"})
	}
	stdoutMu.Lock()
	defer stdoutMu.Unlock()
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(raw)))
	os.Stdout.Write(n[:])
	os.Stdout.Write(raw)
}

func readChrome(r *bufio.Reader) ([]byte, error) {
	var n [4]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(n[:])
	if size > maxChromeMsg {
		return nil, io.ErrUnexpectedEOF
	}
	b := make([]byte, size)
	_, err := io.ReadFull(r, b)
	return b, err
}

func runHost(origin string) int {
	_ = winsec.HardenProcess()
	_ = winsec.HardenThreads()
	_ = winsec.BlockInjection(true)
	if !strings.EqualFold(strings.TrimSuffix(origin, "/"), "chrome-extension://"+extensionID) {
		return 1
	}
	// El puente solo tiene sentido si lo lanza un navegador.
	_, bpid, err := winsec.BrowserProcess(windows.GetCurrentProcessId())
	if err != nil {
		logf("puente no lanzado por un navegador: %v", err)
		return 1
	}
	// Y el otro extremo de su entrada y su salida tiene que ser ese navegador: el padre
	// se puede falsificar al lanzar un proceso, pero quien lo lanza así tiene él las
	// tuberías (Chrome crea las suyas y se las pasa por cmd.exe).
	if in, out := winsec.StdioPipeServers(); in != bpid || out != bpid {
		logf("puente con la entrada o la salida de otro proceso (%d, %d; navegador %d)", in, out, bpid)
		return 1
	}
	in := bufio.NewReader(os.Stdin)
	conn, other, err := connectAgent(8 * time.Second)
	if err != nil {
		msg := "No se pudo arrancar TurboVAULT."
		if other != nil {
			msg = "Hay otro TurboVAULT abierto desde otra carpeta (" + other.Path + "). Ciérralo desde la bandeja."
		}
		logf("puente: %v", err)
		// Contestar con el error a lo que llegue hasta que Chrome cierre.
		for {
			raw, err := readChrome(in)
			if err != nil {
				return 0
			}
			var m ipc.Message
			json.Unmarshal(raw, &m)
			out, _ := json.Marshal(ipc.Message{ID: m.ID, Error: msg, Code: "noagent"})
			writeChrome(out)
		}
	}
	defer conn.Close()

	var mu sync.Mutex
	pairIDs := map[int64]bool{}
	done := make(chan struct{})

	// agente → Chrome
	go func() {
		defer close(done)
		for {
			raw, err := conn.ReadRaw()
			if err != nil {
				return
			}
			var m ipc.Message
			if json.Unmarshal(raw, &m) == nil && m.ID != 0 {
				mu.Lock()
				isPair := pairIDs[m.ID]
				delete(pairIDs, m.ID)
				mu.Unlock()
				if isPair && m.OK {
					spawnUI("--pair")
				}
			}
			if m.Event == "rejected" {
				logf("puente rechazado por el agente: %s", m.Error)
			}
			writeChrome(raw)
		}
	}()

	// Chrome → agente
	go func() {
		for {
			raw, err := readChrome(in)
			if err != nil {
				conn.Close()
				return
			}
			var m ipc.Message
			if json.Unmarshal(raw, &m) != nil {
				continue
			}
			switch m.Cmd {
			case "openApp":
				spawnUI()
				out, _ := json.Marshal(ipc.Message{ID: m.ID, OK: true})
				writeChrome(out)
				continue
			case "unlock":
				// La contraseña maestra nunca pasa por el navegador: se abre la
				// ventanita de TurboVAULT para escribirla allí.
				spawnUI("--unlock")
				out, _ := json.Marshal(ipc.Message{ID: m.ID, OK: true})
				writeChrome(out)
				continue
			case "pairRequest":
				mu.Lock()
				pairIDs[m.ID] = true
				mu.Unlock()
			}
			if err := conn.WriteRaw(raw); err != nil {
				conn.Close()
				return
			}
		}
	}()
	<-done
	return 0
}
