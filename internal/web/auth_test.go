package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ninchistudios/assemble/internal/auth"
	"github.com/ninchistudios/assemble/internal/config"
	"github.com/ninchistudios/assemble/internal/store"
)

func TestSignedOutSeesSignIn(t *testing.T) {
	rec := newEnv(t).get(t, "/")
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "Sign in with Google") {
		t.Fatalf("GET / signed out = %d\n%s", rec.Code, rec.Body)
	}
}

func TestLoginRedirectsToGoogle(t *testing.T) {
	rec := newEnv(t).get(t, "/auth/login")
	loc, _ := url.Parse(rec.Header().Get("Location"))
	q := loc.Query()
	if loc.Host != "accounts.google.com" || q.Get("scope") != "openid email profile" ||
		q.Get("redirect_uri") != testBase+"/auth/callback" || q.Get("access_type") != "" {
		t.Errorf("login redirect = %s", loc)
	}
}

func TestAdminBootstrap(t *testing.T) {
	e := newEnv(t)
	c := e.session(t, adminID)
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.MaxAge < 180*24*3600 {
		t.Errorf("session cookie attributes = %+v", c)
	}
	rec := e.get(t, "/", c)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Signed in as Ada") {
		t.Fatalf("GET / as admin = %d\n%s", rec.Code, rec.Body)
	}
	u, err := e.db.UserBySub(adminID.Sub)
	if err != nil || !u.IsAdmin() {
		t.Fatalf("bootstrap user = %+v, %v", u, err)
	}
	// Signing in again reuses the account rather than creating another.
	e.session(t, adminID)
	if u2, _ := e.db.UserBySub(adminID.Sub); u2.ID != u.ID {
		t.Error("second sign-in created a new user")
	}
}

func TestAdminEmailMatchIgnoresCase(t *testing.T) {
	e := newEnv(t)
	id := adminID
	id.Email = "Admin@Example.COM"
	e.session(t, id)
}

func TestUnverifiedAdminEmailRefused(t *testing.T) {
	id := adminID
	id.EmailVerified = false
	if rec := newEnv(t).googleSignIn(t, id); rec.Code != http.StatusForbidden {
		t.Errorf("unverified admin email = %d, want 403", rec.Code)
	}
}

func TestStrangerRefused(t *testing.T) {
	e := newEnv(t)
	rec := e.googleSignIn(t, auth.UserInfo{Sub: "sub-x", Email: "x@example.com", EmailVerified: true})
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "a member of this group") {
		t.Fatalf("stranger = %d\n%s", rec.Code, rec.Body)
	}
	if cookie(rec, auth.SessionCookie) != nil {
		t.Error("stranger was given a session")
	}
	if _, err := e.db.UserBySub("sub-x"); err == nil {
		t.Error("stranger was stored")
	}
}

// A recycled address must not inherit the admin account.
func TestAdminEmailUnderNewSubRefused(t *testing.T) {
	e := newEnv(t)
	e.session(t, adminID)
	impostor := adminID
	impostor.Sub = "sub-other"
	if rec := e.googleSignIn(t, impostor); rec.Code != http.StatusForbidden {
		t.Errorf("admin email under a new sub = %d, want 403", rec.Code)
	}
}

func TestMemberCannotOpenAdminPages(t *testing.T) {
	e := newEnv(t)
	if _, err := e.db.CreateUser("sub-m", "m@example.com", "Mo", store.RoleMember, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	c := e.session(t, auth.UserInfo{Sub: "sub-m", Email: "m@example.com", EmailVerified: true, Name: "Mo"})
	if rec := e.get(t, "/", c); rec.Code != http.StatusOK {
		t.Errorf("member GET / = %d", rec.Code)
	}
	if rec := e.get(t, "/style", c); rec.Code != http.StatusForbidden {
		t.Errorf("member GET /style = %d, want 403", rec.Code)
	}
}

func TestDisabledUserSignedOutAndRefused(t *testing.T) {
	e := newEnv(t)
	u, _ := e.db.CreateUser("sub-m", "m@example.com", "Mo", store.RoleMember, 0, time.Now())
	id := auth.UserInfo{Sub: "sub-m", Email: "m@example.com", EmailVerified: true}
	c := e.session(t, id)

	if err := e.db.SetEnabled(u.ID, false); err != nil {
		t.Fatal(err)
	}
	rec := e.get(t, "/", c)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("disabled user's next request = %d, want 401", rec.Code)
	}
	if cl := cookie(rec, auth.SessionCookie); cl == nil || cl.MaxAge >= 0 {
		t.Error("disabled user's cookie was not cleared")
	}
	rec = e.googleSignIn(t, id)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "turned off") {
		t.Errorf("disabled user signing in = %d\n%s", rec.Code, rec.Body)
	}
}

