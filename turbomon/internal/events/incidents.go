package events

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Incident es algo que ha ido mal, ya explicado.
type Incident struct {
	ID       string   `json:"id"`
	Time     int64    `json:"time"` // unix
	Kind     string   `json:"kind"` // crash, hang, gpu, bsod, reboot, power, hw
	Level    string   `json:"level"`
	App      string   `json:"app,omitempty"`
	Game     bool     `json:"game"`
	Title    string   `json:"title"`
	Cause    string   `json:"cause"`
	Steps    []string `json:"steps,omitempty"`
	Module   string   `json:"module,omitempty"`
	Code     string   `json:"code,omitempty"`
	Count    int      `json:"count,omitempty"`
	Unstable bool     `json:"unstable,omitempty"` // apunta a inestabilidad del hardware
}

// Report es lo encontrado en los últimos días.
type Report struct {
	Days      int        `json:"days"`
	Incidents []Incident `json:"incidents"`
	Crashes   int        `json:"crashes"`   // juegos cerrados de golpe
	GPUResets int        `json:"gpuResets"` // cuelgues de la gráfica
	Reboots   int        `json:"reboots"`   // reinicios de golpe o pantallazos
	HWErrors  int        `json:"hwErrors"`  // errores de hardware corregidos
	OtherApps int        `json:"otherApps"` // cierres de programas que no son juegos
	Unstable  bool       `json:"unstable"`  // hay señales de inestabilidad
	Advice    string     `json:"advice,omitempty"`
}

// Options para Collect.
type Options struct {
	Days        int
	GameExes    map[string]bool // exes de juegos conocidos (en minúsculas)
	IntelRaptor bool            // Intel 13.ª/14.ª gen «Raptor Lake» sin el microcódigo nuevo
}

