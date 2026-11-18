package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hev/kit/internal/board/facets"
	"github.com/hev/kit/internal/board/settings"
	"github.com/hev/kit/internal/platformops"
)

// AuditFile is the default name of the facet audit log, beside the board's
// data (board.dir).
const AuditFile = "facet-audit.jsonl"

// ErrAudit marks a facet write refused because its audit record could not be
// appended. The rows are left as they were before the write.
var ErrAudit = errors.New("facet audit log")

// AuditRecord is one facet write: a line in the append-only audit log. The
// memory namespace (stage 2 of the board memory design) imports these as
// facet events; until then the file is the audit trail.
type AuditRecord struct {
	At       string      `json:"at"`
	Thread   int64       `json:"thread"`
	Before   *facets.Set `json:"before"`
	BeforeBy string      `json:"before_by"`
	After    *facets.Set `json:"after"`
	AfterBy  string      `json:"after_by"`
	Actor    string      `json:"actor"`
}

// AuditPath is where Open's store appends facet audit records:
// board.facet_audit_log, $HEV_BOARD_FACET_AUDIT_LOG, or facet-audit.jsonl in
// the board directory.
func AuditPath() string {
	if settings.Current.FacetAuditLog != "" {
		return settings.Current.FacetAuditLog
	}
	return filepath.Join(settings.Dir(), AuditFile)
}

// Audit is the file this store appends facet audit records to; "" keeps none.
func (s *Store) Audit() string { return s.audit }

// SetAudit sets the audit file; "" turns the audit off.
func (s *Store) SetAudit(path string) { s.audit = path }

// auditMu serializes facet writes inside one process; the lock file beside
// the log serializes them across processes (a backfill and a server).
var auditMu sync.Mutex

// auditWrite appends one line; tests replace it to fail after the log has
// opened, the one failure the open cannot catch first.
var auditWrite = func(f *os.File, line []byte) error {
	_, err := f.Write(line)
	return err
}

// auditTx is one facet write in progress: the log is open and locked before
// the rows change, so a log that cannot be opened refuses the write with
// nothing changed.
type auditTx struct {
	f      *os.File
	unlock func()
}

// beginAudit takes the facet-write lock and opens the log for appending. With
// no log configured it still takes the in-process lock and returns a nil
// file. The caller must call end.
func (s *Store) beginAudit() (*auditTx, error) {
	auditMu.Lock()
	tx := &auditTx{unlock: auditMu.Unlock}
	if s.audit == "" {
		return tx, nil
	}
	fail := func(err error) (*auditTx, error) {
		tx.end()
		return nil, fmt.Errorf("%w: %v; the facet write was refused", ErrAudit, err)
	}
	if err := platformops.MkdirAll(filepath.Dir(s.audit), 0o700); err != nil {
		return fail(err)
	}
	release, err := platformops.LockPath(s.audit+".lock", false)
	if err != nil {
		return fail(err)
	}
	tx.unlock = func() { release(); auditMu.Unlock() }
	f, err := os.OpenFile(s.audit, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fail(err)
	}
	tx.f = f
	return tx, nil
}

// log appends rec. The file was opened before the write, so this fails only
// when the write itself does (a full disk, say); the caller then puts the
// rows back.
func (tx *auditTx) log(rec AuditRecord) error {
	if tx.f == nil {
		return nil
	}
	rec.At = time.Now().UTC().Format(time.RFC3339Nano)
	b, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAudit, err)
	}
	if err := auditWrite(tx.f, append(b, '\n')); err != nil {
		return fmt.Errorf("%w: %v", ErrAudit, err)
	}
	return nil
}

func (tx *auditTx) end() {
	if tx.f != nil {
		tx.f.Close()
		tx.f = nil
	}
	tx.unlock()
}

// undo puts a thread's facets back after its audit record failed, so the row
// and the log agree; it reports the log error, and the undo's own if any.
func (s *Store) undo(ctx context.Context, id int64, before *facets.Set, beforeBy string, logErr error) error {
	var prev facets.Set
	if before != nil {
		prev = *before
	}
	if err := s.Backend.SetFacets(ctx, id, prev, beforeBy); err != nil {
		return fmt.Errorf("%w; the facet write was not refused cleanly, putting thread %d back failed: %v", logErr, id, err)
	}
	return fmt.Errorf("%w; the facet write was refused and thread %d put back", logErr, id)
}

// sameFacets reports whether two facet states, with their facets_by, match.
func sameFacets(a *facets.Set, aBy string, b *facets.Set, bBy string) bool {
	var x, y facets.Set
	if a != nil {
		x = *a
	}
	if b != nil {
		y = *b
	}
	return x.Equal(y) && (x.Empty() || aBy == bBy)
}

