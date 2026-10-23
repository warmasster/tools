// Package docs reduce y convierte imágenes, PDF y documentos de Office.
// El archivo original nunca se modifica: todo se escribe en archivos nuevos.
package docs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Progress recibe el avance de una operación larga, de 0 a 1. Puede ser nil.
type Progress func(frac float64)

func (p Progress) set(f float64) {
	if p == nil {
		return
	}
	if f < 0 {
		f = 0
	} else if f > 1 {
		f = 1
	}
	p(f)
}

// sub devuelve un Progress que mapea [0,1] al tramo [a,b] de p.
func (p Progress) sub(a, b float64) Progress {
	if p == nil {
		return nil
	}
	return func(f float64) { p.set(a + (b-a)*f) }
}

// Result describe el archivo generado por una operación.
type Result struct {
	Out    string `json:"out"`    // ruta del archivo nuevo ("" si no se guardó)
	Before int64  `json:"before"` // bytes del original (o suma de originales)
	After  int64  `json:"after"`  // bytes del resultado
	Larger bool   `json:"larger"` // el resultado pesaría lo mismo o más que el original
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

// Errores que la interfaz puede reconocer con errors.Is.
var (
	// ErrNotSmaller indica que no se ha podido reducir: el resultado pesaría igual o más.
	ErrNotSmaller = errors.New("no se ha podido reducir más: el archivo ya está optimizado")
	// ErrCanceled indica que el usuario canceló la operación.
	ErrCanceled = errors.New("operación cancelada")
	// ErrSuperseded indica que una vista previa quedó anticuada por otra más reciente.
	ErrSuperseded = errors.New("vista previa sustituida por otra más reciente")
)

// userErr es un error con mensaje para el usuario y causa técnica opcional.
type userErr struct {
	msg   string
	cause error
}

func (e *userErr) Error() string { return e.msg }
func (e *userErr) Unwrap() error { return e.cause }

func uerr(cause error, format string, a ...any) error {
	return &userErr{msg: fmt.Sprintf(format, a...), cause: cause}
}

// ctxErr traduce la cancelación del contexto a ErrCanceled.
func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return ErrCanceled
	}
	return nil
}

// cleanErr deja un error presentable: cancelación, mensajes propios o uno genérico.
func cleanErr(ctx context.Context, err error, generic string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrCanceled) || errors.Is(err, context.Canceled) || (ctx != nil && ctx.Err() != nil) {
		return ErrCanceled
	}
	var ue *userErr
	if errors.As(err, &ue) || errors.Is(err, ErrNotSmaller) {
		return err
	}
	if errors.Is(err, os.ErrNotExist) {
		return uerr(err, "No se encuentra el archivo")
	}
	if errors.Is(err, os.ErrPermission) {
		return uerr(err, "Windows no deja acceder al archivo (¿está abierto en otro programa?)")
	}
	return uerr(err, "%s", generic)
}

// UniquePath devuelve una ruta libre junto a in: «foto (reducida).jpg»,
// «foto (reducida 2).jpg»... Con label vacío: «foto.png», «foto (2).png».
// ext lleva el punto (".jpg"); vacío conserva la extensión de in.
func UniquePath(in, label, ext string) string {
	base := filepath.Base(in)
	oldExt := filepath.Ext(base)
	if ext == "" {
		ext = oldExt
	}
	return uniqueName(filepath.Dir(in), strings.TrimSuffix(base, oldExt), label, ext, in)
}

// uniqueDir devuelve una carpeta libre junto a in: «doc (páginas)», «doc (páginas 2)».
func uniqueDir(in, label string) string {
	base := filepath.Base(in)
	return uniqueName(filepath.Dir(in), strings.TrimSuffix(base, filepath.Ext(base)), label, "", in)
}

