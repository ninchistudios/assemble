package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func setFilter(t *testing.T, e *testEnv, admin *http.Cookie, list string) *httptest.ResponseRecorder {
	t.Helper()
	return e.do(t, http.MethodPost, "/admin/group", url.Values{
		"group_name": {"The Band"}, "slot_start": {"19:00"}, "slot_end": {"22:00"}, "timezone": {"UTC"},
		"banned_words": {list},
	}, admin)
}

func TestLanguageFilter(t *testing.T) {
	e, admin := dayEnv(t)
	_, joC := e.memberSession(t, "jo")

	// Text saved before the filter existed.
	e.do(t, http.MethodPost, "/day/2026-11-20/status", url.Values{"status": {"ok"}}, joC)
	e.do(t, http.MethodPost, "/day/2026-11-20/note", url.Values{"note": {"darn traffic, late"}}, joC)
	e.do(t, http.MethodPost, "/profile", url.Values{"display_name": {"Darn Jo"}}, joC)

	if rec := setFilter(t, e, admin, " darn ,, Bad Apple, "); rec.Code != http.StatusSeeOther {
		t.Fatalf("save filter = %d", rec.Code)
	}
	if st, _ := e.db.Settings(); st.BannedWords != "darn, Bad Apple" {
		t.Errorf("stored list = %q, want it tidied", st.BannedWords)
	}

	// Already-saved text is masked wherever it shows.
	day := e.get(t, "/day/2026-11-20", admin).Body.String()
	if !strings.Contains(day, "*** traffic, late") || !strings.Contains(day, "*** Jo") || strings.Contains(day, "Darn Jo") {
		t.Error("day view doesn't mask the old note and name")
	}
	if body := e.get(t, "/admin/members", admin).Body.String(); strings.Contains(body, "Darn Jo") {
		t.Error("members page shows the unmasked name")
	}
	if body := e.get(t, "/profile", joC).Body.String(); strings.Contains(body, "Darn") {
		t.Error("profile shows the unmasked name")
	}

	// New text is blocked.
	for path, field := range map[string]string{"/day/2026-11-20/note": "note", "/profile": "display_name"} {
		rec := e.do(t, http.MethodPost, path, url.Values{field: {"what a bad  apple"}}, joC)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Blocked by language filter") {
			t.Errorf("%s with a banned phrase = %d", path, rec.Code)
		}
	}
	// Whole words only.
	if rec := e.do(t, http.MethodPost, "/day/2026-11-20/note", url.Values{"note": {"darned socks"}}, joC); rec.Code != http.StatusSeeOther {
		t.Errorf("note with a longer word containing a banned one = %d", rec.Code)
	}
	// The admin can't name the group with a word on the list either.
	rec := e.do(t, http.MethodPost, "/admin/group", url.Values{
		"group_name": {"Darn Band"}, "slot_start": {"19:00"}, "slot_end": {"22:00"}, "timezone": {"UTC"}, "banned_words": {"darn"},
	}, admin)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("blocked group name = %d", rec.Code)
	}
}

func TestFilterListCapped(t *testing.T) {
	e, admin := dayEnv(t)
	if rec := setFilter(t, e, admin, strings.Repeat("word, ", 400)); rec.Code != http.StatusBadRequest {
		t.Errorf("overlong list = %d, want 400", rec.Code)
	}
}

// https://xkcd.com/327/: free text is stored as given and shown escaped;
// nothing in it is ever run.
func TestBobbyTables(t *testing.T) {
	e, admin := dayEnv(t)
	_, joC := e.memberSession(t, "jo")
	bobby := "Robert'); DROP TABLE users;--"
	e.do(t, http.MethodPost, "/profile", url.Values{"display_name": {bobby}}, joC)
	e.do(t, http.MethodPost, "/day/2026-11-20/status", url.Values{"status": {"yes"}}, joC)
	e.do(t, http.MethodPost, "/day/2026-11-20/note", url.Values{"note": {`<script>alert(1)</script> "; DELETE FROM availability; --`}}, joC)

	users, err := e.db.Users()
	if err != nil || len(users) != 2 {
		t.Fatalf("users table after Bobby: %d users, %v", len(users), err)
	}
	body := e.get(t, "/day/2026-11-20", admin).Body.String()
	if !strings.Contains(body, "Robert&#39;); DROP TABLE users;--") {
		t.Error("Bobby's name isn't shown, escaped")
	}
	if strings.Contains(body, "<script>alert") || !strings.Contains(body, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Error("the note isn't escaped")
	}
}

// Invisible formatting characters are dropped from what's saved.
func TestControlCharactersStripped(t *testing.T) {
	e, _ := dayEnv(t)
	jo, joC := e.memberSession(t, "jo")
	e.do(t, http.MethodPost, "/profile", url.Values{"display_name": {"Jo\u202e\u0007 the\n\nDrummer"}}, joC)
	if u, _ := e.db.UserByID(jo.ID); u.DisplayName != "Jo the Drummer" {
		t.Errorf("display name stored as %q", u.DisplayName)
	}
}
