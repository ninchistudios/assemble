package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ninchistudios/assemble/internal/config"
)

func newServer(t *testing.T) *Server {
	t.Helper()
	s, err := New(config.Config{
		Listen:  ":0",
		BaseURL: "https://assemble.example.com",
		Version: "vtest",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func get(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestHealthzReportsVersion(t *testing.T) {
	rec := get(t, newServer(t), "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz = %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.HasPrefix(body, "ok vtest") {
		t.Errorf("GET /healthz body = %q", body)
	}
}

// Error pages are where per-handler headers used to go missing, so check one
// explicitly alongside a normal page.
func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	s := newServer(t)
	for _, tc := range []struct {
		path string
		code int
	}{
		{"/", http.StatusOK},
		{"/no-such-page", http.StatusNotFound},
		{"/healthz", http.StatusOK},
	} {
		rec := get(t, s, tc.path)
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

// The font is self-hosted; a wrong content type makes some browsers refuse it
// and fall back silently to the system sans.
func TestFontServedAsWoff2(t *testing.T) {
	rec := get(t, newServer(t), "/static/fonts/manrope-latin.woff2")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET font = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "font/woff2" {
		t.Errorf("font Content-Type = %q, want font/woff2", ct)
	}
}

func TestStylePageShowsEveryStatus(t *testing.T) {
	rec := get(t, newServer(t), "/style")
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
	s := newServer(t)
	rec := get(t, s, "/static/assemble.css")
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
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Errorf("conditional GET = %d, want 304", rec.Code)
	}
}
