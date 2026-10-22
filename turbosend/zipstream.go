package main

import (
	"encoding/binary"
	"hash/crc32"
	"io"
	"os"
	"time"
)

// ZIP "store" (sin compresión) generado al vuelo, con tamaño final conocido
// de antemano para que el navegador muestre progreso real.
// Usa descriptores de datos (CRC al final de cada entrada) y ZIP64 cuando hace falta.

const (
	u32max = 0xFFFFFFFF
	u16max = 0xFFFF
)

type zipEntry struct {
	name   string
	size   int64
	mod    time.Time
	path   string
	offset int64
	crc    uint32
}

func zipNeeds64(e *zipEntry) bool { return e.size >= u32max || e.offset >= u32max }

func dosTime(t time.Time) (uint16, uint16) {
	t = t.Local()
	if t.Year() < 1980 {
		t = time.Date(1980, 1, 1, 0, 0, 0, 0, time.Local)
	}
	d := uint16((t.Year()-1980)<<9 | int(t.Month())<<5 | t.Day())
	tm := uint16(t.Hour()<<11 | t.Minute()<<5 | t.Second()/2)
	return d, tm
}

func localHeaderLen(e *zipEntry) int64 { return 30 + int64(len(e.name)) }
func descriptorLen(e *zipEntry) int64 {
	if e.size >= u32max {
		return 24
	}
	return 16
}

func centralExtraLen(e *zipEntry) int64 {
	n := int64(0)
	if e.size >= u32max {
		n += 16 // tamaño original + comprimido
	}
	if e.offset >= u32max {
		n += 8
	}
	if n > 0 {
		n += 4
	}
	return n
}

func zipLayout(files []ShareFile) ([]*zipEntry, int64) {
	entries := make([]*zipEntry, len(files))
	var off int64
	for i, f := range files {
		e := &zipEntry{name: f.Rel, size: f.Size, mod: time.Unix(f.ModTime, 0), path: f.Path, offset: off}
		entries[i] = e
		off += localHeaderLen(e) + e.size + descriptorLen(e)
	}
	cdStart := off
	for _, e := range entries {
		off += 46 + int64(len(e.name)) + centralExtraLen(e)
	}
	cdSize := off - cdStart
	if needsZip64End(len(entries), cdStart, cdSize) {
		off += 56 + 20
	}
	off += 22
	return entries, off
}

