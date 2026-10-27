package store

import (
	"errors"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

func TestCreateAndFindUser(t *testing.T) {
	s := newStore(t)
	u, err := s.CreateUser("sub-1", "Admin@Example.com", "Ada", RoleAdmin, 0, t0)
	if err != nil {
		t.Fatal(err)
	}
	if !u.IsAdmin() || !u.Enabled || u.InviteID != 0 || !u.CreatedAt.Equal(t0) {
		t.Errorf("created user = %+v", u)
	}
	got, err := s.UserBySub("sub-1")
	if err != nil || got.ID != u.ID {
		t.Fatalf("UserBySub = %+v, %v", got, err)
	}
	if _, err := s.UserBySub("nobody"); !errors.Is(err, ErrNotFound) {
		t.Errorf("UserBySub(unknown) err = %v, want ErrNotFound", err)
	}
	if _, err := s.CreateUser("sub-1", "x@example.com", "X", RoleMember, 0, t0); err == nil {
		t.Error("duplicate sub accepted")
	}
	if in, _ := s.EmailInUse("admin@example.COM"); !in {
		t.Error("EmailInUse is case-sensitive")
	}
}

func TestSessionLifecycle(t *testing.T) {
	s := newStore(t)
	u, _ := s.CreateUser("sub-1", "a@example.com", "A", RoleMember, 0, t0)
	if err := s.CreateSession("h1", u.ID, t0, time.Hour); err != nil {
		t.Fatal(err)
	}

	got, exp, err := s.SessionUser("h1", t0.Add(time.Minute))
	if err != nil || got.ID != u.ID || !exp.Equal(t0.Add(time.Hour)) {
		t.Fatalf("SessionUser = %+v, %v, %v", got, exp, err)
	}
	if _, _, err := s.SessionUser("h1", t0.Add(2*time.Hour)); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired session err = %v, want ErrNotFound", err)
	}

	if err := s.ExtendSession("h1", t0.Add(30*time.Minute), time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, exp, _ := s.SessionUser("h1", t0.Add(time.Minute)); !exp.Equal(t0.Add(90 * time.Minute)) {
		t.Errorf("extended expiry = %v", exp)
	}

	if err := s.DeleteSession("h1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SessionUser("h1", t0); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted session err = %v", err)
	}
}

func TestSessionOfDisabledUserIsRejected(t *testing.T) {
	s := newStore(t)
	u, _ := s.CreateUser("sub-1", "a@example.com", "A", RoleMember, 0, t0)
	s.CreateSession("h1", u.ID, t0, time.Hour)
	if _, err := s.db.Exec(`UPDATE users SET enabled = 0 WHERE id = ?`, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SessionUser("h1", t0); !errors.Is(err, ErrNotFound) {
		t.Errorf("disabled user's session err = %v, want ErrNotFound", err)
	}
}

func TestCreateSessionSweepsExpired(t *testing.T) {
	s := newStore(t)
	u, _ := s.CreateUser("sub-1", "a@example.com", "A", RoleMember, 0, t0)
	s.CreateSession("old", u.ID, t0, time.Hour)
	s.CreateSession("new", u.ID, t0.Add(2*time.Hour), time.Hour)
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n)
	if n != 1 {
		t.Errorf("sessions after sweep = %d, want 1", n)
	}
}
