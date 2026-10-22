package main

import (
	"encoding/json"
	"errors"
	"hash/crc32"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------- vistas (lo que se manda a la interfaz y a los navegadores) ----------

type MemberView struct {
	PID    string `json:"pid"`
	Name   string `json:"name"`
	Color  int    `json:"color"`
	Online bool   `json:"online"`
}

type OutView struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	Count     int      `json:"count"`
	Total     int64    `json:"total"`
	Downloads int      `json:"downloads"`
	Preview   []string `json:"preview,omitempty"`
	Gone      bool     `json:"gone,omitempty"`
}

type InView struct {
	ID       string      `json:"id"`
	From     string      `json:"from"`
	Name     string      `json:"name"`
	Kind     string      `json:"kind"`
	Count    int         `json:"count"`
	Total    int64       `json:"total"`
	Received int64       `json:"received"`
	Status   string      `json:"status"`
	Error    string      `json:"error,omitempty"`
	Root     string      `json:"root,omitempty"`
	Files    []offerFile `json:"files,omitempty"`
}

type RoomView struct {
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	HostName string             `json:"hostName"`
	Created  int64              `json:"created"`
	Updated  int64              `json:"updated"`
	Members  []MemberView       `json:"members"`
	Messages []Message          `json:"messages"`
	Out      map[string]OutView `json:"out"`
	In       map[string]InView  `json:"in"`
	Auto     string             `json:"auto"`
	Unread   int                `json:"unread"`
	Typing   []string           `json:"typing"`
	Links    *ShareLinks        `json:"links,omitempty"`
}

func inView(b *InBundle, guest bool, viewer string) InView {
	v := InView{ID: b.ID, From: b.From, Name: b.Name, Kind: b.Kind, Count: len(b.Files), Total: b.Total,
		Received: b.Received, Status: b.Status, Error: b.Error}
	if b.Status == "finishing" {
		v.Status = "transferring"
	}
	if !guest {
		v.Root = b.Root
	} else if b.From == viewer && b.Status != "done" && b.Status != "rejected" && b.Status != "cancelled" {
		for _, f := range b.Files {
			v.Files = append(v.Files, offerFile{Rel: f.Rel, Size: f.Size})
		}
	}
	return v
}

func (a *App) outView(id string) OutView {
	sh := a.shares.Get(id)
	if sh == nil {
		return OutView{ID: id, Gone: true, Name: "Envío eliminado"}
	}
	v := OutView{ID: sh.ID, Name: sh.Name, Kind: sh.Kind, Count: len(sh.Files), Total: sh.Total, Downloads: sh.Downloads}
	if sh.Kind == "multi" {
		for i, f := range sh.Files {
			if i >= 4 {
				break
			}
			v.Preview = append(v.Preview, baseName(f.Rel))
		}
	}
	return v
}

// roomView construye la vista de un chat. viewer = pid del invitado ("" para el anfitrión).
func (a *App) roomView(id string, guest bool, viewer string) (RoomView, bool) {
	var v RoomView
	var outIDs []string
	online := a.hub.Online(id)
	ok := a.rooms.With(id, func(r *Room) bool {
		v = RoomView{ID: r.ID, Name: r.Name, Created: r.Created, Updated: r.Updated, Auto: r.Auto, Unread: r.Unread,
			Out: map[string]OutView{}, In: map[string]InView{}}
		for _, m := range r.Members {
			v.Members = append(v.Members, MemberView{PID: m.PID, Name: m.Name, Color: m.Color, Online: online[m.PID]})
		}
		v.Messages = make([]Message, len(r.Messages))
		for i, m := range r.Messages {
			v.Messages[i] = *m
			if m.Kind == "out" {
				outIDs = append(outIDs, m.Bundle)
			}
		}
		for _, b := range r.In {
			v.In[b.ID] = inView(b, guest, viewer)
		}
		return false
	})
	if !ok {
		return v, false
	}
	for _, oid := range outIDs {
		v.Out[oid] = a.outView(oid)
	}
	v.HostName = a.getSettings().HostName
	v.Typing = a.typingNames(id, guest, viewer)
	if v.Members == nil {
		v.Members = []MemberView{}
	}
	return v, true
}

