package web

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/ninchistudios/assemble/internal/store"
)

// memberRow is a user as the members page shows them.
type memberRow struct {
	ID      int64
	Name    string
	Email   string
	Joined  string
	Admin   bool
	Enabled bool
	You     bool
}

func (s *Server) handleMembers(w http.ResponseWriter, r *http.Request, u *store.User) {
	users, err := s.db.Users()
	if err != nil {
		s.fail(w, "Couldn't load the members.", err)
		return
	}
	loc := s.groupLocation()
	rows := make([]memberRow, 0, len(users))
	for _, m := range users {
		rows = append(rows, memberRow{
			ID: m.ID, Name: m.Name, Email: m.Email,
			Joined: shortDate(m.CreatedAt, loc),
			Admin:  m.IsAdmin(), Enabled: m.Enabled, You: m.ID == u.ID,
		})
	}
	s.render(w, http.StatusOK, "members.html", page{Title: "Members", Tab: "members", User: u, Members: rows})
}

// handleMemberAccess returns the handler that enables or disables the member
// named in the path.
func (s *Server) handleMemberAccess(enable bool) userHandler {
	return func(w http.ResponseWriter, r *http.Request, u *store.User) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			s.render(w, http.StatusNotFound, "message.html", page{Title: "Not found", Message: "There's no such member.", User: u})
			return
		}
		// Locking yourself out would leave the group with no way back in
		// short of editing the database.
		if id == u.ID && !enable {
			s.render(w, http.StatusBadRequest, "message.html", page{Title: "Can't disable yourself", Message: "You can't turn off your own access.", User: u})
			return
		}
		if _, err := s.db.UserByID(id); errors.Is(err, store.ErrNotFound) {
			s.render(w, http.StatusNotFound, "message.html", page{Title: "Not found", Message: "There's no such member.", User: u})
			return
		} else if err != nil {
			s.fail(w, "Couldn't change that member's access.", err)
			return
		}
		if err := s.db.SetEnabled(id, enable); err != nil {
			s.fail(w, "Couldn't change that member's access.", err)
			return
		}
		log.Printf("admin: user=%d set user=%d enabled=%v", u.ID, id, enable)
		http.Redirect(w, r, "/admin/members", http.StatusSeeOther)
	}
}
