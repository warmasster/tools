// Package sensors lee los sensores del equipo con lo que trae Windows y el
// driver de la gráfica: sin drivers propios ni servicios.
//
// Está pensado para gastar lo mínimo: una lectura completa cuesta menos de un
// milisegundo de CPU, y en modo ligero solo se leen procesador y memoria.
package sensors

import (
	"math"
	"runtime"
	"strings"
	"sync"
	"time"
)

// CPU es el estado del procesador.
type CPU struct {
	CPUInfo
	Usage   float64   `json:"usage"`   // %
	MaxCore float64   `json:"maxCore"` // % del hilo más ocupado
	PerCore []float64 `json:"perCore,omitempty"`
	MHz     int       `json:"mhz"`
	DPC     float64   `json:"dpc"`     // % del núcleo más castigado por drivers (DPC + interrupciones)
	PerfCap float64   `json:"perfCap"` // % del rendimiento nominal que Windows deja dar (100 = normal; menos = frenado por temperatura o potencia)
	Temp    float64   `json:"temp"`    // °C (0 = no se sabe)
	TempSrc string    `json:"tempSrc"` // hwinfo o "" (no se sabe)
	PowerW  float64   `json:"powerW"`
}

// Memory es la RAM.
type Memory struct {
	Total       uint64 `json:"total"`
	Used        uint64 `json:"used"`
	Available   uint64 `json:"available"`
	Standby     uint64 `json:"standby"` // caché en espera (Windows la suelta cuando hace falta)
	Free        uint64 `json:"free"`    // de verdad libre (ni en uso ni en espera)
	Commit      uint64 `json:"commit"`
	CommitLimit uint64 `json:"commitLimit"`
	Load        int    `json:"load"` // %
}

// GPU es una tarjeta gráfica.
type GPU struct {
	Adapter
	Usage       float64 `json:"usage"`
	Temp        float64 `json:"temp"`
	VRAMUsed    uint64  `json:"vramUsed"`
	FanRPM      int     `json:"fanRPM"`
	ClockMHz    int     `json:"clockMHz"`
	MemClockMHz int     `json:"memClockMHz"`
}

// Snapshot es una lectura de todo.
type Snapshot struct {
	Time     int64   `json:"time"`
	CPU      CPU     `json:"cpu"`
	Mem      Memory  `json:"mem"`
	GPUs     []GPU   `json:"gpus"`
	DiskRead float64 `json:"diskRead"` // bytes/s
	DiskWrit float64 `json:"diskWrite"`
	DiskBusy float64 `json:"diskBusy"` // %
	NetDown  float64 `json:"netDown"`  // bytes/s
	NetUp    float64 `json:"netUp"`
	Battery  int     `json:"battery"` // % (-1 = sin batería)
	Plugged  bool    `json:"plugged"`
	Uptime   int64   `json:"uptime"` // s
	Full     bool    `json:"full"`   // lectura completa (no ligera)
	// Decode: procesos que están decodificando vídeo con la gráfica (solo si se ha pedido con WatchDecode).
	Decode []ProcUse `json:"-"`
}

// ProcUse es lo que usa un proceso de un motor de la gráfica.
type ProcUse struct {
	PID   uint32
	Luid  string
	Usage float64
}

// MainGPU es la gráfica que más importa: la dedicada, o la que haya.
func (s *Snapshot) MainGPU() *GPU {
	var best *GPU
	for i := range s.GPUs {
		g := &s.GPUs[i]
		if best == nil || (g.Dedicated && !best.Dedicated) || (g.Dedicated == best.Dedicated && g.VRAM > best.VRAM) {
			best = g
		}
	}
	return best
}

// Sampler guarda lo necesario entre lecturas (tiempos anteriores, contadores abiertos).
type Sampler struct {
	info     CPUInfo
	adapters []Adapter
	kmt      map[[2]uint32]uint32

	prev     cpuTimes
	prevCore []sppi
	coreBuf  []sppi

	q                                  *pdhQuery
	perf, diskR, diskW, diskIdle       *pdhCounter
	netDown, netUp                     *pdhCounter
	standbyA, standbyB, standbyC, free *pdhCounter
	gpuEngine, gpuMem                  *pdhCounter
	perfLimit, decode                  *pdhCounter
	procGPU                            map[uint32]ProcUse
	lastFull                           time.Time
	hwinfoTry                          time.Time
	hwinfoOK                           bool
}

