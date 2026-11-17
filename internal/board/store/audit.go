package store

import (
	"context"
	"encoding/json"
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
const AuditFile = "facets-audit.jsonl"

// AuditRecord is one facet write: a line in the append-only audit log. The
// memory namespace (stage 2 of the board memory design) imports these as
// facet events; until then the file is the audit trail.
type AuditRecord struct {
	Time     string      `json:"time"`
	Thread   int64       `json:"thread"`
	Before   *facets.Set `json:"before"`
	BeforeBy string      `json:"before_by"`
	After    *facets.Set `json:"after"`
	AfterBy  string      `json:"after_by"`
	Actor    string      `json:"actor"`
}

// AuditPath is where Open's store appends facet audit records:
// board.facets_audit, $HEV_BOARD_FACETS_AUDIT, or facets-audit.jsonl in the
// board directory.
func AuditPath() string {
	if settings.Current.FacetsAudit != "" {
		return settings.Current.FacetsAudit
	}
	return filepath.Join(settings.Dir(), AuditFile)
}

// Audit is the file this store appends facet audit records to; "" keeps none.
func (s *Store) Audit() string { return s.audit }

// SetAudit sets the audit file; "" turns the audit off.
func (s *Store) SetAudit(path string) { s.audit = path }

var auditMu sync.Mutex

func (s *Store) appendAudit(rec AuditRecord) error {
	if s.audit == "" {
		return nil
	}
	rec.Time = time.Now().UTC().Format(time.RFC3339Nano)
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	auditMu.Lock()
	defer auditMu.Unlock()
	if err := platformops.MkdirAll(filepath.Dir(s.audit), 0o700); err != nil {
		return fmt.Errorf("facet audit: %w", err)
	}
	f, err := os.OpenFile(s.audit, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("facet audit: %w", err)
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return fmt.Errorf("facet audit: %w", err)
	}
	return f.Close()
}

// SetFacets replaces a thread's facets and records the change in the audit
// log, with by as the actor.
func (s *Store) SetFacets(ctx context.Context, id int64, set facets.Set, by string) error {
	return s.SetFacetsAs(ctx, id, set, by, by)
}

// SetFacetsAs is SetFacets with the actor named (a person's handle on a
// thread-page edit, the source on a backfill).
func (s *Store) SetFacetsAs(ctx context.Context, id int64, set facets.Set, by, actor string) error {
	before, err := s.Backend.Thread(ctx, id)
	if err != nil {
		return err
	}
	if err := s.Backend.SetFacets(ctx, id, set, by); err != nil {
		return err
	}
	return s.appendAudit(AuditRecord{Thread: id, Before: before.Facets, BeforeBy: before.FacetsBy, After: set.Ptr(), AfterBy: by, Actor: actor})
}

// CreateThread records a new thread's facets, when it has any.
func (s *Store) CreateThread(ctx context.Context, in NewThread) (*Thread, error) {
	t, err := s.Backend.CreateThread(ctx, in)
	if err != nil || t.Facets == nil {
		return t, err
	}
	return t, s.appendAudit(AuditRecord{Thread: t.ID, After: t.Facets, AfterBy: t.FacetsBy, Actor: in.Author})
}

// Reply records the facets a reply adds to its thread.
func (s *Store) Reply(ctx context.Context, in NewPost) (*Post, error) {
	if in.Facets.Normal().Empty() {
		return s.Backend.Reply(ctx, in)
	}
	before, err := s.Backend.Thread(ctx, in.ThreadID)
	if err != nil {
		return nil, err
	}
	p, err := s.Backend.Reply(ctx, in)
	if err != nil {
		return nil, err
	}
	after, err := s.Backend.Thread(ctx, in.ThreadID)
	if err != nil {
		return p, err
	}
	return p, s.appendAudit(AuditRecord{Thread: in.ThreadID, Before: before.Facets, BeforeBy: before.FacetsBy, After: after.Facets, AfterBy: after.FacetsBy, Actor: in.Author})
}
