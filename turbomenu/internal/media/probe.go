package media

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Info describe un archivo multimedia. Width/Height son el tamaño tal como se
// ve (ya girado según Rotation).
type Info struct {
	Path         string   `json:"path"`
	Kind         string   `json:"kind"`      // "video" | "audio" | "image"
	Container    string   `json:"container"` // extensión en minúsculas, sin punto ("mp4", "mkv"…)
	Format       string   `json:"format"`    // nombre de formato de ffprobe ("mov,mp4,m4a,…")
	Size         int64    `json:"size"`      // bytes
	Duration     float64  `json:"duration"`  // segundos (0 en imágenes)
	Width        int      `json:"width"`
	Height       int      `json:"height"`
	FPS          float64  `json:"fps"`
	Rotation     int      `json:"rotation"` // grados (0, 90, 180, 270)
	VideoCodec   string   `json:"videoCodec"`
	AudioCodec   string   `json:"audioCodec"`
	PixFmt       string   `json:"pixFmt"`
	HDR          bool     `json:"hdr"`
	Alpha        bool     `json:"alpha"` // imagen o vídeo con transparencia
	HasAudio     bool     `json:"hasAudio"`
	Channels     int      `json:"channels"`
	SampleRate   int      `json:"sampleRate"`
	Bitrate      int64    `json:"bitrate"` // total, bits/s
	VideoBitrate int64    `json:"videoBitrate"`
	AudioBitrate int64    `json:"audioBitrate"`
	Streams      []Stream `json:"streams"`
}

// Stream es una pista del archivo.
type Stream struct {
	Index       int    `json:"index"`
	Type        string `json:"type"` // "video" | "audio" | "subtitle" | "attachment" | "data"
	Codec       string `json:"codec"`
	AttachedPic bool   `json:"attachedPic"` // carátula (no es vídeo de verdad)
}

type probeJSON struct {
	Streams []struct {
		Index         int               `json:"index"`
		CodecType     string            `json:"codec_type"`
		CodecName     string            `json:"codec_name"`
		Width         int               `json:"width"`
		Height        int               `json:"height"`
		PixFmt        string            `json:"pix_fmt"`
		AvgFrameRate  string            `json:"avg_frame_rate"`
		RFrameRate    string            `json:"r_frame_rate"`
		Duration      string            `json:"duration"`
		BitRate       string            `json:"bit_rate"`
		NbFrames      string            `json:"nb_frames"`
		Channels      int               `json:"channels"`
		SampleRate    string            `json:"sample_rate"`
		ColorTransfer string            `json:"color_transfer"`
		Disposition   map[string]int    `json:"disposition"`
		Tags          map[string]string `json:"tags"`
		SideData      []struct {
			Rotation *float64 `json:"rotation"`
		} `json:"side_data_list"`
	} `json:"streams"`
	Format struct {
		FormatName string            `json:"format_name"`
		Duration   string            `json:"duration"`
		Size       string            `json:"size"`
		BitRate    string            `json:"bit_rate"`
		Tags       map[string]string `json:"tags"`
	} `json:"format"`
}

// Probe analiza un archivo con ffprobe (sin decodificar, tarda unas décimas).
func Probe(ctx context.Context, ffprobe, path string) (*Info, error) {
	if ffprobe == "" {
		return nil, ErrNoFFmpeg
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, userErr("No se encuentra el archivo.", err)
	}
	out, errLog, err := output(ctx, ffprobe, "-v", "error", "-hide_banner",
		"-show_format", "-show_streams", "-of", "json", "-i", path)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, ffmpegErr(errLog, "No se pudo leer el archivo: puede estar dañado o no ser vídeo, audio ni imagen.")
	}
	var pj probeJSON
	if err := json.Unmarshal(out, &pj); err != nil {
		return nil, userErr("No se pudo leer el archivo.", err)
	}
	return buildInfo(path, st.Size(), &pj)
}

