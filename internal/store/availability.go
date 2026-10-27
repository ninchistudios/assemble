package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ninchistudios/assemble/internal/cal"
)

// Answer is one member's availability for one day.
type Answer struct {
	UserID int64
	Day    cal.Day
	Status cal.Status
	Note   string
}

// SetStatus records a member's status for a day. Setting cal.Unknown removes
// the answer, note included: [?] is the absence of a row.
func (s *Store) SetStatus(userID int64, day cal.Day, status cal.Status, now time.Time) error {
	if status == cal.Unknown {
		_, err := s.db.Exec(`DELETE FROM availability WHERE user_id = ? AND day = ?`, userID, day)
		return err
	}
	_, err := s.db.Exec(`
INSERT INTO availability (user_id, day, status, updated_at) VALUES (?, ?, ?, ?)
ON CONFLICT (user_id, day) DO UPDATE SET status = excluded.status, updated_at = excluded.updated_at`,
		userID, day, status, Stamp(now))
	if err != nil {
		return fmt.Errorf("set status: %w", err)
	}
	return nil
}

// Status is one member's status for a day; cal.Unknown if they haven't
// answered.
func (s *Store) Status(userID int64, day cal.Day) (cal.Status, error) {
	var st string
	err := s.db.QueryRow(`SELECT status FROM availability WHERE user_id = ? AND day = ?`, userID, day).Scan(&st)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return cal.Unknown, nil
		}
		return "", err
	}
	return cal.Status(st), nil
}

// Answers lists every recorded answer for days from first to last inclusive,
// keyed by day then user. Members with no row for a day are [?].
func (s *Store) Answers(first, last cal.Day) (map[cal.Day]map[int64]Answer, error) {
	rows, err := s.db.Query(`SELECT user_id, day, status, note FROM availability WHERE day >= ? AND day <= ?`, first, last)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[cal.Day]map[int64]Answer{}
	for rows.Next() {
		var a Answer
		var day, st string
		if err := rows.Scan(&a.UserID, &day, &st, &a.Note); err != nil {
			return nil, err
		}
		a.Day, a.Status = cal.Day(day), cal.Status(st)
		if out[a.Day] == nil {
			out[a.Day] = map[int64]Answer{}
		}
		out[a.Day][a.UserID] = a
	}
	return out, rows.Err()
}

// MaxNote is the longest note a member can leave on a day: a line, not a
// message thread.
const MaxNote = 140

// SetNote sets a member's public note for a day. A note belongs to an
// answer, so a member who hasn't answered (or has gone back to [?]) gets
// ErrNotFound.
func (s *Store) SetNote(userID int64, day cal.Day, note string, now time.Time) error {
	res, err := s.db.Exec(`UPDATE availability SET note = ?, updated_at = ? WHERE user_id = ? AND day = ?`,
		note, Stamp(now), userID, day)
	if err != nil {
		return fmt.Errorf("set note: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}
