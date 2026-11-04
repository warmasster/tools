// Package noise mide el «ruido» del sistema en cada núcleo: cuánto tiempo le
// quitan a un hilo normal las interrupciones, los drivers y los demás
// programas, y qué hueco es el peor. Es la prueba FTQ («fixed time quantum»)
// de los superordenadores, hecha núcleo a núcleo.
//
// La medida la hace un proceso aparte (TurboMON.exe --noise-probe) con Go sin
// interrupciones propias (GODEBUG=asyncpreemptoff=1) y un bucle en
// ensamblador, para no medirse a sí mismo. Con permiso de administrador, a la
// vez se escuchan los eventos de DPC e interrupciones de Windows para saber qué
// driver es el que roba.
package noise

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"turbomon/internal/etw"
)

var (
	modKernel       = windows.NewLazySystemDLL("kernel32.dll")
	pSetAffinity    = modKernel.NewProc("SetThreadAffinityMask")
	pSetThreadPrio  = modKernel.NewProc("SetThreadPriority")
	pQPC            = modKernel.NewProc("QueryPerformanceCounter")
	pQPF            = modKernel.NewProc("QueryPerformanceFrequency")
	pEnumDrivers    = modKernel.NewProc("K32EnumDeviceDrivers")
	pDriverBaseName = modKernel.NewProc("K32GetDeviceDriverBaseNameW")
	modNtdll        = windows.NewLazySystemDLL("ntdll.dll")
	pNtQuerySysInfo = modNtdll.NewProc("NtQuerySystemInformation")
)

func qpc() int64 {
	var v int64
	pQPC.Call(uintptr(unsafe.Pointer(&v)))
	return v
}

func qpf() int64 {
	var v int64
	pQPF.Call(uintptr(unsafe.Pointer(&v)))
	return v
}

// ---------- la medida (proceso hijo) ----------

type probeLine struct {
	Core   int     `json:"core"`
	Stolen float64 `json:"stolen"` // % del tiempo
	MaxUs  float64 `json:"maxUs"`
	N10    uint64  `json:"n10"`   // huecos > 10 µs
	N100   uint64  `json:"n100"`  // > 100 µs
	N1000  uint64  `json:"n1000"` // > 1 ms
	Secs   float64 `json:"secs"`
}

// RunProbe mide cada núcleo de cores durante ms milisegundos y escribe una
// línea JSON por núcleo. Lo llama main con «--noise-probe ms 0,1,2…».
func RunProbe(args []string, w io.Writer) int {
	if len(args) < 2 {
		return 2
	}
	ms, _ := strconv.Atoi(args[0])
	if ms < 50 || ms > 10000 {
		ms = 1000
	}
	var cores []int
	for _, f := range strings.Split(args[1], ",") {
		if c, err := strconv.Atoi(strings.TrimSpace(f)); err == nil && c >= 0 && c < 64 {
			cores = append(cores, c)
		}
	}
	runtime.LockOSThread()
	// Prioridad alta: los programas normales no cuentan (Windows los manda a
	// otro núcleo); solo lo que ningún juego puede esquivar: interrupciones,
	// drivers y hilos de tiempo real.
	windows.SetPriorityClass(windows.CurrentProcess(), windows.HIGH_PRIORITY_CLASS)
	pSetThreadPrio.Call(uintptr(windows.CurrentThread()), 2) // THREAD_PRIORITY_HIGHEST
	// Ciclos del procesador por segundo, comparando con el reloj de Windows.
	q0, t0 := qpc(), tsc()
	time.Sleep(100 * time.Millisecond)
	q1, t1 := qpc(), tsc()
	hz := float64(t1-t0) * float64(qpf()) / float64(q1-q0)
	if hz < 1e8 {
		return 3
	}
	us := hz / 1e6
	enc := json.NewEncoder(w)
	th := windows.CurrentThread()
	for _, c := range cores {
		pSetAffinity.Call(uintptr(th), uintptr(1)<<c)
		time.Sleep(5 * time.Millisecond) // que Windows mueva el hilo a ese núcleo
		dur := uint64(float64(ms) * 1000 * us)
		stolen, maxGap, n1, n2, n3 := spin(dur, uint64(10*us), uint64(100*us), uint64(1000*us))
		enc.Encode(probeLine{
			Core: c, Stolen: math.Round(float64(stolen)/float64(dur)*100*1000) / 1000, MaxUs: math.Round(float64(maxGap) / us),
			N10: n1, N100: n2, N1000: n3, Secs: float64(ms) / 1000,
		})
	}
	return 0
}

