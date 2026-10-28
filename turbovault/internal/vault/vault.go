// Package vault es la bóveda de TurboVAULT: contraseñas, notas y códigos 2FA
// cifrados en un archivo local con una contraseña maestra.
//
// Cómo se protege:
//
//   - De la contraseña maestra sale una clave (KEK) con Argon2id, configurado
//     para ser caro (256 MB de memoria, 6 pasadas): cada intento a lo bruto
//     cuesta lo mismo, también en una tarjeta gráfica.
//   - Los datos van cifrados con una clave aleatoria de 256 bits (DEK), y la DEK
//     va cifrada con la KEK. Todo con AES-256-GCM, que además detecta cualquier
//     cambio en el archivo.
//   - Dentro, la parte secreta de cada entrada (contraseña, notas, 2FA, campos
//     ocultos, historial) va cifrada otra vez por separado: en memoria solo hay
//     títulos, webs y usuarios, y cada secreto se descifra en el momento en que
//     se pide.
//   - La DEK vive en memoria bloqueada y cifrada por Windows (paquete secmem) y
//     se borra al cerrar la bóveda. La contraseña maestra no se guarda nunca.
//   - Al cambiar la contraseña maestra se cambia también la DEK, así que una
//     copia antigua del archivo no sirve para leer los datos nuevos.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"

	"turbovault/internal/secmem"
)

const (
	formatName  = "TurboVAULT"
	fileVersion = 1
	// MinMaster es la longitud mínima de la contraseña maestra.
	MinMaster = 12
	// TrashDays es cuánto se guardan las entradas borradas antes de quitarlas.
	TrashDays = 30
	maxEvents = 400
	maxPwHist = 12
)

var (
	ErrLocked    = errors.New("la bóveda está cerrada")
	ErrNoVault   = errors.New("todavía no hay bóveda")
	ErrExists    = errors.New("ya hay una bóveda")
	ErrWrong     = errors.New("contraseña maestra incorrecta")
	ErrShort     = fmt.Errorf("la contraseña maestra debe tener al menos %d caracteres", MinMaster)
	ErrCorrupted = errors.New("el archivo de la bóveda está dañado o lo han modificado")
	ErrNotFound  = errors.New("esa entrada ya no existe")
	ErrFormat    = errors.New("no es un archivo de TurboVAULT")
)

// Params son los costes de Argon2id.
type Params struct {
	Time    uint32 `json:"t"`
	Memory  uint32 `json:"m"` // KiB
	Threads uint8  `json:"p"`
}

// DefaultParams: 256 MB por intento y unos 0,3 s en un PC rápido (1 s en uno
// modesto). Se nota poco al abrir, pero hace carísimo probar a lo bruto.
var DefaultParams = Params{Time: 6, Memory: 256 * 1024, Threads: 4}

// newParams se puede cambiar en las pruebas para que vayan rápidas.
var newParams = DefaultParams

// SetParamsForTests pone costes bajos (solo para pruebas).
func SetParamsForTests() { newParams = Params{Time: 1, Memory: 8 * 1024, Threads: 1} }

type kdfInfo struct {
	Alg  string `json:"alg"`
	Salt []byte `json:"salt"`
	Params
}

type box struct {
	N []byte `json:"n"`
	C []byte `json:"c"`
}

type fileData struct {
	Format string  `json:"format"`
	V      int     `json:"v"`
	KDF    kdfInfo `json:"kdf"`
	Key    box     `json:"key"`
	Data   box     `json:"data"`
}

// Tipos de entrada.
const (
	TypeLogin = "login"
	TypeNote  = "note"
)