// NewSampler prepara los sensores. Tarda unos milisegundos.
func NewSampler() *Sampler {
	s := &Sampler{info: readCPUInfo()}
	if s.info.Threads == 0 {
		s.info.Threads = runtime.NumCPU()
	}
	n := runtime.NumCPU()
	if n > 64 {
		n = 64
	}
	s.coreBuf = make([]sppi, n)
	s.prev, _ = systemTimes()
	s.prevCore = append([]sppi(nil), coreTimes(s.coreBuf)...)
	s.adapters = Adapters()
	s.kmt = kmtHandles()

	if q, err := newQuery(); err == nil {
		s.q = q
		s.perf = q.add(`\Processor Information(_Total)\% Processor Performance`)
		s.perfLimit = q.add(`\Processor Information(_Total)\% Performance Limit`)
		s.diskR = q.add(`\PhysicalDisk(_Total)\Disk Read Bytes/sec`)
		s.diskW = q.add(`\PhysicalDisk(_Total)\Disk Write Bytes/sec`)
		s.diskIdle = q.add(`\PhysicalDisk(_Total)\% Idle Time`)
		s.netDown = q.add(`\Network Interface(*)\Bytes Received/sec`)
		s.netUp = q.add(`\Network Interface(*)\Bytes Sent/sec`)
		s.standbyA = q.add(`\Memory\Standby Cache Reserve Bytes`)
		s.standbyB = q.add(`\Memory\Standby Cache Normal Priority Bytes`)
		s.standbyC = q.add(`\Memory\Standby Cache Core Bytes`)
		s.free = q.add(`\Memory\Free & Zero Page List Bytes`)
		if len(s.adapters) > 0 {
			s.gpuEngine = q.add(`\GPU Engine(*engtype_3D)\Utilization Percentage`)
			s.gpuMem = q.add(`\GPU Adapter Memory(*)\Dedicated Usage`)
		}
		q.collect()
	}
	return s
}

// Info devuelve los datos fijos del procesador.
func (s *Sampler) Info() CPUInfo { return s.info }

// GPUAdapters devuelve las tarjetas gráficas.
func (s *Sampler) GPUAdapters() []Adapter { return s.adapters }

// Close suelta los contadores.
func (s *Sampler) Close() {
	if s.q != nil {
		s.q.close()
	}
}

