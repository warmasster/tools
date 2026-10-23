package main

// «Enviar con TurboSEND»: se habla con TurboSEND por su vía de control local. Si no está
// abierto, se arranca minimizado (tiene que seguir abierto para que puedan descargar).

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

type sendControl struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

func turbosendDataDir() string {
	base, _ := os.UserConfigDir()
	return filepath.Join(base, "EnviadorV2")
}

func processAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	return windows.GetExitCodeProcess(h, &code) == nil && code == 259 // STILL_ACTIVE
}

func readControl() *sendControl {
	var c sendControl
	b, err := os.ReadFile(filepath.Join(turbosendDataDir(), "control.json"))
	if err != nil || json.Unmarshal(b, &c) != nil || c.Port == 0 || !processAlive(c.PID) {
		return nil
	}
	return &c
}

var ctlClient = &http.Client{Timeout: 8 * time.Second}

func (c *sendControl) call(method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d%s", c.Port, path), rd)
	req.Header.Set("X-Turbo-Token", c.Token)
	resp, err := ctlClient.Do(req)
	if err != nil {
		return errors.New("TurboSEND no responde")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&e)
		if e.Error == "" {
			e.Error = "TurboSEND no ha aceptado el envío"
		}
		return errors.New(e.Error)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// ensureTurboSEND devuelve la conexión con TurboSEND, arrancándolo minimizado si hace falta.
func ensureTurboSEND() (*sendControl, error) {
	if c := readControl(); c != nil {
		return c, nil
	}
	exe := detectDeps().TurboSEND
	if exe == "" {
		return nil, errors.New("TurboSEND no está instalado")
	}
	cmd := exec.Command(exe, "--background")
	if err := cmd.Start(); err != nil {
		return nil, errors.New("No se pudo abrir TurboSEND")
	}
	go cmd.Wait()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(150 * time.Millisecond)
		if c := readControl(); c != nil {
			return c, nil
		}
	}
	return nil, errors.New("TurboSEND tarda en arrancar. Ábrelo y vuelve a probar.")
}

type SendRoom struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Updated int64  `json:"updated"`
	Online  int    `json:"online"`
	Members int    `json:"members"`
	Preview string `json:"preview"`
}

// SendRooms lista los chats de TurboSEND (arrancándolo si hace falta).
func (a *App) SendRooms() ([]SendRoom, error) {
	c, err := ensureTurboSEND()
	if err != nil {
		return nil, err
	}
	var out struct {
		Rooms []SendRoom `json:"rooms"`
	}
	if err := c.call("GET", "/rooms", nil, &out); err != nil {
		return nil, err
	}
	if out.Rooms == nil {
		out.Rooms = []SendRoom{}
	}
	return out.Rooms, nil
}

type SendResult struct {
	Room   string `json:"room"`
	Bundle string `json:"bundle"`
}

// SendTo manda la selección a un chat ("" = chat nuevo con ese nombre).
func (a *App) SendTo(room, name string, paths []string) (*SendResult, error) {
	if len(paths) == 0 {
		if job := a.currentJob(); job != nil {
			paths = job.Paths
		}
	}
	c, err := ensureTurboSEND()
	if err != nil {
		return nil, err
	}
	var res SendResult
	if err := c.call("POST", "/send", map[string]any{"room": room, "name": name, "paths": paths}, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// SendStatus devuelve enlaces del chat y progreso de las descargas del envío.
func (a *App) SendStatus(room, bundle string) (map[string]any, error) {
	c := readControl()
	if c == nil {
		return nil, errors.New("TurboSEND se ha cerrado: los demás ya no pueden descargarlo")
	}
	var out map[string]any
	err := c.call("GET", "/status?room="+room+"&bundle="+bundle, nil, &out)
	return out, err
}

// ShowTurboSEND saca la ventana de TurboSEND (por si el usuario quiere ver el chat).
func (a *App) ShowTurboSEND(room string) error {
	c := readControl()
	if c == nil {
		return errors.New("TurboSEND no está abierto")
	}
	return c.call("POST", "/show?room="+room, nil, nil)
}