// ---------- la prueba (agente) ----------

// Core es el resultado de un procesador lógico.
type Core struct {
	ID        int     `json:"id"`    // procesador lógico
	Phys      int     `json:"phys"`  // núcleo físico
	Class     int     `json:"class"` // eficiencia (en híbridos, más alto = núcleo P)
	Done      bool    `json:"done"`
	Stolen    float64 `json:"stolen"`  // % del tiempo que le quitan
	MaxUs     float64 `json:"maxUs"`   // el peor hueco
	Per100    float64 `json:"per100"`  // huecos > 100 µs por segundo
	Per1000   float64 `json:"per1000"` // huecos > 1 ms por segundo
	DPC       float64 `json:"dpc"`     // % del tiempo en DPC (drivers)
	ISR       float64 `json:"isr"`     // % del tiempo en interrupciones
	Level     string  `json:"level"`   // ok, warn, bad
	WorstDrv  string  `json:"worstDrv,omitempty"`
	WorstDrUs float64 `json:"worstDrvUs,omitempty"`
}

// Driver es lo que gastó un driver en DPC e interrupciones durante la prueba.
type Driver struct {
	File    string  `json:"file"`
	Desc    string  `json:"desc"`
	Count   int     `json:"count"`
	TotalMs float64 `json:"totalMs"`
	MaxUs   float64 `json:"maxUs"`
	Over500 int     `json:"over500"` // ejecuciones de más de 0,5 ms
	ISR     bool    `json:"isr"`     // sobre todo interrupciones
	Level   string  `json:"level"`
}

// State es cómo va la prueba.
type State struct {
	Running bool     `json:"running"`
	Pct     float64  `json:"pct"`
	Current int      `json:"current"`
	Cores   []Core   `json:"cores"`
	Drivers []Driver `json:"drivers"`
	Deep    bool     `json:"deep"`              // con drivers (administrador)
	DeepErr string   `json:"deepErr,omitempty"` // por qué no hay drivers
	Error   string   `json:"error,omitempty"`
	At      int64    `json:"at"`
	Secs    float64  `json:"secs"`
}

// LP es un procesador lógico a medir.
type LP struct{ ID, Phys, Class int }

// Options de la prueba.
type Options struct {
	Exe      string        // TurboMON.exe (para el proceso que mide)
	PerCore  time.Duration // tiempo por núcleo
	Cores    []LP
	Deep     bool                     // escuchar DPC e interrupciones (administrador)
	Describe func(path string) string // descripción de un archivo (versión)
}

// Test es una prueba en marcha o acabada.
type Test struct {
	mu sync.Mutex
	st State
}

// Get devuelve el estado.
func (t *Test) Get() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.st
	s.Cores = append([]Core(nil), t.st.Cores...)
	s.Drivers = append([]Driver(nil), t.st.Drivers...)
	return s
}

func (t *Test) set(f func(*State)) {
	t.mu.Lock()
	f(&t.st)
	t.mu.Unlock()
}

// Start lanza la prueba.
func Start(o Options) *Test {
	if o.PerCore <= 0 {
		o.PerCore = time.Second
	}
	t := &Test{st: State{Running: true, Current: -1}}
	for _, c := range o.Cores {
		t.st.Cores = append(t.st.Cores, Core{ID: c.ID, Phys: c.Phys, Class: c.Class})
	}
	go t.run(o)
	return t
}

// sppi = SYSTEM_PROCESSOR_PERFORMANCE_INFORMATION
type sppi struct {
	Idle, Kernel, User, Dpc, Interrupt int64
	InterruptCount                     uint32
	_                                  uint32
}

