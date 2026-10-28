package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNoticesShowAndDismiss(t *testing.T) {
	e, admin := dayEnv(t)
	jo := lockedDay(t, e, admin, "yes", "yes") // admin locks 2026-11-20

	body := e.get(t, "/", jo).Body.String()
	if !strings.Contains(body, "Locked in: Fri 20 Nov") || !strings.Contains(body, "by Ada") {
		t.Fatalf("Jo doesn't see the lock-in notice:\n%s", body)
	}
	if body := e.get(t, "/admin/group", admin).Body.String(); !strings.Contains(body, "by you") {
		t.Error("the admin's own notice isn't shown on other pages, or doesn't say 'by you'")
	}

	// Jo pulls out; everyone gets a cancellation, Jo included.
	setStatusJSON(e, jo, "no", true)
	if body := e.get(t, "/", admin).Body.String(); !strings.Contains(body, "Cancelled: Fri 20 Nov") || !strings.Contains(body, "by Member jo") {
		t.Error("admin doesn't see who cancelled")
	}

	// Dismissing by script hides it for Jo only.
	ids := noticeIDs(e.get(t, "/", jo).Body.String())
	if len(ids) != 2 {
		t.Fatalf("Jo has %d notices, want 2", len(ids))
	}
	for _, id := range ids {
		req := httptestRequest(http.MethodPost, "/notices/"+id+"/dismiss", testBase, jo)
		req.Header.Set("Accept", "application/json")
		var got map[string]bool
		rec := e.serve(req)
		json.Unmarshal(rec.Body.Bytes(), &got)
		if rec.Code != http.StatusOK || !got["dismissed"] {
			t.Fatalf("dismiss = %d", rec.Code)
		}
	}
	if body := e.get(t, "/", jo).Body.String(); strings.Contains(body, `class="notices"`) {
		t.Error("dismissed notices still shown to Jo")
	}
	if body := e.get(t, "/", admin).Body.String(); !strings.Contains(body, `class="notices"`) {
		t.Error("Jo's dismissals hid the admin's notices")
	}
}

func TestDismissByFormGoesBack(t *testing.T) {
	e, admin := dayEnv(t)
	lockedDay(t, e, admin, "yes", "yes")
	id := noticeIDs(e.get(t, "/", admin).Body.String())[0]
	req := httptestForm("/notices/"+id+"/dismiss", testBase, url.Values{}, admin)
	req.Header.Set("Referer", testBase+"/day/2026-11-20")
	if rec := e.serve(req); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/day/2026-11-20" {
		t.Errorf("form dismiss = %d -> %s", rec.Code, rec.Header().Get("Location"))
	}
	// A foreign referer never becomes the redirect.
	req = httptestForm("/notices/"+id+"/dismiss", testBase, url.Values{}, admin)
	req.Header.Set("Referer", "https://evil.example.com/x")
	if rec := e.serve(req); rec.Header().Get("Location") != "/" {
		t.Errorf("foreign referer redirect = %s", rec.Header().Get("Location"))
	}
}

func TestNoticesExpireWithTheirDay(t *testing.T) {
	e, admin := dayEnv(t)
	lockedDay(t, e, admin, "yes", "yes") // 2026-11-20
	e.s.now = func() time.Time { return time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC) }
	if body := e.get(t, "/", admin).Body.String(); strings.Contains(body, `class="notices"`) {
		t.Error("notice for a past day still shown")
	}
}

// noticeIDs pulls the notice ids out of a page's dismiss forms.
func noticeIDs(body string) []string {
	var ids []string
	for _, part := range strings.Split(body, `action="/notices/`)[1:] {
		ids = append(ids, part[:strings.Index(part, "/")])
	}
	return ids
}

// The admin and managers hear who joined; nobody else does (#38).
func TestJoinNotice(t *testing.T) {
	e, admin := dayEnv(t)
	m, mgr := e.memberSession(t, "m1")
	e.db.SetManager(m.ID, true)
	_, jo := e.memberSession(t, "jo")
	a, _ := e.db.UserBySub(adminID.Sub)
	e.db.CreateInvite("tok", a.ID, fixedNow)
	e.db.JoinViaInvite("tok", "sub-sam", "sam@example.com", "Sam", fixedNow)

	for name, c := range map[string]*http.Cookie{"admin": admin, "manager": mgr} {
		body := e.get(t, "/", c).Body.String()
		if !strings.Contains(body, `<span class="notice-text"><b>Sam joined the group</b></span>`) {
			t.Errorf("%s doesn't see the join notice, or it links somewhere", name)
		}
	}
	if body := e.get(t, "/", jo).Body.String(); strings.Contains(body, "joined the group") {
		t.Error("a plain member sees the join notice")
	}
}

// The admin posts a MOTD; everyone sees it until they dismiss it, and a
// new one shows again (#40).
func TestMOTDNotice(t *testing.T) {
	e, admin := dayEnv(t)
	_, jo := e.memberSession(t, "jo")

	if rec := e.do(t, http.MethodPost, "/admin/motd", url.Values{"motd": {"Gig on the 30th!"}}, jo); rec.Code != http.StatusForbidden {
		t.Errorf("member posting a MOTD = %d, want 403", rec.Code)
	}
	rec := e.do(t, http.MethodPost, "/admin/motd", url.Values{"motd": {"Gig on the 30th!"}}, admin)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/group?motd=posted" {
		t.Fatalf("post MOTD = %d -> %s", rec.Code, rec.Header().Get("Location"))
	}
	if body := e.get(t, "/admin/group", admin).Body.String(); !strings.Contains(body, `id="motd" name="motd" type="text" maxlength="100" value="Gig on the 30th!"`) {
		t.Error("admin form doesn't show the current MOTD")
	}
	body := e.get(t, "/", jo).Body.String()
	if !strings.Contains(body, "<b>Gig on the 30th!</b> <span class=\"muted\">by Ada</span>") {
		t.Fatalf("Jo doesn't see the MOTD:\n%s", body)
	}
	for _, id := range noticeIDs(body) {
		e.do(t, http.MethodPost, "/notices/"+id+"/dismiss", url.Values{}, jo)
	}
	if body := e.get(t, "/", jo).Body.String(); strings.Contains(body, "Gig on the 30th!") {
		t.Error("dismissed MOTD still shown")
	}
	e.do(t, http.MethodPost, "/admin/motd", url.Values{"motd": {"Gig moved to the 31st"}}, admin)
	body = e.get(t, "/", jo).Body.String()
	if !strings.Contains(body, "Gig moved to the 31st") || strings.Contains(body, "Gig on the 30th!") {
		t.Error("a new MOTD doesn't replace the old one for Jo")
	}

	if rec := e.do(t, http.MethodPost, "/admin/motd", url.Values{"motd": {strings.Repeat("x", 101)}}, admin); rec.Code != http.StatusBadRequest {
		t.Errorf("101-character MOTD = %d, want 400", rec.Code)
	}
	rec = e.do(t, http.MethodPost, "/admin/motd", url.Values{"motd": {""}}, admin)
	if rec.Header().Get("Location") != "/admin/group?motd=removed" {
		t.Errorf("clearing the MOTD -> %s", rec.Header().Get("Location"))
	}
	if body := e.get(t, "/", jo).Body.String(); strings.Contains(body, "Gig moved") {
		t.Error("removed MOTD still shown")
	}
}
