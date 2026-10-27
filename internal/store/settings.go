package store

import (
	"database/sql"
	"time"
)

// Settings are the group-wide values the admin edits. There is exactly one
// row, created with defaults by the first migration.
type Settings struct {
	GroupName string
	SlotStart string // HH:MM
	SlotEnd   string // HH:MM; earlier than SlotStart means it runs past midnight
	Timezone  string // IANA name
	UpdatedAt time.Time

	// BannedWords is the admin's language filter, comma-separated.
	BannedWords string
}

// DefaultSettings mirrors the row the first migration inserts. It is what
// pages fall back to if the settings cannot be read.
var DefaultSettings = Settings{GroupName: "assemble", SlotStart: "19:00", SlotEnd: "22:00", Timezone: "UTC"}

func (s *Store) Settings() (Settings, error) {
	var st Settings
	var updated sql.NullString
	err := s.db.QueryRow(`SELECT group_name, slot_start, slot_end, timezone, updated_at, banned_words FROM settings WHERE id = 1`).
		Scan(&st.GroupName, &st.SlotStart, &st.SlotEnd, &st.Timezone, &updated, &st.BannedWords)
	if err != nil {
		return DefaultSettings, err
	}
	if updated.Valid {
		if st.UpdatedAt, err = parseStamp(updated.String); err != nil {
			return DefaultSettings, err
		}
	}
	return st, nil
}

// SaveSettings stores already-validated settings.
func (s *Store) SaveSettings(st Settings, now time.Time) error {
	_, err := s.db.Exec(`UPDATE settings SET group_name = ?, slot_start = ?, slot_end = ?, timezone = ?, banned_words = ?, updated_at = ? WHERE id = 1`,
		st.GroupName, st.SlotStart, st.SlotEnd, st.Timezone, st.BannedWords, Stamp(now))
	return err
}

// AdminBinding returns the ASSEMBLE_ADMIN_EMAIL value that last granted admin,
// and the Google sub it was granted to. Both are empty until the first grant.
func (s *Store) AdminBinding() (email, sub string, err error) {
	var e, u sql.NullString
	err = s.db.QueryRow(`SELECT admin_email, admin_sub FROM settings WHERE id = 1`).Scan(&e, &u)
	return e.String, u.String, err
}

// BindAdmin records that ASSEMBLE_ADMIN_EMAIL=email granted admin to sub.
func (s *Store) BindAdmin(email, sub string) error {
	_, err := s.db.Exec(`UPDATE settings SET admin_email = ?, admin_sub = ? WHERE id = 1`, email, sub)
	return err
}

// AdminsWithEmail lists enabled admins whose email matches, case-insensitively.
// Used once, to bind an admin bootstrapped before bindings were recorded.
func (s *Store) AdminsWithEmail(email string) ([]User, error) {
	rows, err := s.db.Query(`SELECT `+userCols+` FROM users WHERE role = 'admin' AND enabled = 1 AND email = ? COLLATE NOCASE`, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