func uniqueName(dir, name, label, ext, skip string) string {
	for i := 1; ; i++ {
		var n string
		switch {
		case label == "" && i == 1:
			n = name + ext
		case label == "":
			n = fmt.Sprintf("%s (%d)%s", name, i, ext)
		case i == 1:
			n = fmt.Sprintf("%s (%s)%s", name, label, ext)
		default:
			n = fmt.Sprintf("%s (%s %d)%s", name, label, i, ext)
		}
		p := filepath.Join(dir, n)
		if strings.EqualFold(p, skip) {
			continue
		}
		if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
			return p
		}
	}
}

// reserveMu evita que dos operaciones del mismo proceso elijan el mismo nombre.
var reserveMu sync.Mutex

// outFile escribe un archivo nuevo sin pisar nada: reserva el nombre, escribe
// en un temporal de la misma carpeta y lo coloca al final.
type outFile struct {
	Final string
	Tmp   string
	f     *os.File
}

// createOut reserva un nombre libre (UniquePath) y abre un temporal para escribir.
func createOut(in, label, ext string) (*outFile, error) {
	reserveMu.Lock()
	defer reserveMu.Unlock()
	for range 50 {
		final := UniquePath(in, label, ext)
		ph, err := os.OpenFile(final, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return nil, cleanErr(nil, err, "No se puede crear el archivo de salida")
		}
		ph.Close()
		return openTmpFor(final)
	}
	return nil, uerr(nil, "No se puede crear el archivo de salida")
}

// createOutAt usa una ruta final ya elegida (debe no existir o estar reservada).
func createOutAt(final string) (*outFile, error) {
	reserveMu.Lock()
	defer reserveMu.Unlock()
	if _, err := os.Lstat(final); errors.Is(err, os.ErrNotExist) {
		ph, err := os.OpenFile(final, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, cleanErr(nil, err, "No se puede crear el archivo de salida")
		}
		ph.Close()
	}
	return openTmpFor(final)
}

func openTmpFor(final string) (*outFile, error) {
	f, err := os.CreateTemp(filepath.Dir(final), ".turbo-*.tmp")
	if err != nil {
		os.Remove(final)
		return nil, cleanErr(nil, err, "No se puede escribir en esta carpeta")
	}
	return &outFile{Final: final, Tmp: f.Name(), f: f}, nil
}

func (o *outFile) Write(p []byte) (int, error) { return o.f.Write(p) }

// File devuelve el archivo temporal abierto.
func (o *outFile) File() *os.File { return o.f }

// Commit cierra el temporal y lo coloca en su sitio. Devuelve el tamaño final.
func (o *outFile) Commit() (int64, error) {
	if err := o.f.Sync(); err != nil {
		o.Abort()
		return 0, cleanErr(nil, err, "No se ha podido guardar el archivo")
	}
	st, _ := o.f.Stat()
	if err := o.f.Close(); err != nil {
		o.Abort()
		return 0, cleanErr(nil, err, "No se ha podido guardar el archivo")
	}
	if err := os.Rename(o.Tmp, o.Final); err != nil {
		o.Abort()
		return 0, cleanErr(nil, err, "No se ha podido guardar el archivo")
	}
	if st == nil {
		return 0, nil
	}
	return st.Size(), nil
}

// Abort borra el temporal y la reserva.
func (o *outFile) Abort() {
	if o.f != nil {
		o.f.Close()
	}
	os.Remove(o.Tmp)
	os.Remove(o.Final)
}

// writeOut guarda data en un archivo nuevo junto a in.
func writeOut(in, label, ext string, data []byte) (string, error) {
	o, err := createOut(in, label, ext)
	if err != nil {
		return "", err
	}
	if _, err := o.Write(data); err != nil {
		o.Abort()
		return "", cleanErr(nil, err, "No se ha podido guardar el archivo")
	}
	if _, err := o.Commit(); err != nil {
		return "", err
	}
	return o.Final, nil
}

func fileSize(p string) int64 {
	st, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return st.Size()
}

// lowerExt devuelve la extensión en minúsculas sin el punto.
func lowerExt(p string) string {
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(p)), ".")
}
