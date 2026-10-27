package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ninchistudios/assemble/internal/cal"
)

// Lockin is a booked day: the group meets.
type Lockin struct {
	Day      cal.Day
	LockedBy int64
	LockedAt time.Time
	Override bool // locked although not everyone was available
}

// ErrLocked is returned when locking a day that is already locked in.
var ErrLocked = errors.New("already locked in")

// Lock books a day. override records that not every member was available.
func (s *Store) Lock(day cal.Day, by int64, override bool, now time.Time) error {
	res, err := s.db.Exec(`INSERT INTO lockins (day, locked_by, locked_at, override) VALUES (?, ?, ?, ?) ON CONFLICT (day) DO NOTHING`,
		day, by, Stamp(now), override)
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrLocked
	}
	return nil
}

// Unlock cancels a booked day. ErrNotFound if it wasn't locked in.
func (s *Store) Unlock(day cal.Day) error {
	res, err := s.db.Exec(`DELETE FROM lockins WHERE day = ?`, day)
	if err != nil {
		return fmt.Errorf("unlock: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// LockinFor returns the day's lock-in, or ErrNotFound.
func (s *Store) LockinFor(day cal.Day) (Lockin, error) {
	var l Lockin
	var d, at string
	err := s.db.QueryRow(`SELECT day, locked_by, locked_at, override FROM lockins WHERE day = ?`, day).
		Scan(&d, &l.LockedBy, &at, &l.Override)
	if errors.Is(err, sql.ErrNoRows) {
		return Lockin{}, ErrNotFound
	}
	if err != nil {
		return Lockin{}, err
	}
	l.Day = cal.Day(d)
	l.LockedAt, err = parseStamp(at)
	return l, err
}

// Lockins returns every lock-in from first to last inclusive, by day.
func (s *Store) Lockins(first, last cal.Day) (map[cal.Day]Lockin, error) {
	rows, err := s.db.Query(`SELECT day, locked_by, locked_at, override FROM lockins WHERE day >= ? AND day <= ?`, first, last)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[cal.Day]Lockin{}
	for rows.Next() {
		var l Lockin
		var d, at string
		if err := rows.Scan(&d, &l.LockedBy, &at, &l.Override); err != nil {
			return nil, err
		}
		l.Day = cal.Day(d)
		if l.LockedAt, err = parseStamp(at); err != nil {
			return nil, err
		}
		out[l.Day] = l
	}
	return out, rows.Err()
}

// PullOut records a member's new status on a locked-in day and cancels the
// lock-in, in one transaction: there is never a moment where the member has
// pulled out but the day still shows as booked, or the reverse.
func (s *Store) PullOut(userID int64, day cal.Day, status cal.Status, now time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := setStatus(tx, userID, day, status, now); err != nil {
		return fmt.Errorf("pull out: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM lockins WHERE day = ?`, day); err != nil {
		return fmt.Errorf("pull out: %w", err)
	}
	return tx.Commit()
}
