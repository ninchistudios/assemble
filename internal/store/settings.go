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
}

// DefaultSettings mirrors the row the first migration inserts. It is what
// pages fall back to if the settings cannot be read.
var DefaultSettings = Settings{GroupName: "assemble", SlotStart: "19:00", SlotEnd: "22:00", Timezone: "UTC"}

func (s *Store) Settings() (Settings, error) {
	var st Settings
	var updated sql.NullString
	err := s.db.QueryRow(`SELECT group_name, slot_start, slot_end, timezone, updated_at FROM settings WHERE id = 1`).
		Scan(&st.GroupName, &st.SlotStart, &st.SlotEnd, &st.Timezone, &updated)
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
	_, err := s.db.Exec(`UPDATE settings SET group_name = ?, slot_start = ?, slot_end = ?, timezone = ?, updated_at = ? WHERE id = 1`,
		st.GroupName, st.SlotStart, st.SlotEnd, st.Timezone, Stamp(now))
	return err
}