var gameDirs = []string{`\steamapps\common\`, `\epic games\`, `\xboxgames\`, `\riot games\`, `\gog galaxy\games\`, `\gog games\`, `\ea games\`, `\ubisoft game launcher\games\`, `\battle.net\`, `\games\`}

func isGamePath(p string) bool {
	l := strings.ToLower(p)
	for _, d := range gameDirs {
		if strings.Contains(l, d) {
			return true
		}
	}
	return false
}

// Módulos que delatan a quién se le cayó el juego.
var gpuModules = map[string]string{
	"nvwgf2umx.dll": "NVIDIA", "nvwgf2um.dll": "NVIDIA", "nvoglv64.dll": "NVIDIA", "nvgpucomp64.dll": "NVIDIA", "nvldumdx.dll": "NVIDIA", "nvd3dumx.dll": "NVIDIA",
	"amdxx64.dll": "AMD", "atiumd64.dll": "AMD", "atidxx64.dll": "AMD", "amdvlk64.dll": "AMD", "atig6pxx.dll": "AMD", "amdxc64.dll": "AMD", "atiumdag.dll": "AMD",
	"igxelpicd64.dll": "Intel", "igd10iumd64.dll": "Intel", "igd12umd64.dll": "Intel", "igdumdim64.dll": "Intel", "igvk64.dll": "Intel",
}
var overlayModules = map[string]string{
	"gameoverlayrenderer64.dll": "el overlay de Steam", "gameoverlayrenderer.dll": "el overlay de Steam",
	"discordhook64.dll": "el overlay de Discord", "discordhook.dll": "el overlay de Discord",
	"rtsshooks64.dll": "RivaTuner (el contador de MSI Afterburner)", "rtsshooks.dll": "RivaTuner (el contador de MSI Afterburner)",
	"nvspcap64.dll": "el overlay de NVIDIA (grabación / GeForce Experience)", "nvspcap.dll": "el overlay de NVIDIA",
	"graphics-hook64.dll": "la captura de juego de OBS", "graphics-hook32.dll": "la captura de juego de OBS",
	"eosovh-win64-shipping.dll": "el overlay de Epic Games", "igo64.dll": "el overlay de EA", "uplay_r1_loader64.dll": "Ubisoft Connect",
	"overwolf.dll": "Overwolf", "medal-hook64.dll": "Medal", "reshade64.dll": "ReShade", "dxgi.dll_reshade": "ReShade",
}

func steps(s ...string) []string { return s }

var stepsUnstable = steps(
	"Si tienes overclock o undervolt (procesador, gráfica o RAM), quítalo y prueba unos días.",
	"Si tienes XMP/EXPO, prueba el perfil más suave o baja un poco la velocidad de la RAM.",
	"Pasa la prueba de «¿Es estable mi PC?» de TurboMON: busca errores en cada núcleo y en la RAM.",
)

func classifyCrash(app, module, code string) (title, cause string, st []string, unstable bool) {
	m := strings.ToLower(module)
	c := strings.ToLower(code)
	if v, ok := gpuModules[m]; ok {
		return "Fallo en el driver de la gráfica", fmt.Sprintf("El juego se cerró dentro del driver de %s (%s). Suele ser el driver, la gráfica inestable (overclock/undervolt) o falta de VRAM.", v, module),
			steps("Instala el último driver de la gráfica (o vuelve al anterior si empezó al actualizar).", "Si tienes overclock o undervolt en la gráfica, quítalo.", "Baja la calidad de texturas si la VRAM iba llena."), false
	}
	if v, ok := overlayModules[m]; ok {
		return "Lo ha tirado un overlay", fmt.Sprintf("El fallo fue dentro de %s, que se mete en el juego para dibujar encima.", v),
			steps(fmt.Sprintf("Desactiva %s para este juego (o del todo) y prueba.", v), "El overlay de TurboMON no se mete en el juego: puedes usarlo en su lugar."), false
	}
	switch {
	case c == "0xc000001d" || c == "0xc0000096":
		return "Instrucción imposible del procesador", "El procesador ejecutó algo que no existe o no está permitido: casi siempre es inestabilidad (overclock, undervolt, XMP) o un procesador degradado.", stepsUnstable, true
	case c == "0xc0000409":
		return "El juego se cerró a sí mismo", "El juego detectó que algo iba mal por dentro y se cerró a propósito. Suele ser un fallo del juego, de un mod, de un overlay o del antitrampas.",
			steps("Verifica los archivos del juego (Steam: Propiedades > Archivos instalados > Verificar).", "Quita mods y overlays y prueba.", "Si pasa en varios juegos distintos, mira si tu PC es estable (otra tarjeta de esta pestaña)."), false
	case c == "0xc0000005":
		t := "Fallo del juego"
		cs := "El juego intentó usar memoria que no era suya. Si solo pasa con un juego, es del juego; si pasa con varios, puede ser la RAM o el procesador inestables."
		if module != "" && !strings.EqualFold(module, app) && !strings.HasSuffix(m, ".exe") {
			cs = fmt.Sprintf("El fallo fue en «%s». ", module) + cs
		}
		return t, cs, steps("Verifica los archivos del juego y actualízalo.", "Si pasa con varios juegos, pasa la prueba de estabilidad de TurboMON."), false
	case c == "0xe06d7363":
		return "Error interno del juego", "El juego lanzó un error que no supo manejar. Suele ser del juego (o de un mod).", steps("Verifica los archivos del juego, quita mods y búscalo en los foros del juego."), false
	case c == "0xc0000374":
		return "Memoria del juego corrupta", "La memoria del juego se corrompió (fallo del juego, de un mod o RAM inestable).", steps("Verifica los archivos del juego.", "Si se repite en varios juegos, pasa la prueba de estabilidad."), false
	}
	return "El juego se cerró de golpe", "Windows apuntó un cierre inesperado del juego.", steps("Verifica los archivos del juego y actualiza el driver de la gráfica."), false
}

var bugchecks = map[uint64][3]string{
	0x116: {"La gráfica dejó de responder (pantallazo)", "La gráfica se colgó y Windows no pudo recuperarla: driver, gráfica inestable o calor.", "gpu"},
	0x117: {"La gráfica dejó de responder (pantallazo)", "La gráfica se colgó y Windows no pudo recuperarla.", "gpu"},
	0x124: {"Error de hardware (pantallazo)", "El procesador o la RAM dieron un error que no se pudo corregir: inestabilidad (overclock, XMP, undervolt), calor o una pieza que falla.", "hw"},
	0x1A:  {"Error de memoria (pantallazo)", "Windows encontró la memoria corrupta: suele ser la RAM (o su XMP) o un driver.", "hw"},
	0x50:  {"Fallo de un driver (pantallazo)", "Un driver usó memoria que no debía.", "driver"},
	0x0A:  {"Fallo de un driver (pantallazo)", "Un driver hizo algo prohibido.", "driver"},
	0xD1:  {"Fallo de un driver (pantallazo)", "Un driver hizo algo prohibido (a menudo el de red o el de un periférico).", "driver"},
	0x133: {"Windows se quedó colgado (pantallazo)", "Un driver tardó demasiado: a menudo el de un SSD con firmware viejo, el de la red o el de la gráfica.", "driver"},
	0x139: {"Fallo grave (pantallazo)", "Algo dañó estructuras internas de Windows: driver o RAM inestable.", "hw"},
	0x7E:  {"Fallo de un driver (pantallazo)", "Un hilo del sistema falló, casi siempre por un driver.", "driver"},
	0xEF:  {"Se cerró un proceso vital (pantallazo)", "Un proceso de Windows imprescindible se cerró: disco con errores, RAM o archivos de Windows dañados.", "hw"},
	0x9F:  {"Fallo al dormir o despertar (pantallazo)", "Un driver no supo entrar o salir de la suspensión.", "driver"},
	0x3B:  {"Fallo de un driver (pantallazo)", "Un driver o el propio Windows falló.", "driver"},
	0x19:  {"Memoria corrupta (pantallazo)", "La memoria de Windows se corrompió: RAM o un driver.", "hw"},
}

// Collect lee los registros y devuelve lo que ha pasado.
func Collect(o Options) Report {
	if o.Days <= 0 {
		o.Days = 30
	}
	rep := Report{Days: o.Days}
	since := Since(o.Days)
	sys, _ := Query("System", `*[System[(`+
		`(Provider[@Name='Display'] and EventID=4101) or Provider[@Name='nvlddmkm'] or Provider[@Name='amdkmdag'] or Provider[@Name='amdwddmg'] or `+
		`(Provider[@Name='Microsoft-Windows-WHEA-Logger'] and (EventID=17 or EventID=18 or EventID=19 or EventID=47)) or `+
		`(Provider[@Name='Microsoft-Windows-Kernel-Power'] and EventID=41) or (Provider[@Name='Microsoft-Windows-WER-SystemErrorReporting'] and EventID=1001)`+
		`) and `+since+`]]`, 400)
	app, _ := Query("Application", `*[System[(`+
		`(Provider[@Name='Application Error'] and EventID=1000) or (Provider[@Name='Application Hang'] and EventID=1002) or (Provider[@Name='Windows Error Reporting'] and EventID=1001)`+
		`) and `+since+`]]`, 400)
	all := append(sys, app...)
	sort.Slice(all, func(i, j int) bool { return all[i].Time.Before(all[j].Time) })

	var out []Incident
	add := func(in Incident) { out = append(out, in) }
	lastGPU := time.Time{}
	gpuIdx := -1
	var gpuTimes []time.Time
	for _, e := range all {
		if (e.Provider == "Display" && e.ID == 4101) || e.Provider == "nvlddmkm" || e.Provider == "amdkmdag" || e.Provider == "amdwddmg" {
			gpuTimes = append(gpuTimes, e.Time)
		}
	}
	whea := map[string]*Incident{} // por día y tipo
	bsodAt := map[int64]bool{}
	for _, e := range all {
		switch {
		case (e.Provider == "Display" && e.ID == 4101) || e.Provider == "nvlddmkm" || e.Provider == "amdkmdag" || e.Provider == "amdwddmg" ||
			(e.Provider == "Windows Error Reporting" && liveKernelGPU(e)):
			if e.Provider == "nvlddmkm" && e.ID != 13 && e.ID != 14 && e.ID != 153 {
				continue
			}
			if e.Provider == "Windows Error Reporting" && driverGPUBefore(gpuTimes, e.Time) {
				continue // el informe llega más tarde que el error: ya está contado
			}
			if !lastGPU.IsZero() && e.Time.Sub(lastGPU) < 15*time.Minute {
				// Varios avisos seguidos son el mismo episodio.
				if gpuIdx >= 0 {
					out[gpuIdx].Count++
				}
				lastGPU = e.Time
				continue
			}
			lastGPU = e.Time
			gpuIdx = len(out)
			add(Incident{Kind: "gpu", Level: "bad", Time: e.Time.Unix(), Title: "La gráfica dejó de responder", Count: 1,
				Cause: "La gráfica se quedó colgada y Windows reinició su driver (la pantalla se va a negro un momento y el juego suele cerrarse). Las causas típicas: driver, overclock o undervolt de la gráfica, calor o falta de energía de la fuente.",
				Steps: steps("Instala el último driver de la gráfica (o vuelve al anterior si empezó al actualizar).", "Quita cualquier overclock o undervolt de la gráfica.", "Mira la temperatura de la gráfica en «Mi PC» > «En directo» mientras juegas.", "Si se repite mucho con la gráfica al máximo, la fuente de alimentación puede ir justa.")})
		case e.Provider == "Application Error" && e.ID == 1000:
			exe := e.Get("AppName", 0)
			path := e.Get("AppPath", 10)
			mod := e.Get("ModuleName", 3)
			code := strings.ToLower(e.Get("ExceptionCode", 6))
			if code != "" && !strings.HasPrefix(code, "0x") {
				code = "0x" + code
			}
			game := o.GameExes[strings.ToLower(exe)] || isGamePath(path)
			if !game {
				rep.OtherApps++
				continue
			}
			t, cause, st, unst := classifyCrash(exe, mod, code)
			in := Incident{Kind: "crash", Level: "bad", Time: e.Time.Unix(), App: gameLabel(exe, path), Game: true, Title: t, Cause: cause, Steps: st, Module: mod, Code: code, Unstable: unst}
			// Si justo antes se colgó la gráfica, es lo mismo.
			if !lastGPU.IsZero() && e.Time.Sub(lastGPU) < 90*time.Second && len(out) > 0 && out[len(out)-1].Kind == "gpu" {
				prev := &out[len(out)-1]
				prev.App, prev.Game = in.App, true
				prev.Title = "La gráfica se colgó y " + in.App + " se cerró"
				continue
			}
			add(in)
		case e.Provider == "Application Hang" && e.ID == 1002:
			exe := e.Get("AppName", 0)
			path := e.Get("AppPath", 5)
			if !(o.GameExes[strings.ToLower(exe)] || isGamePath(path)) {
				rep.OtherApps++
				continue
			}
			add(Incident{Kind: "hang", Level: "warn", Time: e.Time.Unix(), App: gameLabel(exe, path), Game: true, Title: "El juego se quedó congelado",
				Cause: "El juego dejó de responder y se cerró (o lo cerraste tú). Suele ser el propio juego, a veces una carga muy larga desde un disco lento o la RAM llena.",
				Steps: steps("Si pasa al cargar, comprueba que el juego está en un SSD.", "Verifica los archivos del juego.", "Cierra programas que gasten mucha RAM antes de jugar.")})
		case e.Provider == "Microsoft-Windows-WER-SystemErrorReporting" && e.ID == 1001:
			code := parseBugcheck(e.Get("param1", 0))
			bsodAt[e.Time.Unix()/120] = true
			add(bsodIncident(code, e.Time))
		case e.Provider == "Windows Error Reporting" && e.ID == 1001:
			if strings.EqualFold(e.Get("", 2), "BlueScreen") {
				continue // ya sale como pantallazo en el registro del sistema
			}
		case e.Provider == "Microsoft-Windows-Kernel-Power" && e.ID == 41:
			bc, _ := strconv.ParseUint(e.Get("BugcheckCode", -1), 10, 64)
			pb := e.Get("PowerButtonTimestamp", -1)
			long := strings.EqualFold(e.Get("LongPowerButtonPressDetected", -1), "true")
			switch {
			case bc != 0:
				if bsodAt[e.Time.Unix()/120] || bsodAt[e.Time.Unix()/120-1] {
					continue
				}
				add(bsodIncident(bc, e.Time))
			case (pb != "" && pb != "0") || long:
				add(Incident{Kind: "power", Level: "info", Time: e.Time.Unix(), Title: "Apagado con el botón",
					Cause: "El PC se apagó manteniendo pulsado el botón de encendido (normalmente porque estaba colgado o no respondía)."})
			default:
				add(Incident{Kind: "reboot", Level: "bad", Time: e.Time.Unix(), Title: "El PC se reinició de golpe", Unstable: true,
					Cause: "El PC se apagó o reinició sin pantallazo azul y sin avisar. Las causas típicas: un cuelgue total por inestabilidad (overclock, XMP, undervolt o un procesador degradado), un corte de luz, la fuente de alimentación o el calor.",
					Steps: append(steps("Si fue un corte de luz o alguien lo apagó, olvídalo."), append(stepsUnstable, "Si pasa con la gráfica al máximo (jugando), sospecha también de la fuente de alimentación.")...)})
			}
		case e.Provider == "Microsoft-Windows-WHEA-Logger":
			kind := map[int]string{17: "conexión PCIe", 18: "error grave del procesador", 19: "procesador", 47: "memoria"}[e.ID]
			day := e.Time.Format("2006-01-02") + kind
			if in, ok := whea[day]; ok {
				in.Count++
				continue
			}
			lvl := "warn"
			cause := ""
			unst := false
			switch e.ID {
			case 17:
				lvl = "info"
				cause = "Errores corregidos en una conexión PCIe (gráfica, SSD o un puerto de la placa). Suelen ser inofensivos, pero si van con cuelgues, prueba a quitar el ahorro de energía de PCIe o a actualizar la BIOS."
			case 18:
				lvl = "bad"
				unst = true
				cause = "El procesador dio un error que no se pudo corregir y Windows tuvo que reiniciar."
			case 19:
				unst = true
				cause = "El procesador corrigió errores él solo. No deberían aparecer nunca: es la primera señal de inestabilidad (overclock, undervolt, XMP) o de un procesador que se degrada."
			case 47:
				unst = true
				cause = "La memoria RAM corrigió errores: suele ser el XMP/EXPO demasiado justo o un módulo que falla."
			}
			in := Incident{Kind: "hw", Level: lvl, Time: e.Time.Unix(), Title: "Errores de hardware: " + kind, Cause: cause, Count: 1, Unstable: unst}
			if unst {
				in.Steps = stepsUnstable
			}
			add(in)
			whea[day] = &out[len(out)-1]
		}
	}
	// Números y consejo general.
	unstable := 0
	for i := range out {
		in := &out[i]
		in.ID = fmt.Sprintf("%s-%d", in.Kind, in.Time)
		switch in.Kind {
		case "crash":
			rep.Crashes++
		case "gpu":
			rep.GPUResets++
		case "bsod", "reboot":
			rep.Reboots++
		case "hw":
			rep.HWErrors += max(1, in.Count)
		}
		if in.Unstable {
			unstable++
		}
	}
	// Varios juegos distintos cerrándose por acceso a memoria también apuntan a inestabilidad.
	apps := map[string]bool{}
	for _, in := range out {
		if in.Kind == "crash" && in.Code == "0xc0000005" {
			apps[in.App] = true
		}
	}
	if len(apps) >= 3 {
		unstable++
	}
	rep.Unstable = unstable > 0
	if rep.Unstable && o.IntelRaptor {
		rep.Advice = "Tu procesador es un Intel de 13.ª/14.ª generación con el chip «Raptor Lake» y tu BIOS no trae el microcódigo que lo protege. Con reinicios o errores como estos, lo primero es actualizar la BIOS (en «Revisión» tienes los pasos)."
	} else if rep.Unstable {
		rep.Advice = "Hay señales de que el PC no es del todo estable. Pasa la prueba de «¿Es estable mi PC?» y quita overclocks y undervolts mientras tanto."
	}
	// Lo más nuevo primero.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time > out[j].Time })
	rep.Incidents = out
	return rep
}

// driverGPUBefore: ¿el driver ya avisó de un cuelgue en las 48 h anteriores?
func driverGPUBefore(ts []time.Time, t time.Time) bool {
	for _, g := range ts {
		if d := t.Sub(g); d >= 0 && d < 48*time.Hour {
			return true
		}
	}
	return false
}

func liveKernelGPU(e Event) bool {
	if !strings.EqualFold(e.Get("", 2), "LiveKernelEvent") {
		return false
	}
	switch strings.ToLower(e.Get("", 5)) {
	case "117", "141", "193", "1a8":
		return true
	}
	return false
}

func parseBugcheck(s string) uint64 {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " ("); i > 0 {
		s = s[:i]
	}
	v, _ := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(s), "0x"), 16, 64)
	return v
}

func bsodIncident(code uint64, t time.Time) Incident {
	in := Incident{Kind: "bsod", Level: "bad", Time: t.Unix(), Code: fmt.Sprintf("0x%X", code)}
	if b, ok := bugchecks[code]; ok {
		in.Title, in.Cause = b[0], b[1]
		switch b[2] {
		case "gpu":
			in.Steps = steps("Instala el último driver de la gráfica (o el anterior si empezó al actualizar).", "Quita overclock y undervolt de la gráfica y vigila su temperatura.")
		case "hw":
			in.Steps, in.Unstable = stepsUnstable, true
		case "driver":
			in.Steps = steps("Actualiza los drivers del chipset, la red y la gráfica, y la BIOS.", "Si empezó al instalar algo (un programa de RGB, un antitrampas, un periférico), desinstálalo y prueba.")
		}
	} else {
		in.Title = "Pantallazo azul"
		in.Cause = fmt.Sprintf("Windows se detuvo con el código %s.", in.Code)
		in.Steps = steps("Busca ese código en internet junto al nombre de tu placa o gráfica.", "Actualiza la BIOS y los drivers.")
	}
	return in
}

// gameLabel da un nombre legible al juego por su carpeta de Steam/Epic o su ejecutable.
func gameLabel(exe, path string) string {
	l := strings.ToLower(path)
	for _, d := range []string{`\steamapps\common\`, `\epic games\`, `\xboxgames\`, `\riot games\`, `\gog galaxy\games\`, `\gog games\`, `\ea games\`} {
		if i := strings.Index(l, d); i >= 0 {
			rest := path[i+len(d):]
			if j := strings.IndexAny(rest, `\/`); j > 0 {
				return rest[:j]
			}
		}
	}
	return strings.TrimSuffix(exe, filepath.Ext(exe))
}