// settle squares the log with thread id after the backend failed partway
// through a facet write, which may or may not have changed the row (a write
// can land and still report an error: a Layer timeout, a directory sync
// after the rename). It reads the thread back: facets as they were before
// need no record; changed ones are logged, or put back when the log fails.
// When the thread cannot be read, its facets are put back to before. The
// backend's error, cause, is always reported.
func (s *Store) settle(ctx context.Context, tx *auditTx, id int64, before *facets.Set, beforeBy, actor string, cause error) error {
	now, err := s.Backend.Thread(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return cause
	}
	if err == nil {
		if sameFacets(now.Facets, now.FacetsBy, before, beforeBy) {
			return cause
		}
		rec := AuditRecord{Thread: id, Before: before, BeforeBy: beforeBy, After: now.Facets, AfterBy: now.FacetsBy, Actor: actor}
		if lerr := tx.log(rec); lerr != nil {
			return s.undo(ctx, id, before, beforeBy, fmt.Errorf("%w, after %v", lerr, cause))
		}
		return fmt.Errorf("%w; thread %d's facets changed anyway and are in the audit log", cause, id)
	}
	var prev facets.Set
	if before != nil {
		prev = *before
	}
	uerr := s.Backend.SetFacets(ctx, id, prev, beforeBy)
	if uerr == nil {
		return fmt.Errorf("%w; thread %d could not be read back (%v), so its facets were put back", cause, id, err)
	}
	if errors.Is(uerr, ErrNotFound) {
		return cause
	}
	return fmt.Errorf("%w: %v; thread %d could not be read back (%v) or put back (%v), so its facets may differ from the audit log", ErrAudit, cause, id, err, uerr)
}

// SetFacets replaces a thread's facets and records the change in the audit
// log, with by as the actor.
func (s *Store) SetFacets(ctx context.Context, id int64, set facets.Set, by string) error {
	return s.SetFacetsAs(ctx, id, set, by, by)
}

// SetFacetsAs is SetFacets with the actor named (a person's handle on a
// thread-page edit, the source on a backfill). A write whose audit record
// cannot be appended is refused: the row keeps its facets. A backend error
// is settled against the row, so a write that landed anyway is logged.
func (s *Store) SetFacetsAs(ctx context.Context, id int64, set facets.Set, by, actor string) error {
	tx, err := s.beginAudit()
	if err != nil {
		return err
	}
	defer tx.end()
	before, err := s.Backend.Thread(ctx, id)
	if err != nil {
		return err
	}
	if err := s.Backend.SetFacets(ctx, id, set, by); err != nil {
		return s.settle(ctx, tx, id, before.Facets, before.FacetsBy, actor, err)
	}
	if err := tx.log(AuditRecord{Thread: id, Before: before.Facets, BeforeBy: before.FacetsBy, After: set.Ptr(), AfterBy: by, Actor: actor}); err != nil {
		return s.undo(ctx, id, before.Facets, before.FacetsBy, err)
	}
	return nil
}

// CreateThread records a new thread's facets, when it has any. When the log
// cannot be opened the thread is not posted; when the record cannot be
// written after the post, the thread keeps no facets and the error says so.
// A post the backend wrote but could not read back is logged all the same,
// and one whose write failed is settled against what the backend now holds.
func (s *Store) CreateThread(ctx context.Context, in NewThread) (*Thread, error) {
	if in.Facets.Normal().Empty() {
		return s.Backend.CreateThread(ctx, in)
	}
	tx, err := s.beginAudit()
	if err != nil {
		return nil, err
	}
	defer tx.end()
	t, err := s.Backend.CreateThread(ctx, in)
	if err != nil {
		var ce *CreateError
		if !errors.As(err, &ce) {
			return nil, err
		}
		if !ce.Written {
			return nil, s.settle(ctx, tx, ce.Thread, nil, "", in.Author, err)
		}
		if ce.Facets.Empty() {
			return nil, err
		}
		if lerr := tx.log(AuditRecord{Thread: ce.Thread, After: ce.Facets.Ptr(), AfterBy: ce.FacetsBy, Actor: in.Author}); lerr != nil {
			return nil, s.undo(ctx, ce.Thread, nil, "", fmt.Errorf("%w, after %v", lerr, err))
		}
		return nil, err
	}
	if t.Facets == nil {
		return t, nil
	}
	if err := tx.log(AuditRecord{Thread: t.ID, After: t.Facets, AfterBy: t.FacetsBy, Actor: in.Author}); err != nil {
		err = s.undo(ctx, t.ID, nil, "", err)
		t.Facets, t.FacetsBy = nil, ""
		return t, err
	}
	return t, nil
}

// Reply records the facets a reply adds to its thread, with the same rule:
// no log, no reply; a record that fails after the reply leaves the thread's
// facets as they were. A backend error is settled against the row; a reply
// that landed but whose thread cannot be read back is logged with the
// facets the backend wrote (the thread's, with the reply's added).
func (s *Store) Reply(ctx context.Context, in NewPost) (*Post, error) {
	if in.Facets.Normal().Empty() {
		return s.Backend.Reply(ctx, in)
	}
	tx, err := s.beginAudit()
	if err != nil {
		return nil, err
	}
	defer tx.end()
	before, err := s.Backend.Thread(ctx, in.ThreadID)
	if err != nil {
		return nil, err
	}
	p, err := s.Backend.Reply(ctx, in)
	if err != nil {
		return nil, s.settle(ctx, tx, in.ThreadID, before.Facets, before.FacetsBy, in.Author, err)
	}
	rec := AuditRecord{Thread: in.ThreadID, Before: before.Facets, BeforeBy: before.FacetsBy, Actor: in.Author}
	if after, err := s.Backend.Thread(ctx, in.ThreadID); err == nil {
		rec.After, rec.AfterBy = after.Facets, after.FacetsBy
	} else {
		var have facets.Set
		if before.Facets != nil {
			have = *before.Facets
		}
		rec.After, rec.AfterBy = have.Union(in.Facets.Normal()).Ptr(), facets.Author(in.Author)
	}
	if err := tx.log(rec); err != nil {
		return p, s.undo(ctx, in.ThreadID, before.Facets, before.FacetsBy, err)
	}
	return p, nil
}
