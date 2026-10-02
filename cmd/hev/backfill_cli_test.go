package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func backfillFixture(t *testing.T) (backfillRuntime, backfillManifest, string) {
	t.Helper()
	schema := map[string]json.RawMessage{"commits": json.RawMessage(`{"type":"string","filterable":false}`)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/schema") {
			json.NewEncoder(w).Encode(schema)
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/query") {
			t.Error("unexpected mutation", r.Method, r.URL.Path)
			http.Error(w, "mutation", 500)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if !strings.Contains(fmt.Sprint(body["filters"]), "Lte 300") {
			t.Error("missing upper cutoff", body)
		}
		rows := []trace.SessionRow{{ID: "a", Start: 150, End: 200}, {ID: "b", Start: 150, End: 200}}
		if strings.Contains(fmt.Sprint(body["filters"]), "Gt a") {
			rows = rows[1:]
		} else if strings.Contains(fmt.Sprint(body["filters"]), "Gt b") {
			rows = nil
		}
		top := int(body["top_k"].(float64))
		if len(rows) > top {
			rows = rows[:top]
		}
		json.NewEncoder(w).Encode(map[string]any{"rows": rows})
	}))
	t.Cleanup(srv.Close)
	rt := backfillRuntime{Client: func(string) (*layer.Client, error) { return layer.New(srv.URL, "", "archive", ""), nil }, Save: saveBackfill}
	m := backfillManifest{Format: backfillFormat, Policy: backfillPolicy, Target: backfillTarget{Endpoint: srv.URL, Store: layer.StoreTurbopuffer, Account: "owner", Namespace: "archive", Schema: schema}, Since: 100, Until: 300, Complete: true, Rows: []trace.SessionRow{{ID: "a", Start: 150, End: 200}, {ID: "b", Start: 150, End: 200}}}
	m.seal()
	path := filepath.Join(t.TempDir(), "manifest.json")
	if e := saveBackfillJSON(path, m); e != nil {
		t.Fatal(e)
	}
	return rt, m, path
}
func runBackfillTest(rt backfillRuntime, args ...string) (string, error) {
	cmd := newBackfillCommand(rt)
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	err := cmd.Execute()
	return output.String(), err
}
func TestBackfillCLIExportAndPreviewResume(t *testing.T) {
	rt, _, _ := backfillFixture(t)
	dir := t.TempDir()
	export := filepath.Join(dir, "export.json")
	cp := filepath.Join(dir, "preview.json")
	for i := 0; i < 3; i++ {
		_, e := runBackfillTest(rt, "--account", "owner", "--export", export, "--since", "1970-01-01T00:00:00.100Z", "--until", "1970-01-01T00:00:00.300Z", "--limit", "1")
		if e != nil {
			t.Fatal(e)
		}
	}
	m, e := readBackfillManifest(export)
	if e != nil || !m.Complete || len(m.Rows) != 2 || m.SnapshotConsistent {
		t.Fatalf("export %+v %v", m, e)
	}
	args := []string{"--account", "owner", "--cohort", export, "--checkpoint", cp, "--limit", "1"}
	first, e := runBackfillTest(rt, args...)
	if e != nil {
		t.Fatal(e)
	}
	second, e := runBackfillTest(rt, args...)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(first, `"id":"a"`) || strings.Contains(second, `"id":"a"`) || !strings.Contains(second, `"id":"b"`) {
		t.Fatal(first, second)
	}
	var checkpoint backfillCheckpoint
	b, _ := os.ReadFile(cp)
	json.Unmarshal(b, &checkpoint)
	if !checkpoint.Complete || checkpoint.Processed != 2 || checkpoint.Mode != "preview" || checkpoint.Since != 100 || checkpoint.Until != 300 {
		t.Fatal(checkpoint)
	}
}
func TestBackfillCLIMismatchedResume(t *testing.T) {
	for _, change := range []string{"endpoint", "store", "account", "schema", "policy", "format", "cohort", "linkage", "cutoff", "duplicate"} {
		t.Run(change, func(t *testing.T) {
			rt, m, path := backfillFixture(t)
			cp := filepath.Join(t.TempDir(), "checkpoint.json")
			link := filepath.Join(t.TempDir(), "linkage.json")
			os.WriteFile(link, []byte(`{}`), 0600)
			args := []string{"--account", "owner", "--cohort", path, "--checkpoint", cp, "--linkage", link, "--limit", "1"}
			if _, e := runBackfillTest(rt, args...); e != nil {
				t.Fatal(e)
			}
			switch change {
			case "endpoint":
				m.Target.Endpoint += "/different"
			case "store":
				m.Target.Store = "pgvector"
			case "account":
				m.Target.Account = "other"
			case "schema":
				m.Target.Schema["pr"] = json.RawMessage(`{"type":"string"}`)
			case "policy":
				m.Policy = "other"
			case "format":
				m.Format = "other"
			case "cohort":
				m.Rows[1].Summary = "changed"
			case "linkage":
				os.WriteFile(link, []byte(`{"b":{"pr":"8"}}`), 0600)
			case "cutoff":
				m.Until++
			case "duplicate":
				m.Rows = append(m.Rows, m.Rows[0])
			}
			saveBackfillJSON(path, m)
			if _, e := runBackfillTest(rt, args...); e == nil {
				t.Fatal("mismatch resumed")
			}
		})
	}
}
func TestBackfillCLIAcknowledgmentCrashAndRetry(t *testing.T) {
	for _, failure := range []string{"lost_ack", "checkpoint_crash", "readback_conflict", "zero_rows"} {
		t.Run(failure, func(t *testing.T) {
			rt, _, path := backfillFixture(t)
			cp := filepath.Join(t.TempDir(), "checkpoint.json")
			calls := 0
			persisted := map[string]bool{}
			rt.Patch = func(ctx context.Context, r trace.SessionRow) error {
				calls++
				if failure == "checkpoint_crash" {
					persisted[r.ID] = true
					return nil
				}
				if failure == "lost_ack" {
					persisted[r.ID] = true
				}
				return errors.New(failure)
			}
			saveCalls := 0
			rt.Save = func(path string, c backfillCheckpoint) error {
				saveCalls++
				if failure == "checkpoint_crash" && saveCalls == 2 {
					return errors.New("crash after acknowledged patch")
				}
				return saveBackfill(path, c)
			}
			args := []string{"--apply", "--account", "owner", "--cohort", path, "--checkpoint", cp, "--limit", "1", "--retries", "0"}
			if _, e := runBackfillTest(rt, args...); e == nil {
				t.Fatal("failure advanced")
			}
			var c backfillCheckpoint
			b, _ := os.ReadFile(cp)
			json.Unmarshal(b, &c)
			if c.After != "" || c.Processed != 0 {
				t.Fatal(c)
			}
			rt.Save = saveBackfill
			rt.Patch = func(ctx context.Context, r trace.SessionRow) error { calls++; persisted[r.ID] = true; return nil }
			if _, e := runBackfillTest(rt, args...); e != nil {
				t.Fatal(e)
			}
			b, _ = os.ReadFile(cp)
			json.Unmarshal(b, &c)
			if c.After != "a" || c.Processed != 1 || calls != 2 || len(persisted) != 1 {
				t.Fatal(c, calls, persisted)
			}
		})
	}
	// The actual CLI never treats the local lock/readback as apply clearance.
	rt, _, path := backfillFixture(t)
	_, e := runBackfillTest(rt, "--apply", "--cohort", path)
	if e == nil || !strings.Contains(e.Error(), "later old/cross-host upserts") {
		t.Fatal(e)
	}
}
func TestBackfillCLIReconciliationLateChangedRemoved(t *testing.T) {
	rt, m, path := backfillFixture(t)
	after := m
	after.Rows = append([]trace.SessionRow{}, m.Rows...)
	after.Rows[0].Summary = "preservation changed"
	after.Rows[0].SessionOutcomes.CIState = "pending"
	after.Rows = after.Rows[:1]
	after.Rows = append(after.Rows, trace.SessionRow{ID: "0-backdated", Start: 150, End: 200})
	afterPath := filepath.Join(t.TempDir(), "after.json")
	after.seal()
	saveBackfillJSON(afterPath, after)
	output, e := runBackfillTest(rt, "--cohort", path, "--against", afterPath)
	if e != nil {
		t.Fatal(e)
	}
	var report backfillReconciliation
	json.Unmarshal([]byte(output), &report)
	if len(report.Added) != 1 || report.Added[0] != "0-backdated" || len(report.Removed) != 1 || report.Removed[0] != "b" || len(report.PreservedChanged) != 1 || len(report.EnrichmentChanged) != 1 || len(report.SourceChanged) != 0 || report.SnapshotConsistent || !report.EvidenceAffected || report.PreservationVerified || !report.ComparisonComplete {
		t.Fatal(output)
	}
	// Analyzer source changes are independent from outcome/preservation changes.
	after.Rows[0].RepoURL = "different"
	after.seal()
	saveBackfillJSON(afterPath, after)
	output, e = runBackfillTest(rt, "--cohort", path, "--against", afterPath)
	if e != nil {
		t.Fatal(e)
	}
	json.Unmarshal([]byte(output), &report)
	if len(report.SourceChanged) != 1 {
		t.Fatal(output)
	}
}

