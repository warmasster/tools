// Package safe responde a «¿Es seguro?» (huellas, firma, Defender, VirusTotal y base de
// archivos conocidos, combinados en un veredicto) y a «¿De dónde salió?» (marca de internet).
package safe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Level es el veredicto final.
type Level string

const (
	Clean      Level = "clean"      // 🟢 limpio
	Suspicious Level = "suspicious" // 🟡 sospechoso
	Dangerous  Level = "dangerous"  // 🔴 peligroso
	Unknown    Level = "unknown"    // sin datos suficientes
)

// Umbrales de VirusTotal: a partir de vtDanger detecciones es peligroso.
const (
	vtDanger  = 5
	vtMinScan = 10 // motores mínimos para fiarse de un «0 detecciones»
)

// Evidence reúne lo que se sabe del archivo; cada campo es nil mientras no haya llegado
// (o si esa comprobación no se hizo). Errors guarda los fallos por paso.
type Evidence struct {
	Path       string            `json:"path"`
	Hashes     *Hashes           `json:"hashes,omitempty"`
	Signature  *Signature        `json:"signature,omitempty"`
	Defender   *DefenderResult   `json:"defender,omitempty"`
	VirusTotal *VTReport         `json:"virusTotal,omitempty"`
	Known      *KnownFile        `json:"known,omitempty"`
	Origin     *Origin           `json:"origin,omitempty"`
	Errors     map[string]string `json:"errors,omitempty"` // paso → mensaje para el usuario
}

// Verdict es la conclusión para enseñar.
type Verdict struct {
	Level   Level    `json:"level"`
	Title   string   `json:"title"`   // «Parece seguro», «Ten cuidado»…
	Summary string   `json:"summary"` // la frase principal, p. ej. «0 de 70 antivirus lo detectan»
	Reasons []string `json:"reasons"` // el resto de motivos, de más a menos importante
}

// Pasos de Analyze (también son las claves de Evidence.Errors).
const (
	StepHashes     = "hashes"
	StepSignature  = "signature"
	StepDefender   = "defender"
	StepVirusTotal = "virustotal"
	StepKnown      = "known"
	StepOrigin     = "origin"
	StepDone       = "done"
)

// Options elige qué comprobaciones hace Analyze. El valor cero hace todas menos VirusTotal.
type Options struct {
	VT         *VTClient // nil = sin VirusTotal
	NoDefender bool
	NoKnown    bool     // no consultar la base de archivos conocidos (red)
	OnProgress Progress // avance del cálculo de huellas
}

// Update se envía cada vez que llega un resultado.
type Update struct {
	Step     string   `json:"step"`
	Evidence Evidence `json:"evidence"`
	Verdict  Verdict  `json:"verdict"`
}

