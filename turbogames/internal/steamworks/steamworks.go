// Package steamworks habla con el Steam abierto a través de steamclient64.dll, la
// biblioteca que viene con todo Steam (la misma que usan los juegos por dentro).
// No hace falta ninguna DLL de Valve aparte: se usa la del Steam de cada usuario.
//
// Solo se usa desde un proceso aparte (TurboGAMES.exe --workshop <appid>): Steam
// cree que ese proceso es el juego y así deja suscribirse a sus mods, igual que lo
// hace el propio juego desde su menú. Mientras vive, los amigos ven «jugando a…».
//
// Las interfaces de Steam son clases de C++: se llama a sus métodos por su
// posición en la tabla de funciones (vtable). Las posiciones salen de las
// cabeceras del SDK de Steamworks (isteamclient.h, isteamutils.h, isteamapps.h e
// isteamugc.h) para las versiones que se piden aquí, contando también los
// métodos privados (STEAM_PRIVATE_API), que no salen en steam_api.json pero
// ocupan su sitio. Steam mantiene las versiones antiguas para que los juegos
// viejos sigan funcionando, así que no cambian.
package steamworks

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Versiones de las interfaces que se piden. Las posiciones de abajo son las de
// estas versiones.
const (
	verUGC   = "STEAMUGC_INTERFACE_VERSION020"
	verUtils = "SteamUtils010"
	verApps  = "STEAMAPPS_INTERFACE_VERSION008"
)

var clientVersions = []string{"SteamClient021", "SteamClient020", "SteamClient019", "SteamClient018", "SteamClient017"}

// Posiciones en las vtables.
const (
	// ISteamClient
	cCreateSteamPipe     = 0
	cBReleaseSteamPipe   = 1
	cConnectToGlobalUser = 2
	cReleaseUser         = 4
	cGetISteamUtils      = 9
	cGetISteamApps       = 15
	cGetISteamUGC        = 26 // después de RunFrame (privado, posición 20)

	// ISteamUtils (GetCSERIPPort, privado, ocupa la posición 4)
	uGetAppID                = 9
	uIsAPICallCompleted      = 11
	uGetAPICallFailureReason = 12
	uGetAPICallResult        = 13

	// ISteamApps
	aBIsSubscribedApp = 6

	// ISteamUGC. CreateQueryAllUGCRequest tiene dos versiones (por página y por
	// cursor): en C++ de Microsoft las funciones virtuales con el mismo nombre van
	// juntas y en orden inverso al del archivo, así que la del cursor (declarada
	// después) queda en la posición 1.
	gCreateQueryAllUGCRequestCursor = 1
	gCreateQueryUGCDetailsRequest   = 3
	gSendQueryUGCRequest            = 4
	gGetQueryUGCResult              = 5
	gGetQueryUGCPreviewURL          = 9
	gGetQueryUGCChildren            = 11
	gGetQueryUGCStatistic           = 12
	gGetQueryUGCNumPreviews         = 13 // GetQueryUGCNumAdditionalPreviews
	gGetQueryUGCPreview             = 14 // GetQueryUGCAdditionalPreview
	gAddRequiredTag                 = 22
	gAddExcludedTag                 = 24
	gSetReturnAdditionalPreviews    = 30
	gReleaseQueryUGCRequest         = 21
	gSetReturnLongDescription       = 27
	gSetReturnChildren              = 29
	gSetAllowCachedResponse         = 34
	gSetSearchText                  = 38
	gSetRankedByTrendDays           = 39
	gSubscribeItem                  = 72
	gUnsubscribeItem                = 73
	gGetNumSubscribedItems          = 74
	gGetSubscribedItems             = 75
	gGetItemState                   = 76
	gGetItemInstallInfo             = 77
	gGetItemDownloadInfo            = 78
)

// Números de las respuestas (callbacks) que se esperan.
const (
	cbAPICallCompleted = 703
	cbSubscribe        = 1313
	cbUnsubscribe      = 1315
	cbQueryCompleted   = 3401
)

