package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/board/facets"
)

// failWritesUntil is failWrites with a way to bring the log back mid-test.
func failWritesUntil(t *testing.T) (restore func()) {
	t.Helper()
	old := auditWrite
	auditWrite = func(*os.File, []byte) error { return errors.New("no space left on device") }
	restore = func() { auditWrite = old }
	t.Cleanup(restore)
	return restore
}

func readPending(t *testing.T, audit string) *AuditRecord {
	t.Helper()
	b, err := os.ReadFile(PendingPath(audit))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var p AuditRecord
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	return &p
}

func logLen(t *testing.T, audit string) int {
	t.Helper()
	b, err := os.ReadFile(audit)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return len(strings.TrimSpace(string(b)))
}

// combined checks the state a facet write leaves when the backend fails
// after the write landed, the read-back and put-back fail, and the audit
// append fails: the call reports ErrAudit naming the pending file, the log
// is empty, and the pending file holds the write. Then, with the backend
// still down, a further facet write is refused and changes nothing. Then,
// with everything back, the next facet write settles it: the log gains one
// recovered record whose after is what the row holds, and the pending file
// is gone. A second settle adds nothing.
func combined(t *testing.T, s *Store, audit string, id int64, err error, still func() error, back func(), want string) {
	t.Helper()
	ctx := context.Background()
	if !errors.Is(err, ErrAudit) || !strings.Contains(err.Error(), "next facet write settles it") {
		t.Fatalf("write: %v", err)
	}
	if n := logLen(t, audit); n != 0 {
		t.Fatalf("log not empty after the failed write")
	}
	p := readPending(t, audit)
	if p == nil || p.Thread != id || p.Intent == "" {
		t.Fatalf("pending: %+v", p)
	}
	if still != nil {
		if err := still(); !errors.Is(err, ErrAudit) || !strings.Contains(err.Error(), "refused until it settles") {
			t.Fatalf("write while unsettled: %v", err)
		}
		if readPending(t, audit) == nil || logLen(t, audit) != 0 {
			t.Fatal("an unsettled write was dropped or logged while the backend was down")
		}
	}
	back()
	if err := s.SettlePending(ctx); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if readPending(t, audit) != nil {
		t.Fatal("pending file outlived the settle")
	}
	recs := readAudit(t, audit)
	if len(recs) != 1 || recs[0].Thread != id || !recs[0].Recovered || recs[0].Intent != p.Intent || recs[0].After.String() != want {
		t.Fatalf("audit: %+v", recs)
	}
	got, err := s.Thread(ctx, id)
	if err != nil || got.Facets.String() != recs[0].After.String() || got.FacetsBy != recs[0].AfterBy {
		t.Fatalf("row %v %q disagrees with the log %v %q (%v)", got.Facets, got.FacetsBy, recs[0].After, recs[0].AfterBy, err)
	}
	if err := s.SettlePending(ctx); err != nil || len(readAudit(t, audit)) != 1 {
		t.Fatalf("second settle: %v, %d records", err, len(readAudit(t, audit)))
	}
}