// Item es la parte de una entrada que se puede enseñar en listas: no lleva
// ningún secreto.
type Item struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	URLs        []string `json:"urls,omitempty"`
	Username    string   `json:"username,omitempty"`
	Fav         bool     `json:"fav,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Created     int64    `json:"created"`
	Updated     int64    `json:"updated"`
	PwChanged   int64    `json:"pwChanged,omitempty"`
	Used        int64    `json:"used,omitempty"`
	Uses        int      `json:"uses,omitempty"`
	Deleted     int64    `json:"deleted,omitempty"`
	HasPassword bool     `json:"hasPassword,omitempty"`
	HasTOTP     bool     `json:"hasTotp,omitempty"`
	HasNotes    bool     `json:"hasNotes,omitempty"`
	Fields      int      `json:"fields,omitempty"`
	Score       int      `json:"score"` // fuerza de la contraseña 0-4, -1 si no hay
}

// Field es un campo extra (por ejemplo un PIN o una pregunta de seguridad).
type Field struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Hidden bool   `json:"hidden,omitempty"`
}

// OldPassword es una contraseña anterior de la entrada.
type OldPassword struct {
	Password string `json:"password"`
	Until    int64  `json:"until"`
}

// Secret es la parte secreta de una entrada.
type Secret struct {
	Password string        `json:"password,omitempty"`
	Notes    string        `json:"notes,omitempty"`
	TOTP     string        `json:"totp,omitempty"`
	Fields   []Field       `json:"fields,omitempty"`
	History  []OldPassword `json:"history,omitempty"`
}

func (s *Secret) wipe() {
	// Las cadenas de Go no se pueden borrar; al menos se sueltan cuanto antes.
	*s = Secret{}
}

// Event es una línea del registro de actividad.
type Event struct {
	T      int64  `json:"t"`
	Kind   string `json:"kind"`
	Item   string `json:"item,omitempty"`
	Title  string `json:"title,omitempty"`
	Origin string `json:"origin,omitempty"`
	Src    string `json:"src,omitempty"`
}

// Pair es un navegador vinculado: solo se guarda la huella de su token.
type Pair struct {
	Hash    string `json:"hash"`
	Name    string `json:"name"`
	Created int64  `json:"created"`
	Used    int64  `json:"used"`
}

type record struct {
	Item
	Secret box `json:"secret"`
}

type payload struct {
	Created int64              `json:"created"`
	Rev     int64              `json:"rev"`
	Items   map[string]*record `json:"items"`
	Never   []string           `json:"never,omitempty"`
	Events  []Event            `json:"events,omitempty"`
	Pairs   []Pair             `json:"pairs,omitempty"`
}

// Vault es la bóveda. Se puede usar desde varias goroutines.
type Vault struct {
	mu   sync.Mutex
	path string
	file *fileData
	key  *secmem.Key
	data *payload
	err  error
	now  func() time.Time
}

// Open prepara la bóveda del archivo dado. No pide la contraseña: solo mira si
// existe y si tiene buena pinta.
func Open(path string) *Vault {
	v := &Vault{path: path, now: time.Now}
	v.mu.Lock()
	v.reload()
	v.mu.Unlock()
	return v
}

// Path es la ruta del archivo.
func (v *Vault) Path() string { return v.path }

func (v *Vault) reload() {
	v.file, v.err = nil, nil
	f, err := readFile(v.path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			v.err = err
		}
		return
	}
	v.file = f
}

func readFile(path string) (*fileData, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseFile(b)
}

func parseFile(b []byte) (*fileData, error) {
	var f fileData
	if err := json.Unmarshal(b, &f); err != nil || f.Format != formatName {
		return nil, ErrFormat
	}
	if f.V != fileVersion {
		return nil, fmt.Errorf("este archivo es de una versión más nueva de TurboVAULT (formato %d)", f.V)
	}
	k := f.KDF
	// Límites para que un archivo manipulado no pueda pedir 64 GB de memoria.
	if k.Alg != "argon2id" || len(k.Salt) < 16 || k.Time < 1 || k.Time > 10 ||
		k.Memory < 8*1024 || k.Memory > 1024*1024 || k.Threads < 1 || k.Threads > 16 ||
		len(f.Key.N) != 12 || len(f.Data.N) != 12 || len(f.Key.C) != 32+16 {
		return nil, ErrCorrupted
	}
	return &f, nil
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

// Rev cambia cada vez que cambia el contenido (no la actividad).
func (v *Vault) Rev() int64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.data == nil {
		return 0
	}
	return v.data.Rev
}

func kdfAAD(k kdfInfo) []byte {
	b := []byte("TurboVAULT/1/key|argon2id|")
	b = append(b, k.Salt...)
	b = binary.LittleEndian.AppendUint32(b, k.Time)
	b = binary.LittleEndian.AppendUint32(b, k.Memory)
	return append(b, k.Threads)
}

var dataAAD = []byte("TurboVAULT/1/data")

func itemAAD(id string) []byte { return []byte("TurboVAULT/1/item/" + id) }

func derive(master string, k kdfInfo) []byte {
	return argon2.IDKey([]byte(master), k.Salt, k.Time, k.Memory, k.Threads, 32)
}

func checkMaster(master string) error {
	if utf8.RuneCountInString(master) < MinMaster {
		return ErrShort
	}
	return nil
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

func newKDF() kdfInfo {
	return kdfInfo{Alg: "argon2id", Salt: randomBytes(16), Params: newParams}
}

func seal(key, plain, aad []byte) (box, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return box{}, err
	}
	n := randomBytes(gcm.NonceSize())
	return box{N: n, C: gcm.Seal(nil, n, plain, aad)}, nil
}

func open(key []byte, b box, aad []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(b.N) != gcm.NonceSize() {
		return nil, ErrCorrupted
	}
	return gcm.Open(nil, b.N, b.C, aad)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Create crea una bóveda vacía con esa contraseña maestra y la deja abierta.
func (v *Vault) Create(master string) error {
	if err := checkMaster(master); err != nil {
		return err
	}
	v.mu.Lock()
	exists := v.file != nil
	damaged := v.err
	v.mu.Unlock()
	if exists {
		return ErrExists
	}
	// Si hay un archivo que ahora no se puede leer (antivirus, OneDrive, un fallo
	// del disco), una bóveda nueva lo pisaría: puede que se recupere más tarde o
	// desde una copia.
	if _, err := os.Stat(v.path); damaged != nil || err == nil {
		return errors.New("ya hay una bóveda en este PC aunque ahora no se puede leer: restaura una copia o vuelve a intentarlo, pero no se crea otra encima")
	}
	k := newKDF()
	kek := derive(master, k)
	defer secmem.Wipe(kek)
	dek := randomBytes(32)
	keyBox, err := seal(kek, dek, kdfAAD(k))
	if err != nil {
		return err
	}
	skey, err := secmem.New(dek)
	if err != nil {
		return err
	}
	now := v.now().Unix()
	data := &payload{Created: now, Rev: 1, Items: map[string]*record{}}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.file != nil {
		skey.Destroy()
		return ErrExists
	}
	f := &fileData{Format: formatName, V: fileVersion, KDF: k, Key: keyBox}
	if err := v.write(skey, f, data); err != nil {
		skey.Destroy()
		return err
	}
	v.key, v.data = skey, data
	return nil
}

// unwrap saca la DEK de un archivo con la contraseña maestra. Tarda (Argon2id).
func unwrap(f *fileData, master string) ([]byte, error) {
	kek := derive(master, f.KDF)
	defer secmem.Wipe(kek)
	dek, err := open(kek, f.Key, kdfAAD(f.KDF))
	if err != nil || len(dek) != 32 {
		return nil, ErrWrong
	}
	return dek, nil
}

func decryptPayload(dek []byte, f *fileData) (*payload, error) {
	plain, err := open(dek, f.Data, dataAAD)
	if err != nil {
		return nil, ErrCorrupted
	}
	defer secmem.Wipe(plain)
	var p payload
	if err := json.Unmarshal(plain, &p); err != nil {
		return nil, ErrCorrupted
	}
	if p.Items == nil {
		p.Items = map[string]*record{}
	}
	for id, r := range p.Items {
		if r == nil || r.ID != id {
			return nil, ErrCorrupted
		}
	}
	return &p, nil
}

// Unlock abre la bóveda. Tarda un momento a propósito.
func (v *Vault) Unlock(master string) error {
	v.mu.Lock()
	v.reload() // por si se ha restaurado una copia mientras tanto
	f, ferr := v.file, v.err
	v.mu.Unlock()
	if f == nil {
		if ferr != nil {
			return ferr
		}
		return ErrNoVault
	}
	dek, err := unwrap(f, master)
	if err != nil {
		return err
	}
	data, err := decryptPayload(dek, f)
	if err != nil {
		secmem.Wipe(dek)
		return err
	}
	skey, err := secmem.New(dek)
	if err != nil {
		return err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key != nil {
		v.key.Destroy()
	}
	v.key, v.data = skey, data
	if v.purgeTrash() {
		_ = v.save()
	}
	return nil
}

// VerifyMaster comprueba la contraseña maestra sin cambiar nada (para
// confirmar acciones delicadas como exportar).
func (v *Vault) VerifyMaster(master string) error {
	v.mu.Lock()
	f := v.file
	v.mu.Unlock()
	if f == nil {
		return ErrNoVault
	}
	dek, err := unwrap(f, master)
	if err != nil {
		return err
	}
	secmem.Wipe(dek)
	return nil
}

// Lock cierra la bóveda y borra la clave de la memoria.
func (v *Vault) Lock() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key != nil {
		v.key.Destroy()
	}
	v.key, v.data = nil, nil
}

// write cifra data con la DEK y escribe el archivo de forma segura: primero un
// temporal que se fuerza a disco, después se cambia por el bueno. La versión
// anterior queda en .bak por si acaso.
func (v *Vault) write(dek *secmem.Key, f *fileData, data *payload) error {
	plain, err := json.Marshal(data)
	if err != nil {
		return err
	}
	defer secmem.Wipe(plain)
	var dataBox box
	err = dek.Use(func(k []byte) error {
		var e error
		dataBox, e = seal(k, plain, dataAAD)
		return e
	})
	if err != nil {
		return err
	}
	nf := *f
	nf.Data = dataBox
	out, err := json.Marshal(&nf)
	if err != nil {
		return err
	}
	var prev []byte
	if v.file != nil {
		prev, _ = json.Marshal(v.file)
	}
	if err := writeAtomic(v.path, out, prev); err != nil {
		return err
	}
	v.file, v.err = &nf, nil
	return nil
}

// writeAtomic escribe data en path. Si prev no está vacío, se guarda antes en
// .bak (la versión anterior, por si el disco falla a mitad).
func writeAtomic(path string, data, prev []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if len(prev) > 0 {
		_ = os.WriteFile(path+".bak", prev, 0o600)
	}
	tmp := path + ".tmp"
	fh, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := fh.Write(data); err != nil {
		fh.Close()
		os.Remove(tmp)
		return err
	}
	if err := fh.Sync(); err != nil {
		fh.Close()
		os.Remove(tmp)
		return err
	}
	if err := fh.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func (v *Vault) save() error { return v.write(v.key, v.file, v.data) }

// commit guarda un payload nuevo y, si va bien, lo deja como el actual.
func (v *Vault) commit(next *payload, content bool) error {
	if content {
		next.Rev++
	}
	if err := v.write(v.key, v.file, next); err != nil {
		return err
	}
	v.data = next
	return nil
}

func (p *payload) clone() *payload {
	n := *p
	n.Items = make(map[string]*record, len(p.Items)+1)
	for k, r := range p.Items {
		c := *r
		n.Items[k] = &c
	}
	n.Never = append([]string(nil), p.Never...)
	n.Events = append([]Event(nil), p.Events...)
	n.Pairs = append([]Pair(nil), p.Pairs...)
	return &n
}

func (v *Vault) purgeTrash() bool {
	limit := v.now().Add(-TrashDays * 24 * time.Hour).Unix()
	changed := false
	for id, r := range v.data.Items {
		if r.Deleted != 0 && r.Deleted < limit {
			delete(v.data.Items, id)
			changed = true
		}
	}
	return changed
}

func cloneItem(it Item) Item {
	it.URLs = append([]string(nil), it.URLs...)
	it.Tags = append([]string(nil), it.Tags...)
	return it
}

// Items devuelve las entradas (sin secretos). Con trash=true, solo las de la
// papelera; si no, solo las vivas.
func (v *Vault) Items(trash bool) ([]Item, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil {
		return nil, ErrLocked
	}
	out := make([]Item, 0, len(v.data.Items))
	for _, r := range v.data.Items {
		if (r.Deleted != 0) == trash {
			out = append(out, cloneItem(r.Item))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].Title), strings.ToLower(out[j].Title)
		if a != b {
			return a < b
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (v *Vault) decryptSecret(r *record) (Secret, error) {
	var s Secret
	if len(r.Secret.C) == 0 {
		return s, nil
	}
	var plain []byte
	err := v.key.Use(func(k []byte) error {
		var e error
		plain, e = open(k, r.Secret, itemAAD(r.ID))
		return e
	})
	if err != nil {
		return s, ErrCorrupted
	}
	defer secmem.Wipe(plain)
	if err := json.Unmarshal(plain, &s); err != nil {
		return s, ErrCorrupted
	}
	return s, nil
}

func (v *Vault) encryptSecret(id string, s *Secret) (box, error) {
	plain, err := json.Marshal(s)
	if err != nil {
		return box{}, err
	}
	defer secmem.Wipe(plain)
	var b box
	err = v.key.Use(func(k []byte) error {
		var e error
		b, e = seal(k, plain, itemAAD(id))
		return e
	})
	return b, err
}

// Get devuelve una entrada con sus secretos.
func (v *Vault) Get(id string) (Item, Secret, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil {
		return Item{}, Secret{}, ErrLocked
	}
	r, ok := v.data.Items[id]
	if !ok {
		return Item{}, Secret{}, ErrNotFound
	}
	s, err := v.decryptSecret(r)
	return cloneItem(r.Item), s, err
}

// NewID devuelve un identificador aleatorio.
func NewID() string { return hex.EncodeToString(randomBytes(12)) }

// Entry junta una entrada y su secreto (para guardar varias a la vez).
type Entry struct {
	Item   Item
	Secret *Secret // nil: dejar el secreto como estaba
}

// Put crea o actualiza una entrada. Si sec es nil se conserva el secreto que
// tenía. Si cambia la contraseña, la anterior pasa al historial.
func (v *Vault) Put(it Item, sec *Secret) (Item, error) {
	out, err := v.PutMany([]Entry{{Item: it, Secret: sec}})
	if err != nil {
		return Item{}, err
	}
	return out[0], nil
}

// PutMany guarda varias entradas en una sola escritura (importaciones).
func (v *Vault) PutMany(entries []Entry) ([]Item, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil {
		return nil, ErrLocked
	}
	next := v.data.clone()
	now := v.now().Unix()
	out := make([]Item, 0, len(entries))
	for _, e := range entries {
		it := cloneItem(e.Item)
		if it.Type != TypeNote {
			it.Type = TypeLogin
		}
		it.Title = strings.TrimSpace(it.Title)
		old, exists := next.Items[it.ID]
		if it.ID == "" || !exists {
			if it.ID == "" {
				it.ID = NewID()
			}
			if it.Created == 0 {
				it.Created = now
			}
			old = nil
		} else {
			// Lo que no decide quien llama se conserva.
			it.Created, it.Used, it.Uses, it.Deleted = old.Created, old.Used, old.Uses, old.Deleted
			it.PwChanged = old.PwChanged
		}
		it.Updated = now
		var secBox box
		if e.Secret == nil {
			if old == nil {
				return nil, ErrNotFound
			}
			secBox = old.Secret
			it.HasPassword, it.HasTOTP, it.HasNotes, it.Fields = old.HasPassword, old.HasTOTP, old.HasNotes, old.Fields
			it.Score = old.Score
		} else {
			s := *e.Secret
			if old != nil {
				prev, err := v.decryptSecret(old)
				if err != nil {
					return nil, err
				}
				s.History = prev.History
				if prev.Password != "" && prev.Password != s.Password {
					s.History = append([]OldPassword{{Password: prev.Password, Until: now}}, s.History...)
					it.PwChanged = now
				}
				prev.wipe()
			} else if s.Password != "" && it.PwChanged == 0 {
				it.PwChanged = now
			}
			if len(s.History) > maxPwHist {
				s.History = s.History[:maxPwHist]
			}
			it.HasPassword = s.Password != ""
			it.HasTOTP = s.TOTP != ""
			it.HasNotes = s.Notes != ""
			it.Fields = len(s.Fields)
			if !it.HasPassword {
				it.Score = -1
			}
			b, err := v.encryptSecret(it.ID, &s)
			s.wipe()
			if err != nil {
				return nil, err
			}
			secBox = b
		}
		next.Items[it.ID] = &record{Item: it, Secret: secBox}
		out = append(out, cloneItem(it))
	}
	if err := v.commit(next, true); err != nil {
		return nil, err
	}
	return out, nil
}

func (v *Vault) mutate(id string, content bool, fn func(r *record)) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil {
		return ErrLocked
	}
	if _, ok := v.data.Items[id]; !ok {
		return ErrNotFound
	}
	next := v.data.clone()
	fn(next.Items[id])
	return v.commit(next, content)
}

// Delete manda una entrada a la papelera.
func (v *Vault) Delete(id string) error {
	now := v.now().Unix()
	return v.mutate(id, true, func(r *record) { r.Deleted = now })
}

// Restore saca una entrada de la papelera.
func (v *Vault) Restore(id string) error {
	return v.mutate(id, true, func(r *record) { r.Deleted = 0 })
}

// SetFav marca o desmarca una entrada como favorita.
func (v *Vault) SetFav(id string, fav bool) error {
	return v.mutate(id, true, func(r *record) { r.Fav = fav })
}

// Touch apunta que se ha usado una entrada.
func (v *Vault) Touch(id string) error {
	now := v.now().Unix()
	return v.mutate(id, false, func(r *record) { r.Used = now; r.Uses++ })
}

// Purge borra del todo una entrada (desde la papelera).
func (v *Vault) Purge(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil {
		return ErrLocked
	}
	if _, ok := v.data.Items[id]; !ok {
		return ErrNotFound
	}
	next := v.data.clone()
	delete(next.Items, id)
	return v.commit(next, true)
}

// EmptyTrash vacía la papelera.
func (v *Vault) EmptyTrash() (int, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil {
		return 0, ErrLocked
	}
	next := v.data.clone()
	n := 0
	for id, r := range next.Items {
		if r.Deleted != 0 {
			delete(next.Items, id)
			n++
		}
	}
	if n == 0 {
		return 0, nil
	}
	return n, v.commit(next, true)
}

// Each recorre todas las entradas vivas con su secreto (informes de salud,
// exportar). El secreto solo es válido dentro de fn.
func (v *Vault) Each(fn func(Item, Secret)) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil {
		return ErrLocked
	}
	ids := make([]string, 0, len(v.data.Items))
	for id, r := range v.data.Items {
		if r.Deleted == 0 {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		r := v.data.Items[id]
		s, err := v.decryptSecret(r)
		if err != nil {
			return err
		}
		fn(cloneItem(r.Item), s)
		s.wipe()
	}
	return nil
}

// ---------- actividad ----------

// AddEvent apunta algo en el registro de actividad (cifrado dentro de la bóveda).
func (v *Vault) AddEvent(e Event) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil {
		return ErrLocked
	}
	if e.T == 0 {
		e.T = v.now().Unix()
	}
	next := v.data.clone()
	next.Events = append(next.Events, e)
	if len(next.Events) > maxEvents {
		next.Events = next.Events[len(next.Events)-maxEvents:]
	}
	return v.commit(next, false)
}

// Events devuelve la actividad, lo más reciente primero.
func (v *Vault) Events() ([]Event, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil {
		return nil, ErrLocked
	}
	out := make([]Event, len(v.data.Events))
	for i, e := range v.data.Events {
		out[len(out)-1-i] = e
	}
	return out, nil
}

// ClearEvents borra el registro de actividad.
func (v *Vault) ClearEvents() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil {
		return ErrLocked
	}
	next := v.data.clone()
	next.Events = nil
	return v.commit(next, false)
}

// ---------- webs donde no guardar ----------

// Never devuelve las webs donde no se ofrece guardar contraseñas.
func (v *Vault) Never() ([]string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil {
		return nil, ErrLocked
	}
	return append([]string{}, v.data.Never...), nil // nunca null: la ventana espera una lista
}

// SetNever añade o quita una web de la lista de "no guardar nunca".
func (v *Vault) SetNever(host string, on bool) error {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return nil
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil {
		return ErrLocked
	}
	next := v.data.clone()
	out := next.Never[:0]
	for _, h := range next.Never {
		if h != host {
			out = append(out, h)
		}
	}
	if on {
		out = append(out, host)
		sort.Strings(out)
	}
	next.Never = out
	return v.commit(next, true)
}

// ---------- navegadores vinculados ----------

// Pairs devuelve los navegadores vinculados.
func (v *Vault) Pairs() ([]Pair, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil {
		return nil, ErrLocked
	}
	return append([]Pair{}, v.data.Pairs...), nil
}

// HasPair dice si ese token (su huella) está vinculado y apunta el uso.
func (v *Vault) HasPair(hash string) (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil {
		return false, ErrLocked
	}
	for _, p := range v.data.Pairs {
		if p.Hash == hash {
			return true, nil
		}
	}
	return false, nil
}

// AddPair vincula un navegador.
func (v *Vault) AddPair(hash, name string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil {
		return ErrLocked
	}
	next := v.data.clone()
	for _, p := range next.Pairs {
		if p.Hash == hash {
			return nil
		}
	}
	now := v.now().Unix()
	next.Pairs = append(next.Pairs, Pair{Hash: hash, Name: name, Created: now, Used: now})
	return v.commit(next, true)
}

// RemovePair desvincula un navegador.
func (v *Vault) RemovePair(hash string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil {
		return ErrLocked
	}
	next := v.data.clone()
	out := next.Pairs[:0]
	for _, p := range next.Pairs {
		if p.Hash != hash {
			out = append(out, p)
		}
	}
	next.Pairs = out
	return v.commit(next, true)
}

// ---------- contraseña maestra ----------

// ChangeMaster cambia la contraseña maestra. También cambia la sal y la clave
// de datos (DEK), y vuelve a cifrar todos los secretos con la nueva: las copias
// viejas del archivo no ayudan a leer las nuevas.
func (v *Vault) ChangeMaster(old, newMaster string) error {
	if err := checkMaster(newMaster); err != nil {
		return err
	}
	v.mu.Lock()
	f := v.file
	isOpen := v.key != nil
	v.mu.Unlock()
	if f == nil {
		return ErrNoVault
	}
	if !isOpen {
		return ErrLocked
	}
	oldDEK, err := unwrap(f, old)
	if err != nil {
		return err
	}
	secmem.Wipe(oldDEK)

	k := newKDF()
	kek := derive(newMaster, k)
	defer secmem.Wipe(kek)
	dek := randomBytes(32)
	keyBox, err := seal(kek, dek, kdfAAD(k))
	if err != nil {
		return err
	}
	newKey, err := secmem.New(dek)
	if err != nil {
		return err
	}

	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil {
		newKey.Destroy()
		return ErrLocked
	}
	next := v.data.clone()
	for id, r := range next.Items {
		s, err := v.decryptSecret(r)
		if err != nil {
			newKey.Destroy()
			return err
		}
		plain, _ := json.Marshal(&s)
		s.wipe()
		var b box
		err = newKey.Use(func(key []byte) error {
			var e error
			b, e = seal(key, plain, itemAAD(id))
			return e
		})
		secmem.Wipe(plain)
		if err != nil {
			newKey.Destroy()
			return err
		}
		r.Secret = b
	}
	next.Rev++
	nf := &fileData{Format: formatName, V: fileVersion, KDF: k, Key: keyBox}
	if err := v.write(newKey, nf, next); err != nil {
		newKey.Destroy()
		return err
	}
	// El .bak tiene la clave vieja: se cambia por la versión nueva.
	if b, err := os.ReadFile(v.path); err == nil {
		_ = os.WriteFile(v.path+".bak", b, 0o600)
	}
	v.key.Destroy()
	v.key, v.data = newKey, next
	return nil
}

// ---------- copias ----------

// FileInfo describe un archivo de bóveda (una copia de seguridad).
type FileInfo struct {
	Items   int   `json:"items"`
	Created int64 `json:"created"`
	Rev     int64 `json:"rev"`
}

// CheckFile abre un archivo de bóveda con su contraseña maestra para ver qué
// tiene, sin tocar nada.
func CheckFile(path, master string) (FileInfo, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return FileInfo{}, err
	}
	return checkBytes(b, master)
}

func checkBytes(b []byte, master string) (FileInfo, error) {
	f, err := parseFile(b)
	if err != nil {
		return FileInfo{}, err
	}
	dek, err := unwrap(f, master)
	if err != nil {
		return FileInfo{}, err
	}
	defer secmem.Wipe(dek)
	p, err := decryptPayload(dek, f)
	if err != nil {
		return FileInfo{}, err
	}
	n := 0
	for _, r := range p.Items {
		if r.Deleted == 0 {
			n++
		}
	}
	return FileInfo{Items: n, Created: p.Created, Rev: p.Rev}, nil
}

// LooksLikeVault dice si un archivo parece de TurboVAULT (sin abrirlo).
func LooksLikeVault(path string) bool {
	_, err := readFile(path)
	return err == nil
}

// Bytes devuelve el archivo cifrado tal cual está en disco (para copias).
func (v *Vault) Bytes() ([]byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.file == nil {
		return nil, ErrNoVault
	}
	return json.Marshal(v.file)
}

// ReplaceWith pone en su sitio la bóveda de otro archivo (restaurar una copia).
// Pide su contraseña maestra para no restaurar algo que luego no se pueda abrir.
// La bóveda que había se guarda al lado con la fecha. Queda cerrada.
func (v *Vault) ReplaceWith(path, master string) (string, error) {
	// Se lee una sola vez: lo que se comprueba es exactamente lo que se escribe.
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if _, err := checkBytes(raw, master); err != nil {
		return "", err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	saved := ""
	if _, err := os.Stat(v.path); err == nil {
		saved = strings.TrimSuffix(v.path, filepath.Ext(v.path)) + ".anterior-" + v.now().Format("20060102-150405") + filepath.Ext(v.path)
		if err := os.Rename(v.path, saved); err != nil {
			return "", err
		}
	}
	if err := writeAtomic(v.path, raw, nil); err != nil {
		if saved != "" {
			os.Rename(saved, v.path)
		}
		return "", err
	}
	if v.key != nil {
		v.key.Destroy()
	}
	v.key, v.data = nil, nil
	v.reload()
	return saved, v.err
}

// Destroy borra la bóveda entera (pide la contraseña maestra).
func (v *Vault) Destroy(master string) error {
	if err := v.VerifyMaster(master); err != nil {
		return err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key != nil {
		v.key.Destroy()
	}
	v.key, v.data, v.file, v.err = nil, nil, nil, nil
	os.Remove(v.path + ".bak")
	if err := os.Remove(v.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
