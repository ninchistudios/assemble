package store

// migrations are applied in order and never edited once released -- a released
// migration has already run against real data somewhere. Add a new entry
// instead.
var migrations = []string{
	// 1: the 2026.1 foundation. Availability, lock-in and notifications
	// arrive in 2026.2 as later migrations.
	`
-- One row, always. The defaults stand until the admin first saves.
CREATE TABLE settings (
  id         INTEGER PRIMARY KEY CHECK (id = 1),
  group_name TEXT    NOT NULL,
  slot_start TEXT    NOT NULL,  -- HH:MM, local to timezone
  slot_end   TEXT    NOT NULL,  -- HH:MM; earlier than slot_start means it runs past midnight
  timezone   TEXT    NOT NULL,  -- IANA name, e.g. Australia/Brisbane
  updated_at TEXT
);
INSERT INTO settings (id, group_name, slot_start, slot_end, timezone)
VALUES (1, 'assemble', '19:00', '22:00', 'UTC');

-- Multi-use links. The token is kept in the clear because the admin needs
-- to see and copy it again; it only grants anything while disabled_at is NULL.
CREATE TABLE invites (
  id          INTEGER PRIMARY KEY,
  token       TEXT    NOT NULL UNIQUE,
  created_by  INTEGER NOT NULL REFERENCES users(id),
  created_at  TEXT    NOT NULL,
  disabled_at TEXT
);

-- Identity is the Google sub. Email and name are refreshed on each sign-in
-- and are for display only, apart from the one-off admin bootstrap.
CREATE TABLE users (
  id         INTEGER PRIMARY KEY,
  sub        TEXT    NOT NULL UNIQUE,
  email      TEXT    NOT NULL,
  name       TEXT    NOT NULL,
  role       TEXT    NOT NULL CHECK (role IN ('admin', 'member')),
  enabled    INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
  invite_id  INTEGER REFERENCES invites(id),  -- NULL for the bootstrap admin
  created_at TEXT    NOT NULL
);

-- Only a hash of the session token is stored, so a copy of the database
-- does not let anyone sign in.
CREATE TABLE sessions (
  token_hash TEXT    PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TEXT    NOT NULL,
  expires_at TEXT    NOT NULL
);
CREATE INDEX sessions_user ON sessions(user_id);
`,

	// 2: which Google account ASSEMBLE_ADMIN_EMAIL was last granted to. The
	// address alone is not enough: emails can be reassigned or changed, so
	// once bound, only that account holds admin by virtue of the address.
	// A different value in the variable is a deliberate handover.
	`
ALTER TABLE settings ADD COLUMN admin_email TEXT;
ALTER TABLE settings ADD COLUMN admin_sub   TEXT;
`,

	// 3: the 2026.2 calendar. Days are YYYY-MM-DD in the group timezone,
	// never instants; see docs/calendar.md.
	`
-- One row per member per day they have answered. No row is [?].
CREATE TABLE availability (
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  day        TEXT    NOT NULL,
  status     TEXT    NOT NULL CHECK (status IN ('yes', 'ok', 'no')),
  note       TEXT    NOT NULL DEFAULT '',  -- public, shown to the group (#15)
  updated_at TEXT    NOT NULL,
  PRIMARY KEY (user_id, day)
);
CREATE INDEX availability_day ON availability(day);

-- A locked-in day has a row; cancelling deletes it. The notification
-- records that it happened.
CREATE TABLE lockins (
  day       TEXT    PRIMARY KEY,
  locked_by INTEGER NOT NULL REFERENCES users(id),
  locked_at TEXT    NOT NULL,
  override  INTEGER NOT NULL DEFAULT 0 CHECK (override IN (0, 1))
);

CREATE TABLE notifications (
  id         INTEGER PRIMARY KEY,
  kind       TEXT    NOT NULL CHECK (kind IN ('locked', 'cancelled')),
  day        TEXT    NOT NULL,
  actor_id   INTEGER REFERENCES users(id),
  created_at TEXT    NOT NULL
);
CREATE INDEX notifications_day ON notifications(day);

-- Dismissal is per person: one row each time someone taps it away.
CREATE TABLE dismissals (
  notification_id INTEGER NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
  user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  PRIMARY KEY (notification_id, user_id)
);
`,
}
