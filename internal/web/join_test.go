package web

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ninchistudios/assemble/internal/auth"
	"github.com/ninchistudios/assemble/internal/store"
)

// newInvite creates an active link as a fresh bootstrap admin.
func (e *testEnv) newInvite(t *testing.T) store.Invite {
	t.Helper()
	admin, err := e.db.UserBySub(adminID.Sub)
	if err != nil {
		admin, err = e.db.CreateUser(adminID.Sub, adminID.Email, "Ada", store.RoleAdmin, 0, time.Now())
		if err != nil {
			t.Fatal(err)
		}
	}
	token, _ := auth.NewToken()
	inv, err := e.db.CreateInvite(token, admin.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

// openInvite visits the link and returns the invite cookie it set.
func (e *testEnv) openInvite(t *testing.T, token string) *http.Cookie {
	t.Helper()
	rec := e.get(t, "/join/"+token)
	c := cookie(rec, "assemble_invite")
	if rec.Code != http.StatusOK || c == nil {
		t.Fatalf("GET /join = %d, cookie %v\n%s", rec.Code, c, rec.Body)
	}
	if c.Path != "/auth" || !c.HttpOnly {
		t.Errorf("invite cookie attributes = %+v", c)
	}
	return c
}

var joiner = auth.UserInfo{Sub: "sub-j", Email: "jo@example.com", EmailVerified: true, Name: "Jo"}

func TestJoinViaInvite(t *testing.T) {
	e := newEnv(t)
	inv := e.newInvite(t)
	invite := e.openInvite(t, inv.Token)

	rec := e.googleSignIn(t, joiner, invite)
	session := cookie(rec, auth.SessionCookie)
	if rec.Code != http.StatusSeeOther || session == nil {
		t.Fatalf("join sign-in = %d\n%s", rec.Code, rec.Body)
	}
	if c := cookie(rec, "assemble_invite"); c == nil || c.MaxAge >= 0 {
		t.Error("invite cookie not cleared after joining")
	}
	u, err := e.db.UserBySub(joiner.Sub)
	if err != nil || u.Role != store.RoleMember || u.InviteID != inv.ID || u.Name != "Jo" {
		t.Fatalf("joined user = %+v, %v", u, err)
	}
	if body := e.get(t, "/", session).Body.String(); !strings.Contains(body, `Signed in as <a href="/profile">Jo</a>`) {
		t.Error("new member does not land on the home page")
	}
	if invs, _ := e.db.Invites(); invs[0].Joined != 1 {
		t.Errorf("joined count = %d, want 1", invs[0].Joined)
	}

	// Coming back through the same link just signs them in.
	rec = e.googleSignIn(t, joiner, e.openInvite(t, inv.Token))
	if rec.Code != http.StatusSeeOther {
		t.Errorf("returning member = %d", rec.Code)
	}
	if invs, _ := e.db.Invites(); invs[0].Joined != 1 {
		t.Error("returning member was counted twice")
	}
}

func TestJoinPageForUnknownOrDisabledLink(t *testing.T) {
	e := newEnv(t)
	inv := e.newInvite(t)
	e.db.DisableInvite(inv.ID, time.Now())
	for _, token := range []string{"nonsense", inv.Token} {
		rec := e.get(t, "/join/"+token)
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "no longer valid") {
			t.Errorf("GET /join/%s = %d", token, rec.Code)
		}
		if cookie(rec, "assemble_invite") != nil {
			t.Errorf("GET /join/%s set an invite cookie", token)
		}
	}
}

// The admin disables the link while someone is part-way through Google.
func TestLinkDisabledMidSignInRefused(t *testing.T) {
	e := newEnv(t)
	inv := e.newInvite(t)
	invite := e.openInvite(t, inv.Token)
	e.db.DisableInvite(inv.ID, time.Now())

	rec := e.googleSignIn(t, joiner, invite)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "has been disabled") {
		t.Fatalf("sign-in through a just-disabled link = %d\n%s", rec.Code, rec.Body)
	}
	if _, err := e.db.UserBySub(joiner.Sub); err == nil {
		t.Error("member was created through a disabled link")
	}
}

func TestDisabledMemberCannotRejoin(t *testing.T) {
	e := newEnv(t)
	inv := e.newInvite(t)
	e.googleSignIn(t, joiner, e.openInvite(t, inv.Token))
	u, _ := e.db.UserBySub(joiner.Sub)
	e.db.SetEnabled(u.ID, false)

	rec := e.googleSignIn(t, joiner, e.openInvite(t, inv.Token))
	if rec.Code != http.StatusForbidden || cookie(rec, auth.SessionCookie) != nil {
		t.Errorf("disabled member through an active link = %d", rec.Code)
	}
	if u, _ := e.db.UserBySub(joiner.Sub); u.Enabled {
		t.Error("rejoining re-enabled a disabled member")
	}
}

func TestSignedInUserOpeningInviteGoesHome(t *testing.T) {
	e := newEnv(t)
	inv := e.newInvite(t)
	admin := e.session(t, adminID)
	rec := e.get(t, "/join/"+inv.Token, admin)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Errorf("signed-in GET /join = %d -> %s", rec.Code, rec.Header().Get("Location"))
	}
}
