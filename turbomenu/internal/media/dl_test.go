package media

import (
	"context"
	"os"
	"testing"
	"time"
)

// Descarga real: TURBOMENU_FFMPEG_DOWNLOAD=<carpeta>.
func TestDownloadReal(t *testing.T) {
	dir := os.Getenv("TURBOMENU_FFMPEG_DOWNLOAD")
	if dir == "" || testing.Short() {
		t.Skip("define TURBOMENU_FFMPEG_DOWNLOAD para probar la descarga")
	}
	t0 := time.Now()
	var last int64
	err := Download(context.Background(), dir, func(done, total int64) { last = total })
	if err != nil {
		t.Fatal(err, err.(*Error).Detail)
	}
	f, p := Find(dir)
	t.Logf("descarga %d bytes en %v; ffmpeg=%s ffprobe=%s", last, time.Since(t0), f, p)
	if f == "" || p == "" {
		t.Fatal("no aparece tras descargar")
	}
}
