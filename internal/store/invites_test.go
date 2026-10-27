package store

import (
	"errors"
	"testing"
	"time"
)

func TestInviteLifecycle(t *testing.T) {
	s := newStore(t)
	admin, _ := s.CreateUser("sub-a", "a@example.com", "A", RoleAdmin, 0, t0)
	first, err := s.CreateInvite("tok-1", admin.ID, t0)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := s.CreateInvite("tok-2", admin.ID, t0.Add(time.Hour))
	if _, err := s.CreateInvite("tok-1", admin.ID, t0); err == nil {
		t.Error("duplicate token accepted")
	}

	got, err := s.ActiveInvite("tok-1")
	if err != nil || got.ID != first.ID || !got.Active() {
		t.Fatalf("ActiveInvite = %+v, %v", got, err)
	}

	s.CreateUser("sub-m1", "m1@example.com", "M1", RoleMember, first.ID, t0)
	s.CreateUser("sub-m2", "m2@example.com", "M2", RoleMember, first.ID, t0)

	if err := s.DisableInvite(first.ID, t0.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ActiveInvite("tok-1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("disabled invite still active: %v", err)
	}
	// A second disable keeps the first date.
	s.DisableInvite(first.ID, t0.Add(5*time.Hour))

	list, err := s.Invites()
	if err != nil || len(list) != 2 {
		t.Fatalf("Invites = %v, %v", list, err)
	}
	if list[0].ID != second.ID || !list[0].Active() {
		t.Errorf("active invite not listed first: %+v", list[0])
	}
	if list[1].Joined != 2 || !list[1].DisabledAt.Equal(t0.Add(2*time.Hour)) {
		t.Errorf("disabled invite = %+v", list[1])
	}
}

func TestJoinViaInviteOnlyWhileActive(t *testing.T) {
	s := newStore(t)
	admin, _ := s.CreateUser("sub-a", "a@example.com", "A", RoleAdmin, 0, t0)
	inv, _ := s.CreateInvite("tok", admin.ID, t0)

	u, err := s.JoinViaInvite("tok", "sub-m", "m@example.com", "M", t0)
	if err != nil || u.InviteID != inv.ID || u.Role != RoleMember {
		t.Fatalf("JoinViaInvite = %+v, %v", u, err)
	}
	s.DisableInvite(inv.ID, t0)
	if _, err := s.JoinViaInvite("tok", "sub-n", "n@example.com", "N", t0); !errors.Is(err, ErrNotFound) {
		t.Errorf("join through disabled invite err = %v, want ErrNotFound", err)
	}
	if _, err := s.JoinViaInvite("nope", "sub-o", "o@example.com", "O", t0); !errors.Is(err, ErrNotFound) {
		t.Errorf("join through unknown token err = %v, want ErrNotFound", err)
	}
	if _, err := s.UserBySub("sub-n"); err == nil {
		t.Error("user created through a disabled invite")
	}
}
