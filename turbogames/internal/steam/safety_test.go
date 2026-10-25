package steam

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeRelAndFolderName(t *testing.T) {
	for _, ok := range []string{"abc123/library_capsule.jpg", "library_hero.jpg", "e7f0c3a1"} {
		if !SafeRel(ok) {
			t.Errorf("SafeRel(%q) debería valer", ok)
		}
	}
	for _, bad := range []string{"../../x.jpg", "a/../../b", `..\x`, "C:/x", "/abs", "x?y=1", "a#b", ""} {
		if SafeRel(bad) {
			t.Errorf("SafeRel(%q) no debería valer", bad)
		}
	}
	for _, ok := range []string{"Assassin's Creed", "Tom Clancy's Rainbow Six Siege", "Half-Life 2", "Ñandú (2024)"} {
		if !folderName(ok) {
			t.Errorf("folderName(%q) debería valer", ok)
		}
	}
	for _, bad := range []string{"..", `..\..\Windows`, "../x", "C:", ""} {
		if folderName(bad) {
			t.Errorf("folderName(%q) no debería valer", bad)
		}
	}
}

// Dos copias seguidas del mismo archivo no se pisan.
func TestBackupNamesAreUnique(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "loginusers.vdf")
	in := &Install{BackupDir: filepath.Join(dir, "copias")}
	os.WriteFile(src, []byte("uno"), 0o644)
	in.backup(src)
	os.WriteFile(src, []byte("dos"), 0o644)
	in.backup(src)
	ents, _ := os.ReadDir(in.BackupDir)
	if len(ents) != 2 {
		t.Fatalf("debería haber 2 copias y hay %d", len(ents))
	}
	first, _ := os.ReadFile(filepath.Join(in.BackupDir, ents[0].Name()))
	if string(first) != "uno" {
		t.Errorf("la primera copia (por orden) debería ser la original: %q", first)
	}
}
