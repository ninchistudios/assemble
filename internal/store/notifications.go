package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ninchistudios/assemble/internal/cal"
)

// Notice kinds.
const (
	NoticeLocked    = "locked"
	NoticeCancelled = "cancelled"
	NoticeJoined    = "joined" // a member joined (#38): admin and managers only
	NoticeMOTD      = "motd"   // the admin's message of the day (#40)
)

// Notice is a notification as one member sees it.
type Notice struct {
	ID        int64
	Kind      string
	Day       cal.Day // empty for a notice not about a day
	Body      string  // the MOTD's text
	ActorID   int64
	ActorName string
	CreatedAt time.Time
}

// notify records that something happened to a day, for everyone. It runs
// inside the transaction that made the change, so a lock-in or cancellation
// and its notification are never one without the other.
func notify(tx execer, kind string, day cal.Day, actor int64, now time.Time) error {
	return addNotice(tx, kind, day, "all", "", actor, now)
}

// addNotice records any notice. An empty day makes it a notice that never
// expires: it stays until dismissed. audience is "all" or "managers".
func addNotice(tx execer, kind string, day cal.Day, audience, body string, actor int64, now time.Time) error {
	var d any
	if day != "" {
		d = day
	}
	_, err := tx.Exec(`INSERT INTO notifications (kind, day, audience, body, actor_id, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		kind, d, audience, body, actor, Stamp(now))
	if err != nil {
		return fmt.Errorf("notify: %w", err)
	}
	return nil
}

// Notices lists what userID hasn't dismissed, newest first. manager says
// whether they see the notices meant for the admin and managers. The actor
// is named as the group sees them: their display name if they chose one.
// Notices for days before today have expired and are left out; notices not
// about a day never expire.
func (s *Store) Notices(userID int64, manager bool, today cal.Day) ([]Notice, error) {
	rows, err := s.db.Query(`
SELECT n.id, n.kind, n.day, n.body, COALESCE(n.actor_id, 0), COALESCE(NULLIF(u.display_name, ''), u.name, ''), n.created_at
FROM notifications n
LEFT JOIN users u ON u.id = n.actor_id
WHERE (n.day IS NULL OR n.day >= ?)
  AND (n.audience = 'all' OR ?)
  AND NOT EXISTS (SELECT 1 FROM dismissals d WHERE d.notification_id = n.id AND d.user_id = ?)
ORDER BY n.created_at DESC, n.id DESC`, today, manager, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Notice
	for rows.Next() {
		var n Notice
		var day sql.NullString
		var created string
		if err := rows.Scan(&n.ID, &n.Kind, &day, &n.Body, &n.ActorID, &n.ActorName, &created); err != nil {
			return nil, err
		}
		n.Day = cal.Day(day.String)
		if n.CreatedAt, err = parseStamp(created); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Dismiss hides a notice from one member for good. Dismissing twice, or a
// notice that doesn't exist, is harmless.
func (s *Store) Dismiss(noticeID, userID int64) error {
	_, err := s.db.Exec(`
INSERT INTO dismissals (notification_id, user_id)
SELECT id, ? FROM notifications WHERE id = ?
ON CONFLICT DO NOTHING`, userID, noticeID)
	return err
}

// MOTD is the current message of the day, or "" if there is none.
func (s *Store) MOTD() (string, error) {
	var body string
	err := s.db.QueryRow(`SELECT body FROM notifications WHERE kind = 'motd' ORDER BY id DESC LIMIT 1`).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return body, err
}

// SetMOTD replaces the message of the day. Only one is ever live: the old
// one goes, dismissals and all, so a new message shows to everyone again,
// including those who dismissed the last. Empty just removes it. Saving the
// same text again changes nothing, so it doesn't reappear for anyone.
// changed reports whether anything was replaced.
func (s *Store) SetMOTD(body string, by int64, now time.Time) (changed bool, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var current string
	err = tx.QueryRow(`SELECT body FROM notifications WHERE kind = 'motd' ORDER BY id DESC LIMIT 1`).Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("set motd: %w", err)
	}
	if body == current {
		return false, nil
	}
	if _, err := tx.Exec(`DELETE FROM notifications WHERE kind = 'motd'`); err != nil {
		return false, fmt.Errorf("set motd: %w", err)
	}
	if body != "" {
		if err := addNotice(tx, NoticeMOTD, "", "all", body, by, now); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}
