package checks

// Comprobaciones que solo salen si tocan en este PC: lo de Intel solo con
// Intel, lo de las Radeon solo con Radeon, lo del wifi solo si se juega por
// wifi… Lo que no aplica no aparece.

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"

	"turbomon/internal/parts"
)

func tailoredChecks(c parts.Components) []Result {
	var out []Result
	out = append(out, gpuSlotChecks(c)...)
	if r, ok := intelMicrocode(c); ok {
		out = append(out, r)
	}
	if r, ok := amdX3D(c); ok {
		out = append(out, r)
	}
	out = append(out, networkChecks(c)...)
	out = append(out, diskHealthChecks(c)...)
	if r, ok := batteryCheck(c); ok {
		out = append(out, r)
	}
	return out
}

func gpuSlotChecks(c parts.Components) []Result {
	var out []Result
	for _, g := range c.GPUs {
		if !g.Dedicated || c.Board.Laptop {
			continue
		}
		short := strings.TrimPrefix(strings.TrimPrefix(g.Name, "NVIDIA "), "AMD ")
		if g.MaxWidth > 0 && g.LinkWidth > 0 {
			r := Result{ID: "gpu_pcie", Group: "Gráfica", Title: "Ranura de la gráfica", Weight: 15}
			gen := map[int]string{1: "1.0", 2: "2.0", 3: "3.0", 4: "4.0", 5: "5.0"}[g.MaxGen]
			if g.LinkWidth < g.MaxWidth {
				r.Status = Bad
				r.Value = fmt.Sprintf("x%d de x%d", g.LinkWidth, g.MaxWidth)
				r.Detail = fmt.Sprintf("Tu %s puede usar %d carriles PCIe pero va con %d: suele pasar cuando está en la ranura de abajo de la placa (que va más lenta) o con un cable alargador malo. Pierde rendimiento, sobre todo cuando se llena la VRAM.", short, g.MaxWidth, g.LinkWidth)
				r.HowTo = []string{"Apaga el PC y pasa la gráfica a la ranura PCIe larga de arriba (la más cercana al procesador).", "Si usas un cable alargador (montaje vertical), prueba sin él o con uno PCIe 4.0 de calidad."}
			} else {
				r.Status = OK
				r.Value = fmt.Sprintf("PCIe %s x%d", gen, g.LinkWidth)
				r.Detail = "La gráfica va con todos los carriles que admite."
				if g.MaxWidth == 8 {
					r.Detail = "Tu gráfica está hecha para 8 carriles (x8) y va con los 8: es lo normal en ella."
				}
			}
			out = append(out, r)
		}
		if g.ReBAR >= 0 {
			r := Result{ID: "rebar", Group: "Gráfica", Title: "Resizable BAR", Weight: 5}
			if g.ReBAR == 1 {
				r.Status, r.Value = OK, "Activado"
				r.Detail = "El procesador puede ver toda la memoria de la gráfica de una vez: en muchos juegos da unos FPS más."
			} else {
				r.Status, r.Value = Warn, "Desactivado"
				r.Detail = "Con Resizable BAR el procesador ve toda la VRAM de una vez en vez de a trozos de 256 MB. En muchos juegos son un 5-10 % más."
				if g.Vendor == "intel" {
					r.Status, r.Weight = Bad, 20
					r.Detail = "Las gráficas Intel Arc necesitan Resizable BAR: sin él rinden muchísimo peor (hasta la mitad en algunos juegos)."
				}
				r.HowTo = []string{
					"Entra en la BIOS y busca «Above 4G Decoding» y «Re-Size BAR Support» (o «Smart Access Memory» en AMD): actívalos los dos.",
					"Si no aparecen, quizá haya que desactivar «CSM» (arranque antiguo). Si Windows se instaló hace años en modo antiguo, no lo toques sin mirar antes.",
					"Guarda con F10, reinicia y vuelve a revisar.",
				}
			}
			out = append(out, r)
		}
		break
	}
	return out
}

