package main

import (
	"context"
	"sync"
	"sync/atomic"
)

// jpegHub reparte el último JPEG a quien lo esté mirando (vistas previas de la
// ventana). Nadie espera a nadie: quien llega tarde se salta fotogramas.
type jpegHub struct {
	mu    sync.Mutex
	data  []byte
	seq   uint64
	ready chan struct{} // se cierra al publicar
	subs  atomic.Int32
}

func newJPEGHub() *jpegHub { return &jpegHub{ready: make(chan struct{})} }

func (h *jpegHub) publish(b []byte) {
	if len(b) == 0 {
		return
	}
	h.mu.Lock()
	h.data = b
	h.seq++
	close(h.ready)
	h.ready = make(chan struct{})
	h.mu.Unlock()
}

// watching dice si alguien está mirando (para no codificar en balde).
func (h *jpegHub) watching() bool { return h.subs.Load() > 0 }

func (h *jpegHub) latest() []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.data
}

// next espera un fotograma posterior a last.
func (h *jpegHub) next(ctx context.Context, last uint64) ([]byte, uint64, bool) {
	for {
		h.mu.Lock()
		if h.seq != last && h.data != nil {
			d, s := h.data, h.seq
			h.mu.Unlock()
			return d, s, true
		}
		ready := h.ready
		h.mu.Unlock()
		select {
		case <-ready:
		case <-ctx.Done():
			return nil, last, false
		}
	}
}
