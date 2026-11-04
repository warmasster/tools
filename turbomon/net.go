package main

// Red en el agente: qué programa descarga o sube (eventos de Windows), el ping
// al servidor del juego y a internet, y la búsqueda de redes wifi apagada
// mientras se juega.

import (
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"turbomon/internal/netmon"
	"turbomon/internal/parts"
)

type netState struct {
	mu       sync.Mutex
	mon      *netmon.Monitor
	on       bool
	lastPoll time.Time
	procs    []NetProc
	server   netmon.Server
	hasSrv   bool
	gamePing *netmon.Series
	inetPing *netmon.Series
	noICMP   bool
	misses   int
	wifiOff  bool // la búsqueda de redes está apagada por TurboMON
}

// NetProc es un programa usando la red.
type NetProc struct {
	Name string  `json:"name"`
	Exe  string  `json:"exe"`
	Down float64 `json:"down"`
	Up   float64 `json:"up"`
	Game bool    `json:"game"`
}

var inetRef = netip.MustParseAddr("8.8.8.8") // 1.1.1.1 lo contestan algunos routers por su cuenta

func newNetState() *netState {
	return &netState{mon: netmon.New(), gamePing: netmon.NewSeries(600), inetPing: netmon.NewSeries(600)}
}

// netTick: una vez por segundo. Escucha la red solo si hay partida o la ventana la mira.
func (a *Agent) netTick() {
	n := a.net
	a.st.mu.Lock()
	cur := a.st.cur
	gamePid := uint32(0)
	if cur != nil {
		gamePid = cur.pid
	}
	a.st.mu.Unlock()
	n.mu.Lock()
	want := a.etw != nil && (gamePid != 0 || time.Since(n.lastPoll) < 10*time.Second)
	on := n.on
	n.mu.Unlock()
	if want && !on {
		if err := a.etw.Events().Enable(netmon.Provider, 4, netmon.Keywords, nil, n.mon.Handle); err == nil {
			n.mu.Lock()
			n.on = true
			n.mu.Unlock()
		}
	} else if !want && on {
		a.etw.Events().Disable(netmon.Provider)
		n.mu.Lock()
		n.on, n.procs, n.hasSrv = false, nil, false
		n.mu.Unlock()
		return
	}
	if !want {
		return
	}
	rates := n.mon.Rates()
	byExe := map[string]*NetProc{}
	for _, r := range rates {
		pi := a.procInfo(r.PID)
		key := pi.exe
		if key == "" {
			key = "sistema"
		}
		p := byExe[key]
		if p == nil {
			name := pi.game
			if name == "" {
				name = strings.TrimSuffix(key, ".exe")
			}
			p = &NetProc{Name: name, Exe: key}
			byExe[key] = p
		}
		p.Down += r.Down
		p.Up += r.Up
		if r.PID == gamePid {
			p.Game = true
		}
	}
	list := make([]NetProc, 0, len(byExe))
	for _, p := range byExe {
		if p.Down+p.Up >= 512 || p.Game {
			list = append(list, *p)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Down+list[i].Up > list[j].Down+list[j].Up })
	if len(list) > 12 {
		list = list[:12]
	}
	srv, ok := netmon.Server{}, false
	if gamePid != 0 {
		srv, ok = n.mon.GameServer(gamePid)
	}
	n.mu.Lock()
	n.procs = list
	if ok && (!n.hasSrv || srv.Addr != n.server.Addr) {
		n.server, n.hasSrv, n.noICMP, n.misses = srv, true, false, 0
		n.gamePing.Reset()
		logf("servidor del juego: %v (UDP %v)", srv.Addr, srv.UDP)
	} else if !ok && gamePid == 0 {
		n.hasSrv = false
	}
	n.mu.Unlock()
	if time.Now().Unix()%30 == 0 {
		n.mon.Forget(processAlive)
	}
}

// pingLoop mide el ping cada segundo mientras se escucha la red.
func (a *Agent) pingLoop() {
	n := a.net
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-a.quit:
			return
		case <-t.C:
		}
		n.mu.Lock()
		on, has, srv, noICMP := n.on, n.hasSrv, n.server, n.noICMP
		n.mu.Unlock()
		if !on {
			continue
		}
		if d, err := netmon.Ping(inetRef, time.Second); err == nil {
			n.inetPing.Add(float64(d.Microseconds())/1000, true)
		} else {
			n.inetPing.Add(0, false)
		}
		if has && !noICMP {
			d, err := netmon.Ping(srv.Addr.Addr(), time.Second)
			n.mu.Lock()
			if err != nil {
				n.misses++
				// Muchos servidores de juegos no contestan al ping: tras 5 fallos seguidos se deja.
				if n.misses >= 5 && n.gamePing.Stats().Count <= n.misses {
					n.noICMP = true
				}
			} else {
				n.misses = 0
			}
			n.mu.Unlock()
			if err == nil {
				n.gamePing.Add(float64(d.Microseconds())/1000, true)
			} else if !n.noICMP {
				n.gamePing.Add(0, false)
			}
		}
	}
}

