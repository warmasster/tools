package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type DirectState struct {
	Status   string `json:"status"` // off | searching | ok | verified | manual | cgnat | noupnp | error
	PublicIP string `json:"publicIP"`
	RouterIP string `json:"routerIP"`
	ExtPort  int    `json:"extPort"`
	LocalIP  string `json:"localIP"`
	Lease    int    `json:"lease"`
	Message  string `json:"message"`
}

// Direct gestiona el modo directo: UPnP + detección de CG-NAT.
type Direct struct {
	mu       sync.Mutex
	st       DirectState
	cancel   context.CancelFunc
	done     chan struct{}
	nonce    string
	onChange func()
	// recordPath: dónde se apunta el puerto abierto en el router. Si TurboCAM se
	// cae, en routers que no admiten caducidad el puerto quedaría abierto para
	// siempre: al volver a abrirse, se cierra (o se reutiliza).
	recordPath string
}

type upnpRecord struct {
	Ext      int    `json:"ext"`
	Internal int    `json:"internal"`
	Local    string `json:"local"`
}

func (d *Direct) loadRecord() *upnpRecord {
	if d.recordPath == "" {
		return nil
	}
	var r upnpRecord
	if loadJSON(d.recordPath, &r) != nil || r.Ext <= 0 {
		return nil
	}
	return &r
}

func (d *Direct) saveRecord(r upnpRecord) {
	if d.recordPath != "" {
		if b, err := json.Marshal(r); err == nil {
			os.WriteFile(d.recordPath, b, 0o644)
		}
	}
}

func (d *Direct) clearRecord() {
	if d.recordPath != "" {
		os.Remove(d.recordPath)
	}
}

// dropStale cierra el puerto que dejó abierto una ejecución anterior (si sigue
// siendo nuestro y no es el que se acaba de abrir).
func (d *Direct) dropStale(g *igd, keep int) {
	r := d.loadRecord()
	if r == nil || r.Ext == keep {
		return
	}
	if cl, p := g.mappingOwner(r.Ext); cl == r.Local && p == r.Internal {
		g.Unmap(r.Ext)
	}
}

func newDirect(nonce string, onChange func()) *Direct {
	return &Direct{nonce: nonce, onChange: onChange, st: DirectState{Status: "off"}}
}

func (d *Direct) State() DirectState {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.st
}

func (d *Direct) set(f func(s *DirectState)) {
	d.mu.Lock()
	f(&d.st)
	d.mu.Unlock()
	if d.onChange != nil {
		d.onChange()
	}
}

// Base devuelve https://IP:puerto si el modo directo es utilizable. Es HTTPS con
// certificado propio: el navegador del móvil solo deja usar la cámara en páginas seguras.
func (d *Direct) Base() string {
	s := d.State()
	switch s.Status {
	case "ok", "verified", "manual":
		if s.PublicIP != "" && s.ExtPort > 0 {
			return fmt.Sprintf("https://%s:%d", s.PublicIP, s.ExtPort)
		}
	}
	return ""
}

func (d *Direct) Start(port int, useUPnP, force bool) {
	d.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	d.mu.Lock()
	d.cancel, d.done = cancel, done
	d.mu.Unlock()
	go func() {
		defer close(done)
		d.run(ctx, port, useUPnP, force)
	}()
}

func (d *Direct) Stop() {
	d.mu.Lock()
	cancel, done := d.cancel, d.done
	d.cancel, d.done = nil, nil
	d.mu.Unlock()
	if cancel != nil {
		cancel()
		select {
		case <-done:
		case <-time.After(4 * time.Second):
		}
	}
	d.set(func(s *DirectState) { *s = DirectState{Status: "off"} })
}