func coreTimes() []sppi {
	n := runtime.NumCPU()
	if n > 64 {
		n = 64
	}
	buf := make([]sppi, n)
	var ret uint32
	r, _, _ := pNtQuerySysInfo.Call(8, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))*unsafe.Sizeof(buf[0]), uintptr(unsafe.Pointer(&ret)))
	if r != 0 {
		return nil
	}
	return buf[:ret/uint32(unsafe.Sizeof(buf[0]))]
}

func level(stolen, maxUs float64) string {
	switch {
	case maxUs >= 2000 || stolen >= 3:
		return "bad"
	case maxUs >= 500 || stolen >= 1:
		return "warn"
	}
	return "ok"
}

func (t *Test) run(o Options) {
	start := time.Now()
	defer t.set(func(s *State) {
		s.Running, s.Pct, s.Current, s.At = false, 100, -1, time.Now().Unix()
		s.Secs = math.Round(time.Since(start).Seconds())
	})
	if len(o.Cores) == 0 {
		t.set(func(s *State) { s.Error = "no se han podido leer los núcleos" })
		return
	}
	var deep *deepTrace
	if o.Deep {
		d, err := startDeep()
		if err != nil {
			t.set(func(s *State) { s.DeepErr = err.Error() })
		} else {
			deep = d
			t.set(func(s *State) { s.Deep = true })
		}
	}
	before := coreTimes()
	ids := make([]string, len(o.Cores))
	for i, c := range o.Cores {
		ids[i] = strconv.Itoa(c.ID)
	}
	cmd := exec.Command(o.Exe, "--noise-probe", strconv.Itoa(int(o.PerCore/time.Millisecond)), strings.Join(ids, ","))
	cmd.Env = append(os.Environ(), "GODEBUG=asyncpreemptoff=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	out, err := cmd.StdoutPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		if deep != nil {
			deep.stop()
		}
		t.set(func(s *State) { s.Error = "no se pudo lanzar la medida: " + err.Error() })
		return
	}
	idx := map[int]int{}
	for i, c := range o.Cores {
		idx[c.ID] = i
	}
	t.set(func(s *State) { s.Current = o.Cores[0].ID })
	sc := bufio.NewScanner(out)
	done := 0
	for sc.Scan() {
		var l probeLine
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue
		}
		i, ok := idx[l.Core]
		if !ok {
			continue
		}
		done++
		t.set(func(s *State) {
			c := &s.Cores[i]
			c.Done = true
			c.Stolen, c.MaxUs = l.Stolen, l.MaxUs
			if l.Secs > 0 {
				c.Per100 = math.Round(float64(l.N100)/l.Secs*10) / 10
				c.Per1000 = math.Round(float64(l.N1000)/l.Secs*10) / 10
			}
			c.Level = level(c.Stolen, c.MaxUs)
			s.Pct = float64(done) / float64(len(o.Cores)) * 100
			if done < len(o.Cores) {
				s.Current = o.Cores[done].ID
			}
		})
	}
	cmd.Wait()
	after := coreTimes()
	if len(before) == len(after) {
		t.set(func(s *State) {
			for i := range s.Cores {
				id := s.Cores[i].ID
				if id >= len(after) {
					continue
				}
				b, a := before[id], after[id]
				tot := float64((a.Kernel + a.User) - (b.Kernel + b.User))
				if tot > 0 {
					s.Cores[i].DPC = math.Round(float64(a.Dpc-b.Dpc)/tot*100*100) / 100
					s.Cores[i].ISR = math.Round(float64(a.Interrupt-b.Interrupt)/tot*100*100) / 100
				}
			}
		})
	}
	if done == 0 {
		t.set(func(s *State) { s.Error = "la medida no ha devuelto nada" })
	}
	if deep != nil {
		deep.stop()
		drivers, perCore := deep.result(o.Describe)
		t.set(func(s *State) {
			s.Drivers = drivers
			for i := range s.Cores {
				if w, ok := perCore[s.Cores[i].ID]; ok {
					s.Cores[i].WorstDrv, s.Cores[i].WorstDrUs = w.file, w.us
				}
			}
		})
	}
}