func TestCallbackRejectsBadState(t *testing.T) {
	e := newEnv(t)
	login := e.get(t, "/auth/login")
	rec := e.get(t, "/auth/callback?state=forged&code=x", cookie(login, "assemble_oauth_state"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("forged state = %d, want 400", rec.Code)
	}
}

func TestLogoutEndsSession(t *testing.T) {
	e := newEnv(t)
	c := e.session(t, adminID)
	if rec := e.do(t, http.MethodPost, "/auth/logout", url.Values{}, c); rec.Code != http.StatusSeeOther {
		t.Fatalf("logout = %d", rec.Code)
	}
	// The old cookie is dead server-side, not merely cleared in the browser.
	if rec := e.get(t, "/", c); rec.Code != http.StatusUnauthorized {
		t.Errorf("old session after logout = %d, want 401", rec.Code)
	}
}

func TestCrossOriginPostRefused(t *testing.T) {
	e := newEnv(t)
	c := e.session(t, adminID)
	for _, origin := range []string{"", "https://evil.example.com", "null"} {
		req := httptestRequest(http.MethodPost, "/auth/logout", origin, c)
		rec := e.serve(req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("POST with Origin %q = %d, want 403", origin, rec.Code)
		}
	}
	if rec := e.get(t, "/", c); rec.Code != http.StatusOK {
		t.Error("refused logout still ended the session")
	}
}

func TestSessionRollsForward(t *testing.T) {
	e := newEnv(t)
	u, _ := e.db.CreateUser("sub-m", "m@example.com", "Mo", store.RoleMember, 0, time.Now())
	token, _ := auth.NewToken()
	// A session issued two days ago is due for renewal.
	if err := e.db.CreateSession(auth.HashToken(token), u.ID, time.Now().Add(-48*time.Hour), auth.SessionTTL); err != nil {
		t.Fatal(err)
	}
	rec := e.get(t, "/", &http.Cookie{Name: auth.SessionCookie, Value: token})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d", rec.Code)
	}
	if c := cookie(rec, auth.SessionCookie); c == nil || c.Value != token {
		t.Fatal("renewal did not reissue the cookie")
	}
	_, exp, _ := e.db.SessionUser(auth.HashToken(token), time.Now())
	if time.Until(exp) < auth.SessionTTL-time.Hour {
		t.Errorf("expiry not extended: %v left", time.Until(exp))
	}

	// A fresh one is left alone.
	rec = e.get(t, "/", &http.Cookie{Name: auth.SessionCookie, Value: token})
	if cookie(rec, auth.SessionCookie) != nil {
		t.Error("fresh session was reissued")
	}
}

func TestDevLoginAbsentWithoutBypass(t *testing.T) {
	rec := newEnv(t).do(t, http.MethodPost, "/auth/dev", url.Values{"email": {testAdminEmail}})
	if rec.Code != http.StatusNotFound {
		t.Errorf("POST /auth/dev without bypass = %d, want 404", rec.Code)
	}
}

func devEnv(t *testing.T) *testEnv {
	return newEnv(t, func(c *config.Config) {
		c.BaseURL = "http://localhost:8138"
		c.DevBypass = true
	})
}

func TestDevLoginUsesTheSameRules(t *testing.T) {
	e := devEnv(t)
	post := func(email string) *http.Cookie {
		req := httptestForm("/auth/dev", "http://localhost:8138", url.Values{"email": {email}})
		req.RemoteAddr = "127.0.0.1:50000"
		rec := e.serve(req)
		return cookie(rec, auth.SessionCookie)
	}
	login := httptestRequest(http.MethodGet, "/auth/login", "")
	login.RemoteAddr = "127.0.0.1:50000"
	if rec := e.serve(login); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Google is bypassed") {
		t.Fatalf("GET /auth/login under bypass = %d", rec.Code)
	}
	if c := post(testAdminEmail); c == nil {
		t.Error("dev sign-in as the admin email was refused")
	}
	if c := post("stranger@example.com"); c != nil {
		t.Error("dev sign-in let a stranger in")
	}
}

