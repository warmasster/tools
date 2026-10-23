// Package vault guarda datos secretos (contraseñas, correos, códigos de
// recuperación) cifrados en un archivo local con una contraseña maestra.
//
// La clave se saca de la contraseña maestra con Argon2id (64 MB de memoria,
// 3 pasadas), así que probar contraseñas a lo bruto es muy lento. Los datos van
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
	// MinMaster es la longitud mínima de la contraseña maestra.
	MinMaster = 8
)

var (
	ErrLocked    = errors.New("la bóveda está cerrada")
	ErrNoVault   = errors.New("todavía no hay bóveda")
	ErrExists    = errors.New("ya hay una bóveda")
	ErrWrong     = errors.New("contraseña maestra incorrecta")
	ErrShort     = errors.New("la contraseña maestra debe tener al menos 8 caracteres")
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
	if err := json.Unmarshal(b, &f); err != nil || f.V != fileVersion || f.KDF.Alg != "argon2id" || len(f.KDF.Salt) < 16 || len(f.Nonce) != 12 {
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
	return kdfParams{Alg: "argon2id", Salt: salt, Time: 3, Memory: 64 * 1024, Threads: uint8(th)}
}

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
	defer v.mu.Unlock()
	if v.key != nil {
		wipe(v.key)
	}
	v.key, v.data = key, data
	return nil
}

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
	return nil
}

// Destroy borra la bóveda entera (pide la contraseña maestra para hacerlo).
func (v *Vault) Destroy(master string) error {
	v.mu.Lock()
	f := v.file
	v.mu.Unlock()
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
		_ = os.WriteFile(v.path+".bak", old, 0o600)
	}
	tmp := v.path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, v.path); err != nil {
		os.Remove(tmp)
		return err
	}
	v.file, v.err = f, nil
	return nil
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
