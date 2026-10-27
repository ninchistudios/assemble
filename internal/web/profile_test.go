package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestDisplayName(t *testing.T) {
	e, admin := dayEnv(t)
	_, jo := e.memberSession(t, "jo")

	if rec := e.do(t, http.MethodPost, "/profile", url.Values{"display_name": {"  Jo   the Drummer "}}, jo); rec.Code != http.StatusSeeOther {
		t.Fatalf("set name = %d", rec.Code)
	}
	// The group sees the chosen name, and a later sign-in doesn't undo it.
	e.session(t, jo2Info)
	if body := e.get(t, "/", admin).Body.String(); !strings.Contains(body, "Jo the Drummer") {
		t.Error("the group doesn't see the chosen name")
	}
	if body := e.get(t, "/profile", jo).Body.String(); !strings.Contains(body, `value="Jo the Drummer"`) || !strings.Contains(body, "Member jo") {
		t.Error("profile doesn't show the chosen and Google names")
	}

	if rec := e.do(t, http.MethodPost, "/profile", url.Values{"display_name": {strings.Repeat("x", 41)}}, jo); rec.Code != http.StatusBadRequest {
		t.Errorf("overlong name = %d, want 400", rec.Code)
	}
	// Empty goes back to the Google name.
	e.do(t, http.MethodPost, "/profile", url.Values{"display_name": {""}}, jo)
	if body := e.get(t, "/", admin).Body.String(); strings.Contains(body, "Jo the Drummer") || !strings.Contains(body, "Member jo") {
		t.Error("clearing didn't return to the Google name")
	}
}

// jo2Info is the "jo" member signing in again through Google.
var jo2Info = authInfo("jo", "jo@example.com", "Member jo")
