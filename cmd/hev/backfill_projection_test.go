package main

import (
	"encoding/json"
	"github.com/hev/kit/internal/trace"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackfillPinnedAnalyzerProjectionFields(t *testing.T) {
	base := trace.SessionRow{ID: "row", SessionID: "session", Harness: "harness", Host: "host", RepoURL: "https://example.test/repo", Start: 100, End: 200, ToolCount: 2}
	mutations := map[string]func(*trace.SessionRow){
		"id": func(r *trace.SessionRow) { r.ID = "changed" }, "session_id": func(r *trace.SessionRow) { r.SessionID = "changed" },
		"harness": func(r *trace.SessionRow) { r.Harness = "changed" }, "host": func(r *trace.SessionRow) { r.Host = "changed" },
		"repo_url": func(r *trace.SessionRow) { r.RepoURL = "changed" }, "start": func(r *trace.SessionRow) { r.Start++ },
		"end": func(r *trace.SessionRow) { r.End++ }, "tool_count": func(r *trace.SessionRow) { r.ToolCount++ },
	}
	for field, mutate := range mutations {
		t.Run(field, func(t *testing.T) {
			changed := base
			mutate(&changed)
			if backfillSourceHash(changed) == backfillSourceHash(base) {
				t.Fatal("protected field absent from source fingerprint", field)
			}
		})
	}
	changed := base
	changed.Commits = trace.StringList{strings.Repeat("a", 40)}
	changed.PR = "7"
	changed.CIState = "pending"
	changed.Reverted = "unknown"
	changed.WorkflowAttributes = map[string]string{"ci_workflow_example_conclusion": "failure"}
	if backfillSourceHash(changed) != backfillSourceHash(base) || backfillPreservedHash(changed) != backfillPreservedHash(base) || backfillEnrichmentHash(changed) == backfillEnrichmentHash(base) {
		t.Fatal("enrichment polluted protected projection")
	}
}

func TestBackfillCrossLanguageProjectionFixture(t *testing.T) {
	b, err := os.ReadFile("testdata/session-projection-hashes.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		GoVersion     string `json:"go_hash_version"`
		PythonVersion string `json:"python_hash_version"`
		Cases         []struct {
			Name         string          `json:"name"`
			Row          json.RawMessage `json:"row"`
			GoProjection map[string]any  `json:"go_projection"`
			GoHash       string          `json:"go_source_sha256"`
			PythonHash   string          `json:"python_source_sha256"`
		} `json:"fixtures"`
	}
	if err = json.Unmarshal(b, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.GoVersion != backfillFormat || fixture.PythonVersion == backfillFormat {
		t.Fatal("hash versions silently equated")
	}
	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			var row trace.SessionRow
			if err = json.Unmarshal(c.Row, &row); err != nil {
				t.Fatal(err)
			}
			if got := backfillSourceHash(row); got != c.GoHash {
				t.Fatalf("source hash %s != independent fixture %s", got, c.GoHash)
			}
			if backfillHash(c.GoProjection) != c.GoHash {
				t.Fatal("projection normalization differs")
			}
			// Python keeps nulls, does not HTML-escape and binds ID separately. These
			// versioned digests are intentionally not interchangeable receipt evidence.
			if c.GoHash == c.PythonHash {
				t.Fatal("fixture failed to expose normalization difference")
			}
		})
	}
	var ascii, legacy string
	for _, c := range fixture.Cases {
		if c.Name == "complete_ascii" {
			ascii = c.GoHash
		}
		if c.Name == "legacy_commits" {
			legacy = c.GoHash
		}
	}
	if ascii != legacy {
		t.Fatal("legacy commit encoding changed analyzer digest")
	}
}

func TestBackfillDeclaredProvenanceBindsResumeWithoutVerification(t *testing.T) {
	rt, m, path := backfillFixture(t)
	m.Provenance = &backfillProvenance{EvidenceStatus: "declared-unverified", Publishers: []backfillPublisher{{Host: "example-host", ObservedAt: "2026-01-01T00:00:00Z", EvidenceReference: "private-receipt"}}, ContractReferences: map[string]string{"capture": "known-contract"}}
	if err := saveBackfillJSON(path, m); err != nil {
		t.Fatal(err)
	}
	cp := filepath.Join(t.TempDir(), "checkpoint.json")
	args := []string{"--account", "owner", "--cohort", path, "--checkpoint", cp, "--limit", "1"}
	if _, err := runBackfillTest(rt, args...); err != nil {
		t.Fatal(err)
	}
	revision := "declared-new-publisher-revision"
	m.Provenance.CaptureRevision = &revision
	saveBackfillJSON(path, m)
	if _, err := runBackfillTest(rt, args...); err == nil {
		t.Fatal("changed publisher declaration resumed")
	}
	m.Provenance.PublisherInventoryComplete = true
	if err := m.validate(); err == nil {
		t.Fatal("declared publisher inventory promoted to verification")
	}
}
