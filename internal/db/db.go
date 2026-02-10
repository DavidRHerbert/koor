package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Open opens (or creates) the SQLite database at dataDir/data.db.
// It enables WAL mode for concurrent reads and runs migrations.
func Open(dataDir string) (*sql.DB, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "data.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for concurrent reads.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}

	// Set busy timeout for write contention (5 seconds).
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

// OpenMemory opens an in-memory SQLite database for testing.
func OpenMemory() (*sql.DB, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, fmt.Errorf("open memory database: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS state (
			key          TEXT PRIMARY KEY,
			value        BLOB,
			version      INTEGER NOT NULL DEFAULT 1,
			hash         TEXT NOT NULL,
			content_type TEXT NOT NULL DEFAULT 'application/json',
			updated_at   DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_by   TEXT NOT NULL DEFAULT ''
		)`,

		`CREATE TABLE IF NOT EXISTS specs (
			project    TEXT NOT NULL,
			name       TEXT NOT NULL,
			data       BLOB,
			version    INTEGER NOT NULL DEFAULT 1,
			hash       TEXT NOT NULL,
			updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (project, name)
		)`,

		`CREATE TABLE IF NOT EXISTS events (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			topic      TEXT NOT NULL,
			data       BLOB,
			source     TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		`CREATE TABLE IF NOT EXISTS instances (
			id            TEXT PRIMARY KEY,
			name          TEXT NOT NULL,
			workspace     TEXT NOT NULL DEFAULT '',
			intent        TEXT NOT NULL DEFAULT '',
			stack         TEXT NOT NULL DEFAULT '',
			token         TEXT NOT NULL DEFAULT '',
			registered_at DATETIME NOT NULL DEFAULT (datetime('now')),
			last_seen     DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		`CREATE TABLE IF NOT EXISTS validation_rules (
			project     TEXT NOT NULL,
			rule_id     TEXT NOT NULL,
			severity    TEXT NOT NULL DEFAULT 'error',
			match_type  TEXT NOT NULL DEFAULT 'regex',
			pattern     TEXT NOT NULL,
			message     TEXT NOT NULL DEFAULT '',
			stack       TEXT NOT NULL DEFAULT '',
			applies_to  TEXT NOT NULL DEFAULT '["*"]',
			source      TEXT NOT NULL DEFAULT 'local',
			status      TEXT NOT NULL DEFAULT 'accepted',
			proposed_by TEXT NOT NULL DEFAULT '',
			context     TEXT NOT NULL DEFAULT '',
			created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (project, rule_id)
		)`,
	}

	// Migrate existing databases: add columns that may not exist yet.
	// Errors are ignored because ALTER TABLE fails if the column already exists.
	alterMigrations := []string{
		`ALTER TABLE instances ADD COLUMN stack TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE validation_rules ADD COLUMN stack TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE validation_rules ADD COLUMN source TEXT NOT NULL DEFAULT 'local'`,
		`ALTER TABLE validation_rules ADD COLUMN status TEXT NOT NULL DEFAULT 'accepted'`,
		`ALTER TABLE validation_rules ADD COLUMN proposed_by TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE validation_rules ADD COLUMN context TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE validation_rules ADD COLUMN created_at DATETIME NOT NULL DEFAULT (datetime('now'))`,
	}
	for _, ddl := range alterMigrations {
		db.Exec(ddl) // ignore error — column may already exist
	}

	// Create indexes for common queries.
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_events_topic ON events(topic)`,
		`CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_instances_last_seen ON instances(last_seen)`,
		`CREATE INDEX IF NOT EXISTS idx_instances_stack ON instances(stack)`,
	}

	for _, ddl := range tables {
		if _, err := db.Exec(ddl); err != nil {
			return fmt.Errorf("exec DDL: %w", err)
		}
	}
	for _, ddl := range indexes {
		if _, err := db.Exec(ddl); err != nil {
			return fmt.Errorf("exec index: %w", err)
		}
	}

	return nil
}
