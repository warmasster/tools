package safe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Errores de VirusTotal que la interfaz puede distinguir (el texto ya es para el usuario).
var (
	ErrVTKey     = errors.New("La clave de VirusTotal no es válida")
	ErrVTQuota   = errors.New("Se ha agotado la cuota gratuita de VirusTotal; prueba dentro de un rato")
	ErrVTTooBig  = errors.New("El archivo es demasiado grande para VirusTotal (máximo 650 MB)")
	ErrVTNetwork = errors.New("No se pudo conectar con VirusTotal")
)

const (
	vtBaseURL      = "https://www.virustotal.com/api/v3"
	vtDirectMax    = 32 << 20  // subida directa
	vtUploadMax    = 650 << 20 // con URL de subida
	vtPollInterval = 15 * time.Second
)

var vtKeyRe = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// ValidVTKey indica si la clave tiene el formato de VirusTotal (64 caracteres hexadecimales).
func ValidVTKey(key string) bool { return vtKeyRe.MatchString(strings.TrimSpace(key)) }

// VTClient consulta VirusTotal (API v3) con la clave del usuario.
type VTClient struct {
	Key          string
	BaseURL      string        // vacío = la API oficial (configurable para pruebas)
	HTTP         *http.Client  // vacío = cliente con tiempo límite razonable
	PollInterval time.Duration // espera entre consultas del análisis (vacío = 15 s, por la cuota gratuita)
}

// NewVTClient crea un cliente con la clave dada.
func NewVTClient(key string) *VTClient { return &VTClient{Key: strings.TrimSpace(key)} }

// VTDetection es un antivirus que marca el archivo.
type VTDetection struct {
	Engine   string `json:"engine"`
	Label    string `json:"label"`    // nombre que le da ese antivirus
	Category string `json:"category"` // malicious o suspicious
}

// VTReport es lo que VirusTotal sabe de un archivo.
type VTReport struct {
	Found      bool          `json:"found"`      // false = nadie lo ha subido nunca
	Malicious  int           `json:"malicious"`  // motores que lo detectan
	Suspicious int           `json:"suspicious"` // motores que lo ven sospechoso
	Total      int           `json:"total"`      // motores que lo analizaron de verdad
	Detections []VTDetection `json:"detections,omitempty"`
	Threat     string        `json:"threat,omitempty"`   // etiqueta común, p. ej. «trojan.emotet»
	Name       string        `json:"name,omitempty"`     // nombre más común
	Type       string        `json:"type,omitempty"`     // p. ej. «Win32 EXE»
	FirstSeen  time.Time     `json:"firstSeen,omitzero"` // primera vez que se subió
	LastScan   time.Time     `json:"lastScan,omitzero"`  // último análisis
	Reputation int           `json:"reputation"`         // votos de la comunidad
	SHA256     string        `json:"sha256,omitempty"`
	Link       string        `json:"link,omitempty"` // ficha web
	Summary    string        `json:"summary"`
}

// VTUpload es el avance de una subida y su análisis.
type VTUpload struct {
	Stage string `json:"stage"` // "upload", "queued", "analyzing", "done"
	Sent  int64  `json:"sent"`
	Total int64  `json:"total"`
}

type vtError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type vtStats struct {
	Harmless    int `json:"harmless"`
	Malicious   int `json:"malicious"`
	Suspicious  int `json:"suspicious"`
	Undetected  int `json:"undetected"`
	Unsupported int `json:"type-unsupported"`
	Timeout     int `json:"timeout"`
	Failure     int `json:"failure"`
}

type vtEngine struct {
	Category   string  `json:"category"`
	EngineName string  `json:"engine_name"`
	Result     *string `json:"result"`
}

type vtFile struct {
	Data struct {
		ID         string `json:"id"`
		Attributes struct {
			FirstSubmission    int64               `json:"first_submission_date"`
			LastAnalysis       int64               `json:"last_analysis_date"`
			MeaningfulName     string              `json:"meaningful_name"`
			Names              []string            `json:"names"`
			TypeDescription    string              `json:"type_description"`
			Reputation         int                 `json:"reputation"`
			SHA256             string              `json:"sha256"`
			Stats              vtStats             `json:"last_analysis_stats"`
			Results            map[string]vtEngine `json:"last_analysis_results"`
			PopularThreatClass struct {
				Label string `json:"suggested_threat_label"`
			} `json:"popular_threat_classification"`
		} `json:"attributes"`
	} `json:"data"`
}

type vtAnalysis struct {
	Data struct {
		ID         string `json:"id"`
		Attributes struct {
			Status  string              `json:"status"`
			Date    int64               `json:"date"`
			Stats   vtStats             `json:"stats"`
			Results map[string]vtEngine `json:"results"`
		} `json:"attributes"`
	} `json:"data"`
	Meta struct {
		FileInfo struct {
			SHA256 string `json:"sha256"`
		} `json:"file_info"`
	} `json:"meta"`
}

func (c *VTClient) base() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return vtBaseURL
}

func (c *VTClient) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

