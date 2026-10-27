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

// ErrNeedsOverride is returned when locking a day that not every enabled
// member can make, without the admin having confirmed the override.
var ErrNeedsOverride = errors.New("not everyone is available")

// Lock books a day and notifies the group. Availability is checked in the
// same transaction as the booking, so an answer changing a moment before
// can't slip past: if any enabled member isn't [YES] or [OK], the lock needs
// overrideConfirmed, and the booking records that it was an override.
func (s *Store) Lock(day cal.Day, by int64, overrideConfirmed bool, now time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var locked int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM lockins WHERE day = ?`, day).Scan(&locked); err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	if locked > 0 {
		return ErrLocked
	}
	var missing int
	err = tx.QueryRow(`
SELECT COUNT(*) FROM users u
LEFT JOIN availability a ON a.user_id = u.id AND a.day = ?
WHERE u.enabled = 1 AND (a.status IS NULL OR a.status = 'no')`, day).Scan(&missing)
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	if missing > 0 && !overrideConfirmed {
		return ErrNeedsOverride
	}
	if _, err := tx.Exec(`INSERT INTO lockins (day, locked_by, locked_at, override) VALUES (?, ?, ?, ?)`,
		day, by, Stamp(now), missing > 0); err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	if err := notify(tx, NoticeLocked, day, by, now); err != nil {
		return err
	}
	return tx.Commit()
}

// Unlock cancels a booked day and notifies the group, in one transaction.
// ErrNotFound if it wasn't locked in.
func (s *Store) Unlock(day cal.Day, by int64, now time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := unlock(tx, day, by, now); err != nil {
		return err
	}
	return tx.Commit()
}

// unlock deletes the lock-in and records the cancellation.
func unlock(tx execer, day cal.Day, by int64, now time.Time) error {
	res, err := tx.Exec(`DELETE FROM lockins WHERE day = ?`, day)
	if err != nil {
		return fmt.Errorf("unlock: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	return notify(tx, NoticeCancelled, day, by, now)
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

// ErrNeedsConfirm is returned when a status change would pull a member out
// of a locked-in day and they haven't confirmed cancelling it.
var ErrNeedsConfirm = errors.New("would cancel a locked-in day")

// ChangeStatus sets a member's status for a day. If the day is locked in and
// the member is pulling out ([YES]/[OK] to [NO]/[?]), the change cancels the
// lock-in, and needs confirmed; without it nothing changes and the result is
// ErrNeedsConfirm. The lock-in, the old status and the change are all read
// and written in one transaction, so a lock-in arriving a moment earlier
// can't be missed. cancelled reports whether the day was cancelled.
func (s *Store) ChangeStatus(userID int64, day cal.Day, status cal.Status, confirmed bool, now time.Time) (cancelled bool, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var locked int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM lockins WHERE day = ?`, day).Scan(&locked); err != nil {
		return false, fmt.Errorf("change status: %w", err)
	}
	var current string
	err = tx.QueryRow(`SELECT status FROM availability WHERE user_id = ? AND day = ?`, userID, day).Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("change status: %w", err)
	}
	pullsOut := locked > 0 && cal.Cancels(cal.Status(current), status)
	if pullsOut && !confirmed {
		return false, ErrNeedsConfirm
	}
	if err := setStatus(tx, userID, day, status, now); err != nil {
		return false, fmt.Errorf("change status: %w", err)
	}
	if pullsOut {
		if err := unlock(tx, day, userID, now); err != nil {
			return false, fmt.Errorf("change status: %w", err)
		}
	}
	return pullsOut, tx.Commit()
}
