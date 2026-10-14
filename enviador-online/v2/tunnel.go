package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const cloudflaredURL = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-windows-amd64.exe"

type TunnelState struct {
	Status    string   `json:"status"` // off | preparing | starting | checking | online | error
	Mode      string   `json:"mode"`   // quick | domain
	DomainErr string   `json:"domainErr"`
	URL       string   `json:"url"`
	Message   string   `json:"message"`
	Progress  float64  `json:"progress"`
	Log       []string `json:"log"`
}

type Tunnel struct {
	mu       sync.Mutex
	st       TunnelState
	cancel   context.CancelFunc
	done     chan struct{}
	port     int
	token    string
	host     string
	nonce    string
	dir      string
	onChange func()
}

func newTunnel(dir, nonce string, onChange func()) *Tunnel {
	return &Tunnel{dir: dir, nonce: nonce, onChange: onChange, st: TunnelState{Status: "off"}}
}

func (t *Tunnel) State() TunnelState {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.st
	s.Log = append([]string(nil), t.st.Log...)
	return s
}

func (t *Tunnel) set(f func(s *TunnelState)) {
	t.mu.Lock()
	f(&t.st)
	t.mu.Unlock()
	if t.onChange != nil {
		t.onChange()
	}
}

func (t *Tunnel) logLine(l string) {
	t.mu.Lock()
	t.st.Log = append(t.st.Log, l)
	if len(t.st.Log) > 200 {
		t.st.Log = t.st.Log[len(t.st.Log)-200:]
	}
	t.mu.Unlock()
}

func (t *Tunnel) Start(port int, token, host string) {
	t.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	t.mu.Lock()
	t.port = port
	t.token, t.host = token, host
	t.cancel = cancel
	t.done = make(chan struct{})
	done := t.done
	t.mu.Unlock()
	go func() {
		defer close(done)
		t.loop(ctx)
	}()
}

func (t *Tunnel) Stop() {
	t.mu.Lock()
	cancel, done := t.cancel, t.done
	t.cancel, t.done = nil, nil
	t.mu.Unlock()
	if cancel != nil {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}
	t.set(func(s *TunnelState) { *s = TunnelState{Status: "off", Mode: "quick", Log: s.Log} })
}

