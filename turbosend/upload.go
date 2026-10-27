package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

const inChunk = 16 << 20 // Cloudflare corta peticiones de más de 100 MB

// maxOfferTotal: tope de lo que un invitado puede ofrecer de una vez (4 TB). Sin
// tope, una oferta de miles de archivos «gigantes» hacía trabajar al PC en vano.
const maxOfferTotal = 4 << 40

type offerFile struct {
	Rel  string `json:"rel"`
	Size int64  `json:"size"`
}

var reservedNames = map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true}

// invisibleMark: caracteres de control de dirección y de anchura cero. Sirven para
// disfrazar nombres («foto\u202Egpj.exe» se ve como «fotoexe.jpg»), así que se quitan.
func invisibleMark(r rune) bool {
	return r == 0x200B || r == 0x200E || r == 0x200F || r >= 0x202A && r <= 0x202E || r >= 0x2066 && r <= 0x2069 || r == 0xFEFF
}

// safeComponent limpia un trozo de ruta para que sea válido y no salga de su carpeta.
func safeComponent(s string) string {
	s = strings.Map(func(r rune) rune {
		if invisibleMark(r) {
			return -1
		}
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, s)
	s = strings.TrimRight(strings.TrimSpace(s), ". ")
	if s == "" || s == "." || s == ".." {
		s = "_"
	}
	base := strings.ToUpper(strings.SplitN(s, ".", 2)[0])
	if reservedNames[base] {
		s = "_" + s
	}
	if len([]rune(s)) > 180 {
		s = string([]rune(s)[:180])
	}
	return s
}

func safeRel(rel string) []string {
	var out []string
	for _, p := range strings.Split(strings.ReplaceAll(rel, `\`, "/"), "/") {
		if p == "" || p == "." || p == ".." {
			continue
		}
		out = append(out, safeComponent(p))
	}
	if len(out) == 0 {
		out = []string{"archivo"}
	}
	return out
}

// uniqueName devuelve un nombre libre dentro de dir ("foto (2).jpg" si ya existe).
func uniqueName(dir, name string, taken map[string]bool) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for n := 1; n < 10000; n++ {
		cand := name
		if n > 1 {
			cand = fmt.Sprintf("%s (%d)%s", base, n, ext)
		}
		if taken[strings.ToLower(cand)] {
			continue
		}
		p := filepath.Join(dir, cand)
		if _, err := os.Stat(p); err == nil {
			continue
		}
		if _, err := os.Stat(p + ".turbosend-part"); err == nil {
			continue
		}
		return cand
	}
	return name
}

func freeSpace(dir string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return 0, err
	}
	return free, nil
}

// markFromInternet añade la marca "descargado de Internet" que pone Windows a las descargas.
func markFromInternet(path string) {
	os.WriteFile(path+":Zone.Identifier", []byte("[ZoneTransfer]\r\nZoneId=3\r\n"), 0o644)
}

// Offer registra lo que un invitado quiere enviar.
func (a *App) Offer(roomID string, m *Member, files []offerFile) (*InBundle, error) {
	if len(files) == 0 || len(files) > 100000 {
		return nil, errors.New("lista de archivos no válida")
	}
	b := &InBundle{ID: newID(), From: m.PID, Chunk: inChunk, Status: "offered", Created: time.Now().UnixMilli()}
	tops := map[string]bool{}
	for _, f := range files {
		if f.Size < 0 || f.Size > 1<<44 {
			return nil, errors.New("tamaño no válido")
		}
		parts := safeRel(f.Rel)
		tops[parts[0]] = true
		b.Files = append(b.Files, &InFile{Rel: strings.Join(parts, "/"), Size: f.Size})
		b.Total += f.Size
		if b.Total > maxOfferTotal {
			return nil, errors.New("el envío es demasiado grande (más de 4 TB)")
		}
	}
	switch {
	case len(b.Files) == 1 && !strings.Contains(b.Files[0].Rel, "/"):
		b.Kind, b.Name = "file", b.Files[0].Rel
	case len(tops) == 1:
		for t := range tops {
			b.Kind, b.Name = "multi", t
		}
	default:
		b.Kind, b.Name = "multi", fmt.Sprintf("%d archivos", len(b.Files))
	}

	auto := false
	a.rooms.With(roomID, func(r *Room) bool {
		set := a.getSettings()
		switch r.Auto {
		case "on":
			auto = true
		case "off":
			auto = false
		default:
			auto = set.AutoAccept
		}
		if auto && set.AutoLimitGB > 0 && float64(b.Total) > set.AutoLimitGB*(1<<30) {
			auto = false
		}
		r.In = append(r.In, b)
		return true
	})
	msg := newMessage(m.PID, "in")
	msg.Bundle = b.ID
	a.post(roomID, msg, true)
	if auto {
		if err := a.AcceptIn(roomID, b.ID); err != nil {
			a.notice(roomID, "No se pudo aceptar automáticamente: "+err.Error())
		}
	}
	return b, nil
}

// AcceptIn elige dónde se guardará cada archivo y pide al invitado que empiece a
// enviar. Los archivos se crean al llegar su primer bloque: con un envío de cien
// mil archivos, crearlos todos aquí, con el chat bloqueado, congelaba la app.
func (a *App) AcceptIn(roomID, bid string) error {
	set := a.getSettings()
	// Elegir nombres libres mira el disco una vez por nombre: se hace sin el candado
	// del chat, pero de un envío en un envío para que dos no elijan el mismo.
	a.acceptMu.Lock()
	defer a.acceptMu.Unlock()

	var err error
	var from, base, kind string
	var need int64
	var rels []string // solo la primera vez: aún no tiene nombres
	var taken map[string]bool
	ok := false
	a.rooms.With(roomID, func(r *Room) bool {
		b := r.inBundle(bid)
		if b == nil {
			err = errors.New("ese envío ya no existe")
			return false
		}
		if b.Status != "offered" && b.Status != "failed" {
			return false
		}
		ok = true
		from, kind = b.From, b.Kind
		need = b.Total - b.Received
		base = filepath.Join(set.RecvDir, safeComponent(r.Name))
		if b.Root == "" {
			rels = make([]string, len(b.Files))
			for i, f := range b.Files {
				rels[i] = f.Rel
			}
		}
		return false
	})
	if err != nil || !ok {
		return err
	}
	if rels != nil {
		taken = a.reservedTops(base, bid)
	}
	if e := os.MkdirAll(base, 0o755); e != nil {
		return fmt.Errorf("no se pudo crear la carpeta de recepción: %v", e)
	}
	if free, e := freeSpace(base); e == nil && free < uint64(need)+(256<<20) {
		return fmt.Errorf("no hay espacio suficiente en el disco (hacen falta %s)", humanBytes(need))
	}
	var finals []string
	root := ""
	if rels != nil {
		finals = make([]string, len(rels))
		tops := map[string]string{}
		for i, rel := range rels {
			parts := strings.Split(rel, "/")
			top, seen := tops[parts[0]]
			if !seen {
				top = uniqueName(base, parts[0], taken)
				taken[strings.ToLower(top)] = true
				tops[parts[0]] = top
			}
			parts[0] = top
			finals[i] = filepath.Join(append([]string{base}, parts...)...)
		}
		if kind == "file" || len(tops) == 1 {
			for _, t := range tops {
				root = filepath.Join(base, t)
			}
		} else {
			root = base
		}
	}
	accepted := false
	a.rooms.With(roomID, func(r *Room) bool {
		b := r.inBundle(bid)
		if b == nil {
			err = errors.New("ese envío ya no existe")
			return false
		}
		if b.Status != "offered" && b.Status != "failed" {
			return false
		}
		if b.Root == "" && len(finals) == len(b.Files) {
			for i, f := range b.Files {
				f.Final = finals[i]
			}
			b.Root = root
		}
		b.Status = "accepted"
		b.Error = ""
		accepted = true
		return true
	})
	if err != nil || !accepted {
		return err
	}
	a.bundleChanged(roomID, bid)
	a.hub.SendTo(roomID, from, "upload", map[string]string{"bundle": bid})
	a.maybeFinish(roomID, bid) // por si solo había archivos vacíos
	return nil
}

// reservedTops: nombres ya dados (dentro de base) a envíos sin terminar de cualquier
// chat. Sus archivos pueden no existir aún en el disco, y dos chats con el mismo
// nombre comparten carpeta.
func (a *App) reservedTops(base, except string) map[string]bool {
	taken := map[string]bool{}
	for _, id := range a.rooms.IDs() {
		a.rooms.With(id, func(r *Room) bool {
			for _, o := range r.In {
				if o.ID == except || o.Root == "" || o.Status == "done" || o.Status == "rejected" || o.Status == "cancelled" {
					continue
				}
				for _, f := range o.Files {
					if rel, e := filepath.Rel(base, f.Final); e == nil && rel != "." && !strings.HasPrefix(rel, "..") {
						taken[strings.ToLower(strings.SplitN(filepath.ToSlash(rel), "/", 2)[0])] = true
					}
				}
			}
			return false
		})
	}
	return taken
}

func (a *App) RejectIn(roomID, bid string) {
	a.setInStatus(roomID, bid, "rejected", "", true)
}

// CancelIn cancela un envío entrante y borra lo recibido a medias.
func (a *App) CancelIn(roomID, bid string) {
	a.setInStatus(roomID, bid, "cancelled", "", true)
}

func (a *App) setInStatus(roomID, bid, status, errMsg string, cleanup bool) {
	var parts []string
	a.rooms.With(roomID, func(r *Room) bool {
		b := r.inBundle(bid)
		if b == nil || b.Status == "done" {
			return false
		}
		b.Status, b.Error = status, errMsg
		if cleanup {
			for _, f := range b.Files {
				if f.Final != "" && !f.Done {
					parts = append(parts, f.Final+".turbosend-part")
				}
			}
		}
		return true
	})
	for _, p := range parts {
		os.Remove(p)
	}
	a.tracker.Finish("in|" + bid)
	a.bundleChanged(roomID, bid)
}

// receiveChunk guarda un bloque ya verificado.
func (a *App) receiveChunk(roomID, bid string, fi int, off int64, data []byte) error {
	var final string
	var err error
	fresh := false // aún no ha llegado ningún bloque de este archivo
	from := ""
	a.rooms.With(roomID, func(r *Room) bool {
		b := r.inBundle(bid)
		if b == nil || (b.Status != "accepted" && b.Status != "transferring") {
			err = errors.New("este envío no está aceptado")
			return false
		}
		if fi < 0 || fi >= len(b.Files) {
			err = errors.New("archivo no válido")
			return false
		}
		f := b.Files[fi]
		if off%b.Chunk != 0 || off >= f.Size {
			err = errors.New("bloque no válido")
			return false
		}
		want := b.Chunk
		if f.Size-off < want {
			want = f.Size - off
		}
		if int64(len(data)) != want {
			err = errors.New("tamaño de bloque incorrecto")
			return false
		}
		final = f.Final
		fresh = !f.anyHave()
		from = b.From
		return false
	})
	if err != nil {
		return err
	}
	part := final + ".turbosend-part"
	fh, e := os.OpenFile(part, os.O_WRONLY, 0o644)
	if errors.Is(e, os.ErrNotExist) {
		if !fresh {
			// Ya habían llegado bloques y el archivo a medias no está (un antivirus, o
			// lo han borrado): crearlo de nuevo lo dejaría con ceros donde iban. Se
			// vuelve a pedir entero.
			a.forgetFile(roomID, bid, fi)
			// Cuando al invitado le llegue el error, que vuelva a empezar con lo que falta.
			go func() {
				time.Sleep(1500 * time.Millisecond)
				a.hub.SendTo(roomID, from, "upload", map[string]string{"bundle": bid})
			}()
			return errPartLost
		}
		// Primer bloque de este archivo: se crea ahora (y su carpeta).
		os.MkdirAll(filepath.Dir(final), 0o755)
		fh, e = os.OpenFile(part, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
		if errors.Is(e, os.ErrExist) {
			fh, e = os.OpenFile(part, os.O_WRONLY, 0o644) // otro bloque del mismo archivo lo acaba de crear
		}
	}
	if e != nil {
		return fmt.Errorf("no se pudo abrir el archivo de destino: %v", e)
	}
	_, e = fh.WriteAt(data, off)
	fh.Close()
	if e != nil {
		return fmt.Errorf("no se pudo escribir en el disco: %v", e)
	}
	first, gone := false, false
	a.rooms.With(roomID, func(r *Room) bool {
		b := r.inBundle(bid)
		if b == nil {
			gone = true
			return false
		}
		if b.Status != "accepted" && b.Status != "transferring" && b.Status != "finishing" {
			// Se canceló mientras llegaba el bloque: no se deja el archivo a medias.
			gone = b.Status == "cancelled" || b.Status == "rejected"
			return false
		}
		f := b.Files[fi]
		ci := off / b.Chunk
		if !f.has(ci) {
			f.mark(ci)
			b.Received += int64(len(data))
		}
		if b.Status == "accepted" {
			b.Status = "transferring"
			first = true
		}
		return true
	})
	if gone {
		os.Remove(part)
		return errors.New("este envío ya no está aceptado")
	}
	if first {
		a.bundleChanged(roomID, bid)
	}
	a.maybeFinish(roomID, bid)
	return nil
}

// errPartLost: el archivo a medias ha desaparecido o le falta algo.
var errPartLost = errors.New("el archivo a medias ha desaparecido (¿un antivirus?): se vuelve a pedir")

// anyHave: ha llegado algún bloque de este archivo.
func (f *InFile) anyHave() bool {
	for _, b := range f.Have {
		if b != 0 {
			return true
		}
	}
	return false
}

// forgetFile olvida los bloques recibidos de un archivo (para pedirlo otra vez).
func (a *App) forgetFile(roomID, bid string, fi int) {
	a.rooms.With(roomID, func(r *Room) bool {
		b := r.inBundle(bid)
		if b == nil || fi < 0 || fi >= len(b.Files) {
			return false
		}
		f := b.Files[fi]
		for i := int64(0); i < f.chunks(b.Chunk); i++ {
			if f.has(i) {
				b.Received -= min(b.Chunk, f.Size-i*b.Chunk)
			}
		}
		f.Have = nil
		if b.Received < 0 {
			b.Received = 0
		}
		return true
	})
}

// ensurePart comprueba que el archivo a medias está entero. Los vacíos no reciben
// ningún bloque, así que se crean aquí; uno que falta (o más corto) no se rellena con
// ceros: errPartLost.
func ensurePart(final string, size int64) error {
	part := final + ".turbosend-part"
	st, err := os.Stat(part)
	switch {
	case err == nil && st.Size() == size:
		return nil
	case err == nil && st.Size() > size:
		return os.Truncate(part, size)
	case err == nil || size > 0 || !errors.Is(err, os.ErrNotExist):
		return errPartLost
	}
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return err
	}
	fh, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	return fh.Close()
}

// maybeFinish renombra los archivos cuando han llegado todos los bloques.
func (a *App) maybeFinish(roomID, bid string) {
	var moves []*InFile
	complete := false
	a.rooms.With(roomID, func(r *Room) bool {
		b := r.inBundle(bid)
		if b == nil || (b.Status != "accepted" && b.Status != "transferring") {
			return false
		}
		for _, f := range b.Files {
			if f.Done {
				continue
			}
			n := f.chunks(b.Chunk)
			for i := int64(0); i < n; i++ {
				if !f.has(i) {
					return false
				}
			}
		}
		complete = true
		b.Status = "finishing" // evita que dos bloques que acaban a la vez lo cierren dos veces
		for _, f := range b.Files {
			if !f.Done {
				moves = append(moves, f)
			}
		}
		return true
	})
	if !complete {
		return
	}
	var ferr error
	finals := map[*InFile]string{}
	var lost []*InFile
	for _, f := range moves {
		if err := ensurePart(f.Final, f.Size); err != nil {
			if !errors.Is(err, errPartLost) {
				ferr = err
				continue
			}
			// Sin archivo a medias pero con el definitivo de su tamaño: se guardó justo
			// antes de que se cerrara la app.
			if st, e := os.Stat(f.Final); e == nil && !st.IsDir() && st.Size() == f.Size {
				finals[f] = f.Final
				continue
			}
			lost = append(lost, f)
			continue
		}
		part := f.Final + ".turbosend-part"
		dst := f.Final
		if _, err := os.Stat(dst); err == nil { // alguien creó ese nombre mientras tanto
			dst = filepath.Join(filepath.Dir(dst), uniqueName(filepath.Dir(dst), filepath.Base(dst), nil))
		}
		if err := os.Rename(part, dst); err != nil {
			ferr = err
			continue
		}
		markFromInternet(dst)
		finals[f] = dst
	}
	from := ""
	a.rooms.With(roomID, func(r *Room) bool {
		b := r.inBundle(bid)
		if b == nil {
			return false
		}
		// Con el candado: el guardado y la vista leen Final. Cada archivo que ya se
		// guardó queda hecho aunque otro haya fallado (al reintentar, solo los demás).
		for f, dst := range finals {
			f.Final = dst
			f.Done = true
			f.Have = nil
		}
		if len(lost) > 0 && ferr == nil {
			// Faltan archivos a medias: se olvida lo recibido de ellos y se piden otra vez.
			for _, f := range lost {
				for i := int64(0); i < f.chunks(b.Chunk); i++ {
					if f.has(i) {
						b.Received -= min(b.Chunk, f.Size-i*b.Chunk)
					}
				}
				f.Have = nil
			}
			b.Received = max(b.Received, 0)
			b.Status = "accepted"
			from = b.From
			return true
		}
		if ferr != nil {
			b.Status, b.Error = "failed", "No se pudo guardar: "+ferr.Error()
		} else {
			b.Status = "done"
			b.Finished = time.Now().UnixMilli()
			r.Unread++
		}
		return true
	})
	if from != "" {
		a.bundleChanged(roomID, bid)
		a.hub.SendTo(roomID, from, "upload", map[string]string{"bundle": bid})
		return
	}
	a.tracker.Finish("in|" + bid)
	a.bundleChanged(roomID, bid)
	if ferr == nil {
		a.notify(roomID, "in-done", bid)
	}
}

// missingChunks dice qué bloques faltan (para reanudar).
func (a *App) missingChunks(roomID, bid, pid string) (map[string]any, error) {
	var out map[string]any
	var err error
	a.rooms.With(roomID, func(r *Room) bool {
		b := r.inBundle(bid)
		if b == nil || b.From != pid {
			err = errors.New("envío no encontrado")
			return false
		}
		files := []map[string]any{}
		// Solo hace falta (y solo se calcula) cuando ya se puede subir.
		active := b.Status == "accepted" || b.Status == "transferring"
		for i, f := range b.Files {
			if f.Done || !active {
				continue
			}
			var miss []int64
			n := f.chunks(b.Chunk)
			for c := int64(0); c < n; c++ {
				if !f.has(c) {
					miss = append(miss, c)
				}
			}
			files = append(files, map[string]any{"i": i, "missing": miss})
		}
		out = map[string]any{"status": b.Status, "chunk": b.Chunk, "files": files}
		return false
	})
	if err == nil && out != nil && out["status"] == "accepted" {
		go a.maybeFinish(roomID, bid)
	}
	return out, err
}

// finishPending termina al arrancar los envíos que llegaron enteros pero no se
// llegaron a guardar (la app se cerró justo entonces).
func (a *App) finishPending() {
	for _, id := range a.rooms.IDs() {
		var bids []string
		a.rooms.With(id, func(r *Room) bool {
			for _, b := range r.In {
				if b.Status == "accepted" {
					bids = append(bids, b.ID)
				}
			}
			return false
		})
		for _, bid := range bids {
			a.maybeFinish(id, bid)
		}
	}
}

func humanBytes(n int64) string {
	f := float64(n)
	u := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for f >= 1024 && i < len(u)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", n)
	}
	return strings.Replace(fmt.Sprintf("%.1f %s", f, u[i]), ".", ",", 1)
}
