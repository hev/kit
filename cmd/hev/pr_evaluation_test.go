package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/preval"
	"github.com/hev/kit/internal/workspace"
)

func TestPREvaluationRespectsWorkspaceStorage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_ORDERS_DIR", filepath.Join(home, "orders"))
	t.Setenv("HEV_EVIDENCE_DIR", filepath.Join(home, "personal-evidence"))
	t.Setenv("FACTORY_BOARD_URL", "https://shared.invalid")
	w := workspace.Selection{Name: "fixture-client", Workspace: workspace.Workspace{Login: "own"}}
	board, root, err := prLearningBoard(w)
	if err != nil || board.Info().Location != filepath.Join(w.Root(), "board") ||
		root != filepath.Join(w.Root(), "board") || evidenceWorkspaceRoot(w) != filepath.Join(w.Root(), "evidence") {
		t.Fatalf("isolated workspace selected shared storage: %v", err)
	}
	for _, scope := range []string{"hev", w.Name} {
		dir := filepath.Join(dispatch.Root(), scope)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		for name, doc := range map[string]any{
			"meta.json":   dispatch.Meta{WorkspaceName: scope},
			"result.json": map[string]any{"span_id": "attempt", "end": time.Now(), "original": "keep"},
		} {
			if err := dispatch.WriteJSON(filepath.Join(dir, name), doc); err != nil {
				t.Fatal(err)
			}
		}
		pr := preval.PullRequest{Repository: preval.Repository{Provider: preval.GitHub, Organization: "fixture", Repository: "42"}, ID: 1}
		link := preval.Attribution{PullRequest: pr, Provenance: preval.Provenance{OrderID: scope, AttemptID: "attempt"}}
		e, err := preval.Project(link, preval.Observation{
			Source:     preval.Source{PullRequest: pr, Revision: "fixture-revision", EventID: "fixture-event"},
			ObservedAt: time.Now(), Outcome: preval.Merged,
		}, nil, 0, false)
		if err != nil {
			t.Fatal(err)
		}
		c := localEvaluationDocuments{workspace: w.Name}
		ctx := context.Background()
		if scope == "hev" {
			if err := c.WriteOrder(ctx, e); !errors.Is(err, preval.ErrConflict) {
				t.Fatalf("cross-workspace order update: %v", err)
			}
			if _, err := c.ReadOrder(ctx, link.Provenance); !errors.Is(err, preval.ErrConflict) {
				t.Fatalf("cross-workspace order read: %v", err)
			}
			continue
		}
		if err := c.WriteOrder(ctx, e); err != nil {
			t.Fatal(err)
		}
		if got, err := c.ReadOrder(ctx, link.Provenance); err != nil || got == nil || got.ID != e.ID {
			t.Fatalf("own order inaccessible: %v", err)
		}
	}
}
