package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const createTableSQL = `
CREATE TABLE IF NOT EXISTS reminders (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	chat_id INTEGER NOT NULL,
	remind_at DATETIME NOT NULL,
	message TEXT NOT NULL,
	sent BOOLEAN NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL,
	sent_at DATETIME,
	sent_30m BOOLEAN NOT NULL DEFAULT 0,
	sent_10m BOOLEAN NOT NULL DEFAULT 0,
	sent_5m BOOLEAN NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_reminders_sent_remind_at ON reminders(sent, remind_at);

CREATE TABLE IF NOT EXISTS routines (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	chat_id INTEGER NOT NULL,
	schedule_type TEXT NOT NULL,
	schedule_param TEXT NOT NULL,
	message TEXT NOT NULL,
	created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_routines_chat_id ON routines(chat_id);
`

// advanceNotificationColumns: for existing DBs created before advance notifications.
var advanceNotificationColumns = []string{"sent_30m", "sent_10m", "sent_5m"}

// Open opens a SQLite database at path and runs migrations (creates reminders table).
// If path is not ":memory:", the parent directory is created if missing.
func Open(path string) (*sql.DB, error) {
	if path != ":memory:" {
		if dir := filepath.Dir(path); dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("mkdir: %w", err)
			}
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sql open: %w", err)
	}
	if _, err := db.Exec(createTableSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	for _, col := range advanceNotificationColumns {
		_, err := db.Exec("ALTER TABLE reminders ADD COLUMN " + col + " BOOLEAN NOT NULL DEFAULT 0")
		if err != nil && !strings.Contains(err.Error(), "duplicate column") {
			_ = db.Close()
			return nil, fmt.Errorf("migrate advance column %s: %w", col, err)
		}
	}
	return db, nil
}
