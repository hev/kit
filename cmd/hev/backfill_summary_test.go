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
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\ncat >/dev/null\nprintf '%s' '{\"structured_output\":{\"titles\":[\"Fixed archive summary generation and checked the dashboard.\"]}}'\n"), 0700); err != nil {
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
	err := runSummaryRepair(context.Background(), &out, layer.New(srv.URL, "key", "test", ""), "2026-09-29T00:00:00Z", "2026-10-29T00:00:00Z", "", 10, false, func([]trace.SessionRow) ([]string, error) { return []string{"Fixed archive summaries"}, nil })
	if err != nil {
		t.Fatal(err)
	}
	if string(query["exclude_attributes"]) != `["vector"]` {
		t.Fatalf("prompt omitted: %s", query["exclude_attributes"])
	}
	for _, want := range []string{`"summary_backfilled":true`, `"summary_backfilled":false`, `"summary_reason":"No archived prompt`, `"next_after":"b"`, `"applied":false`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %s: %s", want, &out)
		}
	}
}
