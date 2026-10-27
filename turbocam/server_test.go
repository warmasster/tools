package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// testRig levanta lo mismo que la app (memoria compartida de prueba, motor y servidor).
type testRig struct {
	shm *camShm
	e   *Engine
	s   *Server
	tok string
}

func newRig(t *testing.T) *testRig {
	t.Helper()
	// Cada prueba empieza con la escena y los móviles de fábrica.
	updateSettings(func(s *Settings) {
		s.Scene = Scene{Kind: "phone"}.normalized()
		s.Devices = map[string]*Device{}
		s.Profiles = nil
	})
	shm, err := openShm()
	if err != nil {
		t.Fatal(err)
	}
	e := newEngine(shm, nil, nil)
	s, err := startServer(e, "nonce-de-prueba")
	if err != nil {
		t.Fatal(err)
	}
	e.SetUIVisible(true) // como con la ventana a la vista: el móvil emite a ritmo completo
	r := &testRig{shm: shm, e: e, s: s, tok: getSettings().Token}
	t.Cleanup(func() {
		e.Close()
		s.Close()
		shm.Close()
	})
	return r
}

func (r *testRig) url(scheme, path string) string {
	return fmt.Sprintf("%s://127.0.0.1:%d%s", scheme, r.s.port, path)
}

var insecure = &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
	TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
}}

