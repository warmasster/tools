package media

import (
	"os"
	"path/filepath"
	"strings"
)

// Find busca ffmpeg.exe y ffprobe.exe en binDir, en el PATH y en las carpetas
// típicas (winget, scoop, choco, C:\ffmpeg…). No ejecuta nada. Devuelve "" en
// lo que no encuentre; si puede, devuelve los dos de la misma carpeta.
func Find(binDir string) (ffmpeg, ffprobe string) {
	for _, dir := range candidateDirs(binDir) {
		if publicRoot(dir) && !publicSafe(dir) {
			continue // C:\ffmpeg que otro usuario del PC puede cambiar: no se usa
		}
		f, p := filepath.Join(dir, "ffmpeg.exe"), filepath.Join(dir, "ffprobe.exe")
		hasF, hasP := isFile(f), isFile(p)
		if hasF && hasP {
			return f, p
		}
		if hasF && ffmpeg == "" {
			ffmpeg = f
		}
		if hasP && ffprobe == "" {
			ffprobe = p
		}
	}
	return ffmpeg, ffprobe
}

func candidateDirs(binDir string) []string {
	var dirs []string
	add := func(d string) {
		if d != "" {
			dirs = append(dirs, d)
		}
	}
	add(binDir)
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		add(strings.Trim(d, `"`))
	}
	local := os.Getenv("LOCALAPPDATA")
	home := os.Getenv("USERPROFILE")
	pf := os.Getenv("ProgramFiles")
	pf86 := os.Getenv("ProgramFiles(x86)")
	pd := os.Getenv("ProgramData")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	if local != "" {
		// winget instala en Packages\Gyan.FFmpeg_…\ffmpeg-x.y-…\bin
		if m, _ := filepath.Glob(filepath.Join(local, `Microsoft\WinGet\Packages\*FFmpeg*\*\bin`)); len(m) > 0 {
			dirs = append(dirs, m...)
		}
		add(filepath.Join(local, `Microsoft\WinGet\Links`))
		add(filepath.Join(local, `Programs\ffmpeg\bin`))
	}
	if home != "" {
		add(filepath.Join(home, `scoop\apps\ffmpeg\current\bin`))
		add(filepath.Join(home, `scoop\shims`))
	}
	add(filepath.Join(pd, `chocolatey\lib\ffmpeg\tools\ffmpeg\bin`))
	add(filepath.Join(pd, `chocolatey\bin`))
	add(`C:\ffmpeg\bin`)
	add(`C:\ffmpeg`)
	if pf != "" {
		add(filepath.Join(pf, `ffmpeg\bin`))
	}
	if pf86 != "" {
		add(filepath.Join(pf86, `ffmpeg\bin`))
	}
	return dirs
}

// publicRoot: carpetas que cualquier usuario del PC puede crear (en la raíz de C:).
func publicRoot(dir string) bool {
	d := strings.ToLower(filepath.Clean(dir))
	return d == `c:\ffmpeg` ||
		d == `c:\ffmpeg\bin`
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
