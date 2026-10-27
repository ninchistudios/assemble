package store

import (
	"testing"
	"time"

	"github.com/ninchistudios/assemble/internal/cal"
)

func TestAvailabilitySetClearRead(t *testing.T) {
	s := newStore(t)
	a, _ := s.CreateUser("sub-a", "a@example.com", "A", RoleMember, 0, t0)
	b, _ := s.CreateUser("sub-b", "b@example.com", "B", RoleMember, 0, t0)

	if st, err := s.Status(a.ID, "2026-11-14"); err != nil || st != cal.Unknown {
		t.Fatalf("unanswered = %q, %v", st, err)
	}
	s.SetStatus(a.ID, "2026-11-14", cal.Yes, t0)
	s.SetStatus(a.ID, "2026-11-14", cal.OK, t0.Add(time.Minute)) // overwrite, not duplicate
	s.SetStatus(b.ID, "2026-11-14", cal.No, t0)
	s.SetStatus(a.ID, "2026-11-15", cal.Yes, t0)
	s.SetStatus(a.ID, "2026-12-31", cal.Yes, t0)

	if st, _ := s.Status(a.ID, "2026-11-14"); st != cal.OK {
		t.Errorf("after overwrite = %q, want ok", st)
	}
	got, err := s.Answers("2026-11-14", "2026-11-30")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || len(got["2026-11-14"]) != 2 || got["2026-11-14"][b.ID].Status != cal.No {
		t.Errorf("Answers = %+v", got)
	}
	if _, ok := got["2026-12-31"]; ok {
		t.Error("Answers returned a day outside the range")
	}

	// Back to [?] removes the row.
	if err := s.SetStatus(a.ID, "2026-11-14", cal.Unknown, t0); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.Status(a.ID, "2026-11-14"); st != cal.Unknown {
		t.Errorf("after clearing = %q", st)
	}
}

func TestAvailabilityRejectsBadStatus(t *testing.T) {
	s := newStore(t)
	a, _ := s.CreateUser("sub-a", "a@example.com", "A", RoleMember, 0, t0)
	if err := s.SetStatus(a.ID, "2026-11-14", cal.Status("maybe"), t0); err == nil {
		t.Error("the schema accepted an invalid status")
	}
}