// do hace la petición y devuelve el cuerpo; traduce los errores de la API.
func (c *VTClient) do(req *http.Request, cl *http.Client) ([]byte, int, error) {
	if !ValidVTKey(c.Key) {
		return nil, 0, ErrVTKey
	}
	req.Header.Set("x-apikey", c.Key)
	req.Header.Set("accept", "application/json")
	resp, err := cl.Do(req)
	if err != nil {
		if ctxErr := req.Context().Err(); ctxErr != nil {
			return nil, 0, ctxErr
		}
		return nil, 0, ErrVTNetwork
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, 0, ErrVTNetwork
	}
	if resp.StatusCode == http.StatusOK {
		return body, resp.StatusCode, nil
	}
	var ve vtError
	json.Unmarshal(body, &ve)
	switch {
	case resp.StatusCode == http.StatusTooManyRequests || ve.Error.Code == "QuotaExceededError" || ve.Error.Code == "TooManyRequestsError":
		return nil, resp.StatusCode, ErrVTQuota
	case resp.StatusCode == http.StatusUnauthorized || ve.Error.Code == "WrongCredentialsError" || ve.Error.Code == "AuthenticationRequiredError":
		return nil, resp.StatusCode, ErrVTKey
	case resp.StatusCode == http.StatusNotFound:
		return nil, resp.StatusCode, nil
	case resp.StatusCode == http.StatusRequestEntityTooLarge:
		return nil, resp.StatusCode, ErrVTTooBig
	}
	return nil, resp.StatusCode, fmt.Errorf("VirusTotal respondió con un error (%d)", resp.StatusCode)
}

// Lookup consulta el informe de un archivo por su SHA-256 (o SHA-1/MD5).
// Si VirusTotal no lo conoce devuelve Found=false sin error.
func (c *VTClient) Lookup(ctx context.Context, hash string) (*VTReport, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base()+"/files/"+url.PathEscape(strings.ToLower(hash)), nil)
	if err != nil {
		return nil, err
	}
	body, code, err := c.do(req, c.client())
	if err != nil {
		return nil, err
	}
	if code == http.StatusNotFound {
		r := &VTReport{SHA256: strings.ToLower(hash)}
		r.Summary = vtSummary(r)
		return r, nil
	}
	var f vtFile
	if err := json.Unmarshal(body, &f); err != nil {
		return nil, errors.New("Respuesta de VirusTotal no válida")
	}
	a := f.Data.Attributes
	r := &VTReport{
		Found:      true,
		Threat:     a.PopularThreatClass.Label,
		Name:       a.MeaningfulName,
		Type:       a.TypeDescription,
		Reputation: a.Reputation,
		SHA256:     a.SHA256,
	}
	if r.SHA256 == "" {
		r.SHA256 = f.Data.ID
	}
	if r.Name == "" && len(a.Names) > 0 {
		r.Name = a.Names[0]
	}
	if a.FirstSubmission > 0 {
		r.FirstSeen = time.Unix(a.FirstSubmission, 0)
	}
	if a.LastAnalysis > 0 {
		r.LastScan = time.Unix(a.LastAnalysis, 0)
	}
	fillStats(r, a.Stats, a.Results)
	return r, nil
}

func fillStats(r *VTReport, s vtStats, res map[string]vtEngine) {
	r.Malicious = s.Malicious
	r.Suspicious = s.Suspicious
	r.Total = s.Malicious + s.Suspicious + s.Undetected + s.Harmless
	for name, e := range res {
		if e.Category != "malicious" && e.Category != "suspicious" {
			continue
		}
		d := VTDetection{Engine: e.EngineName, Category: e.Category}
		if d.Engine == "" {
			d.Engine = name
		}
		if e.Result != nil {
			d.Label = *e.Result
		}
		r.Detections = append(r.Detections, d)
	}
	sort.Slice(r.Detections, func(i, j int) bool {
		a, b := r.Detections[i], r.Detections[j]
		if a.Category != b.Category {
			return a.Category == "malicious"
		}
		return strings.ToLower(a.Engine) < strings.ToLower(b.Engine)
	})
	if r.SHA256 != "" {
		r.Link = "https://www.virustotal.com/gui/file/" + r.SHA256
	}
	r.Summary = vtSummary(r)
}

func vtSummary(r *VTReport) string {
	if !r.Found {
		return "VirusTotal no lo conoce: nadie lo ha subido antes"
	}
	if r.Total == 0 {
		return "VirusTotal lo conoce, pero ningún antivirus pudo analizarlo"
	}
	n := r.Malicious
	if n == 1 {
		return fmt.Sprintf("1 de %d antivirus lo detecta", r.Total)
	}
	s := fmt.Sprintf("%d de %d antivirus lo detectan", n, r.Total)
	if n == 0 && r.Suspicious > 0 {
		s += fmt.Sprintf(" (%d lo ven sospechoso)", r.Suspicious)
	}
	return s
}

