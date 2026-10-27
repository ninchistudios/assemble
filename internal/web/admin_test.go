package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ninchistudios/assemble/internal/auth"
	"github.com/ninchistudios/assemble/internal/store"
)

// memberSession creates a member directly and signs them in.
func (e *testEnv) memberSession(t *testing.T, sub string) (*store.User, *http.Cookie) {
	t.Helper()
	u, err := e.db.CreateUser(sub, sub+"@example.com", "Member "+sub, store.RoleMember, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return &u, e.session(t, auth.UserInfo{Sub: sub, Email: sub + "@example.com", EmailVerified: true, Name: u.Name})
}

func TestAdminPagesAreAdminOnly(t *testing.T) {
	e := newEnv(t)
	_, member := e.memberSession(t, "m1")
	for _, path := range []string{"/admin", "/admin/group"} {
		if rec := e.get(t, path); rec.Code != http.StatusUnauthorized {
			t.Errorf("signed-out GET %s = %d, want 401", path, rec.Code)
		}
		if rec := e.get(t, path, member); rec.Code != http.StatusForbidden {
			t.Errorf("member GET %s = %d, want 403", path, rec.Code)
		}
	}
	rec := e.do(t, http.MethodPost, "/admin/group", url.Values{"group_name": {"Hijacked"}}, member)
	if rec.Code != http.StatusForbidden {
		t.Errorf("member POST /admin/group = %d, want 403", rec.Code)
	}
	if st, _ := e.db.Settings(); st.GroupName == "Hijacked" {
		t.Error("member changed the settings")
	}
}

func TestSaveGroupSettings(t *testing.T) {
	e := newEnv(t)
	admin := e.session(t, adminID)
	if rec := e.get(t, "/admin", admin); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/group" {
		t.Errorf("GET /admin = %d -> %s", rec.Code, rec.Header().Get("Location"))
	}

	rec := e.do(t, http.MethodPost, "/admin/group", url.Values{
		"group_name": {"  The Example Band  "},
		"slot_start": {"19:30:00"},
		"slot_end":   {"23:00"},
		"timezone":   {"Australia/Brisbane"},
	}, admin)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/group?saved=1" {
		t.Fatalf("POST /admin/group = %d -> %s\n%s", rec.Code, rec.Header().Get("Location"), rec.Body)
	}
	st, _ := e.db.Settings()
	if st.GroupName != "The Example Band" || st.SlotStart != "19:30" || st.SlotEnd != "23:00" || st.Timezone != "Australia/Brisbane" {
		t.Errorf("saved = %+v", st)
	}

	// Every page carries the new name and slot in its header.
	body := e.get(t, "/admin/group?saved=1", admin).Body.String()
	for _, want := range []string{"The Example Band", "19:30–23:00 · Australia/Brisbane", "Saved."} {
		if !strings.Contains(body, want) {
			t.Errorf("settings page missing %q", want)
		}
	}
	if body := e.get(t, "/").Body.String(); !strings.Contains(body, "<title>The Example Band: Sign in</title>") {
		t.Error("signed-out page does not show the group name")
	}
}

func TestInvalidSettingsAreNotSaved(t *testing.T) {
	e := newEnv(t)
	admin := e.session(t, adminID)
	rec := e.do(t, http.MethodPost, "/admin/group", url.Values{
		"group_name": {"Band"}, "slot_start": {"19:00"}, "slot_end": {"22:00"}, "timezone": {"Mars/Olympus"},
	}, admin)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "a timezone name") {
		t.Fatalf("bad timezone = %d\n%s", rec.Code, rec.Body)
	}
	// The admin's input is kept so it can be corrected.
	if !strings.Contains(rec.Body.String(), `value="Mars/Olympus"`) {
		t.Error("rejected form lost the admin's input")
	}
	if st, _ := e.db.Settings(); st.GroupName != store.DefaultSettings.GroupName {
		t.Errorf("invalid settings were saved: %+v", st)
	}
}

func TestValidateSettings(t *testing.T) {
	ok := store.Settings{GroupName: "Band", SlotStart: "19:00", SlotEnd: "22:00", Timezone: "Australia/Brisbane"}
	for _, tc := range []struct {
		name   string
		mod    func(*store.Settings)
		wantOK bool
	}{
		{"valid", func(*store.Settings) {}, true},
		{"overnight", func(s *store.Settings) { s.SlotStart, s.SlotEnd = "22:00", "01:00" }, true},
		{"UTC", func(s *store.Settings) { s.Timezone = "UTC" }, true},
		{"empty name", func(s *store.Settings) { s.GroupName = "" }, false},
		{"long name", func(s *store.Settings) { s.GroupName = strings.Repeat("x", 61) }, false},
		{"bad start", func(s *store.Settings) { s.SlotStart = "7pm" }, false},
		{"bad end", func(s *store.Settings) { s.SlotEnd = "25:00" }, false},
		{"same times", func(s *store.Settings) { s.SlotEnd = "19:00" }, false},
		{"same minute", func(s *store.Settings) { s.SlotStart, s.SlotEnd = "19:00:00", "19:00:30" }, false},
		{"empty zone", func(s *store.Settings) { s.Timezone = "" }, false},
		{"Local zone", func(s *store.Settings) { s.Timezone = "Local" }, false},
		{"abbreviation", func(s *store.Settings) { s.Timezone = "AEST" }, false},
	} {
		st := ok
		tc.mod(&st)
		if msg := validateSettings(&st); (msg == "") != tc.wantOK {
			t.Errorf("%s: validateSettings = %q, want ok=%v", tc.name, msg, tc.wantOK)
		}
	}
}
