package web

import (
	"net/http"
	"strings"
	"testing"
)

// Google's consent screen links to these, so they must work signed out.
func TestPrivacyAndTermsArePublic(t *testing.T) {
	e := newEnv(t)
	for path, want := range map[string]string{"/privacy": "What is stored", "/terms": "No guarantees"} {
		rec := e.get(t, path)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("signed-out GET %s = %d", path, rec.Code)
		}
	}
	// Linked from where a newcomer signs in.
	body := e.get(t, "/").Body.String()
	if !strings.Contains(body, `href="/privacy"`) || !strings.Contains(body, `href="/terms"`) {
		t.Error("sign-in page doesn't link to privacy and terms")
	}
	inv := e.newInvite(t)
	if body := e.get(t, "/join/"+inv.Token).Body.String(); !strings.Contains(body, `href="/privacy"`) {
		t.Error("join page doesn't link to privacy")
	}
}