// intelMicrocode: los Core de 13.ª y 14.ª generación de sobremesa con el chip
// «Raptor Lake» pueden degradarse con el tiempo si la BIOS no trae el
// microcódigo que lo corrige (0x12B o posterior).
func intelMicrocode(c parts.Components) (Result, bool) {
	cpu := c.CPU
	if cpu.Vendor != "intel" || cpu.Family != 6 || cpu.Model != 0xB7 || c.Board.Laptop || cpu.Microcode == 0 {
		return Result{}, false
	}
	r := Result{ID: "intel_microcode", Group: "Procesador", Title: "Microcódigo del procesador (Intel 13.ª/14.ª gen.)", Weight: 10,
		Value: fmt.Sprintf("0x%X", cpu.Microcode)}
	if cpu.Microcode >= 0x12B {
		r.Status = OK
		r.Detail = "Tu BIOS trae el microcódigo que Intel publicó para evitar que estos procesadores se degraden con el tiempo."
		return r, true
	}
	r.Status = Warn
	board := strings.TrimSpace(c.Board.Maker + " " + c.Board.Model)
	r.Detail = fmt.Sprintf("Tu procesador usa el chip «Raptor Lake», el de los 13.ª y 14.ª generación que pueden degradarse con el tiempo (cuelgues y errores que van a más). Intel lo corrigió con el microcódigo 0x12B y posteriores, que llegan con la BIOS; tú tienes el 0x%X. Intel no ha dejado claro si todos los modelos están afectados, pero actualizar la BIOS es lo que recomienda.", cpu.Microcode)
	how := []string{}
	if board != "" {
		v := c.Board.BIOSVersion
		if c.Board.BIOSDate != "" {
			v += " (" + c.Board.BIOSDate + ")"
		}
		how = append(how, fmt.Sprintf("Tu placa es una %s con la BIOS %s. Busca en la web de %s la página de esa placa, en «Soporte» > «BIOS».", board, v, c.Board.Maker))
	}
	how = append(how, "Descarga una BIOS que mencione «microcode 0x12B» o posterior (0x12F…) o «Intel Default Settings», y sigue las instrucciones del fabricante (suele hacerse con un USB desde la propia BIOS).", "No apagues el PC mientras se actualiza.")
	r.HowTo = how
	return r, true
}

