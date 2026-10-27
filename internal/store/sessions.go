package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// CreateSession records a new session for userID. hash is the token's hash;
// the token itself never reaches the database. Expired sessions are swept out
// here, which is often enough for a group this size.
func (s *Store) CreateSession(hash string, userID int64, now time.Time, ttl time.Duration) error {
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, Stamp(now)); err != nil {
		return fmt.Errorf("sweep sessions: %w", err)
	}
	_, err := s.db.Exec(
		`INSERT INTO sessions (token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		hash, userID, Stamp(now), Stamp(now.Add(ttl)))
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// SessionUser resolves a session to its user, along with when the session
// expires. A session that has expired, or whose user is disabled, is
// ErrNotFound: disabling someone takes effect on their very next request.
func (s *Store) SessionUser(hash string, now time.Time) (User, time.Time, error) {
	var userID int64
	var expires string
	err := s.db.QueryRow(`
SELECT s.user_id, s.expires_at
FROM sessions s JOIN users u ON u.id = s.user_id
WHERE s.token_hash = ? AND s.expires_at > ? AND u.enabled = 1`, hash, Stamp(now)).Scan(&userID, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, time.Time{}, ErrNotFound
	}
	if err != nil {
		return User{}, time.Time{}, err
	}
	exp, err := parseStamp(expires)
	if err != nil {
		return User{}, time.Time{}, err
	}
	u, err := s.UserByID(userID)
	return u, exp, err
}

// ExtendSession pushes a session's expiry out to now+ttl.
func (s *Store) ExtendSession(hash string, now time.Time, ttl time.Duration) error {
	_, err := s.db.Exec(`UPDATE sessions SET expires_at = ? WHERE token_hash = ?`, Stamp(now.Add(ttl)), hash)
	return err
}

func (s *Store) DeleteSession(hash string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, hash)
	return err
}

// DeleteUserSessions signs a user out everywhere.
func (s *Store) DeleteUserSessions(userID int64) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}
