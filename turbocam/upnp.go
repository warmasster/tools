package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Cliente UPnP IGD mínimo (SSDP + SOAP) para abrir un puerto en el router.

type igd struct {
	controlURL  string
	serviceType string
	localIP     string
}

type upnpService struct {
	ServiceType string `xml:"serviceType"`
	ControlURL  string `xml:"controlURL"`
}

type upnpDevice struct {
	DeviceType string        `xml:"deviceType"`
	Services   []upnpService `xml:"serviceList>service"`
	Devices    []upnpDevice  `xml:"deviceList>device"`
}

type upnpRoot struct {
	URLBase string     `xml:"URLBase"`
	Device  upnpDevice `xml:"device"`
}

var httpShort = &http.Client{Timeout: 4 * time.Second}

// http4 fuerza IPv4: la IP pública que nos interesa para el enlace directo es la IPv4.
var http4 = &http.Client{Timeout: 4 * time.Second, Transport: &http.Transport{
	DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 4 * time.Second}).DialContext(ctx, "tcp4", addr)
	},
}}

// privateIPv4s devuelve las IPv4 privadas de interfaces activas.
func privateIPv4s() []net.IP {
	var out []net.IP
	ifs, _ := net.Interfaces()
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipn.IP.To4()
			if ip != nil && ip.IsPrivate() {
				out = append(out, ip)
			}
		}
	}
	return out
}

// discoverIGD busca routers con UPnP por todas las interfaces locales.
func discoverIGD(ctx context.Context) (*igd, error) {
	targets := []string{
		"urn:schemas-upnp-org:device:InternetGatewayDevice:1",
		"urn:schemas-upnp-org:device:InternetGatewayDevice:2",
		"urn:schemas-upnp-org:service:WANIPConnection:1",
		"urn:schemas-upnp-org:service:WANPPPConnection:1",
	}
	locations := map[string]bool{}
	var order []string
	locRe := regexp.MustCompile(`(?im)^location:\s*(\S+)`)

	ips := privateIPv4s()
	if len(ips) == 0 {
		ips = []net.IP{net.IPv4zero}
	}
	dst := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900}
	type res struct{ loc string }
	ch := make(chan res, 64)
	deadline := time.Now().Add(2500 * time.Millisecond)
	var conns []*net.UDPConn
	for _, ip := range ips {
		c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: ip})
		if err != nil {
			continue
		}
		conns = append(conns, c)
		for _, st := range targets {
			msg := "M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nST: " + st + "\r\nMAN: \"ssdp:discover\"\r\nMX: 2\r\n\r\n"
			c.WriteToUDP([]byte(msg), dst)
		}
		c.SetReadDeadline(deadline)
		go func(c *net.UDPConn) {
			buf := make([]byte, 4096)
			for {
				n, _, err := c.ReadFromUDP(buf)
				if err != nil {
					return
				}
				if m := locRe.FindSubmatch(buf[:n]); m != nil {
					ch <- res{string(m[1])}
				}
			}
		}(c)
	}
	if len(conns) == 0 {
		return nil, errors.New("no se pudo abrir socket SSDP")
	}
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
loop:
	for {
		select {
		case r := <-ch:
			if !locations[r.loc] {
				locations[r.loc] = true
				order = append(order, r.loc)
			}
		case <-timer.C:
			break loop
		case <-ctx.Done():
			break loop
		}
	}
	for _, c := range conns {
		c.Close()
	}
	for _, loc := range order {
		if g, err := fetchIGD(loc); err == nil {
			return g, nil
		}
	}
	return nil, errors.New("no se encontró ningún router con UPnP activado")
}

func fetchIGD(location string) (*igd, error) {
	resp, err := httpShort.Get(location)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var root upnpRoot
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&root); err != nil {
		return nil, err
	}
	base, err := url.Parse(location)
	if err != nil {
		return nil, err
	}
	if root.URLBase != "" {
		if b, err := url.Parse(root.URLBase); err == nil {
			base = b
		}
	}
	var found *upnpService
	var walk func(d upnpDevice)
	walk = func(d upnpDevice) {
		for i := range d.Services {
			s := &d.Services[i]
			if found == nil && (strings.Contains(s.ServiceType, "WANIPConnection") || strings.Contains(s.ServiceType, "WANPPPConnection")) {
				found = s
			}
		}
		for _, c := range d.Devices {
			walk(c)
		}
	}
	walk(root.Device)
	if found == nil {
		return nil, errors.New("el dispositivo no es un router IGD")
	}
	ctl, err := base.Parse(strings.TrimSpace(found.ControlURL))
	if err != nil {
		return nil, err
	}
	// IP local con la que llegamos al router
	host := ctl.Host
	if !strings.Contains(host, ":") {
		host += ":80"
	}
	c, err := net.DialTimeout("udp4", host, 2*time.Second)
	if err != nil {
		return nil, err
	}
	local := c.LocalAddr().(*net.UDPAddr).IP.String()
	c.Close()
	return &igd{controlURL: ctl.String(), serviceType: strings.TrimSpace(found.ServiceType), localIP: local}, nil
}

type soapError struct {
	Code int
	Desc string
}

func (e *soapError) Error() string { return fmt.Sprintf("UPnP %d %s", e.Code, e.Desc) }

