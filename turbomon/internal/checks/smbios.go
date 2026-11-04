package checks

import "turbomon/internal/parts"

// DIMM es un módulo de RAM (ver parts.DIMM).
type DIMM = parts.DIMM

// DIMMs devuelve los módulos de RAM.
func DIMMs() []DIMM { return parts.DIMMs() }

// PartSpeed saca la velocidad de fábrica del modelo de un módulo.
func PartSpeed(part string) int { return parts.PartSpeed(part) }

// Channel dice en qué canal está un módulo.
func Channel(d DIMM) string { return parts.Channel(d) }

func nvidiaVersion(v string) string { return parts.NvidiaVersion(v) }
