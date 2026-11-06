package intake

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStoppedComments(t *testing.T) {
	for _, scenario := range []string{"stopped", "marker", "restart-after-post", "reassigned", "dry-run", "batch", "other-author", "old-marker", "pagination", "changed-to-dry-run", "wrong-viewer"} {
		t.Run(scenario, func(t *testing.T) {
			now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
			cfg := Config{StateDir: t.TempDir()}
			out := &bytes.Buffer{}
			p, err := newPoller(cfg, "secret", out)
			if err != nil {
				t.Fatal(err)
			}
			p.now = func() time.Time { return now }
			rule := Rule{Name: "Ready", Action: "fix", Mode: "live", MarkerPrefix: "custom-fix:v1"}
			if scenario == "dry-run" {
				rule.Mode = "dry-run"
			}
			tickets := []Issue{{ID: "one", Identifier: "TEST-1"}}
			if scenario == "batch" {
				tickets = append(tickets, Issue{ID: "two", Identifier: "TEST-2"})
			}
			if err := p.trackRun("test-1-fix-abcdef", rule, tickets); err != nil {
				t.Fatal(err)
			}
			if scenario == "changed-to-dry-run" {
				p.cfg.Status = []Rule{{Name: "Ready", Mode: "dry-run"}}
			}
			run := p.state.CommentRuns["test-1-fix-abcdef"]
			run.Terminal = true
			code := 0
			run.Exit = &code
			posts, reads := 0, 0
			comments := []map[string]any{}
			if scenario == "marker" || scenario == "old-marker" || scenario == "other-author" {
				author, at := "bot", now.Add(time.Second)
				if scenario == "other-author" {
					author = "human"
				}
				if scenario == "old-marker" {
					at = now.Add(-time.Second)
				}
				comments = append(comments, map[string]any{"body": "custom-fix:v1 phase=done", "createdAt": at, "user": map[string]string{"id": author}})
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Query     string
					Variables map[string]any
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				if strings.Contains(request.Query, "mutation") {
					posts++
					input := request.Variables["input"].(map[string]any)
					body := input["body"].(string)
					expected := "hev-intake:v1 phase=stopped run=test-1-fix-abcdef\nThe fix run stopped without updating this ticket (exit 0).\nNothing was changed. To retry, move the ticket out of Ready and back."
					if body != expected {
						t.Errorf("body = %q", body)
					}
					comments = append(comments, map[string]any{"body": body, "createdAt": now.Add(time.Second), "user": map[string]string{"id": "bot"}})
					// Keep batch comments scoped to the ticket.
					if scenario == "batch" {
						comments = nil
					}
					json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"commentCreate": map[string]bool{"success": true}}})
					return
				}
				reads++
				viewer := "bot"
				if scenario == "wrong-viewer" {
					viewer = "human"
				}
				assignee := "bot"
				if scenario == "reassigned" {
					assignee = "human"
				}
				page := map[string]any{"hasNextPage": false}
				nodes := comments
				if scenario == "pagination" && request.Variables["after"] == nil {
					page = map[string]any{"hasNextPage": true, "endCursor": "next"}
					nodes = nil
				}
				json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"viewer": map[string]string{"id": viewer}, "issue": map[string]any{"assignee": map[string]string{"id": assignee}, "comments": map[string]any{"nodes": nodes, "pageInfo": page}}}})
			}))
			defer server.Close()
			p.endpoint = server.URL
			p.checkStopped(context.Background(), "bot")
			if scenario == "restart-after-post" {
				// Simulate a crash after the remote write but before recording its outcome.
				run.Outcomes = map[string]string{}
				if err := p.save(); err != nil {
					t.Fatal(err)
				}
				p, err = newPoller(cfg, "secret", out)
				if err != nil {
					t.Fatal(err)
				}
				p.now = func() time.Time { return now }
				p.endpoint = server.URL
			}
			p.checkStopped(context.Background(), "bot")
			want := 1
			if scenario == "marker" || scenario == "reassigned" || scenario == "dry-run" || scenario == "changed-to-dry-run" || scenario == "wrong-viewer" {
				want = 0
			}
			if scenario == "batch" {
				want = 2
			}
			if posts != want {
				t.Fatalf("posts = %d, want %d", posts, want)
			}
			if scenario == "dry-run" && !strings.Contains(out.String(), "would-comment") {
				t.Fatal("missing dry-run log")
			}
			if scenario == "pagination" && reads < 2 {
				t.Fatal("comments not paginated")
			}
		})
	}
}

