package media

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Las pruebas con ffmpeg de verdad necesitan TURBOMENU_FFMPEG_DIR (carpeta con
// ffmpeg.exe y ffprobe.exe) y no corren con -short.

var (
	samplesOnce sync.Once
	samplesDir  string
	samplesErr  error
)

func tools(t *testing.T) (string, string) {
	t.Helper()
	dir := os.Getenv("TURBOMENU_FFMPEG_DIR")
	if dir == "" || testing.Short() {
		t.Skip("define TURBOMENU_FFMPEG_DIR para las pruebas con ffmpeg")
	}
	f, p := Find(dir)
	if f == "" || p == "" {
		t.Fatalf("no hay ffmpeg en %s", dir)
	}
	CacheDir = filepath.Join(os.TempDir(), "turbomenu-media-test")
	return f, p
}

func TestMain(m *testing.M) {
	code := m.Run()
	if samplesDir != "" && os.Getenv("TURBOMENU_KEEP") == "" {
		os.RemoveAll(samplesDir)
	}
	os.Exit(code)
}

// samples genera una vez los archivos de prueba.
func samples(t *testing.T) string {
	ff, _ := tools(t)
	samplesOnce.Do(func() {
		samplesDir, samplesErr = os.MkdirTemp("", "turbomenu-media-")
		if samplesErr != nil {
			return
		}
		srt := filepath.Join(samplesDir, "subs.srt")
		os.WriteFile(srt, []byte("1\n00:00:00,500 --> 00:00:02,000\nHola\n"), 0o644)
		jobs := map[string][]string{
			// 1080p30 con ruido (difícil de comprimir) y audio estéreo: pesa mucho.
			"grande.mp4": {"-f", "lavfi", "-i", "testsrc2=s=1920x1080:r=30:d=20,noise=alls=8:allf=t",
				"-f", "lavfi", "-i", "sine=f=440:d=20:sample_rate=48000", "-ac", "2",
				"-c:v", "libx264", "-preset", "veryfast", "-crf", "21", "-c:a", "aac", "-b:a", "192k"},
			// HEVC girado 90º (como un móvil en vertical).
			"girado0.mov": {"-f", "lavfi", "-i", "testsrc2=s=1280x720:r=30:d=4",
				"-f", "lavfi", "-i", "sine=d=4", "-c:v", "libx265", "-preset", "ultrafast", "-tag:v", "hvc1", "-c:a", "aac"},
			// MKV con H.264, Opus y subtítulos.
			"pista.mkv": {"-f", "lavfi", "-i", "testsrc2=s=1280x720:r=25:d=6", "-f", "lavfi", "-i", "sine=d=6",
				"-i", srt, "-map", "0", "-map", "1", "-map", "2",
				"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "libopus", "-c:s", "srt"},
			// HDR10 (PQ, 10 bits).
			"hdr.mp4": {"-f", "lavfi", "-i", "testsrc2=s=1280x720:r=30:d=3,setparams=color_primaries=bt2020:color_trc=smpte2084:colorspace=bt2020nc",
				"-pix_fmt", "yuv420p10le", "-c:v", "libx265", "-preset", "ultrafast"},
			"musica.wav": {"-f", "lavfi", "-i", "sine=f=330:d=60:sample_rate=44100", "-ac", "2", "-c:a", "pcm_s16le"},
			"musica.mp3": {"-f", "lavfi", "-i", "sine=f=330:d=60:sample_rate=44100", "-ac", "2", "-c:a", "libmp3lame", "-b:a", "256k"},
			"alfa.png": {"-f", "lavfi", "-i", "testsrc2=s=1600x1200:d=1,format=rgba,geq=r='r(X,Y)':g='g(X,Y)':b='b(X,Y)':a='if(gt(X,800),255,0)'",
				"-frames:v", "1"},
		}
		var wg sync.WaitGroup
		var mu sync.Mutex
		for name, args := range jobs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				full := append([]string{"-hide_banner", "-loglevel", "error", "-y"}, args...)
				full = append(full, filepath.Join(samplesDir, name))
				if _, log, err := output(context.Background(), ff, full...); err != nil {
					mu.Lock()
					samplesErr = &Error{Msg: name + ": " + err.Error(), Detail: string(log)}
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		// El giro va en los metadatos (como hacen los móviles), sin recodificar.
		if samplesErr == nil {
			if _, log, err := output(context.Background(), ff, "-hide_banner", "-loglevel", "error", "-y",
				"-display_rotation", "90", "-i", filepath.Join(samplesDir, "girado0.mov"), "-c", "copy",
				filepath.Join(samplesDir, "girado.mov")); err != nil {
				samplesErr = &Error{Msg: "girado.mov: " + err.Error(), Detail: string(log)}
			}
		}
	})
	if samplesErr != nil {
		t.Fatal(samplesErr, samplesErr.(*Error).Detail)
	}
	return samplesDir
}

func probe(t *testing.T, name string) *Info {
	t.Helper()
	_, fp := tools(t)
	t0 := time.Now()
	in, err := Probe(context.Background(), fp, filepath.Join(samples(t), name))
	if err != nil {
		t.Fatalf("Probe %s: %v", name, err)
	}
	t.Logf("Probe %s en %v: %s %s %dx%d %.2ffps rot=%d v=%s a=%s %.1fs %d kb/s %.1f MB hdr=%v",
		name, time.Since(t0).Round(time.Millisecond), in.Kind, in.Container, in.Width, in.Height, in.FPS,
		in.Rotation, in.VideoCodec, in.AudioCodec, in.Duration, in.Bitrate/1000, float64(in.Size)/(1<<20), in.HDR)
	return in
}

func logProgress(t *testing.T) func(Progress) {
	last := time.Time{}
	stage := ""
	return func(p Progress) {
		if time.Since(last) > time.Second || p.Fraction >= 1 || p.Stage != stage {
			last, stage = time.Now(), p.Stage
			t.Logf("  %5.1f%%  %.2fx  ETA %.1fs  %s", p.Fraction*100, p.Speed, p.ETA, p.Stage)
		}
	}
}

func TestProbe(t *testing.T) {
	in := probe(t, "grande.mp4")
	if in.Kind != "video" || in.Width != 1920 || in.Height != 1080 || !in.HasAudio || in.VideoCodec != "h264" {
		t.Errorf("grande.mp4 mal leído: %+v", in)
	}
	g := probe(t, "girado.mov")
	if g.Rotation%180 != 90 || g.Width != 720 || g.Height != 1280 || g.VideoCodec != "hevc" {
		t.Errorf("girado.mov: rot=%d %dx%d", g.Rotation, g.Width, g.Height)
	}
	if h := probe(t, "hdr.mp4"); !h.HDR {
		t.Error("hdr.mp4 no detectado como HDR")
	}
	if a := probe(t, "musica.mp3"); a.Kind != "audio" || a.AudioCodec != "mp3" || a.AudioBitrate < 200000 {
		t.Errorf("musica.mp3: %+v", a)
	}
	if p := probe(t, "alfa.png"); p.Kind != "image" || !p.Alpha {
		t.Errorf("alfa.png: %+v", p)
	}
	k := probe(t, "pista.mkv")
	if k.AudioBitrate == 0 || len(streamsOf(k, "subtitle")) != 1 {
		t.Errorf("pista.mkv: %+v", k)
	}
}

func TestHardwareEncoder(t *testing.T) {
	ff, _ := tools(t)
	os.Remove(hwCacheFile())
	hwMu.Lock()
	clear(hwCache)
	hwMu.Unlock()
	t0 := time.Now()
	enc := HardwareEncoder(context.Background(), ff)
	t.Logf("codificador por hardware: %q (prueba %v)", enc, time.Since(t0).Round(time.Millisecond))
	hwMu.Lock()
	clear(hwCache)
	hwMu.Unlock()
	t0 = time.Now()
	if again := HardwareEncoder(context.Background(), ff); again != enc {
		t.Errorf("caché en disco: %q != %q", again, enc)
	}
	t.Logf("desde la caché en disco: %v", time.Since(t0))
}

// withEncoder fuerza el codificador (para probar x264 en un PC con GPU).
func withEncoder(t *testing.T, ff, enc string) {
	HardwareEncoder(context.Background(), ff)
	key := hwKey(ff)
	hwMu.Lock()
	old := hwCache[key]
	hwCache[key] = enc
	hwMu.Unlock()
	t.Cleanup(func() {
		hwMu.Lock()
		hwCache[key] = old
		hwMu.Unlock()
	})
}

func TestReduceTarget(t *testing.T) {
	ff, _ := tools(t)
	in := probe(t, "grande.mp4")
	hw := HardwareEncoder(context.Background(), ff)
	for _, c := range []struct {
		enc    string
		target int64
	}{{hw, SizeDiscord}, {"", SizeDiscord}, {hw, 3 << 20}, {"", 3 << 20}, {hw, SizeWhatsApp}, {hw, 600 << 10}, {"", 600 << 10}} {
		name := c.enc
		if name == "" {
			name = "libx264"
		}
		t.Run(name+"-"+itoa(int(c.target>>10))+"KB", func(t *testing.T) {
			withEncoder(t, ff, c.enc)
			est := Estimate(in, ReduceOptions{TargetBytes: c.target})
			res, err := Reduce(context.Background(), ff, in, ReduceOptions{TargetBytes: c.target}, logProgress(t))
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(res.Path)
			out := probe(t, filepath.Base(res.Path))
			t.Logf("%s -> %.2f MB (objetivo %.2f MB, estimado %.2f) en %.1fs con %s, %dx%d",
				filepath.Base(res.Path), float64(res.Size)/(1<<20), float64(c.target)/(1<<20),
				float64(est)/(1<<20), res.Seconds, res.Encoder, out.Width, out.Height)
			if res.Size > c.target || res.Size != fileSize(res.Path) {
				t.Errorf("se pasa del objetivo: %d > %d", res.Size, c.target)
			}
			if out.VideoCodec != "h264" || out.AudioCodec != "aac" || out.PixFmt != "yuv420p" {
				t.Errorf("salida no compatible: %s %s %s", out.VideoCodec, out.AudioCodec, out.PixFmt)
			}
		})
	}
}

func TestReduceQuality(t *testing.T) {
	ff, _ := tools(t)
	for _, name := range []string{"grande.mp4", "girado.mov", "hdr.mp4"} {
		in := probe(t, name)
		for _, q := range []string{QualityHigh, QualityMedium, QualityLow} {
			est := Estimate(in, ReduceOptions{Quality: q})
			t0 := time.Now()
			if s, err := EstimateSampled(context.Background(), ff, in, ReduceOptions{Quality: q}); err == nil {
				t.Logf("%s %s: estimación con muestra %.2f MB en %v", name, q, float64(s)/(1<<20), time.Since(t0).Round(time.Millisecond))
			}
			res, err := Reduce(context.Background(), ff, in, ReduceOptions{Quality: q}, nil)
			if err == ErrNotSmaller {
				t.Logf("%s %s: ya muy comprimido (estimado %.2f MB)", name, q, float64(est)/(1<<20))
				continue
			}
			if err != nil {
				t.Fatalf("%s %s: %v", name, q, err)
			}
			out := probe(t, filepath.Base(res.Path))
			t.Logf("%s %s: %.2f MB -> %.2f MB (estimado %.2f) %dx%d en %.1fs con %s",
				name, q, float64(in.Size)/(1<<20), float64(res.Size)/(1<<20), float64(est)/(1<<20),
				out.Width, out.Height, res.Seconds, res.Encoder)
			if name == "girado.mov" && out.Height < out.Width {
				t.Errorf("se perdió el giro: %dx%d", out.Width, out.Height)
			}
			os.Remove(res.Path)
		}
	}
}

func TestReduceAudio(t *testing.T) {
	ff, _ := tools(t)
	for _, name := range []string{"musica.wav", "musica.mp3"} {
		in := probe(t, name)
		for _, o := range []ReduceOptions{{Quality: QualityHigh}, {Quality: QualityLow}, {TargetBytes: 1 << 20}} {
			res, err := Reduce(context.Background(), ff, in, o, nil)
			if err != nil {
				t.Fatalf("%s %+v: %v", name, o, err)
			}
			t.Logf("%s %+v -> %s %.2f MB (estimado %.2f) en %.2fs", name, o, filepath.Base(res.Path),
				float64(res.Size)/(1<<20), float64(Estimate(in, o))/(1<<20), res.Seconds)
			if o.TargetBytes > 0 && res.Size > o.TargetBytes {
				t.Errorf("se pasa: %d", res.Size)
			}
			os.Remove(res.Path)
		}
	}
}

func TestConvert(t *testing.T) {
	ff, _ := tools(t)
	for _, name := range []string{"pista.mkv", "girado.mov", "musica.wav", "alfa.png"} {
		in := probe(t, name)
		for _, tg := range Targets(in) {
			opt := ConvertOptions{}
			if tg.ID == "gif" {
				opt.Length = 3
			}
			res, err := Convert(context.Background(), ff, in, tg.ID, opt, nil)
			if err != nil {
				t.Errorf("%s -> %s: %v (%s)", name, tg.ID, err, detail(err))
				continue
			}
			out := probe(t, filepath.Base(res.Path))
			t.Logf("%s -> %-5s %-22s %8.1f KB en %.2fs (%s)  [%s]", name, tg.ID, tg.Name,
				float64(res.Size)/1024, res.Seconds, res.Encoder, tg.Desc)
			if !strings.EqualFold(filepath.Ext(res.Path), "."+tg.Ext) {
				t.Errorf("extensión: %s", res.Path)
			}
			if tg.Kind == "video" && (out.Kind != "video" || out.Width != in.Width || out.Height != in.Height) {
				t.Errorf("%s: vídeo %s %dx%d (esperaba %dx%d)", res.Path, out.Kind, out.Width, out.Height, in.Width, in.Height)
			}
			if tg.Kind == "audio" && (out.Kind != "audio" || !out.HasAudio) {
				t.Errorf("%s: no es audio", res.Path)
			}
			os.Remove(res.Path)
		}
	}
}

func TestGIFLong(t *testing.T) {
	ff, _ := tools(t)
	in := probe(t, "grande.mp4")
	for _, o := range []ConvertOptions{{GIFSize: 640, GIFFPS: 12}, {Start: 5, Length: 4}} {
		res, err := Convert(context.Background(), ff, in, "gif", o, logProgress(t))
		if err != nil {
			t.Fatal(err)
		}
		out := probe(t, filepath.Base(res.Path))
		t.Logf("GIF %+v: %.2f MB en %.1fs, %dx%d %.1fs", o, float64(res.Size)/(1<<20), res.Seconds, out.Width, out.Height, out.Duration)
		os.Remove(res.Path)
	}
}

func TestImage(t *testing.T) {
	ff, _ := tools(t)
	dir := samples(t)
	in := filepath.Join(dir, "alfa.png")
	for _, f := range []string{"webp", "avif"} {
		out := filepath.Join(dir, "img."+f)
		t0 := time.Now()
		if err := EncodeImage(context.Background(), ff, in, out, f, 75, 800); err != nil {
			t.Fatal(f, err, detail(err))
		}
		i := probe(t, "img."+f)
		t.Logf("%s: %d KB %dx%d alfa=%v en %v", f, fileSize(out)/1024, i.Width, i.Height, i.Alpha, time.Since(t0).Round(time.Millisecond))
		if i.Width != 800 || i.Height != 600 {
			t.Errorf("%s: tamaño %dx%d", f, i.Width, i.Height)
		}
		// En AVIF la transparencia va como segunda imagen.
		if f == "avif" && len(i.Streams) != 2 || f == "webp" && !i.Alpha {
			t.Errorf("%s: se perdió la transparencia (%d pistas)", f, len(i.Streams))
		}
	}
}

func TestCancel(t *testing.T) {
	ff, _ := tools(t)
	in := probe(t, "grande.mp4")
	withEncoder(t, ff, "") // x264 es lo más lento: da tiempo a cancelar
	out := filepath.Join(samples(t), "cancelado.mp4")
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{}, 1)
	go func() {
		<-started
		time.Sleep(700 * time.Millisecond)
		cancel()
	}()
	t0 := time.Now()
	var tc time.Time
	_, err := Reduce(ctx, ff, in, ReduceOptions{Quality: QualityHigh, Out: out}, func(p Progress) {
		select {
		case started <- struct{}{}:
		default:
		}
		if ctx.Err() != nil && tc.IsZero() {
			tc = time.Now()
		}
	})
	t.Logf("cancelado tras %v: %v", time.Since(t0).Round(time.Millisecond), err)
	if !IsCanceled(err) {
		t.Fatalf("esperaba cancelación, llegó %v", err)
	}
	if exists(out) || exists(tempFor(out)) {
		t.Error("quedó la salida a medias")
	}
}

func TestOutputPath(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "video.mp4")
	os.WriteFile(in, nil, 0o644)
	if p := OutputPath(in, "reducido", "mp4"); filepath.Base(p) != "video (reducido).mp4" {
		t.Error(p)
	}
	os.WriteFile(filepath.Join(dir, "video (reducido).mp4"), nil, 0o644)
	if p := OutputPath(in, "reducido", ".mp4"); filepath.Base(p) != "video (reducido 2).mp4" {
		t.Error(p)
	}
	if p := OutputPath(in, "", "webm"); filepath.Base(p) != "video.webm" {
		t.Error(p)
	}
	if p := OutputPath(in, "", "mp4"); filepath.Base(p) != "video (2).mp4" {
		t.Error(p)
	}
}

func TestPickAsset(t *testing.T) {
	sums := map[string]string{
		"ffmpeg-master-latest-win64-gpl-shared.zip":   "a",
		"ffmpeg-n8.1-latest-win64-gpl-shared-8.1.zip": "b",
		"ffmpeg-n9.0-latest-win64-gpl-shared-9.0.zip": "c",
		"ffmpeg-n9.0-latest-win64-gpl-9.0.zip":        "d",
	}
	if a := pickAsset(sums); a != "ffmpeg-n9.0-latest-win64-gpl-shared-9.0.zip" {
		t.Error(a)
	}
}

func detail(err error) string {
	if e, ok := err.(*Error); ok {
		return e.Detail
	}
	return ""
}

// Prueba con un archivo cualquiera: TURBOMENU_MEDIA_FILE=<ruta>.
func TestReduceFile(t *testing.T) {
	ff, fp := tools(t)
	path := os.Getenv("TURBOMENU_MEDIA_FILE")
	if path == "" {
		t.Skip("define TURBOMENU_MEDIA_FILE")
	}
	in, err := Probe(context.Background(), fp, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", *in)
	for _, o := range []ReduceOptions{{TargetBytes: SizeDiscord}, {TargetBytes: 2 << 20}, {Quality: QualityHigh}, {Quality: QualityMedium}, {Quality: QualityLow}} {
		est := Estimate(in, o)
		res, err := Reduce(context.Background(), ff, in, o, logProgress(t))
		if err != nil {
			t.Logf("%+v: %v", o, err)
			continue
		}
		out, _ := Probe(context.Background(), fp, res.Path)
		t.Logf("%+v -> %.2f MB (estimado %.2f) %dx%d %.0ffps en %.1fs con %s", o, float64(res.Size)/(1<<20),
			float64(est)/(1<<20), out.Width, out.Height, out.FPS, res.Seconds, res.Encoder)
		if o.TargetBytes > 0 && res.Size > o.TargetBytes {
			t.Errorf("se pasa del objetivo")
		}
		os.Remove(res.Path)
	}
}

// Fuerza que el primer intento se pase del objetivo: el reintento debe dejarlo
// por debajo.
func TestReduceTargetRetry(t *testing.T) {
	ff, _ := tools(t)
	in := probe(t, "grande.mp4")
	old := targetMargin
	targetMargin = 1.25
	defer func() { targetMargin = old }()
	for _, enc := range []string{HardwareEncoder(context.Background(), ff), ""} {
		withEncoder(t, ff, enc)
		retried := false
		res, err := Reduce(context.Background(), ff, in, ReduceOptions{TargetBytes: 5 << 20}, func(p Progress) {
			if strings.HasPrefix(p.Stage, "Ajustando") {
				retried = true
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %.2f MB (objetivo 5 MB) en %.1fs, reintento=%v", res.Encoder, float64(res.Size)/(1<<20), res.Seconds, retried)
		if !retried || res.Size > 5<<20 {
			t.Errorf("reintento=%v tamaño=%d", retried, res.Size)
		}
		os.Remove(res.Path)
	}
}

// Si el codificador por hardware falla de verdad, se repite con x264 y se
// deja de usar.
func TestHardwareFallback(t *testing.T) {
	ff, _ := tools(t)
	in := probe(t, "grande.mp4")
	withEncoder(t, ff, "h264_amf") // en un PC sin AMD falla al abrirse
	res, err := Reduce(context.Background(), ff, in, ReduceOptions{Quality: QualityLow}, nil)
	if err != nil {
		t.Fatal(err, detail(err))
	}
	defer os.Remove(res.Path)
	hwMu.Lock()
	now := hwCache[hwKey(ff)]
	hwMu.Unlock()
	t.Logf("resultado con %s, caché ahora %q", res.Encoder, now)
	if res.Encoder != "libx264" || now != "" {
		t.Errorf("no cayó a x264")
	}
	os.Remove(hwCacheFile()) // que la siguiente prueba vuelva a detectar
}

func TestRarePaths(t *testing.T) {
	ff, fp := tools(t)
	dir := samples(t)
	// Acentos, eñes, % y espacios en el nombre.
	src := filepath.Join(dir, "vídeo ñ 100% (1).mkv")
	b, _ := os.ReadFile(filepath.Join(dir, "pista.mkv"))
	os.WriteFile(src, b, 0o644)
	in, err := Probe(context.Background(), fp, src)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Convert(context.Background(), ff, in, "mp4", ConvertOptions{}, nil)
	if err != nil {
		t.Fatal(err, detail(err))
	}
	t.Logf("-> %s", filepath.Base(res.Path))
	png := filepath.Join(dir, "foto %d 50% ñ.png")
	b, _ = os.ReadFile(filepath.Join(dir, "alfa.png"))
	os.WriteFile(png, b, 0o644)
	out := OutputPath(png, "", "webp")
	if err := EncodeImage(context.Background(), ff, png, out, "webp", 80, 0); err != nil {
		t.Fatal(err, detail(err))
	}
	t.Logf("-> %s (%d KB)", filepath.Base(out), fileSize(out)/1024)

	// Archivo dañado: mensaje claro.
	bad := filepath.Join(dir, "roto.mp4")
	os.WriteFile(bad, []byte("esto no es un vídeo"), 0o644)
	if _, err := Probe(context.Background(), fp, bad); err == nil {
		t.Error("debería fallar")
	} else {
		t.Logf("roto.mp4: %v", err)
	}
	if _, err := Probe(context.Background(), fp, filepath.Join(dir, "no-existe.mp4")); err != nil {
		t.Logf("no-existe.mp4: %v", err)
	}
}

// El Job mata a ffmpeg si TurboMENU muere: se lanza este binario de prueba
// como proceso aparte, arranca un ffmpeg largo y se le mata de golpe.
func TestJobKillsChildren(t *testing.T) {
	ff, _ := tools(t)
	if os.Getenv("TURBOMENU_JOB_CHILD") == "1" {
		cmd := command(context.Background(), ff, "-hide_banner", "-loglevel", "error", "-re",
			"-f", "lavfi", "-i", "testsrc2=s=320x240:r=30:d=60", "-f", "null", "-")
		if err := start(cmd); err != nil {
			t.Fatal(err)
		}
		os.Stdout.WriteString("PID " + itoa(cmd.Process.Pid) + "\n")
		cmd.Wait()
		return
	}
	cmd := command(context.Background(), os.Args[0], "-test.run=^TestJobKillsChildren$", "-test.v")
	cmd.Env = append(os.Environ(), "TURBOMENU_JOB_CHILD=1")
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := 0
	buf := make([]byte, 4096)
	var acc string
	deadline := time.Now().Add(20 * time.Second)
	for pid == 0 && time.Now().Before(deadline) {
		n, err := stdout.Read(buf)
		acc += string(buf[:n])
		if i := strings.Index(acc, "PID "); i >= 0 {
			rest := acc[i+4:]
			if j := strings.IndexByte(rest, '\n'); j >= 0 {
				pid = int(parseI(strings.TrimSpace(rest[:j])))
			}
		}
		if err != nil {
			break
		}
	}
	if pid == 0 {
		t.Fatalf("el hijo no arrancó ffmpeg: %q", acc)
	}
	if !processAlive(pid) {
		t.Fatal("ffmpeg no está vivo")
	}
	cmd.Process.Kill() // muerte brusca del «TurboMENU»
	cmd.Wait()
	for i := 0; i < 50 && processAlive(pid); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if processAlive(pid) {
		t.Fatal("ffmpeg siguió vivo tras morir el padre")
	}
	t.Logf("ffmpeg (pid %d) murió con su padre", pid)
}