// Tamaños y posiciones de las estructuras (empaquetado de 8 bytes, el de Windows).
const (
	sizeSubscribeResult = 16
	sizeQueryCompleted  = 280
	sizeDetails         = 9784
	sizeCallbackMsg     = 24
)

// Estados de un mod (EItemState).
const (
	StateSubscribed      = 1
	StateInstalled       = 4
	StateNeedsUpdate     = 8
	StateDownloading     = 16
	StateDownloadPending = 32
)

// Tipos de consulta del Workshop (EUGCQuery).
const (
	QueryByVote         = 0
	QueryByDate         = 1
	QueryByTrend        = 3
	QueryByText         = 11
	QueryBySubscribers  = 12
	QueryByLastUpdated  = 19
	matchingItemsReady  = 2 // k_EUGCMatchingUGCType_Items_ReadyToUse
	fileTypeCollection  = 2 // k_EWorkshopFileTypeCollection
	resultOK            = 1
	resultFileNotFound  = 9
	resultAccessDenied  = 15
	resultNotLoggedOn   = 21
	resultLimitExceeded = 25
)

var (
	// ErrNoSteam: Steam no está abierto (o no se ha podido hablar con él).
	ErrNoSteam = errors.New("Steam no está abierto")
	// ErrNoUser: Steam está abierto pero sin ninguna cuenta dentro.
	ErrNoUser = errors.New("Steam no tiene ninguna cuenta dentro")
	// ErrNotOwned: la cuenta abierta en Steam no tiene el juego.
	ErrNotOwned = errors.New("La cuenta abierta en Steam no tiene este juego")
	// ErrUnsupported: este Steam no ofrece las interfaces que hacen falta.
	ErrUnsupported = errors.New("Este Steam no deja gestionar los mods desde fuera")
)

var (
	kernel32    = windows.NewLazySystemDLL("kernel32.dll")
	procMoveMem = kernel32.NewProc("RtlMoveMemory")
)

// call llama a una función de C por su dirección. Los punteros de Go que se le
// pasen (convertidos a uintptr en la propia llamada) siguen vivos y en su sitio
// hasta que vuelve.
//
//go:uintptrescapes
func call(fn uintptr, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(fn, args...)
	return r
}

// readMem copia n bytes de memoria de Steam (no de Go) a dst.
//
//go:uintptrescapes
func readMem(dst uintptr, src uintptr, n int) {
	if src == 0 || n <= 0 {
		return
	}
	procMoveMem.Call(dst, src, uintptr(n))
}

func readPtr(addr uintptr) uintptr {
	var v uintptr
	readMem(uintptr(unsafe.Pointer(&v)), addr, int(unsafe.Sizeof(v)))
	return v
}

