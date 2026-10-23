package media

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Target es un formato al que se puede convertir.
type Target struct {
	ID   string `json:"id"`   // "mp4", "gif", "mp3"…
	Ext  string `json:"ext"`  // extensión sin punto
	Name string `json:"name"` // nombre corto para el botón
	Desc string `json:"desc"` // para qué sirve, en una frase
	Kind string `json:"kind"` // "video" | "gif" | "audio" | "image"
}

var videoTargets = []Target{
	{"mp4", "mp4", "MP4", "MP4: se abre en cualquier móvil, PC o web", "video"},
	{"webm", "webm", "WEBM", "WEBM: ideal para webs, más ligero que MP4", "video"},
	{"mkv", "mkv", "MKV", "MKV: guarda varias pistas de audio y subtítulos", "video"},
	{"mov", "mov", "MOV", "MOV: el formato de Apple (iPhone, Mac, Final Cut)", "video"},
	{"avi", "avi", "AVI", "AVI: para reproductores y teles antiguas", "video"},
	{"gif", "gif", "GIF", "GIF: animación sin sonido para chats y foros", "gif"},
}

var audioTargets = []Target{
	{"mp3", "mp3", "MP3", "MP3: suena en cualquier sitio", "audio"},
	{"m4a", "m4a", "M4A", "M4A: buena calidad con poco peso (iPhone, iTunes)", "audio"},
	{"opus", "opus", "OPUS", "OPUS: el audio más eficiente, ideal para voz", "audio"},
	{"ogg", "ogg", "OGG", "OGG: audio libre, para juegos y Linux", "audio"},
	{"flac", "flac", "FLAC", "FLAC: sin pérdida de calidad, ocupa menos que WAV", "audio"},
	{"wav", "wav", "WAV", "WAV: sin comprimir, para editar o grabar en CD", "audio"},
}

var imageTargets = []Target{
	{"webp", "webp", "WEBP", "WEBP: imagen ligera para webs", "image"},
	{"avif", "avif", "AVIF", "AVIF: aún más ligera, para navegadores modernos", "image"},
}

// Targets lista los formatos a los que se puede convertir ese archivo (sin
// el que ya tiene). En vídeos incluye GIF y «solo audio».
func Targets(in *Info) []Target {
	if in == nil {
		return nil
	}
	var out []Target
	add := func(ts []Target, prefix string) {
		for _, t := range ts {
			if t.Ext == in.Container {
				continue
			}
			if prefix != "" {
				t.Name += " " + prefix
				t.Desc = strings.Replace(t.Desc, ": ", ": solo el sonido, ", 1)
			}
			out = append(out, t)
		}
	}
	switch in.Kind {
	case "video":
		add(videoTargets, "")
		if in.HasAudio {
			add(audioTargets, "(solo audio)")
		}
	case "audio":
		add(audioTargets, "")
	case "image":
		add(imageTargets, "")
	}
	return out
}

func findTarget(id string) (Target, bool) {
	for _, ts := range [][]Target{videoTargets, audioTargets, imageTargets} {
		for _, t := range ts {
			if t.ID == id {
				return t, true
			}
		}
	}
	return Target{}, false
}

// ConvertOptions ajusta Convert. Todo es opcional.
type ConvertOptions struct {
	Out     string  `json:"out"`     // ruta de salida; "" = mismo nombre con la nueva extensión
	Start   float64 `json:"start"`   // recortar: segundo de inicio
	Length  float64 `json:"length"`  // recortar: duración en segundos (0 = hasta el final)
	GIFFPS  int     `json:"gifFps"`  // fotogramas por segundo del GIF (0 = 15)
	GIFSize int     `json:"gifSize"` // lado mayor del GIF en píxeles (0 = 480)
	Quality int     `json:"quality"` // 1..100, para imágenes (0 = 80)
}

