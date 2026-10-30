package main

// WebRTC: el vídeo del móvil llega directo por UDP, como en una videollamada, sin
// pasar por el túnel de Cloudflare. El navegador del móvil comprime con su
// codificador por hardware y ajusta solo la tasa de bits a lo que da la red (con
// las cuentas que le devuelve el PC); los paquetes perdidos se vuelven a pedir.
//
// La negociación va por la conexión de siempre (WebSocket o HTTP), así que vale
// cualquier enlace. Si la conexión directa no sale (hay redes que no la dejan), el
// vídeo sigue por la conexión de siempre, también en H.264: no se nota nada.
//
// Todo el UDP entra por un solo puerto, el mismo número que el de la página: así el
// modo directo lo abre en el router con UPnP junto al TCP.

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/interceptor"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media/samplebuilder"
)

// rtcTestLoss (TURBOCAM_RTCLOSS=1, solo para pruebas): se tira la mitad de lo que
// llega por WebRTC, como unos datos móviles malos.
var rtcTestLoss = os.Getenv("TURBOCAM_RTCLOSS") != ""

// stunServers: para que cada lado sepa su dirección pública (y abrir paso en el NAT).
var stunServers = []string{"stun:stun.l.google.com:19302", "stun:stun.cloudflare.com:3478"}

type rtcHub struct {
	api  *webrtc.API
	conn net.PacketConn
	mux  *ice.UniversalUDPMuxDefault
	port int
}

func newRTCHub(port int) (*rtcHub, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: port})
	if err != nil {
		if conn, err = net.ListenUDP("udp4", &net.UDPAddr{}); err != nil {
			return nil, err
		}
	}
	quiet := &logging.DefaultLoggerFactory{Writer: io.Discard, DefaultLogLevel: logging.LogLevelDisabled, ScopeLevels: map[string]logging.LogLevel{}}
	mux := ice.NewUniversalUDPMuxDefault(ice.UniversalUDPMuxParams{UDPConn: conn, Logger: quiet.NewLogger("ice")})

	m := &webrtc.MediaEngine{}
	fb := []webrtc.RTCPFeedback{{Type: "goog-remb"}, {Type: "ccm", Parameter: "fir"}, {Type: "nack"}, {Type: "nack", Parameter: "pli"}, {Type: "transport-cc"}}
	// Solo H.264 (lo que decodifica Windows), en los perfiles que suelen dar los móviles.
	for i, pl := range []string{"42e01f", "42001f", "4d001f", "640c1f"} {
		pt := webrtc.PayloadType(102 + 2*i)
		if err := m.RegisterCodec(webrtc.RTPCodecParameters{RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType: webrtc.MimeTypeH264, ClockRate: 90000, RTCPFeedback: fb,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=" + pl,
		}, PayloadType: pt}, webrtc.RTPCodecTypeVideo); err != nil {
			conn.Close()
			return nil, err
		}
		if err := m.RegisterCodec(webrtc.RTPCodecParameters{RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType: webrtc.MimeTypeRTX, ClockRate: 90000, SDPFmtpLine: "apt=" + strconv.Itoa(int(pt)),
		}, PayloadType: pt + 1}, webrtc.RTPCodecTypeVideo); err != nil {
			conn.Close()
			return nil, err
		}
	}
	ir := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(m, ir); err != nil {
		conn.Close()
		return nil, err
	}
	se := webrtc.SettingEngine{LoggerFactory: quiet}
	se.SetICEUDPMux(mux)
	se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeQueryOnly) // el móvil esconde su IP local tras un nombre .local
	se.SetICETimeouts(4*time.Second, 8*time.Second, time.Second)
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(ir), webrtc.WithSettingEngine(se))
	return &rtcHub{api: api, conn: conn, mux: mux, port: conn.LocalAddr().(*net.UDPAddr).Port}, nil
}

func (h *rtcHub) Close() {
	h.mux.Close()
	h.conn.Close()
}

// rtcPeer es la conexión WebRTC de un móvil.
type rtcPeer struct {
	pc     *webrtc.PeerConnection
	ssrc   atomic.Uint32
	lastRx atomic.Int64 // último paquete de vídeo (UnixNano)
	once   sync.Once
}

func (r *rtcPeer) close() { r.once.Do(func() { r.pc.Close() }) }

// pli pide un fotograma clave al codificador del móvil.
func (r *rtcPeer) pli() {
	if s := r.ssrc.Load(); s != 0 {
		r.pc.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: s}})
	}
}

// rtcSignal: mensajes de negociación del móvil ({t:"rtc", sdp} y {t:"ice", c}).
type rtcSignal struct {
	T   string                   `json:"t"`
	SDP string                   `json:"sdp"`
	C   *webrtc.ICECandidateInit `json:"c"`
}