func needsZip64End(n int, cdStart, cdSize int64) bool {
	return n >= u16max || cdStart >= u32max || cdSize >= u32max
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// writeZip escribe el ZIP completo en w. buf se usa para copiar datos.
func writeZip(w io.Writer, entries []*zipEntry, buf []byte) error {
	cw := &countWriter{w: w}
	le := binary.LittleEndian
	for _, e := range entries {
		d, t := dosTime(e.mod)
		ver := uint16(20)
		if zipNeeds64(e) {
			ver = 45
		}
		h := make([]byte, 30, 30+len(e.name))
		le.PutUint32(h[0:], 0x04034b50)
		le.PutUint16(h[4:], ver)
		le.PutUint16(h[6:], 0x0808) // descriptor de datos + nombres UTF-8
		le.PutUint16(h[8:], 0)      // store
		le.PutUint16(h[10:], t)
		le.PutUint16(h[12:], d)
		// crc y tamaños a 0: van en el descriptor
		le.PutUint16(h[26:], uint16(len(e.name)))
		h = append(h, e.name...)
		if _, err := cw.Write(h); err != nil {
			return err
		}

		crc := crc32.NewIEEE()
		f, err := os.Open(e.path)
		if err != nil {
			return err
		}
		n, err := io.CopyBuffer(io.MultiWriter(cw, crc), io.LimitReader(f, e.size), buf)
		f.Close()
		if err != nil {
			return err
		}
		if n < e.size { // el archivo ha encogido: rellenamos para no romper el ZIP
			pad := make([]byte, 64*1024)
			rest := e.size - n
			for rest > 0 {
				k := int64(len(pad))
				if rest < k {
					k = rest
				}
				if _, err := io.MultiWriter(cw, crc).Write(pad[:k]); err != nil {
					return err
				}
				rest -= k
			}
		}
		e.crc = crc.Sum32()

		dd := make([]byte, 0, 24)
		dd = le.AppendUint32(dd, 0x08074b50)
		dd = le.AppendUint32(dd, e.crc)
		if e.size >= u32max {
			dd = le.AppendUint64(dd, uint64(e.size))
			dd = le.AppendUint64(dd, uint64(e.size))
		} else {
			dd = le.AppendUint32(dd, uint32(e.size))
			dd = le.AppendUint32(dd, uint32(e.size))
		}
		if _, err := cw.Write(dd); err != nil {
			return err
		}
	}

	cdStart := cw.n
	for _, e := range entries {
		d, t := dosTime(e.mod)
		ver := uint16(20)
		if zipNeeds64(e) {
			ver = 45
		}
		extraLen := centralExtraLen(e)
		h := make([]byte, 46, 46+len(e.name)+int(extraLen))
		le.PutUint32(h[0:], 0x02014b50)
		le.PutUint16(h[4:], 45)
		le.PutUint16(h[6:], ver)
		le.PutUint16(h[8:], 0x0808)
		le.PutUint16(h[10:], 0)
		le.PutUint16(h[12:], t)
		le.PutUint16(h[14:], d)
		le.PutUint32(h[16:], e.crc)
		sz := uint32(e.size)
		if e.size >= u32max {
			sz = u32max
		}
		le.PutUint32(h[20:], sz)
		le.PutUint32(h[24:], sz)
		le.PutUint16(h[28:], uint16(len(e.name)))
		le.PutUint16(h[30:], uint16(extraLen))
		off := uint32(e.offset)
		if e.offset >= u32max {
			off = u32max
		}
		le.PutUint32(h[42:], off)
		h = append(h, e.name...)
		if extraLen > 0 {
			h = le.AppendUint16(h, 0x0001)
			h = le.AppendUint16(h, uint16(extraLen-4))
			if e.size >= u32max {
				h = le.AppendUint64(h, uint64(e.size))
				h = le.AppendUint64(h, uint64(e.size))
			}
			if e.offset >= u32max {
				h = le.AppendUint64(h, uint64(e.offset))
			}
		}
		if _, err := cw.Write(h); err != nil {
			return err
		}
	}
	cdSize := cw.n - cdStart
	n := len(entries)

	if needsZip64End(n, cdStart, cdSize) {
		z64 := cw.n
		r := make([]byte, 0, 76)
		r = le.AppendUint32(r, 0x06064b50)
		r = le.AppendUint64(r, 44)
		r = le.AppendUint16(r, 45)
		r = le.AppendUint16(r, 45)
		r = le.AppendUint32(r, 0)
		r = le.AppendUint32(r, 0)
		r = le.AppendUint64(r, uint64(n))
		r = le.AppendUint64(r, uint64(n))
		r = le.AppendUint64(r, uint64(cdSize))
		r = le.AppendUint64(r, uint64(cdStart))
		// localizador
		r = le.AppendUint32(r, 0x07064b50)
		r = le.AppendUint32(r, 0)
		r = le.AppendUint64(r, uint64(z64))
		r = le.AppendUint32(r, 1)
		if _, err := cw.Write(r); err != nil {
			return err
		}
	}

	clamp16 := func(v int) uint16 {
		if v >= u16max {
			return u16max
		}
		return uint16(v)
	}
	clamp32 := func(v int64) uint32 {
		if v >= u32max {
			return u32max
		}
		return uint32(v)
	}
	end := make([]byte, 0, 22)
	end = le.AppendUint32(end, 0x06054b50)
	end = le.AppendUint16(end, 0)
	end = le.AppendUint16(end, 0)
	end = le.AppendUint16(end, clamp16(n))
	end = le.AppendUint16(end, clamp16(n))
	end = le.AppendUint32(end, clamp32(cdSize))
	end = le.AppendUint32(end, clamp32(cdStart))
	end = le.AppendUint16(end, 0)
	_, err := cw.Write(end)
	return err
}