// Convert pasa el archivo al formato target (un ID de Targets). Copia sin
// recodificar cuando el formato lo permite (instantáneo).
func Convert(ctx context.Context, ffmpeg string, in *Info, target string, opt ConvertOptions, progress func(Progress)) (*Result, error) {
	if ffmpeg == "" {
		return nil, ErrNoFFmpeg
	}
	if in == nil {
		return nil, userErr("No se pudo leer el archivo.", nil)
	}
	t, ok := findTarget(target)
	if !ok {
		return nil, userErr("Formato no disponible.", nil)
	}
	t0 := time.Now()
	out := opt.Out
	if out == "" {
		out = OutputPath(in.Path, "", t.Ext)
	}
	if t.Kind == "image" {
		if err := EncodeImage(ctx, ffmpeg, in.Path, out, t.ID, opt.Quality, 0); err != nil {
			return nil, err
		}
		return &Result{Path: out, Size: fileSize(out), Encoder: t.ID, Seconds: time.Since(t0).Seconds()}, nil
	}
	if (t.Kind == "video" || t.Kind == "gif") && in.Kind != "video" {
		return nil, userErr("Este archivo no tiene vídeo.", nil)
	}
	if t.Kind == "audio" && !in.HasAudio {
		return nil, userErr("Este archivo no tiene sonido.", nil)
	}
	tmp := tempFor(out)
	defer os.Remove(tmp)

	// Recorte como opciones de entrada (antes de -i): salto rápido.
	dur := in.Duration
	var pre []string
	if opt.Start > 0 {
		pre = append(pre, "-ss", ftoa(opt.Start))
		dur = max(dur-opt.Start, 0)
	}
	if opt.Length > 0 {
		pre = append(pre, "-t", ftoa(opt.Length))
		if dur == 0 || opt.Length < dur {
			dur = opt.Length
		}
	}
	rep := newReporter(progress)
	var enc string
	var err error
	switch t.Kind {
	case "gif":
		enc, err = convertGIF(ctx, ffmpeg, in, opt, pre, dur, tmp, rep)
	case "audio":
		enc, err = convertAudio(ctx, ffmpeg, in, t, pre, dur, tmp, rep)
	default:
		enc, err = convertVideo(ctx, ffmpeg, in, t, pre, dur, tmp, rep)
	}
	if err != nil {
		return nil, err
	}
	sz := fileSize(tmp)
	p, err := finalize(tmp, out)
	if err != nil {
		return nil, err
	}
	return &Result{Path: p, Size: sz, Encoder: enc, Seconds: time.Since(t0).Seconds()}, nil
}

// Códecs que cada contenedor admite tal cual (sin recodificar).
var (
	mp4Video  = set("h264", "hevc", "av1", "mpeg4")
	movVideo  = set("h264", "hevc", "av1", "mpeg4", "prores", "mjpeg", "dnxhd")
	webmVideo = set("vp8", "vp9", "av1")
	aviVideo  = set("mpeg4", "msmpeg4v3", "mjpeg")
	mp4Audio  = set("aac", "mp3", "ac3", "eac3", "alac")
	movAudio  = set("aac", "mp3", "ac3", "eac3", "alac", "pcm_s16le", "pcm_s24le")
	webmAudio = set("opus", "vorbis")
	aviAudio  = set("mp3", "ac3", "pcm_s16le")
	mkvSubs   = set("subrip", "ass", "ssa", "hdmv_pgs_subtitle", "dvd_subtitle", "webvtt", "mov_text")
)