func (d *Direct) run(ctx context.Context, port int, useUPnP, force bool) {
	for ctx.Err() == nil {
		g, ext := d.attempt(ctx, port, useUPnP, force)
		if g != nil && ext > 0 {
			d.keep(ctx, g, port, ext)
			unctx, c := context.WithTimeout(context.Background(), 3*time.Second)
			done := make(chan struct{})
			go func() { g.Unmap(ext); close(done) }()
			select {
			case <-done:
				d.clearRecord()
			case <-unctx.Done():
			}
			c()
			if ctx.Err() != nil {
				return
			}
			continue
		}
		// Sin UPnP o con CG-NAT: volvemos a mirar cada 10 minutos por si cambia la red.
		sleepCtx(ctx, 10*time.Minute)
	}
}

func (d *Direct) attempt(ctx context.Context, port int, useUPnP, force bool) (*igd, int) {
	d.set(func(s *DirectState) {
		*s = DirectState{Status: "searching", Message: "Buscando el router…", LocalIP: lanIP()}
	})
	pub, _ := publicIP()
	manual := func(msg string) {
		d.set(func(s *DirectState) {
			s.PublicIP, s.ExtPort = pub, port
			if force && pub != "" {
				s.Status, s.Message = "manual", "Enlace directo forzado: asegúrate de tener el puerto abierto en el router."
			} else {
				s.Status, s.Message = "noupnp", msg
			}
		})
	}
	if !useUPnP {
		manual("UPnP desactivado en ajustes.")
		if d.loadRecord() != nil {
			// Quedó un puerto abierto de antes de desactivar UPnP (o de un cierre a
			// destiempo): se cierra.
			if g, err := discoverIGD(ctx); err == nil {
				d.dropStale(g, 0)
				d.clearRecord()
			}
		}
		return nil, 0
	}
	g, err := discoverIGD(ctx)
	if err != nil {
		manual("Tu router no responde por UPnP (puede estar desactivado en su configuración).")
		return nil, 0
	}
	routerIP, _ := g.ExternalIP()
	d.set(func(s *DirectState) { s.RouterIP, s.PublicIP, s.LocalIP = routerIP, pub, g.localIP })
	if routerIP != "" && (isCGNATorPrivate(routerIP) || (pub != "" && routerIP != pub)) {
		d.set(func(s *DirectState) {
			s.Status = "cgnat"
			s.Message = "Tu operadora usa CG-NAT o hay doble router: desde Internet no se puede llegar directo a tu PC. Usa el enlace de Cloudflare."
		})
		return nil, 0
	}
	ext, lease, err := g.Map(port)
	if err != nil {
		d.set(func(s *DirectState) {
			s.Status, s.Message = "error", "El router no ha aceptado abrir el puerto: "+err.Error()
		})
		return nil, 0
	}
	d.dropStale(g, ext)
	d.saveRecord(upnpRecord{Ext: ext, Internal: port, Local: g.localIP})
	if pub == "" {
		pub = routerIP
	}
	d.set(func(s *DirectState) {
		s.Status, s.PublicIP, s.ExtPort, s.Lease = "ok", pub, ext, lease
		s.Message = "Puerto abierto en el router."
	})
	if d.selfTest(ctx, pub, ext) {
		d.set(func(s *DirectState) { s.Status, s.Message = "verified", "Comprobado: tu PC responde desde Internet." })
	}
	return g, ext
}

// selfTest intenta llegar a nuestra propia IP pública (solo funciona si el router hace NAT loopback).
func (d *Direct) selfTest(ctx context.Context, ip string, port int) bool {
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://%s:%d/ping", ip, port), nil)
	// El certificado es nuestro y autofirmado: aquí solo se comprueba que responde.
	c := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	return strings.TrimSpace(string(b)) == d.nonce
}

// keep renueva la concesión y vigila cambios de IP pública.
func (d *Direct) keep(ctx context.Context, g *igd, port, ext int) {
	tick := time.NewTicker(5 * time.Minute)
	defer tick.Stop()
	lastRenew := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		lease := d.State().Lease
		if lease > 0 && time.Since(lastRenew) > time.Duration(lease/2)*time.Second {
			if err := g.addMapping(ext, port, lease); err == nil {
				lastRenew = time.Now()
			}
		}
		if pub, err := publicIP(); err == nil && pub != d.State().PublicIP {
			return // la IP ha cambiado: repetir todo el proceso
		}
	}
}
