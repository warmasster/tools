package media

import (
	"context"
	"math"
	"os"
	"time"
)

// Calidades para Reduce.
const (
	QualityHigh   = "alta"
	QualityMedium = "media"
	QualityLow    = "baja"
)

// Pesos objetivo habituales.
const (
	SizeDiscord  int64 = 10 << 20
	SizeWhatsApp int64 = 16 << 20
)

// ReduceOptions elige cómo reducir: por peso objetivo (TargetBytes > 0) o por
// calidad (Quality).
type ReduceOptions struct {
	TargetBytes int64  `json:"targetBytes"` // el resultado quedará por debajo de esto
	Quality     string `json:"quality"`     // QualityHigh | QualityMedium | QualityLow
	Out         string `json:"out"`         // ruta de salida; "" = «nombre (reducido).ext» junto al original
}

// Margen del primer intento en modo peso objetivo (variable para las pruebas).
var targetMargin = 0.955

// Sobrecoste del contenedor y mínimos en modo peso objetivo.
const (
	muxOverhead    = 24 << 10
	minVideoBps    = 80_000
	minAudioBpsAll = 24_000
)

// Reduce crea una copia más ligera del vídeo o audio (el original no se toca).
// Vídeo: MP4 H.264 + AAC. Audio: MP3 si ya lo era, si no M4A (AAC).
func Reduce(ctx context.Context, ffmpeg string, in *Info, opt ReduceOptions, progress func(Progress)) (*Result, error) {
	if ffmpeg == "" {
		return nil, ErrNoFFmpeg
	}
	if in == nil || (in.Kind != "video" && in.Kind != "audio") {
		return nil, userErr("Esta opción es para vídeos y audios.", nil)
	}
	if opt.TargetBytes > 0 && in.Duration <= 0 {
		return nil, userErr("No se sabe cuánto dura el archivo: prueba con una calidad en vez de un peso.", nil)
	}
	if opt.TargetBytes == 0 && opt.Quality == "" {
		opt.Quality = QualityMedium
	}
	t0 := time.Now()
	ext := "mp4"
	if in.Kind == "audio" {
		ext = "m4a"
		if in.AudioCodec == "mp3" {
			ext = "mp3"
		}
	}
	out := opt.Out
	if out == "" {
		out = OutputPath(in.Path, "reducido", ext)
	}
	rep := newReporter(progress)
	var res *Result
	var err error
	if in.Kind == "video" {
		res, err = reduceVideo(ctx, ffmpeg, in, opt, out, rep)
	} else {
		res, err = reduceAudio(ctx, ffmpeg, in, opt, out, ext, rep)
	}
	if err != nil {
		return nil, err
	}
	res.Seconds = time.Since(t0).Seconds()
	return res, nil
}

// --- vídeo ---

type videoJob struct {
	v     vplan
	abps  int64 // 0 = copiar el audio
	mono  bool
	noAud bool
}

func qualityCRF(q string) int {
	switch q {
	case QualityHigh:
		return 23
	case QualityLow:
		return 29
	}
	return 26
}

func qualityPlan(in *Info, q string) videoJob {
	j := videoJob{v: vplan{crf: qualityCRF(q)}}
	switch q {
	case QualityHigh:
		j.abps = 160_000
	case QualityLow:
		j.v.w, j.v.h = fitShort(in.Width, in.Height, 720)
		j.v.fps = 30
		j.abps = 96_000
	default:
		j.v.w, j.v.h = fitShort(in.Width, in.Height, 1080)
		j.abps = 128_000
	}
	j.noAud = !in.HasAudio
	// Si el audio ya es AAC ligero se copia tal cual (más rápido y sin pérdida).
	if in.AudioCodec == "aac" && in.AudioBitrate > 0 && in.AudioBitrate <= j.abps*11/10 && in.Channels <= 2 {
		j.abps = 0
	}
	return j
}

// Escalones de resolución (lado corto) para el modo peso objetivo.
var ladder = []int{2160, 1440, 1080, 720, 540, 480, 360, 270, 240}

