package safe

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf16"
)

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "datos.bin")
	data := make([]byte, 9<<20+123) // más de dos bloques
	for i := range data {
		data[i] = byte(i * 7)
	}
	os.WriteFile(p, data, 0o644)
	var calls int
	h, err := HashFile(context.Background(), p, func(done, total int64) { calls++ })
	if err != nil {
		t.Fatal(err)
	}
	s256, s1, m5 := sha256.Sum256(data), sha1.Sum(data), md5.Sum(data)
	if h.SHA256 != hex.EncodeToString(s256[:]) || h.SHA1 != hex.EncodeToString(s1[:]) || h.MD5 != hex.EncodeToString(m5[:]) {
		t.Fatalf("huellas distintas: %+v", h)
	}
	if h.Size != int64(len(data)) || calls < 2 {
		t.Fatalf("tamaño %d, llamadas %d", h.Size, calls)
	}

	// Vacío y cancelado.
	empty := filepath.Join(dir, "vacio")
	os.WriteFile(empty, nil, 0o644)
	if h, err := HashFile(context.Background(), empty, nil); err != nil || h.SHA256 != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatalf("vacío: %v %+v", err, h)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := HashFile(ctx, p, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("esperaba cancelado: %v", err)
	}
	if _, err := HashFile(context.Background(), filepath.Join(dir, "no"), nil); err == nil {
		t.Fatal("esperaba error")
	}
}

// Velocidad con un archivo grande: TURBOMENU_BIG=ruta (opcional).
func TestHashSpeed(t *testing.T) {
	p := os.Getenv("TURBOMENU_BIG")
	if p == "" {
		t.Skip("define TURBOMENU_BIG")
	}
	start := time.Now()
	h, err := HashFile(context.Background(), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	d := time.Since(start)
	t.Logf("%d MB en %v (%.0f MB/s)", h.Size>>20, d, float64(h.Size)/(1<<20)/d.Seconds())
}

func writeZone(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path+zoneStream, []byte(content), 0o644); err != nil {
		t.Skip("el disco no admite flujos alternativos: " + err.Error())
	}
}