// Analyze lanza todas las comprobaciones en paralelo y llama a onUpdate (opcional, nunca a
// la vez desde dos hilos) según llegan: firma y origen en milisegundos, huellas y
// consultas por red después, Defender en unos segundos. Devuelve lo reunido al final.
func Analyze(ctx context.Context, path string, opt Options, onUpdate func(Update)) Evidence {
	ev := Evidence{Path: path}
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		msg := "Es una carpeta, no un archivo"
		if err != nil {
			msg = errOpen(err).Error()
		}
		ev.Errors = map[string]string{StepHashes: msg}
		if onUpdate != nil {
			onUpdate(Update{Step: StepDone, Evidence: ev, Verdict: Judge(ev)})
		}
		return ev
	}
	var mu sync.Mutex
	set := func(step string, apply func(), err error) {
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if ev.Errors == nil {
				ev.Errors = map[string]string{}
			}
			ev.Errors[step] = err.Error()
		} else {
			apply()
		}
		if onUpdate != nil {
			snap := ev.clone()
			onUpdate(Update{Step: step, Evidence: snap, Verdict: Judge(snap)})
		}
	}

	var wg sync.WaitGroup
	run := func(f func()) { wg.Add(1); go func() { defer wg.Done(); f() }() }

	run(func() {
		o, err := ReadOrigin(path)
		set(StepOrigin, func() { ev.Origin = o }, err)
	})
	run(func() {
		s, err := CheckSignature(ctx, path)
		set(StepSignature, func() { ev.Signature = s }, err)
	})
	if !opt.NoDefender {
		run(func() {
			d, err := ScanDefender(ctx, path)
			set(StepDefender, func() { ev.Defender = d }, err)
		})
	}
	run(func() {
		h, err := HashFile(ctx, path, opt.OnProgress)
		set(StepHashes, func() { ev.Hashes = h }, err)
		if err != nil {
			return
		}
		// Con las huellas ya se puede preguntar por red.
		var inner sync.WaitGroup
		if opt.VT != nil {
			inner.Add(1)
			go func() {
				defer inner.Done()
				r, err := opt.VT.Lookup(ctx, h.SHA256)
				set(StepVirusTotal, func() { ev.VirusTotal = r }, err)
			}()
		}
		if !opt.NoKnown {
			inner.Add(1)
			go func() {
				defer inner.Done()
				k, err := LookupKnown(ctx, h.SHA1)
				set(StepKnown, func() { ev.Known = k }, err)
			}()
		}
		inner.Wait()
	})
	wg.Wait()

	mu.Lock()
	final := ev.clone()
	mu.Unlock()
	if onUpdate != nil && ctx.Err() == nil {
		onUpdate(Update{Step: StepDone, Evidence: final, Verdict: Judge(final)})
	}
	return final
}

// clone copia la evidencia (los resultados no se modifican después, basta copiar punteros).
func (e Evidence) clone() Evidence {
	c := e
	if e.Errors != nil {
		c.Errors = make(map[string]string, len(e.Errors))
		for k, v := range e.Errors {
			c.Errors[k] = v
		}
	}
	return c
}

// Extensiones que pueden ejecutar código al abrirse.
var riskyExt = map[string]bool{
	".exe": true, ".dll": true, ".scr": true, ".com": true, ".pif": true, ".cpl": true, ".msi": true,
	".msp": true, ".msix": true, ".appx": true, ".appinstaller": true, ".bat": true, ".cmd": true,
	".ps1": true, ".psm1": true, ".vbs": true, ".vbe": true, ".js": true, ".jse": true, ".wsf": true,
	".wsh": true, ".hta": true, ".lnk": true, ".url": true, ".jar": true, ".reg": true, ".sys": true,
	".inf": true, ".chm": true, ".iso": true, ".img": true, ".vhd": true, ".vhdx": true, ".xll": true,
	".docm": true, ".xlsm": true, ".pptm": true, ".dotm": true, ".xlam": true, ".ocx": true,
	".zip": true, ".7z": true, ".rar": true, ".cab": true, ".one": true, ".svg": true,
}

// Tipos que admiten firma digital: solo en ellos «sin firma» es una pista.
var signableExt = map[string]bool{
	".exe": true, ".dll": true, ".scr": true, ".cpl": true, ".msi": true, ".msp": true, ".msix": true,
	".appx": true, ".sys": true, ".ocx": true, ".ps1": true, ".psm1": true, ".vbs": true, ".js": true,
	".wsf": true, ".cab": true, ".cat": true, ".efi": true, ".drv": true, ".xll": true,
}

// IsRisky indica si el tipo de archivo puede ejecutar código (programas, scripts, macros…).
func IsRisky(path string) bool { return riskyExt[strings.ToLower(filepath.Ext(path))] }