// ---------- DPC e interrupciones (administrador) ----------

var (
	perfInfoGUID = windows.GUID{Data1: 0xce1dbfb4, Data2: 0x137e, Data3: 0x4da6, Data4: [8]byte{0x87, 0xb0, 0x3f, 0x59, 0xaa, 0x10, 0x2c, 0xbc}}
	imageGUID    = windows.GUID{Data1: 0x2cb15d1d, Data2: 0x5fc1, Data3: 0x11d2, Data4: [8]byte{0xab, 0xe1, 0x00, 0xa0, 0xc9, 0x11, 0xf5, 0x18}}
)

const kernelImageLoad = 0x00000004

type routine struct {
	count, over500 int
	total, max     int64
	isr            int
}

type module struct {
	base, size uint64
	file       string
}

type worst struct {
	file string
	us   float64
}

type deepTrace struct {
	s        *etw.Session
	freq     float64
	routines map[uint64]*routine
	cores    map[int]coreWorst
	mods     []module
}

type coreWorst struct {
	addr uint64
	max  int64
}

func startDeep() (*deepTrace, error) {
	d := &deepTrace{freq: float64(qpf()), routines: map[uint64]*routine{}, cores: map[int]coreWorst{}}
	name := "TurboMON-Ruido"
	s, err := etw.Start(name, etw.Options{KernelFlags: etw.KernelDPC | etw.KernelInterrupt | kernelImageLoad, BufferKB: 256})
	if err != nil {
		if err == etw.ErrNeedAdmin {
			return nil, fmt.Errorf("para saber qué driver es, hace falta el permiso de administrador")
		}
		return nil, fmt.Errorf("Windows no deja escuchar los drivers: %v", err)
	}
	d.s = s
	s.OnAll(d.handle)
	return d, nil
}

// handle corre en el hilo de la sesión: nadie más toca los mapas hasta stop.
func (d *deepTrace) handle(r *etw.Record) {
	h := &r.Header
	switch h.Provider {
	case perfInfoGUID:
		op := h.Opcode
		if op != 66 && op != 67 && op != 68 && op != 69 && op != 50 {
			return
		}
		rd := etw.NewReader(r.Data())
		init := int64(rd.U64())
		addr := rd.Ptr()
		if !rd.OK() || init <= 0 || h.TimeStamp < init {
			return
		}
		dur := h.TimeStamp - init
		if float64(dur) > d.freq { // más de 1 s: marca rara
			return
		}
		rt := d.routines[addr]
		if rt == nil {
			rt = &routine{}
			d.routines[addr] = rt
		}
		rt.count++
		rt.total += dur
		if dur > rt.max {
			rt.max = dur
		}
		if float64(dur) > d.freq/2000 {
			rt.over500++
		}
		if op == 67 || op == 50 {
			rt.isr++
		}
		cpu := int(r.BufferContext & 0xFF)
		if h.Flags&0x0200 != 0 { // EVENT_HEADER_FLAG_PROCESSOR_INDEX
			cpu = int(r.BufferContext & 0xFFFF)
		}
		if w := d.cores[cpu]; dur > w.max {
			w.addr, w.max = addr, dur
			d.cores[cpu] = w
		}
	case imageGUID:
		if h.Opcode != 3 && h.Opcode != 10 { // DCStart, Load
			return
		}
		rd := etw.NewReader(r.Data())
		base := rd.Ptr()
		size := rd.Ptr()
		pid := rd.U32()
		rd.Skip(4 + 4 + 4 + 8 + 16)
		file := rd.UTF16Z()
		if rd.OK() && pid == 0 && base != 0 {
			d.mods = append(d.mods, module{base: base, size: size, file: file})
		}
	}
}

func (d *deepTrace) stop() { d.s.Stop() }