func TestOrigin(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "setup.exe")
	os.WriteFile(p, []byte("x"), 0o644)

	o, err := ReadOrigin(p)
	if err != nil || o.Marked || o.Summary != "Este archivo no tiene origen registrado" || o.Created.IsZero() {
		t.Fatalf("sin marca: %v %+v", err, o)
	}

	cases := []struct {
		name, zone string
		want       func(*Origin) bool
		summary    string
	}{
		{"navegador completo", "[ZoneTransfer]\r\nZoneId=3\r\nReferrerUrl=https://www.example.com/descargas\r\nHostUrl=https://cdn.example.org/setup.exe\r\n",
			func(o *Origin) bool {
				return o.ZoneID == 3 && o.Zone == "Internet" && o.Host == "https://cdn.example.org/setup.exe" && o.Site == "example.com"
			}, "Se descargó de example.com"},
		{"nulo al final", "[ZoneTransfer]\r\nZoneId=3\r\nHostUrl=https://dl.example.net/a.rar?t=1\x00",
			func(o *Origin) bool {
				return o.Host == "https://dl.example.net/a.rar?t=1" && o.Site == "dl.example.net"
			},
			"Se descargó de dl.example.net"},
		{"solo zona", "[ZoneTransfer]\r\nZoneId=3\r\n",
			func(o *Origin) bool { return o.Host == "" && o.Referrer == "" },
			"Llegó de internet, pero el programa que lo guardó no anotó la dirección"},
		{"extraído de zip", "[ZoneTransfer]\r\nZoneId=3\r\nReferrerUrl=C:\\Users\\a\\Downloads\\juego.zip\r\n",
			func(o *Origin) bool { return o.FromArchive == `C:\Users\a\Downloads\juego.zip` && o.Referrer == "" },
			"Se sacó del comprimido «juego.zip», que vino de internet"},
		{"correo", "[ZoneTransfer]\r\nZoneId=3\r\nLastWriterPackageFamilyName=Microsoft.OutlookForWindows_8wekyb3d8bbwe\r\n",
			func(o *Origin) bool { return o.SavedBy == "Correo (Outlook)" },
			"Llegó de internet a través de Correo (Outlook)"},
		{"intranet", "[ZoneTransfer]\nZoneId=1\nHostUrl=about:internet\n",
			func(o *Origin) bool { return o.Zone == "Red local (intranet)" && o.Host == "" },
			"Llegó desde la red local"},
	}
	for _, c := range cases {
		writeZone(t, p, c.zone)
		o, err := ReadOrigin(p)
		if err != nil || !o.Marked || !c.want(o) || o.Summary != c.summary {
			t.Errorf("%s: %v %+v", c.name, err, o)
		}
	}

	// En UTF-16 con BOM.
	u := utf16.Encode([]rune("[ZoneTransfer]\r\nZoneId=3\r\nHostUrl=https://github.com/x.exe\r\n"))
	b := []byte{0xFF, 0xFE}
	for _, r := range u {
		b = append(b, byte(r), byte(r>>8))
	}
	os.WriteFile(p+zoneStream, b, 0o644)
	if o, _ := ReadOrigin(p); o.Site != "github.com" {
		t.Errorf("utf16: %+v", o)
	}

	// Desbloquear.
	if err := Unblock(p); err != nil {
		t.Fatal(err)
	}
	if o, _ := ReadOrigin(p); o.Marked {
		t.Fatal("la marca sigue")
	}
	if err := Unblock(p); err != nil {
		t.Fatal("sin marca no debería fallar:", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "x" {
		t.Fatal("el contenido cambió")
	}
}

// Servidor que imita la API v3 de VirusTotal.
const (
	testKey   = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	knownHash = "275a021bbfb6489e54d471899f7db9d1663fc695ec2fe2a2c4538aabf651fd0f"
)

const vtFileJSON = `{"data": {"id": "275a021bbfb6489e54d471899f7db9d1663fc695ec2fe2a2c4538aabf651fd0f", "type": "file",
 "links": {"self": "https://www.virustotal.com/api/v3/files/275a021bbfb6489e54d471899f7db9d1663fc695ec2fe2a2c4538aabf651fd0f"},
 "attributes": {"first_submission_date": 1148301722, "last_analysis_date": 1729000000,
  "meaningful_name": "eicar.com", "names": ["eicar.com", "eicar.txt"], "type_description": "DOS COM",
  "reputation": -1234, "sha256": "275a021bbfb6489e54d471899f7db9d1663fc695ec2fe2a2c4538aabf651fd0f",
  "popular_threat_classification": {"suggested_threat_label": "virus.eicar/test"},
  "last_analysis_stats": {"malicious": 3, "suspicious": 1, "undetected": 2, "harmless": 0, "timeout": 0,
   "confirmed-timeout": 0, "failure": 0, "type-unsupported": 1},
  "last_analysis_results": {
   "Kaspersky": {"category": "malicious", "engine_name": "Kaspersky", "engine_version": "22.0", "result": "EICAR-Test-File", "method": "blacklist", "engine_update": "20241015"},
   "ESET-NOD32": {"category": "malicious", "engine_name": "ESET-NOD32", "result": "Eicar test file", "method": "blacklist"},
   "Avast": {"category": "malicious", "engine_name": "Avast", "result": "EICAR Test-NOT virus!!!", "method": "blacklist"},
   "Bkav": {"category": "suspicious", "engine_name": "Bkav", "result": "W32.AIDetect", "method": "blacklist"},
   "ClamAV": {"category": "undetected", "engine_name": "ClamAV", "result": null, "method": "blacklist"},
   "Zoner": {"category": "undetected", "engine_name": "Zoner", "result": null, "method": "blacklist"},
   "Trapmine": {"category": "type-unsupported", "engine_name": "Trapmine", "result": null, "method": "blacklist"}
  }}}}`

var lastUpload atomic.Value // ruta a la que llegó la última subida

func fakeVT(t *testing.T) (*httptest.Server, *int32) {
	var polls int32
	mux := http.NewServeMux()
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		switch r.Header.Get("x-apikey") {
		case testKey:
			return true
		case strings.Repeat("f", 64):
			w.WriteHeader(429)
			io.WriteString(w, `{"error": {"code": "QuotaExceededError", "message": "Quota exceeded"}}`)
			return false
		}
		w.WriteHeader(401)
		io.WriteString(w, `{"error": {"code": "WrongCredentialsError", "message": "Wrong API key"}}`)
		return false
	}
	var srv *httptest.Server
	mux.HandleFunc("GET /files/upload_url", func(w http.ResponseWriter, r *http.Request) {
		if auth(w, r) {
			io.WriteString(w, `{"data": "`+srv.URL+`/_ah/upload/abc"}`)
		}
	})
	mux.HandleFunc("GET /files/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		if r.PathValue("id") == knownHash {
			io.WriteString(w, vtFileJSON)
			return
		}
		w.WriteHeader(404)
		fmt.Fprintf(w, `{"error": {"code": "NotFoundError", "message": "File \"%s\" not found"}}`, r.PathValue("id"))
	})
	upload := func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		lastUpload.Store(r.URL.Path)
		f, fh, err := r.FormFile("file")
		if err != nil {
			w.WriteHeader(400)
			return
		}
		b, _ := io.ReadAll(f)
		if fh.Filename != "subir.exe" || string(b) != "contenido" {
			w.WriteHeader(400)
			return
		}
		io.WriteString(w, `{"data": {"type": "analysis", "id": "YW5hbGlzaXM=", "links": {"self": "x"}}}`)
	}
	mux.HandleFunc("POST /files", upload)
	mux.HandleFunc("POST /_ah/upload/abc", upload)
	mux.HandleFunc("GET /analyses/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		n := atomic.AddInt32(&polls, 1)
		status := "queued"
		if n >= 3 {
			status = "completed"
		}
		fmt.Fprintf(w, `{"data": {"id": "YW5hbGlzaXM=", "type": "analysis", "attributes": {"date": 1729000000, "status": %q,
 "stats": {"harmless": 0, "malicious": 0, "suspicious": 0, "undetected": 65, "timeout": 0, "type-unsupported": 8},
 "results": {"ClamAV": {"category": "undetected", "engine_name": "ClamAV", "result": null}}}},
 "meta": {"file_info": {"sha256": "aaaa", "sha1": "b", "md5": "c", "size": 9}}}`, status)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &polls
}

