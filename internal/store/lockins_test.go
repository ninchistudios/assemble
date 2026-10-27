package store

import (
	"errors"
	"testing"
)

func TestLockinLifecycle(t *testing.T) {
	s := newStore(t)
	a, _ := s.CreateUser("sub-a", "a@example.com", "A", RoleAdmin, 0, t0)

	if _, err := s.LockinFor("2026-11-14"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unlocked day err = %v", err)
	}
	if err := s.Lock("2026-11-14", a.ID, true, t0); err != nil {
		t.Fatal(err)
	}
	if err := s.Lock("2026-11-14", a.ID, false, t0); !errors.Is(err, ErrLocked) {
		t.Errorf("double lock err = %v, want ErrLocked", err)
	}
	l, err := s.LockinFor("2026-11-14")
	if err != nil || l.LockedBy != a.ID || !l.Override || !l.LockedAt.Equal(t0) {
		t.Errorf("LockinFor = %+v, %v", l, err)
	}
	s.Lock("2026-12-01", a.ID, false, t0)
	if m, _ := s.Lockins("2026-11-01", "2026-11-30"); len(m) != 1 {
		t.Errorf("Lockins in November = %d, want 1", len(m))
	}
	if err := s.Unlock("2026-11-14"); err != nil {
		t.Fatal(err)
	}
	if err := s.Unlock("2026-11-14"); !errors.Is(err, ErrNotFound) {
		t.Errorf("second unlock err = %v, want ErrNotFound", err)
	}
}