// enumDrivers es el plan B para saber dónde está cada driver.
func enumDrivers() []module {
	enableDebugPrivilege()
	bases := make([]uintptr, 1024)
	var need uint32
	r, _, _ := pEnumDrivers.Call(uintptr(unsafe.Pointer(&bases[0])), uintptr(len(bases))*unsafe.Sizeof(bases[0]), uintptr(unsafe.Pointer(&need)))
	if r == 0 {
		return nil
	}
	n := int(need / uint32(unsafe.Sizeof(bases[0])))
	if n > len(bases) {
		n = len(bases)
	}
	var out []module
	buf := make([]uint16, 260)
	for _, b := range bases[:n] {
		if b == 0 {
			continue
		}
		l, _, _ := pDriverBaseName.Call(b, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if l == 0 {
			continue
		}
		out = append(out, module{base: uint64(b), file: windows.UTF16ToString(buf[:l])})
	}
	return out
}

// enableDebugPrivilege activa un permiso que el administrador ya tiene pero
// viene apagado: sin él, Windows 11 24H2 no dice dónde están los drivers.
func enableDebugPrivilege() {
	var tok windows.Token
	if windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &tok) != nil {
		return
	}
	defer tok.Close()
	var luid windows.LUID
	name, _ := windows.UTF16PtrFromString("SeDebugPrivilege")
	if windows.LookupPrivilegeValue(nil, name, &luid) != nil {
		return
	}
	tp := windows.Tokenprivileges{PrivilegeCount: 1}
	tp.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
	windows.AdjustTokenPrivileges(tok, false, &tp, 0, nil, nil)
}

func baseName(p string) string {
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		p = p[i+1:]
	}
	return p
}

// Nombres claros de los drivers más corrientes.
var knownDrivers = map[string]string{
	"nvlddmkm.sys": "Driver de la gráfica NVIDIA", "amdkmdag.sys": "Driver de la gráfica AMD", "atikmdag.sys": "Driver de la gráfica AMD",
	"igdkmd64.sys": "Driver de la gráfica Intel", "igdkmdn64.sys": "Driver de la gráfica Intel", "igdkmdnd64.sys": "Driver de la gráfica Intel",
	"dxgkrnl.sys": "DirectX (Windows)", "dxgmms2.sys": "DirectX (Windows)", "ntoskrnl.exe": "Núcleo de Windows",
	"ndis.sys": "Red (Windows)", "tcpip.sys": "Red (Windows)", "netio.sys": "Red (Windows)", "wdf01000.sys": "Marco de drivers de Windows (lo usa otro driver)",
	"storport.sys": "Discos (Windows)", "stornvme.sys": "Disco NVMe", "storahci.sys": "Discos SATA", "iastorac.sys": "Discos Intel RST", "iastora.sys": "Discos Intel RST",
	"usbxhci.sys": "USB", "ucx01000.sys": "USB", "usbport.sys": "USB", "hidclass.sys": "Ratón y teclado (HID)", "hidusb.sys": "Ratón y teclado (HID)",
	"hdaudbus.sys": "Audio de la placa", "portcls.sys": "Audio (Windows)", "hdaudio.sys": "Audio de alta definición", "acpi.sys": "ACPI (placa base y energía)",
	"intelppm.sys": "Gestión de energía del procesador Intel", "amdppm.sys": "Gestión de energía del procesador AMD", "pci.sys": "Bus PCI", "clipsp.sys": "Licencias de Windows",
	"rt640x64.sys": "Red Realtek", "rtux64w10.sys": "Red Realtek USB", "e1d68x64.sys": "Red Intel", "e1r68x64.sys": "Red Intel", "e2f68.sys": "Red Intel 2,5 GbE", "e2f.sys": "Red Intel 2,5 GbE",
	"netwtw10.sys": "Wifi Intel", "netwtw14.sys": "Wifi Intel", "netwtw08.sys": "Wifi Intel", "netwaw16.sys": "Wifi Intel", "rtwlane.sys": "Wifi Realtek", "rtwlane6.sys": "Wifi Realtek", "mtkwl6ex.sys": "Wifi MediaTek",
	"killernetwork.sys": "Red Killer", "bthport.sys": "Bluetooth", "bthusb.sys": "Bluetooth", "nvhda64v.sys": "Audio HDMI de NVIDIA", "atihdwt6.sys": "Audio HDMI de AMD",
	"wdfilter.sys": "Antivirus de Windows", "wd.sys": "Antivirus de Windows", "fltmgr.sys": "Filtros de archivos (antivirus y similares)",
	"winhvr.sys": "Hyper-V (WSL o máquinas virtuales)", "winhv.sys": "Hyper-V (WSL o máquinas virtuales)", "vmbusr.sys": "Hyper-V (WSL o máquinas virtuales)", "vmbus.sys": "Hyper-V (WSL o máquinas virtuales)",
	"vmswitch.sys": "Red virtual de Hyper-V", "classpnp.sys": "Discos (Windows)", "disk.sys": "Discos (Windows)", "volmgr.sys": "Discos (Windows)",
	"nvvhci.sys": "Mandos virtuales de NVIDIA", "nvvad64v.sys": "Audio virtual de NVIDIA", "mouhid.sys": "Ratón", "kbdhid.sys": "Teclado", "i8042prt.sys": "Teclado y ratón PS/2 (portátil)",
}

