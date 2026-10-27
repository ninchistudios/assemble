package web

import (
	"errors"
	"net/http"

	"github.com/ninchistudios/assemble/internal/auth"
	"github.com/ninchistudios/assemble/internal/store"
)

// handleJoin is where an invite link lands. It remembers the invite and offers
// sign-in; the member is only created once Google confirms who they are, in
// signIn.
func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	if s.currentUser(w, r) != nil {
		// Already signed in here: an invite has nothing more to give.
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	token := r.PathValue("token")
	if _, err := s.db.ActiveInvite(token); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			s.fail(w, "Couldn't check that invite link.", err)
			return
		}
		s.render(w, http.StatusNotFound, "message.html", page{Title: "Invite no longer valid",
			Message: "This invite link has been disabled, or was never valid. Ask your group's admin for a new one."})
		return
	}
	auth.SetInvite(w, token, s.cfg.Secure())
	s.render(w, http.StatusOK, "join.html", page{Title: "Join"})
}
