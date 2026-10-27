// Package vault guarda datos secretos (contraseñas, correos, códigos de
// recuperación) cifrados en un archivo local con una contraseña maestra.
//
// La clave se saca de la contraseña maestra con Argon2id (256 MB de memoria,
// 6 pasadas, como TurboVAULT), así que probar contraseñas a lo bruto es muy lento.
// Las bóvedas creadas con los parámetros de antes (64 MB, 3 pasadas) se siguen
// abriendo y se refuerzan solas la primera vez que se abren. Los datos van
// cifrados y autenticados con AES-256-GCM: si alguien cambia un solo byte del
// archivo, no se abre. La contraseña maestra no se guarda en ningún sitio y la
// clave solo está en memoria mientras la bóveda está abierta.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	fileVersion = 1
	aadText     = "TurboGAMES boveda v1"
	// MinMaster es la longitud mínima de una contraseña maestra nueva (al crear o
	// cambiarla). Las de antes, de 8, siguen abriendo su bóveda.
	MinMaster = 12
)

// defaultKDF: los parámetros de las bóvedas nuevas (las pruebas los bajan).
var defaultKDF = kdfParams{Time: 6, Memory: 256 * 1024}

var (
	ErrLocked    = errors.New("la bóveda está cerrada")
	ErrNoVault   = errors.New("todavía no hay bóveda")
	ErrExists    = errors.New("ya hay una bóveda")
	ErrWrong     = errors.New("contraseña maestra incorrecta")
	ErrShort     = fmt.Errorf("la contraseña maestra debe tener al menos %d caracteres", MinMaster)
	ErrCorrupted = errors.New("el archivo de la bóveda está dañado")
)

// Entry son los secretos de una cuenta.
type Entry struct {
	Password      string `json:"password,omitempty"`
	Email         string `json:"email,omitempty"`
	EmailPassword string `json:"emailPassword,omitempty"`
	Recovery      string `json:"recovery,omitempty"` // código de recuperación de Steam Guard (R12345)
	Notes         string `json:"notes,omitempty"`
	Updated       int64  `json:"updated"`
}

// Empty dice si no hay nada guardado.
func (e Entry) Empty() bool {
	return e.Password == "" && e.Email == "" && e.EmailPassword == "" && e.Recovery == "" && e.Notes == ""
}

type kdfParams struct {
	Alg     string `json:"alg"`
	Salt    []byte `json:"salt"`
	Time    uint32 `json:"t"`
	Memory  uint32 `json:"m"` // KiB
	Threads uint8  `json:"p"`
}

type fileData struct {
	V     int       `json:"v"`
	KDF   kdfParams `json:"kdf"`
	Nonce []byte    `json:"nonce"`
	Data  []byte    `json:"data"`
}

type payload struct {
	Created int64            `json:"created"`
	Entries map[string]Entry `json:"entries"`
}

// Vault es la bóveda. Se puede usar desde varias goroutines.
type Vault struct {
	mu   sync.Mutex
	path string
	file *fileData
	key  []byte
	data *payload
	err  error // error al leer el archivo (dañado)

	upgrading sync.WaitGroup
}

// Open prepara la bóveda del archivo dado. No pide la contraseña: solo mira si
// existe.
func Open(path string) *Vault {
	v := &Vault{path: path}
	v.reload()
	return v
}

func (v *Vault) reload() {
	v.file, v.err = nil, nil
	b, err := os.ReadFile(v.path)
	if err != nil {
		if !os.IsNotExist(err) {
			v.err = err
		}
		return
	}
	var f fileData
	if err := json.Unmarshal(b, &f); err != nil || f.V != fileVersion || f.KDF.Alg != "argon2id" || len(f.KDF.Salt) < 16 || len(f.Nonce) != 12 ||
		// Parámetros fuera de rango: con t=0 Argon2 falla y con m enorme se queda sin memoria.
		f.KDF.Time < 1 || f.KDF.Time > 10 || f.KDF.Memory < 8*1024 || f.KDF.Memory > 1024*1024 || f.KDF.Threads < 1 || f.KDF.Threads > 16 {
		v.err = ErrCorrupted
		return
	}
	v.file = &f
}

// Exists dice si ya se ha creado la bóveda.
func (v *Vault) Exists() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.file != nil
}

// Damaged devuelve el error si el archivo existe pero no se puede leer.
func (v *Vault) Damaged() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.err
}