func (e *Engine) onRTCSignal(p *phone, data []byte) {
	var m rtcSignal
	if json.Unmarshal(data, &m) != nil {
		return
	}
	switch m.T {
	case "rtc":
		e.rtcOffer(p, m.SDP)
	case "ice":
		p.mu.Lock()
		peer := p.rtc
		p.mu.Unlock()
		if peer != nil && m.C != nil && m.C.Candidate != "" {
			err := peer.pc.AddICECandidate(*m.C)
			dumpLog(p.id, fmt.Sprintf("rtc: candidato del móvil %s (err=%v)", m.C.Candidate, err))
		}
	case "rtcstop":
		e.rtcDrop(p, nil, false)
	}
}

func (e *Engine) rtcOffer(p *phone, sdp string) {
	if e.rtc == nil || len(sdp) > 64<<10 {
		p.t.send(map[string]any{"t": "rtcoff"})
		return
	}
	e.rtcDrop(p, nil, false)
	var ice []webrtc.ICEServer
	for _, u := range stunServers {
		ice = append(ice, webrtc.ICEServer{URLs: []string{u}})
	}
	pc, err := e.rtc.api.NewPeerConnection(webrtc.Configuration{ICEServers: ice, BundlePolicy: webrtc.BundlePolicyMaxBundle})
	if err != nil {
		p.t.send(map[string]any{"t": "rtcoff"})
		return
	}
	peer := &rtcPeer{pc: pc}
	p.mu.Lock()
	p.rtc = peer
	p.mu.Unlock()
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			init := c.ToJSON()
			dumpLog(p.id, "rtc: candidato del PC "+init.Candidate)
			p.t.send(map[string]any{"t": "ice", "c": init})
		}
	})
	pc.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		msg := "rtc: ICE " + s.String()
		if s == webrtc.ICEConnectionStateConnected {
			for _, tr := range pc.GetTransceivers() {
				if r := tr.Receiver(); r != nil && r.Transport() != nil {
					if pair, err := r.Transport().ICETransport().GetSelectedCandidatePair(); err == nil && pair != nil {
						msg += " por " + pair.String()
					}
					break
				}
			}
		}
		dumpLog(p.id, msg)
	})
	pc.OnTrack(func(tr *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if tr.Kind() != webrtc.RTPCodecTypeVideo {
			return
		}
		peer.ssrc.Store(uint32(tr.SSRC()))
		go e.readRTC(p, peer, tr)
	})
	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		if s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateClosed {
			e.rtcDrop(p, peer, true)
		}
	})
	fail := func() {
		e.rtcDrop(p, peer, true)
	}
	if _, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
		fail()
		return
	}
	for _, l := range strings.Split(sdp, "\n") {
		if strings.HasPrefix(l, "a=candidate:") {
			dumpLog(p.id, "rtc: candidato del móvil (oferta) "+strings.TrimSpace(l))
		}
	}
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: sdp}); err != nil {
		dumpLog(p.id, fmt.Sprintf("rtc: oferta no válida: %v", err))
		fail()
		return
	}
	ans, err := pc.CreateAnswer(nil)
	if err != nil {
		fail()
		return
	}
	if err := pc.SetLocalDescription(ans); err != nil {
		fail()
		return
	}
	p.mu.Lock()
	start := p.bitrate
	p.mu.Unlock()
	p.t.send(map[string]any{"t": "rtc", "sdp": withStartBitrate(ans.SDP, start)})
	// La IP pública con el puerto que el router tiene abierto por UPnP. pion solo
	// anuncia la que le da STUN, que sale por otro puerto (al azar) que el router no
	// reenvía: desde los datos del móvil no se llegaría.
	if e.rtcPublic != nil {
		if ip, port := e.rtcPublic(); ip != "" {
			c := publicCandidate(ip, port, e.rtc.port)
			mid := sdpMid(ans.SDP)
			idx := uint16(0)
			dumpLog(p.id, "rtc: candidato del PC (UPnP) "+c)
			p.t.send(map[string]any{"t": "ice", "c": webrtc.ICECandidateInit{Candidate: c, SDPMid: &mid, SDPMLineIndex: &idx}})
		}
	}
}

// publicCandidate escribe un candidato ICE «srflx» para la IP pública y el puerto
// externo que el router reenvía al puerto local de WebRTC.
func publicCandidate(ip string, ext, local int) string {
	return fmt.Sprintf("candidate:4207563210 1 udp 1694498815 %s %d typ srflx raddr 0.0.0.0 rport %d", ip, ext, local)
}

// sdpMid: el identificador de la primera pista (a=mid:) de una descripción SDP.
func sdpMid(sdp string) string {
	for _, l := range strings.Split(sdp, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(l), "a=mid:"); ok {
			return v
		}
	}
	return "0"
}

