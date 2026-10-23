package main

// Vía de control local para TurboMENU («Enviar con TurboSEND» desde el clic derecho).
// Escucha solo en 127.0.0.1 en un puerto al azar y exige un token que se guarda en
// control.json, dentro de la carpeta de datos del usuario. No pasa por el túnel ni por
// el puerto público: es un servidor aparte.

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type controlInfo struct {
	Port    int    `json:"port"`
	Token   string `json:"token"`
	PID     int    `json:"pid"`
	Version string `json:"version"`
}

func controlPath() string { return filepath.Join(dataDir(), "control.json") }

func (a *App) startControl() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return
	}
	token := newID() + newID()
	info := controlInfo{Port: ln.Addr().(*net.TCPAddr).Port, Token: token, PID: os.Getpid(), Version: "2"}
	b, _ := json.Marshal(info)
	tmp := controlPath() + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, controlPath())
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /rooms", a.ctlRooms)
	mux.HandleFunc("POST /send", a.ctlSend)
	mux.HandleFunc("GET /status", a.ctlStatus)
	mux.HandleFunc("POST /show", func(w http.ResponseWriter, r *http.Request) {
		room := r.URL.Query().Get("room")
		runtime.WindowUnminimise(a.ctx)
		runtime.WindowShow(a.ctx)
		if room != "" {
			a.emit("open", room)
		}
		w.WriteHeader(204)
	})
	srv := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := r.Header.Get("X-Turbo-Token")
			host, _, _ := net.SplitHostPort(r.Host)
			if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 || host != "127.0.0.1" || r.Header.Get("Origin") != "" {
				http.Error(w, "no", http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			mux.ServeHTTP(w, r)
		}),
	}
	go srv.Serve(ln)
	a.control = srv
}

func (a *App) stopControl() {
	if a.control != nil {
		a.control.Close()
		// Solo borramos el archivo si sigue siendo el nuestro.
		var info controlInfo
		if loadJSON(controlPath(), &info) == nil && info.PID == os.Getpid() {
			os.Remove(controlPath())
		}
	}
}

func (a *App) ctlRooms(w http.ResponseWriter, r *http.Request) {
	st := a.GetState()
	json.NewEncoder(w).Encode(map[string]any{"rooms": st.Rooms, "host": st.Settings.HostName})
}

func (a *App) ctlSend(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Room  string   `json:"room"`
		Name  string   `json:"name"`
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req); err != nil || len(req.Paths) == 0 {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "Petición incorrecta"})
		return
	}
	room, bundle, err := a.sendTo(req.Room, strings.TrimSpace(req.Name), req.Paths)
	if err != nil {
		w.WriteHeader(422)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"room": room, "bundle": bundle})
}

func (a *App) ctlStatus(w http.ResponseWriter, r *http.Request) {
	room, bundle := r.URL.Query().Get("room"), r.URL.Query().Get("bundle")
	out := map[string]any{"tunnel": a.tunnel.State().Status, "direct": a.direct.State().Status}
	if v, err := a.GetRoom(room); err == nil {
		out["links"] = v.Links
		out["name"] = v.Name
		out["members"] = len(v.Members)
		online := 0
		for _, m := range v.Members {
			if m.Online {
				online++
			}
		}
		out["online"] = online
	}
	if bundle != "" {
		ov := a.outView(bundle)
		out["bundle"] = ov
		list, _, _ := a.tracker.Snapshot()
		var active []TransferView
		for _, t := range list {
			if t.ShareID == bundle {
				active = append(active, t)
			}
		}
		out["transfers"] = active
	}
	json.NewEncoder(w).Encode(out)
}

// sendTo crea un envío en un chat (o en uno nuevo) y devuelve sus identificadores.
func (a *App) sendTo(room, name string, paths []string) (string, string, error) {
	if room == "" || !a.rooms.Exists(room) {
		if name == "" {
			name = "Chat nuevo"
		}
		r := a.rooms.Create(cleanName(name))
		if name != "Chat nuevo" {
			a.rooms.With(r.ID, func(r *Room) bool { r.NameSet = true; return true })
		}
		room = r.ID
	}
	sh, err := a.shares.Create(paths, room)
	if err != nil {
		return "", "", err
	}
	if sh == nil {
		return "", "", errors.New("No hay nada que enviar")
	}
	a.rooms.With(room, func(r *Room) bool {
		if !r.NameSet && r.Name == "Chat nuevo" && len(r.Members) == 0 {
			r.Name = sh.Name
		}
		return true
	})
	m := newMessage("host", "out")
	m.Bundle = sh.ID
	a.post(room, m, false)
	a.emit("roomchg", room)
	return room, sh.ID, nil
}
