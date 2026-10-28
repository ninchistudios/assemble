package store

import (
	"database/sql"
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

// TestMigration5KeepsNotices upgrades a 2026.2 database: the notifications
// rebuild must carry every notice and dismissal across, and the old
// constraints must have gone.
func TestMigration5KeepsNotices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	for i, m := range migrations[:4] {
		if _, err := raw.Exec(m); err != nil {
			t.Fatalf("migration %d: %v", i+1, err)
		}
	}
	for _, q := range []string{
		`PRAGMA user_version = 4`,
		`INSERT INTO users (id, sub, email, name, role, created_at) VALUES (1, 's', 'a@example.com', 'Ada', 'admin', '2026-11-01T00:00:00Z')`,
		`INSERT INTO notifications (id, kind, day, actor_id, created_at) VALUES (7, 'locked', '2026-11-20', 1, '2026-11-01T00:00:00Z')`,
		`INSERT INTO dismissals (notification_id, user_id) VALUES (7, 1)`,
	} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	raw.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	defer s.Close()
	var kind, day, audience string
	if err := s.db.QueryRow(`SELECT kind, day, audience FROM notifications WHERE id = 7`).Scan(&kind, &day, &audience); err != nil {
		t.Fatalf("notice lost: %v", err)
	}
	if kind != "locked" || day != "2026-11-20" || audience != "all" {
		t.Errorf("notice = %s %s %s", kind, day, audience)
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM dismissals WHERE notification_id = 7 AND user_id = 1`).Scan(&n)
	if n != 1 {
		t.Error("dismissal lost")
	}
	// The cascade still works on the rebuilt table.
	s.db.Exec(`DELETE FROM notifications WHERE id = 7`)
	if s.db.QueryRow(`SELECT COUNT(*) FROM dismissals`).Scan(&n); n != 0 {
		t.Error("dismissals no longer cascade")
	}
	if _, err := s.db.Exec(`INSERT INTO notifications (kind, audience, created_at) VALUES ('joined', 'managers', '2026-11-01T00:00:00Z')`); err != nil {
		t.Errorf("a dayless notice for managers is refused: %v", err)
	}
	if u, _ := s.UserByID(1); u.Manager || !u.CanManage() {
		t.Errorf("admin after upgrade = %+v", u)
	}
}