func convertVideo(ctx context.Context, ffmpeg string, in *Info, t Target, pre []string, dur float64, tmp string, rep *reporter) (string, error) {
	var vOK, aOK map[string]bool
	format := t.ID
	switch t.ID {
	case "mp4":
		vOK, aOK = mp4Video, mp4Audio
	case "mov":
		vOK, aOK = movVideo, movAudio
	case "webm":
		vOK, aOK = webmVideo, webmAudio
	case "avi":
		vOK, aOK = aviVideo, aviAudio
	case "mkv":
		format = "matroska"
	}
	copyAll := func(string) bool { return true }
	vcopy, acopy := copyAll, copyAll
	if vOK != nil {
		vcopy = func(c string) bool { return vOK[c] }
		acopy = func(c string) bool { return aOK[c] }
	}

	vi := firstStream(in, "video")
	vcodec := ""
	for _, s := range in.Streams {
		if s.Index == vi {
			vcodec = s.Codec
		}
	}
	copyVideo := vcopy(vcodec) && in.Container != "gif"

	a := append(append([]string{}, pre...), "-i", in.Path)
	a = append(a, "-map", "0:"+itoa(vi))
	auds := streamsOf(in, "audio")
	for _, s := range auds {
		a = append(a, "-map", "0:"+itoa(s.Index))
	}
	// Subtítulos: solo los que el destino admite.
	nsub := 0
	var subArgs []string
	for _, s := range streamsOf(in, "subtitle") {
		keep, codec := false, "copy"
		switch t.ID {
		case "mkv":
			keep = mkvSubs[s.Codec]
			if s.Codec == "mov_text" {
				codec = "srt"
			}
		case "mp4", "mov":
			keep = s.Codec == "mov_text"
		case "webm":
			keep = s.Codec == "webvtt"
		}
		if keep {
			a = append(a, "-map", "0:"+itoa(s.Index))
			subArgs = append(subArgs, "-c:s:"+itoa(nsub), codec)
			nsub++
		}
	}

	allAudioCopy := true
	for i, s := range auds {
		k := "-c:a:" + itoa(i)
		if acopy(s.Codec) {
			a = append(a, k, "copy")
			continue
		}
		allAudioCopy = false
		switch t.ID {
		case "webm":
			a = append(a, k, "libopus", "-b:a:"+itoa(i), "128k")
		case "avi":
			a = append(a, k, "libmp3lame", "-b:a:"+itoa(i), "192k")
		default:
			a = append(a, k, "aac", "-b:a:"+itoa(i), "192k")
		}
	}
	if !allAudioCopy && in.Channels > 2 && (t.ID == "webm" || t.ID == "avi") {
		a = append(a, "-ac", "2")
	}
	a = append(a, subArgs...)
	if t.ID == "mp4" || t.ID == "mov" {
		a = append(a, "-movflags", "+faststart")
	}
	a = append(a, "-map_metadata", "0")

	if copyVideo {
		rep.phase(0, 1, "Convirtiendo")
		args := append(a, "-c:v", "copy")
		if vcodec == "hevc" && (t.ID == "mp4" || t.ID == "mov") {
			args = append(args, "-tag:v", "hvc1")
		}
		args = append(args, "-f", format, tmp)
		return "copia", runFF(ctx, ffmpeg, args, dur, rep, "No se pudo convertir el vídeo.")
	}

	rep.phase(0, 1, "Convirtiendo")
	switch t.ID {
	case "webm":
		args := append(a, "-vf", videoFilter(in, vplan{}, "yuv420p"),
			"-c:v", "libvpx-vp9", "-crf", "32", "-b:v", "0", "-deadline", "good", "-cpu-used", "4",
			"-row-mt", "1", "-tile-columns", "2", "-f", "webm", tmp)
		return "libvpx-vp9", runFF(ctx, ffmpeg, args, dur, rep, "No se pudo convertir el vídeo.")
	case "avi":
		args := append(a, "-vf", videoFilter(in, vplan{}, "yuv420p"),
			"-c:v", "mpeg4", "-vtag", "xvid", "-q:v", "3", "-f", "avi", tmp)
		return "mpeg4", runFF(ctx, ffmpeg, args, dur, rep, "No se pudo convertir el vídeo.")
	}
	// mp4, mov y mkv: H.264 de buena calidad (por hardware si hay).
	p := vplan{crf: 20}
	return encodeH264(ctx, ffmpeg, dur, rep, func(enc string) []string {
		args := append(append([]string{}, a...), "-vf", videoFilter(in, p, pixFor(enc)))
		args = append(args, h264Args(enc, p)...)
		return append(args, "-f", format, tmp)
	})
}

func convertAudio(ctx context.Context, ffmpeg string, in *Info, t Target, pre []string, dur float64, tmp string, rep *reporter) (string, error) {
	a := append(append([]string{}, pre...), "-i", in.Path)
	a = append(a, "-map", "0:"+itoa(firstStream(in, "audio")), "-vn", "-sn", "-dn", "-map_metadata", "0")
	c := in.AudioCodec
	stereo := in.Channels > 2
	var enc, format string
	switch t.ID {
	case "mp3":
		format = "mp3"
		if c == "mp3" {
			enc = "copy"
		} else {
			enc = "libmp3lame"
			a = append(a, "-c:a", enc, "-q:a", "2")
		}
		a = append(a, "-id3v2_version", "3")
	case "m4a":
		format = "ipod"
		if c == "aac" || c == "alac" {
			enc = "copy"
		} else {
			enc = "aac"
			a = append(a, "-c:a", enc, "-b:a", "192k")
			stereo = false
		}
		a = append(a, "-movflags", "+faststart")
	case "wav":
		format, enc = "wav", "pcm_s16le"
		a = append(a, "-c:a", enc)
		stereo = false
	case "flac":
		format = "flac"
		if c == "flac" {
			enc = "copy"
		} else {
			enc = "flac"
			a = append(a, "-c:a", enc)
		}
		stereo = false
	case "ogg":
		format = "ogg"
		if c == "vorbis" || c == "opus" {
			enc = "copy"
		} else {
			enc = "libvorbis"
			a = append(a, "-c:a", enc, "-q:a", "5")
		}
	case "opus":
		format = "opus"
		if c == "opus" {
			enc = "copy"
		} else {
			enc = "libopus"
			a = append(a, "-c:a", enc, "-b:a", "128k")
		}
	}
	if enc == "copy" {
		a = append(a, "-c:a", "copy")
	} else if stereo {
		a = append(a, "-ac", "2")
	}
	a = append(a, "-f", format, tmp)
	if enc == "copy" {
		enc = "copia"
	}
	rep.phase(0, 1, "Convirtiendo")
	return enc, runFF(ctx, ffmpeg, a, dur, rep, "No se pudo convertir el audio.")
}

