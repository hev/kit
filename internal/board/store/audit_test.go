package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/board/facets"
)

// unloggable points s's audit log below a regular file, so it cannot open.
func unloggable(t *testing.T, s *Store) {
	t.Helper()
	plain := filepath.Join(t.TempDir(), "regular-file")
	if err := os.WriteFile(plain, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.SetAudit(filepath.Join(plain, AuditFile))
}

// failWrites makes every audit append fail after the log opened (a full
// disk), until the test ends.
func failWrites(t *testing.T) {
	t.Helper()
	old := auditWrite
	auditWrite = func(*os.File, []byte) error { return errors.New("no space left on device") }
	t.Cleanup(func() { auditWrite = old })
}

// A facet write whose audit record cannot be appended is refused: the row
// and the log agree (board memory design, section 3). The log is
// opened before anything changes, so an unopenable log changes nothing.
func TestUnloggableFacetWritesAreRefused(t *testing.T) {
	s, dir := files(t)
	ctx := context.Background()
	who := Poster{Author: "agent-a"}
	th, err := s.CreateThread(ctx, NewThread{Board: "tooling", Title: "invented", Body: "b", Poster: who,
		Facets: facets.Set{Area: []string{"ci"}}, FacetsBy: facets.ByTagMap})
	if err != nil {
		t.Fatal(err)
	}
	unloggable(t, s)

	if err := s.SetFacetsAs(ctx, th.ID, facets.Set{Repo: []string{"alpha"}}, "inventory-x", "backfill:inventory-x"); !errors.Is(err, ErrAudit) {
		t.Fatalf("set facets: %v", err)
	}
	if got, _ := s.Thread(ctx, th.ID); got.Facets.String() != "area:ci" || got.FacetsBy != facets.ByTagMap {
		t.Fatalf("refused write persisted: %v %q", got.Facets, got.FacetsBy)
	}

	if _, err := s.Reply(ctx, NewPost{ThreadID: th.ID, Body: "r", Poster: who, Facets: facets.Set{Tool: []string{"git"}}}); !errors.Is(err, ErrAudit) {
		t.Fatalf("reply: %v", err)
	}
	if got, _ := s.Thread(ctx, th.ID); got.Replies != 0 || got.Facets.String() != "area:ci" {
		t.Fatalf("refused reply persisted: %d replies, %v", got.Replies, got.Facets)
	}

	if _, err := s.CreateThread(ctx, NewThread{Board: "tooling", Title: "second", Body: "b", Poster: who,
		Facets: facets.Set{Area: []string{"ci"}}, FacetsBy: facets.ByTagMap}); !errors.Is(err, ErrAudit) {
		t.Fatalf("create: %v", err)
	}
	if list, _ := s.Threads(ctx, ListOpts{}); len(list) != 1 {
		t.Fatalf("refused thread posted: %d threads", len(list))
	}

	// Writes without facets do not touch the log and still work.
	if _, err := s.Reply(ctx, NewPost{ThreadID: th.ID, Body: "plain", Poster: who}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateThread(ctx, NewThread{Board: "tooling", Title: "plain", Body: "b", Poster: who}); err != nil {
		t.Fatal(err)
	}
	if recs := readAudit(t, filepath.Join(dir, AuditFile)); len(recs) != 1 {
		t.Fatalf("audit lines: %+v", recs)
	}
}

// When the record fails after the row changed, the row is put back and the
// call fails: the log never misses a facet the rows carry.
func TestAuditWriteFailurePutsTheRowBack(t *testing.T) {
	s, dir := files(t)
	ctx := context.Background()
	who := Poster{Author: "agent-a"}
	th, err := s.CreateThread(ctx, NewThread{Board: "tooling", Title: "invented", Body: "b", Poster: who,
		Facets: facets.Set{Area: []string{"ci"}}, FacetsBy: facets.ByTagMap})
	if err != nil {
		t.Fatal(err)
	}
	failWrites(t)

	err = s.SetFacetsAs(ctx, th.ID, facets.Set{Repo: []string{"alpha"}}, "inventory-x", "backfill:inventory-x")
	if !errors.Is(err, ErrAudit) || !strings.Contains(err.Error(), "put back") {
		t.Fatalf("set facets: %v", err)
	}
	if got, _ := s.Thread(ctx, th.ID); got.Facets.String() != "area:ci" || got.FacetsBy != facets.ByTagMap {
		t.Fatalf("row not put back: %v %q", got.Facets, got.FacetsBy)
	}

	if _, err := s.Reply(ctx, NewPost{ThreadID: th.ID, Body: "r", Poster: who, Facets: facets.Set{Tool: []string{"git"}}}); !errors.Is(err, ErrAudit) {
		t.Fatalf("reply: %v", err)
	}
	if got, _ := s.Thread(ctx, th.ID); got.Facets.String() != "area:ci" || got.FacetsBy != facets.ByTagMap {
		t.Fatalf("reply's facets not put back: %v %q", got.Facets, got.FacetsBy)
	}

	nt, err := s.CreateThread(ctx, NewThread{Board: "tooling", Title: "second", Body: "b", Poster: who,
		Facets: facets.Set{Area: []string{"ci"}}, FacetsBy: facets.ByTagMap})
	if !errors.Is(err, ErrAudit) || nt == nil {
		t.Fatalf("create: %v %v", nt, err)
	}
	if got, _ := s.Thread(ctx, nt.ID); got.Facets != nil || got.FacetsBy != "" {
		t.Fatalf("new thread kept unlogged facets: %v %q", got.Facets, got.FacetsBy)
	}
	if recs := readAudit(t, filepath.Join(dir, AuditFile)); len(recs) != 1 {
		t.Fatalf("audit lines: %+v", recs)
	}
}

// On Layer too, an unloggable facet write sends nothing to the namespace.
func TestLayerUnloggableFacetWriteSendsNothing(t *testing.T) {
	f := &layerFake{rows: []map[string]any{{"id": "t:7", "kind": "thread", "thread_id": 7, "board": "tooling", "title": "x", "author": "a", "created": "2099-01-01T00:00:00Z", "updated": "2099-01-01T00:00:00Z"}}}
	s := f.store(t)
	unloggable(t, s)
	if err := s.SetFacets(context.Background(), 7, facets.Set{Area: []string{"ci"}}, "inventory-x"); !errors.Is(err, ErrAudit) {
		t.Fatalf("set facets: %v", err)
	}
	for _, b := range f.bodies {
		if p, _ := b["_path"].(string); !strings.HasSuffix(p, "/query") {
			t.Fatalf("write sent: %v", b)
		}
	}
}
