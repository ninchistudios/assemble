package store

import (
	"fmt"
	"time"

	"github.com/ninchistudios/assemble/internal/cal"
)

// Notice kinds.
const (
	NoticeLocked    = "locked"
	NoticeCancelled = "cancelled"
)

// Notice is a notification as one member sees it.
type Notice struct {
	ID        int64
	Kind      string
	Day       cal.Day
	ActorID   int64
	ActorName string
	CreatedAt time.Time
}

// notify records that something happened to a day. It runs inside the
// transaction that made the change, so a lock-in or cancellation and its
// notification are never one without the other.
func notify(tx execer, kind string, day cal.Day, actor int64, now time.Time) error {
	_, err := tx.Exec(`INSERT INTO notifications (kind, day, actor_id, created_at) VALUES (?, ?, ?, ?)`,
		kind, day, actor, Stamp(now))
	if err != nil {
		return fmt.Errorf("notify: %w", err)
	}
	return nil
}

// Notices lists what userID hasn't dismissed, newest first. The actor is
// named as the group sees them: their display name if they chose one. Notices for days
// before today have expired and are left out.
func (s *Store) Notices(userID int64, today cal.Day) ([]Notice, error) {
	rows, err := s.db.Query(`
SELECT n.id, n.kind, n.day, COALESCE(n.actor_id, 0), COALESCE(NULLIF(u.display_name, ''), u.name, ''), n.created_at
FROM notifications n
LEFT JOIN users u ON u.id = n.actor_id
WHERE n.day >= ?
  AND NOT EXISTS (SELECT 1 FROM dismissals d WHERE d.notification_id = n.id AND d.user_id = ?)
ORDER BY n.created_at DESC, n.id DESC`, today, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Notice
	for rows.Next() {
		var n Notice
		var day, created string
		if err := rows.Scan(&n.ID, &n.Kind, &day, &n.ActorID, &n.ActorName, &created); err != nil {
			return nil, err
		}
		n.Day = cal.Day(day)
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
