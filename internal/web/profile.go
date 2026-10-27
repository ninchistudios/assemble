package web

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/ninchistudios/assemble/internal/store"
)

// handleProfile shows the signed-in member their name and how to change it.
func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request, u *store.User) {
	s.render(w, http.StatusOK, "profile.html", page{Title: "Profile", User: u,
		Saved: r.URL.Query().Get("saved") == "1", MaxDisplayName: store.MaxDisplayName})
}

// handleProfilePost sets the member's display name; empty goes back to the
// name from Google.
func (s *Server) handleProfilePost(w http.ResponseWriter, r *http.Request, u *store.User) {
	name := strings.Join(strings.Fields(r.FormValue("display_name")), " ")
	if utf8.RuneCountInString(name) > store.MaxDisplayName {
		s.render(w, http.StatusBadRequest, "profile.html", page{Title: "Profile", User: u, MaxDisplayName: store.MaxDisplayName,
			Error: fmt.Sprintf("Keep your name to %d characters or fewer.", store.MaxDisplayName)})
		return
	}
	// Choosing exactly your Google name is the same as not choosing one, and
	// keeps following Google if that name changes later.
	if name == u.GoogleName {
		name = ""
	}
	if err := s.db.SetDisplayName(u.ID, name); err != nil {
		s.fail(w, "Couldn't save your name.", err)
		return
	}
	log.Printf("profile: user=%d set display name", u.ID)
	http.Redirect(w, r, "/profile?saved=1", http.StatusSeeOther)
}
