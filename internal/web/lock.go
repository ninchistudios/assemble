package web

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/ninchistudios/assemble/internal/store"
	"github.com/ninchistudios/assemble/internal/words"
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

// handleLock books a day. If anyone enabled isn't [YES] or [OK], the admin or
// manager must confirm an override: the day page's script asks first, and without
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
	msg, problem := s.lockMessage(r)
	if problem != "" {
		s.render(w, http.StatusBadRequest, "message.html", page{Title: "Can't lock in", Message: problem, User: u})
		return
	}
	// The store checks availability in the same transaction as the booking,
	// so an answer changing after this page was loaded is still caught.
	err := s.db.Lock(d, u.ID, r.FormValue("override") == "1", msg, s.now())
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
			Confirm: &confirmForm{Action: dayPath(d) + "/lock", Fields: map[string]string{"override": "1", "message": msg},
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

// handleLockMessage rewords a locked-in day's message (#41). Nobody is
// notified: it's a correction, not news.
func (s *Server) handleLockMessage(w http.ResponseWriter, r *http.Request, u *store.User) {
	d, ok := s.dayFromPath(w, r)
	if !ok {
		return
	}
	if _, win := s.today(); !win.Editable(d) {
		s.render(w, http.StatusConflict, "message.html", page{Title: "Can't change that", Message: "This day has passed.", User: u})
		return
	}
	msg, problem := s.lockMessage(r)
	if problem != "" {
		s.render(w, http.StatusBadRequest, "message.html", page{Title: "Can't save that", Message: problem, User: u})
		return
	}
	err := s.db.SetLockMessage(d, msg)
	if errors.Is(err, store.ErrNotFound) {
		s.render(w, http.StatusConflict, "message.html", page{Title: "Can't change that", Message: "This day isn't locked in.", User: u})
		return
	}
	if err != nil {
		s.fail(w, "Couldn't save the message.", err)
		return
	}
	log.Printf("calendar: user=%d reworded %s", u.ID, d)
	http.Redirect(w, r, dayPath(d), http.StatusSeeOther)
}

// defaultLockMessage pre-fills the lock-in form. It's only a starting point:
// what's saved is whatever the manager leaves in the field, so a one-off
// change of time goes in the message rather than in the settings.
func (s *Server) defaultLockMessage() string {
	st := s.groupSettings()
	return "Locked in for " + st.SlotStart + " to " + st.SlotEnd
}

// lockMessage reads the lock-in message from a form. A form without the
// field gets the default. problem, if not empty, says why it's refused.
func (s *Server) lockMessage(r *http.Request) (msg, problem string) {
	if _, ok := r.Form["message"]; !ok {
		return s.defaultLockMessage(), ""
	}
	msg = words.Clean(r.FormValue("message"))
	return msg, s.checkMessage(msg)
}

// checkMessage refuses a lock-in message or MOTD that's too long or trips
// the language filter, returning why; empty means it's fine.
func (s *Server) checkMessage(msg string) string {
	if utf8.RuneCountInString(msg) > store.MaxMessage {
		return fmt.Sprintf("Keep the message to %d characters or fewer.", store.MaxMessage)
	}
	if s.filter().Blocked(msg) {
		return blockedMsg
	}
	return ""
}

// lockedSummary is the group line on a locked-in day: its message, in place
// of the phrase about everyone's answers (#41).
func lockedSummary(msg string) string {
	if msg == "" {
		return "Locked in"
	}
	return msg
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
		v.LockMessage = s.filter().Mask(l.Message)
		v.Summary = lockedSummary(v.LockMessage)
	case !errors.Is(err, store.ErrNotFound):
		return err
	}
	v.Unavailable = unavailable(v.Group)
	manage := u.CanManage() && v.Editable
	v.CanLock = manage && !v.Locked
	v.CanCancel = manage && v.Locked
	v.DefaultMessage, v.MaxMessage = s.defaultLockMessage(), store.MaxMessage
	return nil
}
