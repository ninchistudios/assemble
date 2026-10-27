package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

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
}

// memberStatus is one enabled member's answer for the day.
type memberStatus struct {
	Name   string
	Me     bool
	Status cal.Status
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
		m := memberStatus{Name: u.Name, Me: u.ID == me.ID, Status: answers[d][u.ID].Status}
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
	for _, m := range group {
		if m.Me {
			mine = m.Status
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
	}
	if !win.Beyond(d.Add(1)) {
		v.Next = d.Add(1)
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
	if err := s.db.SetStatus(u.ID, d, st, s.now()); err != nil {
		log.Printf("set status user=%d day=%s: %v", u.ID, d, err)
		s.answer(w, r, d, http.StatusInternalServerError, "Couldn't save that. Please try again.")
		return
	}
	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, map[string]string{"status": st.Icon()})
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
