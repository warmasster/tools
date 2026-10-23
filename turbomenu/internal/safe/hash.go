package safe

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"os"
	"sync"
	"time"
)

// Hashes son las huellas del archivo en hexadecimal (minúsculas).
type Hashes struct {
	SHA256 string `json:"sha256"`
	SHA1   string `json:"sha1"`
	MD5    string `json:"md5"`
	Size   int64  `json:"size"`
}

// Progress recibe cuánto se ha procesado de un total (bytes).
type Progress func(done, total int64)

const hashChunk = 4 << 20 // 4 MB por lectura

// HashFile calcula SHA-256, SHA-1 y MD5 en una sola lectura del archivo.
// Lee y calcula en paralelo; progress (opcional) se llama unas pocas veces por segundo.
func HashFile(ctx context.Context, path string, progress Progress) (*Hashes, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errOpen(err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, errOpen(err)
	}
	if st.IsDir() {
		return nil, errors.New("Es una carpeta, no un archivo")
	}
	total := st.Size()

	hs := []hash.Hash{sha256.New(), sha1.New(), md5.New()}
	// Un trabajador por algoritmo; cada bloque se reparte a los tres.
	work := make([]chan []byte, len(hs))
	var wg sync.WaitGroup
	for i := range hs {
		work[i] = make(chan []byte)
		go func(h hash.Hash, in chan []byte) {
			for b := range in {
				h.Write(b)
				wg.Done()
			}
		}(hs[i], work[i])
	}
	defer func() {
		for _, c := range work {
			close(c)
		}
	}()

	// Doble búfer: mientras se calcula un bloque se lee el siguiente.
	bufs := [2][]byte{make([]byte, hashChunk), make([]byte, hashChunk)}
	var done int64
	last := time.Now()
	if progress != nil {
		progress(0, total)
	}
	for i := 0; ; i ^= 1 {
		if err := ctx.Err(); err != nil {
			wg.Wait()
			return nil, err
		}
		n, rerr := io.ReadFull(f, bufs[i])
		wg.Wait() // el bloque anterior ya está calculado
		if n > 0 {
			wg.Add(len(hs))
			for _, c := range work {
				c <- bufs[i][:n]
			}
			done += int64(n)
			if progress != nil && time.Since(last) > 100*time.Millisecond {
				last = time.Now()
				progress(done, total)
			}
		}
		if rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
			break
		}
		if rerr != nil {
			wg.Wait()
			return nil, errors.New("No se pudo leer el archivo")
		}
	}
	wg.Wait()
	if progress != nil {
		progress(done, total)
	}
	return &Hashes{
		SHA256: hex.EncodeToString(hs[0].Sum(nil)),
		SHA1:   hex.EncodeToString(hs[1].Sum(nil)),
		MD5:    hex.EncodeToString(hs[2].Sum(nil)),
		Size:   done,
	}, nil
}

// errOpen traduce los fallos al abrir un archivo a un mensaje corto.
func errOpen(err error) error {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return errors.New("El archivo ya no existe")
	case errors.Is(err, os.ErrPermission):
		return errors.New("No hay permiso para leer el archivo")
	}
	return errors.New("No se pudo abrir el archivo (puede que esté en uso)")
}
