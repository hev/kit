package nativejob

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
)

// ReadOnlyOriginal authenticates an existing enrollment and optionally reads one
// original job without schema setup, enrollment, fence issuance or recovery.
// SQLite mode=ro forbids database writes. No missing journal is created.
func ReadOnlyOriginal(ctx context.Context, root string, enrollment Enrollment, id string) (Job, error) {
	var j Job
	if journalPlatformPreflight() != nil || !filepath.IsAbs(root) || filepath.Clean(root) != root || enrollment.HostID == "" || enrollment.ExecutorID == "" {
		return j, ErrConflict
	}
	st, e := os.Lstat(root)
	if e != nil || !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return j, ErrConflict
	}
	path := filepath.Join(root, "journal.sqlite")
	st, e = os.Lstat(path)
	if e != nil || !st.Mode().IsRegular() {
		return j, ErrConflict
	}
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
