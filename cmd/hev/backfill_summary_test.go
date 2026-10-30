package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

func TestArchiveSummaryAllHarnesses(t *testing.T) {
	// Exercise the real configured CLI adapter without a provider or credential.
	dir := t.TempDir()
	t.Setenv("HEV_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\ncat >/dev/null\nprintf '%s' '{\"structured_output\":{\"titles\":[{\"task_present\":true,\"summary\":\"Fixed archive summary generation and checked the dashboard.\"}]}}'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, harness := range []string{"claude_code", "codex", "dot", "pi"} {
		t.Run(harness, func(t *testing.T) {
			row := summarizeArchiveSession(trace.SessionRow{Harness: harness, FirstPrompt: "Fix archive summary generation and check the dashboard"}, claudeSummary)
			if row.Summary == "" || strings.Count(row.Summary, "\n") > 1 || row.SummaryReason != "" {
				t.Fatalf("repair: %+v", row)
			}
		})
	}
}

func TestArchiveSummaryExplicitFailures(t *testing.T) {
	for _, tc := range []struct {
		name, prompt string
		summary      string
		err          error
	}{
		{name: "missing prompt"}, {name: "empty model", prompt: "fix it"}, {name: "whitespace model", prompt: "fix it", summary: " \n "}, {name: "model error", prompt: "fix it", err: errors.New("provider error")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := summarizeArchiveSession(trace.SessionRow{FirstPrompt: tc.prompt}, func(trace.SessionRow) (string, error) { return tc.summary, tc.err })
			if row.Summary != "" || row.SummaryReason == "" {
				t.Fatalf("blank without reason: %+v", row)
			}
		})
	}
	changed, errs := fillMissingSummaries([]trace.SessionRow{{SessionID: "s"}}, func(trace.SessionRow) (string, error) { return " \n ", nil })
	if len(changed) != 0 || len(errs) != 1 {
		t.Fatalf("empty result acknowledged: %v %v", changed, errs)
	}
}

func TestSummaryRepairPreviewNoWrites(t *testing.T) {
	var query map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/query") {
			t.Errorf("preview attempted write: %s", r.URL.Path)
			http.Error(w, "write", 500)
			return
		}
		json.NewDecoder(r.Body).Decode(&query)
		json.NewEncoder(w).Encode(map[string]any{"rows": []trace.SessionRow{{ID: "a", Harness: "codex", FirstPrompt: "fix archive"}, {ID: "b", Harness: "pi"}}})
	}))
	defer srv.Close()
	var out bytes.Buffer
	err := runSummaryRepair(context.Background(), &out, layer.New(srv.URL, "key", "test", ""), "2026-09-29T00:00:00Z", "2026-10-29T00:00:00Z", "", 10, false, func([]trace.SessionRow) ([]string, error) {
		return []string{`{"task_present":true,"summary":"Fixed archive summaries"}`}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(query["exclude_attributes"]) != `["vector"]` {
		t.Fatalf("prompt omitted: %s", query["exclude_attributes"])
	}
	for _, want := range []string{`"summary_backfilled":true`, `"summary_backfilled":false`, `"summary_reason":"Only injected harness context`, `"next_after":"b"`, `"applied":false`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %s: %s", want, &out)
		}
	}
}

func TestArchiveSummaryInjectedContextAndGuard(t *testing.T) {
	row := summarizeArchiveSession(trace.SessionRow{FirstPrompt: "<INSTRUCTIONS>Coordinate agents</INSTRUCTIONS>", SummaryBackfilled: true}, func(trace.SessionRow) (string, error) { t.Fatal("called model for context"); return "", nil })
	if row.SummaryReason != trace.InjectedSummaryReason || row.SummaryBackfilled || row.Summary != "" {
		t.Fatalf("context: %+v", row)
	}
	for _, answer := range []string{"NO_TASK", "Dashboard session: agent coordination instructions and environment setup", "Agent coordination instructions and environment setup", "Sandbox permissions and environment configuration", "hevmind-promo session: archived prompt is only the agent-coordination preamble", "Contains instructions only; no actual request", "Only AGENTS.md instructions and sandbox context"} {
		row = summarizeArchiveSession(trace.SessionRow{FirstPrompt: "Repair the dashboard summaries"}, func(trace.SessionRow) (string, error) {
			encoded, _ := json.Marshal(map[string]any{"task_present": true, "summary": answer})
			return string(encoded), nil
		})
		if row.SummaryBackfilled || row.Summary != "" || row.SummaryReason == "" {
			t.Fatalf("accepted preamble: %+v", row)
		}
	}
	row = summarizeArchiveSession(trace.SessionRow{FirstPrompt: "Repair AGENTS.md instructions"}, func(trace.SessionRow) (string, error) {
		return `{"task_present":true,"summary":"Repair AGENTS.md instructions"}`, nil
	})
	if !row.SummaryBackfilled {
		t.Fatalf("rejected real task: %+v", row)
	}
}

func TestSummaryRepairReadsLaterArchivedRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/query") {
			t.Fatal("unexpected write")
		}
		if r.URL.Path == "/v2/namespaces/test/query" {
			json.NewEncoder(w).Encode(map[string]any{"rows": []trace.BlockRow{{SessionID: "sid", Seq: 1, Role: "user", BlockType: "text", Text: "<INSTRUCTIONS>Coordinate agents</INSTRUCTIONS>"}, {SessionID: "sid", Seq: 2, Role: "user", BlockType: "text", Text: "Repair the dashboard"}}})
		} else {
			json.NewEncoder(w).Encode(map[string]any{"rows": []trace.SessionRow{{ID: "a", SessionID: "sid", Harness: "codex", FirstPrompt: "<environment_context>bash</environment_context>"}}})
		}
	}))
	defer srv.Close()
	var out bytes.Buffer
	err := runSummaryRepair(context.Background(), &out, layer.New(srv.URL, "key", "test", ""), "2026-09-29T00:00:00Z", "2026-10-29T00:00:00Z", "", 10, false, func(rows []trace.SessionRow) ([]string, error) {
		if len(rows) != 1 || rows[0].FirstPrompt != "Repair the dashboard" || rows[0].FirstPromptShort != "Repair the dashboard" {
			t.Fatalf("model inputs: %+v", rows)
		}
		return []string{`{"task_present":true,"summary":"Repair dashboard summaries"}`}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"summary_backfilled":true`) {
		t.Fatal(out.String())
	}
}

func TestArchiveSummaryAgentsSubjectMatter(t *testing.T) {
	row := summarizeArchiveSession(trace.SessionRow{FirstPrompt: "Repair coordination between agents running dashboard jobs"}, func(trace.SessionRow) (string, error) {
		return `{"task_present":true,"summary":"Repair coordination between dashboard agents"}`, nil
	})
	if !row.SummaryBackfilled || row.SummaryReason != "" {
		t.Fatalf("rejected real agents task: %+v", row)
	}
}

func TestArchiveSummaryOutputContract(t *testing.T) {
	for _, answer := range []string{"NO_TASK", "", "  ", "not JSON", `{"task_present":false,"summary":"Repair dashboard"}`, `{"summary":"Repair dashboard"}`, `{"task_present":true,"summary":""}`, "Read AGENTS.md and CLAUDE.md instructions; no user task provided"} {
		t.Run(answer, func(t *testing.T) {
			row := summarizeArchiveSession(trace.SessionRow{FirstPrompt: "Repair dashboard summary selection"}, func(trace.SessionRow) (string, error) { return answer, nil })
			if row.Summary != "" || row.SummaryBackfilled || row.SummaryReason == "" {
				t.Fatalf("accepted: %+v", row)
			}
		})
	}
	for _, phrase := range []string{"no user task", "no task provided", "no request", "no concrete task", "nothing to do", "no instructions from the user"} {
		t.Run(phrase, func(t *testing.T) {
			answer, _ := json.Marshal(map[string]any{"task_present": true, "summary": "Read instructions; " + strings.ToUpper(strings.ReplaceAll(phrase, " ", "! "))})
			row := summarizeArchiveSession(trace.SessionRow{FirstPrompt: "Repair dashboard"}, func(trace.SessionRow) (string, error) { return string(answer), nil })
			if row.SummaryBackfilled || row.Summary != "" || row.SummaryReason == "" {
				t.Fatalf("accepted: %+v", row)
			}
		})
	}
}

func TestArchiveSummaryValidContract(t *testing.T) {
	for _, summary := range []string{"Repair dashboard summary selection", "Repaired dashboard summary selection", "Read AGENTS.md and CLAUDE.md instructions; no user task provided"} {
		answer, _ := json.Marshal(map[string]any{"task_present": true, "summary": summary})
		row := summarizeArchiveSession(trace.SessionRow{FirstPrompt: "Repair dashboard summary selection"}, func(trace.SessionRow) (string, error) { return string(answer), nil })
		want := !strings.Contains(summary, "no user task")
		if row.SummaryBackfilled != want || (want && (row.Summary != summary || row.SummaryReason != "")) {
			t.Fatalf("contract: %+v", row)
		}
	}
}

func TestArchiveSummaryGrounding(t *testing.T) {
	for _, tc := range []struct {
		request, summary string
		keep             bool
	}{
		{"Repair dashboard summaries", "hevmind-promo session: archived prompt is only the agent-coordination preamble", false},
		{"Repair dashboard summaries", "Only AGENTS.md instructions and sandbox context", false},
		{"Repair dashboard summaries", "Sandbox permissions and environment configuration", false},
		{"Repair dashboard summaries", "Dashboard session: agent coordination instructions and environment setup", false},
		{"Repair dashboard summaries", "Read AGENTS.md and CLAUDE.md instructions; no user task provided", false},
		{"Update AGENTS.md sandbox permission rules for Codex", "Updated AGENTS.md sandbox permission rules", true},
		{"Repair dashboard summaries", "Repaired dashboard summary selection", true},
		{"Repair dashboard summaries", strings.Repeat("Dashboard repair ", 20) + "sandbox", false},
	} {
		t.Run(tc.summary, func(t *testing.T) {
			answer, _ := json.Marshal(map[string]any{"task_present": true, "summary": tc.summary})
			row := summarizeArchiveSession(trace.SessionRow{FirstPrompt: tc.request}, func(trace.SessionRow) (string, error) { return string(answer), nil })
			if row.SummaryBackfilled != tc.keep || (!tc.keep && (row.Summary != "" || row.SummaryReason == "")) {
				t.Fatalf("grounding: %+v", row)
			}
			if !tc.keep && !strings.Contains(tc.summary, "no user task") && row.SummaryReason != "summary describes setup, not the request" {
				t.Fatalf("reason: %q", row.SummaryReason)
			}
		})
	}
}

// Pin the closed order probes at the archive repair boundary, including flags
// and reasons, rather than testing only the vocabulary helper.
func TestArchiveSummaryClosedGuardProbes(t *testing.T) {
	const request = "Repair the dashboard summaries"
	for _, tc := range []struct {
		name, prompt, summary, raw string
		noTask, keep               bool
	}{
		{name: "archived preamble", summary: "hevmind-promo session: archived prompt is only the agent-coordination preamble"},
		{name: "instructions sandbox", summary: "Only AGENTS.md instructions and sandbox context"},
		{name: "environment configuration", summary: "Sandbox permissions and environment configuration"},
		{name: "coordination setup", summary: "Dashboard session: agent coordination instructions and environment setup"},
		{name: "no user task", summary: "Read AGENTS.md and CLAUDE.md instructions; no user task provided"},
		{name: "injected harness", summary: "Archived prompt contains only injected harness context"},
		{name: "echoed reason", summary: "Only injected harness context in archived prompts"},
		{name: "sentinel", raw: "NO_TASK"},
		{name: "JSON sentinel", summary: "NO_TASK"},
		{name: "empty"},
		{name: "whitespace", raw: " \n "},
		{name: "unparseable", raw: "not JSON"},
		{name: "false with summary", noTask: true, summary: "Repaired dashboard summary selection"},
		{name: "real repair", summary: "Repaired dashboard summary selection", keep: true},
		{name: "instruction file task", prompt: "Update AGENTS.md sandbox permission rules for Codex", summary: "Updated AGENTS.md sandbox permission rules", keep: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prompt := tc.prompt
			if prompt == "" {
				prompt = request
			}
			answer := tc.raw
			if tc.summary != "" {
				encoded, err := json.Marshal(map[string]any{"task_present": !tc.noTask, "summary": tc.summary})
				if err != nil {
					t.Fatal(err)
				}
				answer = string(encoded)
			}
			row := summarizeArchiveSession(trace.SessionRow{FirstPrompt: prompt, SummaryBackfilled: true}, func(trace.SessionRow) (string, error) { return answer, nil })
			if tc.keep {
				if !row.SummaryBackfilled || row.Summary != tc.summary || row.SummaryReason != "" {
					t.Fatalf("rejected valid answer: %+v", row)
				}
			} else if row.Summary != "" || row.SummaryBackfilled || row.SummaryReason == "" {
				t.Fatalf("accepted invalid answer: %+v", row)
			}
		})
	}
}
