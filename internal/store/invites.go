package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Invite struct {
	ID         int64
	Token      string
	CreatedBy  int64
	CreatedAt  time.Time
	DisabledAt time.Time // zero while the link is active
	Joined     int       // members who registered through it
}

func (i Invite) Active() bool { return i.DisabledAt.IsZero() }

func (s *Store) CreateInvite(token string, createdBy int64, now time.Time) (Invite, error) {
	res, err := s.db.Exec(`INSERT INTO invites (token, created_by, created_at) VALUES (?, ?, ?)`,
		token, createdBy, Stamp(now))
	if err != nil {
		return Invite{}, fmt.Errorf("create invite: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Invite{}, err
	}
	return Invite{ID: id, Token: token, CreatedBy: createdBy, CreatedAt: now.UTC().Truncate(time.Second)}, nil
}

const inviteQuery = `
SELECT i.id, i.token, i.created_by, i.created_at, i.disabled_at,
       (SELECT COUNT(*) FROM users u WHERE u.invite_id = i.id)
FROM invites i`

func scanInvite(row scanner) (Invite, error) {
	var inv Invite
	var created string
	var disabled sql.NullString
	err := row.Scan(&inv.ID, &inv.Token, &inv.CreatedBy, &created, &disabled, &inv.Joined)
	if errors.Is(err, sql.ErrNoRows) {
		return Invite{}, ErrNotFound
	}
	if err != nil {
		return Invite{}, err
	}
	if inv.CreatedAt, err = parseStamp(created); err != nil {
		return Invite{}, err
	}
	if disabled.Valid {
		if inv.DisabledAt, err = parseStamp(disabled.String); err != nil {
			return Invite{}, err
		}
	}
	return inv, nil
}

// Invites lists every link, active ones first, newest first within each.
func (s *Store) Invites() ([]Invite, error) {
	rows, err := s.db.Query(inviteQuery + ` ORDER BY i.disabled_at IS NOT NULL, i.created_at DESC, i.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Invite
	for rows.Next() {
		inv, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// ActiveInvite finds a link by its token, only while it is still active.
func (s *Store) ActiveInvite(token string) (Invite, error) {
	return scanInvite(s.db.QueryRow(inviteQuery+` WHERE i.token = ? AND i.disabled_at IS NULL`, token))
}

// JoinViaInvite creates a member through the invite with this token, only if
// that invite is active at the moment of the insert. The check and the insert
// are one statement, so a link disabled a moment earlier cannot let anyone
// through. ErrNotFound means there is no active invite with that token.
func (s *Store) JoinViaInvite(token, sub, email, name string, now time.Time) (User, error) {
	res, err := s.db.Exec(`
INSERT INTO users (sub, email, name, role, invite_id, created_at)
SELECT ?, ?, ?, ?, id, ? FROM invites WHERE token = ? AND disabled_at IS NULL`,
		sub, email, name, RoleMember, Stamp(now), token)
	if err != nil {
		return User{}, fmt.Errorf("join via invite: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return User{}, err
	} else if n == 0 {
		return User{}, ErrNotFound
	}
	return s.UserBySub(sub)
}

// DisableInvite turns a link off for good. Disabling an already-disabled link
// keeps its original date.
func (s *Store) DisableInvite(id int64, now time.Time) error {
	_, err := s.db.Exec(`UPDATE invites SET disabled_at = ? WHERE id = ? AND disabled_at IS NULL`, Stamp(now), id)
	return err
}
