package web

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/ninchistudios/assemble/internal/store"
)

// confirmForm is a one-button page asking "are you sure?", for the cases a
// script would handle with confirm() but a plain form post can't.
type confirmForm struct {
	Action string
	Fields map[string]string
	Button string
	Back   string
}

// unavailable names the enabled members who aren't [YES] or [OK], e.g.
// "Jo (NO), Sam (?)". Empty means everyone can make it.
func unavailable(group []memberStatus) string {
	var out []string
	for _, m := range group {
		if !m.Status.Available() {
			out = append(out, m.Name+" ("+m.Status.Label()+")")
		}
	}
	return strings.Join(out, ", ")
}

// handleLock books a day. If anyone enabled isn't [YES] or [OK], the admin
// must confirm an override: the day page's script asks first, and without
// it (or if answers changed since the page loaded) this shows a confirm page.
func (s *Server) handleLock(w http.ResponseWriter, r *http.Request, u *store.User) {
	d, ok := s.dayFromPath(w, r)
	if !ok {
		return
	}
	if _, win := s.today(); !win.Editable(d) {
		s.render(w, http.StatusConflict, "message.html", page{Title: "Can't lock in", Message: "This day has passed.", User: u})
		return
	}
	// The store checks availability in the same transaction as the booking,
	// so an answer changing after this page was loaded is still caught.
	err := s.db.Lock(d, u.ID, r.FormValue("override") == "1", s.now())
	if errors.Is(err, store.ErrNeedsOverride) {
		group, gerr := s.groupFor(u, d)
		if gerr != nil {
			s.fail(w, "Couldn't lock in that day.", gerr)
			return
		}
		s.render(w, http.StatusConflict, "confirm.html", page{
			Title:   "Lock in anyway?",
			Message: "Not everyone can make " + d.Short() + ": " + unavailable(group) + ".",
			User:    u,
			Confirm: &confirmForm{Action: dayPath(d) + "/lock", Fields: map[string]string{"override": "1"},
				Button: "Lock in anyway", Back: dayPath(d)},
		})
		return
	}
	if err != nil && !errors.Is(err, store.ErrLocked) {
		s.fail(w, "Couldn't lock in that day.", err)
		return
	}
	if err == nil {
		log.Printf("calendar: user=%d locked in %s", u.ID, d)
	}
	http.Redirect(w, r, dayPath(d), http.StatusSeeOther)
}

// handleCancel unbooks a locked-in day.
func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request, u *store.User) {
	d, ok := s.dayFromPath(w, r)
	if !ok {
		return
	}
	if _, win := s.today(); !win.Editable(d) {
		s.render(w, http.StatusConflict, "message.html", page{Title: "Can't cancel", Message: "This day has passed.", User: u})
		return
	}
	err := s.db.Unlock(d, u.ID, s.now())
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.fail(w, "Couldn't cancel that day.", err)
		return
	}
	if err == nil {
		log.Printf("calendar: user=%d cancelled %s", u.ID, d)
	}
	http.Redirect(w, r, dayPath(d), http.StatusSeeOther)
}

// lockState fills in the lock-in parts of a day view.
func (s *Server) lockState(v *dayView, u *store.User) error {
	l, err := s.db.LockinFor(v.Day)
	switch {
	case err == nil:
		v.Locked, v.LockedOverride = true, l.Override
	case !errors.Is(err, store.ErrNotFound):
		return err
	}
	v.Unavailable = unavailable(v.Group)
	admin := u.IsAdmin() && v.Editable
	v.CanLock = admin && !v.Locked
	v.CanCancel = admin && v.Locked
	return nil
}
