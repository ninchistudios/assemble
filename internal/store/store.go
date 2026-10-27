// Package store owns the SQLite database: schema, migrations and every query.
// Nothing outside this package writes SQL.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// TimeLayout is how instants are stored: UTC, second precision, sortable as
// text. Calendar days (2026.2) are stored separately as YYYY-MM-DD in the
// group timezone, never as instants. See agents.md > Stack.
const TimeLayout = "2006-01-02T15:04:05Z"

// Stamp renders an instant in the stored layout.
func Stamp(t time.Time) string { return t.UTC().Format(TimeLayout) }

// parseStamp is the inverse of Stamp.
func parseStamp(s string) (time.Time, error) { return time.Parse(TimeLayout, s) }

// ErrNotFound is returned when a lookup matches no row.
var ErrNotFound = errors.New("not found")

type Store struct{ db *sql.DB }

// Open connects to the database at path, applying any outstanding migrations.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create %s: %w", dir, err)
		}
	}
	dsn := "file:" + path +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=foreign_keys(ON)" +
		"&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	// One connection. A handful of people tapping a few times a week will
	// never notice serialised access, and it removes every "database is
	// locked" failure mode.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping %s: %w", path, err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Ping reports whether the database is reachable, for /healthz.
func (s *Store) Ping() error { return s.db.Ping() }

// migrate applies migrations in order, tracking progress in PRAGMA
// user_version. No down-migrations: a bad migration is recovered from a
// backup of the volume rather than reversed.
func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	if version > len(migrations) {
		// An older image started against a database a newer one migrated.
		// Running on would mean writing rows the schema no longer matches.
		return fmt.Errorf("database is at schema %d but this build only knows %d; run a newer image", version, len(migrations))
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("migration %d: begin: %w", i+1, err)
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		// PRAGMA will not take a bound parameter, so the number is formatted
		// in. It is a loop index, never user input.
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: set user_version: %w", i+1, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migration %d: commit: %w", i+1, err)
		}
	}
	return nil
}

// Version reports how many migrations have been applied.
func (s *Store) Version() (int, error) {
	var v int
	err := s.db.QueryRow("PRAGMA user_version").Scan(&v)
	return v, err
}
