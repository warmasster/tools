package media

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CacheDir es donde se guarda qué codificador por hardware funciona, para no
// volver a probarlo en cada ventana. Vacío: %LOCALAPPDATA%\TurboMENU.
var CacheDir string

// Orden de preferencia: NVIDIA, Intel, AMD.
var hwEncoders = []string{"h264_nvenc", "h264_qsv", "h264_amf"}

var (
	hwMu    sync.Mutex
	hwCache = map[string]string{} // clave del ffmpeg -> codificador ("" = ninguno)
)

// HardwareEncoder devuelve el codificador H.264 por hardware que funciona en
// este PC ("h264_nvenc", "h264_qsv", "h264_amf") o "" si no hay. La primera vez
// prueba de verdad un fotograma con cada uno (en paralelo, ~0,4 s); después lo
// recuerda en memoria y en disco. Conviene llamarlo al abrir la ventana.
func HardwareEncoder(ctx context.Context, ffmpeg string) string {
	if ffmpeg == "" {
		return ""
	}
	key := hwKey(ffmpeg)
	hwMu.Lock()
	defer hwMu.Unlock()
	if enc, ok := hwCache[key]; ok {
		return enc
	}
	disk := loadHWCache()
	if enc, ok := disk[key]; ok {
		hwCache[key] = enc
		return enc
	}
	ok := make([]bool, len(hwEncoders))
	var wg sync.WaitGroup
	for i, enc := range hwEncoders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok[i] = testEncoder(ctx, ffmpeg, enc)
		}()
	}
	wg.Wait()
	if ctx.Err() != nil {
		return ""
	}
	found := ""
	for i, enc := range hwEncoders {
		if ok[i] {
			found = enc
			break
		}
	}
	hwCache[key] = found
	disk[key] = found
	saveHWCache(disk)
	return found
}

// forgetHardware se usa cuando el codificador por hardware falla en un
// trabajo real (driver cambiado, GPU ocupada…): a partir de ahí, x264.
func forgetHardware(ffmpeg string) {
	key := hwKey(ffmpeg)
	hwMu.Lock()
	defer hwMu.Unlock()
	hwCache[key] = ""
	disk := loadHWCache()
	disk[key] = ""
	saveHWCache(disk)
}

func testEncoder(ctx context.Context, ffmpeg, enc string) bool {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	pix := "yuv420p"
	if enc == "h264_qsv" {
		pix = "nv12"
	}
	// Con las mismas opciones que se usan de verdad (algunas GPU antiguas no
	// admiten todas).
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=black:s=640x360:r=30", "-frames:v", "2", "-pix_fmt", pix}
	args = append(args, h264Args(enc, vplan{crf: 23})...)
	_, _, err := output(ctx, ffmpeg, append(args, "-f", "null", "-")...)
	return err == nil
}

// hwKey identifica el ffmpeg (ruta, tamaño y fecha): si se actualiza, se
// vuelve a probar.
func hwKey(ffmpeg string) string {
	st, err := os.Stat(ffmpeg)
	if err != nil {
		return ffmpeg
	}
	return fmt.Sprintf("%s|%d|%d", ffmpeg, st.Size(), st.ModTime().Unix())
}

func hwCacheFile() string {
	dir := CacheDir
	if dir == "" {
		dir = filepath.Join(os.Getenv("LOCALAPPDATA"), "TurboMENU")
	}
	return filepath.Join(dir, "codificador.json")
}

func loadHWCache() map[string]string {
	m := map[string]string{}
	if b, err := os.ReadFile(hwCacheFile()); err == nil {
		json.Unmarshal(b, &m)
	}
	return m
}

func saveHWCache(m map[string]string) {
	p := hwCacheFile()
	os.MkdirAll(filepath.Dir(p), 0o755)
	b, _ := json.Marshal(m)
	tmp := p + ".part"
	if os.WriteFile(tmp, b, 0o644) == nil {
		os.Rename(tmp, p)
	}
}