// targetPlan reparte el peso objetivo entre vídeo y audio y elige la
// resolución más alta que se vea bien con ese bitrate.
func targetPlan(in *Info, target int64, factor float64) (videoJob, error) {
	j := videoJob{noAud: !in.HasAudio}
	total := (float64(target)*targetMargin - muxOverhead) * 8 / in.Duration * factor
	if !j.noAud {
		switch {
		case total >= 3_000_000:
			j.abps = 128_000
		case total >= 1_000_000:
			j.abps = 96_000
		case total >= 400_000:
			j.abps = 64_000
		default:
			j.abps, j.mono = 40_000, true
		}
	}
	vb := int64(total) - j.abps
	if vb < minVideoBps {
		return j, userErr("No cabe: el vídeo es demasiado largo para ese peso. Prueba con un peso mayor.", nil)
	}
	if in.VideoBitrate > 0 && vb > in.VideoBitrate {
		vb = in.VideoBitrate
	}
	j.v.bitrate = vb
	fps := in.FPS
	if fps <= 0 {
		fps = 30
	}
	// Por debajo de ~0,05 bits por píxel H.264 se emborrona: se baja la
	// resolución (y a 30 fps si iba a 60) hasta llegar a eso.
	const minBPP = 0.05
	srcShort := min(in.Width, in.Height)
	w, h := in.Width, in.Height
	if fps > 31 && float64(vb)/(float64(w*h)*fps) < minBPP {
		fps = 30
		j.v.fps = 30
	}
	for _, s := range ladder {
		if s > srcShort {
			continue
		}
		w, h = fitShort(in.Width, in.Height, s)
		if float64(vb)/(float64(w*h)*fps) >= minBPP {
			break
		}
	}
	if w != in.Width || h != in.Height {
		j.v.w, j.v.h = w, h
	}
	return j, nil
}

func videoArgs(in *Info, j videoJob, enc, tmp string) []string {
	a := []string{"-i", in.Path, "-map", "0:" + itoa(firstStream(in, "video"))}
	if !j.noAud {
		a = append(a, "-map", "0:"+itoa(firstStream(in, "audio")))
	}
	a = append(a, "-vf", videoFilter(in, j.v, pixFor(enc)))
	a = append(a, h264Args(enc, j.v)...)
	if !j.noAud {
		if j.abps == 0 {
			a = append(a, "-c:a", "copy")
		} else {
			a = append(a, "-c:a", "aac", "-b:a", kbps(j.abps))
			if j.mono {
				a = append(a, "-ac", "1")
			} else if in.Channels > 2 {
				a = append(a, "-ac", "2")
			}
		}
	}
	return append(a, "-map_chapters", "-1", "-movflags", "+faststart", "-f", "mp4", tmp)
}

func reduceVideo(ctx context.Context, ffmpeg string, in *Info, opt ReduceOptions, out string, rep *reporter) (*Result, error) {
	tmp := tempFor(out)
	defer os.Remove(tmp)

	// Si ya cabe y es H.264/AAC, basta con pasarlo a MP4 sin recodificar.
	if opt.TargetBytes > 0 && in.Size <= opt.TargetBytes && in.VideoCodec == "h264" &&
		(!in.HasAudio || in.AudioCodec == "aac" || in.AudioCodec == "mp3") {
		rep.phase(0, 1, "Copiando")
		args := []string{"-i", in.Path, "-map", "0:" + itoa(firstStream(in, "video"))}
		if in.HasAudio {
			args = append(args, "-map", "0:"+itoa(firstStream(in, "audio")))
		}
		args = append(args, "-c", "copy", "-map_chapters", "-1", "-movflags", "+faststart", "-f", "mp4", tmp)
		err := runFF(ctx, ffmpeg, args, in.Duration, rep, "No se pudo procesar el vídeo.")
		if IsCanceled(err) {
			return nil, err
		}
		if sz := fileSize(tmp); err == nil && sz > 0 && sz <= opt.TargetBytes {
			p, err := finalize(tmp, out)
			if err != nil {
				return nil, err
			}
			return &Result{Path: p, Size: sz, Encoder: "copia", Note: "Ya pesaba menos de lo pedido: se ha copiado sin perder calidad."}, nil
		}
	}

	if opt.TargetBytes <= 0 {
		j := qualityPlan(in, opt.Quality)
		rep.phase(0, 1, "Reduciendo")
		enc, err := encodeH264(ctx, ffmpeg, in.Duration, rep, func(enc string) []string { return videoArgs(in, j, enc, tmp) })
		if err != nil {
			return nil, err
		}
		sz := fileSize(tmp)
		if sz >= in.Size {
			return nil, ErrNotSmaller
		}
		p, err := finalize(tmp, out)
		if err != nil {
			return nil, err
		}
		return &Result{Path: p, Size: sz, Encoder: enc}, nil
	}

	// Peso objetivo: se codifica, se mide y, si se pasa, se repite con menos.
	factor := 1.0
	var enc string
	for attempt := 0; attempt < 3; attempt++ {
		j, err := targetPlan(in, opt.TargetBytes, factor)
		if err != nil {
			return nil, err
		}
		stage := "Reduciendo"
		if attempt > 0 {
			stage = "Ajustando para que quepa"
		}
		rep.phase(0, 1, stage)
		enc, err = encodeH264(ctx, ffmpeg, in.Duration, rep, func(enc string) []string { return videoArgs(in, j, enc, tmp) })
		if err != nil {
			return nil, err
		}
		sz := fileSize(tmp)
		if sz > 0 && sz <= opt.TargetBytes {
			p, err := finalize(tmp, out)
			if err != nil {
				return nil, err
			}
			return &Result{Path: p, Size: sz, Encoder: enc}, nil
		}
		// Lo que hay que quitar sale solo del vídeo (el audio pesa lo mismo).
		audio := float64(j.abps) * in.Duration / 8
		if j.abps == 0 && in.HasAudio {
			audio = float64(in.AudioBitrate) * in.Duration / 8
		}
		want := float64(opt.TargetBytes)*(0.93-0.03*float64(attempt)) - audio
		got := float64(sz) - audio
		if got <= 0 || want <= 0 {
			break
		}
		factor *= math.Min(want/got, 0.95)
	}
	return nil, userErr("No se ha conseguido que quepa en ese peso. Prueba con un peso mayor.", nil)
}

