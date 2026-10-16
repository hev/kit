package nativejob

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db         *sql.DB
	root       string
	enrollment Enrollment
	now        func() time.Time
	boundary   func(string) error
}

// Open binds a private journal to an explicit enrollment. SQLite WAL with FULL
// synchronous commits is the durability boundary; callers must use local durable
// storage with working fsync, never a network filesystem or temporary volume.
func Open(root string, enrollment Enrollment) (*Store, error) {
	if !filepath.IsAbs(root) || enrollment.HostID == "" || enrollment.ExecutorID == "" {
		return nil, errors.New("absolute root and explicit enrollment required")
	}
	if err := os.Mkdir(root, 0700); err != nil && !os.IsExist(err) {
		return nil, err
	}
	st, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return nil, errors.New("journal root must be a private directory (0700)")
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "journal.sqlite"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, root: root, enrollment: enrollment, now: time.Now}
	fail := func(err error) (*Store, error) { db.Close(); return nil, err }
	for _, q := range []string{"PRAGMA busy_timeout=5000", "PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL", `CREATE TABLE IF NOT EXISTS metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL)`, `CREATE TABLE IF NOT EXISTS fences (order_uid TEXT PRIMARY KEY, body TEXT NOT NULL)`, `CREATE TABLE IF NOT EXISTS jobs (id TEXT PRIMARY KEY, intent TEXT UNIQUE NOT NULL, body TEXT NOT NULL)`, `CREATE TABLE IF NOT EXISTS outbox (event_id TEXT PRIMARY KEY, job_id TEXT UNIQUE NOT NULL, body TEXT NOT NULL, ack INTEGER NOT NULL DEFAULT 0)`} {
		if _, err = db.Exec(q); err != nil {
			return fail(err)
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback()
	expected := Version + ":" + Digest(enrollment)
	if _, err = tx.Exec(`INSERT OR IGNORE INTO metadata VALUES ('identity',?)`, expected); err != nil {
		return fail(err)
	}
	var got string
	if err = tx.QueryRow(`SELECT value FROM metadata WHERE key='identity'`).Scan(&got); err != nil {
		return fail(err)
	}
	if got != expected {
		return fail(errors.New("journal version/enrollment mismatch"))
	}
	if err = tx.Commit(); err != nil {
		return fail(err)
	}
	// Persist initial directory entries as well as database contents.
	dir, err := os.Open(root)
	if err != nil {
		return fail(err)
	}
	err = dir.Sync()
	dir.Close()
	if err != nil {
		return fail(err)
	}
	parent, err := os.Open(filepath.Dir(root))
	if err != nil {
		return fail(err)
	}
	err = parent.Sync()
	parent.Close()
	if err != nil {
		return fail(err)
	}
	return s, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) hit(name string) error {
	if s.boundary != nil {
		return s.boundary(name)
	}
	return nil
}
func encode(v any) string { b, _ := json.Marshal(v); return string(b) }
func load(tx *sql.Tx, id string) (Job, error) {
	var body string
	err := tx.QueryRow(`SELECT body FROM jobs WHERE id=? OR intent=?`, id, id).Scan(&body)
	if err != nil {
		return Job{}, err
	}
	var j Job
	err = json.Unmarshal([]byte(body), &j)
	return j, err
}
func save(tx *sql.Tx, j Job) error {
	_, err := tx.Exec(`UPDATE jobs SET body=? WHERE id=?`, encode(j), j.ID)
	return err
}
func (s *Store) check(tx *sql.Tx, i Identity) error {
	var body string
	if err := tx.QueryRow(`SELECT body FROM fences WHERE order_uid=?`, i.OrderUID).Scan(&body); err != nil {
		return ErrFence
	}
	var f Fence
	if json.Unmarshal([]byte(body), &f) != nil || f.Epoch != i.Fence || f.ChefID != i.ChefID || !s.now().Before(f.Expires) {
		return ErrFence
	}
	return nil
}

// AdvanceFence is an administrative, local enrollment-owner operation. The
// adapter must authenticate upstream authority before calling it. Same-epoch
// expiry changes are forbidden; renewals use a higher epoch.
func (s *Store) AdvanceFence(f Fence) error {
	if f.OrderUID == "" || f.ChefID == "" || f.Epoch == 0 || !s.now().Before(f.Expires) {
		return ErrFence
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Acquire SQLite's writer lock before reading the old epoch.
	if _, err = tx.Exec(`INSERT OR IGNORE INTO fences VALUES (?,?)`, f.OrderUID, encode(f)); err != nil {
		return err
	}
	var body string
	if err = tx.QueryRow(`SELECT body FROM fences WHERE order_uid=?`, f.OrderUID).Scan(&body); err != nil {
		return err
	}
	var old Fence
	if err = json.Unmarshal([]byte(body), &old); err != nil {
		return err
	}
	if old.Epoch > f.Epoch || (old.Epoch == f.Epoch && encode(old) != encode(f)) {
		return ErrFence
	}
	if _, err = tx.Exec(`UPDATE fences SET body=? WHERE order_uid=?`, encode(f), f.OrderUID); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Submit(r Request) (Job, error) {
	if err := r.validate(s.enrollment); err != nil {
		return Job{}, err
	}
	if !filepath.IsAbs(r.Directory) || !filepath.IsAbs(r.Argv[0]) {
		return Job{}, errors.New("absolute executable and directory required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	// Serialize admission against fence transfer and competing submissions.
	if _, err = tx.Exec(`UPDATE metadata SET value=value WHERE key='identity'`); err != nil {
		return Job{}, err
	}
	if err = s.check(tx, r.Identity); err != nil {
		return Job{}, err
	}
	j, err := load(tx, r.IntentKey)
	if err == nil {
		if encode(j.Request) != encode(r) {
			return Job{}, ErrConflict
		}
		return j, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Job{}, err
	}
	j = Job{ID: "job-" + Digest(r.IntentKey)[7:], Request: r, State: "Pending"}
	if _, err = tx.Exec(`INSERT INTO jobs VALUES (?,?,?)`, j.ID, r.IntentKey, encode(j)); err != nil {
		return Job{}, err
	}
	if err = tx.Commit(); err != nil {
		return Job{}, err
	}
	if err = s.hit("intent-committed"); err != nil {
		return Job{}, err
	}
	return j, nil
}
func (s *Store) Query(key string) (Job, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	return load(tx, key)
}
func (s *Store) Outbox() ([]Event, error) {
	rows, err := s.db.Query(`SELECT body FROM outbox WHERE ack=0 ORDER BY event_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []Event{}
	for rows.Next() {
		var b string
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		var e Event
		if err = json.Unmarshal([]byte(b), &e); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
func finish(tx *sql.Tx, j *Job, r Result) error {
	j.State = "Completed"
	j.Result = &r
	j.ResultDigest = Digest(r)
	j.EventID = "result-" + Digest(struct{ ID, Digest string }{j.ID, j.ResultDigest})[7:]
	e := Event{Version: Version, EventID: j.EventID, JobID: j.ID, Identity: j.Request.Identity, Result: r, ResultDigest: j.ResultDigest}
	if err := save(tx, *j); err != nil {
		return err
	}
	_, err := tx.Exec(`INSERT INTO outbox(event_id,job_id,body) VALUES(?,?,?)`, e.EventID, j.ID, encode(e))
	return err
}
func (s *Store) Ack(id string, identity Identity, eventID, digest string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE metadata SET value=value WHERE key='identity'`); err != nil {
		return err
	}
	j, err := load(tx, id)
	if err != nil {
		return err
	}
	if encode(identity) != encode(j.Request.Identity) || eventID != j.EventID || digest != j.ResultDigest || j.Result == nil {
		return ErrConflict
	}
	// ACK is a receipt for an immutable historical result, not a new effect. It
	// deliberately survives fence transfer; the receiver checks current admission.
	j.Acknowledged = true
	if err = save(tx, j); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE outbox SET ack=1 WHERE event_id=?`, eventID); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return s.hit("ack-committed")
}
func (s *Store) Reconcile(id string, current Identity, in Inspection) (Job, error) {
	if in.ID == "" || in.Evidence == "" || in.Inspector == "" {
		return Job{}, errors.New("inspection identity and evidence required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE metadata SET value=value WHERE key='identity'`); err != nil {
		return Job{}, err
	}
	j, err := load(tx, id)
	if err != nil {
		return Job{}, err
	}
	original := j.Request.Identity
	original.ChefID = current.ChefID
	original.Fence = current.Fence
	if encode(original) != encode(current) {
		return Job{}, ErrConflict
	}
	if err = s.check(tx, current); err != nil {
		return Job{}, err
	}
	for _, old := range j.Inspections {
		if old.ID == in.ID {
			if encode(old) != encode(in) {
				return Job{}, ErrConflict
			}
			return j, tx.Commit()
		}
	}
	if j.State != "Uncertain" {
		return Job{}, fmt.Errorf("inspection requires Uncertain, got %s", j.State)
	}
	if alive(j.Owner) || alive(j.Birth) {
		return Job{}, errors.New("inspection requires stopped supervisor and child; live process ownership is held")
	}
	j.Inspections = append(j.Inspections, in)
	if err = finish(tx, &j, in.Result); err != nil {
		return Job{}, err
	}
	if err = tx.Commit(); err != nil {
		return Job{}, err
	}
	return j, s.hit("result-committed")
}
