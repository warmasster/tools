package media

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Progress es el avance de una operación larga.
type Progress struct {
	Fraction float64 `json:"fraction"` // 0..1
	Speed    float64 `json:"speed"`    // veces el tiempo real (2 = el doble de rápido que verlo)
	ETA      float64 `json:"eta"`      // segundos restantes estimados; -1 si aún no se sabe
	Stage    string  `json:"stage"`    // qué se está haciendo, para enseñarlo tal cual
}

// Result es lo que deja una operación terminada.
type Result struct {
	Path    string  `json:"path"`
	Size    int64   `json:"size"`
	Encoder string  `json:"encoder"` // "h264_nvenc", "libx264", "copia"…
	Seconds float64 `json:"seconds"` // lo que tardó
	Note    string  `json:"note"`    // aviso opcional para el usuario
}

// reporter traduce el avance de cada fase (0..1) a un avance global con ETA.
type reporter struct {
	fn         func(Progress)
	base, span float64
	stage      string
	phaseStart time.Time
	eta        float64
}

func newReporter(fn func(Progress)) *reporter {
	return &reporter{fn: fn, span: 1, eta: -1, phaseStart: time.Now()}
}

// phase empieza una fase que ocupa [base, base+span] del total.
func (r *reporter) phase(base, span float64, stage string) {
	if r == nil {
		return
	}
	r.base, r.span, r.stage = base, span, stage
	r.phaseStart = time.Now()
	r.eta = -1
	r.send(0, 0)
}

func (r *reporter) send(local, speed float64) {
	if r == nil || r.fn == nil {
		return
	}
	local = min(max(local, 0), 1)
	g := r.base + r.span*local
	el := time.Since(r.phaseStart).Seconds()
	if local > 0.01 && el > 0.4 {
		// Segundos por unidad de avance medidos en esta fase.
		est := el / (r.span * local) * (1 - g)
		if r.eta < 0 {
			r.eta = est
		} else {
			r.eta = 0.6*r.eta + 0.4*est
		}
	}
	eta := r.eta
	if local >= 1 && g >= 0.999 {
		eta = 0
	}
	r.fn(Progress{Fraction: g, Speed: speed, ETA: eta, Stage: r.stage})
}

// ffmpegArgsBase son las opciones comunes: sin banner, sin preguntar y con
// el progreso por la salida estándar.
var ffmpegArgsBase = []string{"-hide_banner", "-nostdin", "-y", "-loglevel", "error",
	"-nostats", "-progress", "pipe:1", "-stats_period", "0.4"}

// runFF ejecuta ffmpeg; dur es la duración de lo que se codifica (para el
// avance). Si se cancela devuelve ctx.Err().
func runFF(ctx context.Context, ffmpeg string, args []string, dur float64, rep *reporter, fallback string) error {
	full := append(append([]string{}, ffmpegArgsBase...), args...)
	cmd := command(ctx, ffmpeg, full...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return userErr(fallback, err)
	}
	errBuf := &tailBuffer{max: maxStderrCaptured}
	cmd.Stderr = errBuf
	if err := start(cmd); err != nil {
		return userErr("No se pudo abrir FFmpeg.", err)
	}
	sc := bufio.NewScanner(stdout)
	var outUs int64
	var speed float64
	t0 := time.Now()
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		switch k {
		case "out_time_us":
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
				outUs = n
			}
		case "speed":
			speed = parseF(strings.TrimSuffix(strings.TrimSpace(v), "x"))
		case "progress":
			sec := float64(outUs) / 1e6
			if speed <= 0 {
				if el := time.Since(t0).Seconds(); el > 0 {
					speed = sec / el
				}
			}
			frac := 0.0
			if dur > 0 {
				frac = min(sec/dur, 0.999)
			}
			if v == "end" {
				frac = 1
			}
			rep.send(frac, speed)
		}
	}
	err = cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return ffmpegErr(errBuf.Bytes(), fallback)
	}
	return nil
}

// OutputPath da una ruta junto a in que no pisa nada: «video (reducido).mp4»,
// «video (reducido 2).mp4»… Con suffix vacío: «video.webm», «video (2).webm».
func OutputPath(in, suffix, ext string) string {
	ext = strings.TrimPrefix(ext, ".")
	dir := filepath.Dir(in)
	base := strings.TrimSuffix(filepath.Base(in), filepath.Ext(in))
	for i := 1; ; i++ {
		var name string
		switch {
		case suffix == "" && i == 1:
			name = base
		case suffix == "":
			name = fmt.Sprintf("%s (%d)", base, i)
		case i == 1:
			name = fmt.Sprintf("%s (%s)", base, suffix)
		default:
			name = fmt.Sprintf("%s (%s %d)", base, suffix, i)
		}
		p := filepath.Join(dir, name+"."+ext)
		if strings.EqualFold(p, in) {
			continue
		}
		if !exists(p) && !exists(p+".part") {
			return p
		}
	}
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// tempFor es el archivo provisional donde escribe ffmpeg antes de renombrar.
func tempFor(out string) string { return out + ".part" }

// finalize renombra el provisional a su nombre final. Si mientras tanto ha
// aparecido un archivo con ese nombre, busca otro libre.
func finalize(tmp, out string) (string, error) {
	if exists(out) {
		ext := filepath.Ext(out)
		out = OutputPath(strings.TrimSuffix(out, ext)+".x", "", ext)
	}
	if err := os.Rename(tmp, out); err != nil {
		os.Remove(tmp)
		return "", diskErr(err)
	}
	return out, nil
}

func fileSize(p string) int64 {
	st, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return st.Size()
}