// Unlocked dice si está abierta.
func (v *Vault) Unlocked() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.key != nil
}

func newParams() kdfParams {
	th := runtime.NumCPU()
	if th > 4 {
		th = 4
	}
	if th < 1 {
		th = 1
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		panic(err)
	}
	return kdfParams{Alg: "argon2id", Salt: salt, Time: defaultKDF.Time, Memory: defaultKDF.Memory, Threads: uint8(th)}
}

// weak: la bóveda se creó con parámetros más flojos que los de ahora.
func weak(p kdfParams) bool { return p.Time < defaultKDF.Time || p.Memory < defaultKDF.Memory }

func derive(master string, p kdfParams) []byte {
	return argon2.IDKey([]byte(master), p.Salt, p.Time, p.Memory, p.Threads, 32)
}

func checkMaster(master string) error {
	if utf8.RuneCountInString(master) < MinMaster {
		return ErrShort
	}
	return nil
}

// Create crea una bóveda vacía con esa contraseña maestra y la deja abierta.
func (v *Vault) Create(master string) error {
	if err := checkMaster(master); err != nil {
		return err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.file != nil {
		return ErrExists
	}
	// Un archivo que ahora no se puede leer no se pisa con una bóveda vacía (ni su
	// .bak, que es la versión buena anterior).
	if _, err := os.Stat(v.path); v.err != nil || err == nil {
		return errors.New("ya hay una bóveda en este PC aunque ahora no se puede leer: no se crea otra encima")
	}
	p := newParams()
	key := derive(master, p)
	data := &payload{Created: time.Now().Unix(), Entries: map[string]Entry{}}
	if err := v.save(key, p, data); err != nil {
		wipe(key)
		return err
	}
	v.key, v.data = key, data
	return nil
}

// Unlock abre la bóveda. Tarda un momento a propósito (Argon2id).
func (v *Vault) Unlock(master string) error {
	v.mu.Lock()
	f := v.file
	v.mu.Unlock()
	if f == nil {
		if err := v.Damaged(); err != nil {
			return err
		}
		return ErrNoVault
	}
	key := derive(master, f.KDF) // sin el cerrojo: es lento
	data, err := decrypt(key, f)
	if err != nil {
		wipe(key)
		return err
	}
	v.mu.Lock()
	if v.key != nil {
		wipe(v.key)
	}
	v.key, v.data = key, data
	v.mu.Unlock()
	if weak(f.KDF) {
		// Bóveda de antes: se vuelve a cifrar con los parámetros de ahora. Va aparte
		// para que abrir no tarde el doble; si falla, sigue como estaba.
		v.upgrading.Add(1)
		go func() {
			defer v.upgrading.Done()
			v.strengthen(master, f)
		}()
	}
	return nil
}

// strengthen vuelve a cifrar la bóveda (mismos datos, misma maestra) con sal y
// parámetros nuevos. No hace nada si mientras tanto se ha cerrado o cambiado.
func (v *Vault) strengthen(master string, was *fileData) {
	p := newParams()
	key := derive(master, p)
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil || v.file != was && !weak(v.file.KDF) {
		wipe(key)
		return
	}
	if err := v.save(key, p, v.data); err != nil {
		wipe(key)
		return
	}
	wipe(v.key)
	v.key = key
	// La copia .bak era la versión floja: pasa a ser la reforzada.
	if b, err := os.ReadFile(v.path); err == nil {
		writeSynced(v.path+".bak", b)
	}
}

// WaitUpgrade espera a que termine el refuerzo de una bóveda vieja (pruebas).
func (v *Vault) WaitUpgrade() { v.upgrading.Wait() }

func decrypt(key []byte, f *fileData) (*payload, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, f.Nonce, f.Data, []byte(aadText))
	if err != nil {
		return nil, ErrWrong
	}
	defer wipe(plain)
	var p payload
	if err := json.Unmarshal(plain, &p); err != nil {
		return nil, ErrCorrupted
	}
	if p.Entries == nil {
		p.Entries = map[string]Entry{}
	}
	return &p, nil
}

// Lock cierra la bóveda y borra la clave de la memoria.
func (v *Vault) Lock() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key != nil {
		wipe(v.key)
	}
	v.key, v.data = nil, nil
}

// Get devuelve los secretos de una cuenta.
func (v *Vault) Get(name string) (Entry, bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil {
		return Entry{}, false, ErrLocked
	}
	e, ok := v.data.Entries[name]
	return e, ok, nil
}

