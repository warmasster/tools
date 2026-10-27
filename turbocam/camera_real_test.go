package main

// Prueba de verdad de la cámara virtual: la registra en HKCU, publica una carta de
// ajuste y la captura con ffmpeg por DirectShow, como haría cualquier app.
//
//	$env:TURBOCAM_REAL="1"; go test -run Real -v .
//
// Usa el ffmpeg de TurboMENU (o el del PATH). Deja la cámara registrada, igual que
// al abrir TurboCAM.

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func findFFmpeg() string {
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	if base, err := os.UserCacheDir(); err == nil {
		p := filepath.Join(base, "TurboMENU", "bin", "ffmpeg.exe")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// testPattern: franjas de color (en NV12) que se desplazan con n.
func testPattern(dst []byte, w, h, n int) {
	type yuv struct{ y, u, v byte }
	bars := []yuv{{235, 128, 128}, {210, 16, 146}, {170, 166, 16}, {145, 54, 34}, {106, 202, 222}, {81, 90, 240}, {41, 240, 110}, {16, 128, 128}}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst[y*w+x] = bars[((x+n*8)*len(bars)/w)%len(bars)].y
		}
	}
	uv := dst[w*h:]
	for y := 0; y < h/2; y++ {
		for x := 0; x < w/2; x++ {
			b := bars[((2*x+n*8)*len(bars)/w)%len(bars)]
			uv[y*w+2*x], uv[y*w+2*x+1] = b.u, b.v
		}
	}
}

func TestRealVirtualCamera(t *testing.T) {
	if os.Getenv("TURBOCAM_REAL") == "" {
		t.Skip("solo con TURBOCAM_REAL=1")
	}
	ff := findFFmpeg()
	if ff == "" {
		t.Skip("no hay ffmpeg")
	}
	if err := installCamera(); err != nil {
		t.Fatal(err)
	}

	out, _ := exec.Command(ff, "-hide_banner", "-list_devices", "true", "-f", "dshow", "-i", "dummy").CombinedOutput()
	if !strings.Contains(string(out), `"TurboCAM"`) {
		t.Fatalf("ffmpeg no ve la cámara:\n%s", out)
	}

	// Sin TurboCAM abierto: la tarjeta «TurboCAM está cerrado».
	shot := func(name, size, pix string) image.Image {
		t.Helper()
		args := []string{"-hide_banner", "-loglevel", "error", "-f", "dshow"}
		if size != "" {
			args = append(args, "-video_size", size)
		}
		if pix != "" {
			args = append(args, "-pixel_format", pix)
		}
		args = append(args, "-i", "video=TurboCAM", "-frames:v", "1", "-f", "image2pipe", "-vcodec", "png", "-")
		cmd := exec.Command(ff, args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		b, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, stderr.String())
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if dir := os.Getenv("TURBOCAM_SHOTS"); dir != "" {
			os.WriteFile(filepath.Join(dir, name+".png"), b, 0o644)
		}
		return img
	}
	img := shot("cerrado", "", "")
	t.Logf("tarjeta: %v", img.Bounds())

	s, err := openShm()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.SetFormat(1280, 720, 30)
	shot("esperando", "640x480", "yuyv422")

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 1280*720*3/2)
		for n := 0; ; n++ {
			select {
			case <-stop:
				return
			case <-time.After(33 * time.Millisecond):
			}
			testPattern(buf, 1280, 720, n)
			s.WriteFrame(1280, 720, func(dst []byte) { copy(dst, buf) })
			s.SetState(tcStateLive)
			s.Beat()
		}
	}()
	time.Sleep(200 * time.Millisecond)

	for _, c := range []struct{ name, size, pix string }{
		{"nv12-720", "1280x720", "nv12"},
		{"rgb24-1080", "1920x1080", "bgr24"},
		{"i420-360", "640x360", "yuv420p"},
	} {
		img := shot(c.name, c.size, c.pix)
		// La primera franja de la izquierda es blanca o de color, nunca el fondo oscuro.
		r, g, b, _ := img.At(img.Bounds().Dx()/16, img.Bounds().Dy()/2).RGBA()
		if r>>8 < 40 && g>>8 < 40 && b>>8 < 40 {
			t.Errorf("%s: no se ve la carta de ajuste (%d,%d,%d)", c.name, r>>8, g>>8, b>>8)
		}
		t.Logf("%s: %v ok", c.name, img.Bounds())
	}

	// Mientras ffmpeg captura, la cámara debe apuntarse como consumidor.
	cmd := exec.Command(ff, "-hide_banner", "-loglevel", "error", "-f", "dshow", "-i", "video=TurboCAM", "-t", "3", "-f", "null", "-")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	seen := false
	for i := 0; i < 25 && !seen; i++ {
		time.Sleep(100 * time.Millisecond)
		for _, c := range s.Consumers() {
			t.Logf("consumidor: %+v", c)
			seen = strings.EqualFold(c.Exe, "ffmpeg.exe")
		}
	}
	cmd.Wait()
	close(stop)
	<-done
	if !seen {
		t.Error("la cámara no ha apuntado a ffmpeg como consumidor")
	}
}

