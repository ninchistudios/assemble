package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/ninchistudios/assemble/internal/auth"
)

func path(id int64, action string) string {
	return "/admin/members/" + strconv.FormatInt(id, 10) + "/" + action
}

func TestDisableAndEnableMember(t *testing.T) {
	e := newEnv(t)
	admin := e.session(t, adminID)
	m, member := e.memberSession(t, "m1")

	body := e.get(t, "/admin/members", admin).Body.String()
	for _, want := range []string{"Ada", "(you)", "Member m1", "m1@example.com", path(m.ID, "disable")} {
		if !strings.Contains(body, want) {
			t.Errorf("members page missing %q", want)
		}
	}

	if rec := e.do(t, http.MethodPost, path(m.ID, "disable"), url.Values{}, admin); rec.Code != http.StatusSeeOther {
		t.Fatalf("disable = %d", rec.Code)
	}
	// Signed out on their next request, and refused at sign-in.
	if rec := e.get(t, "/", member); rec.Code != http.StatusUnauthorized {
		t.Errorf("disabled member's next request = %d, want 401", rec.Code)
	}
	id := auth.UserInfo{Sub: "m1", Email: "m1@example.com", EmailVerified: true}
	if rec := e.googleSignIn(t, id); rec.Code != http.StatusForbidden {
		t.Errorf("disabled member signing in = %d, want 403", rec.Code)
	}
	if body := e.get(t, "/admin/members", admin).Body.String(); !strings.Contains(body, path(m.ID, "enable")) {
		t.Error("disabled member has no enable button")
	}

	if rec := e.do(t, http.MethodPost, path(m.ID, "enable"), url.Values{}, admin); rec.Code != http.StatusSeeOther {
		t.Fatalf("enable = %d", rec.Code)
	}
	c := e.session(t, id)
	if rec := e.get(t, "/", c); rec.Code != http.StatusOK {
		t.Errorf("re-enabled member GET / = %d", rec.Code)
	}
}

func TestAdminCannotDisableThemselves(t *testing.T) {
	e := newEnv(t)
	admin := e.session(t, adminID)
	me, _ := e.db.UserBySub(adminID.Sub)
	if body := e.get(t, "/admin/members", admin).Body.String(); strings.Contains(body, path(me.ID, "disable")) {
		t.Error("admin is offered a button to disable themselves")
	}
	if rec := e.do(t, http.MethodPost, path(me.ID, "disable"), url.Values{}, admin); rec.Code != http.StatusBadRequest {
		t.Errorf("self-disable = %d, want 400", rec.Code)
	}
	if rec := e.get(t, "/", admin); rec.Code != http.StatusOK {
		t.Error("admin was locked out")
	}
}

func TestMemberAccessIsAdminOnly(t *testing.T) {
	e := newEnv(t)
	e.session(t, adminID)
	other, _ := e.memberSession(t, "m1")
	_, member := e.memberSession(t, "m2")
	if rec := e.get(t, "/admin/members", member); rec.Code != http.StatusForbidden {
		t.Errorf("member GET /admin/members = %d", rec.Code)
	}
	if rec := e.do(t, http.MethodPost, path(other.ID, "disable"), url.Values{}, member); rec.Code != http.StatusForbidden {
		t.Errorf("member disabling another = %d", rec.Code)
	}
	if u, _ := e.db.UserByID(other.ID); !u.Enabled {
		t.Error("a member disabled someone")
	}
}

func TestUnknownMemberIs404(t *testing.T) {
	e := newEnv(t)
	admin := e.session(t, adminID)
	for _, p := range []string{"/admin/members/999/disable", "/admin/members/x/enable"} {
		if rec := e.do(t, http.MethodPost, p, url.Values{}, admin); rec.Code != http.StatusNotFound {
			t.Errorf("POST %s = %d, want 404", p, rec.Code)
		}
	}
}