// countingReader cuenta lo enviado para informar del avance de la subida.
type countingReader struct {
	r     io.Reader
	sent  int64
	total int64
	fn    func(VTUpload)
	last  time.Time
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.sent += int64(n)
	if c.fn != nil && (time.Since(c.last) > 150*time.Millisecond || err == io.EOF) {
		c.last = time.Now()
		c.fn(VTUpload{Stage: "upload", Sent: c.sent, Total: c.total})
	}
	return n, err
}

// Upload sube el archivo a VirusTotal (pedir permiso antes al usuario: pasa a ser público
// para los suscriptores de VirusTotal) y devuelve el id del análisis.
func (c *VTClient) Upload(ctx context.Context, path string, progress func(VTUpload)) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", errOpen(err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", errOpen(err)
	}
	size := st.Size()
	if size > vtUploadMax {
		return "", ErrVTTooBig
	}
	target := c.base() + "/files"
	if size > vtDirectMax {
		u, err := c.uploadURL(ctx)
		if err != nil {
			return "", err
		}
		target = u
	}

	// Cuerpo multipart con longitud conocida: cabecera + archivo + cierre.
	var head bytes.Buffer
	mw := multipart.NewWriter(&head)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeQuotes(filepath.Base(path))))
	h.Set("Content-Type", "application/octet-stream")
	if _, err := mw.CreatePart(h); err != nil {
		return "", err
	}
	tail := "\r\n--" + mw.Boundary() + "--\r\n"
	body := &countingReader{r: io.MultiReader(&head, f, strings.NewReader(tail)), fn: progress}
	body.total = int64(head.Len()) + size + int64(len(tail))

	req, err := http.NewRequestWithContext(ctx, "POST", target, body)
	if err != nil {
		return "", err
	}
	req.ContentLength = body.total
	req.Header.Set("Content-Type", mw.FormDataContentType())
	// La subida puede tardar: sin límite global, solo el del contexto.
	cl := *c.client()
	cl.Timeout = 0
	resp, _, err := c.do(req, &cl)
	if err != nil {
		return "", err
	}
	var out struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(resp, &out) != nil || out.Data.ID == "" {
		return "", errors.New("Respuesta de VirusTotal no válida")
	}
	return out.Data.ID, nil
}

func (c *VTClient) uploadURL(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base()+"/files/upload_url", nil)
	if err != nil {
		return "", err
	}
	body, code, err := c.do(req, c.client())
	if err != nil {
		return "", err
	}
	var out struct {
		Data string `json:"data"`
	}
	if code != http.StatusOK || json.Unmarshal(body, &out) != nil || out.Data == "" {
		return "", errors.New("VirusTotal no dio una dirección de subida")
	}
	return out.Data, nil
}

func escapeQuotes(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\r", "", "\n", "").Replace(s)
}

// WaitAnalysis consulta el análisis hasta que termina y devuelve el informe.
func (c *VTClient) WaitAnalysis(ctx context.Context, id string, progress func(VTUpload)) (*VTReport, error) {
	wait := c.PollInterval
	if wait <= 0 {
		wait = vtPollInterval
	}
	for {
		req, err := http.NewRequestWithContext(ctx, "GET", c.base()+"/analyses/"+url.PathEscape(id), nil)
		if err != nil {
			return nil, err
		}
		body, code, err := c.do(req, c.client())
		if err != nil && err != ErrVTQuota { // con cuota agotada se espera y se reintenta
			return nil, err
		}
		if err == nil && code == http.StatusNotFound {
			return nil, errors.New("VirusTotal no encuentra el análisis")
		}
		if err == nil {
			var a vtAnalysis
			if json.Unmarshal(body, &a) != nil {
				return nil, errors.New("Respuesta de VirusTotal no válida")
			}
			at := a.Data.Attributes
			if at.Status == "completed" {
				r := &VTReport{Found: true, SHA256: a.Meta.FileInfo.SHA256}
				if at.Date > 0 {
					r.LastScan = time.Unix(at.Date, 0)
					r.FirstSeen = r.LastScan
				}
				fillStats(r, at.Stats, at.Results)
				// La ficha completa añade nombre, tipo y primera vez visto (si la cuota deja).
				if r.SHA256 != "" {
					if lk, err := c.Lookup(ctx, r.SHA256); err == nil && lk.Found {
						r.Name, r.Type, r.Threat, r.Reputation = lk.Name, lk.Type, lk.Threat, lk.Reputation
						if !lk.FirstSeen.IsZero() {
							r.FirstSeen = lk.FirstSeen
						}
					}
				}
				if progress != nil {
					progress(VTUpload{Stage: "done"})
				}
				return r, nil
			}
			if progress != nil {
				stage := "analyzing"
				if at.Status == "queued" {
					stage = "queued"
				}
				progress(VTUpload{Stage: stage})
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
}

// UploadAndWait sube el archivo y espera el resultado del análisis.
func (c *VTClient) UploadAndWait(ctx context.Context, path string, progress func(VTUpload)) (*VTReport, error) {
	id, err := c.Upload(ctx, path, progress)
	if err != nil {
		return nil, err
	}
	if progress != nil {
		progress(VTUpload{Stage: "queued"})
	}
	return c.WaitAnalysis(ctx, id, progress)
}