// withStartBitrate le dice a Chrome a qué tasa empezar (en kb/s): la que ya se había
// medido por la conexión de siempre. Si no, empieza muy abajo (300 kb/s, 320x180) y
// tarda unos segundos en subir.
func withStartBitrate(sdp string, kbps int) string {
	if kbps < 600 {
		kbps = 600
	}
	if kbps > 6000 {
		kbps = 6000
	}
	h264 := map[string]bool{}
	lines := strings.Split(sdp, "\r\n")
	for _, l := range lines {
		// a=rtpmap:102 H264/90000
		if rest, ok := strings.CutPrefix(l, "a=rtpmap:"); ok {
			if pt, codec, ok := strings.Cut(rest, " "); ok && strings.HasPrefix(strings.ToUpper(codec), "H264/") {
				h264[pt] = true
			}
		}
	}
	for i, l := range lines {
		if rest, ok := strings.CutPrefix(l, "a=fmtp:"); ok {
			if pt, _, ok := strings.Cut(rest, " "); ok && h264[pt] && !strings.Contains(l, "x-google-start-bitrate") {
				lines[i] = l + ";x-google-start-bitrate=" + strconv.Itoa(kbps)
			}
		}
	}
	return strings.Join(lines, "\r\n")
}

// rtcDrop cierra la conexión WebRTC de un móvil (solo si sigue siendo peer, o la que
// haya con peer nil). tell: avisar al móvil para que vuelva a la conexión de siempre.
func (e *Engine) rtcDrop(p *phone, peer *rtcPeer, tell bool) {
	p.mu.Lock()
	cur := p.rtc
	if cur == nil || (peer != nil && cur != peer) {
		p.mu.Unlock()
		return
	}
	p.rtc = nil
	p.mu.Unlock()
	cur.close()
	p.rtcLive.Store(false)
	if tell {
		p.t.send(map[string]any{"t": "rtcoff"})
	}
	e.changed()
}

// readRTC junta los paquetes RTP en fotogramas H.264 y se los da al decodificador.
func (e *Engine) readRTC(p *phone, peer *rtcPeer, tr *webrtc.TrackRemote) {
	// Si falta un paquete se espera un poco a que llegue el reenvío; si no llega, el
	// fotograma se pierde y se pide un clave.
	sb := samplebuilder.New(512, &codecs.H264Packet{}, 90000, samplebuilder.WithMaxTimeDelay(250*time.Millisecond))
	var n uint32
	for {
		pkt, _, err := tr.ReadRTP()
		if err != nil {
			return
		}
		peer.lastRx.Store(time.Now().UnixNano())
		sb.Push(pkt)
		for s := sb.Pop(); s != nil; s = sb.Pop() {
			n++
			if rtcTestLoss && n%2 == 0 {
				continue // pruebas: un WebRTC que pierde la mitad
			}
			e.queueFrame(p, videoChunk{kind: frameKindH264, seq: n, key: h264IsKey(s.Data), gap: s.PrevDroppedPackets > 0, rtc: true, data: s.Data})
		}
	}
}

// rtcWatch: si por WebRTC deja de llegar vídeo (el móvil ha cambiado de red, por
// ejemplo), se vuelve a la conexión de siempre. Lo llama el mantenimiento cada segundo.
//
// También compara: si tras unos segundos por WebRTC llegan bastantes menos
// fotogramas que por la conexión de siempre (pasa con datos móviles que pierden
// paquetes), se vuelve a la de siempre y el móvil no lo reintenta en un buen rato.
func (e *Engine) rtcWatch() {
	s := getSettings()
	e.mu.Lock()
	phones := e.phoneList()
	e.mu.Unlock()
	for _, p := range phones {
		p.mu.Lock()
		peer, visible, live := p.rtc, p.visible, p.live
		fps := p.fps
		if time.Since(p.frameAt) > 2*time.Second {
			fps = 0
		}
		p.mu.Unlock()
		if !visible || !live || s.FPS < 15 {
			continue
		}
		avg := func(a *float64, v float64) {
			if *a == 0 {
				*a = v
			} else {
				*a = *a*0.75 + v*0.25
			}
		}
		if !p.rtcLive.Load() {
			p.mu.Lock()
			if fps > 0 {
				avg(&p.wsAvg, fps)
			}
			p.rtcSecs, p.rtcAvg = 0, 0
			p.mu.Unlock()
			continue
		}
		if peer == nil {
			continue
		}
		if time.Since(time.Unix(0, peer.lastRx.Load())) > 3*time.Second {
			e.rtcDrop(p, peer, true)
			continue
		}
		p.mu.Lock()
		p.rtcSecs++
		if p.rtcSecs > 2 { // los primeros, mientras arranca
			avg(&p.rtcAvg, fps)
		}
		worse := p.rtcSecs >= 8 && p.wsAvg >= 10 && p.rtcAvg < p.wsAvg*0.75
		ws, rtc := p.wsAvg, p.rtcAvg
		p.mu.Unlock()
		if worse {
			dumpLog(p.id, fmt.Sprintf("rtc: va peor (%.0f fps) que la conexión de siempre (%.0f fps): se deja", rtc, ws))
			p.t.send(map[string]any{"t": "rtcworse"})
			e.rtcDrop(p, peer, true)
		}
	}
}

// h264IsKey dice si un fotograma (Annex B) lleva un IDR, desde el que se puede
// empezar a decodificar.
func h264IsKey(b []byte) bool {
	for i := 0; i+3 < len(b); i++ {
		if b[i] == 0 && b[i+1] == 0 && b[i+2] == 1 {
			if b[i+3]&0x1f == 5 {
				return true
			}
			i += 2
		}
	}
	return false
}