var tagRe = regexp.MustCompile(`<(?:\w+:)?(\w+)>([^<]*)</`)

func (g *igd) call(action string, args [][2]string) (map[string]string, error) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>`)
	fmt.Fprintf(&b, `<u:%s xmlns:u="%s">`, action, g.serviceType)
	for _, a := range args {
		fmt.Fprintf(&b, "<%s>", a[0])
		xml.EscapeText(&b, []byte(a[1]))
		fmt.Fprintf(&b, "</%s>", a[0])
	}
	fmt.Fprintf(&b, `</u:%s></s:Body></s:Envelope>`, action)
	req, _ := http.NewRequest("POST", g.controlURL, bytes.NewBufferString(b.String()))
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction", fmt.Sprintf(`"%s#%s"`, g.serviceType, action))
	resp, err := httpShort.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	out := map[string]string{}
	for _, m := range tagRe.FindAllSubmatch(body, -1) {
		out[string(m[1])] = string(m[2])
	}
	if resp.StatusCode != 200 {
		code := 0
		fmt.Sscan(out["errorCode"], &code)
		return out, &soapError{Code: code, Desc: out["errorDescription"]}
	}
	return out, nil
}

func (g *igd) ExternalIP() (string, error) {
	r, err := g.call("GetExternalIPAddress", nil)
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(r["NewExternalIPAddress"])
	if net.ParseIP(ip) == nil {
		return "", errors.New("el router no devolvió su IP externa")
	}
	return ip, nil
}

func (g *igd) addMapping(ext, internal int, lease int) error {
	_, err := g.call("AddPortMapping", [][2]string{
		{"NewRemoteHost", ""},
		{"NewExternalPort", fmt.Sprint(ext)},
		{"NewProtocol", "TCP"},
		{"NewInternalPort", fmt.Sprint(internal)},
		{"NewInternalClient", g.localIP},
		{"NewEnabled", "1"},
		{"NewPortMappingDescription", "TurboCAM"},
		{"NewLeaseDuration", fmt.Sprint(lease)},
	})
	return err
}

// mappingOwner devuelve el cliente interno de un mapeo existente, si lo hay.
func (g *igd) mappingOwner(ext int) (string, int) {
	r, err := g.call("GetSpecificPortMappingEntry", [][2]string{
		{"NewRemoteHost", ""},
		{"NewExternalPort", fmt.Sprint(ext)},
		{"NewProtocol", "TCP"},
	})
	if err != nil {
		return "", 0
	}
	port := 0
	fmt.Sscan(r["NewInternalPort"], &port)
	return r["NewInternalClient"], port
}

// Map abre internal en el router, probando puertos externos alternativos si hay conflicto.
// Devuelve el puerto externo y la duración de la concesión usada (0 = permanente).
func (g *igd) Map(internal int) (int, int, error) {
	var last error
	for i := 0; i < 8; i++ {
		ext := internal + i
		conflict := false
		for _, lease := range []int{7200, 0} {
			err := g.addMapping(ext, internal, lease)
			if err == nil {
				return ext, lease, nil
			}
			last = err
			var se *soapError
			if errors.As(err, &se) && se.Code == 718 { // puerto ocupado
				if cl, p := g.mappingOwner(ext); cl == g.localIP && p == internal {
					return ext, 0, nil
				}
				conflict = true
				break
			}
			if !errors.As(err, &se) { // error de red: no tiene sentido seguir
				return 0, 0, err
			}
		}
		if !conflict {
			break
		}
	}
	return 0, 0, last
}

func (g *igd) Unmap(ext int) {
	g.call("DeletePortMapping", [][2]string{
		{"NewRemoteHost", ""},
		{"NewExternalPort", fmt.Sprint(ext)},
		{"NewProtocol", "TCP"},
	})
}

var cgnatNet = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

func isCGNATorPrivate(s string) bool {
	ip := net.ParseIP(s)
	if ip == nil {
		return true
	}
	return ip.IsPrivate() || cgnatNet.Contains(ip) || ip.IsLoopback() || ip.IsUnspecified()
}

// publicIP pregunta a Cloudflare cuál es nuestra IPv4 pública.
func publicIP() (string, error) {
	for _, u := range []string{"https://cloudflare.com/cdn-cgi/trace", "https://1.1.1.1/cdn-cgi/trace"} {
		resp, err := http4.Get(u)
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		resp.Body.Close()
		for _, line := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(line, "ip="); ok {
				v = strings.TrimSpace(v)
				if ip := net.ParseIP(v); ip != nil && ip.To4() != nil {
					return v, nil
				}
			}
		}
	}
	resp, err := http4.Get("https://api.ipify.org")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
	v := strings.TrimSpace(string(b))
	if net.ParseIP(v) == nil {
		return "", errors.New("no se pudo obtener la IP pública")
	}
	return v, nil
}

// lanIP devuelve la IP local de la interfaz con ruta por defecto.
func lanIP() string {
	c, err := net.Dial("udp4", "1.1.1.1:53")
	if err == nil {
		defer c.Close()
		ip := c.LocalAddr().(*net.UDPAddr).IP
		if ip.IsPrivate() {
			return ip.String()
		}
	}
	if ips := privateIPv4s(); len(ips) > 0 {
		return ips[0].String()
	}
	return ""
}
