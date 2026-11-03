package checks

// Discos: espacio libre, si son SSD y dónde están instalados los juegos.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// IsSSD dice si la unidad (p. ej. "C:") es un SSD. ok=false si no se sabe.
func IsSSD(drive string) (ssd, ok bool) {
	p, _ := windows.UTF16PtrFromString(`\\.\` + strings.TrimSuffix(drive, `\`))
	h, err := windows.CreateFile(p, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return false, false
	}
	defer windows.CloseHandle(h)
	// STORAGE_PROPERTY_QUERY{PropertyId: StorageDeviceSeekPenaltyProperty, QueryType: PropertyStandardQuery}
	q := [3]uint32{7, 0, 0}
	var out [3]uint32 // DEVICE_SEEK_PENALTY_DESCRIPTOR
	var n uint32
	const ioctlStorageQueryProperty = 0x2D1400
	if err := windows.DeviceIoControl(h, ioctlStorageQueryProperty, (*byte)(unsafe.Pointer(&q[0])), 12, (*byte)(unsafe.Pointer(&out[0])), 12, &n, nil); err != nil || n < 9 {
		return false, false
	}
	return out[2]&0xff == 0, true
}

// FreeSpace devuelve libre y total de una unidad, en bytes.
func FreeSpace(drive string) (free, total uint64, ok bool) {
	p, _ := windows.UTF16PtrFromString(strings.TrimSuffix(drive, `\`) + `\`)
	var avail, tot, totFree uint64
	if windows.GetDiskFreeSpaceEx(p, &avail, &tot, &totFree) != nil {
		return 0, 0, false
	}
	return avail, tot, true
}

// GameLibrary es una carpeta con juegos instalados.
type GameLibrary struct {
	Launcher string `json:"launcher"`
	Path     string `json:"path"`
	Drive    string `json:"drive"`
	Games    int    `json:"games"`
}

var vdfPath = regexp.MustCompile(`"path"\s+"([^"]+)"`)

// GameLibraries busca las bibliotecas de Steam y los juegos de Epic.
func GameLibraries() []GameLibrary {
	var out []GameLibrary
	// Steam
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam`, registry.QUERY_VALUE); err == nil {
		sp, _, _ := k.GetStringValue("SteamPath")
		k.Close()
		if sp != "" {
			sp = filepath.Clean(sp)
			paths := []string{sp}
			if b, err := os.ReadFile(filepath.Join(sp, "steamapps", "libraryfolders.vdf")); err == nil {
				for _, m := range vdfPath.FindAllStringSubmatch(string(b), -1) {
					paths = append(paths, filepath.Clean(strings.ReplaceAll(m[1], `\\`, `\`)))
				}
			}
			seen := map[string]bool{}
			for _, p := range paths {
				key := strings.ToLower(p)
				if seen[key] {
					continue
				}
				seen[key] = true
				ents, err := os.ReadDir(filepath.Join(p, "steamapps", "common"))
				if err != nil {
					continue
				}
				n := 0
				for _, e := range ents {
					if e.IsDir() && !strings.HasPrefix(strings.ToLower(e.Name()), "steamworks") {
						n++
					}
				}
				if n > 0 {
					out = append(out, GameLibrary{Launcher: "Steam", Path: p, Drive: filepath.VolumeName(p), Games: n})
				}
			}
		}
	}
	// Epic Games
	man := filepath.Join(os.Getenv("ProgramData"), `Epic\EpicGamesLauncher\Data\Manifests`)
	if files, err := filepath.Glob(filepath.Join(man, "*.item")); err == nil {
		byDrive := map[string]*GameLibrary{}
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			var it struct{ InstallLocation string }
			if json.Unmarshal(b, &it) != nil || it.InstallLocation == "" {
				continue
			}
			d := strings.ToUpper(filepath.VolumeName(it.InstallLocation))
			if byDrive[d] == nil {
				byDrive[d] = &GameLibrary{Launcher: "Epic Games", Path: filepath.Dir(it.InstallLocation), Drive: d}
			}
			byDrive[d].Games++
		}
		for _, l := range byDrive {
			out = append(out, *l)
		}
	}
	return out
}
