package store

import (
	"testing"
	"time"
)

func TestNoticesFollowLockAndCancel(t *testing.T) {
	s := newStore(t)
	a, _ := s.CreateUser("sub-a", "a@example.com", "Ada", RoleAdmin, 0, t0)
	j, _ := s.CreateUser("sub-j", "j@example.com", "Jo", RoleMember, 0, t0)

	s.Lock("2026-11-20", a.ID, true, "", t0)
	s.Unlock("2026-11-20", a.ID, t0.Add(time.Minute))
	s.SetStatus(j.ID, "2026-11-21", "yes", t0)
	s.Lock("2026-11-21", a.ID, true, "", t0.Add(2*time.Minute))
	s.ChangeStatus(j.ID, "2026-11-21", "no", true, t0.Add(3*time.Minute))

	got, err := s.Notices(j.ID, false, "2026-11-14")
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
	s.Lock("2026-11-22", a.ID, true, "", t0)
	s.Lock("2026-11-22", a.ID, true, "", t0)
	if n, _ := s.Notices(j.ID, false, "2026-11-22"); len(n) != 1 {
		t.Errorf("double lock made %d notices for the day, want 1", len(n))
	}
}

func TestDismissIsPerPersonAndNoticesExpire(t *testing.T) {
	s := newStore(t)
	a, _ := s.CreateUser("sub-a", "a@example.com", "Ada", RoleAdmin, 0, t0)
	j, _ := s.CreateUser("sub-j", "j@example.com", "Jo", RoleMember, 0, t0)
	s.Lock("2026-11-20", a.ID, true, "", t0)
	n, _ := s.Notices(j.ID, false, "2026-11-14")

	if err := s.Dismiss(n[0].ID, j.ID); err != nil {
		t.Fatal(err)
	}
	s.Dismiss(n[0].ID, j.ID) // twice is fine
	s.Dismiss(999, j.ID)     // so is a notice that doesn't exist
	if got, _ := s.Notices(j.ID, false, "2026-11-14"); len(got) != 0 {
		t.Error("dismissed notice still shown to Jo")
	}
	if got, _ := s.Notices(a.ID, false, "2026-11-14"); len(got) != 1 {
		t.Error("Jo's dismissal hid it from Ada too")
	}
	if got, _ := s.Notices(a.ID, false, "2026-11-21"); len(got) != 0 {
		t.Error("notice for a past day still shown")
	}
}

// #32: notices named the actor by their Google name, not their chosen one.
func TestNoticesUseDisplayName(t *testing.T) {
	s := newStore(t)
	a, _ := s.CreateUser("sub-a", "a@example.com", "Ada Lovelace", RoleAdmin, 0, t0)
	s.SetDisplayName(a.ID, "Ada")
	s.Lock("2026-11-20", a.ID, true, "", t0)
	n, _ := s.Notices(a.ID, false, "2026-11-14")
	if len(n) != 1 || n[0].ActorName != "Ada" {
		t.Errorf("notice actor = %+v, want display name Ada", n)
	}
}

// A join is told to the admin and managers only, and never expires (#38).
func TestJoinNoticeIsForManagers(t *testing.T) {
	s := newStore(t)
	a, _ := s.CreateUser("sub-a", "a@example.com", "Ada", RoleAdmin, 0, t0)
	inv, _ := s.CreateInvite("tok", a.ID, t0)
	sam, err := s.JoinViaInvite("tok", "sub-s", "s@example.com", "Sam", t0)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.Notices(a.ID, true, "2027-06-01") // long after: still there
	if len(got) != 1 || got[0].Kind != NoticeJoined || got[0].ActorName != "Sam" || got[0].Day != "" {
		t.Fatalf("manager's notices = %+v", got)
	}
	if got, _ := s.Notices(sam.ID, false, "2026-11-14"); len(got) != 0 {
		t.Errorf("a plain member sees the join notice: %+v", got)
	}
	// A refused join tells nobody.
	s.DisableInvite(inv.ID, t0)
	s.JoinViaInvite("tok", "sub-x", "x@example.com", "Xi", t0)
	if got, _ := s.Notices(a.ID, true, "2026-11-14"); len(got) != 1 {
		t.Errorf("refused join made a notice: %+v", got)
	}
}

// Only the latest MOTD shows; a new one shows again to everyone (#40).
func TestMOTD(t *testing.T) {
	s := newStore(t)
	a, _ := s.CreateUser("sub-a", "a@example.com", "Ada", RoleAdmin, 0, t0)
	j, _ := s.CreateUser("sub-j", "j@example.com", "Jo", RoleMember, 0, t0)

	if changed, err := s.SetMOTD("Gig on the 30th!", a.ID, t0); err != nil || !changed {
		t.Fatalf("SetMOTD = %v, %v", changed, err)
	}
	got, _ := s.Notices(j.ID, false, "2027-06-01")
	if len(got) != 1 || got[0].Kind != NoticeMOTD || got[0].Body != "Gig on the 30th!" {
		t.Fatalf("notices = %+v", got)
	}
	s.Dismiss(got[0].ID, j.ID)
	// The same text again doesn't bring it back.
	if changed, _ := s.SetMOTD("Gig on the 30th!", a.ID, t0); changed {
		t.Error("saving the same MOTD counted as a change")
	}
	if got, _ := s.Notices(j.ID, false, "2026-11-14"); len(got) != 0 {
		t.Error("unchanged MOTD reappeared after dismissal")
	}
	// A new one does, and replaces the old for everyone.
	s.SetMOTD("Gig moved to the 31st", a.ID, t0.Add(time.Minute))
	got, _ = s.Notices(a.ID, false, "2026-11-14")
	if len(got) != 1 || got[0].Body != "Gig moved to the 31st" {
		t.Fatalf("after replacing: %+v", got)
	}
	if m, _ := s.MOTD(); m != "Gig moved to the 31st" {
		t.Errorf("MOTD = %q", m)
	}
	// Empty clears it.
	s.SetMOTD("", a.ID, t0)
	if got, _ := s.Notices(j.ID, false, "2026-11-14"); len(got) != 0 {
		t.Errorf("cleared MOTD still shows: %+v", got)
	}
	if m, _ := s.MOTD(); m != "" {
		t.Errorf("MOTD after clearing = %q", m)
	}
}