func (t *Tunnel) loop(ctx context.Context) {
	backoff := 2 * time.Second
	t.mu.Lock()
	token, host := t.token, t.host
	t.mu.Unlock()
	if token != "" && host != "" {
		// Primero el dominio propio; si no responde, seguimos con el túnel temporal.
		exe, err := t.ensureBinary(ctx)
		if err == nil {
			err = t.runNamed(ctx, exe, token, host)
		}
		if ctx.Err() != nil {
			return
		}
		msg := "Tu dominio no responde"
		if err != nil {
			msg += ": " + err.Error()
		}
		t.set(func(s *TunnelState) { s.DomainErr = msg + ". Usando enlace temporal." })
	}
	for ctx.Err() == nil {
		exe, err := t.ensureBinary(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			t.set(func(s *TunnelState) { s.Status, s.Message, s.URL = "error", err.Error(), "" })
			sleepCtx(ctx, 20*time.Second)
			continue
		}
		started := time.Now()
		err = t.runOnce(ctx, exe)
		if ctx.Err() != nil {
			return
		}
		msg := "El túnel se ha caído, reconectando…"
		if err != nil {
			msg = err.Error()
		}
		t.set(func(s *TunnelState) { s.Status, s.Message, s.URL = "error", msg, "" })
		if time.Since(started) > time.Minute {
			backoff = 2 * time.Second
		}
		sleepCtx(ctx, backoff)
		if backoff < 60*time.Second {
			backoff *= 2
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// ensureBinary busca cloudflared instalado o lo descarga (verificando la firma de Cloudflare).
func (t *Tunnel) ensureBinary(ctx context.Context) (string, error) {
	local := filepath.Join(t.dir, "bin", "cloudflared.exe")
	if _, err := os.Stat(local); err == nil {
		return local, nil
	}
	if p, err := exec.LookPath("cloudflared"); err == nil {
		return p, nil
	}
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
		p := filepath.Join(os.Getenv(env), "cloudflared", "cloudflared.exe")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	t.set(func(s *TunnelState) {
		s.Status, s.Message, s.Progress = "preparing", "Descargando Cloudflare Tunnel (solo la primera vez)…", 0
	})
	os.MkdirAll(filepath.Dir(local), 0o755)
	part := local + ".part"
	req, _ := http.NewRequestWithContext(ctx, "GET", cloudflaredURL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("no se pudo descargar cloudflared: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("no se pudo descargar cloudflared (HTTP %d)", resp.StatusCode)
	}
	f, err := os.Create(part)
	if err != nil {
		return "", err
	}
	total := resp.ContentLength
	var got int64
	buf := make([]byte, 256<<10)
	last := time.Now()
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				return "", werr
			}
			got += int64(n)
			if total > 0 && time.Since(last) > 200*time.Millisecond {
				last = time.Now()
				pr := float64(got) / float64(total)
				t.set(func(s *TunnelState) { s.Progress = pr })
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			os.Remove(part)
			return "", fmt.Errorf("descarga de cloudflared interrumpida: %w", rerr)
		}
	}
	f.Close()
	if err := verifyCloudflareSignature(part); err != nil {
		os.Remove(part)
		return "", fmt.Errorf("cloudflared descargado rechazado: %w", err)
	}
	if err := os.Rename(part, local); err != nil {
		return "", err
	}
	return local, nil
}

// runNamed levanta el túnel con nombre (dominio propio). Devuelve error si no llega a
// responder; mientras funcione, se queda aquí.
func (t *Tunnel) runNamed(ctx context.Context, exe, token, host string) error {
	u := "https://" + host
	t.set(func(s *TunnelState) {
		s.Status, s.Mode, s.URL, s.Message, s.DomainErr = "starting", "domain", "", "Conectando tu dominio…", ""
	})
	home := filepath.Join(t.dir, "cf-home")
	os.MkdirAll(home, 0o755)
	cmd := exec.Command(exe, "tunnel", "--no-autoupdate", "run")
	// el token va por variable de entorno para que no se vea en la lista de procesos
	cmd.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "TUNNEL_TOKEN="+token)
	hideConsole(cmd)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	cmd.Stdout = io.Discard
	if err := cmd.Start(); err != nil {
		return err
	}
	attachKillOnClose(cmd.Process.Pid)
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	var lastErr string
	go func() {
		sc := bufio.NewScanner(stderr)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			line := sc.Text()
			t.logLine(line)
			if strings.Contains(line, " ERR ") {
				lastErr = cleanLog(line)
			}
		}
	}()

	ready := make(chan bool, 1)
	go func() {
		c := &http.Client{Timeout: 5 * time.Second}
		deadline := time.Now().Add(60 * time.Second)
		for time.Now().Before(deadline) && ctx.Err() == nil {
			req, _ := http.NewRequestWithContext(ctx, "GET", u+"/ping", nil)
			if resp, err := c.Do(req); err == nil {
				b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
				resp.Body.Close()
				if strings.TrimSpace(string(b)) == t.nonce {
					ready <- true
					return
				}
			}
			sleepCtx(ctx, 2*time.Second)
		}
		ready <- false
	}()

	kill := func() { cmd.Process.Kill(); <-exited }
	select {
	case <-ctx.Done():
		kill()
		return nil
	case <-exited:
		if lastErr != "" {
			return errors.New(lastErr)
		}
		return errors.New("cloudflared se cerró (¿token incorrecto?)")
	case ok := <-ready:
		if !ok {
			kill()
			if lastErr != "" {
				return errors.New(lastErr)
			}
			return fmt.Errorf("%s no responde (revisa el «Public hostname» en Cloudflare)", host)
		}
	}
	t.set(func(s *TunnelState) { s.Status, s.URL, s.Message = "online", u, "" })
	select {
	case <-ctx.Done():
		kill()
		return nil
	case <-exited:
		return errors.New("el túnel de tu dominio se ha caído")
	}
}

var tryURLRe = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)

