package dashboard

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteReviewStore is a durable local review journal. It owns a separate
// database; it never opens or mutates producer archives. Use a private directory
// on persistent local disk (not ephemeral container storage or a network mount).
// Private deployments can instead implement ReviewStore in their existing store.
type SQLiteReviewStore struct{ db *sql.DB }

func OpenReviewStore(dir string) (*SQLiteReviewStore, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	filename := filepath.Join(dir, "reviews.sqlite")
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	if err = os.Chmod(filename, 0600); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filename)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA busy_timeout=5000; CREATE TABLE IF NOT EXISTS reviews (
 tenant TEXT NOT NULL, item TEXT NOT NULL, revision INTEGER NOT NULL,
 reviewer TEXT NOT NULL, verdict TEXT NOT NULL, note TEXT NOT NULL, at TEXT NOT NULL,
 PRIMARY KEY (tenant,item,revision));`); err != nil {
		db.Close()
		return nil, err
	}
	return &SQLiteReviewStore{db}, nil
}
func (s *SQLiteReviewStore) Close() error { return s.db.Close() }
func (s *SQLiteReviewStore) History(ctx context.Context, tenant, item string) ([]Review, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT revision,reviewer,verdict,note,at FROM reviews WHERE tenant=? AND item=? ORDER BY revision`, tenant, item)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Review{}
	for rows.Next() {
		r := Review{Tenant: tenant, Item: item, ActorType: "human"}
		var at string
		if err := rows.Scan(&r.Revision, &r.Reviewer, &r.Verdict, &r.Note, &at); err != nil {
			return nil, err
		}
		r.At, err = time.Parse(time.RFC3339Nano, at)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *SQLiteReviewStore) Save(ctx context.Context, p Principal, item string, c ReviewChange) (Review, error) {
	if p.Tenant == "" || p.Reviewer == "" || item == "" || c.ExpectedRevision < 0 || len(c.Note) > 8000 || (c.Verdict != "correct" && c.Verdict != "incorrect" && c.Verdict != "uncertain") {
		return Review{}, errors.New("invalid review")
	}
	r := Review{Revision: c.ExpectedRevision + 1, Tenant: p.Tenant, Item: item, Reviewer: p.Reviewer, ActorType: "human", Verdict: c.Verdict, Note: c.Note, At: time.Now().UTC()}
	// A single statement holds SQLite's writer lock while comparing and appending.
	result, err := s.db.ExecContext(ctx, `INSERT INTO reviews (tenant,item,revision,reviewer,verdict,note,at)
 SELECT ?,?,?,?,?,?,? WHERE (SELECT COALESCE(MAX(revision),0) FROM reviews WHERE tenant=? AND item=?)=?`, p.Tenant, item, r.Revision, p.Reviewer, r.Verdict, r.Note, r.At.Format(time.RFC3339Nano), p.Tenant, item, c.ExpectedRevision)
	if err != nil {
		return Review{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return Review{}, err
	}
	if n != 1 {
		return Review{}, ErrReviewConflict
	}
	return r, nil
}
