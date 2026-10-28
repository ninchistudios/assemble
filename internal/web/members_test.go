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

// A manager can lock in and cancel, but can't open the admin pages, and
// only the admin can make or unmake one (#39).
func TestManagerRole(t *testing.T) {
	e, admin := dayEnv(t)
	m, mgr := e.memberSession(t, "m1")
	_, jo := e.memberSession(t, "jo")

	// A plain member can't lock in, nor make themselves a manager.
	if rec := e.do(t, http.MethodPost, "/day/2026-11-20/lock", url.Values{"override": {"1"}}, mgr); rec.Code != http.StatusForbidden {
		t.Errorf("member lock = %d, want 403", rec.Code)
	}
	if rec := e.do(t, http.MethodPost, path(m.ID, "grant-manager"), url.Values{}, mgr); rec.Code != http.StatusForbidden {
		t.Errorf("member granting themselves manager = %d, want 403", rec.Code)
	}

	if rec := e.do(t, http.MethodPost, path(m.ID, "grant-manager"), url.Values{}, admin); rec.Code != http.StatusSeeOther {
		t.Fatalf("grant = %d", rec.Code)
	}
	body := e.get(t, "/admin/members", admin).Body.String()
	if !strings.Contains(body, ">manager<") || !strings.Contains(body, path(m.ID, "revoke-manager")) {
		t.Error("members page doesn't show the manager or offer to remove them")
	}

	// The manager is offered lock-in, uses it, and can cancel.
	if body := e.get(t, "/day/2026-11-20", mgr).Body.String(); !strings.Contains(body, `action="/day/2026-11-20/lock"`) {
		t.Error("manager isn't offered lock-in")
	}
	if rec := e.do(t, http.MethodPost, "/day/2026-11-20/lock", url.Values{"override": {"1"}}, mgr); rec.Code != http.StatusSeeOther {
		t.Fatalf("manager lock = %d", rec.Code)
	}
	if body := e.get(t, "/day/2026-11-20", jo).Body.String(); strings.Contains(body, "/cancel") {
		t.Error("plain member is offered cancel")
	}
	if rec := e.do(t, http.MethodPost, "/day/2026-11-20/cancel", url.Values{}, mgr); rec.Code != http.StatusSeeOther {
		t.Fatalf("manager cancel = %d", rec.Code)
	}
	// But no admin pages, and no admin link.
	if rec := e.get(t, "/admin/members", mgr); rec.Code != http.StatusForbidden {
		t.Errorf("manager GET /admin/members = %d, want 403", rec.Code)
	}
	if body := e.get(t, "/day/2026-11-20", mgr).Body.String(); strings.Contains(body, `href="/admin"`) {
		t.Error("manager is shown the admin link")
	}

	// Revoked: back to a plain member.
	e.do(t, http.MethodPost, path(m.ID, "revoke-manager"), url.Values{}, admin)
	if rec := e.do(t, http.MethodPost, "/day/2026-11-21/lock", url.Values{"override": {"1"}}, mgr); rec.Code != http.StatusForbidden {
		t.Errorf("revoked manager lock = %d, want 403", rec.Code)
	}

	// The admin is always a manager, so the flag isn't theirs to change.
	me, _ := e.db.UserBySub(adminID.Sub)
	if strings.Contains(e.get(t, "/admin/members", admin).Body.String(), path(me.ID, "grant-manager")) {
		t.Error("admin is offered the manager flag")
	}
	if rec := e.do(t, http.MethodPost, path(me.ID, "grant-manager"), url.Values{}, admin); rec.Code != http.StatusBadRequest {
		t.Errorf("granting the admin manager = %d, want 400", rec.Code)
	}
}
