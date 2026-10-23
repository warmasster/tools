package safe

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// KnownFile es lo que dice CIRCL hashlookup (bases como la NSRL de archivos legítimos).
type KnownFile struct {
	Known     bool   `json:"known"`              // aparece en la base de archivos conocidos
	Malicious bool   `json:"malicious"`          // la base lo marca como malicioso
	FileName  string `json:"fileName,omitempty"` // nombre con el que se registró
	Product   string `json:"product,omitempty"`  // programa al que pertenece
	Source    string `json:"source,omitempty"`   // p. ej. «NSRL»
	Trust     int    `json:"trust"`              // confianza 0–100 (50 = neutro)
	Summary   string `json:"summary"`
}

// HashLookupURL es la dirección base de CIRCL hashlookup (configurable para pruebas).
var HashLookupURL = "https://hashlookup.circl.lu"

// LookupKnown consulta si el archivo (por SHA-1, que es como se indexa la NSRL) es un
// archivo conocido. Tiempo límite corto: si no responde, devuelve error.
func LookupKnown(ctx context.Context, sha1 string) (*KnownFile, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(HashLookupURL, "/")+"/lookup/sha1/"+strings.ToLower(sha1), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.New("No se pudo consultar la base de archivos conocidos")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return &KnownFile{Trust: 50, Summary: "No aparece en la base de archivos conocidos"}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("La base de archivos conocidos no está disponible")
	}
	var raw struct {
		FileName       string          `json:"FileName"`
		Source         string          `json:"source"`
		Trust          *int            `json:"hashlookup:trust"`
		KnownMalicious json.RawMessage `json:"KnownMalicious"`
		ProductCode    struct {
			ProductName    string `json:"ProductName"`
			ProductVersion string `json:"ProductVersion"`
		} `json:"ProductCode"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&raw); err != nil {
		return nil, errors.New("Respuesta no válida de la base de archivos conocidos")
	}
	k := &KnownFile{Known: true, FileName: raw.FileName, Source: raw.Source, Trust: 50,
		Malicious: knownMalicious(raw.KnownMalicious)}
	if raw.Trust != nil {
		k.Trust = *raw.Trust
	}
	k.Product = strings.TrimSpace(raw.ProductCode.ProductName + " " + raw.ProductCode.ProductVersion)
	switch {
	case k.Malicious:
		k.Summary = "La base de archivos conocidos lo marca como malicioso"
	case k.Product != "":
		k.Summary = "Archivo conocido y legítimo: forma parte de " + k.Product
	default:
		k.Summary = "Archivo conocido y legítimo"
	}
	return k, nil
}

// knownMalicious acepta el campo como texto o lista (según la fuente).
func knownMalicious(m json.RawMessage) bool {
	v := strings.TrimSpace(string(m))
	return v != "" && v != "null" && v != `""` && v != "[]" && v != "false"
}
