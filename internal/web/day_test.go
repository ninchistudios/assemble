package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ninchistudios/assemble/internal/cal"
)

// fixedNow is 10:00 UTC on Saturday 14 November 2026; the test group is on
// UTC, so today is 2026-11-14 and the horizon is 2027-01-14.
var fixedNow = time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)

func dayEnv(t *testing.T) (*testEnv, *http.Cookie) {
	t.Helper()
	e := newEnv(t)
	e.s.now = func() time.Time { return fixedNow }
	return e, e.session(t, adminID)
}

func pressed(body, value string) bool {
	return strings.Contains(body, `value="`+value+`" aria-pressed="true"`)
}

func TestHomeIsToday(t *testing.T) {
	e, c := dayEnv(t)
	body := e.get(t, "/", c).Body.String()
	for _, want := range []string{"Saturday 14 November", ">today<", `action="/day/2026-11-14/status"`, `href="/day/2026-11-13"`, `href="/day/2026-11-15"`} {
		if !strings.Contains(body, want) {
			t.Errorf("home missing %q", want)
		}
	}
	if !pressed(body, "unknown") {
		t.Error("unanswered day doesn't show [?] selected")
	}
}

func TestSetStatusByScript(t *testing.T) {
	e, c := dayEnv(t)
	req := httptestForm("/day/2026-11-20/status", testBase, url.Values{"status": {"yes"}}, c)
	req.Header.Set("Accept", "application/json")
	rec := e.serve(req)
	var got map[string]string
	json.Unmarshal(rec.Body.Bytes(), &got)
	if rec.Code != http.StatusOK || got["status"] != "yes" {
		t.Fatalf("JSON set = %d %v", rec.Code, got)
	}
	if body := e.get(t, "/day/2026-11-20", c).Body.String(); !pressed(body, "yes") {
		t.Error("saved status not shown as selected")
	}
}

func TestSetStatusByForm(t *testing.T) {
	e, c := dayEnv(t)
	rec := e.do(t, http.MethodPost, "/day/2026-11-14/status", url.Values{"status": {"ok"}}, c)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/day/2026-11-14" {
		t.Fatalf("form set = %d -> %s", rec.Code, rec.Header().Get("Location"))
	}
	// Back to [?] clears it.
	e.do(t, http.MethodPost, "/day/2026-11-14/status", url.Values{"status": {"unknown"}}, c)
	if body := e.get(t, "/", c).Body.String(); !pressed(body, "unknown") {
		t.Error("clearing didn't return the day to [?]")
	}
}

func TestPastDaysAreReadOnly(t *testing.T) {
	e, c := dayEnv(t)
	body := e.get(t, "/day/2026-11-13", c).Body.String()
	if !strings.Contains(body, "has passed") || !strings.Contains(body, "disabled") {
		t.Error("past day isn't shown read-only")
	}
	rec := e.do(t, http.MethodPost, "/day/2026-11-13/status", url.Values{"status": {"yes"}}, c)
	if rec.Code != http.StatusConflict {
		t.Errorf("setting a past day = %d, want 409", rec.Code)
	}
	u, _ := e.db.UserBySub(adminID.Sub)
	if st, _ := e.db.Status(u.ID, "2026-11-13"); st != cal.Unknown {
		t.Error("past day was changed")
	}
}

func TestHorizon(t *testing.T) {
	e, c := dayEnv(t)
	last := e.get(t, "/day/2027-01-14", c)
	if last.Code != http.StatusOK || strings.Contains(last.Body.String(), `rel="next"`) {
		t.Errorf("last day = %d, or offers a next day", last.Code)
	}
	if rec := e.get(t, "/day/2027-01-15", c); rec.Code != http.StatusNotFound {
		t.Errorf("beyond the horizon = %d, want 404", rec.Code)
	}
	rec := e.do(t, http.MethodPost, "/day/2027-01-15/status", url.Values{"status": {"yes"}}, c)
	if rec.Code != http.StatusNotFound {
		t.Errorf("setting beyond the horizon = %d, want 404", rec.Code)
	}
}

func TestBadDayAndStatus(t *testing.T) {
	e, c := dayEnv(t)
	for _, p := range []string{"/day/tomorrow", "/day/2026-02-30", "/day/2026-11-5"} {
		if rec := e.get(t, p, c); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", p, rec.Code)
		}
	}
	if rec := e.do(t, http.MethodPost, "/day/2026-11-14/status", url.Values{"status": {"maybe"}}, c); rec.Code != http.StatusBadRequest {
		t.Errorf("bad status = %d, want 400", rec.Code)
	}
}

func TestDayNeedsSignIn(t *testing.T) {
	e, _ := dayEnv(t)
	if rec := e.get(t, "/day/2026-11-14"); rec.Code != http.StatusUnauthorized {
		t.Errorf("signed-out day = %d", rec.Code)
	}
	if rec := e.do(t, http.MethodPost, "/day/2026-11-14/status", url.Values{"status": {"yes"}}); rec.Code != http.StatusUnauthorized {
		t.Errorf("signed-out set = %d", rec.Code)
	}
}

// Today follows the group's timezone: at 23:30 UTC on the 14th it is already
// the 15th in Brisbane.
func TestTodayFollowsGroupTimezone(t *testing.T) {
	e, c := dayEnv(t)
	e.s.now = func() time.Time { return time.Date(2026, 11, 14, 23, 30, 0, 0, time.UTC) }
	st, _ := e.db.Settings()
	st.Timezone = "Australia/Brisbane"
	e.db.SaveSettings(st, fixedNow)
	if body := e.get(t, "/", c).Body.String(); !strings.Contains(body, "Sunday 15 November") {
		t.Error("home isn't today in the group timezone")
	}
}

