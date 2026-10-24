// Package secmem guarda claves en memoria que no se va al archivo de paginación
// (VirtualLock) y que, mientras no se usa, está cifrada con CryptProtectMemory:
// un volcado de memoria o el archivo de hibernación no la enseñan en claro.
// La clave solo se descifra durante el instante en que se usa.
package secmem

import (
	"errors"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	blockSize       = 16 // CRYPTPROTECTMEMORY_BLOCK_SIZE
	sameProcess     = 0  // CRYPTPROTECTMEMORY_SAME_PROCESS
	pageSize        = 4096
	maxSecretLength = pageSize
)

var (
	crypt32         = windows.NewLazySystemDLL("crypt32.dll")
	procProtect     = crypt32.NewProc("CryptProtectMemory")
	procUnprotect   = crypt32.NewProc("CryptUnprotectMemory")
	errTooLong      = errors.New("secmem: secreto demasiado largo")
	ErrDestroyed    = errors.New("secmem: la clave ya se ha borrado")
	errProtectFails = errors.New("secmem: Windows no pudo proteger la memoria")
)

// Key es un secreto pequeño (una clave) protegido en memoria.
type Key struct {
	mu   sync.Mutex
	addr uintptr
	buf  []byte // vista sobre la página reservada (longitud múltiplo de 16)
	n    int    // longitud real del secreto
}

// New copia k a memoria protegida y borra k.
func New(k []byte) (*Key, error) {
	defer Wipe(k)
	if len(k) == 0 || len(k) > maxSecretLength {
		return nil, errTooLong
	}
	addr, err := windows.VirtualAlloc(0, pageSize, windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_READWRITE)
	if err != nil {
		return nil, err
	}
	// Si no se puede bloquear (límite del proceso) se sigue: el cifrado en memoria
	// ya protege lo importante.
	_ = windows.VirtualLock(addr, pageSize)
	size := (len(k) + blockSize - 1) / blockSize * blockSize
	// La página no la gestiona Go: se lee la dirección como puntero sin
	// convertir un uintptr (go vet lo daría por sospechoso).
	ptr := *(*unsafe.Pointer)(unsafe.Pointer(&addr))
	buf := unsafe.Slice((*byte)(ptr), size)
	copy(buf, k)
	key := &Key{addr: addr, buf: buf, n: len(k)}
	if err := key.protect(); err != nil {
		key.free()
		return nil, err
	}
	return key, nil
}

func (k *Key) protect() error {
	r, _, _ := procProtect.Call(uintptr(unsafe.Pointer(&k.buf[0])), uintptr(len(k.buf)), sameProcess)
	if r == 0 {
		return errProtectFails
	}
	return nil
}

func (k *Key) unprotect() error {
	r, _, _ := procUnprotect.Call(uintptr(unsafe.Pointer(&k.buf[0])), uintptr(len(k.buf)), sameProcess)
	if r == 0 {
		return errProtectFails
	}
	return nil
}

// Use descifra la clave, llama a fn con ella y la vuelve a cifrar. fn no debe
// guardarse el slice: deja de ser válido al volver.
func (k *Key) Use(fn func(key []byte) error) error {
	if k == nil {
		return ErrDestroyed
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.addr == 0 {
		return ErrDestroyed
	}
	if err := k.unprotect(); err != nil {
		return err
	}
	defer k.protect()
	return fn(k.buf[:k.n])
}

// Destroy borra la clave y libera la memoria.
func (k *Key) Destroy() {
	if k == nil {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.free()
}

func (k *Key) free() {
	if k.addr == 0 {
		return
	}
	Wipe(k.buf)
	windows.VirtualUnlock(k.addr, pageSize)
	windows.VirtualFree(k.addr, 0, windows.MEM_RELEASE)
	k.addr, k.buf, k.n = 0, nil, 0
}

// Wipe pone a cero un slice.
func Wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
