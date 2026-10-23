package media

import (
	"context"
	"errors"
	"strings"
)

// Error es un fallo con un mensaje para el usuario (Error) y el detalle
// técnico aparte (Detail), útil para un registro.
type Error struct {
	Msg    string
	Detail string
}

func (e *Error) Error() string { return e.Msg }

// ErrNotSmaller: el resultado no pesaría menos que el original (ya estaba muy
// comprimido). No se deja ningún archivo.
var ErrNotSmaller = &Error{Msg: "Este archivo ya está muy comprimido: con esta opción no pesaría menos."}

// ErrNoFFmpeg: falta ffmpeg o ffprobe.
var ErrNoFFmpeg = &Error{Msg: "Falta FFmpeg. Descárgalo para usar las funciones de vídeo y audio."}

func userErr(msg string, cause error) error {
	e := &Error{Msg: msg}
	if cause != nil {
		e.Detail = cause.Error()
	}
	return e
}

func diskErr(err error) error {
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "not enough space") || strings.Contains(s, "disk is full") || strings.Contains(s, "no space") {
		return userErr("No queda espacio en el disco.", err)
	}
	return userErr("No se pudo escribir el archivo.", err)
}

// ffmpegErr traduce el log de error de ffmpeg a un mensaje corto.
func ffmpegErr(log []byte, fallback string) error {
	detail := strings.TrimSpace(string(log))
	s := strings.ToLower(detail)
	msg := fallback
	switch {
	case strings.Contains(s, "no space left") || strings.Contains(s, "not enough space"):
		msg = "No queda espacio en el disco."
	case strings.Contains(s, "permission denied"):
		msg = "No se puede escribir ahí: la carpeta está protegida o el archivo está abierto."
	case strings.Contains(s, "no such file"):
		msg = "No se encuentra el archivo."
	case strings.Contains(s, "invalid data found"), strings.Contains(s, "moov atom not found"),
		strings.Contains(s, "could not find codec parameters"), strings.Contains(s, "error while decoding"):
		msg = "No se pudo leer el archivo: puede estar dañado o no ser compatible."
	case strings.Contains(s, "does not contain any stream"), strings.Contains(s, "matches no streams"):
		msg = "El archivo no tiene vídeo ni audio que se pueda usar."
	}
	return &Error{Msg: msg, Detail: detail}
}

// IsCanceled dice si el error viene de cancelar la operación (el contexto).
func IsCanceled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
