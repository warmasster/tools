package vault

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const master = "caballo-grapa-bateria-correcta"

func init() { SetParamsForTests() }

func newVault(t *testing.T) *Vault {
	t.Helper()
	v := Open(filepath.Join(t.TempDir(), "vault.tvault"))
	if err := v.Create(master); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestCreateUnlockLock(t *testing.T) {
	v := newVault(t)
	if !v.Exists() || !v.Unlocked() {
		t.Fatal("recién creada debe existir y estar abierta")
	}
	it, err := v.Put(Item{Title: "GitHub", URLs: []string{"https://github.com"}, Username: "fer"}, &Secret{Password: "s3cr3t-muy-largo", TOTP: "JBSWY3DPEHPK3PXP"})
	if err != nil {
		t.Fatal(err)
	}
	if it.ID == "" || !it.HasPassword || !it.HasTOTP || it.PwChanged == 0 {
		t.Fatalf("item mal rellenado: %+v", it)
	}
	v.Lock()
	if v.Unlocked() {
		t.Fatal("debería estar cerrada")
	}
	if _, err := v.Items(false); !errors.Is(err, ErrLocked) {
		t.Fatalf("cerrada: %v", err)
	}
	if err := v.Unlock("otra-contraseña-larga"); !errors.Is(err, ErrWrong) {
		t.Fatalf("contraseña mala: %v", err)
	}
	// Una instancia nueva sobre el mismo archivo.
	v2 := Open(v.Path())
	if err := v2.Unlock(master); err != nil {
		t.Fatal(err)
	}
	got, sec, err := v2.Get(it.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "GitHub" || sec.Password != "s3cr3t-muy-largo" || sec.TOTP != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("datos distintos: %+v %+v", got, sec)
	}
}

func TestShortMaster(t *testing.T) {
	v := Open(filepath.Join(t.TempDir(), "v.tvault"))
	if err := v.Create("corta"); !errors.Is(err, ErrShort) {
		t.Fatalf("got %v", err)
	}
}

func TestNoPlaintextOnDisk(t *testing.T) {
	v := newVault(t)
	v.Put(Item{Title: "BancoSecreto", Username: "usuario-unico-xyz"}, &Secret{Password: "ContraseñaSuperSecreta123", Notes: "nota privada"})
	raw, _ := os.ReadFile(v.Path())
	for _, s := range []string{"BancoSecreto", "usuario-unico-xyz", "ContraseñaSuperSecreta123", "nota privada"} {
		if bytes.Contains(raw, []byte(s)) {
			t.Fatalf("%q aparece en claro en el archivo", s)
		}
	}
}

func TestTamperDetected(t *testing.T) {
	v := newVault(t)
	v.Put(Item{Title: "x"}, &Secret{Password: "y"})
	raw, _ := os.ReadFile(v.Path())
	var f fileData
	json.Unmarshal(raw, &f)
	f.Data.C[len(f.Data.C)/2] ^= 1
	out, _ := json.Marshal(f)
	os.WriteFile(v.Path(), out, 0o600)
	v2 := Open(v.Path())
	if err := v2.Unlock(master); !errors.Is(err, ErrCorrupted) {
		t.Fatalf("manipulado: %v", err)
	}
}

func TestKDFParamsBound(t *testing.T) {
	v := newVault(t)
	raw, _ := os.ReadFile(v.Path())
	var f fileData
	json.Unmarshal(raw, &f)
	f.KDF.Time++ // un atacante cambia los parámetros: no debe abrir
	out, _ := json.Marshal(f)
	os.WriteFile(v.Path(), out, 0o600)
	if err := Open(v.Path()).Unlock(master); !errors.Is(err, ErrWrong) {
		t.Fatalf("parámetros cambiados: %v", err)
	}
	f.KDF.Memory = 64 * 1024 * 1024 // 64 GB: ni lo intenta
	out, _ = json.Marshal(f)
	os.WriteFile(v.Path(), out, 0o600)
	if err := Open(v.Path()).Unlock(master); !errors.Is(err, ErrCorrupted) {
		t.Fatalf("memoria absurda: %v", err)
	}
}

func TestSecretSwapDetected(t *testing.T) {
	// Cambiar el secreto de una entrada por el de otra no debe colar.
	v := newVault(t)
	a, _ := v.Put(Item{Title: "a"}, &Secret{Password: "pa"})
	b, _ := v.Put(Item{Title: "b"}, &Secret{Password: "pb"})
	v.mu.Lock()
	v.data.Items[a.ID].Secret = v.data.Items[b.ID].Secret
	v.mu.Unlock()
	if _, _, err := v.Get(a.ID); !errors.Is(err, ErrCorrupted) {
		t.Fatalf("secreto cambiado: %v", err)
	}
}

func TestPasswordHistoryAndKeepSecret(t *testing.T) {
	v := newVault(t)
	it, _ := v.Put(Item{Title: "web", Username: "a"}, &Secret{Password: "uno"})
	it.Username = "b"
	it, _ = v.Put(it, nil) // sin tocar el secreto
	_, s, _ := v.Get(it.ID)
	if s.Password != "uno" || it.Username != "b" || !it.HasPassword {
		t.Fatalf("Put sin secreto: %+v %+v", it, s)
	}
	v.Put(it, &Secret{Password: "dos"})
	v.Put(it, &Secret{Password: "tres"})
	_, s, _ = v.Get(it.ID)
	if s.Password != "tres" || len(s.History) != 2 || s.History[0].Password != "dos" || s.History[1].Password != "uno" {
		t.Fatalf("historial: %+v", s.History)
	}
	// Guardar la misma contraseña no añade historial.
	v.Put(it, &Secret{Password: "tres", Notes: "n"})
	_, s, _ = v.Get(it.ID)
	if len(s.History) != 2 || s.Notes != "n" {
		t.Fatalf("historial repetido: %+v", s)
	}
}

func TestTrash(t *testing.T) {
	v := newVault(t)
	it, _ := v.Put(Item{Title: "borrar"}, &Secret{Password: "p"})
	if err := v.Delete(it.ID); err != nil {
		t.Fatal(err)
	}
	live, _ := v.Items(false)
	trash, _ := v.Items(true)
	if len(live) != 0 || len(trash) != 1 {
		t.Fatalf("papelera: %d vivas, %d en papelera", len(live), len(trash))
	}
	v.Restore(it.ID)
	live, _ = v.Items(false)
	if len(live) != 1 {
		t.Fatal("no se restauró")
	}
	v.Delete(it.ID)
	// Pasados 31 días se borra al abrir.
	v.Lock()
	v.now = func() time.Time { return time.Now().Add(31 * 24 * time.Hour) }
	if err := v.Unlock(master); err != nil {
		t.Fatal(err)
	}
	trash, _ = v.Items(true)
	if len(trash) != 0 {
		t.Fatal("la papelera vieja no se vació")
	}
}

func TestChangeMasterRotatesKey(t *testing.T) {
	v := newVault(t)
	it, _ := v.Put(Item{Title: "x"}, &Secret{Password: "secreto"})
	oldFile, _ := os.ReadFile(v.Path())
	const nuevo = "otra-maestra-bastante-larga"
	if err := v.ChangeMaster("mala-mala-mala", nuevo); !errors.Is(err, ErrWrong) {
		t.Fatalf("maestra vieja mala: %v", err)
	}
	if err := v.ChangeMaster(master, nuevo); err != nil {
		t.Fatal(err)
	}
	_, s, err := v.Get(it.ID)
	if err != nil || s.Password != "secreto" {
		t.Fatalf("después de cambiar: %v %+v", err, s)
	}
	v.Lock()
	if err := v.Unlock(master); !errors.Is(err, ErrWrong) {
		t.Fatal("la maestra vieja sigue abriendo")
	}
	if err := v.Unlock(nuevo); err != nil {
		t.Fatal(err)
	}
	// La DEK vieja (sacada de una copia vieja) no abre el archivo nuevo.
	var of, nf fileData
	json.Unmarshal(oldFile, &of)
	cur, _ := os.ReadFile(v.Path())
	json.Unmarshal(cur, &nf)
	oldDEK, err := unwrap(&of, master)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decryptPayload(oldDEK, &nf); err == nil {
		t.Fatal("la clave vieja abre los datos nuevos")
	}
	bak, _ := os.ReadFile(v.Path() + ".bak")
	if bytes.Equal(bak, oldFile) {
		t.Fatal("el .bak sigue teniendo la clave vieja")
	}
}

func TestEventsNeverPairs(t *testing.T) {
	v := newVault(t)
	rev := v.Rev()
	for i := 0; i < maxEvents+10; i++ {
		v.AddEvent(Event{Kind: "fill", Origin: "https://a.test"})
	}
	ev, _ := v.Events()
	if len(ev) != maxEvents {
		t.Fatalf("eventos: %d", len(ev))
	}
	if v.Rev() != rev {
		t.Fatal("la actividad no debe contar como cambio de contenido")
	}
	v.SetNever("Ejemplo.com", true)
	v.SetNever("a.com", true)
	v.SetNever("ejemplo.com", false)
	n, _ := v.Never()
	if strings.Join(n, ",") != "a.com" {
		t.Fatalf("never: %v", n)
	}
	v.AddPair("h1", "Chrome")
	ok, _ := v.HasPair("h1")
	no, _ := v.HasPair("h2")
	if !ok || no {
		t.Fatal("vinculación")
	}
	v.RemovePair("h1")
	ok, _ = v.HasPair("h1")
	if ok {
		t.Fatal("no se desvinculó")
	}
}

func TestBackupRestore(t *testing.T) {
	v := newVault(t)
	v.Put(Item{Title: "en la copia"}, &Secret{Password: "1"})
	raw, _ := v.Bytes()
	backup := filepath.Join(t.TempDir(), "copia.tvault")
	os.WriteFile(backup, raw, 0o600)
	info, err := CheckFile(backup, master)
	if err != nil || info.Items != 1 {
		t.Fatalf("CheckFile: %v %+v", err, info)
	}
	if _, err := CheckFile(backup, "no-es-la-maestra!"); !errors.Is(err, ErrWrong) {
		t.Fatalf("copia con maestra mala: %v", err)
	}
	v.Put(Item{Title: "después"}, &Secret{Password: "2"})
	saved, err := v.ReplaceWith(backup, master)
	if err != nil {
		t.Fatal(err)
	}
	if saved == "" || !LooksLikeVault(saved) {
		t.Fatalf("no guardó la anterior: %q", saved)
	}
	if v.Unlocked() {
		t.Fatal("tras restaurar debe quedar cerrada")
	}
	v.Unlock(master)
	items, _ := v.Items(false)
	if len(items) != 1 || items[0].Title != "en la copia" {
		t.Fatalf("restaurado: %+v", items)
	}
}

func TestEachAndPutMany(t *testing.T) {
	v := newVault(t)
	var es []Entry
	for i := 0; i < 50; i++ {
		es = append(es, Entry{Item: Item{Title: "imp"}, Secret: &Secret{Password: "p"}})
	}
	if _, err := v.PutMany(es); err != nil {
		t.Fatal(err)
	}
	n := 0
	v.Each(func(it Item, s Secret) {
		if s.Password == "p" {
			n++
		}
	})
	if n != 50 {
		t.Fatalf("Each: %d", n)
	}
}

func TestDefaultParamsCost(t *testing.T) {
	if testing.Short() {
		t.Skip("lento")
	}
	k := kdfInfo{Alg: "argon2id", Salt: randomBytes(16), Params: DefaultParams}
	t0 := time.Now()
	derive("prueba-de-tiempo", k)
	t.Logf("Argon2id %+v: %v", DefaultParams, time.Since(t0))
}

func TestCardItem(t *testing.T) {
	v := newVault(t)
	c := &Card{Holder: "FERNANDO GARCIA", Number: "4242424242424242", ExpMonth: "08", ExpYear: "2028", CVC: "123", PIN: "1234"}
	it, err := v.Put(Item{Type: TypeCard, Title: "Visa del banco"}, &Secret{Card: c})
	if err != nil {
		t.Fatal(err)
	}
	if it.Type != TypeCard || it.Brand != "visa" || it.Last4 != "4242" || it.Exp != "2028-08" || it.HasPassword || it.Score != -1 {
		t.Fatalf("tarjeta mal guardada: %+v", it)
	}
	// La lista no lleva el número ni el CVC.
	items, _ := v.Items(false)
	raw, _ := json.Marshal(items)
	if strings.Contains(string(raw), "424242424242") || strings.Contains(string(raw), "123") && strings.Contains(string(raw), "cvc") {
		t.Fatalf("la lista enseña datos de la tarjeta: %s", raw)
	}
	// Ni el archivo.
	b, _ := os.ReadFile(v.Path())
	if bytes.Contains(b, []byte("4242424242424242")) || bytes.Contains(b, []byte("FERNANDO")) {
		t.Fatal("datos de la tarjeta sin cifrar en el disco")
	}
	// Cambiar solo el título conserva los datos.
	it.Title = "Visa"
	if _, err := v.Put(it, nil); err != nil {
		t.Fatal(err)
	}
	got, sec, err := v.Get(it.ID)
	if err != nil || got.Last4 != "4242" || got.Brand != "visa" || sec.Card == nil || sec.Card.CVC != "123" || sec.Card.PIN != "1234" {
		t.Fatalf("tras editar: %+v %+v %v", got, sec.Card, err)
	}
	// Al pasarla a inicio de sesión se quitan los datos de la tarjeta.
	got.Type = TypeLogin
	if _, err := v.Put(got, &Secret{Password: "x", Card: c}); err != nil {
		t.Fatal(err)
	}
	got, sec, _ = v.Get(it.ID)
	if sec.Card != nil || got.Brand != "" || got.Last4 != "" {
		t.Fatalf("una entrada que no es tarjeta guarda datos de tarjeta: %+v %+v", got, sec.Card)
	}
}
