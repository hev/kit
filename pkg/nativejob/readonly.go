package nativejob

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"path/filepath"
)

// ReadOnlyOriginal authenticates an existing enrollment and optionally reads one
// original job without schema setup, enrollment, fence issuance or recovery.
// SQLite mode=ro forbids database writes. No missing journal is created.
func ReadOnlyOriginal(ctx context.Context, root string, enrollment Enrollment, id string) (j Job, err error) {
	if journalPlatformPreflight() != nil || !filepath.IsAbs(root) || filepath.Clean(root) != root || enrollment.HostID == "" || enrollment.ExecutorID == "" {
		return j, ErrConflict
	}
	closeFiles, e := journalOpenReadOnly(root)
	if closeFiles != nil {
		defer func() {
			if closeErr := closeFiles(); closeErr != nil {
				err = errors.Join(err, closeErr)
				j = Job{}
			}
		}()
	}
	if e != nil {
		return j, e
	}
	if closeFiles == nil {
		return j, ErrConflict
	}
	path := filepath.Join(root, "journal.sqlite")
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("mode", "ro")
	q.Add("_pragma", "query_only(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	db, e := sql.Open("sqlite", u.String())
	if e != nil {
		return j, e
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var got string
	if e = db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key='identity'`).Scan(&got); e != nil {
		return j, e
	}
	expected := Version + ":" + Digest(enrollment)
	if got != expected && got != expected+":"+StartVersion {
		return j, ErrConflict
	}
	if id == "" {
		return j, nil
	}
	var body string
	if e = db.QueryRowContext(ctx, `SELECT body FROM jobs WHERE id=?`, id).Scan(&body); e != nil {
		return j, e
	}
	if json.Unmarshal([]byte(body), &j) != nil || j.ID != id || j.Request.validate(enrollment) != nil {
		return Job{}, ErrConflict
	}
	return j, nil
}
