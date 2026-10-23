package media

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Origen de las builds de FFmpeg (GitHub, BtbN/FFmpeg-Builds, Release "latest").
const ffmpegRelease = "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/"

// Build "gpl-shared": mismos códecs que la estática (x264, x265, vpx, aom,
// svt-av1, opus, lame, webp…) pero la descarga pesa menos de la mitad, porque
// ffmpeg y ffprobe comparten las DLL en vez de llevar cada uno una copia.
var releaseAsset = regexp.MustCompile(`^ffmpeg-n(\d+)\.(\d+)-latest-win64-gpl-shared-[\d.]+\.zip$`)

const masterAsset = "ffmpeg-master-latest-win64-gpl-shared.zip"

var httpClient = &http.Client{Transport: &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           (&net.Dialer{Timeout: 20 * time.Second}).DialContext,
	TLSHandshakeTimeout:   20 * time.Second,
	ResponseHeaderTimeout: 30 * time.Second,
}}

// Download descarga FFmpeg para Windows de 64 bits (última versión estable de
// BtbN/FFmpeg-Builds, unos 85-90 MB), comprueba su SHA-256 y deja en binDir
// ffmpeg.exe, ffprobe.exe y sus DLL. progress recibe los bytes descargados y
// el total (0 si no se sabe); puede ser nil.
func Download(ctx context.Context, binDir string, progress func(done, total int64)) error {
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return userErr("No se pudo crear la carpeta para FFmpeg.", err)
	}
	sums, err := fetchChecksums(ctx)
	if err != nil {
		return err
	}
	name := pickAsset(sums)
	if name == "" {
		return userErr("No se encontró FFmpeg para descargar. Inténtalo más tarde.", nil)
	}
	zipPath := filepath.Join(binDir, "ffmpeg-descarga.zip.part")
	defer os.Remove(zipPath)

	got, err := downloadFile(ctx, ffmpegRelease+name, zipPath, progress)
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, sums[name]) {
		// La Release se regenera a diario: puede haber cambiado a mitad.
		if again, err2 := fetchChecksums(ctx); err2 != nil || !strings.EqualFold(got, again[name]) {
			return userErr("La descarga de FFmpeg llegó dañada. Vuelve a intentarlo.", nil)
		}
	}
	return extractFFmpeg(zipPath, binDir)
}

func fetchChecksums(ctx context.Context) (map[string]string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", ffmpegRelease+"checksums.sha256", nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, netErr(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, userErr("No se pudo contactar con el servidor de FFmpeg. Inténtalo más tarde.", fmt.Errorf("HTTP %d", resp.StatusCode))
	}
	sums := map[string]string{}
	sc := bufio.NewScanner(io.LimitReader(resp.Body, 1<<20))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 {
			sums[strings.TrimPrefix(f[1], "*")] = f[0]
		}
	}
	return sums, nil
}

// pickAsset elige la versión estable más alta; si no hay, la de desarrollo.
func pickAsset(sums map[string]string) string {
	best, bestV := "", -1
	for n := range sums {
		m := releaseAsset.FindStringSubmatch(n)
		if m == nil {
			continue
		}
		major, _ := strconv.Atoi(m[1])
		minor, _ := strconv.Atoi(m[2])
		if v := major*1000 + minor; v > bestV {
			best, bestV = n, v
		}
	}
	if best == "" && sums[masterAsset] != "" {
		best = masterAsset
	}
	return best
}

func downloadFile(ctx context.Context, url, dst string, progress func(done, total int64)) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", netErr(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", userErr("No se pudo descargar FFmpeg. Inténtalo más tarde.", fmt.Errorf("HTTP %d", resp.StatusCode))
	}
	f, err := os.Create(dst)
	if err != nil {
		return "", userErr("No se pudo guardar FFmpeg en el disco.", err)
	}
	defer f.Close()
	h := sha256.New()
	total := resp.ContentLength
	if total < 0 {
		total = 0
	}
	buf := make([]byte, 256<<10)
	var done int64
	last := time.Time{}
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return "", diskErr(err)
			}
			h.Write(buf[:n])
			done += int64(n)
			if progress != nil && time.Since(last) > 100*time.Millisecond {
				last = time.Now()
				progress(done, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", netErr(ctx, rerr)
		}
	}
	if progress != nil {
		progress(done, total)
	}
	if total > 0 && done != total {
		return "", userErr("La descarga de FFmpeg se cortó. Vuelve a intentarlo.", nil)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractFFmpeg saca bin/ffmpeg.exe, bin/ffprobe.exe y bin/*.dll. Cada archivo
// se escribe como .part y se renombra; ffmpeg.exe va el último para que Find
// no lo dé por instalado hasta que esté todo.
func extractFFmpeg(zipPath, binDir string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return userErr("La descarga de FFmpeg llegó dañada. Vuelve a intentarlo.", err)
	}
	defer zr.Close()
	var dlls, exes []*zip.File
	for _, f := range zr.File {
		dir, base := path.Split(f.Name)
		if !strings.HasSuffix(dir, "bin/") || base == "" {
			continue
		}
		lb := strings.ToLower(base)
		switch {
		case strings.HasSuffix(lb, ".dll"):
			dlls = append(dlls, f)
		case lb == "ffprobe.exe":
			exes = append([]*zip.File{f}, exes...)
		case lb == "ffmpeg.exe":
			exes = append(exes, f)
		}
	}
	if len(exes) != 2 {
		return userErr("La descarga de FFmpeg no tiene lo esperado.", nil)
	}
	for _, f := range append(dlls, exes...) {
		if err := extractOne(f, filepath.Join(binDir, path.Base(f.Name))); err != nil {
			return err
		}
	}
	return nil
}

func extractOne(f *zip.File, dst string) error {
	rc, err := f.Open()
	if err != nil {
		return userErr("La descarga de FFmpeg llegó dañada. Vuelve a intentarlo.", err)
	}
	defer rc.Close()
	tmp := dst + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return userErr("No se pudo guardar FFmpeg en el disco.", err)
	}
	_, err = io.Copy(out, rc)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return diskErr(err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		// Si ya existía y está en uso (otra ventana usándolo), se deja el que hay.
		if isFile(dst) {
			return nil
		}
		return userErr("No se pudo guardar FFmpeg en el disco.", err)
	}
	return nil
}

func netErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return userErr("No hay conexión o el servidor no responde. Inténtalo más tarde.", err)
}