func TestBackfillCLIBoundedRetriesAndPreviewApplySeparation(t *testing.T) {
	rt, _, path := backfillFixture(t)
	cp := filepath.Join(t.TempDir(), "checkpoint.json")
	failures := 0
	rt.Patch = func(ctx context.Context, r trace.SessionRow) error {
		failures++
		return errors.New("unacknowledged patch")
	}
	args := []string{"--apply", "--account", "owner", "--cohort", path, "--checkpoint", cp, "--limit", "1", "--retries", "1"}
	if _, e := runBackfillTest(rt, args...); e == nil || failures != 2 {
		t.Fatal("retry bound", e, failures)
	}
	var c backfillCheckpoint
	b, _ := os.ReadFile(cp)
	json.Unmarshal(b, &c)
	if c.After != "" {
		t.Fatal("unsafe acknowledgment", c)
	}
	// A preview cursor can never be reused as persisted-write acknowledgment.
	if _, e := runBackfillTest(rt, "--account", "owner", "--cohort", path, "--checkpoint", cp); e == nil {
		t.Fatal("apply/preview journal shared")
	}
}

func TestBackfillManifestCanonicalFingerprints(t *testing.T) {
	rt, m, path := backfillFixture(t)
	// Schema object formatting/order is immaterial to canonical identity.
	a := map[string]json.RawMessage{"x": json.RawMessage(`{"type":"string", "filterable":true}`)}
	b := map[string]json.RawMessage{"x": json.RawMessage(`{"filterable":true,"type":"string"}`)}
	if backfillHash(a) != backfillHash(b) {
		t.Fatal("noncanonical schema hash")
	}
	m.Rows[0].Summary = "tampered without source fingerprint"
	saveBackfillJSON(path, m)
	if _, err := runBackfillTest(rt, "--account", "owner", "--cohort", path, "--checkpoint", filepath.Join(t.TempDir(), "cp.json")); err == nil {
		t.Fatal("tampered source resumed")
	}
}