func (a *App) typingNames(room string, guest bool, viewer string) []string {
	out := []string{}
	for _, who := range a.hub.Typing(room) {
		switch {
		case who == "host":
			if guest {
				out = append(out, a.getSettings().HostName)
			}
		case who == viewer:
		default:
			if n := a.rooms.MemberName(room, who); n != "" {
				out = append(out, n)
			}
		}
	}
	return out
}

// ---------- utilidades HTTP ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any, limit int64) error {
	return json.NewDecoder(io.LimitReader(r.Body, limit)).Decode(v)
}

// member identifica al invitado por el secreto que guarda su navegador.
func (p *PublicServer) member(r *http.Request, mid string) (*Member, error) {
	room := r.PathValue("room")
	if !p.app.rooms.Exists(room) {
		return nil, errors.New("chat no encontrado")
	}
	if mid == "" {
		mid = r.Header.Get("X-Member")
	}
	m := p.app.rooms.MemberByMid(room, mid)
	if m == nil {
		return nil, errors.New("no te has unido a este chat")
	}
	return m, nil
}

// limitador sencillo de mensajes por invitado
var rateMu sync.Mutex
var rateHits = map[string][]time.Time{}

func allow(key string, n int, per time.Duration) bool {
	rateMu.Lock()
	defer rateMu.Unlock()
	now := time.Now()
	hits := rateHits[key][:0]
	for _, t := range rateHits[key] {
		if now.Sub(t) < per {
			hits = append(hits, t)
		}
	}
	if len(hits) >= n {
		rateHits[key] = hits
		return false
	}
	rateHits[key] = append(hits, now)
	return true
}

// ---------- rutas del chat ----------

func (p *PublicServer) handleRoomPage(w http.ResponseWriter, r *http.Request) {
	if !p.app.rooms.Exists(r.PathValue("room")) {
		p.handleNotFound(w, r)
		return
	}
	p.serveWeb(w, "room.html", 200)
}

func (p *PublicServer) roomPayload(r *http.Request, room string, me *Member) map[string]any {
	viewer := ""
	if me != nil {
		viewer = me.PID
	}
	v, _ := p.app.roomView(room, true, viewer)
	_, via := clientInfo(r)
	set := p.app.getSettings()
	out := map[string]any{"room": v, "via": via, "conns": set.Connections, "chunk": 8 << 20, "inChunk": inChunk}
	if me != nil {
		out["me"] = MemberView{PID: me.PID, Name: me.Name, Color: me.Color, Online: true}
	}
	if via == "cloudflare" && set.ShowDirectOnTunnel {
		if base := p.app.directBase(); base != "" {
			out["direct"] = base + "/r/" + room
		}
	}
	return out
}

func (p *PublicServer) handleRoomInfo(w http.ResponseWriter, r *http.Request) {
	room := r.PathValue("room")
	if !p.app.rooms.Exists(room) {
		jsonErr(w, 404, "not_found")
		return
	}
	me := p.app.rooms.MemberByMid(room, r.URL.Query().Get("mid"))
	writeJSON(w, 200, p.roomPayload(r, room, me))
}

func (p *PublicServer) handleJoin(w http.ResponseWriter, r *http.Request) {
	room := r.PathValue("room")
	var req struct{ Mid, Name string }
	if err := readJSON(r, &req, 4096); err != nil {
		jsonErr(w, 400, "petición no válida")
		return
	}
	ip, _ := clientInfo(r)
	if !allow("join|"+ip, 20, time.Minute) {
		jsonErr(w, 429, "demasiados intentos, espera un momento")
		return
	}
	m, isNew, err := p.app.rooms.Join(room, req.Mid, req.Name)
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if isNew {
		p.app.notice(room, m.Name+" se ha unido al chat")
	} else {
		p.app.membersChanged(room)
	}
	writeJSON(w, 200, p.roomPayload(r, room, m))
}

