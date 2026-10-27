package web

import (
	"encoding/json"
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
	mine, err := s.db.Status(u.ID, d)
	if err != nil {
		s.fail(w, "Couldn't load that day.", err)
		return
	}
	v := dayView{
		Day:      d,
		Label:    d.Label(today),
		IsToday:  d == today,
		Editable: win.Editable(d),
		Past:     win.Past(d),
		Prev:     d.Add(-1),
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
	s.render(w, http.StatusOK, "day.html", page{Title: title, User: u, DayView: &v})
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