func TestBackfillCLIExportRefusesChangedBoundsAndBusyJournal(t *testing.T) {
	rt, _, _ := backfillFixture(t)
	path := filepath.Join(t.TempDir(), "export.json")
	args := []string{"--account", "owner", "--export", path, "--since", "1970-01-01T00:00:00.100Z", "--until", "1970-01-01T00:00:00.300Z", "--limit", "1"}
	if _, err := runBackfillTest(rt, args...); err != nil {
		t.Fatal(err)
	}
	unlock, err := lockBackfill(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runBackfillTest(rt, args...); err == nil {
		t.Fatal("concurrent export allowed")
	}
	unlock()
	// Locate the bound by its flag rather than relying on argument positions.
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--until" {
			args[i+1] = "1970-01-01T00:00:00.350Z"
		}
	}
	if _, err := runBackfillTest(rt, args...); err == nil {
		t.Fatal("changed export bound resumed")
	}
}

func TestBackfillCLIReconciliationBaselineEvidence(t *testing.T) {
	rt, _, path := backfillFixture(t)
	for _, baseline := range []string{"", filepath.Join(t.TempDir(), "missing.json")} {
		output, err := runBackfillTest(rt, "--cohort", baseline, "--against", path)
		var report backfillReconciliation
		if json.Unmarshal([]byte(output), &report) != nil || err == nil || report.BaselineState != "absent" || report.ComparisonComplete || report.PreservationVerified {
			t.Fatalf("missing baseline: %s %v", output, err)
		}
	}
	output, err := runBackfillTest(rt, "--cohort", path, "--against", path)
	var report backfillReconciliation
	if json.Unmarshal([]byte(output), &report) != nil || err != nil || report.BaselineState != "observed_complete_projection" || !report.ComparisonComplete || report.PreservationVerified || report.EvidenceAffected {
		t.Fatalf("observed comparison: %s %v", output, err)
	}
}

func TestBackfillReconciliationEnrichmentOnly(t *testing.T) {
	_, before, _ := backfillFixture(t)
	after := before
	after.Rows = append([]trace.SessionRow{}, before.Rows...)
	after.Rows[0].PR = "42"
	after.seal()
	report, err := reconcileBackfill(before, after)
	if err != nil || report.EvidenceAffected || report.PreservationVerified || len(report.EnrichmentChanged) != 1 || len(report.SourceChanged) != 0 || len(report.PreservedChanged) != 0 {
		t.Fatalf("%+v %v", report, err)
	}
}
