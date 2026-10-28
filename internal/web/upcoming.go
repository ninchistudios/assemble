package web

import (
	"net/http"

	"github.com/ninchistudios/assemble/internal/cal"
	"github.com/ninchistudios/assemble/internal/store"
)

// upcomingRow is one day in the overview.
type upcomingRow struct {
	Day          cal.Day
	Short        string
	Month        string // set on the first row of each month, for a heading
	IsToday      bool
	Counts       [4]int   // the other members: yes, ok, no, unknown
	Segments     []string // one per other member, in that order, for the bar
	Mine         cal.Status
	Locked       bool
	AllAvailable bool
}

// handleUpcoming lists every day from today to the horizon, so what's ahead
// and what's booked can be seen at a glance.
func (s *Server) handleUpcoming(w http.ResponseWriter, r *http.Request, u *store.User) {
	today, win := s.today()
	users, err := s.db.Users()
	if err != nil {
		s.fail(w, "Couldn't load the upcoming days.", err)
		return
	}
	answers, err := s.db.Answers(win.First, win.Last)
	if err != nil {
		s.fail(w, "Couldn't load the upcoming days.", err)
		return
	}
	lockins, err := s.db.Lockins(win.First, win.Last)
	if err != nil {
		s.fail(w, "Couldn't load the upcoming days.", err)
		return
	}

	var rows []upcomingRow
	month := ""
	for _, d := range win.Days() {
		row := upcomingRow{Day: d, Short: d.Short(), IsToday: d == today, Mine: answers[d][u.ID].Status}
		_, row.Locked = lockins[d]
		if m := d.MonthLabel(); m != month {
			row.Month, month = m, m
		}
		// The bar is everyone else; my own answer has its own column. Whether
		// the day works for everyone still counts me.
		enabled, in := 0, 0
		for _, m := range users {
			if !m.Enabled {
				continue
			}
			enabled++
			st := answers[d][m.ID].Status
			if st.Available() {
				in++
			}
			if m.ID == u.ID {
				continue
			}
			switch st {
			case cal.Yes:
				row.Counts[0]++
			case cal.OK:
				row.Counts[1]++
			case cal.No:
				row.Counts[2]++
			default:
				row.Counts[3]++
			}
		}
		row.AllAvailable = enabled > 0 && in == enabled
		for i, name := range []string{"yes", "ok", "no", "unknown"} {
			for range row.Counts[i] {
				row.Segments = append(row.Segments, name)
			}
		}
		rows = append(rows, row)
	}
	s.render(w, http.StatusOK, "upcoming.html", page{User: u, Upcoming: rows})
}