// --- audio ---

var mp3Rates = []int64{320, 256, 224, 192, 160, 128, 112, 96, 80, 64, 56, 48, 40, 32}

func audioQualityBps(q string) int64 {
	switch q {
	case QualityHigh:
		return 192_000
	case QualityLow:
		return 96_000
	}
	return 128_000
}

func snapMP3(bps int64) int64 {
	for _, r := range mp3Rates {
		if r*1000 <= bps {
			return r * 1000
		}
	}
	return 32_000
}

// audioBps decide el bitrate de audio para Reduce.
func audioBps(in *Info, opt ReduceOptions, mp3 bool, factor float64) (int64, error) {
	var b int64
	if opt.TargetBytes > 0 {
		b = int64((float64(opt.TargetBytes)*0.97 - 8<<10) * 8 / in.Duration * factor)
		if b < 32_000 {
			return 0, userErr("No cabe: el audio es demasiado largo para ese peso. Prueba con un peso mayor.", nil)
		}
		b = min(b, 320_000)
	} else {
		b = audioQualityBps(opt.Quality)
	}
	// Recodificar por encima del original solo engorda: se baja un escalón.
	if in.AudioBitrate > 0 && b > in.AudioBitrate*9/10 {
		b = max(in.AudioBitrate*3/4, 32_000)
	}
	if mp3 {
		b = snapMP3(b)
	}
	return b, nil
}

func reduceAudio(ctx context.Context, ffmpeg string, in *Info, opt ReduceOptions, out, ext string, rep *reporter) (*Result, error) {
	tmp := tempFor(out)
	defer os.Remove(tmp)
	mp3 := ext == "mp3"
	if opt.TargetBytes > 0 && in.Size <= opt.TargetBytes {
		if err := copyFile(ctx, in.Path, tmp); err != nil {
			return nil, err
		}
		p, err := finalize(tmp, out)
		if err != nil {
			return nil, err
		}
		return &Result{Path: p, Size: in.Size, Encoder: "copia", Note: "Ya pesaba menos de lo pedido: se ha copiado sin perder calidad."}, nil
	}
	factor := 1.0
	for attempt := 0; attempt < 3; attempt++ {
		b, err := audioBps(in, opt, mp3, factor)
		if err != nil {
			return nil, err
		}
		a := []string{"-i", in.Path, "-map", "0:" + itoa(firstStream(in, "audio")), "-vn", "-sn", "-dn"}
		enc := "aac"
		if mp3 {
			enc = "libmp3lame"
			a = append(a, "-c:a", "libmp3lame", "-b:a", kbps(b), "-id3v2_version", "3", "-f", "mp3", tmp)
		} else {
			if in.Channels > 2 {
				a = append(a, "-ac", "2")
			}
			a = append(a, "-c:a", "aac", "-b:a", kbps(b), "-movflags", "+faststart", "-f", "ipod", tmp)
		}
		stage := "Reduciendo"
		if attempt > 0 {
			stage = "Ajustando para que quepa"
		}
		rep.phase(0, 1, stage)
		if err := runFF(ctx, ffmpeg, a, in.Duration, rep, "No se pudo procesar el audio."); err != nil {
			return nil, err
		}
		sz := fileSize(tmp)
		if opt.TargetBytes <= 0 {
			if sz >= in.Size {
				return nil, ErrNotSmaller
			}
		} else if sz > opt.TargetBytes {
			factor *= math.Min(float64(opt.TargetBytes)*0.95/float64(sz), 0.95)
			continue
		}
		p, err := finalize(tmp, out)
		if err != nil {
			return nil, err
		}
		return &Result{Path: p, Size: sz, Encoder: enc}, nil
	}
	return nil, userErr("No se ha conseguido que quepa en ese peso. Prueba con un peso mayor.", nil)
}

