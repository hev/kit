package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

func TestCoverageChunkPageAndBatchRetry(t *testing.T) {
	for _, recover := range []bool{false, true} {
		t.Run(fmt.Sprint(recover), func(t *testing.T) {
			var rows []trace.SessionRow
			var chunks []trace.BlockRow
			for i := 0; i < 25; i++ {
				id := fmt.Sprintf("s%02d", i)
				rows = append(rows, trace.SessionRow{ID: id, SessionID: id, FirstPrompt: "# AGENTS.md instructions\nInjected rules"})
				chunks = append(chunks, trace.BlockRow{SessionID: id, Seq: 1, Role: "user", BlockType: "text", Text: "<INSTRUCTIONS>rules</INSTRUCTIONS>"})
				if i < 24 {
					chunks = append(chunks, trace.BlockRow{SessionID: id, Seq: 2, Role: "user", BlockType: "text", Text: "Repair dashboard " + id})
				}
			}
			queries := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || !strings.HasSuffix(r.URL.Path, "/query") {
					t.Error("unexpected write")
				}
				if r.URL.Path == "/v2/namespaces/test/query" {
					queries++
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					raw, _ := json.Marshal(body)
					for _, want := range []string{`"In"`, `"role","Eq","user"`, `"block_type","Eq","text"`, `"rank_by":["seq","asc"]`, `s24`} {
						if !strings.Contains(string(raw), want) {
							t.Errorf("missing %s: %s", want, raw)
						}
					}
					json.NewEncoder(w).Encode(map[string]any{"rows": chunks})
				} else {
					json.NewEncoder(w).Encode(map[string]any{"rows": rows})
				}
			}))
			defer srv.Close()
			failures := 0
			var out bytes.Buffer
			err := runSummaryRepair(context.Background(), &out, layer.New(srv.URL, "key", "test", ""), "2026-09-29T00:00:00Z", "2026-10-29T00:00:00Z", "", 40, false, func(inputs []trace.SessionRow) ([]string, error) {
				if len(inputs) > 10 {
					t.Fatalf("oversize batch: %d", len(inputs))
				}
				for _, row := range inputs {
					if row.FirstPrompt != "Repair dashboard "+row.ID {
						t.Fatalf("input: %+v", row)
					}
				}
				if inputs[0].ID == "s10" {
					failures++
					if failures == 1 {
						return nil, fmt.Errorf("fake timeout")
					}
					if !recover {
						return nil, nil
					}
				} // count mismatch is retryable too
				answers := make([]string, len(inputs))
				for i := range answers {
					answers[i] = `{"task_present":true,"summary":"Repair dashboard summaries"}`
				}
				return answers, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if queries != 1 || failures != 2 {
				t.Fatalf("queries=%d attempts=%d", queries, failures)
			}
			want := 14
			if recover {
				want = 24
			}
			if got := strings.Count(out.String(), `"summary_backfilled":true`); got != want {
				t.Fatalf("summarized %d want %d: %s", got, want, &out)
			}
			failed := 10
			if recover {
				failed = 0
			}
			if strings.Count(out.String(), "Configured summarizer failed") != failed {
				t.Fatal(out.String())
			}
			if !strings.Contains(out.String(), trace.InjectedSummaryReason) {
				t.Fatal(out.String())
			}
		})
	}
}

func TestCoverageRolePrompt(t *testing.T) {
	dir := t.TempDir()
	capture := filepath.Join(dir, "prompt")
	t.Setenv("HEV_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PROMPT_CAPTURE", capture)
	err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\ncat >\"$PROMPT_CAPTURE\"\nprintf '%s' '{\"structured_output\":{\"titles\":[{\"task_present\":true,\"summary\":\"Sous chef delivery decision\"}]}}'\n"), 0700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := claudeSummaryBatch([]trace.SessionRow{{FirstPrompt: "You are the sous chef making one bounded delivery decision"}}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"A role or persona plus a job to do counts as a task", "only when the input is purely injected harness context", "You are the sous chef", "Run one iteration of the @hevmind X loop", "Sous chef delivery decision for oven-84"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("missing %q", want)
		}
	}
	if summaryTimeout.Seconds() != 180 {
		t.Fatal(summaryTimeout)
	}
	if _, err := claudeSummaryBatch(make([]trace.SessionRow, 11)); err == nil {
		t.Fatal("oversize batch accepted")
	}
}

func TestCoverageChunkFragments(t *testing.T) {
	chunks := []trace.BlockRow{{Seq: 3, Role: "user", BlockType: "text", Text: "dashboard summaries"}, {Seq: 2, Role: "user", BlockType: "text", Text: "Repair"}, {Seq: 1, Role: "user", BlockType: "text", Text: "<environment_context>shell</environment_context>"}}
	if got := summaryChunkSource(chunks); got != "Repair\ndashboard summaries" {
		t.Fatal(got)
	}
	if got := summaryChunkSource([]trace.BlockRow{{Role: "user", BlockType: "text", Text: strings.Repeat("界", 1100)}}); len([]rune(got)) != 1000 {
		t.Fatal(len([]rune(got)))
	}
}