func TestGroupStatusForADay(t *testing.T) {
	e, admin := dayEnv(t)
	jo, joC := e.memberSession(t, "jo")
	sam, _ := e.memberSession(t, "sam")
	gone, _ := e.memberSession(t, "gone")
	e.do(t, http.MethodPost, "/day/2026-11-20/status", url.Values{"status": {"yes"}}, admin)
	e.do(t, http.MethodPost, "/day/2026-11-20/status", url.Values{"status": {"no"}}, joC)
	e.db.SetStatus(gone.ID, "2026-11-20", cal.Yes, fixedNow)
	e.db.SetEnabled(gone.ID, false)

	// Jo sees everyone enabled, Jo first, Sam as [?], and nothing of Gone.
	body := e.get(t, "/day/2026-11-20", joC).Body.String()
	if !strings.Contains(body, "1 can&#39;t make it") {
		t.Errorf("summary wrong:\n%s", body)
	}
	i := strings.Index
	if !(i(body, jo.Name) < i(body, "Ada") && i(body, "Ada") < i(body, sam.Name)) {
		t.Error("the viewer isn't listed first, or the others are out of join order")
	}
	if strings.Contains(body, gone.Name) {
		t.Error("a disabled member is listed")
	}
	if !strings.Contains(body, `data-me data-status="no"`) {
		t.Error("Jo's own row doesn't carry Jo's status")
	}
}

func TestNotes(t *testing.T) {
	e, admin := dayEnv(t)
	_, jo := e.memberSession(t, "jo")
	note := func(c *http.Cookie, day, text string) int {
		return e.do(t, http.MethodPost, "/day/"+day+"/note", url.Values{"note": {text}}, c).Code
	}

	if code := note(jo, "2026-11-20", "late"); code != http.StatusConflict {
		t.Errorf("note before a status = %d, want 409", code)
	}
	e.do(t, http.MethodPost, "/day/2026-11-20/status", url.Values{"status": {"ok"}}, jo)
	if code := note(jo, "2026-11-20", "  not until   8pm <b>sorry</b> "); code != http.StatusSeeOther {
		t.Fatalf("note = %d", code)
	}
	body := e.get(t, "/day/2026-11-20", admin).Body.String()
	if !strings.Contains(body, "not until 8pm &lt;b&gt;sorry&lt;/b&gt;") {
		t.Error("the group doesn't see the note, tidied and escaped")
	}
	if code := note(jo, "2026-11-20", strings.Repeat("x", 141)); code != http.StatusBadRequest {
		t.Errorf("overlong note = %d, want 400", code)
	}
	if code := note(jo, "2026-11-13", "too late"); code != http.StatusConflict {
		t.Errorf("note on a past day = %d, want 409", code)
	}

	// The note field is offered only once there's an answer.
	if body := e.get(t, "/day/2026-11-21", jo).Body.String(); !strings.Contains(body, `class="note-form" method="post" action="/day/2026-11-21/note" hidden`) {
		t.Error("note field shown before any answer")
	}
}

func TestSummaryPhrases(t *testing.T) {
	m := func(me bool, st cal.Status) memberStatus { return memberStatus{Me: me, Status: st} }
	for _, tc := range []struct {
		name  string
		group []memberStatus
		want  string
	}{
		{"someone out", []memberStatus{m(true, cal.Yes), m(false, cal.No), m(false, cal.No), m(false, cal.Unknown)}, "2 can't make it"},
		{"me out", []memberStatus{m(true, cal.No), m(false, cal.Yes)}, "1 can't make it"},
		{"everyone in", []memberStatus{m(true, cal.OK), m(false, cal.Yes)}, "Ready to lock in!"},
		{"just me, in", []memberStatus{m(true, cal.Yes)}, "Ready to lock in!"},
		{"all others in", []memberStatus{m(true, cal.Unknown), m(false, cal.Yes), m(false, cal.OK)}, "All others are in, great option!"},
		{"some unsure", []memberStatus{m(true, cal.Yes), m(false, cal.Yes), m(false, cal.Unknown)}, "2 are in, it might work..."},
		{"one in", []memberStatus{m(true, cal.Unknown), m(false, cal.Yes), m(false, cal.Unknown)}, "1 is in, it might work..."},
		{"nobody yet", []memberStatus{m(true, cal.Unknown), m(false, cal.Unknown)}, "No answers yet"},
		{"just me, unsure", []memberStatus{m(true, cal.Unknown)}, "No answers yet"},
	} {
		if got := summarise(tc.group); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

// The script shows the server's phrase rather than working it out itself.
func TestStatusChangeReturnsSummary(t *testing.T) {
	e, admin := dayEnv(t)
	req := httptestForm("/day/2026-11-20/status", testBase, url.Values{"status": {"yes"}}, admin)
	req.Header.Set("Accept", "application/json")
	var got map[string]any
	json.Unmarshal(e.serve(req).Body.Bytes(), &got)
	if got["summary"] != "Ready to lock in!" {
		t.Errorf("summary = %v", got["summary"])
	}
}

func TestDayHasUpcomingButton(t *testing.T) {
	e, admin := dayEnv(t)
	body := e.get(t, "/", admin).Body.String()
	if !strings.Contains(body, `<a class="btn btn-secondary" href="/upcoming">Upcoming</a>`) {
		t.Error("no full-width Upcoming button")
	}
	if strings.Contains(body, "lock-hint") {
		t.Error("the hint under Lock in is still there")
	}
}
