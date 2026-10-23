package media

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// EncodeImage convierte una imagen a "webp" o "avif" con ffmpeg (lee también
// HEIC, AVIF y WEBP de entrada). quality va de 1 a 100 (0 = 80); maxSide
// limita el lado mayor en píxeles (0 = sin cambiar, nunca amplía). Conserva
// la transparencia. Escribe en un provisional y renombra al terminar.
func EncodeImage(ctx context.Context, ffmpeg, in, out, format string, quality int, maxSide int) error {
	if ffmpeg == "" {
		return ErrNoFFmpeg
	}
	if quality <= 0 || quality > 100 {
		quality = 80
	}
	format = strings.ToLower(strings.TrimPrefix(format, "."))
	var vf []string
	if maxSide > 0 {
		s := itoa(maxSide)
		vf = append(vf, "scale='min(iw,"+s+")':'min(ih,"+s+")':force_original_aspect_ratio=decrease:flags=lanczos")
	}
	a := []string{"-i", in, "-map", "0:v:0", "-frames:v", "1", "-map_metadata", "-1"}
	tmp := tempFor(out)
	defer os.Remove(tmp)

	switch format {
	case "webp":
		if len(vf) > 0 {
			a = append(a, "-vf", strings.Join(vf, ","))
		}
		a = append(a, "-c:v", "libwebp", "-quality", itoa(quality), "-compression_level", "4",
			"-preset", "picture", "-f", "webp", tmp)
	case "avif":
		// AV1 en modo imagen fija; calidad 80 ≈ CRF 20, 50 ≈ CRF 35.
		crf := itoa(min(max((100-quality)/2+10, 0), 63))
		codec := []string{"-c:v", "libaom-av1", "-still-picture", "1", "-usage", "allintra",
			"-cpu-used", "6", "-row-mt", "1", "-crf", crf}
		chain := strings.Join(vf, ",")
		if imageHasAlpha(ctx, ffmpeg, in) {
			// AVIF guarda la transparencia como una segunda imagen en gris.
			pre := "[0:v:0]"
			if chain != "" {
				pre += chain + ","
			}
			a = []string{"-i", in, "-frames:v", "1", "-map_metadata", "-1", "-filter_complex",
				pre + "format=yuva444p,split[c][a];[c]format=yuv420p[c2];[a]alphaextract,format=gray[a2]",
				"-map", "[c2]", "-map", "[a2]"}
		} else {
			if chain != "" {
				chain += ","
			}
			a = append(a, "-vf", chain+"format=yuv420p")
		}
		a = append(a, codec...)
		a = append(a, "-f", "avif", tmp)
	default:
		return userErr("Formato de imagen no disponible.", nil)
	}
	if err := runFF(ctx, ffmpeg, a, 0, nil, "No se pudo convertir la imagen."); err != nil {
		return err
	}
	if _, err := finalizeExact(tmp, out); err != nil {
		return err
	}
	return nil
}

// finalizeExact renombra al nombre pedido (el llamador eligió out).
func finalizeExact(tmp, out string) (string, error) {
	if err := os.Rename(tmp, out); err != nil {
		os.Remove(tmp)
		return "", diskErr(err)
	}
	return out, nil
}

// imageHasAlpha mira con el ffprobe de al lado si la imagen tiene transparencia.
func imageHasAlpha(ctx context.Context, ffmpeg, in string) bool {
	probe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe.exe")
	if !isFile(probe) {
		return false
	}
	out, _, err := output(ctx, probe, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=pix_fmt", "-of", "csv=p=0", in)
	return err == nil && hasAlpha(strings.TrimSpace(string(out)))
}