func (d *deepTrace) result(describe func(string) string) ([]Driver, map[int]worst) {
	mods := d.mods
	if len(mods) < 10 {
		mods = append(mods, enumDrivers()...)
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i].base < mods[j].base })
	find := func(addr uint64) (module, bool) {
		i := sort.Search(len(mods), func(i int) bool { return mods[i].base > addr }) - 1
		if i < 0 {
			return module{}, false
		}
		m := mods[i]
		if m.size > 0 && addr >= m.base+m.size {
			return module{}, false
		}
		return m, true
	}
	byFile := map[string]*Driver{}
	paths := map[string]string{}
	us := func(t int64) float64 { return float64(t) * 1e6 / d.freq }
	for addr, rt := range d.routines {
		file := "desconocido"
		if m, ok := find(addr); ok {
			file = baseName(m.file)
			paths[strings.ToLower(file)] = m.file
		}
		k := strings.ToLower(file)
		dr := byFile[k]
		if dr == nil {
			dr = &Driver{File: file}
			byFile[k] = dr
		}
		dr.Count += rt.count
		dr.TotalMs += us(rt.total) / 1000
		if m := us(rt.max); m > dr.MaxUs {
			dr.MaxUs = m
		}
		dr.Over500 += rt.over500
		if rt.isr*2 > rt.count {
			dr.ISR = true
		}
	}
	var out []Driver
	for k, dr := range byFile {
		if desc, ok := knownDrivers[k]; ok {
			dr.Desc = desc
		} else if describe != nil {
			p := paths[k]
			if strings.HasPrefix(p, `\SystemRoot\`) {
				p = os.Getenv("SystemRoot") + p[len(`\SystemRoot`):]
			} else if !strings.Contains(p, `:`) {
				p = os.Getenv("SystemRoot") + `\System32\drivers\` + dr.File
			}
			dr.Desc = describe(p)
		}
		dr.TotalMs = math.Round(dr.TotalMs*100) / 100
		dr.MaxUs = math.Round(dr.MaxUs)
		switch {
		case dr.MaxUs >= 2000:
			dr.Level = "bad"
		case dr.MaxUs >= 500:
			dr.Level = "warn"
		default:
			dr.Level = "ok"
		}
		out = append(out, *dr)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MaxUs != out[j].MaxUs {
			return out[i].MaxUs > out[j].MaxUs
		}
		return out[i].TotalMs > out[j].TotalMs
	})
	if len(out) > 12 {
		out = out[:12]
	}
	perCore := map[int]worst{}
	for cpu, w := range d.cores {
		file := "desconocido"
		if m, ok := find(w.addr); ok {
			file = baseName(m.file)
		}
		perCore[cpu] = worst{file: file, us: math.Round(us(w.max))}
	}
	return out, perCore
}
