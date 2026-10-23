package media

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// vplan es cómo recodificar el vídeo.
type vplan struct {
	w, h    int     // 0 = sin cambiar
	fps     float64 // 0 = sin cambiar
	crf     int     // calidad constante (escala x264) si bitrate == 0
	bitrate int64   // bits/s para modo peso objetivo
}

// h264Args devuelve las opciones del codificador H.264 elegido.
func h264Args(enc string, p vplan) []string {
	if p.bitrate > 0 && enc != "libx264" {
		// Los codificadores por hardware tienden a pasarse un poco del bitrate.
		p.bitrate = p.bitrate * 96 / 100
	}
	b := strconv.FormatInt(p.bitrate, 10)
	maxr := func(f float64) string { return strconv.FormatInt(int64(float64(p.bitrate)*f), 10) }
	switch enc {
	case "h264_nvenc":
		a := []string{"-c:v", "h264_nvenc", "-preset", "p5", "-tune", "hq", "-profile:v", "high",
			"-spatial-aq", "1", "-rc-lookahead", "20", "-bf", "3", "-b_ref_mode", "middle"}
		if p.bitrate > 0 {
			return append(a, "-rc", "vbr", "-multipass", "qres", "-b:v", b, "-maxrate", maxr(1.5), "-bufsize", maxr(2))
		}
		return append(a, "-rc", "vbr", "-cq", strconv.Itoa(p.crf+1), "-b:v", "0")
	case "h264_qsv":
		a := []string{"-c:v", "h264_qsv", "-preset", "medium", "-profile:v", "high"}
		if p.bitrate > 0 {
			return append(a, "-b:v", b, "-maxrate", maxr(1.5), "-bufsize", maxr(2))
		}
		return append(a, "-global_quality", strconv.Itoa(p.crf))
	case "h264_amf":
		a := []string{"-c:v", "h264_amf", "-quality", "balanced", "-profile:v", "high"}
		if p.bitrate > 0 {
			return append(a, "-rc", "vbr_peak", "-b:v", b, "-maxrate", maxr(1.5), "-bufsize", maxr(2))
		}
		q := strconv.Itoa(p.crf)
		return append(a, "-rc", "cqp", "-qp_i", q, "-qp_p", q, "-qp_b", q)
	}
	a := []string{"-c:v", "libx264", "-preset", "faster", "-profile:v", "high"}
	if p.bitrate > 0 {
		return append(a, "-b:v", b, "-maxrate", maxr(2), "-bufsize", maxr(4))
	}
	return append(a, "-crf", strconv.Itoa(p.crf))
}

// videoFilter monta el filtro: escalar, limitar fps, pasar HDR a SDR y dejar
// el formato de píxel compatible (4:2:0 de 8 bits).
func videoFilter(in *Info, p vplan, pix string) string {
	var f []string
	// Primero los fps: así se escalan menos fotogramas.
	if p.fps > 0 && in.FPS > p.fps+0.5 {
		f = append(f, "fps="+strconv.FormatFloat(p.fps, 'f', -1, 64))
	}
	w, h := p.w, p.h
	if w == 0 || h == 0 {
		w, h = in.Width, in.Height
	}
	if w > 0 && h > 0 {
		w, h = even(w), even(h)
		if w != in.Width || h != in.Height {
			f = append(f, fmt.Sprintf("scale=%d:%d:flags=bicubic", w, h))
		}
	} else {
		f = append(f, "scale=trunc(iw/2)*2:trunc(ih/2)*2")
	}
	if in.HDR {
		f = append(f, "zscale=t=linear:npl=100", "format=gbrpf32le", "zscale=p=bt709",
			"tonemap=tonemap=hable:desat=0", "zscale=t=bt709:m=bt709:r=tv")
	}
	f = append(f, "format="+pix)
	return strings.Join(f, ",")
}

func pixFor(enc string) string {
	if enc == "h264_qsv" {
		return "nv12"
	}
	return "yuv420p"
}

func even(n int) int {
	if n < 2 {
		return 2
	}
	return n &^ 1
}

// fitShort escala (w,h) para que el lado corto no pase de short.
func fitShort(w, h, short int) (int, int) {
	s := min(w, h)
	if short <= 0 || s <= short || s == 0 {
		return w, h
	}
	f := float64(short) / float64(s)
	return even(int(math.Round(float64(w) * f))), even(int(math.Round(float64(h) * f)))
}

// fitLong escala (w,h) para que el lado largo no pase de long.
func fitLong(w, h, long int) (int, int) {
	l := max(w, h)
	if long <= 0 || l <= long || l == 0 {
		return w, h
	}
	f := float64(long) / float64(l)
	return max(int(math.Round(float64(w)*f)), 1), max(int(math.Round(float64(h)*f)), 1)
}

func firstStream(in *Info, typ string) int {
	for _, s := range in.Streams {
		if s.Type == typ && !s.AttachedPic {
			return s.Index
		}
	}
	return -1
}

func streamsOf(in *Info, typ string) []Stream {
	var out []Stream
	for _, s := range in.Streams {
		if s.Type == typ && !s.AttachedPic {
			out = append(out, s)
		}
	}
	return out
}

// encodeH264 ejecuta una codificación de vídeo con el codificador por
// hardware si lo hay; si falla (y no se ha cancelado), repite con x264, y si
// x264 sí funciona se deja de usar el hardware. build recibe el codificador y
// devuelve los argumentos completos.
func encodeH264(ctx context.Context, ffmpeg string, dur float64, rep *reporter,
	build func(enc string) []string) (string, error) {
	enc := HardwareEncoder(ctx, ffmpeg)
	if enc != "" {
		err := runFF(ctx, ffmpeg, build(enc), dur, rep, "No se pudo procesar el vídeo.")
		if err == nil || IsCanceled(err) {
			return enc, err
		}
		if rep != nil {
			rep.phase(rep.base, rep.span, rep.stage)
		}
		if err := runFF(ctx, ffmpeg, build("libx264"), dur, rep, "No se pudo procesar el vídeo."); err != nil {
			return "libx264", err
		}
		forgetHardware(ffmpeg)
		return "libx264", nil
	}
	return "libx264", runFF(ctx, ffmpeg, build("libx264"), dur, rep, "No se pudo procesar el vídeo.")
}

func kbps(b int64) string { return strconv.FormatInt(b/1000, 10) + "k" }
