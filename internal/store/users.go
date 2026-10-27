package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

type User struct {
	ID        int64
	Sub       string
	Email     string
	Name      string
	Role      string
	Enabled   bool
	InviteID  int64 // 0 for the bootstrap admin
	CreatedAt time.Time
}

func (u User) IsAdmin() bool { return u.Role == RoleAdmin }

const userCols = `id, sub, email, name, role, enabled, COALESCE(invite_id, 0), created_at`

type scanner interface{ Scan(...any) error }

func scanUser(row scanner) (User, error) {
	var u User
	var created string
	err := row.Scan(&u.ID, &u.Sub, &u.Email, &u.Name, &u.Role, &u.Enabled, &u.InviteID, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	u.CreatedAt, err = parseStamp(created)
	return u, err
}

// UserBySub finds a user by their Google subject identifier.
func (s *Store) UserBySub(sub string) (User, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE sub = ?`, sub))
}

func (s *Store) UserByID(id int64) (User, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE id = ?`, id))
}

// EmailInUse reports whether any user record carries this email,
// case-insensitively. The admin bootstrap uses it to refuse a second Google
// account presenting an address that already belongs to someone.
func (s *Store) EmailInUse(email string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE email = ? COLLATE NOCASE`, email).Scan(&n)
	return n > 0, err
}

// CreateUser adds a user. inviteID is 0 for the bootstrap admin.
func (s *Store) CreateUser(sub, email, name, role string, inviteID int64, now time.Time) (User, error) {
	var invite any
	if inviteID != 0 {
		invite = inviteID
	}
	res, err := s.db.Exec(
		`INSERT INTO users (sub, email, name, role, invite_id, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		sub, email, name, role, invite, Stamp(now))
	if err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, err
	}
	return s.UserByID(id)
}

// UpdateProfile refreshes the display details Google reported at sign-in.
func (s *Store) UpdateProfile(id int64, email, name string) error {
	_, err := s.db.Exec(`UPDATE users SET email = ?, name = ? WHERE id = ?`, email, name, id)
	return err
}

// SetEnabled turns a user's access on or off. Turning it off also ends every
// session they hold, in the same transaction, so there is no window in which
// a disabled user is still signed in somewhere.
func (s *Store) SetEnabled(id int64, enabled bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE users SET enabled = ? WHERE id = ?`, enabled, id); err != nil {
		return err
	}
	if !enabled {
		if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SetRole(id int64, role string) error {
	_, err := s.db.Exec(`UPDATE users SET role = ? WHERE id = ?`, role, id)
	return err
}
