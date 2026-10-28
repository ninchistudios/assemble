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
	// The form names who's missing and marks itself for the script's
	// confirm, but doesn't carry the override itself: a plain form post must
	// get the server's confirm page.
	if !strings.Contains(body, "Member jo (?)") || !strings.Contains(body, "data-override") {
		t.Error("lock form doesn't name who's missing or mark the override")
	}
	if strings.Contains(body, `name="override"`) {
		t.Error("lock form carries the override before anyone confirmed")
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

// The lock-in form is pre-filled from the timeslot; what's saved is what the
// manager left there, and it replaces the group line on the day (#41).
func TestLockInMessage(t *testing.T) {
	e, admin := dayEnv(t)
	_, jo := e.memberSession(t, "jo")
	e.do(t, http.MethodPost, "/day/2026-11-20/status", url.Values{"status": {"yes"}}, admin)
	e.do(t, http.MethodPost, "/day/2026-11-20/status", url.Values{"status": {"yes"}}, jo)

	body := e.get(t, "/day/2026-11-20", admin).Body.String()
	if !strings.Contains(body, `name="message" type="text" maxlength="100" value="Locked in for 19:00 to 22:00"`) {
		t.Fatal("lock-in form isn't pre-filled from the timeslot")
	}
	e.do(t, http.MethodPost, "/day/2026-11-20/lock", url.Values{"message": {"  From 8pm   at the studio "}}, admin)
	body = e.get(t, "/day/2026-11-20", jo).Body.String()
	if !strings.Contains(body, `class="summary locked">From 8pm at the studio<`) || strings.Contains(body, "Ready to lock in!") {
		t.Error("the lock-in message doesn't replace the group line")
	}
	// It stays the group line when an answer changes without pulling out.
	if _, got := setStatusJSON(e, jo, "ok", false); got["summary"] != "From 8pm at the studio" {
		t.Errorf("summary after a change = %v", got["summary"])
	}

	// Reworded by a manager: no notice for anyone.
	notices := len(noticeIDs(e.get(t, "/", jo).Body.String()))
	if rec := e.do(t, http.MethodPost, "/day/2026-11-20/message", url.Values{"message": {"From 8:30pm"}}, admin); rec.Code != http.StatusSeeOther {
		t.Fatalf("reword = %d", rec.Code)
	}
	if body := e.get(t, "/day/2026-11-20", jo).Body.String(); !strings.Contains(body, ">From 8:30pm<") {
		t.Error("reworded message not shown")
	}
	if n := len(noticeIDs(e.get(t, "/", jo).Body.String())); n != notices {
		t.Errorf("rewording made notices: %d, was %d", n, notices)
	}
	if rec := e.do(t, http.MethodPost, "/day/2026-11-20/message", url.Values{"message": {"mine now"}}, jo); rec.Code != http.StatusForbidden {
		t.Errorf("member rewording = %d, want 403", rec.Code)
	}
	if rec := e.do(t, http.MethodPost, "/day/2026-11-21/message", url.Values{"message": {"x"}}, admin); rec.Code != http.StatusConflict {
		t.Errorf("rewording an unlocked day = %d, want 409", rec.Code)
	}
	if rec := e.do(t, http.MethodPost, "/day/2026-11-20/message", url.Values{"message": {strings.Repeat("x", 101)}}, admin); rec.Code != http.StatusBadRequest {
		t.Errorf("101-character message = %d, want 400", rec.Code)
	}
	e.db.SaveSettings(store.Settings{GroupName: "Band", SlotStart: "19:00", SlotEnd: "22:00", Timezone: "UTC", BannedWords: "heck"}, fixedNow)
	if rec := e.do(t, http.MethodPost, "/day/2026-11-20/message", url.Values{"message": {"what the heck"}}, admin); rec.Code != http.StatusBadRequest {
		t.Errorf("filtered message = %d, want 400", rec.Code)
	}
}

// An override's confirm page carries the message through.
func TestLockInMessageSurvivesOverrideConfirm(t *testing.T) {
	e, admin := dayEnv(t)
	e.memberSession(t, "jo")
	rec := e.do(t, http.MethodPost, "/day/2026-11-20/lock", url.Values{"message": {"Short one, 8 to 9"}}, admin)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `name="message" value="Short one, 8 to 9"`) {
		t.Fatalf("override confirm = %d, without the message", rec.Code)
	}
	e.do(t, http.MethodPost, "/day/2026-11-20/lock", url.Values{"override": {"1"}, "message": {"Short one, 8 to 9"}}, admin)
	if l, _ := e.db.LockinFor("2026-11-20"); l.Message != "Short one, 8 to 9" {
		t.Errorf("message = %q", l.Message)
	}
}