func TestStoppedInventory(t *testing.T) {
	for _, state := range []string{"done", "failed", "stopped", "running", "reserved", "launch-unconfirmed"} {
		t.Run(state, func(t *testing.T) {
			p, err := newPoller(Config{StateDir: t.TempDir()}, "", &bytes.Buffer{})
			if err != nil {
				t.Fatal(err)
			}
			p.state.CommentRuns = map[string]*CommentRun{"run": {}}
			p.command = func(context.Context, ...string) ([]byte, error) {
				return []byte(`{"host":{"rows":[{"name":"run","state":"` + state + `","exit":7}]}}`), nil
			}
			if _, _, err := p.inventory(context.Background()); err != nil {
				t.Fatal(err)
			}
			run := p.state.CommentRuns["run"]
			want := state == "done" || state == "failed" || state == "stopped"
			if run.Terminal != want || run.Exit == nil || *run.Exit != 7 {
				t.Fatalf("run = %+v", run)
			}
		})
	}
}

func TestStoppedFailureIsolation(t *testing.T) {
	for _, failure := range []string{"5xx", "rejected"} {
		t.Run(failure, func(t *testing.T) {
			f := setup(t)
			f.live(t)
			now := f.p.now()
			f.p.now = func() time.Time { return now }
			posts := 0
			f.stoppedHandler = func(w http.ResponseWriter, query string) {
				if strings.Contains(query, "StoppedTicket") {
					fmt.Fprint(w, `{"data":{"viewer":{"id":"bot-id"},"issue":{"assignee":{"id":"bot-id"},"comments":{"nodes":[],"pageInfo":{}}}}}`)
					return
				}
				posts++
				if failure == "5xx" {
					w.WriteHeader(503)
					return
				}
				fmt.Fprint(w, `{"data":{"commentCreate":{"success":false}}}`)
			}
			f.p.state.CommentRuns = map[string]*CommentRun{"old-run": {Terminal: true, Rule: Rule{Name: "Review", Action: "triage", Mode: "live", MarkerPrefix: "custom:v1"}, Tickets: []Issue{{ID: "old", Identifier: "TEST-OLD"}}}}
			f.poll(t)
			if posts != 1 || f.p.day().Dispatches != 1 {
				t.Fatalf("same-poll dispatch blocked: posts=%d dispatches=%d", posts, f.p.day().Dispatches)
			}
			// Persist/reload retry state as a restart would, without changing fixture wiring.
			reloaded, err := newPoller(f.p.cfg, "fake-only", f.out)
			if err != nil {
				t.Fatal(err)
			}
			f.p.state = reloaded.state
			good := f.issue()
			good.ID = "second"
			good.Identifier = "TEST-2"
			f.extraIssues = []Issue{good}
			f.poll(t)
			if posts != 1 || f.p.day().Dispatches != 2 {
				t.Fatalf("subsequent poll blocked: posts=%d dispatches=%d", posts, f.p.day().Dispatches)
			}
			for attempt := 1; attempt <= 10; attempt++ {
				retry := f.p.state.CommentRuns["old-run"].Retries["old"]
				if retry.Attempts != attempt || !retry.NextRetry.After(now) || retry.NextRetry.Sub(now) > time.Hour {
					t.Fatalf("retry=%+v now=%v", retry, now)
				}
				now = retry.NextRetry.Add(-time.Nanosecond)
				f.poll(t)
				if posts != attempt {
					t.Fatalf("retried early: %d", posts)
				}
				now = retry.NextRetry
				f.poll(t)
				if posts != attempt+1 {
					t.Fatalf("did not retry: %d", posts)
				}
				if !f.p.state.BackoffUntil.IsZero() || f.p.state.Failures != 0 {
					t.Fatalf("global backoff: %v failures=%d", f.p.state.BackoffUntil, f.p.state.Failures)
				}
			}
		})
	}
}
