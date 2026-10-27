package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestUpcoming(t *testing.T) {
	e, admin := dayEnv(t)
	_, jo := e.memberSession(t, "jo")
	e.do(t, http.MethodPost, "/day/2026-11-20/status", url.Values{"status": {"yes"}}, admin)
	e.do(t, http.MethodPost, "/day/2026-11-20/status", url.Values{"status": {"ok"}}, jo)
	e.do(t, http.MethodPost, "/day/2026-11-21/status", url.Values{"status": {"no"}}, jo)
	e.do(t, http.MethodPost, "/day/2026-11-22/lock", url.Values{"override": {"1"}}, admin)

	rec := e.get(t, "/upcoming", jo)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /upcoming = %d", rec.Code)
	}
	body := rec.Body.String()
	// From today to the horizon, with month headings.
	for _, want := range []string{`href="/day/2026-11-14"`, `href="/day/2027-01-14"`, ">November 2026<", ">December 2026<", ">January 2027<"} {
		if !strings.Contains(body, want) {
			t.Errorf("overview missing %q", want)
		}
	}
	for _, gone := range []string{`href="/day/2026-11-13"`, `href="/day/2027-01-15"`} {
		if strings.Contains(body, gone) {
			t.Errorf("overview includes %s, outside the window", gone)
		}
	}
	if !strings.Contains(body, `class="up-row all" href="/day/2026-11-20"`) {
		t.Error("a day everyone can make doesn't stand out")
	}
	if !strings.Contains(body, `class="up-row locked" href="/day/2026-11-22"`) {
		t.Error("a locked-in day isn't marked")
	}
	// Jo is viewing, so the bar is the others: just the admin, who didn't
	// answer the 21st. Jo's own NO is in the "you" column.
	if !strings.Contains(body, `aria-label="0 yes, 0 ok, 0 no, 1 not answered"`) {
		t.Error("the 21st's bar isn't just the others")
	}
	// The column headings repeat on each month heading: three months here.
	if n := strings.Count(body, `>Others<`); n != 3 || strings.Count(body, `>You<`) != 3 {
		t.Errorf("Others/You headings = %d, want one per month (3)", n)
	}
}
