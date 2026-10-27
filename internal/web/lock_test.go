package web

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/ninchistudios/assemble/internal/store"
)

func TestLockWhenEveryoneAvailable(t *testing.T) {
	e, admin := dayEnv(t)
	_, jo := e.memberSession(t, "jo")
	e.do(t, http.MethodPost, "/day/2026-11-20/status", url.Values{"status": {"yes"}}, admin)
	e.do(t, http.MethodPost, "/day/2026-11-20/status", url.Values{"status": {"ok"}}, jo)

	body := e.get(t, "/day/2026-11-20", admin).Body.String()
	if !strings.Contains(body, `action="/day/2026-11-20/lock">`) || strings.Contains(body, "Lock in anyway?") {
		t.Error("admin isn't offered a plain lock-in when everyone is available")
	}
	if rec := e.do(t, http.MethodPost, "/day/2026-11-20/lock", url.Values{}, admin); rec.Code != http.StatusSeeOther {
		t.Fatalf("lock = %d", rec.Code)
	}
	l, err := e.db.LockinFor("2026-11-20")
	if err != nil || l.Override {
		t.Fatalf("lock-in = %+v, %v", l, err)
	}
	// Everyone sees it; only the admin can cancel.
	if body := e.get(t, "/day/2026-11-20", jo).Body.String(); !strings.Contains(body, "Locked in.") || strings.Contains(body, "/cancel") {
		t.Error("member doesn't see the lock-in, or is offered cancel")
	}
}

func TestLockNeedsOverrideConfirm(t *testing.T) {
	e, admin := dayEnv(t)
	e.memberSession(t, "jo") // jo hasn't answered: [?]
	e.do(t, http.MethodPost, "/day/2026-11-20/status", url.Values{"status": {"yes"}}, admin)

	body := e.get(t, "/day/2026-11-20", admin).Body.String()
	if !strings.Contains(body, "Member jo (?)") || !strings.Contains(body, `name="override" value="1"`) {
		t.Error("lock form doesn't name who's missing or carry the override")
	}
	// Without the override the server asks first.
	rec := e.do(t, http.MethodPost, "/day/2026-11-20/lock", url.Values{}, admin)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "Lock in anyway") {
		t.Fatalf("lock without override = %d", rec.Code)
	}
	if _, err := e.db.LockinFor("2026-11-20"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("locked without the override")
	}
	e.do(t, http.MethodPost, "/day/2026-11-20/lock", url.Values{"override": {"1"}}, admin)
	if l, err := e.db.LockinFor("2026-11-20"); err != nil || !l.Override {
		t.Errorf("override lock-in = %+v, %v", l, err)
	}
	if body := e.get(t, "/day/2026-11-20", admin).Body.String(); !strings.Contains(body, "not everyone said they could") {
		t.Error("override lock-in isn't marked as such")
	}
}

func TestCancelLockIn(t *testing.T) {
	e, admin := dayEnv(t)
	e.do(t, http.MethodPost, "/day/2026-11-20/status", url.Values{"status": {"yes"}}, admin)
	e.do(t, http.MethodPost, "/day/2026-11-20/lock", url.Values{}, admin)
	if rec := e.do(t, http.MethodPost, "/day/2026-11-20/cancel", url.Values{}, admin); rec.Code != http.StatusSeeOther {
		t.Fatalf("cancel = %d", rec.Code)
	}
	if _, err := e.db.LockinFor("2026-11-20"); !errors.Is(err, store.ErrNotFound) {
		t.Error("still locked after cancel")
	}
}

func TestLockIsAdminOnlyAndNotInThePast(t *testing.T) {
	e, admin := dayEnv(t)
	_, jo := e.memberSession(t, "jo")
	if rec := e.do(t, http.MethodPost, "/day/2026-11-20/lock", url.Values{"override": {"1"}}, jo); rec.Code != http.StatusForbidden {
		t.Errorf("member lock = %d, want 403", rec.Code)
	}
	if rec := e.do(t, http.MethodPost, "/day/2026-11-13/lock", url.Values{"override": {"1"}}, admin); rec.Code != http.StatusConflict {
		t.Errorf("lock in the past = %d, want 409", rec.Code)
	}
	if body := e.get(t, "/day/2026-11-13", admin).Body.String(); strings.Contains(body, "/lock") {
		t.Error("a past day offers lock-in")
	}
}
