package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninchistudios/assemble/internal/auth"
	"github.com/ninchistudios/assemble/internal/config"
	"github.com/ninchistudios/assemble/internal/store"
)

const (
	testBase       = "https://assemble.example.com"
	testAdminEmail = "admin@example.com"
)

// fakeGoogle answers the token and userinfo calls. The authorization code is
// the key into ids, and doubles as the access token.
type fakeGoogle struct {
	srv *httptest.Server
	ids map[string]auth.UserInfo
}

func newFakeGoogle(t *testing.T) *fakeGoogle {
	t.Helper()
	f := &fakeGoogle{ids: map[string]auth.UserInfo{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		code := r.FormValue("code")
		if _, ok := f.ids[code]; !ok {
			http.Error(w, "bad code", http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"access_token": code})
	})
	mux.HandleFunc("GET /userinfo", func(w http.ResponseWriter, r *http.Request) {
		ui, ok := f.ids[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
		if !ok {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(ui)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

type testEnv struct {
	s      *Server
	db     *store.Store
	google *fakeGoogle
}

func newEnv(t *testing.T, mod ...func(*config.Config)) *testEnv {
	t.Helper()
	cfg := config.Config{
		Listen:             ":0",
		BaseURL:            testBase,
		Version:            "vtest",
		GoogleClientID:     "id",
		GoogleClientSecret: "secret",
		AdminEmail:         testAdminEmail,
	}
	for _, m := range mod {
		m(&cfg)
	}
	db := newStoreForWeb(t)
	s, err := New(cfg, db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	g := newFakeGoogle(t)
	s.google.HTTP = g.srv.Client()
	s.google.TokenEndpoint = g.srv.URL + "/token"
	s.google.UserinfoEndpoint = g.srv.URL + "/userinfo"
	return &testEnv{s: s, db: db, google: g}
}

func newStoreForWeb(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// do sends a request with the given cookies. Unsafe methods carry our Origin
// unless the test sets its own.
func (e *testEnv) do(t *testing.T, method, path string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if method != http.MethodGet {
		req.Header.Set("Origin", testBase)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	e.s.ServeHTTP(rec, req)
	return rec
}

func (e *testEnv) get(t *testing.T, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	return e.do(t, http.MethodGet, path, nil, cookies...)
}

func (e *testEnv) serve(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.s.ServeHTTP(rec, req)
	return rec
}

// httptestRequest builds a request with an explicit Origin ("" for none).
func httptestRequest(method, path, origin string, cookies ...*http.Cookie) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	return req
}

// httptestForm builds a form POST from the given origin.
func httptestForm(path, origin string, form url.Values, cookies ...*http.Cookie) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", origin)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	return req
}

func cookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// googleSignIn walks the full round trip as ui and returns the callback
// response plus any extra cookies the caller wants carried through it.
func (e *testEnv) googleSignIn(t *testing.T, ui auth.UserInfo, extra ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	login := e.get(t, "/auth/login", extra...)
	if login.Code != http.StatusSeeOther {
		t.Fatalf("GET /auth/login = %d", login.Code)
	}
	loc, _ := url.Parse(login.Header().Get("Location"))
	state := loc.Query().Get("state")
	stateCookie := cookie(login, "assemble_oauth_state")
	if state == "" || stateCookie == nil {
		t.Fatalf("login set no state: Location=%s", loc)
	}
	code := "code-" + ui.Sub
	e.google.ids[code] = ui
	return e.get(t, "/auth/callback?state="+state+"&code="+code, append(extra, stateCookie)...)
}

// session signs in as ui and returns the session cookie, failing the test if
// sign-in was refused.
func (e *testEnv) session(t *testing.T, ui auth.UserInfo) *http.Cookie {
	t.Helper()
	rec := e.googleSignIn(t, ui)
	c := cookie(rec, auth.SessionCookie)
	if rec.Code != http.StatusSeeOther || c == nil {
		t.Fatalf("sign in as %s: status %d, cookie %v\n%s", ui.Email, rec.Code, c, rec.Body)
	}
	return c
}

var adminID = auth.UserInfo{Sub: "sub-admin", Email: testAdminEmail, EmailVerified: true, Name: "Ada"}

func TestHealthzReportsVersion(t *testing.T) {
	rec := newEnv(t).get(t, "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz = %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.HasPrefix(body, "ok vtest") {
		t.Errorf("GET /healthz body = %q", body)
	}
}

func TestHealthzFailsWithoutDatabase(t *testing.T) {
	e := newEnv(t)
	e.db.Close()
	if rec := e.get(t, "/healthz"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("GET /healthz with closed db = %d, want 503", rec.Code)
	}
}

// Error pages are where per-handler headers used to go missing, so check them
// explicitly alongside a normal page.
func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	e := newEnv(t)
	for _, tc := range []struct {
		path string
		code int
	}{
		{"/", http.StatusUnauthorized},
		{"/no-such-page", http.StatusNotFound},
		{"/healthz", http.StatusOK},
	} {
		rec := e.get(t, tc.path)
		if rec.Code != tc.code {
			t.Errorf("GET %s = %d, want %d", tc.path, rec.Code, tc.code)
		}
		for _, h := range []string{"Content-Security-Policy", "Referrer-Policy", "X-Content-Type-Options"} {
			if rec.Header().Get(h) == "" {
				t.Errorf("GET %s: missing %s", tc.path, h)
			}
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
			t.Errorf("GET %s: Cache-Control = %q, want no-store", tc.path, cc)
		}
	}
}

// Under no-referrer (or any policy that withholds the origin from same-origin
// requests) browsers send "Origin: null" on our own form posts, and the origin
// check refuses every form. That shipped once: #13.
func TestReferrerPolicyKeepsSameOriginPosts(t *testing.T) {
	if p := newEnv(t).get(t, "/").Header().Get("Referrer-Policy"); p != "same-origin" {
		t.Errorf("Referrer-Policy = %q, want same-origin", p)
	}
}

// A local test copy marks every page; the real instance never does.
func TestLocalInstanceIsMarked(t *testing.T) {
	if body := newEnv(t).get(t, "/").Body.String(); strings.Contains(body, "env-local") {
		t.Error("public instance is marked local")
	}
	local := newEnv(t, func(c *config.Config) { c.BaseURL = "http://localhost:8138" })
	body := local.get(t, "/").Body.String()
	if !strings.Contains(body, `<body class="env-local">`) || !strings.Contains(body, ">local</span>") {
		t.Error("local instance is not marked")
	}
}

func TestStaticServesOnlyKnownFiles(t *testing.T) {
	e := newEnv(t)
	for _, p := range []string{"/static/", "/static/fonts/", "/static/nope.css"} {
		if rec := e.get(t, p); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", p, rec.Code)
		}
	}
}

func TestOversizedBodyRefused(t *testing.T) {
	e := newEnv(t)
	admin := e.session(t, adminID)
	big := url.Values{"group_name": {strings.Repeat("x", maxBody)}}
	rec := e.do(t, http.MethodPost, "/admin/group", big, admin)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized POST = %d, want 413", rec.Code)
	}
}

// The font is self-hosted; a wrong content type makes some browsers refuse it
// and fall back silently to the system sans.
func TestFontServedAsWoff2(t *testing.T) {
	rec := newEnv(t).get(t, "/static/fonts/manrope-latin.woff2")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET font = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "font/woff2" {
		t.Errorf("font Content-Type = %q, want font/woff2", ct)
	}
}

func TestStylePageShowsEveryStatus(t *testing.T) {
	e := newEnv(t)
	rec := e.get(t, "/style", e.session(t, adminID))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /style = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, st := range []string{"status-yes", "status-ok", "status-no", "status-unknown"} {
		if !strings.Contains(body, st) {
			t.Errorf("/style missing %s", st)
		}
	}
	// The CSP forbids inline styles, so any style attribute is a silent bug.
	if strings.Contains(body, "style=") {
		t.Error("/style contains an inline style attribute")
	}
}

func TestStaticRevalidates(t *testing.T) {
	e := newEnv(t)
	rec := e.get(t, "/static/assemble.css")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /static/assemble.css = %d", rec.Code)
	}
	tag := rec.Header().Get("ETag")
	if tag == "" || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("static headers: ETag=%q Cache-Control=%q", tag, rec.Header().Get("Cache-Control"))
	}

	req := httptest.NewRequest(http.MethodGet, "/static/assemble.css", nil)
	req.Header.Set("If-None-Match", tag)
	rec = httptest.NewRecorder()
	e.s.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Errorf("conditional GET = %d, want 304", rec.Code)
	}
}