// Sample hace una lectura. Con full=false solo lee procesador y memoria (lo
// que cuesta casi nada), para cuando nadie está mirando.
func (s *Sampler) Sample(full bool) Snapshot {
	snap := Snapshot{Time: time.Now().UnixMilli(), Full: full, Battery: -1}
	snap.CPU.CPUInfo = s.info

	if t, ok := systemTimes(); ok {
		dt := t.total - s.prev.total
		di := t.idle - s.prev.idle
		if dt > 0 {
			snap.CPU.Usage = clamp(100 - pct(di, dt))
		}
		s.prev = t
	}
	if cur := coreTimes(s.coreBuf); len(cur) > 0 && len(cur) == len(s.prevCore) {
		snap.CPU.PerCore = make([]float64, len(cur))
		for i := range cur {
			tot := uint64((cur[i].Kernel + cur[i].User) - (s.prevCore[i].Kernel + s.prevCore[i].User))
			idle := uint64(cur[i].Idle - s.prevCore[i].Idle)
			u := 0.0
			if tot > 0 {
				u = clamp(100 - pct(idle, tot))
			}
			snap.CPU.PerCore[i] = round1(u)
			if u > snap.CPU.MaxCore {
				snap.CPU.MaxCore = u
			}
			if tot > 0 {
				d := pct(uint64((cur[i].Dpc-s.prevCore[i].Dpc)+(cur[i].Interrupt-s.prevCore[i].Interrupt)), tot)
				if d > snap.CPU.DPC {
					snap.CPU.DPC = round1(d)
				}
			}
		}
		copy(s.prevCore, cur)
	} else if len(cur) > 0 {
		s.prevCore = append(s.prevCore[:0], cur...)
	}

	if m, ok := memoryStatus(); ok {
		snap.Mem.Total = m.TotalPhys
		snap.Mem.Available = m.AvailPhys
		snap.Mem.Used = m.TotalPhys - m.AvailPhys
		snap.Mem.Commit = m.TotalPage - m.AvailPage
		snap.Mem.CommitLimit = m.TotalPage
		snap.Mem.Load = int(m.MemoryLoad)
	}

	if !full {
		snap.CPU.Usage = round1(snap.CPU.Usage)
		snap.CPU.MaxCore = round1(snap.CPU.MaxCore)
		return snap
	}

	s.q.collect()
	if v, ok := s.perf.value(); ok && s.info.BaseMHz > 0 {
		snap.CPU.MHz = int(v * float64(s.info.BaseMHz) / 100)
	}
	if v, ok := s.perfLimit.value(); ok {
		snap.CPU.PerfCap = round1(v)
	}
	snap.DiskRead, _ = s.diskR.value()
	snap.DiskWrit, _ = s.diskW.value()
	if v, ok := s.diskIdle.value(); ok {
		snap.DiskBusy = round1(clamp(100 - v))
	}
	for _, it := range s.netDown.array() {
		if !virtualNIC(it.Name) {
			snap.NetDown += it.Value
		}
	}
	for _, it := range s.netUp.array() {
		if !virtualNIC(it.Name) {
			snap.NetUp += it.Value
		}
	}
	var sb uint64
	for _, c := range []*pdhCounter{s.standbyA, s.standbyB, s.standbyC} {
		if v, ok := c.value(); ok {
			sb += uint64(v)
		}
	}
	snap.Mem.Standby = sb
	if v, ok := s.free.value(); ok {
		snap.Mem.Free = uint64(v)
	}

	// Temperatura del procesador: HWiNFO si está abierto (se mira cada 10 s si
	// no lo estaba). La «zona térmica ACPI» de Windows no se usa: en casi todos
	// los PC de sobremesa es un valor fijo o inventado (27,8 °C, 17 °C…).
	if s.hwinfoOK || time.Since(s.hwinfoTry) > 10*time.Second {
		s.hwinfoTry = time.Now()
		if r, ok := readHWiNFO(); ok {
			s.hwinfoOK = true
			snap.CPU.Temp, snap.CPU.TempSrc, snap.CPU.PowerW = round1(r.cpuTemp), "hwinfo", round1(r.cpuPower)
		} else {
			s.hwinfoOK = false
		}
	}

	engine := map[string]float64{}
	type pidLuid struct {
		pid  uint32
		luid string
	}
	perPid := map[pidLuid]float64{}
	for _, it := range s.gpuEngine.array() {
		if k := luidOf(it.Name); k != "" {
			engine[k] += it.Value
			if pid := pidOf(it.Name); pid != 0 && it.Value > 0 {
				perPid[pidLuid{pid, k}] += it.Value
			}
		}
	}
	pg := map[uint32]ProcUse{}
	for k, v := range perPid {
		if v > pg[k.pid].Usage {
			pg[k.pid] = ProcUse{PID: k.pid, Luid: k.luid, Usage: round1(v)}
		}
	}
	s.procGPU = pg
	if s.decode != nil {
		for _, it := range s.decode.array() {
			if pid := pidOf(it.Name); pid != 0 && it.Value >= 1 {
				snap.Decode = append(snap.Decode, ProcUse{PID: pid, Luid: luidOf(it.Name), Usage: round1(it.Value)})
			}
		}
	}
	vram := map[string]uint64{}
	for _, it := range s.gpuMem.array() {
		if k := luidOf(it.Name); k != "" {
			vram[k] += uint64(it.Value)
		}
	}
	for _, a := range s.adapters {
		g := GPU{Adapter: a}
		if h, ok := s.kmt[[2]uint32{a.LuidLow, uint32(a.LuidHigh)}]; ok {
			if p, ok := kmtPerf(h); ok {
				g.Temp = float64(p.Temperature) / 10
				g.FanRPM = int(p.FanRPM)
				if p.MemoryFrequency > 0 {
					g.MemClockMHz = int(p.MemoryFrequency / 1e6)
				}
			}
			g.ClockMHz = kmtClock(h)
		}
		k := a.luidKey()
		g.Usage = round1(clamp(engine[k]))
		g.VRAMUsed = vram[k]
		snap.GPUs = append(snap.GPUs, g)
	}

	if has, plugged, p := Battery(); has {
		snap.Battery, snap.Plugged = p, plugged
	} else {
		snap.Plugged = true
	}
	snap.Uptime = uptimeSeconds()
	snap.CPU.Usage = round1(snap.CPU.Usage)
	snap.CPU.MaxCore = round1(snap.CPU.MaxCore)
	s.lastFull = time.Now()
	return snap
}

