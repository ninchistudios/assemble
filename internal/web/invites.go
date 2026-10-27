package web

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/ninchistudios/assemble/internal/auth"
	"github.com/ninchistudios/assemble/internal/store"
)

// inviteRow is an invite as the admin page shows it.
type inviteRow struct {
	ID       int64
	URL      string
	Active   bool
	Created  string
	Disabled string
	Joined   int
}

// groupLocation is the group's timezone, for showing dates. Validation keeps
// bad names out, so the fallback only matters if the row is unreadable.
func (s *Server) groupLocation() *time.Location {
	loc, err := time.LoadLocation(s.groupSettings().Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// shortDate is how dates read in lists: "3 Nov", with the year only when it
// is not this year.
func shortDate(t time.Time, loc *time.Location) string {
	t = t.In(loc)
	if t.Year() != time.Now().In(loc).Year() {
		return t.Format("2 Jan 2006")
	}
	return t.Format("2 Jan")
}

func (s *Server) handleInvites(w http.ResponseWriter, r *http.Request, u *store.User) {
	invites, err := s.db.Invites()
	if err != nil {
		s.fail(w, "Couldn't load the invite links.", err)
		return
	}
	loc := s.groupLocation()
	rows := make([]inviteRow, 0, len(invites))
	for _, inv := range invites {
		row := inviteRow{
			ID:      inv.ID,
			URL:     s.cfg.BaseURL + "/join/" + inv.Token,
			Active:  inv.Active(),
			Created: shortDate(inv.CreatedAt, loc),
			Joined:  inv.Joined,
		}
		if !inv.Active() {
			row.Disabled = shortDate(inv.DisabledAt, loc)
		}
		rows = append(rows, row)
	}
	s.render(w, http.StatusOK, "invites.html", page{Title: "Invites", Tab: "invites", User: u, Invites: rows})
}

func (s *Server) handleInviteCreate(w http.ResponseWriter, r *http.Request, u *store.User) {
	token, err := auth.NewToken()
	if err != nil {
		s.fail(w, "Couldn't create an invite link.", err)
		return
	}
	inv, err := s.db.CreateInvite(token, u.ID, time.Now())
	if err != nil {
		s.fail(w, "Couldn't create an invite link.", err)
		return
	}
	log.Printf("admin: user=%d created invite=%d", u.ID, inv.ID)
	http.Redirect(w, r, "/admin/invites", http.StatusSeeOther)
}

func (s *Server) handleInviteDisable(w http.ResponseWriter, r *http.Request, u *store.User) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.render(w, http.StatusNotFound, "message.html", page{Title: "Not found", Message: "There's no such invite link.", User: u})
		return
	}
	if err := s.db.DisableInvite(id, time.Now()); err != nil {
		s.fail(w, "Couldn't disable the invite link.", err)
		return
	}
	log.Printf("admin: user=%d disabled invite=%d", u.ID, id)
	http.Redirect(w, r, "/admin/invites", http.StatusSeeOther)
}