func buildInfo(path string, size int64, pj *probeJSON) (*Info, error) {
	in := &Info{
		Path:      path,
		Size:      size,
		Format:    pj.Format.FormatName,
		Container: strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."),
		Duration:  parseF(pj.Format.Duration),
		Bitrate:   parseI(pj.Format.BitRate),
	}
	videoFrames := int64(-1)
	for _, s := range pj.Streams {
		st := Stream{Index: s.Index, Type: s.CodecType, Codec: s.CodecName, AttachedPic: s.Disposition["attached_pic"] == 1}
		in.Streams = append(in.Streams, st)
		switch s.CodecType {
		case "video":
			if st.AttachedPic || in.VideoCodec != "" {
				continue
			}
			in.VideoCodec = s.CodecName
			in.Width, in.Height = s.Width, s.Height
			in.PixFmt = s.PixFmt
			in.FPS = parseRate(s.AvgFrameRate)
			if in.FPS <= 0 || in.FPS > 240 {
				in.FPS = parseRate(s.RFrameRate)
			}
			in.VideoBitrate = streamBitrate(s.BitRate, s.Tags)
			in.HDR = s.ColorTransfer == "smpte2084" || s.ColorTransfer == "arib-std-b67"
			in.Alpha = hasAlpha(s.PixFmt)
			if s.NbFrames != "" {
				videoFrames = parseI(s.NbFrames)
			}
			rot := 0.0
			for _, sd := range s.SideData {
				if sd.Rotation != nil {
					rot = *sd.Rotation
				}
			}
			if rot == 0 {
				rot = parseF(s.Tags["rotate"])
			}
			r := ((int(math.Round(rot/90)) * 90 % 360) + 360) % 360
			in.Rotation = r
			if r == 90 || r == 270 {
				in.Width, in.Height = in.Height, in.Width
			}
			if in.Duration == 0 {
				in.Duration = parseF(s.Duration)
			}
		case "audio":
			if in.HasAudio {
				continue
			}
			in.HasAudio = true
			in.AudioCodec = s.CodecName
			in.Channels = s.Channels
			in.SampleRate = int(parseI(s.SampleRate))
			in.AudioBitrate = streamBitrate(s.BitRate, s.Tags)
			if in.Duration == 0 {
				in.Duration = parseF(s.Duration)
			}
		}
	}

	f := pj.Format.FormatName
	brand := strings.ToLower(pj.Format.Tags["major_brand"])
	switch {
	case in.VideoCodec != "" && (strings.HasSuffix(f, "_pipe") || f == "image2" ||
		brand == "avif" || brand == "heic" || brand == "heix" || brand == "mif1" ||
		(videoFrames == 1 && !in.HasAudio) || (in.Duration == 0 && !in.HasAudio && imageCodecs[in.VideoCodec])):
		in.Kind = "image"
		in.Duration, in.FPS = 0, 0
	case in.VideoCodec != "":
		in.Kind = "video"
	case in.HasAudio:
		in.Kind = "audio"
	default:
		return nil, userErr("Este archivo no tiene vídeo ni audio.", nil)
	}

	if in.Bitrate == 0 && in.Duration > 0 {
		in.Bitrate = int64(float64(size*8) / in.Duration)
	}
	if in.Kind == "video" && in.VideoBitrate == 0 && in.Bitrate > 0 {
		in.VideoBitrate = max(in.Bitrate-in.AudioBitrate, in.Bitrate*7/10)
	}
	if in.Kind == "audio" && in.AudioBitrate == 0 {
		in.AudioBitrate = in.Bitrate
	}
	if in.HasAudio && in.AudioBitrate == 0 && in.Kind == "video" {
		in.AudioBitrate = 128000
	}
	return in, nil
}

var imageCodecs = map[string]bool{"png": true, "mjpeg": true, "webp": true, "bmp": true, "tiff": true,
	"gif": true, "jpegxl": true, "av1": true, "hevc": true, "jpeg2000": true, "qoi": true}

// streamBitrate lee bit_rate o, en MKV, la etiqueta BPS.
func streamBitrate(v string, tags map[string]string) int64 {
	if b := parseI(v); b > 0 {
		return b
	}
	for k, t := range tags {
		if strings.HasPrefix(strings.ToUpper(k), "BPS") {
			if b := parseI(t); b > 0 {
				return b
			}
		}
	}
	return 0
}

func hasAlpha(pf string) bool {
	switch {
	case strings.HasPrefix(pf, "yuva"), strings.HasPrefix(pf, "rgba"), strings.HasPrefix(pf, "bgra"),
		strings.HasPrefix(pf, "argb"), strings.HasPrefix(pf, "abgr"), strings.HasPrefix(pf, "ya"),
		strings.HasPrefix(pf, "gbrap"), pf == "pal8":
		return true
	}
	return false
}

func parseRate(s string) float64 {
	n, d, ok := strings.Cut(s, "/")
	if !ok {
		return parseF(s)
	}
	a, b := parseF(n), parseF(d)
	if b == 0 {
		return 0
	}
	return a / b
}

func parseF(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

func parseI(s string) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return int64(parseF(s))
	}
	return v
}
