// Package checks revisa que el equipo esté configurado para rendir lo que
// debe en juegos: la RAM a su velocidad, la pantalla a sus hercios, la gráfica
// bien conectada, la energía sin límites… Cada comprobación dice qué hay, por
// qué importa y cómo se arregla; las que se pueden arreglar sin riesgo desde
// aquí traen su arreglo.
package checks

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows/registry"

	"turbomon/internal/parts"
	"turbomon/internal/sensors"
)

// Estados.
const (
	OK   = "ok"
	Warn = "warn"
	Bad  = "bad"
	Info = "info"
)

// Result es una comprobación.
type Result struct {
	ID       string   `json:"id"`
	Group    string   `json:"group"`
	Title    string   `json:"title"`
	Status   string   `json:"status"`
	Value    string   `json:"value"`  // lo que hay, en corto
	Detail   string   `json:"detail"` // por qué importa, en lenguaje normal
	Fix      string   `json:"fix,omitempty"`
	FixLabel string   `json:"fixLabel,omitempty"`
	HowTo    []string `json:"howTo,omitempty"`
	Weight   int      `json:"weight"`
}

// Report es la revisión completa.
type Report struct {
	Score   int      `json:"score"`
	Results []Result `json:"results"`
	Time    int64    `json:"time"`
}

// Run hace todas las comprobaciones (tarda menos de un segundo). comp son las
// piezas del equipo (las lee el agente, que con permiso de administrador ve
// también la salud de los discos); si es nil se leen aquí.
func Run(comp *parts.Components) Report {
	if comp == nil {
		c := parts.Read(sensors.TotalRAM())
		comp = &c
	}
	hasBattery, plugged, _ := sensors.Battery()
	adapters := sensors.Adapters()
	monitors := comp.Monitors
	var rs []Result
	rs = append(rs, checkRAM(hasBattery)...)
	rs = append(rs, checkMonitors(monitors)...)
	if r, ok := checkMonitorGPU(monitors, adapters, hasBattery); ok {
		rs = append(rs, r)
	}
	if r, ok := checkDualMonitor(monitors); ok {
		rs = append(rs, r)
	}
	if r, ok := checkDriver(adapters); ok {
		rs = append(rs, r)
	}
	rs = append(rs, checkHAGS())
	rs = append(rs, checkPower(hasBattery)...)
	if hasBattery {
		rs = append(rs, checkPlugged(plugged))
	}
	rs = append(rs, checkGameMode(), checkGameDVR(), checkPagefile())
	rs = append(rs, checkDisks()...)
	rs = append(rs, checkMouse(), checkStartup())
	rs = append(rs, tailoredChecks(*comp)...)
	var out []Result
	for _, r := range rs {
		if r.Status != "" {
			out = append(out, r)
		}
	}
	order := map[string]int{"Memoria": 0, "Procesador": 1, "Pantalla": 2, "Gráfica": 3, "Energía": 4, "Windows": 5, "Red": 6, "Discos": 7}
	sort.SliceStable(out, func(i, j int) bool { return order[out[i].Group] < order[out[j].Group] })
	return Report{Score: Score(out), Results: out, Time: time.Now().Unix()}
}