// Judge combina lo que haya en la evidencia (aunque falten pasos) y da el veredicto.
func Judge(e Evidence) Verdict {
	var bad, warn, good, info []string
	risky := IsRisky(e.Path)
	signable := signableExt[strings.ToLower(filepath.Ext(e.Path))]
	sig, vt, def, kn := e.Signature, e.VirusTotal, e.Defender, e.Known
	signedOK := sig != nil && sig.Status == SignValid
	knownOK := kn != nil && kn.Known && !kn.Malicious

	if def != nil && def.Available && !def.Clean {
		bad = append(bad, def.Summary)
	}
	if kn != nil && kn.Malicious {
		bad = append(bad, kn.Summary)
	}
	vtClean := false
	if vt != nil {
		switch {
		case !vt.Found || vt.Total == 0:
			info = append(info, vt.Summary)
		case vt.Malicious >= vtDanger:
			bad = append(bad, vtLine(vt))
		case vt.Malicious >= 2:
			warn = append(warn, vtLine(vt))
		case vt.Malicious == 1 && !signedOK && !knownOK:
			warn = append(warn, vtLine(vt))
		case vt.Malicious == 1:
			good = append(good, vt.Summary+" (seguramente una falsa alarma)")
			vtClean = true
		case vt.Total >= vtMinScan:
			good = append(good, vt.Summary)
			vtClean = true
		default:
			info = append(info, vt.Summary)
		}
	}
	if sig != nil {
		switch sig.Status {
		case SignValid:
			good = append(good, sig.Summary)
		case SignDistrusted:
			bad = append(bad, sig.Summary)
		case SignTampered, SignRevoked:
			warn = append(warn, sig.Summary)
		case SignUntrusted, SignInvalid:
			if risky && !vtClean && !knownOK {
				warn = append(warn, sig.Summary)
			} else {
				info = append(info, sig.Summary)
			}
		case SignExpired:
			info = append(info, sig.Summary)
		case SignUnsigned:
			if signable {
				info = append(info, sig.Summary)
			}
		}
	}
	if knownOK {
		good = append(good, kn.Summary)
	}
	defClean := def != nil && def.Available && def.Clean
	if defClean {
		if risky {
			info = append(info, def.Summary)
		} else {
			good = append(good, def.Summary)
		}
	} else if def != nil && !def.Available {
		info = append(info, def.Summary)
	}
	if o := e.Origin; o != nil {
		info = append(info, o.Summary)
	}

	v := Verdict{}
	switch {
	case len(bad) > 0:
		v.Level, v.Title = Dangerous, "Peligroso"
		v.Summary, v.Reasons = bad[0], join(bad[1:], warn, good, info)
	case len(warn) > 0:
		v.Level, v.Title = Suspicious, "Ten cuidado"
		v.Summary, v.Reasons = warn[0], join(warn[1:], good, info)
	case len(good) > 0:
		v.Level, v.Title = Clean, "Parece seguro"
		v.Summary, v.Reasons = good[0], join(good[1:], info)
	default:
		v.Level, v.Title = Unknown, "No se puede asegurar"
		v.Summary = unknownLine(e, signable)
		v.Reasons = join(info)
	}
	if v.Reasons == nil {
		v.Reasons = []string{}
	}
	return v
}

func vtLine(vt *VTReport) string {
	s := vt.Summary
	if vt.Threat != "" {
		s += " (" + vt.Threat + ")"
	} else if len(vt.Detections) > 0 && vt.Detections[0].Label != "" {
		s += fmt.Sprintf(" (%s: %s)", vt.Detections[0].Engine, vt.Detections[0].Label)
	}
	return s
}

// unknownLine explica por qué no hay veredicto.
func unknownLine(e Evidence, signable bool) string {
	if e.Signature == nil && e.Defender == nil && e.VirusTotal == nil && e.Known == nil {
		return "Analizando…"
	}
	unsigned := signable && e.Signature != nil && e.Signature.Status == SignUnsigned
	fromNet := e.Origin != nil && e.Origin.Marked && e.Origin.ZoneID >= 3
	site := ""
	if fromNet {
		site = e.Origin.Site
	}
	switch {
	case unsigned && site != "":
		return "Sin firma, descargado de " + site
	case unsigned && fromNet:
		return "Sin firma y llegó de internet"
	case unsigned:
		return "Sin firma, de un origen desconocido"
	case site != "":
		return "Descargado de " + site + ", sin más datos para comprobarlo"
	case fromNet:
		return "Llegó de internet, sin más datos para comprobarlo"
	}
	return "No hay datos suficientes para dar un veredicto"
}

func join(parts ...[]string) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range parts {
		for _, s := range p {
			if s != "" && !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}
