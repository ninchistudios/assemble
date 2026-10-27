package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/ninchistudios/assemble/internal/cal"
	"github.com/ninchistudios/assemble/internal/store"
)

// dayView is a day as the day page shows it.
type dayView struct {
	Day      cal.Day
	Label    string
	IsToday  bool
	Editable bool
	Past     bool
	Prev     cal.Day
	Next     cal.Day // empty at the horizon
	Picks    []pick
	Group    []memberStatus
	Summary  string
	Mine     cal.Status
	MyNote   string
	MaxNote  int

	Locked         bool
	LockedOverride bool
	Unavailable    string // who isn't [YES]/[OK]; empty if everyone is
	CanLock        bool   // admin, editable, not locked
	CanCancel      bool   // admin, editable, locked
}

// memberStatus is one enabled member's answer for the day.
type memberStatus struct {
	Name   string
	Me     bool
	Status cal.Status
	Note   string
}

// summarise counts statuses the way the page and its script both write
// them: "3 yes · 1 ok · 1 no · 1 ?".
func summarise(group []memberStatus) string {
	n := map[cal.Status]int{}
	for _, m := range group {
		n[m.Status]++
	}
	return fmt.Sprintf("%d yes · %d ok · %d no · %d ?", n[cal.Yes], n[cal.OK], n[cal.No], n[cal.Unknown])
}

// groupFor lists every enabled member's status for d, me first, then in the
// order people joined. Disabled members are left out entirely.
func (s *Server) groupFor(me *store.User, d cal.Day) ([]memberStatus, error) {
	users, err := s.db.Users()
	if err != nil {
		return nil, err
	}
	answers, err := s.db.Answers(d, d)
	if err != nil {
		return nil, err
	}
	var mine []memberStatus
	var rest []memberStatus
	for _, u := range users {
		if !u.Enabled {
			continue
		}
		a := answers[d][u.ID]
		m := memberStatus{Name: u.Name, Me: u.ID == me.ID, Status: a.Status, Note: a.Note}
		if m.Me {
			mine = append(mine, m)
		} else {
			rest = append(rest, m)
		}
	}
	return append(mine, rest...), nil
}

// pick is one of the four status buttons.
type pick struct {
	Value    string
	Label    string
	Icon     string
	Selected bool
}

// today is the current day and window on the group's clock.
func (s *Server) today() (cal.Day, cal.Window) {
	d := cal.Today(s.now(), s.groupLocation())
	return d, cal.WindowFor(d)
}

// dayFromPath reads /day/{date}, answering 404 for anything that isn't a day
// or lies beyond the horizon. Past days are allowed: they are read-only.
func (s *Server) dayFromPath(w http.ResponseWriter, r *http.Request) (cal.Day, bool) {
	d, err := cal.ParseDay(r.PathValue("date"))
	if err != nil {
		s.notFound(w)
		return "", false
	}
	if _, win := s.today(); win.Beyond(d) {
		s.notFound(w)
		return "", false
	}
	return d, true
}

func (s *Server) handleToday(w http.ResponseWriter, r *http.Request, u *store.User) {
	d, _ := s.today()
	s.showDay(w, u, d)
}

func (s *Server) handleDay(w http.ResponseWriter, r *http.Request, u *store.User) {
	if d, ok := s.dayFromPath(w, r); ok {
		s.showDay(w, u, d)
	}
}

func (s *Server) showDay(w http.ResponseWriter, u *store.User, d cal.Day) {
	today, win := s.today()
	group, err := s.groupFor(u, d)
	if err != nil {
		s.fail(w, "Couldn't load that day.", err)
		return
	}
	var mine cal.Status
	var myNote string
	for _, m := range group {
		if m.Me {
			mine, myNote = m.Status, m.Note
		}
	}
	v := dayView{
		Day:      d,
		Label:    d.Label(today),
		IsToday:  d == today,
		Editable: win.Editable(d),
		Past:     win.Past(d),
		Prev:     d.Add(-1),
		Group:    group,
		Summary:  summarise(group),
		Mine:     mine,
		MyNote:   myNote,
		MaxNote:  store.MaxNote,
	}
	if !win.Beyond(d.Add(1)) {
		v.Next = d.Add(1)
	}
	if err := s.lockState(&v, u); err != nil {
		s.fail(w, "Couldn't load that day.", err)
		return
	}
	for _, st := range cal.All {
		v.Picks = append(v.Picks, pick{Value: st.Icon(), Label: st.Label(), Icon: st.Icon(), Selected: st == mine})
	}
	title := v.Label
	if v.IsToday {
		title = ""
	}
	s.render(w, http.StatusOK, "day.html", page{Title: title, User: u, DayView: &v,
		StatusIcons: []string{"yes", "ok", "no", "unknown"}})
}

