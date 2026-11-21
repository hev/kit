package hookapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

const rowLimit = 16384

type stateRow struct {
	kind, id string
	at, due  int64
	done     bool
	value    any
}

func eventRow(e record) stateRow {
	return stateRow{kind: "event", id: e.ID, at: e.Time.UnixMilli(), done: !e.Pending, value: e}
}

func jobRow(j job) stateRow {
	var due int64
	if !j.Blocking && !j.Test && j.Completed == nil {
		due = j.Created.UnixMilli()
		if j.Next != nil {
			due = j.Next.UnixMilli()
		}
	}
	return stateRow{kind: "job", id: j.ID, at: j.Created.UnixMilli(), due: due, done: j.Completed != nil, value: j}
}

func (s *Service) openState(ctx context.Context) error {
	path := s.path + ".sqlite"
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	info, statErr := f.Stat()
	f.Close()
	if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return errors.New("invalid hook database file")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	s.db = db
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err = db.ExecContext(ctx, `
		PRAGMA journal_mode=WAL;
		CREATE TABLE IF NOT EXISTS hook_rows (
			account TEXT, kind TEXT, id TEXT, at INTEGER, due INTEGER, done INTEGER, body BLOB NOT NULL,
			PRIMARY KEY(account, kind, id)
		);
		CREATE INDEX IF NOT EXISTS hook_due_kind ON hook_rows(kind, due, account, id) WHERE due != 0;
		CREATE INDEX IF NOT EXISTS hook_due_account ON hook_rows(kind, account, due, id) WHERE due != 0;
		CREATE INDEX IF NOT EXISTS hook_completed ON hook_rows(account,kind,at,id) WHERE done = 1;
	`)
	if err != nil {
		return err
	}
	var marker []byte
	err = db.QueryRowContext(ctx, "SELECT body FROM hook_rows WHERE account='' AND kind='key' AND id=''").Scan(&marker)
	if errors.Is(err, sql.ErrNoRows) {
		if err := s.loadLegacy(); err != nil {
			return err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for id, a := range s.accounts {
			for key, h := range a.Hooks {
				if err := s.writeRow(ctx, tx, id, stateRow{kind: "hook", id: key, value: h}); err != nil {
					return err
				}
			}
			for _, e := range a.Events {
				if err := s.writeRow(ctx, tx, id, eventRow(e)); err != nil {
					return err
				}
			}
			for _, j := range a.Jobs {
				if err := s.writeRow(ctx, tx, id, jobRow(j)); err != nil {
					return err
				}
			}
			for key, receipt := range a.Creates {
				if err := s.writeRow(ctx, tx, id, stateRow{kind: "create", id: fmt.Sprintf("%x", sha256.Sum256([]byte(key))), value: receipt}); err != nil {
					return err
				}
			}
		}
		if err := s.writeRow(ctx, tx, "", stateRow{kind: "key", value: true}); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if _, err := s.seal.Open(nil, nil, marker, rowAAD("", "key", "")); err != nil {
		return errors.New("cannot authenticate hook state")
	}
	// A successful migration leaves SQLite as the only source of truth.
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	s.accounts = make(map[string]*account)
	return nil
}

func (s *Service) recoverAccount(ctx context.Context, id string, a *account) error {
	now := time.Now().UTC()
	var changed []stateRow
	for key, j := range a.Jobs {
		// Any volatile payload pins its account. If this account had to be
		// loaded from disk, those payloads belonged to a previous process.
		if j.Completed == nil && (j.Content && len(j.Reference) == 0 || j.Blocking || j.Test) {
			j.Status, j.Completed, j.Next, j.Payload = "failed", &now, nil, nil
			j.FailedOpen = j.Blocking
			if j.Blocking {
				j.Decision = "allow"
			}
			a.setJob(key, j)
			changed = append(changed, jobRow(j))
		}
	}
	for key, e := range a.Events {
		if e.Pending {
			e.Pending = false
			e.Failed = e.Blocking
			a.Events[key] = e
			changed = append(changed, eventRow(e))
		}
	}
	return s.write(ctx, id, changed...)
}

func rowAAD(account, kind, id string) []byte {
	raw, _ := json.Marshal([3]string{account, kind, id})
	return raw
}

func (s *Service) loadAccount(ctx context.Context, id string, a *account) error {
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	if err := s.readAccount(ctx, id, a); err != nil {
		return err
	}
	return s.recoverAccount(ctx, id, a)
}

func (s *Service) readAccount(ctx context.Context, id string, a *account) error {
	rows, err := s.db.QueryContext(ctx, "SELECT kind,id,body FROM hook_rows WHERE account=? AND kind NOT IN ('key','publication')", id)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var kind, key string
		var ciphertext []byte
		if err := rows.Scan(&kind, &key, &ciphertext); err != nil {
			return err
		}
		body, err := s.seal.Open(nil, nil, ciphertext, rowAAD(id, kind, key))
		if err != nil {
			return err
		}
		switch kind {
		case "hook":
			var h storedHook
			err = json.Unmarshal(body, &h)
			a.Hooks[key] = h
		case "event":
			var e record
			err = json.Unmarshal(body, &e)
			a.Events[key] = e
		case "job":
			var j job
			err = json.Unmarshal(body, &j)
			a.setJob(key, j)
		case "create":
			var r createReceipt
			err = json.Unmarshal(body, &r)
			a.Creates[key] = r
		default:
			return errors.New("invalid hook state row")
		}
		if err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Service) accountPage(ctx context.Context, after string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, "SELECT DISTINCT account FROM hook_rows WHERE account>? ORDER BY account LIMIT 128", after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Service) writeRow(ctx context.Context, tx *sql.Tx, account string, r stateRow) error {
	if r.value == nil {
		_, err := tx.ExecContext(ctx, "DELETE FROM hook_rows WHERE account=? AND kind=? AND id=?", account, r.kind, r.id)
		return err
	}
	raw, err := json.Marshal(r.value)
	if err != nil {
		return err
	}
	body := s.seal.Seal(nil, nil, raw, rowAAD(account, r.kind, r.id))
	_, err = tx.ExecContext(ctx, "INSERT OR REPLACE INTO hook_rows VALUES(?,?,?,?,?,?,?)", account, r.kind, r.id, r.at, r.due, r.done, body)
	return err
}

func (s *Service) write(ctx context.Context, account string, rows ...stateRow) error {
	if len(rows) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range rows {
		if err := s.writeRow(ctx, tx, account, r); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Service) oldest(ctx context.Context, account, kind string, count int) ([]stateRow, error) {
	if count <= 0 {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	result, err := s.db.QueryContext(ctx, "SELECT id FROM hook_rows WHERE account=? AND kind=? AND done=1 ORDER BY at,id LIMIT ?", account, kind, count)
	if err != nil {
		return nil, err
	}
	defer result.Close()
	var rows []stateRow
	for result.Next() {
		r := stateRow{kind: kind}
		if err := result.Scan(&r.id); err != nil {
			return nil, err
		}
		rows = append(rows, r)
	}
	if err := result.Err(); err != nil {
		return nil, err
	}
	if kind == "event" && len(rows) < count {
		return nil, errors.New("hook queue capacity reached")
	}
	return rows, nil
}
