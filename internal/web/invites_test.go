package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestInviteCreateAndDisable(t *testing.T) {
	e := newEnv(t)
	admin := e.session(t, adminID)

	if body := e.get(t, "/admin/invites", admin).Body.String(); !strings.Contains(body, "No invite links yet") {
		t.Error("empty invite page missing its empty state")
	}
	for range 2 {
		if rec := e.do(t, http.MethodPost, "/admin/invites", url.Values{}, admin); rec.Code != http.StatusSeeOther {
			t.Fatalf("create invite = %d", rec.Code)
		}
	}
	invites, _ := e.db.Invites()
	if len(invites) != 2 || invites[0].Token == invites[1].Token || len(invites[0].Token) != 43 {
		t.Fatalf("invites = %+v", invites)
	}
	body := e.get(t, "/admin/invites", admin).Body.String()
	for _, inv := range invites {
		if !strings.Contains(body, testBase+"/join/"+inv.Token) {
			t.Errorf("page missing link for invite %d", inv.ID)
		}
	}

	id := strconv.FormatInt(invites[0].ID, 10)
	if rec := e.do(t, http.MethodPost, "/admin/invites/"+id+"/disable", url.Values{}, admin); rec.Code != http.StatusSeeOther {
		t.Fatalf("disable = %d", rec.Code)
	}
	if _, err := e.db.ActiveInvite(invites[0].Token); err == nil {
		t.Error("disabled invite is still active")
	}
	body = e.get(t, "/admin/invites", admin).Body.String()
	// A disabled link's URL is no use to anyone, so it is not shown.
	if strings.Contains(body, invites[0].Token) || !strings.Contains(body, "disabled") {
		t.Error("disabled invite still shows its link, or isn't marked disabled")
	}
	if !strings.Contains(body, invites[1].Token) {
		t.Error("the other invite went missing")
	}
}

func TestInvitesAreAdminOnly(t *testing.T) {
	e := newEnv(t)
	admin := e.session(t, adminID)
	e.do(t, http.MethodPost, "/admin/invites", url.Values{}, admin)
	invites, _ := e.db.Invites()
	_, member := e.memberSession(t, "m1")

	if rec := e.get(t, "/admin/invites", member); rec.Code != http.StatusForbidden {
		t.Errorf("member GET /admin/invites = %d", rec.Code)
	}
	if rec := e.do(t, http.MethodPost, "/admin/invites", url.Values{}, member); rec.Code != http.StatusForbidden {
		t.Errorf("member create = %d", rec.Code)
	}
	id := strconv.FormatInt(invites[0].ID, 10)
	if rec := e.do(t, http.MethodPost, "/admin/invites/"+id+"/disable", url.Values{}, member); rec.Code != http.StatusForbidden {
		t.Errorf("member disable = %d", rec.Code)
	}
	if after, _ := e.db.Invites(); len(after) != 1 || !after[0].Active() {
		t.Errorf("member changed invites: %+v", after)
	}
}

func TestAdminScriptServed(t *testing.T) {
	rec := newEnv(t).get(t, "/static/admin.js")
	// Go says text/javascript; on Windows the registry says
	// application/javascript. Browsers run both under nosniff.
	ct := rec.Header().Get("Content-Type")
	if rec.Code != http.StatusOK || !(strings.HasPrefix(ct, "text/javascript") || strings.HasPrefix(ct, "application/javascript")) {
		t.Errorf("admin.js = %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
}
