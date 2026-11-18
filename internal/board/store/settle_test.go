package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/hev/kit/internal/board/facets"
	"github.com/hev/kit/internal/board/layer"
)

// stateLayer is a Layer namespace that keeps its rows, answers id/thread_id
// Eq queries and applies upserts and patches, with two failures to switch
// on: reads failing once anything has been written (a readback after a
// successful write), and writes that land and still report an error.
type stateLayer struct {
	mu              sync.Mutex
	rows            map[string]map[string]any
	writes          int
	failReadsAfter  bool // queries fail once a write has landed
	failWritesAfter bool // writes land, then report HTTP 500
	failPatchOnly   bool // as failWritesAfter, for patches only
	dropPatches     bool // patches are not applied
	// failReadsAfterPatch fails queries once a patch has landed;
	// rejectWritesAfter (when set) rejects every write past that many,
	// unapplied: the backend going down partway through a facet write.
	failReadsAfterPatch bool
	rejectWritesAfter   int
	patches             int
}

// recover brings the namespace back: every failure is switched off.
func (f *stateLayer) recover() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failReadsAfter, f.failWritesAfter, f.failPatchOnly, f.dropPatches = false, false, false, false
	f.failReadsAfterPatch, f.rejectWritesAfter = false, 0
}

func (f *stateLayer) store(t *testing.T) *Store {
	t.Helper()
	if f.rows == nil {
		f.rows = map[string]map[string]any{}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/query") {
			if (f.failReadsAfter && f.writes > 0) || (f.failReadsAfterPatch && f.patches > 0) {
				http.Error(w, "invented read failure", http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"rows": f.match(body)})
			return
		}
		_, patching := body["patch_rows"]
		if f.rejectWritesAfter > 0 && f.writes >= f.rejectWritesAfter {
			http.Error(w, "invented outage", http.StatusServiceUnavailable)
			return
		}
		if patching && !f.dropPatches {
			f.patches++
		}
		for _, key := range []string{"upsert_rows", "patch_rows"} {
			rows, _ := body[key].([]any)
			if key == "patch_rows" && f.dropPatches {
				continue
			}
			for _, x := range rows {
				row := x.(map[string]any)
				id := fmt.Sprint(row["id"])
				if key == "upsert_rows" || f.rows[id] == nil {
					if key == "patch_rows" {
						continue
					}
					f.rows[id] = map[string]any{}
				}
				for k, v := range row {
					f.rows[id][k] = v
				}
			}
		}
		f.writes++
		if f.failWritesAfter || (patching && f.failPatchOnly) {
			http.Error(w, "invented write timeout", http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{"status":"OK"}`))
	}))
	t.Cleanup(srv.Close)
	return New(layer.New(srv.URL, "invented-key", "invented-board", ""))
}

// match answers a query whose filter is [attr, "Eq", v], ranked by one
// attribute, up to top_k.
func (f *stateLayer) match(body map[string]any) []map[string]any {
	out := []map[string]any{}
	flt, _ := body["filters"].([]any)
	for _, row := range f.rows {
		if len(flt) == 3 && flt[1] == "Eq" && fmt.Sprint(row[flt[0].(string)]) != fmt.Sprint(flt[2]) {
			continue
		}
		out = append(out, row)
	}
	rank, _ := body["rank_by"].([]any)
	if len(rank) == 2 {
		attr, desc := rank[0].(string), rank[1] == "desc"
		sort.Slice(out, func(i, j int) bool {
			a, b := fmt.Sprint(out[i][attr]), fmt.Sprint(out[j][attr])
			if desc {
				return a > b
			}
			return a < b
		})
	}
	if k, ok := body["top_k"].(float64); ok && int(k) < len(out) {
		out = out[:int(k)]
	}
	return out
}

func seedThread(f *stateLayer) {
	f.rows = map[string]map[string]any{"t:7": {
		"id": "t:7", "kind": "thread", "thread_id": 7, "n": 1, "board": "tooling", "title": "x", "author": "a",
		"created": "2026-01-01T00:00:00.000000Z", "updated": "2026-01-01T00:00:00.000000Z",
		"facet_area": []any{"ci"}, "facets_by": "tag-map",
	}}
}

// A Layer create whose upsert succeeds and whose readback fails still
// stands, with its facets: the audit log records it and the call fails.
// (Review of PR #267 at afc3acc, P2.)
func TestLayerCreateReadbackFailureIsAudited(t *testing.T) {
	f := &stateLayer{failReadsAfter: true}
	s := f.store(t)
	audit := filepath.Join(t.TempDir(), AuditFile)
	s.SetAudit(audit)
	_, err := s.CreateThread(context.Background(), NewThread{Board: "tooling", Title: "invented", Body: "b",
		Poster: Poster{Author: "tester"}, Facets: facets.Set{Area: []string{"ci"}}, FacetsBy: "author:tester"})
	var ce *CreateError
	if !errors.As(err, &ce) || !ce.Written || ce.Thread != 1 {
		t.Fatalf("create: %v", err)
	}
	if f.rows["t:1"]["facet_area"] == nil {
		t.Fatalf("row not written: %v", f.rows)
	}
	recs := readAudit(t, audit)
	if len(recs) != 1 || recs[0].Thread != 1 || recs[0].Before != nil || recs[0].After.String() != "area:ci" ||
		recs[0].AfterBy != "author:tester" || recs[0].Actor != "tester" {
		t.Fatalf("audit: %+v", recs)
	}
}

// When that record cannot be written either, the call says the row could
// not be put back rather than claiming a clean refusal.
func TestLayerCreateReadbackAndLogFailure(t *testing.T) {
	f := &stateLayer{failReadsAfter: true}
	s := f.store(t)
	s.SetAudit(filepath.Join(t.TempDir(), AuditFile))
	failWrites(t)
	_, err := s.CreateThread(context.Background(), NewThread{Board: "tooling", Title: "invented", Body: "b",
		Poster: Poster{Author: "tester"}, Facets: facets.Set{Area: []string{"ci"}}})
	if !errors.Is(err, ErrAudit) || !strings.Contains(err.Error(), "not refused cleanly") {
		t.Fatalf("create: %v", err)
	}
}

// Writes that land and still report an error are settled against the row:
// the change is in the log, and the call fails.
func TestLayerWriteErrorAfterLandingIsAudited(t *testing.T) {
	ctx := context.Background()
	t.Run("set facets", func(t *testing.T) {
		f := &stateLayer{}
		seedThread(f)
		s := f.store(t)
		audit := filepath.Join(t.TempDir(), AuditFile)
		s.SetAudit(audit)
		f.failWritesAfter = true
		err := s.SetFacetsAs(ctx, 7, facets.Set{Repo: []string{"alpha"}}, "inventory-x", "backfill:inventory-x")
		if err == nil || !strings.Contains(err.Error(), "in the audit log") {
			t.Fatalf("set facets: %v", err)
		}
		recs := readAudit(t, audit)
		if len(recs) != 1 || recs[0].Before.String() != "area:ci" || recs[0].After.String() != "repo:alpha" || recs[0].Actor != "backfill:inventory-x" {
			t.Fatalf("audit: %+v", recs)
		}
	})
	t.Run("reply", func(t *testing.T) {
		f := &stateLayer{}
		seedThread(f)
		s := f.store(t)
		audit := filepath.Join(t.TempDir(), AuditFile)
		s.SetAudit(audit)
		f.failWritesAfter = true
		_, err := s.Reply(ctx, NewPost{ThreadID: 7, Body: "r", Poster: Poster{Author: "tester"}, Facets: facets.Set{Tool: []string{"git"}}})
		if err == nil {
			t.Fatal("reply: no error")
		}
		// The post's own upsert reports an error before the facet patch:
		// the thread's facets did not change, so nothing is logged.
		if b, _ := os.ReadFile(audit); len(b) != 0 {
			t.Fatalf("audit: %s", b)
		}
		// Now let the post land and only the facet patch report an error.
		f.mu.Lock()
		f.failWritesAfter, f.failPatchOnly = false, true
		f.rows = map[string]map[string]any{"t:7": f.rows["t:7"]}
		f.mu.Unlock()
		_, err = s.Reply(ctx, NewPost{ThreadID: 7, Body: "r", Poster: Poster{Author: "tester"}, Facets: facets.Set{Tool: []string{"git"}}})
		if err == nil || !strings.Contains(err.Error(), "in the audit log") {
			t.Fatalf("reply: %v", err)
		}
		recs := readAudit(t, audit)
		if len(recs) != 1 || recs[0].After.String() != "tool:git area:ci" || recs[0].AfterBy != "author:tester" {
			t.Fatalf("audit: %+v", recs)
		}
	})
	t.Run("create", func(t *testing.T) {
		f := &stateLayer{failWritesAfter: true}
		s := f.store(t)
		audit := filepath.Join(t.TempDir(), AuditFile)
		s.SetAudit(audit)
		_, err := s.CreateThread(ctx, NewThread{Board: "tooling", Title: "invented", Body: "b",
			Poster: Poster{Author: "tester"}, Facets: facets.Set{Area: []string{"ci"}}})
		if err == nil || !strings.Contains(err.Error(), "in the audit log") {
			t.Fatalf("create: %v", err)
		}
		if recs := readAudit(t, audit); len(recs) != 1 || recs[0].Thread != 1 || recs[0].After.String() != "area:ci" {
			t.Fatalf("audit: %+v", recs)
		}
	})
}

// A write error that left nothing changed is reported as it was, with no
// record.
func TestLayerWriteErrorThatChangedNothing(t *testing.T) {
	f := &stateLayer{}
	seedThread(f)
	s := f.store(t)
	audit := filepath.Join(t.TempDir(), AuditFile)
	s.SetAudit(audit)
	// The patch reports an error and did not land: the settle reads the
	// facets as they were.
	f.mu.Lock()
	f.dropPatches, f.failWritesAfter = true, true
	f.mu.Unlock()
	if err := s.SetFacets(context.Background(), 7, facets.Set{Repo: []string{"alpha"}}, "inventory-x"); err == nil || strings.Contains(err.Error(), "audit log") {
		t.Fatalf("set facets: %v", err)
	}
	if b, _ := os.ReadFile(audit); len(b) != 0 {
		t.Fatalf("audit: %s", b)
	}
}

// files: a thread file whose rename landed but whose directory sync failed.
func failAfterRename(t *testing.T) {
	t.Helper()
	old := writeThreadFile
	writeThreadFile = func(path string, data []byte, mode os.FileMode) error {
		if err := old(path, data, mode); err != nil {
			return err
		}
		return errors.New("invented directory sync failure")
	}
	t.Cleanup(func() { writeThreadFile = old })
}

func TestFilesWriteErrorAfterRenameIsAudited(t *testing.T) {
	s, dir := files(t)
	ctx := context.Background()
	who := Poster{Author: "agent-a"}
	th, err := s.CreateThread(ctx, NewThread{Board: "tooling", Title: "invented", Body: "b", Poster: who,
		Facets: facets.Set{Area: []string{"ci"}}, FacetsBy: facets.ByTagMap})
	if err != nil {
		t.Fatal(err)
	}
	failAfterRename(t)

	if err := s.SetFacetsAs(ctx, th.ID, facets.Set{Repo: []string{"alpha"}}, "inventory-x", "backfill:inventory-x"); err == nil {
		t.Fatal("set facets: no error")
	}
	if _, err := s.Reply(ctx, NewPost{ThreadID: th.ID, Body: "r", Poster: who, Facets: facets.Set{Tool: []string{"git"}}}); err == nil {
		t.Fatal("reply: no error")
	}
	var ce *CreateError
	if _, err := s.CreateThread(ctx, NewThread{Board: "tooling", Title: "second", Body: "b", Poster: who,
		Facets: facets.Set{Host: []string{"box"}}, FacetsBy: facets.ByTagMap}); !errors.As(err, &ce) || ce.Thread != 2 {
		t.Fatalf("create: %v", err)
	}
	recs := readAudit(t, filepath.Join(dir, AuditFile))
	want := []string{"1 area:ci", "1 repo:alpha", "1 repo:alpha tool:git", "2 host:box"}
	var got []string
	for _, r := range recs {
		got = append(got, fmt.Sprintf("%d %s", r.Thread, r.After))
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("audit: %v", got)
	}
	// Every record's after is what the rows hold now.
	for id, want := range map[int64]string{1: "repo:alpha tool:git", 2: "host:box"} {
		if got, _ := s.Thread(ctx, id); got.Facets.String() != want {
			t.Fatalf("thread %d: %v", id, got.Facets)
		}
	}
}