// The guard checks who is connecting, not what they claim: a remote client
// that forges both Host and Origin is still refused.
func TestDevLoginRefusedOffLoopback(t *testing.T) {
	e := devEnv(t)
	for _, peer := range []string{"192.168.1.20:50000", "172.17.0.1:50000", "203.0.113.9:50000"} {
		req := httptestForm("/auth/dev", "http://localhost:8138", url.Values{"email": {testAdminEmail}})
		req.Host = "localhost:8138"
		req.RemoteAddr = peer
		rec := e.serve(req)
		if rec.Code != http.StatusNotFound || cookie(rec, auth.SessionCookie) != nil {
			t.Errorf("dev sign-in from %s = %d", peer, rec.Code)
		}
		login := httptestRequest(http.MethodGet, "/auth/login", "")
		login.RemoteAddr = peer
		if rec := e.serve(login); rec.Code != http.StatusNotFound {
			t.Errorf("dev login page from %s = %d", peer, rec.Code)
		}
	}
	for _, peer := range []string{"127.0.0.1:50000", "[::1]:50000"} {
		req := httptestForm("/auth/dev", "http://localhost:8138", url.Values{"email": {testAdminEmail}})
		req.RemoteAddr = peer
		if rec := e.serve(req); rec.Code != http.StatusSeeOther {
			t.Errorf("dev sign-in from loopback %s = %d", peer, rec.Code)
		}
	}
}

// The original admin's Google address changes, and someone else later signs
// in with the old one. The variable is unchanged, so the grant stays with the
// bound account.
func TestAdminAddressReusedByAnotherAccountRefused(t *testing.T) {
	e := newEnv(t)
	e.session(t, adminID)
	moved := adminID
	moved.Email = "ada.new@example.com"
	e.session(t, moved) // profile now carries the new address

	claimant := auth.UserInfo{Sub: "sub-new-owner", Email: testAdminEmail, EmailVerified: true}
	if rec := e.googleSignIn(t, claimant); rec.Code != http.StatusForbidden {
		t.Errorf("new owner of the admin address = %d, want 403", rec.Code)
	}
	if u, _ := e.db.UserBySub(adminID.Sub); !u.IsAdmin() {
		t.Error("original admin lost the role")
	}
}

// A member who comes to hold the admin address is not promoted while the
// variable still names the address it was bound under.
func TestMemberWithBoundAdminAddressNotPromoted(t *testing.T) {
	e := newEnv(t)
	e.session(t, adminID)
	moved := adminID
	moved.Email = "ada.new@example.com"
	e.session(t, moved)

	m, _ := e.memberSession(t, "m1")
	e.session(t, auth.UserInfo{Sub: "m1", Email: testAdminEmail, EmailVerified: true})
	if u, _ := e.db.UserByID(m.ID); u.IsAdmin() {
		t.Error("member was promoted through a bound address")
	}
}

// Changing the variable is a deliberate handover.
func TestHandoverByChangingAdminEmail(t *testing.T) {
	e := newEnv(t)
	e.session(t, adminID)
	m, _ := e.memberSession(t, "m1")

	e.s.cfg.AdminEmail = "m1@example.com"
	e.session(t, auth.UserInfo{Sub: "m1", Email: "m1@example.com", EmailVerified: true})
	if u, _ := e.db.UserByID(m.ID); !u.IsAdmin() {
		t.Fatal("handover did not promote the member")
	}
	email, sub, _ := e.db.AdminBinding()
	if email != "m1@example.com" || sub != "m1" {
		t.Errorf("binding after handover = %s/%s", email, sub)
	}
}

// An admin bootstrapped before bindings were recorded is bound at startup.
func TestExistingAdminBoundAtStartup(t *testing.T) {
	db := newStoreForWeb(t)
	if _, err := db.CreateUser(adminID.Sub, testAdminEmail, "Ada", store.RoleAdmin, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := New(config.Config{BaseURL: testBase, AdminEmail: "Admin@Example.com"}, db); err != nil {
		t.Fatal(err)
	}
	if _, sub, _ := db.AdminBinding(); sub != adminID.Sub {
		t.Errorf("startup binding sub = %q, want %q", sub, adminID.Sub)
	}
}
