package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
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

// Base devuelve http://IP:puerto si el modo directo es utilizable.
func (d *Direct) Base() string {
	s := d.State()
	switch s.Status {
	case "ok", "verified", "manual":
		if s.PublicIP != "" && s.ExtPort > 0 {
			return fmt.Sprintf("http://%s:%d", s.PublicIP, s.ExtPort)
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
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://%s:%d/ping", ip, port), nil)
	c := &http.Client{Timeout: 3 * time.Second}
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