// vcall llama al método idx de un objeto de C++ (this va primero).
//
//go:uintptrescapes
func vcall(obj uintptr, idx int, args ...uintptr) uintptr {
	vt := readPtr(obj)
	fn := readPtr(vt + uintptr(idx)*unsafe.Sizeof(uintptr(0)))
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, obj)
	all = append(all, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

func vbool(obj uintptr, idx int, args ...uintptr) bool { return vcall(obj, idx, args...)&0xff != 0 }

func cstr(s string) *byte {
	p, err := windows.BytePtrFromString(s)
	if err != nil {
		p, _ = windows.BytePtrFromString("")
	}
	return p
}

// Client es la conexión con Steam como si fuéramos el juego AppID.
type Client struct {
	AppID uint32

	client, utils, apps, ugc uintptr
	pipe, user               uintptr

	pGetCallback, pFreeCallback, pGetResult uintptr
}

// Open conecta con el Steam abierto haciéndose pasar por el juego appid.
// steamDir es la carpeta de Steam; su steamclient64.dll tiene que estar firmado
// por Valve (check lo comprueba).
func Open(steamDir string, appid uint32, check func(path string) bool) (*Client, error) {
	if appid == 0 {
		return nil, errors.New("falta el juego")
	}
	dll := filepath.Join(steamDir, "steamclient64.dll")
	if _, err := os.Stat(dll); err != nil {
		return nil, ErrUnsupported
	}
	if check != nil && !check(dll) {
		return nil, errors.New("el steamclient64.dll de la carpeta de Steam no está firmado por Valve: no se usa")
	}
	// Steam mira estas variables para saber qué juego es este proceso.
	id := strconv.FormatUint(uint64(appid), 10)
	os.Setenv("SteamAppId", id)
	os.Setenv("SteamGameId", id)

	h, err := windows.LoadLibraryEx(dll, 0, windows.LOAD_WITH_ALTERED_SEARCH_PATH)
	if err != nil {
		return nil, fmt.Errorf("no se pudo cargar steamclient64.dll: %w", err)
	}
	proc := func(name string) uintptr {
		p, err := windows.GetProcAddress(h, name)
		if err != nil {
			return 0
		}
		return p
	}
	create := proc("CreateInterface")
	c := &Client{AppID: appid, pGetCallback: proc("Steam_BGetCallback"), pFreeCallback: proc("Steam_FreeLastCallback"), pGetResult: proc("Steam_GetAPICallResult")}
	if create == 0 || c.pGetCallback == 0 || c.pFreeCallback == 0 || c.pGetResult == 0 {
		return nil, ErrUnsupported
	}
	if known := proc("Steam_IsKnownInterface"); known != 0 {
		if call(known, uintptr(unsafe.Pointer(cstr(verUGC))))&0xff == 0 {
			return nil, ErrUnsupported
		}
	}
	for _, v := range clientVersions {
		var rc int32
		c.client = call(create, uintptr(unsafe.Pointer(cstr(v))), uintptr(unsafe.Pointer(&rc)))
		if c.client != 0 {
			break
		}
	}
	if c.client == 0 {
		return nil, ErrUnsupported
	}
	c.pipe = vcall(c.client, cCreateSteamPipe) & 0xffffffff
	if c.pipe == 0 {
		return nil, ErrNoSteam
	}
	c.user = vcall(c.client, cConnectToGlobalUser, c.pipe) & 0xffffffff
	if c.user == 0 {
		vcall(c.client, cBReleaseSteamPipe, c.pipe)
		return nil, ErrNoUser
	}
	c.utils = vcall(c.client, cGetISteamUtils, c.pipe, uintptr(unsafe.Pointer(cstr(verUtils))))
	c.apps = vcall(c.client, cGetISteamApps, c.user, c.pipe, uintptr(unsafe.Pointer(cstr(verApps))))
	c.ugc = vcall(c.client, cGetISteamUGC, c.user, c.pipe, uintptr(unsafe.Pointer(cstr(verUGC))))
	if c.utils == 0 || c.apps == 0 || c.ugc == 0 {
		c.Close()
		return nil, ErrUnsupported
	}
	if got := uint32(vcall(c.utils, uGetAppID)); got != appid {
		c.Close()
		return nil, fmt.Errorf("Steam no ha reconocido el juego (%d)", got)
	}
	if !vbool(c.apps, aBIsSubscribedApp, uintptr(appid)) {
		c.Close()
		return nil, ErrNotOwned
	}
	return c, nil
}

// Close suelta la conexión (Steam deja de ver el juego abierto cuando el proceso
// termina).
func (c *Client) Close() {
	if c == nil || c.client == 0 {
		return
	}
	if c.user != 0 {
		vcall(c.client, cReleaseUser, c.pipe, c.user)
		c.user = 0
	}
	if c.pipe != 0 {
		vcall(c.client, cBReleaseSteamPipe, c.pipe)
		c.pipe = 0
	}
}

// pump vacía la cola de avisos de Steam. Devuelve si ha llegado el de que la
// petición call ha terminado.
func (c *Client) pump(want uint64) bool {
	done := false
	for i := 0; i < 500; i++ {
		var msg [sizeCallbackMsg]byte
		if call(c.pGetCallback, c.pipe, uintptr(unsafe.Pointer(&msg[0])))&0xff == 0 {
			break
		}
		id := int32(binary.LittleEndian.Uint32(msg[4:]))
		if id == cbAPICallCompleted {
			param := uintptr(binary.LittleEndian.Uint64(msg[8:]))
			var cc [16]byte
			readMem(uintptr(unsafe.Pointer(&cc[0])), param, len(cc))
			if binary.LittleEndian.Uint64(cc[:]) == want {
				done = true
			}
		}
		call(c.pFreeCallback, c.pipe)
	}
	return done
}

// wait espera la respuesta de una petición asíncrona y la devuelve.
func (c *Client) wait(h uint64, cb int, size int, timeout time.Duration) ([]byte, error) {
	if h == 0 {
		return nil, errors.New("Steam no ha aceptado la petición")
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		got := c.pump(h)
		var failed uint8
		if !got {
			got = vbool(c.utils, uIsAPICallCompleted, uintptr(h), uintptr(unsafe.Pointer(&failed)))
		}
		if got {
			buf := make([]byte, size)
			failed = 0
			ok := call(c.pGetResult, c.pipe, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(size), uintptr(cb), uintptr(unsafe.Pointer(&failed)))&0xff != 0
			if !ok {
				ok = vbool(c.utils, uGetAPICallResult, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(size), uintptr(cb), uintptr(unsafe.Pointer(&failed)))
			}
			if !ok || failed != 0 {
				reason := int32(vcall(c.utils, uGetAPICallFailureReason, uintptr(h)))
				if reason == 1 || reason == 0 { // sin conexión con los servidores de Steam
					return nil, errors.New("Steam no tiene conexión con sus servidores")
				}
				return nil, fmt.Errorf("Steam no pudo completar la petición (%d)", reason)
			}
			return buf, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil, errors.New("Steam no ha respondido a tiempo")
}

func resultErr(r int32) error {
	switch r {
	case resultOK:
		return nil
	case resultFileNotFound:
		return errors.New("Ese mod ya no existe o es privado")
	case resultAccessDenied:
		return errors.New("Steam no lo permite con esta cuenta")
	case resultNotLoggedOn:
		return errors.New("Steam no tiene conexión con sus servidores")
	case resultLimitExceeded:
		return errors.New("Has llegado al límite de suscripciones de Steam")
	}
	return fmt.Errorf("Steam ha dicho que no (código %d)", r)
}

// Subscribe suscribe la cuenta abierta en Steam a un mod.
func (c *Client) Subscribe(id uint64) error {
	b, err := c.wait(uint64(vcall(c.ugc, gSubscribeItem, uintptr(id))), cbSubscribe, sizeSubscribeResult, 30*time.Second)
	if err != nil {
		return err
	}
	return resultErr(int32(binary.LittleEndian.Uint32(b)))
}

// Unsubscribe quita la suscripción (Steam borra sus archivos cuando puede).
func (c *Client) Unsubscribe(id uint64) error {
	b, err := c.wait(uint64(vcall(c.ugc, gUnsubscribeItem, uintptr(id))), cbUnsubscribe, sizeSubscribeResult, 30*time.Second)
	if err != nil {
		return err
	}
	return resultErr(int32(binary.LittleEndian.Uint32(b)))
}

// BatchWindow es cuántas suscripciones o bajas se dejan en marcha a la vez.
const BatchWindow = 10

// Batch suscribe (sub) o quita unos mods con varios a la vez en marcha (window),
// para que cientos no tarden una eternidad: Steam contesta cada uno por su
// cuenta. done se llama cuando termina cada mod. stop se mira antes de mandar
// cada uno: si dice que pare, no se mandan más (los que ya van se esperan).
func (c *Client) Batch(sub bool, ids []uint64, window int, done func(id uint64, err error), stop func() bool) {
	method, cb := gUnsubscribeItem, cbUnsubscribe
	if sub {
		method, cb = gSubscribeItem, cbSubscribe
	}
	if window < 1 {
		window = 1
	}
	type call struct {
		id uint64
		at time.Time
	}
	pending := map[uint64]call{}
	next := 0
	lastPoll := time.Now()
	for next < len(ids) || len(pending) > 0 {
		for len(pending) < window && next < len(ids) {
			if stop != nil && stop() {
				next = len(ids)
				break
			}
			id := ids[next]
			next++
			h := uint64(vcall(c.ugc, method, uintptr(id)))
			if h == 0 {
				done(id, errors.New("Steam no ha aceptado la petición"))
				continue
			}
			pending[h] = call{id, time.Now()}
		}
		finished := c.completedCalls()
		if time.Since(lastPoll) > 250*time.Millisecond {
			// por si algún aviso de Steam se ha perdido
			for h := range pending {
				var failed uint8
				if vbool(c.utils, uIsAPICallCompleted, uintptr(h), uintptr(unsafe.Pointer(&failed))) {
					finished = append(finished, h)
				}
			}
			lastPoll = time.Now()
		}
		for _, h := range finished {
			cl, ok := pending[h]
			if !ok {
				continue
			}
			delete(pending, h)
			done(cl.id, c.callResult(h, cb))
		}
		for h, cl := range pending {
			if time.Since(cl.at) > 45*time.Second {
				delete(pending, h)
				done(cl.id, errors.New("Steam no ha respondido a tiempo"))
			}
		}
		if len(finished) == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// completedCalls vacía la cola de avisos de Steam y devuelve las peticiones
// que han terminado.
func (c *Client) completedCalls() []uint64 {
	var out []uint64
	for i := 0; i < 500; i++ {
		var msg [sizeCallbackMsg]byte
		if call(c.pGetCallback, c.pipe, uintptr(unsafe.Pointer(&msg[0])))&0xff == 0 {
			break
		}
		if int32(binary.LittleEndian.Uint32(msg[4:])) == cbAPICallCompleted {
			var cc [16]byte
			readMem(uintptr(unsafe.Pointer(&cc[0])), uintptr(binary.LittleEndian.Uint64(msg[8:])), len(cc))
			out = append(out, binary.LittleEndian.Uint64(cc[:]))
		}
		call(c.pFreeCallback, c.pipe)
	}
	return out
}

// callResult lee la respuesta de una suscripción o baja ya terminada.
func (c *Client) callResult(h uint64, cb int) error {
	buf := make([]byte, sizeSubscribeResult)
	var failed uint8
	ok := call(c.pGetResult, c.pipe, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(cb), uintptr(unsafe.Pointer(&failed)))&0xff != 0
	if !ok {
		ok = vbool(c.utils, uGetAPICallResult, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(cb), uintptr(unsafe.Pointer(&failed)))
	}
	if !ok || failed != 0 {
		return errors.New("Steam no tiene conexión con sus servidores")
	}
	return resultErr(int32(binary.LittleEndian.Uint32(buf)))
}

// Subscribed devuelve los mods de este juego a los que está suscrita la cuenta.
func (c *Client) Subscribed() []uint64 {
	n := uint32(vcall(c.ugc, gGetNumSubscribedItems))
	if n == 0 {
		return []uint64{}
	}
	if n > 100000 {
		n = 100000
	}
	ids := make([]uint64, n)
	got := uint32(vcall(c.ugc, gGetSubscribedItems, uintptr(unsafe.Pointer(&ids[0])), uintptr(n)))
	if got < n {
		ids = ids[:got]
	}
	return ids
}

// ItemState es cómo tiene Steam un mod en este PC.
type ItemState struct {
	ID         uint64 `json:"id"`
	Flags      uint32 `json:"flags"`
	Size       int64  `json:"size"`
	Folder     string `json:"folder"`
	Updated    int64  `json:"updated"`
	Downloaded int64  `json:"downloaded"`
	Total      int64  `json:"total"`
}

// State dice si está suscrito, instalado, pendiente de bajar…
func (c *Client) State(id uint64) ItemState {
	s := ItemState{ID: id, Flags: uint32(vcall(c.ugc, gGetItemState, uintptr(id)))}
	if s.Flags&StateInstalled != 0 {
		var size uint64
		var ts uint32
		folder := make([]byte, 1024)
		if vbool(c.ugc, gGetItemInstallInfo, uintptr(id), uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&folder[0])), uintptr(len(folder)), uintptr(unsafe.Pointer(&ts))) {
			s.Size, s.Folder, s.Updated = int64(size), cString(folder), int64(ts)
		}
	}
	if s.Flags&(StateDownloading|StateDownloadPending) != 0 {
		var done, total uint64
		if vbool(c.ugc, gGetItemDownloadInfo, uintptr(id), uintptr(unsafe.Pointer(&done)), uintptr(unsafe.Pointer(&total))) {
			s.Downloaded, s.Total = int64(done), int64(total)
		}
	}
	return s
}

// Item son los datos de un mod en el Workshop.
type Item struct {
	ID            uint64   `json:"id"`
	AppID         uint32   `json:"appid"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Owner         string   `json:"owner"` // SteamID64 del autor
	Created       int64    `json:"created"`
	Updated       int64    `json:"updated"`
	Visibility    int32    `json:"visibility"`
	Banned        bool     `json:"banned"`
	Tags          []string `json:"tags"`
	Size          int64    `json:"size"`
	Preview       string   `json:"preview"`
	VotesUp       uint32   `json:"votesUp"`
	VotesDown     uint32   `json:"votesDown"`
	Score         float32  `json:"score"` // 0 a 1
	Collection    bool     `json:"collection"`
	Children      []uint64 `json:"children"` // mods que necesita (o los de la colección)
	Subscriptions uint64   `json:"subscriptions"`
	Favorites     uint64   `json:"favorites"`
	Comments      uint64   `json:"comments"`
	Missing       bool     `json:"missing"` // ya no existe o es privado
	// Imágenes y vídeos de su página (solo si se han pedido: HasPreviews)
	Previews    []Preview `json:"previews,omitempty"`
	HasPreviews bool      `json:"hasPreviews,omitempty"`
}

// Preview es una imagen (URL) o un vídeo de YouTube (Video, su id) de un mod.
type Preview struct {
	URL   string `json:"url,omitempty"`
	Video string `json:"video,omitempty"`
}

// Page es una página de resultados del Workshop.
type Page struct {
	Items  []Item `json:"items"`
	Total  uint32 `json:"total"`
	Cursor string `json:"cursor"` // para la página siguiente ("" = no hay más)
}

func cString(b []byte) string {
	if i := indexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return strings.ToValidUTF8(string(b), "")
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

// runQuery envía una consulta ya preparada y lee sus resultados.
func (c *Client) runQuery(h uint64, children, previews bool) (Page, error) {
	var p Page
	if h == 0 || h == math.MaxUint64 {
		return p, errors.New("Steam no ha aceptado la búsqueda")
	}
	defer vcall(c.ugc, gReleaseQueryUGCRequest, uintptr(h))
	b, err := c.wait(uint64(vcall(c.ugc, gSendQueryUGCRequest, uintptr(h))), cbQueryCompleted, sizeQueryCompleted, 40*time.Second)
	if err != nil {
		return p, err
	}
	if r := int32(binary.LittleEndian.Uint32(b[8:])); r != resultOK {
		return p, resultErr(r)
	}
	n := binary.LittleEndian.Uint32(b[12:])
	p.Total = binary.LittleEndian.Uint32(b[16:])
	p.Cursor = cString(b[21:])
	p.Items = make([]Item, 0, n)
	d := make([]byte, sizeDetails+64)
	for i := uint32(0); i < n; i++ {
		clear(d)
		if !vbool(c.ugc, gGetQueryUGCResult, uintptr(h), uintptr(i), uintptr(unsafe.Pointer(&d[0]))) {
			continue
		}
		it := parseDetails(d)
		if it.Missing {
			p.Items = append(p.Items, it)
			continue
		}
		url := make([]byte, 1024)
		if vbool(c.ugc, gGetQueryUGCPreviewURL, uintptr(h), uintptr(i), uintptr(unsafe.Pointer(&url[0])), uintptr(len(url))) {
			it.Preview = cString(url)
		}
		stat := func(kind int) uint64 {
			var v uint64
			if vbool(c.ugc, gGetQueryUGCStatistic, uintptr(h), uintptr(i), uintptr(kind), uintptr(unsafe.Pointer(&v))) {
				return v
			}
			return 0
		}
		it.Subscriptions, it.Favorites, it.Comments = stat(0), stat(1), stat(10)
		if nc := binary.LittleEndian.Uint32(d[9768:]); children && nc > 0 {
			if nc > 5000 {
				nc = 5000
			}
			kids := make([]uint64, nc)
			if vbool(c.ugc, gGetQueryUGCChildren, uintptr(h), uintptr(i), uintptr(unsafe.Pointer(&kids[0])), uintptr(nc)) {
				it.Children = kids
			}
		}
		if it.Children == nil {
			it.Children = []uint64{}
		}
		if previews {
			it.HasPreviews = true
			n := uint32(vcall(c.ugc, gGetQueryUGCNumPreviews, uintptr(h), uintptr(i)))
			for j := uint32(0); j < n && j < 60; j++ {
				u := make([]byte, 1024)
				name := make([]byte, 260)
				var kind int32
				if !vbool(c.ugc, gGetQueryUGCPreview, uintptr(h), uintptr(i), uintptr(j), uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)), uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)), uintptr(unsafe.Pointer(&kind))) {
					continue
				}
				switch v := cString(u); kind {
				case 0: // imagen
					it.Previews = append(it.Previews, Preview{URL: v})
				case 1: // vídeo de YouTube
					it.Previews = append(it.Previews, Preview{Video: v})
				}
			}
		}
		p.Items = append(p.Items, it)
	}
	return p, nil
}

// parseDetails lee un SteamUGCDetails_t (versión 020 de la interfaz).
func parseDetails(d []byte) Item {
	le := binary.LittleEndian
	it := Item{
		ID:    le.Uint64(d[0:]),
		AppID: le.Uint32(d[20:]),
		Tags:  []string{},
	}
	if r := int32(le.Uint32(d[8:])); r != resultOK {
		it.Missing = true
		return it
	}
	it.Collection = int32(le.Uint32(d[12:])) == fileTypeCollection
	it.Title = strings.TrimSpace(cString(d[24 : 24+129]))
	it.Description = cString(d[153 : 153+8000])
	if o := le.Uint64(d[8160:]); o != 0 {
		it.Owner = strconv.FormatUint(o, 10)
	}
	it.Created = int64(le.Uint32(d[8168:]))
	it.Updated = int64(le.Uint32(d[8172:]))
	it.Visibility = int32(le.Uint32(d[8180:]))
	it.Banned = d[8184] != 0
	for _, t := range strings.Split(cString(d[8187:8187+1025]), ",") {
		if t = strings.TrimSpace(t); t != "" {
			it.Tags = append(it.Tags, t)
		}
	}
	it.Size = int64(int32(le.Uint32(d[9492:])))
	if total := int64(le.Uint64(d[9776:])); total > it.Size {
		it.Size = total
	}
	it.VotesUp = le.Uint32(d[9756:])
	it.VotesDown = le.Uint32(d[9760:])
	it.Score = math.Float32frombits(le.Uint32(d[9764:]))
	return it
}

// Details pide los datos de unos mods (de 50 en 50, lo que deja Steam).
func (c *Client) Details(ids []uint64) ([]Item, error) {
	out := make([]Item, 0, len(ids))
	for len(ids) > 0 {
		n := min(len(ids), 50)
		chunk := append([]uint64(nil), ids[:n]...)
		ids = ids[n:]
		h := uint64(vcall(c.ugc, gCreateQueryUGCDetailsRequest, uintptr(unsafe.Pointer(&chunk[0])), uintptr(len(chunk))))
		if h == 0 || h == math.MaxUint64 {
			return out, errors.New("Steam no ha aceptado la consulta")
		}
		vcall(c.ugc, gSetReturnLongDescription, uintptr(h), 1)
		vcall(c.ugc, gSetReturnChildren, uintptr(h), 1)
		vcall(c.ugc, gSetReturnAdditionalPreviews, uintptr(h), 1)
		vcall(c.ugc, gSetAllowCachedResponse, uintptr(h), 300)
		p, err := c.runQuery(h, true, true)
		if err != nil {
			return out, err
		}
		out = append(out, p.Items...)
	}
	return out, nil
}

// Search busca mods de este juego en el Workshop. sort es uno de los Query*;
// con texto se ordena por parecido. cursor "" es la primera página. tags son
// etiquetas del Workshop que tienen que tener todos.
func (c *Client) Search(text string, sort int, cursor string, tags []string) (Page, error) {
	return c.SearchEx(SearchOptions{Text: text, Sort: sort, Cursor: cursor, Tags: tags})
}

// SearchOptions son los filtros de una búsqueda en el Workshop.
type SearchOptions struct {
	Text    string
	Sort    int      // uno de los Query*
	Days    int      // para QueryByTrend: populares de los últimos N días
	Cursor  string   // "" = primera página
	Tags    []string // tienen que tener todas
	Exclude []string // no pueden tener ninguna
}

// SearchEx busca con todos los filtros.
func (c *Client) SearchEx(o SearchOptions) (Page, error) {
	text, sort, cursor, tags := strings.TrimSpace(o.Text), o.Sort, o.Cursor, o.Tags
	if cursor == "" {
		cursor = "*"
	}
	h := uint64(vcall(c.ugc, gCreateQueryAllUGCRequestCursor, uintptr(sort), matchingItemsReady, uintptr(c.AppID), uintptr(c.AppID), uintptr(unsafe.Pointer(cstr(cursor)))))
	if h == 0 || h == math.MaxUint64 {
		return Page{}, errors.New("Steam no ha aceptado la búsqueda")
	}
	if text != "" {
		vcall(c.ugc, gSetSearchText, uintptr(h), uintptr(unsafe.Pointer(cstr(text))))
	}
	if sort == QueryByTrend {
		days := o.Days
		if days <= 0 || days > 365 {
			days = 7
		}
		vcall(c.ugc, gSetRankedByTrendDays, uintptr(h), uintptr(days))
	}
	for _, t := range o.Exclude {
		if t = strings.TrimSpace(t); t != "" {
			vcall(c.ugc, gAddExcludedTag, uintptr(h), uintptr(unsafe.Pointer(cstr(t))))
		}
	}
	for _, t := range tags {
		if t = strings.TrimSpace(t); t != "" {
			vcall(c.ugc, gAddRequiredTag, uintptr(h), uintptr(unsafe.Pointer(cstr(t))))
		}
	}
	vcall(c.ugc, gSetAllowCachedResponse, uintptr(h), 120)
	p, err := c.runQuery(h, false, false)
	if err == nil && p.Cursor == cursor {
		p.Cursor = ""
	}
	return p, err
}
