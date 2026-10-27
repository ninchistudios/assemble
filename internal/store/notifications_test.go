package store

import (
	"testing"
	"time"
)

func TestNoticesFollowLockAndCancel(t *testing.T) {
	s := newStore(t)
	a, _ := s.CreateUser("sub-a", "a@example.com", "Ada", RoleAdmin, 0, t0)
	j, _ := s.CreateUser("sub-j", "j@example.com", "Jo", RoleMember, 0, t0)

	s.Lock("2026-11-20", a.ID, true, t0)
	s.Unlock("2026-11-20", a.ID, t0.Add(time.Minute))
	s.SetStatus(j.ID, "2026-11-21", "yes", t0)
	s.Lock("2026-11-21", a.ID, true, t0.Add(2*time.Minute))
	s.ChangeStatus(j.ID, "2026-11-21", "no", true, t0.Add(3*time.Minute))

	got, err := s.Notices(j.ID, "2026-11-14")
	if err != nil || len(got) != 4 {
		t.Fatalf("Notices = %+v, %v", got, err)
	}
	// Newest first: Jo's pull-out cancelled the 21st.
	if got[0].Kind != NoticeCancelled || got[0].Day != "2026-11-21" || got[0].ActorName != "Jo" {
		t.Errorf("newest notice = %+v", got[0])
	}
	if got[3].Kind != NoticeLocked || got[3].ActorName != "Ada" {
		t.Errorf("oldest notice = %+v", got[3])
	}

	// A failed lock (already locked) adds nothing.
	s.Lock("2026-11-22", a.ID, true, t0)
	s.Lock("2026-11-22", a.ID, true, t0)
	if n, _ := s.Notices(j.ID, "2026-11-22"); len(n) != 1 {
		t.Errorf("double lock made %d notices for the day, want 1", len(n))
	}
}

func TestDismissIsPerPersonAndNoticesExpire(t *testing.T) {
	s := newStore(t)
	a, _ := s.CreateUser("sub-a", "a@example.com", "Ada", RoleAdmin, 0, t0)
	j, _ := s.CreateUser("sub-j", "j@example.com", "Jo", RoleMember, 0, t0)
	s.Lock("2026-11-20", a.ID, true, t0)
	n, _ := s.Notices(j.ID, "2026-11-14")

	if err := s.Dismiss(n[0].ID, j.ID); err != nil {
		t.Fatal(err)
	}
	s.Dismiss(n[0].ID, j.ID) // twice is fine
	s.Dismiss(999, j.ID)     // so is a notice that doesn't exist
	if got, _ := s.Notices(j.ID, "2026-11-14"); len(got) != 0 {
		t.Error("dismissed notice still shown to Jo")
	}
	if got, _ := s.Notices(a.ID, "2026-11-14"); len(got) != 1 {
		t.Error("Jo's dismissal hid it from Ada too")
	}
	if got, _ := s.Notices(a.ID, "2026-11-21"); len(got) != 0 {
		t.Error("notice for a past day still shown")
	}
}

// #32: notices named the actor by their Google name, not their chosen one.
func TestNoticesUseDisplayName(t *testing.T) {
	s := newStore(t)
	a, _ := s.CreateUser("sub-a", "a@example.com", "Ada Lovelace", RoleAdmin, 0, t0)
	s.SetDisplayName(a.ID, "Ada")
	s.Lock("2026-11-20", a.ID, true, t0)
	n, _ := s.Notices(a.ID, "2026-11-14")
	if len(n) != 1 || n[0].ActorName != "Ada" {
		t.Errorf("notice actor = %+v, want display name Ada", n)
	}
}