func amdX3D(c parts.Components) (Result, bool) {
	cpu := c.CPU
	if cpu.Vendor != "amd" || !strings.Contains(strings.ToUpper(cpu.Name), "X3D") || cpu.L3Groups < 2 {
		return Result{}, false
	}
	r := Result{ID: "amd_x3d", Group: "Procesador", Title: "Juegos en el chip con más caché (X3D)", Weight: 10}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\amd3dvcache`, registry.QUERY_VALUE)
	if err == nil {
		k.Close()
		r.Status, r.Value = OK, "Driver instalado"
		r.Detail = "Tu procesador tiene dos chips y solo uno lleva la caché extra. El driver de AMD está instalado para que los juegos vayan a ese."
		return r, true
	}
	r.Status, r.Value = Warn, "Falta el driver"
	r.Detail = "Tu procesador tiene dos chips y solo uno lleva la caché extra que tanto ayuda en juegos. Sin el driver «3D V-Cache Performance Optimizer» de AMD, los juegos pueden ir al chip sin caché y perder bastante."
	r.HowTo = []string{"Instala los «AMD Chipset Drivers» más nuevos de amd.com (traen ese driver).", "Deja activada la Xbox Game Bar: AMD la usa para saber cuándo hay un juego abierto."}
	return r, true
}

func networkChecks(c parts.Components) []Result {
	var out []Result
	for _, n := range c.Network {
		if !n.Default {
			continue
		}
		switch n.Kind {
		case "cable":
			r := Result{ID: "net_cable", Group: "Red", Title: "Velocidad del cable de red", Weight: 6}
			if n.LinkMbps > 0 && n.LinkMbps <= 100 {
				r.Status, r.Value = Warn, fmt.Sprintf("%d Mbps", n.LinkMbps)
				r.Detail = "El cable va a 100 Mbps en vez de 1000. Casi siempre es el cable (un hilo roto o un cable muy viejo) o el conector: con 100 Mbps las descargas van lentísimas y el ping sube cuando algo descarga."
				r.HowTo = []string{"Cambia el cable por uno «Cat 5e» o «Cat 6» (vienen escritos en el propio cable).", "Prueba otro puerto del router.", "Vuelve a revisar: debería salir «1000 Mbps»."}
			} else {
				r.Status, r.Value = OK, fmt.Sprintf("%s Mbps", fmtMbps(n.LinkMbps))
				r.Detail = "Juegas por cable a buena velocidad: lo mejor para el ping."
			}
			out = append(out, r)
		case "wifi":
			r := Result{ID: "net_wifi", Group: "Red", Title: "Wifi para jugar", Weight: 6}
			r.Value = strings.TrimSpace(n.WifiStd + " " + n.WifiBand)
			if n.WifiSignal > 0 {
				r.Value += fmt.Sprintf(" · señal %d %%", n.WifiSignal)
			}
			switch {
			case n.WifiBand == "2,4 GHz" || n.WifiStd == "Wi-Fi 4" || n.WifiStd == "Wi-Fi antiguo":
				r.Status = Warn
				r.Detail = "Juegas por wifi en 2,4 GHz o con un adaptador antiguo: es la banda más saturada (vecinos, microondas, Bluetooth) y la que más picos de ping da."
				r.HowTo = []string{"Si puedes, usa cable: es lo que más se nota en juegos online.", "Si no, conéctate a la red de 5 GHz de tu router (suele llamarse igual con «_5G»). Si tu adaptador no la ve, uno USB con Wi-Fi 6 cuesta poco."}
			case n.WifiSignal > 0 && n.WifiSignal < 50:
				r.Status = Warn
				r.Detail = "La señal del wifi es floja: habrá más picos de ping y paquetes perdidos."
				r.HowTo = []string{"Acerca el router o el PC, o usa un repetidor en malla. Con cable se acaba el problema."}
			default:
				r.Status = OK
				r.Detail = "El wifi es bueno, aunque el cable siempre da un ping más estable."
			}
			out = append(out, r)
		}
	}
	return out
}

func fmtMbps(v int) string {
	if v >= 1000 && v%1000 == 0 {
		return fmt.Sprintf("%d000", v/1000)
	}
	return fmt.Sprint(v)
}

func diskHealthChecks(c parts.Components) []Result {
	var out []Result
	for _, d := range c.Disks {
		if d.Health == nil && d.TempC == 0 {
			continue
		}
		name := d.Model
		if len(d.Letters) > 0 {
			name += " (" + strings.Join(d.Letters, ", ") + ")"
		}
		if h := d.Health; h != nil {
			r := Result{ID: "disk_health:" + fmt.Sprint(d.Number), Group: "Discos", Title: "Salud de " + name, Weight: 8}
			switch {
			case h.Warning || h.MediaErrors > 0:
				r.Status, r.Value = Bad, "El disco avisa de un problema"
				r.Detail = "El propio disco informa de errores. Haz copia de lo importante cuanto antes."
			case h.UsedPct >= 90:
				r.Status, r.Value = Bad, fmt.Sprintf("Gastado al %d %%", h.UsedPct)
				r.Detail = "Está al final de su vida útil según el fabricante. Haz copia y piensa en cambiarlo."
			case h.Reallocated > 0:
				r.Status, r.Value = Warn, fmt.Sprintf("%d sectores dañados", h.Reallocated)
				r.Detail = "Tiene sectores reasignados: es la primera señal de que un disco empieza a fallar. Haz copia de lo importante."
			case h.UsedPct >= 70:
				r.Status, r.Value = Warn, fmt.Sprintf("Gastado al %d %%", h.UsedPct)
				r.Detail = "Ha gastado buena parte de su vida útil. Todavía vale, pero vigílalo."
			case h.UsedPct >= 0:
				r.Status, r.Value = OK, fmt.Sprintf("Gastado al %d %%", h.UsedPct)
				r.Detail = "Está sano."
			default:
				r.Status, r.Value, r.Detail = OK, "Sin errores", "No informa de ningún problema."
			}
			out = append(out, r)
		}
		if d.TempC >= 70 {
			out = append(out, Result{ID: "disk_temp:" + fmt.Sprint(d.Number), Group: "Discos", Title: "Temperatura de " + name, Weight: 4, Status: Warn,
				Value: fmt.Sprintf("%d °C", d.TempC), Detail: "Va muy caliente: los SSD NVMe bajan la velocidad para protegerse (y se nota en las cargas).",
				HowTo: []string{"Ponle un disipador (muchas placas traen uno para el NVMe) o mejora el flujo de aire de la caja."}})
		}
	}
	return out
}

func batteryCheck(c parts.Components) (Result, bool) {
	b := c.Battery
	if b == nil || !c.Board.Laptop {
		return Result{}, false
	}
	r := Result{ID: "battery", Group: "Energía", Title: "Salud de la batería", Weight: 3, Value: fmt.Sprintf("%d %% de la original", b.Health)}
	switch {
	case b.Health < 50:
		r.Status, r.Detail = Warn, "La batería ya solo guarda la mitad de lo que guardaba nueva: dura mucho menos y algunos portátiles bajan el rendimiento con ella."
	default:
		r.Status, r.Detail = OK, "La batería está bien."
	}
	return r, true
}