// checkDualMonitor: dos pantallas a hercios distintos son la causa más típica
// de tirones con un vídeo en la otra pantalla.
func checkDualMonitor(ms []Monitor) (Result, bool) {
	if len(ms) < 2 {
		return Result{}, false
	}
	hz := map[int]bool{}
	var list []string
	for _, m := range ms {
		hz[m.Hz] = true
		list = append(list, fmt.Sprintf("%d", m.Hz))
	}
	r := Result{ID: "dual_monitor", Group: "Pantalla", Title: "Dos pantallas", Weight: 0, Value: strings.Join(list, " y ") + " Hz"}
	mpo := "activado"
	if v, ok := regDword(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\Dwm`, "OverlayTestMode"); ok && v == 5 {
		mpo = "desactivado"
	}
	if len(hz) > 1 {
		r.Status = Info
		r.Detail = "Tus pantallas van a hercios distintos. Es lo normal, pero con un vídeo en la otra pantalla a algunos PC les da tirones en el juego (MPO " + mpo + "). Si te pasa, TurboMON lo detecta en «Solucionar» > «Dos pantallas»."
	} else {
		r.Status = OK
		r.Detail = "Las pantallas van a los mismos hercios: es lo que menos problemas da con un vídeo en la otra."
	}
	return r, true
}

// Score: 100 menos lo que resta cada cosa mal (entera) o mejorable (la mitad).
func Score(rs []Result) int {
	s := 100.0
	for _, r := range rs {
		switch r.Status {
		case Bad:
			s -= float64(r.Weight)
		case Warn:
			s -= float64(r.Weight) / 2
		}
	}
	if s < 0 {
		s = 0
	}
	return int(math.Round(s))
}

func gb(b uint64) string {
	v := float64(b) / (1 << 30)
	if v >= 10 {
		return strings.Replace(fmt.Sprintf("%.0f GB", v), ".", ",", 1)
	}
	return strings.Replace(fmt.Sprintf("%.1f GB", v), ".", ",", 1)
}

// ---------- memoria ----------

func checkRAM(laptop bool) []Result {
	var out []Result
	total := sensors.TotalRAM()
	totalGB := int(math.Round(float64(total) / (1 << 30)))
	amount := Result{ID: "ram_amount", Group: "Memoria", Title: "Cantidad de RAM", Weight: 8, Value: fmt.Sprintf("%d GB", totalGB)}
	switch {
	case totalGB < 8:
		amount.Status, amount.Detail = Bad, "Con menos de 8 GB los juegos actuales van a tirones o no arrancan. Lo mínimo hoy son 16 GB."
	case totalGB < 15:
		amount.Status, amount.Detail = Warn, "Con 8 GB muchos juegos actuales van justos (sobre todo con el navegador o Discord abiertos). Lo recomendable son 16 GB."
	default:
		amount.Status, amount.Detail = OK, "Suficiente para jugar."
	}
	dimms := DIMMs()
	if len(dimms) == 0 {
		return []Result{amount}
	}
	configured, maxSpec, rated := 0, 0, 0
	typ := dimms[0].Type
	for _, d := range dimms {
		c := d.Configured
		if c == 0 {
			c = d.Speed
		}
		if configured == 0 || (c > 0 && c < configured) {
			configured = c
		}
		if d.Speed > maxSpec {
			maxSpec = d.Speed
		}
		if d.Rated > rated {
			rated = d.Rated
		}
	}
	target := rated
	if maxSpec > target {
		target = maxSpec
	}
	xmp := Result{ID: "ram_xmp", Group: "Memoria", Title: "Velocidad de la RAM (XMP / EXPO)", Weight: 15}
	profile := "XMP"
	if strings.Contains(strings.ToLower(cpuName()), "amd") {
		profile = "EXPO"
	}
	howXMP := []string{
		"Reinicia el PC y entra en la BIOS (suele ser la tecla Supr o F2 justo al encender).",
		"Busca «XMP», «EXPO», «DOCP» o «A-XMP». Suele estar en la pantalla principal o en «AI Tweaker», «OC» o «Extreme Tweaker».",
		"Ponlo en «Perfil 1» (o «Enabled»), guarda con F10 y reinicia.",
		"Vuelve aquí y pulsa «Revisar otra vez». Si el PC no arranca, se queda solo como estaba tras unos reinicios.",
	}
	label := typ
	if label == "" {
		label = "RAM"
	}
	switch {
	case configured > 0 && target > 0 && configured < target*95/100:
		xmp.Value = fmt.Sprintf("Va a %d de %d MT/s", configured, target)
		if laptop {
			xmp.Status = Info
			xmp.Detail = fmt.Sprintf("Tu memoria podría ir a %d MT/s pero va a %d. En los portátiles casi nunca se puede cambiar: es normal.", target, configured)
		} else {
			xmp.Status = Bad
			xmp.Detail = fmt.Sprintf("Tu RAM está hecha para ir a %d MT/s pero va a %d: le falta activar el perfil %s en la BIOS. Es seguro (es la velocidad que pone en la caja) y da más FPS, sobre todo en los juegos que tiran del procesador.", target, configured, profile)
			xmp.HowTo = howXMP
		}
	case !laptop && rated == 0 && ((typ == "DDR4" && configured > 0 && configured <= 2666) || (typ == "DDR5" && configured > 0 && configured <= 4800)):
		xmp.Status = Warn
		xmp.Value = fmt.Sprintf("%s a %d MT/s", label, configured)
		xmp.Detail = fmt.Sprintf("Va a la velocidad básica de la %s. Casi toda la RAM para jugar puede ir más rápido con el perfil %s de la BIOS: mira en la etiqueta del módulo o en la caja si pone más de %d.", label, profile, configured)
		xmp.HowTo = howXMP
	default:
		xmp.Status = OK
		xmp.Value = fmt.Sprintf("%s a %d MT/s", label, configured)
		xmp.Detail = "Tu memoria va a la velocidad para la que está hecha."
		if rated > 0 && configured >= rated {
			xmp.Detail = fmt.Sprintf("Tu memoria va a %d MT/s, su velocidad de fábrica: el perfil %s está activado.", configured, profile)
		}
	}
	out = append(out, xmp)

	ch := Result{ID: "ram_channels", Group: "Memoria", Title: "Doble canal", Weight: 12}
	if len(dimms) == 1 {
		ch.Value = "1 módulo"
		if laptop {
			ch.Status = Warn
		} else {
			ch.Status = Bad
		}
		ch.Detail = "Con un solo módulo la memoria trabaja en un canal: la mitad de velocidad. Con dos módulos iguales los juegos ganan FPS, sobre todo en los tirones (el 1 % bajo)."
		if !laptop {
			ch.HowTo = []string{"Compra otro módulo igual (o un kit de dos) y ponlos en las ranuras que diga el manual de la placa para «dual channel» (casi siempre la 2.ª y la 4.ª contando desde el procesador)."}
		}
	} else {
		chans := map[string]int{}
		known := 0
		for _, d := range dimms {
			if c := Channel(d); c != "" {
				chans[c]++
				known++
			}
		}
		ch.Value = fmt.Sprintf("%d módulos", len(dimms))
		if known == len(dimms) && len(chans) == 1 && len(dimms) == 2 {
			ch.Status = Warn
			var c string
			for k := range chans {
				c = k
			}
			ch.Detail = fmt.Sprintf("Los dos módulos parecen estar en el mismo canal (%s): así no hay doble canal. Mira en el manual de la placa qué ranuras usar (casi siempre la 2.ª y la 4.ª).", c)
			ch.HowTo = []string{"Apaga el PC, cambia uno de los módulos a la otra ranura que diga el manual para «dual channel» y vuelve a revisar."}
		} else {
			ch.Status = OK
			ch.Detail = "La memoria trabaja en doble canal."
		}
	}
	out = append(out, ch, amount)
	return out
}

func cpuName() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	s, _, _ := k.GetStringValue("ProcessorNameString")
	return s
}

// ---------- pantalla y gráfica ----------

func checkMonitors(ms []Monitor) []Result {
	var out []Result
	for _, m := range ms {
		name := m.Name
		if len(ms) == 1 {
			name = "tu pantalla"
		}
		hz := Result{ID: "monitor_hz:" + m.Device, Group: "Pantalla", Title: "Hercios de " + name, Weight: 15}
		if m.Hz < m.MaxHz-1 {
			hz.Status = Bad
			hz.Value = fmt.Sprintf("%d de %d Hz", m.Hz, m.MaxHz)
			hz.Detail = fmt.Sprintf("Tu pantalla puede ir a %d Hz pero Windows la tiene a %d. Es muy común (Windows la pone a 60 al conectarla) y se nota muchísimo: todo se ve más fluido.", m.MaxHz, m.Hz)
			hz.Fix, hz.FixLabel = "hz:"+m.Device, fmt.Sprintf("Poner a %d Hz", m.MaxHz)
			hz.HowTo = []string{"Si después no llegan los hercios, comprueba que el cable es DisplayPort o HDMI 2.0/2.1 y que está en la tarjeta gráfica."}
		} else {
			hz.Status = OK
			hz.Value = fmt.Sprintf("%d Hz", m.Hz)
			hz.Detail = "Va a los hercios más altos que admite."
			if m.MaxHz <= 60 {
				hz.Detail = "Va a su máximo (60 Hz). Una pantalla de 144 Hz o más se nota mucho en juegos rápidos."
			}
		}
		out = append(out, hz)
		if m.MaxWidth*m.MaxHeight > m.Width*m.Height {
			out = append(out, Result{
				ID: "monitor_res:" + m.Device, Group: "Pantalla", Title: "Resolución de " + name, Weight: 4, Status: Warn,
				Value:  fmt.Sprintf("%d×%d de %d×%d", m.Width, m.Height, m.MaxWidth, m.MaxHeight),
				Detail: fmt.Sprintf("Windows está a %d×%d y tu pantalla es de %d×%d: se ve menos nítido. (Los juegos pueden ir a menos resolución si quieres más FPS; esto es solo el escritorio).", m.Width, m.Height, m.MaxWidth, m.MaxHeight),
				Fix:    "res:" + m.Device, FixLabel: fmt.Sprintf("Poner %d×%d", m.MaxWidth, m.MaxHeight),
			})
		}
	}
	return out
}

func checkMonitorGPU(ms []Monitor, ads []sensors.Adapter, laptop bool) (Result, bool) {
	if laptop || len(ms) == 0 {
		return Result{}, false
	}
	var dedicated, integrated []sensors.Adapter
	for _, a := range ads {
		if a.Dedicated {
			dedicated = append(dedicated, a)
		} else {
			integrated = append(integrated, a)
		}
	}
	if len(dedicated) == 0 {
		return Result{}, false
	}
	onDed, onInt := 0, 0
	for _, m := range ms {
		for _, a := range dedicated {
			if sensors.SameLUID(m.LuidLow, m.LuidHigh, a) {
				onDed++
			}
		}
		for _, a := range integrated {
			if sensors.SameLUID(m.LuidLow, m.LuidHigh, a) {
				onInt++
			}
		}
	}
	r := Result{ID: "monitor_gpu", Group: "Pantalla", Title: "Pantalla conectada a la gráfica", Weight: 20}
	switch {
	case onDed == 0 && onInt > 0:
		r.Status = Bad
		r.Value = "Conectada a la placa base"
		r.Detail = fmt.Sprintf("El cable de la pantalla está en la placa base, no en la %s: los juegos van muchísimo peor (o usan la gráfica integrada). Es de los fallos más comunes.", dedicated[0].Name)
		r.HowTo = []string{"Apaga el PC y mira detrás: los conectores de la placa base están arriba, en vertical; los de la tarjeta gráfica, más abajo y en horizontal.", "Pasa el cable de la pantalla a uno de la tarjeta gráfica y enciende."}
	case onInt > 0:
		r.Status = Info
		r.Value = fmt.Sprintf("%d en la gráfica, %d en la placa", onDed, onInt)
		r.Detail = "Una pantalla va a la gráfica y otra a la placa base. Juega en la que está en la tarjeta gráfica."
	default:
		r.Status = OK
		r.Value = "En la " + dedicated[0].Name
		r.Detail = "La pantalla está conectada a la tarjeta gráfica, como debe."
	}
	return r, true
}

func checkDriver(ads []sensors.Adapter) (Result, bool) {
	var main *sensors.Adapter
	for i := range ads {
		if main == nil || (ads[i].Dedicated && !main.Dedicated) {
			main = &ads[i]
		}
	}
	if main == nil {
		return Result{}, false
	}
	date, ver := driverDate(main.Name)
	if date.IsZero() {
		return Result{}, false
	}
	days := int(time.Since(date).Hours() / 24)
	r := Result{ID: "gpu_driver", Group: "Gráfica", Title: "Driver de la gráfica", Weight: 6,
		Value: fmt.Sprintf("Del %s", date.Format("02/01/2006"))}
	if ver != "" {
		r.Value += " · " + ver
	}
	how := map[string]string{
		"nvidia": "Abre la «NVIDIA App» (o descárgala de nvidia.com) y pulsa «Descargar» en Controladores.",
		"amd":    "Abre «AMD Software: Adrenalin Edition» (o descárgalo de amd.com) y busca actualizaciones.",
		"intel":  "Usa «Intel Driver & Support Assistant» (intel.es) o Windows Update.",
	}[main.Vendor]
	switch {
	case days > 365:
		r.Status, r.Detail = Bad, fmt.Sprintf("El driver tiene más de un año (%d días). Los nuevos traen mejoras de FPS y arreglos para los juegos recientes.", days)
	case days > 180:
		r.Status, r.Detail = Warn, fmt.Sprintf("El driver tiene %d meses. Merece la pena actualizarlo, sobre todo si juegas a cosas nuevas.", days/30)
	default:
		r.Status, r.Detail = OK, "El driver es reciente."
	}
	if r.Status != OK && how != "" {
		r.HowTo = []string{how}
	}
	return r, true
}

// driverDate busca en el registro la fecha del driver de una tarjeta.
func driverDate(name string) (time.Time, string) {
	const cls = `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, cls, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return time.Time{}, ""
	}
	defer k.Close()
	subs, _ := k.ReadSubKeyNames(-1)
	for _, s := range subs {
		sk, err := registry.OpenKey(registry.LOCAL_MACHINE, cls+`\`+s, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		desc, _, _ := sk.GetStringValue("DriverDesc")
		date, _, _ := sk.GetStringValue("DriverDate")
		ver, _, _ := sk.GetStringValue("DriverVersion")
		sk.Close()
		if !strings.EqualFold(strings.TrimSpace(desc), name) {
			continue
		}
		// «8-24-2026» (mes-día-año)
		parts := strings.Split(date, "-")
		if len(parts) != 3 {
			continue
		}
		mo, _ := strconv.Atoi(parts[0])
		d, _ := strconv.Atoi(parts[1])
		y, _ := strconv.Atoi(parts[2])
		if y < 2000 {
			continue
		}
		return time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.Local), nvidiaVersion(ver)
	}
	return time.Time{}, ""
}

func checkHAGS() Result {
	r := Result{ID: "hags", Group: "Gráfica", Title: "Programación de GPU acelerada por hardware", Weight: 0}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\GraphicsDrivers`, registry.QUERY_VALUE)
	if err != nil {
		return Result{}
	}
	v, _, err := k.GetIntegerValue("HwSchMode")
	k.Close()
	if err != nil {
		return Result{} // la gráfica no lo admite
	}
	if v == 2 {
		r.Status, r.Value, r.Detail = OK, "Activada", "La necesita la generación de fotogramas (DLSS 3 de NVIDIA, por ejemplo) y suele bajar un poco la latencia."
		return r
	}
	r.Status, r.Value = Info, "Desactivada"
	r.Detail = "Activarla suele bajar un poco la latencia y la necesita la generación de fotogramas (DLSS 3 de NVIDIA). En algunos juegos no cambia nada."
	r.HowTo = []string{"Configuración > Sistema > Pantalla > Gráficos > «Cambiar la configuración de gráficos predeterminada».", "Activa «Programación de GPU acelerada por hardware» y reinicia."}
	return r
}

// ---------- energía ----------

func checkPower(laptop bool) []Result {
	var out []Result
	plan := Result{ID: "power_plan", Group: "Energía", Title: "Plan de energía", Weight: 8}
	if p, err := ActivePlan(); err == nil {
		plan.Value = p.Name
		lname := strings.ToLower(p.Name)
		switch {
		case p.GUID == PlanSaver || strings.Contains(lname, "ahorro") || strings.Contains(lname, "saver"):
			plan.Status = Bad
			plan.Detail = "Con «Economizador» Windows frena el procesador a propósito: los juegos van peor."
			plan.Fix, plan.FixLabel = "plan", "Cambiar a "+map[bool]string{true: "«Equilibrado»", false: "más rendimiento"}[laptop]
		default:
			if OverlayMode() == "efficiency" {
				plan.Status = Warn
				plan.Value += " · mejor eficiencia"
				plan.Detail = "El modo de energía de Windows está en «Mejor eficiencia energética»: limita el procesador para gastar menos."
				plan.HowTo = []string{"Configuración > Sistema > Energía (y batería) > Modo de energía: «Mejor rendimiento» o «Máximo rendimiento»."}
			} else {
				plan.Status = OK
				plan.Detail = "No limita el rendimiento."
			}
		}
		out = append(out, plan)
	}
	if ac, _, err := ProcessorMax(); err == nil {
		r := Result{ID: "cpu_max", Group: "Energía", Title: "Procesador sin límite", Weight: 10, Value: fmt.Sprintf("Máximo al %d %%", ac)}
		if ac < 100 {
			r.Status = Bad
			r.Detail = fmt.Sprintf("El plan de energía no deja pasar al procesador del %d %%: va más lento de lo que puede. Suele quedar así por algún programa de «ahorro».", ac)
			r.Fix, r.FixLabel = "cpumax", "Quitar el límite"
		} else {
			r.Status, r.Value, r.Detail = OK, "Al 100 %", "El procesador puede dar todo lo que tiene."
		}
		out = append(out, r)
	}
	return out
}

func checkPlugged(plugged bool) Result {
	r := Result{ID: "plugged", Group: "Energía", Title: "Portátil enchufado", Weight: 6}
	if plugged {
		r.Status, r.Value, r.Detail = OK, "Enchufado", "Con el cargador puesto el portátil rinde al máximo."
	} else {
		r.Status, r.Value, r.Detail = Warn, "Con batería", "Sin cargador, Windows y la gráfica bajan el rendimiento a propósito para durar más. Para jugar, enchúfalo."
	}
	return r
}

// ---------- Windows ----------

func regDword(root registry.Key, path, name string) (uint64, bool) {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return 0, false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue(name)
	return v, err == nil
}

func checkGameMode() Result {
	r := Result{ID: "game_mode", Group: "Windows", Title: "Modo de juego de Windows", Weight: 3}
	v, ok := regDword(registry.CURRENT_USER, `Software\Microsoft\GameBar`, "AutoGameModeEnabled")
	if ok && v == 0 {
		r.Status, r.Value = Warn, "Desactivado"
		r.Detail = "El modo de juego da prioridad al juego y evita que Windows Update se ponga a instalar mientras juegas."
		r.Fix, r.FixLabel = "gamemode", "Activarlo"
		return r
	}
	r.Status, r.Value, r.Detail = OK, "Activado", "Windows da prioridad al juego y no instala actualizaciones mientras juegas."
	return r
}

func checkGameDVR() Result {
	r := Result{ID: "game_dvr", Group: "Windows", Title: "Grabación en segundo plano (Xbox)", Weight: 5}
	app, okA := regDword(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\GameDVR`, "AppCaptureEnabled")
	hist, okH := regDword(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\GameDVR`, "HistoricalCaptureEnabled")
	if okH && hist == 1 && (!okA || app != 0) {
		r.Status, r.Value = Warn, "Grabando siempre"
		r.Detail = "La Xbox Game Bar está grabando siempre los últimos minutos para poder guardarlos: cuesta FPS todo el rato aunque no lo uses."
		r.Fix, r.FixLabel = "dvr", "Dejar de grabar de fondo"
		return r
	}
	r.Status, r.Value, r.Detail = OK, "Desactivada", "La Xbox Game Bar no está grabando de fondo."
	return r
}

func checkPagefile() Result {
	r := Result{ID: "pagefile", Group: "Windows", Title: "Memoria virtual", Weight: 6}
	pf := sensors.PageFileBytes()
	if pf < 256<<20 {
		r.Status, r.Value = Bad, "Desactivada"
		r.Detail = "Sin memoria virtual, si un juego se queda sin RAM se cierra de golpe. Muchas guías dicen quitarla, pero no gana FPS."
		r.HowTo = []string{"Pulsa Windows, escribe «Ajustar la apariencia y rendimiento de Windows» y ábrelo.", "Opciones avanzadas > Memoria virtual > Cambiar…", "Marca «Administrar automáticamente el tamaño del archivo de paginación» y reinicia."}
		return r
	}
	r.Status, r.Value, r.Detail = OK, gb(pf), "Windows tiene memoria virtual de reserva por si se acaba la RAM."
	return r
}

// ---------- discos ----------

func checkDisks() []Result {
	var out []Result
	sys := os.Getenv("SystemDrive")
	if sys == "" {
		sys = "C:"
	}
	if free, total, ok := FreeSpace(sys); ok {
		r := Result{ID: "disk_free", Group: "Discos", Title: "Espacio libre en " + sys, Weight: 5, Value: gb(free) + " libres"}
		switch {
		case free < 10<<30:
			r.Status, r.Detail = Bad, "Con tan poco espacio Windows va lento, no se pueden instalar actualizaciones y los juegos pueden dar errores al guardar."
		case float64(free) < float64(total)*0.10:
			r.Status, r.Detail = Warn, "Queda menos del 10 %. Libera algo de espacio (TurboCUT te dice qué ocupa más)."
		default:
			r.Status, r.Detail = OK, "Hay espacio de sobra."
		}
		out = append(out, r)
	}
	libs := GameLibraries()
	drives := map[string]int{}
	for _, l := range libs {
		if l.Drive != "" {
			drives[strings.ToUpper(l.Drive)] += l.Games
		}
	}
	r := Result{ID: "games_ssd", Group: "Discos", Title: "Juegos en SSD", Weight: 5}
	if len(drives) == 0 {
		ssd, ok := IsSSD(sys)
		if !ok {
			return out
		}
		r.Title = "Windows en SSD"
		if ssd {
			r.Status, r.Value, r.Detail = OK, sys+" es SSD", "Windows está en un SSD."
		} else {
			r.Status, r.Value, r.Detail = Bad, sys+" es un disco duro", "Windows está en un disco duro: todo arranca y carga muy lento. Un SSD es la mejora que más se nota."
			r.Weight = 10
		}
		return append(out, r)
	}
	var hdd []string
	hddGames := 0
	keys := make([]string, 0, len(drives))
	for d := range drives {
		keys = append(keys, d)
	}
	sort.Strings(keys)
	known := false
	for _, d := range keys {
		ssd, ok := IsSSD(d)
		if !ok {
			continue
		}
		known = true
		if !ssd {
			hdd = append(hdd, d)
			hddGames += drives[d]
		}
	}
	if !known {
		return out
	}
	if len(hdd) > 0 {
		r.Status = Warn
		r.Value = fmt.Sprintf("%d en disco duro (%s)", hddGames, strings.Join(hdd, ", "))
		r.Detail = "Los juegos en disco duro tardan mucho en cargar y pueden dar tirones al entrar en zonas nuevas. Pasa a un SSD los que más juegues (Steam: Propiedades > Archivos instalados > Mover)."
	} else {
		r.Status, r.Value, r.Detail = OK, "Todos en SSD", "Tus juegos están en SSD: cargan rápido."
	}
	return append(out, r)
}

// ---------- ratón y arranque ----------

func checkMouse() Result {
	r := Result{ID: "mouse_accel", Group: "Windows", Title: "Aceleración del ratón", Weight: 2}
	k, err := registry.OpenKey(registry.CURRENT_USER, `Control Panel\Mouse`, registry.QUERY_VALUE)
	if err != nil {
		return Result{}
	}
	v, _, _ := k.GetStringValue("MouseSpeed")
	k.Close()
	if strings.TrimSpace(v) != "0" && v != "" {
		r.Status, r.Value = Warn, "Activada"
		r.Detail = "«Mejorar la precisión del puntero» hace que el ratón vaya más o menos lejos según lo rápido que lo muevas. Para apuntar en juegos estorba: la memoria muscular no funciona."
		r.Fix, r.FixLabel = "mouse", "Desactivarla"
		return r
	}
	r.Status, r.Value, r.Detail = OK, "Desactivada", "El ratón se mueve siempre lo mismo para el mismo gesto: ideal para apuntar."
	return r
}

func checkStartup() Result {
	n := countStartup()
	r := Result{ID: "startup", Group: "Windows", Title: "Programas que arrancan con Windows", Weight: 3, Value: fmt.Sprintf("%d programas", n)}
	if n > 15 {
		r.Status = Warn
		r.Detail = "Cada programa que arranca solo gasta memoria y procesador de fondo mientras juegas."
		r.HowTo = []string{"Abre el Administrador de tareas (Ctrl + Mayús + Esc) > «Aplicaciones de arranque».", "Desactiva los que no necesites nada más encender (no se desinstalan, solo no arrancan solos)."}
		return r
	}
	r.Status, r.Detail = OK, "Pocos programas arrancan solos."
	return r
}

func countStartup() int {
	n := 0
	type src struct {
		root     registry.Key
		run      string
		approved string
	}
	for _, s := range []src{
		{registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`},
		{registry.LOCAL_MACHINE, `Software\Microsoft\Windows\CurrentVersion\Run`, `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`},
		{registry.LOCAL_MACHINE, `Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Run`, `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run32`},
	} {
		k, err := registry.OpenKey(s.root, s.run, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		names, _ := k.ReadValueNames(-1)
		k.Close()
		ap, _ := registry.OpenKey(s.root, s.approved, registry.QUERY_VALUE)
		for _, nm := range names {
			if ap != 0 {
				if b, _, err := ap.GetBinaryValue(nm); err == nil && len(b) > 0 && b[0]&1 == 1 {
					continue // desactivado en el Administrador de tareas
				}
			}
			n++
		}
		if ap != 0 {
			ap.Close()
		}
	}
	for _, dir := range []string{
		filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs\Startup`),
		filepath.Join(os.Getenv("ProgramData"), `Microsoft\Windows\Start Menu\Programs\StartUp`),
	} {
		ents, _ := os.ReadDir(dir)
		for _, e := range ents {
			if !strings.EqualFold(e.Name(), "desktop.ini") {
				n++
			}
		}
	}
	return n
}

// ---------- arreglos ----------

var (
	pSysParams = modUser.NewProc("SystemParametersInfoW")
)

// Fix aplica el arreglo de una comprobación. laptop decide qué plan de energía poner.
func Fix(id string) error {
	switch {
	case strings.HasPrefix(id, "hz:"):
		return SetMaxHz(strings.TrimPrefix(id, "hz:"))
	case strings.HasPrefix(id, "res:"):
		return SetNativeRes(strings.TrimPrefix(id, "res:"))
	case id == "plan":
		has, _, _ := sensors.Battery()
		if !has {
			if p, ok := BestPlan(); ok {
				return SetPlan(p.GUID)
			}
		}
		return SetPlan(PlanBalanced)
	case id == "cpumax":
		return SetProcessorMax100()
	case id == "gamemode":
		k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\GameBar`, registry.SET_VALUE)
		if err != nil {
			return err
		}
		defer k.Close()
		k.SetDWordValue("AllowAutoGameMode", 1)
		return k.SetDWordValue("AutoGameModeEnabled", 1)
	case id == "dvr":
		k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\GameDVR`, registry.SET_VALUE)
		if err != nil {
			return err
		}
		defer k.Close()
		return k.SetDWordValue("HistoricalCaptureEnabled", 0)
	case id == "mouse":
		// SPI_SETMOUSE con umbrales y aceleración a 0, guardado en el perfil y avisando a los programas.
		v := [3]int32{0, 0, 0}
		if r, _, e := pSysParams.Call(0x0004, 0, uintptr(unsafe.Pointer(&v[0])), 0x1|0x2); r == 0 {
			return e
		}
		return nil
	}
	return errors.New("esto no se puede arreglar desde aquí")
}