func (a *Agent) hNet(w http.ResponseWriter, r *http.Request) {
	n := a.net
	n.mu.Lock()
	n.lastPoll = time.Now()
	procs := n.procs
	if procs == nil {
		procs = []NetProc{}
	}
	out := map[string]any{"active": n.on, "elevated": a.elevated, "procs": procs, "wifiOff": n.wifiOff}
	if n.hasSrv {
		out["server"] = map[string]any{"addr": n.server.Addr.String(), "udp": n.server.UDP, "noPing": n.noICMP}
	}
	n.mu.Unlock()
	out["gamePing"] = n.gamePing.Stats()
	out["inetPing"] = n.inetPing.Stats()
	a.st.mu.Lock()
	if a.st.cur != nil {
		out["game"] = a.st.cur.game
	}
	a.st.mu.Unlock()
	a.kick()
	writeJSON(w, out)
}

// ---------- wifi sin búsqueda de redes durante la partida ----------

func wifiMarker() string { return filepath.Join(dataDir(), "wifi-busqueda-apagada.txt") }

func (a *Agent) wifiSessionStart() {
	if !getSettings().WifiNoScan {
		return
	}
	onWifi := false
	for _, ad := range parts.Read(0).Network {
		if ad.Default && ad.Kind == "wifi" {
			onWifi = true
		}
	}
	if !onWifi {
		return
	}
	if parts.WifiBackgroundScan(false) > 0 {
		a.net.mu.Lock()
		a.net.wifiOff = true
		a.net.mu.Unlock()
		os.WriteFile(wifiMarker(), []byte("1"), 0o644)
		logf("wifi: búsqueda de redes apagada durante la partida")
	}
}

func (a *Agent) wifiSessionEnd() {
	a.net.mu.Lock()
	off := a.net.wifiOff
	a.net.wifiOff = false
	a.net.mu.Unlock()
	if off {
		parts.WifiBackgroundScan(true)
		os.Remove(wifiMarker())
		logf("wifi: búsqueda de redes encendida otra vez")
	}
}

// restoreWifi deja la búsqueda encendida si TurboMON se cerró en mitad de una partida.
func restoreWifi() {
	if _, err := os.Stat(wifiMarker()); err == nil {
		parts.WifiBackgroundScan(true)
		os.Remove(wifiMarker())
	}
}

// sessionNet resume la red de una partida: ping y los programas que más descargaron.
func (a *Agent) sessionNet(c *liveSession) (*PingStat, []NetHog) {
	n := a.net
	var ps *PingStat
	g, in := n.gamePing.Stats(), n.inetPing.Stats()
	n.mu.Lock()
	srv, has, noICMP := n.server, n.hasSrv, n.noICMP
	n.mu.Unlock()
	if g.Count > 10 || in.Count > 10 {
		ps = &PingStat{Avg: g.Avg, P95: g.P95, Jitter: g.Jitter, Loss: g.Loss, Inet: in.Avg, NoICMP: noICMP}
		if has {
			ps.Server = srv.Addr.String()
		}
	}
	if c.netStart == nil {
		return ps, nil
	}
	byName := map[string]*NetHog{}
	for pid, tot := range n.mon.Totals() {
		if pid == c.pid {
			continue
		}
		st := c.netStart[pid]
		down, up := tot[0]-st[0], tot[1]-st[1]
		if tot[0] < st[0] || tot[1] < st[1] || down+up < 50<<20 {
			continue
		}
		pi := a.procInfo(pid)
		name := pi.game
		if name == "" {
			name = strings.TrimSuffix(pi.exe, ".exe")
		}
		h := byName[name]
		if h == nil {
			h = &NetHog{Name: name}
			byName[name] = h
		}
		h.Down += down
		h.Up += up
	}
	var hogs []NetHog
	for _, h := range byName {
		hogs = append(hogs, *h)
	}
	sort.Slice(hogs, func(i, j int) bool { return hogs[i].Down+hogs[i].Up > hogs[j].Down+hogs[j].Up })
	if len(hogs) > 3 {
		hogs = hogs[:3]
	}
	return ps, hogs
}