// --- estimación ---

// Estimate calcula de forma aproximada cuánto pesará Reduce con esas
// opciones (en bytes), para enseñarlo junto a cada botón. Es instantáneo.
func Estimate(in *Info, opt ReduceOptions) int64 {
	if in == nil || in.Duration <= 0 {
		return 0
	}
	if opt.TargetBytes > 0 {
		if in.Size <= opt.TargetBytes {
			return in.Size
		}
		return int64(float64(opt.TargetBytes) * 0.93)
	}
	if opt.Quality == "" {
		opt.Quality = QualityMedium
	}
	if in.Kind == "audio" {
		b, _ := audioBps(in, opt, in.AudioCodec == "mp3", 1)
		return int64(float64(b)*in.Duration/8) + 16<<10
	}
	if in.Kind != "video" {
		return 0
	}
	j := qualityPlan(in, opt.Quality)
	w, h := j.v.w, j.v.h
	if w == 0 {
		w, h = in.Width, in.Height
	}
	fps := in.FPS
	if fps <= 0 {
		fps = 30
	}
	if j.v.fps > 0 {
		fps = min(fps, j.v.fps)
	}
	// Modelo sencillo de x264 a calidad constante: ~3,5 Mb/s en 1080p30 con
	// CRF 23; la mitad cada 6 puntos de CRF.
	vb := 8.3 * math.Pow(float64(w*h), 0.75) * math.Pow(fps, 0.6) * math.Pow(2, -float64(j.v.crf-23)/6)
	if in.VideoBitrate > 0 {
		// Nunca mucho más que lo que ya tenía (escalado a la nueva resolución).
		scale := float64(w*h) / float64(max(in.Width*in.Height, 1))
		vb = math.Min(vb, float64(in.VideoBitrate)*math.Min(scale, 1)*0.9)
	}
	ab := float64(j.abps)
	if j.abps == 0 {
		ab = float64(in.AudioBitrate)
	}
	if j.noAud {
		ab = 0
	}
	est := int64((vb+ab)*in.Duration/8) + 32<<10
	return min(est, in.Size)
}

// EstimateSampled afina Estimate en vídeos por calidad codificando de verdad
// unos segundos del centro (≈0,5-1 s con GPU) y extrapolando. En el resto de
// casos devuelve Estimate. Pensado para actualizar la cifra tras mostrar la
// rápida.
func EstimateSampled(ctx context.Context, ffmpeg string, in *Info, opt ReduceOptions) (int64, error) {
	quick := Estimate(in, opt)
	if ffmpeg == "" || in == nil || in.Kind != "video" || opt.TargetBytes > 0 || in.Duration <= 0 {
		return quick, nil
	}
	if opt.Quality == "" {
		opt.Quality = QualityMedium
	}
	j := qualityPlan(in, opt.Quality)
	const sample = 4.0
	start, length := 0.0, in.Duration
	if in.Duration > 2*sample {
		start, length = in.Duration*0.4, sample
	}
	f, err := os.CreateTemp("", "turbomenu-muestra-*.mp4")
	if err != nil {
		return quick, nil
	}
	tmp := f.Name()
	f.Close()
	defer os.Remove(tmp)
	_, err = encodeH264(ctx, ffmpeg, length, nil, func(enc string) []string {
		a := videoArgs(in, j, enc, tmp)
		return append([]string{"-ss", ftoa(start), "-t", ftoa(length)}, a...)
	})
	if err != nil {
		if IsCanceled(err) {
			return 0, err
		}
		return quick, nil
	}
	sz := fileSize(tmp)
	if sz <= 0 {
		return quick, nil
	}
	est := int64(float64(sz) * in.Duration / length)
	return min(est, in.Size), nil
}
