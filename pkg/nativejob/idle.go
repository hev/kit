package nativejob

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"path/filepath"
	"time"
)

const IdleProofVersion = "native-idle-journal/v1"

// IdleProof observes a single SQLite snapshot. It grants no admission, process
// absence, tree termination or capacity authority. A later dispatch can change it.
type IdleProof struct {
	Version           string     `json:"version"`
	Enrollment        Enrollment `json:"enrollment"`
	JobCount          uint64     `json:"job_count"`
	CompletedACKCount uint64     `json:"completed_ack_count"`
	ObservedAt        time.Time  `json:"observed_at"`
}

// ReadOnlyIdle accepts only an existing authenticated journal containing zero
// jobs or valid Completed+ACK jobs. No Open/setup/recovery/ACK is performed.
// SQLite mode=ro can create WAL/SHM sidecars; this is not a zero-filesystem-write claim.
func ReadOnlyIdle(ctx context.Context, root string, enrollment Enrollment) (IdleProof, error) {
	var out IdleProof
	if journalPlatformPreflight() != nil || !filepath.IsAbs(root) || filepath.Clean(root) != root || enrollment.HostID == "" || enrollment.ExecutorID == "" {
		return out, ErrConflict
	}
	closeFiles, e := journalOpenReadOnly(root)
	if e != nil {
		return out, e
	}
	defer closeFiles()
	u := url.URL{Scheme: "file", Path: filepath.Join(root, "journal.sqlite")}
	q := u.Query()
	q.Set("mode", "ro")
	q.Add("_pragma", "query_only(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	db, e := sql.Open("sqlite", u.String())
	if e != nil {
		return out, e
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	tx, e := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	var got string
	if e = tx.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key='identity'`).Scan(&got); e != nil {
		return out, e
	}
	expected := Version + ":" + Digest(enrollment)
	if got != expected && got != expected+":"+StartVersion {
		return out, ErrConflict
	}
	rows, e := tx.QueryContext(ctx, `SELECT id,body FROM jobs`)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	out = IdleProof{Version: IdleProofVersion, Enrollment: enrollment}
	completed := map[string]Job{}
	for rows.Next() {
		var id, body string
		var j Job
		if e = rows.Scan(&id, &body); e != nil {
			return IdleProof{}, e
		}
		if len(body) > 1<<20 || len(completed) >= 10000 || json.Unmarshal([]byte(body), &j) != nil || j.ID != id || j.Request.validate(enrollment) != nil || j.State != "Completed" || !j.Acknowledged || j.Result == nil || j.EventID == "" || Digest(*j.Result) != j.ResultDigest {
			return IdleProof{}, ErrConflict
		}
		completed[j.ID] = j
		out.JobCount++
		out.CompletedACKCount++
	}
	if e = rows.Err(); e != nil {
		return IdleProof{}, e
	}
	if e = rows.Close(); e != nil {
		return IdleProof{}, e
	}
	events, e := tx.QueryContext(ctx, `SELECT job_id,event_id,body,ack FROM outbox`)
	if e != nil {
		return IdleProof{}, e
	}
	defer events.Close()
	seen := map[string]bool{}
	for events.Next() {
		var id, eventID, body string
		var ack int
		var event Event
		if e = events.Scan(&id, &eventID, &body, &ack); e != nil {
			return IdleProof{}, e
		}
		j, ok := completed[id]
		if !ok || seen[id] || ack != 1 || len(body) > 1<<20 || json.Unmarshal([]byte(body), &event) != nil || eventID != j.EventID || encode(event) != encode(Event{Version: Version, EventID: j.EventID, JobID: j.ID, Identity: j.Request.Identity, Result: *j.Result, ResultDigest: j.ResultDigest, StartReceipt: j.StartReceipt}) {
			return IdleProof{}, ErrConflict
		}
		seen[id] = true
	}
	if e = events.Err(); e != nil {
		return IdleProof{}, e
	}
	if len(seen) != len(completed) {
		return IdleProof{}, ErrConflict
	}
	if e = ctx.Err(); e != nil {
		return IdleProof{}, e
	}
	out.ObservedAt = time.Now().UTC()
	return out, nil
}
