// Package events lee los registros de Windows (Visor de eventos) y saca de
// ellos los cierres de juegos, cuelgues de la gráfica, pantallazos, reinicios
// de golpe y errores de hardware, explicados en lenguaje normal.
package events

import (
	"encoding/xml"
	"fmt"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modWevt  = windows.NewLazySystemDLL("wevtapi.dll")
	pQuery   = modWevt.NewProc("EvtQuery")
	pNext    = modWevt.NewProc("EvtNext")
	pRender  = modWevt.NewProc("EvtRender")
	pEvClose = modWevt.NewProc("EvtClose")
)

// Event es un evento del registro, ya leído.
type Event struct {
	Provider string
	ID       int
	Time     time.Time
	Data     []Data
}

// Data es un campo del evento (con nombre o solo por posición).
type Data struct {
	Name  string `xml:"Name,attr"`
	Value string `xml:",chardata"`
}

// Get devuelve un campo por nombre (o por posición si no tienen nombre).
func (e Event) Get(name string, pos int) string {
	for _, d := range e.Data {
		if strings.EqualFold(d.Name, name) {
			return strings.TrimSpace(d.Value)
		}
	}
	if pos >= 0 && pos < len(e.Data) && e.Data[pos].Name == "" {
		return strings.TrimSpace(e.Data[pos].Value)
	}
	return ""
}

type xmlEvent struct {
	System struct {
		Provider struct {
			Name string `xml:"Name,attr"`
		} `xml:"Provider"`
		EventID     int `xml:"EventID"`
		TimeCreated struct {
			SystemTime string `xml:"SystemTime,attr"`
		} `xml:"TimeCreated"`
	} `xml:"System"`
	EventData struct {
		Data []Data `xml:"Data"`
	} `xml:"EventData"`
	UserData struct {
		Inner []byte `xml:",innerxml"`
	} `xml:"UserData"`
}

// Query lee los eventos de un registro («System», «Application») que cumplan
// la consulta XPath, de los más nuevos a los más viejos.
func Query(channel, xpath string, max int) ([]Event, error) {
	if modWevt.Load() != nil {
		return nil, syscall.ERROR_PROC_NOT_FOUND
	}
	ch, _ := windows.UTF16PtrFromString(channel)
	q, _ := windows.UTF16PtrFromString(xpath)
	h, _, e := pQuery.Call(0, uintptr(unsafe.Pointer(ch)), uintptr(unsafe.Pointer(q)), 0x1|0x200) // EvtQueryChannelPath | EvtQueryReverseDirection
	if h == 0 {
		return nil, e
	}
	defer pEvClose.Call(h)
	var out []Event
	handles := make([]uintptr, 32)
	buf := make([]uint16, 16<<10)
	for len(out) < max {
		var got uint32
		r, _, _ := pNext.Call(h, uintptr(len(handles)), uintptr(unsafe.Pointer(&handles[0])), 1000, 0, uintptr(unsafe.Pointer(&got)))
		if r == 0 || got == 0 {
			break
		}
		for _, eh := range handles[:got] {
			var used, props uint32
			r, _, _ := pRender.Call(0, eh, 1, uintptr(len(buf)*2), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&used)), uintptr(unsafe.Pointer(&props))) // EvtRenderEventXml
			if r == 0 && used > uint32(len(buf)*2) {
				buf = make([]uint16, used/2+16)
				r, _, _ = pRender.Call(0, eh, 1, uintptr(len(buf)*2), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&used)), uintptr(unsafe.Pointer(&props)))
			}
			pEvClose.Call(eh)
			if r == 0 {
				continue
			}
			if ev, ok := parseXML(windows.UTF16ToString(buf)); ok {
				out = append(out, ev)
			}
		}
	}
	return out, nil
}

func parseXML(s string) (Event, bool) {
	var x xmlEvent
	if xml.Unmarshal([]byte(s), &x) != nil {
		return Event{}, false
	}
	t, _ := time.Parse(time.RFC3339Nano, x.System.TimeCreated.SystemTime)
	ev := Event{Provider: x.System.Provider.Name, ID: x.System.EventID, Time: t.Local(), Data: x.EventData.Data}
	if len(ev.Data) == 0 && len(x.UserData.Inner) > 0 {
		// Algunos eventos (WHEA) llevan los datos en UserData con nombres propios.
		ev.Data = flatten(x.UserData.Inner)
	}
	return ev, true
}

// flatten saca los elementos hoja de un trozo de XML como pares nombre-valor.
func flatten(b []byte) []Data {
	d := xml.NewDecoder(strings.NewReader(string(b)))
	var out []Data
	var stack []string
	var text strings.Builder
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
			text.Reset()
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			if len(stack) > 0 {
				if v := strings.TrimSpace(text.String()); v != "" {
					out = append(out, Data{Name: stack[len(stack)-1], Value: v})
				}
				stack = stack[:len(stack)-1]
			}
			text.Reset()
		}
	}
	return out
}

// Since devuelve el trozo de XPath «en los últimos días».
func Since(days int) string {
	return fmt.Sprintf("TimeCreated[timediff(@SystemTime) <= %d]", int64(days)*86400000)
}