// handleSetStatus sets the signed-in member's status for a day. The day
// page's script posts here and asks for JSON; without the script it is a
// plain form post answered with a redirect back to the day.
func (s *Server) handleSetStatus(w http.ResponseWriter, r *http.Request, u *store.User) {
	d, ok := s.dayFromPath(w, r)
	if !ok {
		return
	}
	st, err := cal.ParseStatus(r.FormValue("status"))
	if err != nil {
		s.answer(w, r, d, http.StatusBadRequest, "That isn't a status.")
		return
	}
	if _, win := s.today(); !win.Editable(d) {
		s.answer(w, r, d, http.StatusConflict, "This day has passed, so it can't be changed.")
		return
	}

	// Pulling out of a locked-in day cancels it, so ask first.
	cancels, err := s.wouldCancel(u, d, st)
	if err != nil {
		log.Printf("set status user=%d day=%s: %v", u.ID, d, err)
		s.answer(w, r, d, http.StatusInternalServerError, "Couldn't save that. Please try again.")
		return
	}
	if cancels && r.FormValue("confirm") != "1" {
		s.askToPullOut(w, r, u, d, st)
		return
	}
	if cancels {
		err = s.db.PullOut(u.ID, d, st, s.now())
	} else {
		err = s.db.SetStatus(u.ID, d, st, s.now())
	}
	if err != nil {
		log.Printf("set status user=%d day=%s: %v", u.ID, d, err)
		s.answer(w, r, d, http.StatusInternalServerError, "Couldn't save that. Please try again.")
		return
	}
	if cancels {
		log.Printf("calendar: user=%d pulled out of %s, cancelling it", u.ID, d)
	}
	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, map[string]any{"status": st.Icon(), "cancelled": cancels})
		return
	}
	http.Redirect(w, r, dayPath(d), http.StatusSeeOther)
}

// pullOutWarning is the question asked before a change cancels a booking.
const pullOutWarning = "This will cancel the locked-in session. Are you sure?"

// wouldCancel reports whether setting st on d pulls u out of a locked-in
// day: the day is locked and u goes from [YES]/[OK] to [NO]/[?].
func (s *Server) wouldCancel(u *store.User, d cal.Day, st cal.Status) (bool, error) {
	if _, err := s.db.LockinFor(d); errors.Is(err, store.ErrNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	current, err := s.db.Status(u.ID, d)
	if err != nil {
		return false, err
	}
	return cal.Cancels(current, st), nil
}

// askToPullOut asks for the confirm: the script gets JSON to show confirm()
// with, a plain form gets a page with a button that resends with confirm=1.
func (s *Server) askToPullOut(w http.ResponseWriter, r *http.Request, u *store.User, d cal.Day, st cal.Status) {
	if wantsJSON(r) {
		writeJSON(w, http.StatusConflict, map[string]string{"confirm": pullOutWarning})
		return
	}
	s.render(w, http.StatusConflict, "confirm.html", page{
		Title:   "Cancel the locked-in session?",
		Message: "You're changing to " + st.Label() + " on " + d.Short() + ", which is locked in. " + pullOutWarning,
		User:    u,
		Confirm: &confirmForm{Action: dayPath(d) + "/status", Fields: map[string]string{"status": st.Icon(), "confirm": "1"},
			Button: "Yes, cancel it", Back: dayPath(d)},
	})
}

// handleSetNote sets the signed-in member's public note for a day. Like
// handleSetStatus, it answers the script with JSON and a plain form with a
// redirect.
func (s *Server) handleSetNote(w http.ResponseWriter, r *http.Request, u *store.User) {
	d, ok := s.dayFromPath(w, r)
	if !ok {
		return
	}
	note := strings.Join(strings.Fields(r.FormValue("note")), " ")
	if utf8.RuneCountInString(note) > store.MaxNote {
		s.answer(w, r, d, http.StatusBadRequest, fmt.Sprintf("Keep the note to %d characters or fewer.", store.MaxNote))
		return
	}
	if _, win := s.today(); !win.Editable(d) {
		s.answer(w, r, d, http.StatusConflict, "This day has passed, so it can't be changed.")
		return
	}
	err := s.db.SetNote(u.ID, d, note, s.now())
	if errors.Is(err, store.ErrNotFound) {
		s.answer(w, r, d, http.StatusConflict, "Set your status first: a note goes with your answer.")
		return
	}
	if err != nil {
		log.Printf("set note user=%d day=%s: %v", u.ID, d, err)
		s.answer(w, r, d, http.StatusInternalServerError, "Couldn't save that. Please try again.")
		return
	}
	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, map[string]string{"note": note})
		return
	}
	http.Redirect(w, r, dayPath(d), http.StatusSeeOther)
}

// answer reports a problem to whichever kind of client asked.
func (s *Server) answer(w http.ResponseWriter, r *http.Request, d cal.Day, status int, msg string) {
	if wantsJSON(r) {
		writeJSON(w, status, map[string]string{"error": msg})
		return
	}
	s.render(w, status, "message.html", page{Title: "Couldn't save", Message: msg})
}

func dayPath(d cal.Day) string { return "/day/" + string(d) }

func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