// handlePoll es el canal en tiempo real (long-polling): devuelve los eventos
// posteriores a "since" en cuanto los hay, o una lista vacía a los 25 segundos.
func (p *PublicServer) handlePoll(w http.ResponseWriter, r *http.Request) {
	room := r.PathValue("room")
	m, err := p.member(r, r.URL.Query().Get("mid"))
	if err != nil {
		if !p.app.rooms.Exists(room) {
			jsonErr(w, 404, "not_found")
			return
		}
		jsonErr(w, 403, err.Error())
		return
	}
	since, perr := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	if perr != nil {
		since = -1
	}
	hub := p.app.hub
	hub.Enter(room, m.PID)
	defer hub.Leave(room, m.PID)

	hello := func() {
		_, seq, _, _ := hub.Since(room, m.PID, 1<<62)
		data, _ := json.Marshal(p.roomPayload(r, room, m))
		writeJSON(w, 200, map[string]any{"seq": seq, "events": []hubEvent{{Name: "hello", Data: data}}})
		go p.app.resumeUploadsFor(room, m.PID)
	}
	if since < 0 {
		hello()
		return
	}
	timeout := time.NewTimer(25 * time.Second)
	defer timeout.Stop()
	for {
		evs, seq, wait, reset := hub.Since(room, m.PID, since)
		if reset {
			hello()
			return
		}
		if len(evs) > 0 {
			writeJSON(w, 200, map[string]any{"seq": seq, "events": evs})
			return
		}
		select {
		case <-wait:
			// pequeño margen para agrupar ráfagas de eventos en una sola respuesta
			time.Sleep(30 * time.Millisecond)
		case <-timeout.C:
			writeJSON(w, 200, map[string]any{"seq": seq, "events": []hubEvent{}})
			return
		case <-r.Context().Done():
			return
		}
	}
}

func (p *PublicServer) handleMsg(w http.ResponseWriter, r *http.Request) {
	room := r.PathValue("room")
	var req struct{ Mid, Text string }
	if err := readJSON(r, &req, 64<<10); err != nil {
		jsonErr(w, 400, "petición no válida")
		return
	}
	m, err := p.member(r, req.Mid)
	if err != nil {
		jsonErr(w, 403, err.Error())
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		jsonErr(w, 400, "mensaje vacío")
		return
	}
	if len([]rune(text)) > 4000 {
		text = string([]rune(text)[:4000])
	}
	if !allow("msg|"+room+"|"+m.PID, 12, 10*time.Second) {
		jsonErr(w, 429, "vas muy rápido, espera un momento")
		return
	}
	p.app.hub.ClearTyping(room, m.PID)
	msg := newMessage(m.PID, "text")
	msg.Text = text
	cp := p.app.post(room, msg, true)
	writeJSON(w, 200, cp)
}

func (p *PublicServer) handleTyping(w http.ResponseWriter, r *http.Request) {
	room := r.PathValue("room")
	var req struct{ Mid string }
	readJSON(r, &req, 4096)
	m, err := p.member(r, req.Mid)
	if err != nil {
		jsonErr(w, 403, err.Error())
		return
	}
	p.app.hub.SetTyping(room, m.PID)
	p.app.typingChanged(room)
	w.WriteHeader(204)
}

func (p *PublicServer) handleOffer(w http.ResponseWriter, r *http.Request) {
	room := r.PathValue("room")
	var req struct {
		Mid   string
		Files []offerFile
	}
	if err := readJSON(r, &req, 32<<20); err != nil {
		jsonErr(w, 400, "petición no válida")
		return
	}
	m, err := p.member(r, req.Mid)
	if err != nil {
		jsonErr(w, 403, err.Error())
		return
	}
	if !allow("offer|"+room+"|"+m.PID, 10, time.Minute) {
		jsonErr(w, 429, "demasiados envíos seguidos, espera un momento")
		return
	}
	b, err := p.app.Offer(room, m, req.Files)
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"bundle": b.ID})
}