// Review of PR #267 at d071d2e, P2: a facet write whose read-back, audit
// append and put-back all fail leaves a durable pending record, which the
// next facet write settles once the backend and the log are back. Every
// mutation path, on Layer.
func TestLayerCombinedFailureIsRecovered(t *testing.T) {
	ctx := context.Background()
	t.Run("create", func(t *testing.T) {
		// The upsert lands, its readback fails, the log fails, and the
		// put-back (which reads the thread first) fails.
		f := &stateLayer{failReadsAfter: true, rejectWritesAfter: 1}
		s := f.store(t)
		audit := filepath.Join(t.TempDir(), AuditFile)
		s.SetAudit(audit)
		logBack := failWritesUntil(t)
		_, err := s.CreateThread(ctx, NewThread{Board: "tooling", Title: "invented", Body: "b",
			Poster: Poster{Author: "tester"}, Facets: facets.Set{Area: []string{"ci"}}, FacetsBy: "author:tester"})
		if f.rows["t:1"]["facet_area"] == nil {
			t.Fatalf("row not written: %v", f.rows)
		}
		still := func() error { return s.SetFacets(ctx, 1, facets.Set{Repo: []string{"alpha"}}, "inventory-x") }
		combined(t, s, audit, 1, err, still, func() { f.recover(); logBack() }, "area:ci")
	})
	t.Run("set facets", func(t *testing.T) {
		// The patch lands and reports an error, then reads and writes fail.
		f := &stateLayer{failPatchOnly: true, failReadsAfterPatch: true, rejectWritesAfter: 1}
		seedThread(f)
		s := f.store(t)
		audit := filepath.Join(t.TempDir(), AuditFile)
		s.SetAudit(audit)
		logBack := failWritesUntil(t)
		err := s.SetFacetsAs(ctx, 7, facets.Set{Repo: []string{"alpha"}}, "inventory-x", "backfill:inventory-x")
		still := func() error {
			_, err := s.CreateThread(ctx, NewThread{Board: "tooling", Title: "other", Body: "b",
				Poster: Poster{Author: "tester"}, Facets: facets.Set{Host: []string{"box"}}})
			return err
		}
		combined(t, s, audit, 7, err, still, func() { f.recover(); logBack() }, "repo:alpha")
		if _, ok := f.rows["t:8"]; ok {
			t.Fatal("a create went through while a facet write was unsettled")
		}
	})
	t.Run("reply", func(t *testing.T) {
		// The post lands, the facet patch lands and reports an error, then
		// reads and writes fail.
		f := &stateLayer{failPatchOnly: true, failReadsAfterPatch: true, rejectWritesAfter: 2}
		seedThread(f)
		s := f.store(t)
		audit := filepath.Join(t.TempDir(), AuditFile)
		s.SetAudit(audit)
		logBack := failWritesUntil(t)
		_, err := s.Reply(ctx, NewPost{ThreadID: 7, Body: "r", Poster: Poster{Author: "tester"}, Facets: facets.Set{Tool: []string{"git"}}})
		combined(t, s, audit, 7, err, nil, func() { f.recover(); logBack() }, "tool:git area:ci")
	})
}

// The same on files: the thread file's rename lands and its directory sync
// fails, the log fails, and every later thread write fails before it lands.
func TestFilesCombinedFailureIsRecovered(t *testing.T) {
	ctx := context.Background()
	who := Poster{Author: "agent-a"}
	downAfterFirst := func(t *testing.T) (back func()) {
		old := writeThreadFile
		n := 0
		writeThreadFile = func(path string, data []byte, mode os.FileMode) error {
			n++
			if n > 1 {
				return errors.New("invented outage")
			}
			if err := old(path, data, mode); err != nil {
				return err
			}
			return errors.New("invented directory sync failure")
		}
		back = func() { writeThreadFile = old }
		t.Cleanup(back)
		return back
	}
	seed := func(t *testing.T) (*Store, string) {
		s, dir := files(t)
		if _, err := s.CreateThread(ctx, NewThread{Board: "tooling", Title: "invented", Body: "b", Poster: who,
			Facets: facets.Set{Area: []string{"ci"}}, FacetsBy: facets.ByTagMap}); err != nil {
			t.Fatal(err)
		}
		audit := filepath.Join(dir, AuditFile)
		if err := os.Remove(audit); err != nil {
			t.Fatal(err)
		}
		return s, audit
	}
	t.Run("set facets", func(t *testing.T) {
		s, audit := seed(t)
		filesBack, logBack := downAfterFirst(t), failWritesUntil(t)
		err := s.SetFacetsAs(ctx, 1, facets.Set{Repo: []string{"alpha"}}, "inventory-x", "backfill:inventory-x")
		combined(t, s, audit, 1, err, nil, func() { filesBack(); logBack() }, "repo:alpha")
	})
	t.Run("reply", func(t *testing.T) {
		s, audit := seed(t)
		filesBack, logBack := downAfterFirst(t), failWritesUntil(t)
		_, err := s.Reply(ctx, NewPost{ThreadID: 1, Body: "r", Poster: who, Facets: facets.Set{Tool: []string{"git"}}})
		combined(t, s, audit, 1, err, nil, func() { filesBack(); logBack() }, "tool:git area:ci")
	})
	t.Run("create", func(t *testing.T) {
		s, audit := seed(t)
		filesBack, logBack := downAfterFirst(t), failWritesUntil(t)
		_, err := s.CreateThread(ctx, NewThread{Board: "tooling", Title: "second", Body: "b", Poster: who,
			Facets: facets.Set{Host: []string{"box"}}, FacetsBy: facets.ByTagMap})
		combined(t, s, audit, 2, err, nil, func() { filesBack(); logBack() }, "host:box")
	})
}

