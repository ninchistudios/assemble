// Package auth handles the Google sign-in flow and the session cookie.
//
// We authenticate against Google exactly once per session, then issue our own
// random session token. The database keeps only its hash, so sessions can be
// revoked per user (disabling someone ends theirs at once) and a copy of the
// database cannot be used to sign in. No refresh tokens, no offline scope, no
// stored Google credentials, and no signing secret to manage.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"time"
)

const (
	SessionCookie = "assemble_session"
	stateCookie   = "assemble_oauth_state"

	// SessionTTL is 6 months, comfortably inside Chrome's 400-day cookie cap
	// (RFC 6265bis). Rolling: see RenewAfter.
	SessionTTL = 182 * 24 * time.Hour

	// RenewAfter is how old a session's expiry may get before a request pushes
	// it out again. Daily rather than on every request, so browsing is not a
	// stream of database writes.
	RenewAfter = 24 * time.Hour

	// stateTTL bounds how long a sign-in may sit half-finished.
	stateTTL = 10 * time.Minute
)

// NewToken returns a random URL-safe token of 32 bytes, used for sessions and
// invite links alike.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken is what the database stores in place of a session token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// SetSession writes the session cookie. secure is driven by the public origin,
// not the inbound request, because TLS terminates at the proxy.
func SetSession(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSession expires the session cookie.
func ClearSession(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
}

// NewState returns a random CSRF token for the OAuth round trip.
func NewState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func SetState(w http.ResponseWriter, state string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: state, Path: "/auth", MaxAge: int(stateTTL.Seconds()),
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
}

// CheckState compares the state echoed by Google against our cookie and clears it.
func CheckState(w http.ResponseWriter, r *http.Request, secure bool) bool {
	c, err := r.Cookie(stateCookie)
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: "", Path: "/auth", MaxAge: -1,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
	if err != nil || c.Value == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(r.URL.Query().Get("state"))) == 1
}
