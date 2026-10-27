package store

import (
	"path/filepath"
	"testing"
)

// newStore opens a fresh database in a temp dir. A real file rather than
// :memory:, so WAL and reopening behave as they do on the volume.
func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrateFreshDatabase(t *testing.T) {
	s := newStore(t)
	v, err := s.Version()
	if err != nil {
		t.Fatal(err)
	}
	if v != len(migrations) {
		t.Errorf("Version = %d, want %d", v, len(migrations))
	}
}

func TestReopenKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE settings SET group_name = 'kept'`); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s, err = Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s.Close()
	var name string
	if err := s.db.QueryRow(`SELECT group_name FROM settings`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "kept" {
		t.Errorf("group_name after reopen = %q", name)
	}
}

func TestRefusesNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("PRAGMA user_version = 999"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if s, err := Open(path); err == nil {
		s.Close()
		t.Fatal("Open accepted a database from a newer build")
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	s := newStore(t)
	_, err := s.db.Exec(`INSERT INTO sessions (token_hash, user_id, created_at, expires_at) VALUES ('x', 42, '', '')`)
	if err == nil {
		t.Fatal("session for a missing user was accepted")
	}
}