func TestValidVTKey(t *testing.T) {
	if !ValidVTKey(testKey) || !ValidVTKey(" "+strings.ToUpper(testKey)+"\n") {
		t.Fatal("clave buena rechazada")
	}
	for _, k := range []string{"", "abc", testKey + "0", strings.Repeat("g", 64)} {
		if ValidVTKey(k) {
			t.Fatalf("clave mala aceptada: %q", k)
		}
	}
}

func TestVTLookup(t *testing.T) {
	srv, _ := fakeVT(t)
	c := &VTClient{Key: testKey, BaseURL: srv.URL}
	r, err := c.Lookup(context.Background(), strings.ToUpper(knownHash))
	if err != nil {
		t.Fatal(err)
	}
	if !r.Found || r.Malicious != 3 || r.Suspicious != 1 || r.Total != 6 || len(r.Detections) != 4 ||
		r.Detections[0].Engine != "Avast" || r.Detections[3].Category != "suspicious" ||
		r.Name != "eicar.com" || r.FirstSeen.Year() != 2006 || r.Threat != "virus.eicar/test" ||
		r.Link != "https://www.virustotal.com/gui/file/"+knownHash || r.Summary != "3 de 6 antivirus lo detectan" {
		t.Fatalf("%+v", r)
	}

	r, err = c.Lookup(context.Background(), strings.Repeat("0", 64))
	if err != nil || r.Found || r.Summary != "VirusTotal no lo conoce: nadie lo ha subido antes" {
		t.Fatalf("no encontrado: %v %+v", err, r)
	}

	bad := &VTClient{Key: strings.Repeat("e", 64), BaseURL: srv.URL}
	if _, err := bad.Lookup(context.Background(), knownHash); err != ErrVTKey {
		t.Fatalf("clave mala: %v", err)
	}
	quota := &VTClient{Key: strings.Repeat("f", 64), BaseURL: srv.URL}
	if _, err := quota.Lookup(context.Background(), knownHash); err != ErrVTQuota {
		t.Fatalf("cuota: %v", err)
	}
	if _, err := (&VTClient{Key: "corta", BaseURL: srv.URL}).Lookup(context.Background(), knownHash); err != ErrVTKey {
		t.Fatalf("formato: %v", err)
	}
	off := &VTClient{Key: testKey, BaseURL: "http://127.0.0.1:1"}
	if _, err := off.Lookup(context.Background(), knownHash); err != ErrVTNetwork {
		t.Fatalf("sin red: %v", err)
	}
}