// TestRealPhoneE2E prueba la cadena entera: la página del móvil en un Chrome de
// verdad (con su cámara falsa) → servidor → motor → memoria compartida → cámara
// virtual → ffmpeg, como Discord o Meet.
//
//	$env:TURBOCAM_REAL="1"; go test -run RealPhone -v .
func TestRealPhoneE2E(t *testing.T) {
	if os.Getenv("TURBOCAM_REAL") == "" {
		t.Skip("solo con TURBOCAM_REAL=1")
	}
	ff := findFFmpeg()
	node, err := exec.LookPath("node")
	if ff == "" || err != nil {
		t.Skip("hace falta ffmpeg y node")
	}
	if err := installCamera(); err != nil {
		t.Fatal(err)
	}
	shm, err := openShm()
	if err != nil {
		t.Fatal(err)
	}
	defer shm.Close()
	e := newEngine(shm, nil, nil)
	defer e.Close()
	e.SetUIVisible(true)
	srv, err := startServer(e, "nonce")
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	url := fmt.Sprintf("https://127.0.0.1:%d/c/%s/", srv.port, getSettings().Token)

	phone := exec.Command(node, "dev/phone.mjs", url, "25")
	var logs bytes.Buffer
	phone.Stdout = &logs
	phone.Env = os.Environ()
	if err := phone.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		phone.Process.Kill()
		phone.Wait()
		t.Logf("móvil:\n%s", logs.String())
	}()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if st := e.State(); st.Status == "live" && len(st.Phones) == 1 && st.Phones[0].FPS > 5 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	st := e.State()
	if st.Status != "live" || len(st.Phones) != 1 {
		t.Fatalf("el móvil no emite: %+v", st)
	}
	p := st.Phones[0]
	// El móvil de pruebas no se queda en los ajustes de verdad.
	defer updateSettings(func(s *Settings) { delete(s.Devices, p.ID) })
	t.Logf("móvil: %s vía %s, %dx%d a %.0f fps, %.0f kb/s, calidad %.2f, %d ms", p.Name, p.Via, p.Width, p.Height, p.FPS, p.Kbps, p.Quality, p.RTT)
	if p.Width != 1280 || p.Height != 720 {
		t.Errorf("el móvil manda %dx%d, quería 1280x720 (el tamaño de salida)", p.Width, p.Height)
	}

	// Lo que ve una app: ni la tarjeta oscura ni basura, la imagen del móvil.
	out, err := exec.Command(ff, "-hide_banner", "-loglevel", "error", "-f", "dshow", "-video_size", "1280x720",
		"-i", "video=TurboCAM", "-frames:v", "1", "-f", "image2pipe", "-vcodec", "png", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if dir := os.Getenv("TURBOCAM_SHOTS"); dir != "" {
		os.WriteFile(filepath.Join(dir, "e2e.png"), out, 0o644)
	}
	var sum uint64
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 8 {
		for x := b.Min.X; x < b.Max.X; x += 8 {
			r, g, bb, _ := img.At(x, y).RGBA()
			sum += uint64(r>>8+g>>8+bb>>8) / 3
		}
	}
	avg := sum / uint64((b.Dx()/8)*(b.Dy()/8))
	t.Logf("brillo medio de lo que ve ffmpeg: %d", avg)
	if avg < 40 {
		t.Error("ffmpeg ve una imagen oscura: ¿la tarjeta en vez del móvil?")
	}

	// Espejo desde el PC: llega al móvil y la imagen cambia.
	adj := defaultAdjust()
	adj.Mirror, adj.Zoom = true, 1.5
	e.SetAdjust(p.ID, adj)
	time.Sleep(1500 * time.Millisecond)
	if d := getSettings().Devices[p.ID]; d == nil || !d.Adjust.Mirror {
		t.Error("el ajuste no se ha guardado")
	}
	out2, _ := exec.Command(ff, "-hide_banner", "-loglevel", "error", "-f", "dshow", "-i", "video=TurboCAM",
		"-frames:v", "1", "-f", "image2pipe", "-vcodec", "png", "-").Output()
	if dir := os.Getenv("TURBOCAM_SHOTS"); dir != "" {
		os.WriteFile(filepath.Join(dir, "e2e-espejo.png"), out2, 0o644)
	}
	e.SetAdjust(p.ID, defaultAdjust())

	// Congelar: el número de fotograma deja de avanzar.
	e.SetFrozen(true)
	time.Sleep(300 * time.Millisecond)
	before := atomic.LoadUint32(shm.u32(tcOffLatest))
	time.Sleep(700 * time.Millisecond)
	if atomic.LoadUint32(shm.u32(tcOffLatest)) != before {
		t.Error("congelada, la imagen sigue cambiando")
	}
	e.SetFrozen(false)

	// Foto a toda resolución pedida al móvil.
	path, err := e.TakePhoto()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	if f, err := os.Open(path); err == nil {
		cfg, _, err := image.DecodeConfig(f)
		f.Close()
		t.Logf("foto: %dx%d (%v)", cfg.Width, cfg.Height, err)
	}
}