// A write that cannot be announced is refused with nothing changed.
func TestUnannouncedFacetWriteIsRefused(t *testing.T) {
	s, dir := files(t)
	ctx := context.Background()
	who := Poster{Author: "agent-a"}
	old := pendingWrite
	pendingWrite = func(string, []byte, os.FileMode) error { return errors.New("invented full disk") }
	t.Cleanup(func() { pendingWrite = old })
	if _, err := s.CreateThread(ctx, NewThread{Board: "tooling", Title: "invented", Body: "b", Poster: who,
		Facets: facets.Set{Area: []string{"ci"}}}); !errors.Is(err, ErrAudit) {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.Thread(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("refused create persisted: %v", err)
	}
	pendingWrite = old
	if _, err := s.CreateThread(ctx, NewThread{Board: "tooling", Title: "invented", Body: "b", Poster: who}); err != nil {
		t.Fatal(err)
	}
	pendingWrite = func(string, []byte, os.FileMode) error { return errors.New("invented full disk") }
	if err := s.SetFacets(ctx, 1, facets.Set{Repo: []string{"alpha"}}, "inventory-x"); !errors.Is(err, ErrAudit) {
		t.Fatalf("set facets: %v", err)
	}
	if _, err := s.Reply(ctx, NewPost{ThreadID: 1, Body: "r", Poster: who, Facets: facets.Set{Tool: []string{"git"}}}); !errors.Is(err, ErrAudit) {
		t.Fatalf("reply: %v", err)
	}
	if got, _ := s.Thread(ctx, 1); got.Facets != nil || got.Replies != 0 {
		t.Fatalf("refused writes persisted: %v, %d replies", got.Facets, got.Replies)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, AuditFile)); len(b) != 0 {
		t.Fatalf("audit: %s", b)
	}
}

// Writes that settle in the call leave no pending file, and a pending file
// whose write is already in the log (the remove failed) is dropped without
// a second record.
func TestPendingWriteIsDroppedOnceSettled(t *testing.T) {
	s, dir := files(t)
	ctx := context.Background()
	audit := filepath.Join(dir, AuditFile)
	if _, err := s.CreateThread(ctx, NewThread{Board: "tooling", Title: "invented", Body: "b", Poster: Poster{Author: "agent-a"},
		Facets: facets.Set{Area: []string{"ci"}}}); err != nil {
		t.Fatal(err)
	}
	if readPending(t, audit) != nil {
		t.Fatal("pending file after a clean write")
	}
	recs := readAudit(t, audit)
	if len(recs) != 1 || recs[0].Intent == "" {
		t.Fatalf("audit: %+v", recs)
	}
	stale, _ := json.Marshal(AuditRecord{Thread: 1, After: recs[0].After, Actor: "agent-a", Intent: recs[0].Intent})
	if err := os.WriteFile(PendingPath(audit), stale, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFacets(ctx, 1, facets.Set{Repo: []string{"alpha"}}, "inventory-x"); err != nil {
		t.Fatal(err)
	}
	if recs := readAudit(t, audit); len(recs) != 2 || recs[1].Recovered || recs[1].After.String() != "repo:alpha" {
		t.Fatalf("audit: %+v", recs)
	}
	if readPending(t, audit) != nil {
		t.Fatal("pending file outlived the settle")
	}
}