func (p *PublicServer) handleInStatus(w http.ResponseWriter, r *http.Request) {
	m, err := p.member(r, r.URL.Query().Get("mid"))
	if err != nil {
		jsonErr(w, 403, err.Error())
		return
	}
	st, err := p.app.missingChunks(r.PathValue("room"), r.PathValue("bid"), m.PID)
	if err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, st)
}

func (p *PublicServer) handleInCancel(w http.ResponseWriter, r *http.Request) {
	room, bid := r.PathValue("room"), r.PathValue("bid")
	m, err := p.member(r, r.URL.Query().Get("mid"))
	if err != nil {
		jsonErr(w, 403, err.Error())
		return
	}
	owner := false
	p.app.rooms.With(room, func(rm *Room) bool {
		if b := rm.inBundle(bid); b != nil && b.From == m.PID {
			owner = true
		}
		return false
	})
	if !owner {
		jsonErr(w, 403, "no es tuyo")
		return
	}
	p.app.CancelIn(room, bid)
	w.WriteHeader(204)
}

// countingReader cuenta lo que llega para mostrar el progreso en vivo.
type countingReader struct {
	r  io.Reader
	t  *Transfer
	tr *Tracker
}

func (c *countingReader) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	if n > 0 {
		c.t.Add(int64(n))
		c.tr.In(int64(n))
	}
	return n, err
}

var upBufPool = sync.Pool{New: func() any { b := make([]byte, inChunk); return &b }}

// handleUpload recibe un bloque de un archivo que nos envía un invitado.
func (p *PublicServer) handleUpload(w http.ResponseWriter, r *http.Request) {
	room, bid := r.PathValue("room"), r.PathValue("bid")
	m, err := p.member(r, r.URL.Query().Get("mid"))
	if err != nil {
		jsonErr(w, 403, err.Error())
		return
	}
	fi, err1 := strconv.Atoi(r.PathValue("i"))
	off, err2 := strconv.ParseInt(r.URL.Query().Get("off"), 10, 64)
	want, err3 := strconv.ParseUint(r.Header.Get("X-CRC32"), 16, 32)
	if err1 != nil || err2 != nil || err3 != nil || r.ContentLength <= 0 || r.ContentLength > inChunk {
		jsonErr(w, 400, "bloque no válido")
		return
	}
	var total, received int64
	var owner bool
	p.app.rooms.With(room, func(rm *Room) bool {
		if b := rm.inBundle(bid); b != nil {
			owner = b.From == m.PID
			total, received = b.Total, b.Received
		}
		return false
	})
	if !owner {
		jsonErr(w, 403, "envío no encontrado")
		return
	}
	ip, via := clientInfo(r)
	t := p.app.tracker.Get("in|"+bid, func() *Transfer {
		return &Transfer{ShareID: bid, Label: m.Name, Via: via, IP: ip, Total: total, Dir: "in", Who: m.Name, Room: room}
	})
	if t.bytes.Load() < received {
		p.app.tracker.SetBytes(t, received)
	}
	t.Begin()
	defer t.End()

	bp := upBufPool.Get().(*[]byte)
	defer upBufPool.Put(bp)
	buf := (*bp)[:r.ContentLength]
	n, err := io.ReadFull(&countingReader{r: r.Body, t: t, tr: p.app.tracker}, buf)
	if err != nil || int64(n) != r.ContentLength {
		t.Add(-int64(n))
		jsonErr(w, 400, "bloque incompleto")
		return
	}
	if crc32.ChecksumIEEE(buf) != uint32(want) {
		t.Add(-int64(n))
		jsonErr(w, 422, "CRC no coincide")
		return
	}
	if err := p.app.receiveChunk(room, bid, fi, off, buf); err != nil {
		jsonErr(w, 409, err.Error())
		return
	}
	w.WriteHeader(204)
}
