// Package fps mide los fotogramas por segundo de cualquier juego escuchando los
// eventos que Windows ya emite cada vez que un programa presenta un fotograma
// (ETW: DXGI para DirectX 10/11/12, D3D9 y, para Vulkan y OpenGL, el núcleo
// gráfico). Es lo mismo que hace PresentMon: no se inyecta nada en el juego, así
// que no hay problemas con los antitrampas.
//
// Escuchar estos eventos necesita permiso de administrador.
package fps

import (
	"errors"

	"golang.org/x/sys/windows"

	"turbomon/internal/etw"
)

// Fuentes de un fotograma.
const (
	SrcDXGI   = 1
	SrcD3D9   = 2
	SrcKernel = 3
)

var (
	guidDXGI    = windows.GUID{Data1: 0xCA11C036, Data2: 0x0102, Data3: 0x4A2D, Data4: [8]byte{0xA6, 0xAD, 0xF0, 0x3C, 0xFE, 0xD5, 0xD3, 0xC9}}
	guidD3D9    = windows.GUID{Data1: 0x783ACA0A, Data2: 0x790E, Data3: 0x4D7F, Data4: [8]byte{0x84, 0x51, 0xAA, 0x85, 0x05, 0x11, 0xC6, 0xB9}}
	guidDxgKrnl = windows.GUID{Data1: 0x802EC45A, Data2: 0x1E99, Data3: 0x4B83, Data4: [8]byte{0x99, 0x20, 0x87, 0xC9, 0x82, 0x77, 0xBA, 0x9D}}
)

const (
	levelInfo        = 4
	kwDxgKrnlBase    = 0x1
	kwDxgKrnlPresent = 0x8000000
)

// Now es la marca de tiempo actual, en el mismo reloj que los eventos.
func Now() int64 { return etw.Now() }

// Freq es cuántas marcas hay en un segundo.
func Freq() int64 { return etw.Freq() }

// ErrNeedAdmin: Windows solo deja escuchar estos eventos a un administrador.
var ErrNeedAdmin = errors.New("medir los FPS necesita permiso de administrador")

// Session es la sesión de eventos de los fotogramas. Se le pueden añadir más
// proveedores (red, disco) con Events().
type Session struct{ s *etw.Session }

// Start empieza a escuchar en una sesión con ese nombre. onPresent se llama
// (desde otro hilo) con cada fotograma: proceso, marca de tiempo y fuente.
func Start(name string, onPresent func(pid uint32, ts int64, src int)) (*Session, error) {
	s, err := etw.Start(name, etw.Options{})
	if err != nil {
		if errors.Is(err, etw.ErrNeedAdmin) {
			return nil, ErrNeedAdmin
		}
		return nil, err
	}
	err = s.Enable(guidDXGI, levelInfo, ^uint64(0), nil, func(r *etw.Record) {
		if id := r.Header.ID; id == 42 || id == 55 { // Present_Start, PresentMultiplaneOverlay_Start
			onPresent(r.Header.ProcessID, r.Header.TimeStamp, SrcDXGI)
		}
	})
	if err != nil {
		s.Stop()
		return nil, err
	}
	s.Enable(guidD3D9, levelInfo, ^uint64(0), nil, func(r *etw.Record) {
		if r.Header.ID == 1 { // Present_Start
			onPresent(r.Header.ProcessID, r.Header.TimeStamp, SrcD3D9)
		}
	})
	s.Enable(guidDxgKrnl, levelInfo, kwDxgKrnlBase|kwDxgKrnlPresent, nil, func(r *etw.Record) {
		switch r.Header.ID {
		case 184, 168, 252: // Present_Info, Flip_Info, FlipMultiPlaneOverlay_Info
			onPresent(r.Header.ProcessID, r.Header.TimeStamp, SrcKernel)
		}
	})
	return &Session{s: s}, nil
}

// Events devuelve la sesión de eventos, para escuchar más proveedores en ella.
func (s *Session) Events() *etw.Session {
	if s == nil {
		return nil
	}
	return s.s
}

// Stop para la sesión.
func (s *Session) Stop() {
	if s != nil {
		s.s.Stop()
	}
}

// EventsLost dice cuántos eventos se han perdido.
func (s *Session) EventsLost() uint32 {
	if s == nil {
		return 0
	}
	return s.s.EventsLost()
}
