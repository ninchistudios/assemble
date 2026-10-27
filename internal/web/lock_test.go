package web

import (
	"encoding/json"
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

// lockedDay sets up 2026-11-20 locked in with the admin and Jo at the given
// statuses, returning Jo's session.
func lockedDay(t *testing.T, e *testEnv, admin *http.Cookie, adminSt, joSt string) *http.Cookie {
	t.Helper()
	_, jo := e.memberSession(t, "jo")
	e.do(t, http.MethodPost, "/day/2026-11-20/status", url.Values{"status": {adminSt}}, admin)
	if joSt != "unknown" {
		e.do(t, http.MethodPost, "/day/2026-11-20/status", url.Values{"status": {joSt}}, jo)
	}
	e.do(t, http.MethodPost, "/day/2026-11-20/lock", url.Values{"override": {"1"}}, admin)
	if _, err := e.db.LockinFor("2026-11-20"); err != nil {
		t.Fatal("setup didn't lock the day")
	}
	return jo
}

func setStatusJSON(e *testEnv, c *http.Cookie, status string, confirm bool) (int, map[string]any) {
	form := url.Values{"status": {status}}
	if confirm {
		form.Set("confirm", "1")
	}
	req := httptestForm("/day/2026-11-20/status", testBase, form, c)
	req.Header.Set("Accept", "application/json")
	rec := e.serve(req)
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body
}

func TestPullingOutAsksThenCancels(t *testing.T) {
	e, admin := dayEnv(t)
	jo := lockedDay(t, e, admin, "yes", "ok")

	code, body := setStatusJSON(e, jo, "no", false)
	if code != http.StatusConflict || body["confirm"] != pullOutWarning {
		t.Fatalf("pull out without confirm = %d %v", code, body)
	}
	joUser, _ := e.db.UserBySub("jo")
	if st, _ := e.db.Status(joUser.ID, "2026-11-20"); st != "ok" {
		t.Error("status changed before the confirm")
	}
	if _, err := e.db.LockinFor("2026-11-20"); err != nil {
		t.Error("lock-in gone before the confirm")
	}

	code, body = setStatusJSON(e, jo, "no", true)
	if code != http.StatusOK || body["cancelled"] != true {
		t.Fatalf("confirmed pull out = %d %v", code, body)
	}
	if _, err := e.db.LockinFor("2026-11-20"); !errors.Is(err, store.ErrNotFound) {
		t.Error("still locked in after pulling out")
	}
}

func TestOnlyPullingOutCancels(t *testing.T) {
	e, admin := dayEnv(t)
	jo := lockedDay(t, e, admin, "yes", "no") // override: Jo was already NO

	for _, tc := range []struct {
		who    *http.Cookie
		status string
	}{
		{jo, "unknown"}, // NO -> ?: not pulling out
		{admin, "ok"},   // YES -> OK: still available
	} {
		if code, body := setStatusJSON(e, tc.who, tc.status, false); code != http.StatusOK || body["cancelled"] != false {
			t.Errorf("change to %s = %d %v", tc.status, code, body)
		}
	}
	if _, err := e.db.LockinFor("2026-11-20"); err != nil {
		t.Error("a change that isn't pulling out cancelled the day")
	}
}

// Without JS the server shows its own confirm page.
func TestPullingOutByFormGetsAConfirmPage(t *testing.T) {
	e, admin := dayEnv(t)
	jo := lockedDay(t, e, admin, "yes", "yes")
	rec := e.do(t, http.MethodPost, "/day/2026-11-20/status", url.Values{"status": {"no"}}, jo)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `name="confirm" value="1"`) {
		t.Fatalf("form pull out = %d", rec.Code)
	}
	rec = e.do(t, http.MethodPost, "/day/2026-11-20/status", url.Values{"status": {"no"}, "confirm": {"1"}}, jo)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("confirmed form pull out = %d", rec.Code)
	}
	if _, err := e.db.LockinFor("2026-11-20"); !errors.Is(err, store.ErrNotFound) {
		t.Error("still locked in")
	}
}
