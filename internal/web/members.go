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
	Manager bool
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
	f := s.filter()
	rows := make([]memberRow, 0, len(users))
	for _, m := range users {
		rows = append(rows, memberRow{
			ID: m.ID, Name: f.Mask(m.Name), Email: m.Email,
			Joined: shortDate(m.CreatedAt, loc),
			Admin:  m.IsAdmin(), Manager: m.Manager, Enabled: m.Enabled, You: m.ID == u.ID,
		})
	}
	s.render(w, http.StatusOK, "members.html", page{Title: "Members", Tab: "members", User: u, Members: rows})
}

// handleMemberManager returns the handler that grants or revokes the manager
// flag for the member named in the path (#39). The admin can always manage,
// so the flag isn't offered on them.
func (s *Server) handleMemberManager(grant bool) userHandler {
	return func(w http.ResponseWriter, r *http.Request, u *store.User) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			s.render(w, http.StatusNotFound, "message.html", page{Title: "Not found", Message: "There's no such member.", User: u})
			return
		}
		m, err := s.db.UserByID(id)
		if errors.Is(err, store.ErrNotFound) {
			s.render(w, http.StatusNotFound, "message.html", page{Title: "Not found", Message: "There's no such member.", User: u})
			return
		} else if err != nil {
			s.fail(w, "Couldn't change that member's role.", err)
			return
		}
		if m.IsAdmin() {
			s.render(w, http.StatusBadRequest, "message.html", page{Title: "Already the admin", Message: "The admin can always lock in and cancel.", User: u})
			return
		}
		if err := s.db.SetManager(id, grant); err != nil {
			s.fail(w, "Couldn't change that member's role.", err)
			return
		}
		log.Printf("admin: user=%d set user=%d manager=%v", u.ID, id, grant)
		http.Redirect(w, r, "/admin/members", http.StatusSeeOther)
	}
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
