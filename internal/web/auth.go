package web

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ninchistudios/assemble/internal/auth"
	"github.com/ninchistudios/assemble/internal/store"
)

// userHandler is a handler that runs only for a signed-in, enabled user.
type userHandler func(http.ResponseWriter, *http.Request, *store.User)

// currentUser resolves the session cookie to a user, or nil. It also rolls
// the session forward: once a day of its life has been used, the expiry is
// pushed back out to the full TTL, so anyone who opens the app at least every
// six months is never asked to sign in again.
func (s *Server) currentUser(w http.ResponseWriter, r *http.Request) *store.User {
	c, err := r.Cookie(auth.SessionCookie)
	if err != nil || c.Value == "" {
		return nil
	}
	hash := auth.HashToken(c.Value)
	now := time.Now()
	u, exp, err := s.db.SessionUser(hash, now)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			log.Printf("session lookup: %v", err)
		}
		// Expired, revoked or the user is disabled: clear it so the next
		// load is clean.
		auth.ClearSession(w, s.cfg.Secure())
		return nil
	}
	if exp.Sub(now) < auth.SessionTTL-auth.RenewAfter {
		if err := s.db.ExtendSession(hash, now, auth.SessionTTL); err != nil {
			log.Printf("extend session: %v", err)
		} else {
			auth.SetSession(w, c.Value, s.cfg.Secure())
		}
	}
	return &u
}

// requireUser shows the sign-in page in place of the requested one when there
// is no valid session.
func (s *Server) requireUser(next userHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := s.currentUser(w, r)
		if u == nil {
			s.render(w, http.StatusUnauthorized, "signin.html", page{Title: "Sign in"})
			return
		}
		next(w, r, u)
	}
}

// requireAdmin is requireUser plus a 403 for members.
func (s *Server) requireAdmin(next userHandler) http.HandlerFunc {
	return s.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		if !u.IsAdmin() {
			s.render(w, http.StatusForbidden, "message.html", page{Title: "Admins only", Message: "Only the group's admin can open that page.", User: u})
			return
		}
		next(w, r, u)
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DevBypass {
		s.localOnly(func(w http.ResponseWriter, r *http.Request) {
			s.render(w, http.StatusOK, "devlogin.html", page{Title: "Dev sign-in"})
		})(w, r)
		return
	}
	state, err := auth.NewState()
	if err != nil {
		s.fail(w, "Couldn't start sign-in.", err)
		return
	}
	auth.SetState(w, state, s.cfg.Secure())
	http.Redirect(w, r, s.google.AuthURL(state), http.StatusSeeOther)
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("error") != "" {
		// Cancelled at Google, or consent was withdrawn.
		s.render(w, http.StatusUnauthorized, "signin.html", page{Title: "Sign in", Error: "Sign-in was cancelled."})
		return
	}
	if !auth.CheckState(w, r, s.cfg.Secure()) {
		// Either CSRF, or a stale tab whose 10-minute state cookie lapsed.
		s.render(w, http.StatusBadRequest, "signin.html", page{Title: "Sign in", Error: "Sign-in expired. Please try again."})
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		s.render(w, http.StatusBadRequest, "signin.html", page{Title: "Sign in", Error: "Sign-in didn't complete. Please try again."})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	ui, err := s.google.Exchange(ctx, code)
	if err != nil {
		s.fail(w, "Couldn't complete sign-in.", err)
		return
	}
	s.signIn(w, r, ui)
}

// localOnly serves the dev sign-in only to requests addressed to this machine
// by a loopback name. config.Load already refuses the bypass unless the base
// URL is localhost, but that is a statement about configuration, not about
// who can reach the port: a misconfigured instance behind a proxy would
// otherwise hand anyone who forges an Origin header any account they like.
// Through a proxy the Host is the public name, so this fails closed.
func (s *Server) localOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		switch strings.Trim(host, "[]") {
		case "localhost", "127.0.0.1", "::1":
			next(w, r)
		default:
			s.refusals.Printf("refused dev sign-in for host %q", r.Host)
			s.notFound(w)
		}
	}
}

// handleDevLogin stands in for Google under the bypass: the typed email
// becomes a verified identity with a stable, obviously fake sub.
func (s *Server) handleDevLogin(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.FormValue("email"))
	if email == "" {
		s.render(w, http.StatusBadRequest, "devlogin.html", page{Title: "Dev sign-in", Error: "Enter an email."})
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name, _, _ = strings.Cut(email, "@")
	}
	s.signIn(w, r, auth.UserInfo{
		Sub:           "dev:" + strings.ToLower(email),
		Email:         email,
		EmailVerified: true,
		Name:          name,
	})
}

