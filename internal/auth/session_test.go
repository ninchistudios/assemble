package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewTokenIsRandomAndURLSafe(t *testing.T) {
	a, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewToken()
	if a == b {
		t.Fatal("two tokens were equal")
	}
	if len(a) != 43 { // 32 bytes, unpadded base64
		t.Errorf("token length = %d, want 43", len(a))
	}
	for _, c := range a {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			t.Fatalf("token has non-URL-safe char %q", c)
		}
	}
}

func TestHashTokenIsStable(t *testing.T) {
	if HashToken("x") != HashToken("x") || HashToken("x") == HashToken("y") {
		t.Fatal("HashToken is not a stable, distinguishing hash")
	}
	if HashToken("x") == "x" {
		t.Fatal("HashToken returned its input")
	}
}

func TestCheckState(t *testing.T) {
	for _, tc := range []struct {
		cookie, query string
		want          bool
	}{
		{"abc", "abc", true},
		{"abc", "abd", false},
		{"", "abc", false},
		{"abc", "", false},
	} {
		r := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+tc.query, nil)
		if tc.cookie != "" {
			r.AddCookie(&http.Cookie{Name: stateCookie, Value: tc.cookie})
		}
		if got := CheckState(httptest.NewRecorder(), r, true); got != tc.want {
			t.Errorf("CheckState(cookie=%q, state=%q) = %v, want %v", tc.cookie, tc.query, got, tc.want)
		}
	}
}
