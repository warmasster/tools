package main

// Lo que más gasta ahora, agrupado por programa (Chrome con 30 procesos sale
// como uno solo).

import (
	"errors"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ProcGroup es un programa con todos sus procesos.
type ProcGroup struct {
	Name    string  `json:"name"`
	Exe     string  `json:"exe"`
	Count   int     `json:"count"`
	CPU     float64 `json:"cpu"`   // % del total del equipo
	MemMB   float64 `json:"memMB"` // memoria privada
	CanKill bool    `json:"canKill"`
}

var protectedExe = map[string]bool{}

func init() {
	for _, n := range strings.Fields(`system registry smss.exe csrss.exe wininit.exe winlogon.exe services.exe lsass.exe lsaiso.exe
		svchost.exe dwm.exe explorer.exe fontdrvhost.exe memory compression msmpeng.exe nissrv.exe securityhealthservice.exe
		sihost.exe ctfmon.exe taskhostw.exe runtimebroker.exe audiodg.exe spoolsv.exe conhost.exe dllhost.exe
		msedgewebview2.exe turbomon.exe searchindexer.exe startmenuexperiencehost.exe shellexperiencehost.exe
		textinputhost.exe applicationframehost.exe wudfhost.exe smartscreen.exe`) {
		protectedExe[n] = true
	}
	protectedExe["memory compression"] = true
	protectedExe["system idle process"] = true
}

type procSampler struct {
	mu    sync.Mutex
	prev  map[uint32]uint64 // pid → tiempo de CPU (100 ns)
	at    time.Time
	names map[string]string // exe → nombre bonito
}

func newProcSampler() *procSampler {
	return &procSampler{prev: map[uint32]uint64{}, names: map[string]string{}}
}

func (p *procSampler) sample() []ProcGroup {
	p.mu.Lock()
	defer p.mu.Unlock()
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	now := time.Now()
	wall := now.Sub(p.at)
	ncpu := float64(numCPU())
	cur := map[uint32]uint64{}
	groups := map[string]*ProcGroup{}
	paths := map[string]string{}
	self := uint32(os.Getpid())
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		pid := e.ProcessID
		if pid == 0 || pid == 4 {
			continue
		}
		exe := strings.ToLower(windows.UTF16ToString(e.ExeFile[:]))
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			continue
		}
		var c, x, k, u windows.Filetime
		var cpuT uint64
		if windows.GetProcessTimes(h, &c, &x, &k, &u) == nil {
			cpuT = (uint64(k.HighDateTime)<<32 | uint64(k.LowDateTime)) + (uint64(u.HighDateTime)<<32 | uint64(u.LowDateTime))
		}
		var pmc [80]byte
		*(*uint32)(unsafe.Pointer(&pmc[0])) = 80
		mem := 0.0
		if r, _, _ := procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&pmc[0])), 80); r != 0 {
			mem = float64(*(*uintptr)(unsafe.Pointer(&pmc[72]))) / (1 << 20)
		}
		if _, ok := paths[exe]; !ok {
			var buf [windows.MAX_PATH * 2]uint16
			n := uint32(len(buf))
			if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) == nil {
				paths[exe] = windows.UTF16ToString(buf[:n])
			}
		}
		windows.CloseHandle(h)
		cur[pid] = cpuT
		g := groups[exe]
		if g == nil {
			g = &ProcGroup{Exe: exe, CanKill: !protectedExe[exe] && pid != self}
			groups[exe] = g
		}
		g.Count++
		g.MemMB += mem
		if prev, ok := p.prev[pid]; ok && wall > 0 && cpuT >= prev {
			g.CPU += float64(cpuT-prev) * 100 / (float64(wall) / 100) / ncpu
		}
	}
	first := p.at.IsZero()
	p.prev, p.at = cur, now
	out := make([]ProcGroup, 0, len(groups))
	for exe, g := range groups {
		name, ok := p.names[exe]
		if !ok {
			name = fileDescription(paths[exe])
			if name == "" || len(name) > 48 {
				name = strings.TrimSuffix(exe, ".exe")
			}
			p.names[exe] = name
		}
		g.Name = name
		g.CPU = math.Round(g.CPU*10) / 10
		if first {
			g.CPU = 0
		}
		g.MemMB = math.Round(g.MemMB)
		if g.MemMB >= 40 || g.CPU >= 0.5 {
			out = append(out, *g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MemMB > out[j].MemMB })
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}

func numCPU() int {
	n := 0
	var mask, sys uintptr
	if r, _, _ := procGetAffinity.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&mask)), uintptr(unsafe.Pointer(&sys))); r != 0 {
		for m := sys; m != 0; m &= m - 1 {
			n++
		}
	}
	if n == 0 {
		n = 1
	}
	return n
}

var procGetAffinity = kernel32.NewProc("GetProcessAffinityMask")

// killGroup cierra todos los procesos de un programa (no los del sistema).
func killGroup(exe string) (int, error) {
	exe = strings.ToLower(exe)
	if protectedExe[exe] {
		return 0, errors.New("eso es parte de Windows: no se cierra")
	}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	n, failed := 0, 0
	self := uint32(os.Getpid())
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if e.ProcessID == self || strings.ToLower(windows.UTF16ToString(e.ExeFile[:])) != exe {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, e.ProcessID)
		if err != nil {
			failed++
			continue
		}
		if windows.TerminateProcess(h, 1) == nil {
			n++
		} else {
			failed++
		}
		windows.CloseHandle(h)
	}
	if n == 0 && failed > 0 {
		return 0, errors.New("Windows no deja cerrarlo (es de otro usuario o del sistema)")
	}
	return n, nil
}