var (
	sbMu sync.Mutex
	sbQ  *pdhQuery
	sbC  []*pdhCounter
)

// StandbyBytes lee al momento cuánta memoria hay «en espera» (con su propia consulta).
func StandbyBytes() uint64 {
	sbMu.Lock()
	defer sbMu.Unlock()
	if sbQ == nil {
		q, err := newQuery()
		if err != nil {
			return 0
		}
		sbQ = q
		for _, p := range []string{`\Memory\Standby Cache Reserve Bytes`, `\Memory\Standby Cache Normal Priority Bytes`, `\Memory\Standby Cache Core Bytes`} {
			sbC = append(sbC, q.add(p))
		}
	}
	sbQ.collect()
	var t uint64
	for _, c := range sbC {
		if v, ok := c.value(); ok {
			t += uint64(v)
		}
	}
	return t
}

// WatchDecode empieza o deja de mirar qué procesos decodifican vídeo con la
// gráfica (cuesta algo: solo durante las partidas con dos pantallas).
func (s *Sampler) WatchDecode(on bool) {
	if s.q == nil || on == (s.decode != nil) {
		return
	}
	if on {
		s.decode = s.q.add(`\GPU Engine(*engtype_VideoDecode)\Utilization Percentage`)
		return
	}
	s.q.remove(s.decode)
	s.decode = nil
}

// ProcGPU dice en qué gráfica (luid) dibuja un proceso y cuánto la usa (última lectura completa).
func (s *Sampler) ProcGPU(pid uint32) (ProcUse, bool) {
	u, ok := s.procGPU[pid]
	return u, ok
}

// LuidKey es la clave de una gráfica como la usan ProcUse y los contadores.
func (a Adapter) LuidKey() string { return a.luidKey() }

// pidOf saca el proceso de «pid_1234_luid_…».
func pidOf(name string) uint32 {
	n := strings.ToLower(name)
	if !strings.HasPrefix(n, "pid_") {
		return 0
	}
	v := uint32(0)
	for _, c := range n[4:] {
		if c < '0' || c > '9' {
			break
		}
		v = v*10 + uint32(c-'0')
	}
	return v
}

// luidOf saca «0x00000000_0x0000d1f3» de un nombre de instancia de los contadores de la GPU.
func luidOf(name string) string {
	n := strings.ToLower(name)
	i := strings.Index(n, "luid_")
	if i < 0 || len(n) < i+5+21 {
		return ""
	}
	return n[i+5 : i+5+21]
}

// virtualNIC: adaptadores que duplicarían el tráfico (Hyper-V, VPN en bucle…).
func virtualNIC(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "loopback") || strings.Contains(n, "isatap") || strings.Contains(n, "teredo") || strings.Contains(n, "hyper-v virtual") || strings.Contains(n, "vethernet")
}

func clamp(v float64) float64 {
	if math.IsNaN(v) || v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