func TestVTUploadAndWait(t *testing.T) {
	srv, polls := fakeVT(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "subir.exe")
	os.WriteFile(p, []byte("contenido"), 0o644)

	c := &VTClient{Key: testKey, BaseURL: srv.URL, PollInterval: 10 * time.Millisecond}
	var stages []string
	var sent int64
	r, err := c.UploadAndWait(context.Background(), p, func(u VTUpload) {
		if len(stages) == 0 || stages[len(stages)-1] != u.Stage {
			stages = append(stages, u.Stage)
		}
		if u.Stage == "upload" {
			sent = u.Sent
			if u.Total < 9 {
				t.Errorf("total %d", u.Total)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 65 || r.Malicious != 0 || r.Summary != "0 de 65 antivirus lo detectan" || atomic.LoadInt32(polls) != 3 {
		t.Fatalf("%+v polls=%d", r, *polls)
	}
	if strings.Join(stages, ",") != "upload,queued,done" || sent == 0 {
		t.Fatalf("etapas %v enviado %d", stages, sent)
	}

	// Archivo de más de 32 MB: pide antes la URL de subida.
	big := filepath.Join(dir, "subir2.exe")
	os.WriteFile(big, make([]byte, vtDirectMax+1), 0o644)
	if _, err := c.Upload(context.Background(), big, nil); err == nil || err.Error() != "VirusTotal respondió con un error (400)" {
		// el servidor de prueba rechaza el contenido, pero la petición debe llegar a /_ah/upload
		t.Fatalf("grande: %v", err)
	}
	if lastUpload.Load() != "/_ah/upload/abc" {
		t.Fatalf("la subida grande fue a %v", lastUpload.Load())
	}

	// Cancelar durante el sondeo.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	slow := &VTClient{Key: testKey, BaseURL: srv.URL, PollInterval: time.Hour}
	atomic.StoreInt32(polls, -100)
	if _, err := slow.WaitAnalysis(ctx, "YW5hbGlzaXM=", nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancelar: %v", err)
	}
}

func TestJudge(t *testing.T) {
	signed := &Signature{Status: SignValid, Signer: "Valve Corporation", Summary: "Firmado por Valve Corporation"}
	unsigned := &Signature{Status: SignUnsigned, Summary: "Sin firma digital"}
	defClean := &DefenderResult{Available: true, Clean: true, Summary: "Microsoft Defender no encuentra nada"}
	defBad := &DefenderResult{Available: true, Threats: []string{"Trojan:Win32/X"}, Summary: "Microsoft Defender lo detecta como Trojan:Win32/X"}
	vt := func(m, total int) *VTReport {
		r := &VTReport{Found: true, Malicious: m, Total: total}
		r.Summary = vtSummary(r)
		return r
	}
	net := &Origin{Marked: true, ZoneID: 3, Site: "example.com", Summary: "Se descargó de example.com"}

	cases := []struct {
		name    string
		ev      Evidence
		level   Level
		summary string
	}{
		{"nada aún", Evidence{Path: "a.exe"}, Unknown, "Analizando…"},
		{"vt limpio", Evidence{Path: "a.exe", VirusTotal: vt(0, 70), Signature: unsigned}, Clean, "0 de 70 antivirus lo detectan"},
		{"firmado + defender", Evidence{Path: "a.exe", Signature: signed, Defender: defClean}, Clean, "Firmado por Valve Corporation"},
		{"defender detecta", Evidence{Path: "a.exe", Signature: signed, Defender: defBad, VirusTotal: vt(0, 70)}, Dangerous, defBad.Summary},
		{"muchas detecciones", Evidence{Path: "a.exe", VirusTotal: vt(30, 70)}, Dangerous, "30 de 70 antivirus lo detectan"},
		{"pocas detecciones", Evidence{Path: "a.exe", VirusTotal: vt(3, 70), Signature: unsigned}, Suspicious, "3 de 70 antivirus lo detectan"},
		{"una + firmado", Evidence{Path: "a.exe", VirusTotal: vt(1, 70), Signature: signed}, Clean, "1 de 70 antivirus lo detecta (seguramente una falsa alarma)"},
		{"una sin firma", Evidence{Path: "a.exe", VirusTotal: vt(1, 70), Signature: unsigned}, Suspicious, "1 de 70 antivirus lo detecta"},
		{"sin firma de internet", Evidence{Path: "a.exe", Signature: unsigned, Defender: defClean, Origin: net}, Unknown, "Sin firma, descargado de example.com"},
		{"sin firma local", Evidence{Path: "a.exe", Signature: unsigned, Defender: defClean, Origin: &Origin{}}, Unknown, "Sin firma, de un origen desconocido"},
		{"foto + defender", Evidence{Path: "foto.jpg", Signature: unsigned, Defender: defClean, VirusTotal: &VTReport{Summary: "x"}}, Clean, defClean.Summary},
		{"firma rota", Evidence{Path: "a.exe", Signature: &Signature{Status: SignTampered, Summary: "rota"}, Defender: defClean}, Suspicious, "rota"},
		{"rar de internet", Evidence{Path: "a.rar", Signature: unsigned, Defender: defClean, Origin: net}, Unknown, "Descargado de example.com, sin más datos para comprobarlo"},
		{"conocido", Evidence{Path: "a.dll", Signature: unsigned, Known: &KnownFile{Known: true, Summary: "Archivo conocido y legítimo"}}, Clean, "Archivo conocido y legítimo"},
		{"conocido malicioso", Evidence{Path: "a.dll", Known: &KnownFile{Known: true, Malicious: true, Summary: "mal"}}, Dangerous, "mal"},
	}
	for _, c := range cases {
		v := Judge(c.ev)
		if v.Level != c.level || v.Summary != c.summary || v.Title == "" || v.Reasons == nil {
			t.Errorf("%s: %+v", c.name, v)
		}
	}
}

func TestParseDefender(t *testing.T) {
	out := "Scan starting...\r\nScan finished.\r\nScanning C:\\x\\eicar.com found 1 threats.\r\n\r\n" +
		"<===========================LIST OF DETECTED THREATS==========================>\r\n" +
		"----------------------------- Threat information ------------------------------\r\n" +
		"Threat                  : Virus:DOS/EICAR_Test_File\r\nResources               : 1 total\r\n"
	r, _ := parseDefender(&DefenderResult{Available: true}, 2, out)
	if r.Clean || len(r.Threats) != 1 || r.Threats[0] != "Virus:DOS/EICAR_Test_File" {
		t.Fatalf("%+v", r)
	}
	r, _ = parseDefender(&DefenderResult{Available: true}, 0, "Scan starting...\nScan finished.\nScanning x found no threats.\n")
	if !r.Clean {
		t.Fatalf("%+v", r)
	}
	r, err := parseDefender(&DefenderResult{Available: true}, 2147943354, "CmdTool: Failed with hr = 0x800106BA. Check C:\\...\\MpCmdRun.log")
	if err != nil || r.Available {
		t.Fatalf("no disponible: %v %+v", err, r)
	}
	if versionLess("4.18.26090.9-0", "4.18.26080.4-0") || !versionLess("4.18.9.1-0", "4.18.26080.4-0") {
		t.Fatal("versiones")
	}
}