// convertGIF hace un GIF con paleta propia en dos pasos: la paleta sale solo
// de los fotogramas clave (casi gratis) y luego se escribe el GIF en
// streaming, con avance real y sin acumular fotogramas en memoria.
func convertGIF(ctx context.Context, ffmpeg string, in *Info, opt ConvertOptions, pre []string, dur float64, tmp string, rep *reporter) (string, error) {
	fps := opt.GIFFPS
	if fps <= 0 {
		fps = 15
	}
	if in.FPS > 0 && float64(fps) > in.FPS {
		fps = max(int(in.FPS+0.5), 1)
	}
	size := opt.GIFSize
	if size <= 0 {
		size = 480
	}
	w, h := fitLong(in.Width, in.Height, size)
	scale := "scale=" + itoa(w) + ":" + itoa(h) + ":flags=lanczos"
	if in.Width == 0 || in.Height == 0 {
		scale = "scale='min(iw," + itoa(size) + ")':'min(ih," + itoa(size) + ")':force_original_aspect_ratio=decrease:flags=lanczos"
	}
	if in.HDR {
		scale += ",zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709,tonemap=tonemap=hable:desat=0,zscale=t=bt709:m=bt709:r=tv"
	}
	vi := "0:" + itoa(firstStream(in, "video"))
	input := func(extra ...string) []string {
		return append(append(append([]string{}, extra...), pre...), "-i", in.Path)
	}

	pal := tmp + ".paleta.png"
	defer os.Remove(pal)
	rep.phase(0, 0.1, "Preparando colores")
	palArgs := func(extra ...string) []string {
		return append(input(extra...), "-map", vi, "-vf", scale+",palettegen=stats_mode=full",
			"-frames:v", "1", "-update", "1", "-f", "image2", pal)
	}
	err := runFF(ctx, ffmpeg, palArgs("-skip_frame", "nokey"), 0, rep, "No se pudo crear el GIF.")
	if err != nil && !IsCanceled(err) || err == nil && fileSize(pal) == 0 {
		// Sin fotogramas clave en el tramo: paleta con todos.
		err = runFF(ctx, ffmpeg, palArgs(), dur, rep, "No se pudo crear el GIF.")
	}
	if err != nil {
		return "", err
	}
	rep.phase(0.1, 0.9, "Creando GIF")
	a := append(input(), "-i", pal, "-filter_complex",
		"["+vi+"]fps="+itoa(fps)+","+scale+"[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle[o]",
		"-map", "[o]", "-loop", "0", "-f", "gif", tmp)
	return "gif", runFF(ctx, ffmpeg, a, dur, rep, "No se pudo crear el GIF.")
}

// --- utilidades ---

func set(v ...string) map[string]bool {
	m := make(map[string]bool, len(v))
	for _, s := range v {
		m[s] = true
	}
	return m
}

func itoa(i int) string { return strconv.Itoa(i) }

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', 3, 64) }

// copyFile copia in en dst (cancelable).
func copyFile(ctx context.Context, in, dst string) error {
	src, err := os.Open(in)
	if err != nil {
		return userErr("No se pudo leer el archivo.", err)
	}
	defer src.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return diskErr(err)
	}
	f, err := os.Create(dst)
	if err != nil {
		return diskErr(err)
	}
	_, err = io.Copy(f, readerCtx{ctx, src})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dst)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return diskErr(err)
	}
	return nil
}

type readerCtx struct {
	ctx context.Context
	r   io.Reader
}

func (r readerCtx) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
