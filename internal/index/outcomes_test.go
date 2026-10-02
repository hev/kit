package index

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hev/kit/internal/trace"
	"reflect"
	"strings"
	"testing"
	"time"
)

const outcomeSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const revertedSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

var mergedTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func fixtureOutcome(t *testing.T, now time.Time, mode string) (*OutcomeEnricher, *trace.SessionRow, *int) {
	t.Helper()
	calls := 0
	g := &OutcomeEnricher{Now: func() time.Time { return now }, Run: func(_ context.Context, _ string, _ string, args ...string) ([]byte, error) {
		calls++
		path := args[len(args)-1]
		var v any
		switch {
		case strings.Contains(path, "pulls/"):
			if mode == "unavailable" {
				return nil, errors.New("offline")
			}
			state := "closed"
			merged := true
			if mode == "closed" {
				merged = false
			}
			if mode == "open" {
				state = "open"
				merged = false
			}
			v = map[string]any{"state": state, "merged": merged, "merged_at": mergedTime, "html_url": "https://example/pr/7", "merge_commit_sha": outcomeSHA, "head": map[string]any{"sha": outcomeSHA}, "base": map[string]any{"ref": "main"}}
		case strings.Contains(path, "actions/runs"):
			if mode == "ci-unavailable" {
				return nil, errors.New("forbidden")
			}
			runs := []workflowRun{{ID: 1, WorkflowID: 10, Attempt: 1, Name: "layer-pro", SHA: outcomeSHA, Status: "completed", Conclusion: "success", URL: "https://example/run/1"}, {ID: 2, WorkflowID: 11, Attempt: 1, Name: "CI", SHA: outcomeSHA, Status: "completed", Conclusion: "failure", URL: "https://example/run/2"}, {ID: 3, WorkflowID: 10, Attempt: 2, Name: "layer-pro", SHA: outcomeSHA, Status: "completed", Conclusion: "success", URL: "https://example/run/3"}}
			if mode == "pending" {
				runs[1].Status = "in_progress"
				runs[1].Conclusion = ""
			}
			if mode == "pagination" && strings.Contains(path, "page=1") {
				runs = make([]workflowRun, 100)
				for i := range runs {
					runs[i] = workflowRun{ID: int64(i + 1), WorkflowID: 10, Name: "layer-pro", SHA: outcomeSHA, Status: "completed", Conclusion: "success"}
				}
			}
			if mode == "pagination" && strings.Contains(path, "page=2") {
				runs = []workflowRun{{ID: 200, WorkflowID: 11, Name: "CI", SHA: outcomeSHA, Status: "completed", Conclusion: "failure"}}
			}
			v = map[string]any{"workflow_runs": runs}
		case strings.Contains(path, "commits?"):
			if mode == "history-unavailable" {
				return nil, errors.New("offline")
			}
			commits := []historyCommit{}
			if mode == "revert" || mode == "late-revert" {
				var c historyCommit
				c.SHA = revertedSHA
				c.Commit.Message = "Revert topic\n\nThis reverts commit " + outcomeSHA + "."
				c.Commit.Committer.Date = mergedTime.Add(24 * time.Hour)
				if mode == "late-revert" {
					c.Commit.Committer.Date = mergedTime.Add(15 * 24 * time.Hour)
				}
				commits = append(commits, c)
			}
			v = commits
		default:
			return nil, fmt.Errorf("unexpected API %s", path)
		}
		return json.Marshal(v)
	}}
	return g, &trace.SessionRow{ID: "s", PR: "7", RepoURL: "https://github.com/example/repo", Commits: trace.StringList{outcomeSHA}}, &calls
}
func TestOutcomesStatesAndWindow(t *testing.T) {
	for _, tc := range []struct {
		mode, state, merged, closed, ci, reverted, window string
		days                                              int
	}{
		{"mixed", "merged", "true", "true", "complete", "false", "complete", 15},
		{"closed", "closed", "false", "true", "complete", "unknown", "unknown", 15},
		{"open", "open", "false", "false", "complete", "unknown", "unknown", 15},
		{"pending", "merged", "true", "true", "pending", "unknown", "observing", 3},
		{"unavailable", "unknown", "unknown", "unknown", "unknown", "unknown", "unknown", 15},
		{"ci-unavailable", "merged", "true", "true", "unknown", "false", "complete", 15},
		{"history-unavailable", "merged", "true", "true", "complete", "unknown", "unknown", 15},
		{"mixed", "merged", "true", "true", "complete", "unknown", "observing", 13},
		{"mixed", "merged", "true", "true", "complete", "false", "complete", 14},
		{"revert", "merged", "true", "true", "complete", "true", "observed", 3},
		{"late-revert", "merged", "true", "true", "complete", "false", "complete", 16},
	} {
		t.Run(fmt.Sprintf("%s-%d", tc.mode, tc.days), func(t *testing.T) {
			g, r, _ := fixtureOutcome(t, mergedTime.Add(time.Duration(tc.days)*24*time.Hour), tc.mode)
			g.Enrich(context.Background(), r)
			got := []string{r.PRState, r.PRMerged, r.PRClosed, r.CIState, r.Reverted, r.RevertState}
			want := []string{tc.state, tc.merged, tc.closed, tc.ci, tc.reverted, tc.window}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v want %v", got, want)
			}
			if tc.mode == "mixed" {
				if !reflect.DeepEqual(r.CIConclusions, trace.StringList{"CI=failure", "layer-pro=success"}) {
					t.Fatal(r.CIConclusions)
				}
				key := WorkflowAttribute("layer-pro", 10)
				if r.WorkflowAttributes[key+"_run"] != "3" || r.WorkflowAttributes[key+"_attempt"] != "2" {
					t.Fatal(r.WorkflowAttributes)
				}
			}
			old := r.SessionOutcomes
			g.Enrich(context.Background(), r)
			if !reflect.DeepEqual(old, r.SessionOutcomes) {
				t.Fatal("not idempotent")
			}
		})
	}
}
func TestWorkflowPagination(t *testing.T) {
	g, r, calls := fixtureOutcome(t, mergedTime.Add(15*24*time.Hour), "pagination")
	g.Enrich(context.Background(), r)
	if *calls != 4 || !reflect.DeepEqual(r.CIConclusions, trace.StringList{"CI=failure", "layer-pro=success"}) {
		t.Fatalf("calls %d row %+v", *calls, r)
	}
}
func TestPaginationExhaustionUnknown(t *testing.T) {
	g, r, _ := fixtureOutcome(t, mergedTime.Add(15*24*time.Hour), "mixed")
	old := g.Run
	g.Run = func(ctx context.Context, d, n string, args ...string) ([]byte, error) {
		if strings.Contains(args[len(args)-1], "actions/runs") {
			return json.Marshal(map[string]any{"workflow_runs": make([]workflowRun, 100)})
		}
		return old(ctx, d, n, args...)
	}
	g.Enrich(context.Background(), r)
	if r.CIState != "unknown" || len(r.CIConclusions) != 0 {
		t.Fatal(r.SessionOutcomes)
	}
}
func TestMergeRevertMessage(t *testing.T) {
	if len(revertMessage.FindStringSubmatch("This reverts commit "+outcomeSHA+", reversing\nchanges made to xyz.")) != 2 {
		t.Fatal("merge revert not recognized")
	}
}
