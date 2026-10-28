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
	if err := s.Lock("2026-11-14", a.ID, true, "", t0); err != nil {
		t.Fatal(err)
	}
	if err := s.Lock("2026-11-14", a.ID, true, "", t0); !errors.Is(err, ErrLocked) {
		t.Errorf("double lock err = %v, want ErrLocked", err)
	}
	l, err := s.LockinFor("2026-11-14")
	if err != nil || l.LockedBy != a.ID || !l.Override || !l.LockedAt.Equal(t0) {
		t.Errorf("LockinFor = %+v, %v", l, err)
	}
	s.Lock("2026-12-01", a.ID, true, "", t0)
	if m, _ := s.Lockins("2026-11-01", "2026-11-30"); len(m) != 1 {
		t.Errorf("Lockins in November = %d, want 1", len(m))
	}
	if err := s.Unlock("2026-11-14", a.ID, t0); err != nil {
		t.Fatal(err)
	}
	if err := s.Unlock("2026-11-14", a.ID, t0); !errors.Is(err, ErrNotFound) {
		t.Errorf("second unlock err = %v, want ErrNotFound", err)
	}
}

func TestChangeStatusOnALockedDay(t *testing.T) {
	s := newStore(t)
	a, _ := s.CreateUser("sub-a", "a@example.com", "A", RoleAdmin, 0, t0)
	s.SetStatus(a.ID, "2026-11-14", "yes", t0)
	s.Lock("2026-11-14", a.ID, false, "", t0)

	// Pulling out needs the confirm, and changes nothing without it.
	if _, err := s.ChangeStatus(a.ID, "2026-11-14", "no", false, t0); !errors.Is(err, ErrNeedsConfirm) {
		t.Fatalf("unconfirmed pull-out err = %v", err)
	}
	if st, _ := s.Status(a.ID, "2026-11-14"); st != "yes" {
		t.Error("status changed without the confirm")
	}
	// Staying available is just a change.
	if c, err := s.ChangeStatus(a.ID, "2026-11-14", "ok", false, t0); err != nil || c {
		t.Errorf("YES -> OK = %v, %v", c, err)
	}
	c, err := s.ChangeStatus(a.ID, "2026-11-14", "no", true, t0)
	if err != nil || !c {
		t.Fatalf("confirmed pull-out = %v, %v", c, err)
	}
	if _, err := s.LockinFor("2026-11-14"); !errors.Is(err, ErrNotFound) {
		t.Error("still locked in after pulling out")
	}
}

// The availability check is part of the lock's own transaction.
func TestLockRechecksAvailability(t *testing.T) {
	s := newStore(t)
	a, _ := s.CreateUser("sub-a", "a@example.com", "A", RoleAdmin, 0, t0)
	j, _ := s.CreateUser("sub-j", "j@example.com", "J", RoleMember, 0, t0)
	s.SetStatus(a.ID, "2026-11-14", "yes", t0)
	s.SetStatus(j.ID, "2026-11-14", "no", t0)
	if err := s.Lock("2026-11-14", a.ID, false, "", t0); !errors.Is(err, ErrNeedsOverride) {
		t.Fatalf("lock with someone out, unconfirmed = %v", err)
	}
	// A disabled member doesn't count.
	s.SetEnabled(j.ID, false)
	if err := s.Lock("2026-11-14", a.ID, false, "", t0); err != nil {
		t.Fatalf("lock with only enabled members in = %v", err)
	}
	if l, _ := s.LockinFor("2026-11-14"); l.Override {
		t.Error("a lock with everyone in was recorded as an override")
	}
}

func TestLockMessage(t *testing.T) {
	s := newStore(t)
	a, _ := s.CreateUser("sub-a", "a@example.com", "Ada", RoleAdmin, 0, t0)
	if err := s.Lock("2026-11-14", a.ID, true, "Locked in for 19:00 to 22:00", t0); err != nil {
		t.Fatal(err)
	}
	if l, _ := s.LockinFor("2026-11-14"); l.Message != "Locked in for 19:00 to 22:00" {
		t.Errorf("message = %q", l.Message)
	}
	before, _ := s.Notices(a.ID, false, "2026-11-14")
	if err := s.SetLockMessage("2026-11-14", "From 8pm at Jo's"); err != nil {
		t.Fatal(err)
	}
	if ls, _ := s.Lockins("2026-11-14", "2026-11-14"); ls["2026-11-14"].Message != "From 8pm at Jo's" {
		t.Errorf("reworded message = %q", ls["2026-11-14"].Message)
	}
	if after, _ := s.Notices(a.ID, false, "2026-11-14"); len(after) != len(before) {
		t.Error("rewording a lock-in notified the group")
	}
	if err := s.SetLockMessage("2026-11-15", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("message on an unlocked day = %v, want ErrNotFound", err)
	}
}