func testJPEG(t *testing.T, w, h int, y uint8) []byte {
	var b bytes.Buffer
	if err := jpeg.Encode(&b, solid(w, h, y, 128, 128), &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func frameMsg(kind byte, n uint32, data []byte) []byte {
	out := make([]byte, 5+len(data))
	out[0] = kind
	binary.LittleEndian.PutUint32(out[1:], n)
	copy(out[5:], data)
	return out
}

func waitFor(t *testing.T, what string, f func() bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if f() {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("no ha pasado: %s", what)
}

func TestServerPages(t *testing.T) {
	r := newRig(t)
	get := func(u string) (*http.Response, string) {
		resp, err := insecure.Get(u)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}
	// /ping responde por HTTP (lo usa el túnel) y por HTTPS (lo usa la prueba UPnP).
	for _, sch := range []string{"http", "https"} {
		if _, body := get(r.url(sch, "/ping")); body != "nonce-de-prueba" {
			t.Errorf("%s /ping = %q", sch, body)
		}
	}
	if resp, _ := get(r.url("https", "/c/secreto-falso/")); resp.StatusCode != 404 {
		t.Errorf("sin el secreto: %d", resp.StatusCode)
	}
	resp, body := get(r.url("https", "/c/"+r.tok+"/"))
	if resp.StatusCode != 200 || !strings.Contains(body, "phone.js") {
		t.Fatalf("página del móvil: %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Security-Policy"), "default-src 'self'") {
		t.Error("falta la CSP de la página del móvil")
	}
	if resp, _ := get(r.url("https", "/c/"+r.tok+"/phone.js")); resp.StatusCode != 200 {
		t.Error("no se sirve phone.js")
	}
	if resp, _ := get(r.url("https", "/c/"+r.tok+"/server.go")); resp.StatusCode != 404 {
		t.Error("sirve archivos que no son de la página")
	}
	resp, body = get(r.url("https", "/c/"+r.tok+"/manifest.webmanifest"))
	var man map[string]any
	if json.Unmarshal([]byte(body), &man) != nil || man["start_url"] != "/c/"+r.tok+"/" {
		t.Errorf("manifiesto: %s", body)
	}
}

func dialWS(t *testing.T, r *testRig, scheme string) *websocket.Conn {
	t.Helper()
	d := websocket.Dialer{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, HandshakeTimeout: 5 * time.Second}
	c, _, err := d.Dial(r.url(scheme, "/c/"+r.tok+"/ws"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func readMsg(t *testing.T, c *websocket.Conn, typ string) map[string]any {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		_, b, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("esperando %q: %v", typ, err)
		}
		var m map[string]any
		json.Unmarshal(b, &m)
		if m["t"] == typ {
			return m
		}
	}
}

func TestPhoneOverWebSocket(t *testing.T) {
	r := newRig(t)
	t.Run("ahorro", func(t *testing.T) {
		// Sin nadie mirando, el móvil solo manda una miniatura por segundo.
		r.e.SetUIVisible(false)
		c := dialWS(t, r, "ws")
		c.WriteJSON(map[string]string{"t": "hello", "id": "movilahorro1", "name": "Ahorro", "model": "x"})
		if cfg := readMsg(t, c, "config"); cfg["fps"].(float64) != 1 || cfg["w"].(float64) != 480 {
			t.Errorf("en reposo: %v", cfg)
		}
		c.Close()
		waitFor(t, "baja", func() bool { return len(r.e.State().Phones) == 0 })
		r.e.SetUIVisible(true)
	})
	c := dialWS(t, r, "wss")
	c.WriteJSON(map[string]string{"t": "hello", "id": "movilprueba01", "name": "Móvil de prueba", "model": "Pixel"})
	cfg := readMsg(t, c, "config")
	if cfg["role"] != "main" || cfg["w"].(float64) != 1280 || cfg["h"].(float64) != 720 {
		t.Fatalf("configuración inicial: %v", cfg)
	}
	c.WriteJSON(map[string]any{"t": "state", "visible": true, "live": true, "battery": 0.5})

	// Un fotograma del tamaño justo: va directo a la cámara.
	c.WriteMessage(websocket.BinaryMessage, frameMsg(frameKindVideo, 7, testJPEG(t, 1280, 720, 150)))
	if ack := readMsg(t, c, "ack"); ack["n"].(float64) != 7 {
		t.Fatalf("acuse: %v", ack)
	}
	waitFor(t, "fotograma en la memoria compartida", func() bool {
		return atomicLoad(r.shm, tcOffState) == tcStateLive && atomicLoad(r.shm, tcOffLatest) != 0
	})
	seq := atomicLoad(r.shm, tcOffLatest)
	slot := tcSlotBase + int(seq%tcSlots)*tcSlotStride
	if w, h := atomicLoad(r.shm, slot+tcSlotWidth), atomicLoad(r.shm, slot+tcSlotHeight); w != 1280 || h != 720 {
		t.Errorf("tamaño en la ranura: %dx%d", w, h)
	}
	if y := r.shm.mem[slot+tcSlotData+640*1280+640]; y < 145 || y > 155 {
		t.Errorf("Y en la cámara = %d, quería ~150", y)
	}

	// Uno de otro tamaño se escala.
	c.WriteMessage(websocket.BinaryMessage, frameMsg(frameKindVideo, 8, testJPEG(t, 640, 360, 90)))
	readMsg(t, c, "ack")
	waitFor(t, "fotograma escalado", func() bool { return atomicLoad(r.shm, tcOffLatest) != seq })

	// En segundo plano: la cámara enseña «en pausa».
	c.WriteJSON(map[string]any{"t": "state", "visible": false})
	waitFor(t, "estado en pausa", func() bool { return atomicLoad(r.shm, tcOffState) == tcStatePaused })

	// Retoques desde el móvil: se guardan.
	c.WriteJSON(map[string]any{"t": "adjust", "adjust": map[string]any{"zoom": 2.5, "mirror": true, "fit": "cover"}})
	waitFor(t, "retoques guardados", func() bool {
		d := getSettings().Devices["movilprueba01"]
		return d != nil && d.Adjust.Zoom == 2.5 && d.Adjust.Mirror
	})

	// Foto: el PC la pide, el móvil la manda.
	done := make(chan string, 1)
	go func() {
		p, err := r.e.TakePhoto()
		if err != nil {
			done <- "error: " + err.Error()
			return
		}
		done <- p
	}()
	readMsg(t, c, "photo")
	c.WriteMessage(websocket.BinaryMessage, frameMsg(frameKindPhoto, 0, testJPEG(t, 1920, 1080, 200)))
	select {
	case p := <-done:
		if !strings.HasSuffix(p, ".jpg") {
			t.Fatalf("foto: %s", p)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("la foto no ha llegado")
	}

	// Al cerrar, desaparece del motor.
	c.Close()
	waitFor(t, "móvil dado de baja", func() bool { return len(r.e.State().Phones) == 0 })
}

func atomicLoad(s *camShm, off int) uint32 { return atomic.LoadUint32(s.u32(off)) }

func TestPhoneOverHTTPFallback(t *testing.T) {
	r := newRig(t)
	req, _ := http.NewRequest("GET", r.url("https", "/c/"+r.tok+"/ev?id=movilhttp0001&name=iPhone&model=iPhone"), nil)
	client := &http.Client{Transport: insecure.Transport} // sin límite de tiempo: es un flujo
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	events := make(chan map[string]any, 32)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if l, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				var m map[string]any
				json.Unmarshal([]byte(l), &m)
				events <- m
			}
		}
		close(events)
	}()
	next := func(typ string) map[string]any {
		for {
			select {
			case m, ok := <-events:
				if !ok {
					t.Fatalf("se cerró el flujo esperando %q", typ)
				}
				if m["t"] == typ {
					return m
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("esperando %q", typ)
			}
		}
	}
	conn := next("conn")["c"].(string)
	next("config")
	post := func(body []byte, ct string) {
		resp, err := insecure.Post(r.url("https", "/c/"+r.tok+"/x?c="+conn), ct, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 204 {
			t.Fatalf("POST: %d", resp.StatusCode)
		}
	}
	post([]byte(`{"t":"state","visible":true,"live":true}`), "application/json")
	post(frameMsg(frameKindVideo, 3, testJPEG(t, 1280, 720, 100)), "application/octet-stream")
	if ack := next("ack"); ack["n"].(float64) != 3 {
		t.Fatalf("acuse: %v", ack)
	}
	waitFor(t, "imagen por HTTP", func() bool { return atomicLoad(r.shm, tcOffState) == tcStateLive })
}

func TestPlainHTTPFromOutsideRedirects(t *testing.T) {
	// Sin cifrar solo se atiende desde este mismo PC (el túnel); desde fuera se
	// redirige a HTTPS. Una prueba no puede conectar desde otra IP, así que se
	// comprueba la función que lo decide.
	if !isLoopback("127.0.0.1:5000") || isLoopback("192.168.1.5:5000") || isLoopback("basura") {
		t.Error("isLoopback")
	}
}

func TestCoverAndFreeze(t *testing.T) {
	r := newRig(t)
	r.e.SetCovered(true)
	waitFor(t, "cámara tapada", func() bool { return atomicLoad(r.shm, tcOffState) == tcStateCover })
	r.e.SetCovered(false)
	waitFor(t, "cámara destapada", func() bool { return atomicLoad(r.shm, tcOffState) == tcStateWaiting })
}

// TestScreenScenes compone «móvil sobre la pantalla» con una captura de pantalla de
// verdad. Necesita escritorio (no vale en un servicio), así que no corre con -short.
func TestScreenScenes(t *testing.T) {
	if testing.Short() {
		t.Skip("captura la pantalla: sin -short")
	}
	if len(listMonitors()) == 0 {
		t.Skip("no hay pantallas")
	}
	r := newRig(t)
	c := dialWS(t, r, "wss")
	c.WriteJSON(map[string]string{"t": "hello", "id": "movilescena1", "name": "Escena", "model": "x"})
	readMsg(t, c, "config")
	c.WriteJSON(map[string]any{"t": "state", "visible": true, "live": true})
	updateSettings(func(s *Settings) {
		s.Scene = Scene{Kind: "pip", Main: "screen", Pip: "movilescena1", PipCorner: "br", PipSize: .3}
	})
	r.e.SceneChanged()
	// El móvil pasa a ser el recuadro: se le pide su tamaño (30 % del ancho).
	cfg := readMsg(t, c, "config")
	if cfg["role"] != "pip" || cfg["w"].(float64) != 384 {
		t.Fatalf("configuración del recuadro: %v", cfg)
	}
	c.WriteMessage(websocket.BinaryMessage, frameMsg(frameKindVideo, 1, testJPEG(t, 384, 216, 235)))
	readMsg(t, c, "ack")
	time.Sleep(400 * time.Millisecond) // unos cuantos tics del compositor
	if atomicLoad(r.shm, tcOffState) != tcStateLive {
		t.Fatalf("estado %d, quería en directo (¿falla la captura de pantalla? %q)", atomicLoad(r.shm, tcOffState), r.e.State().ScreenError)
	}
	seq := atomicLoad(r.shm, tcOffLatest)
	slot := tcSlotBase + int(seq%tcSlots)*tcSlotStride
	data := r.shm.mem[slot+tcSlotData:]
	// Centro del recuadro (abajo a la derecha): el blanco del móvil.
	x, y := 1280-32-192, 720-32-108
	if v := data[y*1280+x]; v < 225 {
		t.Errorf("en el recuadro Y = %d, quería el blanco del móvil", v)
	}
	// Y la pantalla sigue llegando: el número de fotograma avanza solo.
	time.Sleep(300 * time.Millisecond)
	if atomicLoad(r.shm, tcOffLatest) == seq {
		t.Error("con la pantalla en la escena, la imagen no se renueva")
	}
}

func TestSendLimitsAndOrder(t *testing.T) {
	r := newRig(t)
	c := dialWS(t, r, "wss")
	c.WriteJSON(map[string]string{"t": "hello", "id": "movillimite1", "name": "Límites", "model": "Pixel 9"})
	readMsg(t, c, "config")
	c.WriteJSON(map[string]any{"t": "state", "visible": true, "live": true})

	// 360p y 15 fps para este móvil: es lo que se le pide.
	adj := defaultAdjust()
	adj.Res, adj.FPS = 360, 15
	r.e.SetAdjust("movillimite1", adj)
	cfg := readMsg(t, c, "config")
	if cfg["w"].(float64) != 640 || cfg["h"].(float64) != 360 || cfg["fps"].(float64) != 15 {
		t.Fatalf("con límites: %v", cfg)
	}

	// Llega primero el 5 y luego el 4 (otro hilo del móvil acabó más tarde): se
	// confirman los dos, pero en la cámara se queda el 5.
	c.WriteMessage(websocket.BinaryMessage, frameMsg(frameKindVideo, 5, testJPEG(t, 640, 360, 200)))
	readMsg(t, c, "ack")
	waitFor(t, "el 5 en la cámara", func() bool { return atomicLoad(r.shm, tcOffState) == tcStateLive })
	time.Sleep(100 * time.Millisecond)
	c.WriteMessage(websocket.BinaryMessage, frameMsg(frameKindVideo, 4, testJPEG(t, 640, 360, 40)))
	readMsg(t, c, "ack")
	time.Sleep(200 * time.Millisecond)
	seq := atomicLoad(r.shm, tcOffLatest)
	slot := tcSlotBase + int(seq%tcSlots)*tcSlotStride
	if y := r.shm.mem[slot+tcSlotData+360*1280+640]; y < 180 {
		t.Errorf("se ha colado un fotograma viejo (Y = %d)", y)
	}
}

func TestForgetFromPhone(t *testing.T) {
	r := newRig(t)
	c := dialWS(t, r, "wss")
	c.WriteJSON(map[string]string{"t": "hello", "id": "movilolvido1", "name": "Olvido", "model": "Galaxy"})
	readMsg(t, c, "config")
	c.WriteJSON(map[string]string{"t": "forget"})
	if m := readMsg(t, c, "bye"); m["reason"] != "forgotten" {
		t.Fatalf("despedida: %v", m)
	}
	waitFor(t, "olvidado", func() bool { return getSettings().Devices["movilolvido1"] == nil && len(r.e.State().Phones) == 0 })
}

func TestAdoptDeviceFromOtherLink(t *testing.T) {
	r := newRig(t)
	// Un móvil conocido, con sus ajustes y protagonista de la escena.
	updateSettings(func(s *Settings) {
		a := defaultAdjust()
		a.Zoom, a.Mirror = 2, true
		s.Devices["viejoenlace01"] = &Device{Name: "El mío", Model: "23129RAA4G", Adjust: a}
		s.Scene.Main = "viejoenlace01"
	})
	// Entra por otro enlace: el navegador le da otra identidad, pero es el mismo modelo.
	c := dialWS(t, r, "wss")
	c.WriteJSON(map[string]string{"t": "hello", "id": "nuevoenlace01", "name": "23129RAA4G", "model": "23129RAA4G"})
	cfg := readMsg(t, c, "config")
	adj := cfg["adjust"].(map[string]any)
	s := getSettings()
	if adj["zoom"].(float64) != 2 || adj["mirror"] != true || cfg["name"] != "El mío" {
		t.Errorf("no ha recuperado sus ajustes: %v", cfg)
	}
	if s.Devices["viejoenlace01"] != nil || s.Scene.Main != "nuevoenlace01" {
		t.Errorf("no se ha cambiado la identidad: %+v", s.Scene)
	}
}

func TestAliveEndpoint(t *testing.T) {
	r := newRig(t)
	for tok, want := range map[string]int{r.tok: 204, strings.Repeat("0", 32): 404} {
		resp, err := insecure.Get(r.url("https", "/c/"+tok+"/alive"))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("alive con %s…: %d, quería %d", tok[:6], resp.StatusCode, want)
		}
	}
	// Los scripts del móvil (también los de los workers) llevan su tipo correcto.
	for _, f := range []string{"phone.js", "encoder.js", "paint.js", "sw.js"} {
		resp, err := insecure.Get(r.url("https", "/c/"+r.tok+"/"+f))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if ct := resp.Header.Get("Content-Type"); resp.StatusCode != 200 || !strings.HasPrefix(ct, "text/javascript") {
			t.Errorf("%s: %d %s", f, resp.StatusCode, ct)
		}
	}
}
