package daemon

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Ledger struct {
	db *sql.DB
}

func OpenLedger(dir string) (*Ledger, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}
	dbPath := filepath.Join(dir, "ledger.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", dbPath, err)
	}
	// WAL mode for better concurrent read performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("pragma wal: %w", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS uploaded (
			path        TEXT PRIMARY KEY,
			sha256      TEXT NOT NULL,
			size_bytes  INTEGER NOT NULL,
			uploaded_at TEXT NOT NULL,
			s3_key      TEXT NOT NULL
		)
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create table: %w", err)
	}
	return &Ledger{db: db}, nil
}

// IsUploaded returns true if the file at path with the given sha256 has
// already been uploaded. A changed hash for the same path returns false
// (re-upload needed).
func (l *Ledger) IsUploaded(path, sha256 string) (bool, error) {
	var count int
	err := l.db.QueryRow(
		"SELECT COUNT(*) FROM uploaded WHERE path = ? AND sha256 = ?",
		path, sha256,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("query uploaded: %w", err)
	}
	return count > 0, nil
}

// MarkUploaded records a successful upload in the ledger.
func (l *Ledger) MarkUploaded(path, sha256 string, sizeBytes int64, s3Key string) error {
	_, err := l.db.Exec(`
		INSERT INTO uploaded (path, sha256, size_bytes, uploaded_at, s3_key)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			sha256      = excluded.sha256,
			size_bytes  = excluded.size_bytes,
			uploaded_at = excluded.uploaded_at,
			s3_key      = excluded.s3_key
	`, path, sha256, sizeBytes, time.Now().UTC().Format(time.RFC3339), s3Key)
	return err
}

func (l *Ledger) Close() error {
	return l.db.Close()
}