func (t *Tunnel) runOnce(ctx context.Context, exe string) error {
	t.mu.Lock()
	port := t.port
	t.mu.Unlock()
	t.set(func(s *TunnelState) {
		s.Status, s.Mode, s.Message, s.URL, s.Progress = "starting", "quick", "Conectando con Cloudflare…", "", 0
	})

	// HOME aislado: cloudflared se niega a crear quick tunnels si encuentra un config.yml del usuario.
	home := filepath.Join(t.dir, "cf-home")
	os.MkdirAll(home, 0o755)
	cmd := exec.Command(exe, "tunnel", "--no-autoupdate", "--url", fmt.Sprintf("http://127.0.0.1:%d", port))
	cmd.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	hideConsole(cmd)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	cmd.Stdout = io.Discard
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("no se pudo iniciar cloudflared: %w", err)
	}
	attachKillOnClose(cmd.Process.Pid)

	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	checkCtx, cancelCheck := context.WithCancel(ctx)
	defer cancelCheck()
	var lastErr string
	registered := make(chan struct{})
	var regOnce sync.Once
	go func() {
		sc := bufio.NewScanner(stderr)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		gotURL := false
		for sc.Scan() {
			line := sc.Text()
			t.logLine(line)
			if !gotURL {
				if u := tryURLRe.FindString(line); u != "" {
					gotURL = true
					t.set(func(s *TunnelState) {
						s.Status, s.URL, s.Message = "checking", u, "Esperando a que el enlace esté accesible…"
					})
					go t.checkReady(checkCtx, u, registered)
				}
			}
			if strings.Contains(line, "Registered tunnel connection") {
				regOnce.Do(func() { close(registered) })
			}
			if strings.Contains(line, " ERR ") {
				lastErr = line
				if strings.Contains(line, "429") || strings.Contains(strings.ToLower(line), "too many requests") {
					lastErr = "Cloudflare limita los túneles rápidos ahora mismo (demasiadas peticiones). Reintentando…"
				}
			}
		}
	}()

	select {
	case <-ctx.Done():
		cmd.Process.Kill()
		<-exited
		return nil
	case err := <-exited:
		if lastErr != "" {
			return errors.New(cleanLog(lastErr))
		}
		if err != nil {
			return fmt.Errorf("cloudflared terminó: %v", err)
		}
		return errors.New("cloudflared terminó inesperadamente")
	}
}

func cleanLog(l string) string {
	if i := strings.Index(l, " ERR "); i >= 0 {
		l = strings.TrimSpace(l[i+5:])
	}
	if len(l) > 180 {
		l = l[:180] + "…"
	}
	return l
}

// checkReady espera a que el túnel esté registrado, a que responda de verdad entrando
// por la red de Cloudflare y a que su DNS ya exista (consultado por DoH con conexiones
// nuevas para no quedarnos con un "no existe" guardado en caché).
func (t *Tunnel) checkReady(ctx context.Context, u string, registered <-chan struct{}) {
	select {
	case <-registered:
	case <-time.After(25 * time.Second):
	case <-ctx.Done():
		return
	}
	pu, _ := url.Parse(u)
	host := pu.Hostname()
	start := time.Now()
	edge := ""
	for ctx.Err() == nil && time.Since(start) < 45*time.Second {
		if edge == "" {
			edge = dohLookup(ctx, "trycloudflare.com")
		}
		if edge != "" && probePing(ctx, host, edge, t.nonce) {
			if dohLookup(ctx, host) != "" {
				break
			}
		}
		sleepCtx(ctx, 2*time.Second)
	}
	if ctx.Err() != nil {
		return
	}
	msg := ""
	if time.Since(start) >= 45*time.Second {
		msg = "No se pudo verificar el enlace, pero el túnel está levantado."
	}
	t.set(func(s *TunnelState) {
		if s.URL == u {
			s.Status, s.Message = "online", msg
		}
	})
}

var dohRR atomic.Uint32

// dohLookup resuelve un nombre por DNS sobre HTTPS, alternando resolvedores y sin
// reutilizar conexiones (algunos routers bloquean 1.1.1.1).
func dohLookup(ctx context.Context, host string) string {
	bases := []string{"https://cloudflare-dns.com/dns-query", "https://dns.google/resolve", "https://1.1.1.1/dns-query"}
	first := int(dohRR.Add(1))
	for k := range bases {
		base := bases[(first+k)%len(bases)]
		tr := &http.Transport{DisableKeepAlives: true}
		c := &http.Client{Transport: tr, Timeout: 4 * time.Second}
		req, _ := http.NewRequestWithContext(ctx, "GET", base+"?type=A&name="+url.QueryEscape(host), nil)
		req.Header.Set("Accept", "application/dns-json")
		resp, err := c.Do(req)
		if err != nil {
			continue
		}
		var r struct {
			Answer []struct {
				Type int    `json:"type"`
				Data string `json:"data"`
			}
		}
		err = json.NewDecoder(resp.Body).Decode(&r)
		resp.Body.Close()
		if err != nil {
			continue
		}
		for _, a := range r.Answer {
			if a.Type == 1 && net.ParseIP(a.Data) != nil {
				return a.Data
			}
		}
		return ""
	}
	return ""
}

func probePing(ctx context.Context, host, ip, nonce string) bool {
	d := &net.Dialer{Timeout: 4 * time.Second}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return d.DialContext(ctx, "tcp", net.JoinHostPort(ip, "443"))
		},
		TLSClientConfig: &tls.Config{ServerName: host},
	}
	defer tr.CloseIdleConnections()
	c := &http.Client{Transport: tr, Timeout: 6 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://"+host+"/ping", nil)
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	return resp.StatusCode == 200 && strings.TrimSpace(string(b)) == nonce
}