// signIn decides whether a Google identity may enter, creating the bootstrap
// admin on first sight, and issues a session if so. Google and the dev bypass
// both land here, so the rules are the same either way.
func (s *Server) signIn(w http.ResponseWriter, r *http.Request, ui auth.UserInfo) {
	now := time.Now()
	// Taken (and cleared) up front, so a refused or failed attempt does not
	// leave an invite lying around for the next person on this browser.
	inviteToken := auth.TakeInvite(w, r, s.cfg.Secure())
	isAdminEmail := ui.EmailVerified && strings.EqualFold(ui.Email, s.cfg.AdminEmail)
	name := ui.Name
	if name == "" {
		name, _, _ = strings.Cut(ui.Email, "@")
	}

	u, err := s.db.UserBySub(ui.Sub)
	switch {
	case err == nil:
		if !u.Enabled {
			s.refusals.Printf("auth: refused disabled user=%d", u.ID)
			s.render(w, http.StatusForbidden, "denied.html", page{Title: "Access turned off", Email: ui.Email,
				Message: "The admin has turned off your access to this group."})
			return
		}
		if err := s.db.UpdateProfile(u.ID, ui.Email, name); err != nil {
			log.Printf("auth: update profile user=%d: %v", u.ID, err)
		}
		// The operator moved ASSEMBLE_ADMIN_EMAIL to an existing member.
		if isAdminEmail && !u.IsAdmin() {
			if err := s.db.SetRole(u.ID, store.RoleAdmin); err != nil {
				s.fail(w, "Couldn't complete sign-in.", err)
				return
			}
			log.Printf("auth: promoted user=%d to admin from ASSEMBLE_ADMIN_EMAIL", u.ID)
		}

	case errors.Is(err, store.ErrNotFound) && isAdminEmail:
		// Bootstrap. Refuse if the address already belongs to a user under a
		// different sub: emails can be reassigned, and a recycled address
		// must not inherit the admin account.
		taken, err := s.db.EmailInUse(ui.Email)
		if err != nil {
			s.fail(w, "Couldn't complete sign-in.", err)
			return
		}
		if taken {
			s.refusals.Printf("auth: refused admin bootstrap for sub=%s: email already belongs to another account", ui.Sub)
			s.deny(w, ui)
			return
		}
		if u, err = s.db.CreateUser(ui.Sub, ui.Email, name, store.RoleAdmin, 0, now); err != nil {
			s.fail(w, "Couldn't complete sign-in.", err)
			return
		}
		log.Printf("auth: bootstrapped admin user=%d", u.ID)

	case errors.Is(err, store.ErrNotFound) && inviteToken != "":
		// The link may have been disabled while they were at Google.
		inv, err := s.db.ActiveInvite(inviteToken)
		if errors.Is(err, store.ErrNotFound) {
			s.refusals.Printf("auth: refused sub=%s: invite no longer active", ui.Sub)
			s.render(w, http.StatusForbidden, "denied.html", page{Title: "Invite no longer valid", Email: ui.Email,
				Message: "The invite link you used has been disabled. Ask your group's admin for a new one."})
			return
		}
		if err != nil {
			s.fail(w, "Couldn't complete sign-in.", err)
			return
		}
		if u, err = s.db.CreateUser(ui.Sub, ui.Email, name, store.RoleMember, inv.ID, now); err != nil {
			s.fail(w, "Couldn't complete sign-in.", err)
			return
		}
		log.Printf("auth: user=%d joined via invite=%d", u.ID, inv.ID)

	case errors.Is(err, store.ErrNotFound):
		s.refusals.Printf("auth: refused sub=%s: not a member", ui.Sub)
		s.deny(w, ui)
		return

	default:
		s.fail(w, "Couldn't complete sign-in.", err)
		return
	}

	token, err := auth.NewToken()
	if err != nil {
		s.fail(w, "Couldn't complete sign-in.", err)
		return
	}
	if err := s.db.CreateSession(auth.HashToken(token), u.ID, now, auth.SessionTTL); err != nil {
		s.fail(w, "Couldn't complete sign-in.", err)
		return
	}
	auth.SetSession(w, token, s.cfg.Secure())
	log.Printf("auth: signed in user=%d", u.ID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// deny shows a stranger the not-a-member page. The account they used is their
// own, so naming it helps them spot a wrong-account sign-in; nothing about the
// group's members is revealed.
func (s *Server) deny(w http.ResponseWriter, ui auth.UserInfo) {
	s.render(w, http.StatusForbidden, "denied.html", page{Title: "Not a member", Email: ui.Email,
		Message: "This Google account isn't a member of this group. Ask your group's admin for an invite link."})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.SessionCookie); err == nil && c.Value != "" {
		if err := s.db.DeleteSession(auth.HashToken(c.Value)); err != nil {
			log.Printf("logout: %v", err)
		}
	}
	auth.ClearSession(w, s.cfg.Secure())
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
