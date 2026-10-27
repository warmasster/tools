package steam

import (
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type proc struct {
	pid  uint32
	name string
}

// processes lista los procesos abiertos (nombre del .exe y PID).
func processes() []proc {
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(h)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	var out []proc
	for err = windows.Process32First(h, &pe); err == nil; err = windows.Process32Next(h, &pe) {
		out = append(out, proc{pid: pe.ProcessID, name: windows.UTF16ToString(pe.ExeFile[:])})
	}
	return out
}

// Procesos de Steam que hay que cerrar para cambiar de cuenta. steamservice.exe
// no: es un servicio del sistema y no guarda nada de la cuenta.
var steamProcs = []string{"steam.exe", "steamwebhelper.exe", "gameoverlayui.exe", "gameoverlayui64.exe", "steamerrorreporter.exe", "steamerrorreporter64.exe"}

func isSteamProc(name string) bool {
	for _, n := range steamProcs {
		if strings.EqualFold(n, name) {
			return true
		}
	}
	return false
}

func steamPIDs(onlyMain bool) []uint32 {
	var out []uint32
	for _, p := range processes() {
		if onlyMain && !strings.EqualFold(p.name, "steam.exe") {
			continue
		}
		if isSteamProc(p.name) {
			out = append(out, p.pid)
		}
	}
	return out
}

// ownPIDs: los procesos de Steam que se cierran a la fuerza, solo si su .exe está
// dentro de la carpeta de Steam (otro programa puede llamarse steam.exe). Las rutas
// se comparan resueltas: la carpeta de Steam puede ser un enlace a otro disco.
func (in *Install) ownPIDs(onlyMain bool) []uint32 {
	var out []uint32
	dirs := []string{strings.ToLower(filepath.Clean(in.Dir)) + `\`}
	if fp := finalPath(in.Dir); fp != "" {
		dirs = append(dirs, strings.ToLower(fp)+`\`)
	}
	for _, pid := range steamPIDs(onlyMain) {
		p := imagePath(pid)
		if p == "" {
			continue
		}
		cands := []string{strings.ToLower(filepath.Clean(p))}
		if fp := finalPath(p); fp != "" {
			cands = append(cands, strings.ToLower(fp))
		}
	match:
		for _, c := range cands {
			for _, d := range dirs {
				if strings.HasPrefix(c, d) {
					out = append(out, pid)
					break match
				}
			}
		}
	}
	return out
}

// finalPath resuelve enlaces y uniones de una ruta ("" si no se puede).
func finalPath(p string) string {
	u, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return ""
	}
	h, err := windows.CreateFile(u, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
	if err != nil || n == 0 || int(n) > len(buf) {
		return ""
	}
	s := windows.UTF16ToString(buf[:n])
	if strings.HasPrefix(s, `\\?\UNC\`) {
		return `\\` + s[8:]
	}
	return filepath.Clean(strings.TrimPrefix(s, `\\?\`))
}

// waitOwnGone es como waitGone, pero solo con los procesos de la carpeta de Steam.
func (in *Install) waitOwnGone(onlyMain bool, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for {
		if len(in.ownPIDs(onlyMain)) == 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// imagePath: la ruta del .exe de un proceso ("" si no se puede saber).
func imagePath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

func killPID(pid uint32) {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	if windows.TerminateProcess(h, 1) == nil {
		windows.WaitForSingleObject(h, 3000)
	}
}