// Names devuelve las cuentas que tienen algo guardado.
func (v *Vault) Names() ([]string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil {
		return nil, ErrLocked
	}
	out := make([]string, 0, len(v.data.Entries))
	for k := range v.data.Entries {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

// Set guarda (o borra, si está vacío) los secretos de una cuenta.
func (v *Vault) Set(name string, e Entry) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil {
		return ErrLocked
	}
	next := clonePayload(v.data)
	if e.Empty() {
		delete(next.Entries, name)
	} else {
		e.Updated = time.Now().Unix()
		next.Entries[name] = e
	}
	if err := v.save(v.key, v.file.KDF, next); err != nil {
		return err
	}
	v.data = next
	return nil
}

// Rename mueve los secretos de una cuenta a otro nombre.
func (v *Vault) Rename(from, to string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil {
		return ErrLocked
	}
	e, ok := v.data.Entries[from]
	if !ok || from == to {
		return nil
	}
	next := clonePayload(v.data)
	delete(next.Entries, from)
	next.Entries[to] = e
	if err := v.save(v.key, v.file.KDF, next); err != nil {
		return err
	}
	v.data = next
	return nil
}

// ChangeMaster cambia la contraseña maestra (con sal nueva).
func (v *Vault) ChangeMaster(old, newMaster string) error {
	if err := checkMaster(newMaster); err != nil {
		return err
	}
	v.mu.Lock()
	f := v.file
	v.mu.Unlock()
	if f == nil {
		return ErrNoVault
	}
	oldKey := derive(old, f.KDF)
	data, err := decrypt(oldKey, f)
	wipe(oldKey)
	if err != nil {
		return err
	}
	p := newParams()
	key := derive(newMaster, p)
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.save(key, p, data); err != nil {
		wipe(key)
		return err
	}
	if v.key != nil {
		wipe(v.key)
	}
	v.key, v.data = key, data
	// El .bak guardaba la versión con la maestra vieja: si se cambió porque alguien
	// la vio, esa copia seguiría abriéndose con ella. Pasa a ser la nueva.
	if b, err := os.ReadFile(v.path); err == nil {
		writeSynced(v.path+".bak", b)
	}
	return nil
}

// Destroy borra la bóveda entera (pide la contraseña maestra para hacerlo).
func (v *Vault) Destroy(master string) error {
	v.mu.Lock()
	f, damaged := v.file, v.err
	v.mu.Unlock()
	if f == nil && damaged != nil {
		// Sin poder leerla no se puede comprobar la maestra: no se borra.
		return errors.New("la bóveda no se puede leer, así que no se puede comprobar la contraseña maestra: no se borra (el archivo es " + v.path + ")")
	}
	if f != nil {
		key := derive(master, f.KDF)
		_, err := decrypt(key, f)
		wipe(key)
		if err != nil {
			return err
		}
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key != nil {
		wipe(v.key)
	}
	v.key, v.data, v.file, v.err = nil, nil, nil, nil
	os.Remove(v.path + ".bak")
	if err := os.Remove(v.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// save cifra y escribe el archivo. Antes guarda la versión anterior en .bak por
// si el disco falla a mitad.
func (v *Vault) save(key []byte, p kdfParams, data *payload) error {
	plain, err := json.Marshal(data)
	if err != nil {
		return err
	}
	defer wipe(plain)
	gcm, err := newGCM(key)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	f := &fileData{V: fileVersion, KDF: p, Nonce: nonce, Data: gcm.Seal(nil, nonce, plain, []byte(aadText))}
	out, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(v.path), 0o700); err != nil {
		return err
	}
	if old, err := os.ReadFile(v.path); err == nil {
		writeSynced(v.path+".bak", old)
	}
	if err := writeSynced(v.path, out); err != nil {
		return err
	}
	v.file, v.err = f, nil
	return nil
}

// writeSynced escribe en un temporal, lo fuerza al disco y lo renombra: si se va la
// luz a mitad, queda el archivo anterior entero (sin el Sync, NTFS podía dejar el
// nuevo vacío tras el renombrado).
func writeSynced(path string, data []byte) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func clonePayload(p *payload) *payload {
	n := &payload{Created: p.Created, Entries: make(map[string]Entry, len(p.Entries)+1)}
	for k, e := range p.Entries {
		n.Entries[k] = e
	}
	return n
}

func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
