package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/hygiene"
)

func TestHygieneCLIRunAndStatus(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfgPath := filepath.Join(home, ".hev", "config.toml")
	t.Setenv("HEV_CONFIG", cfgPath)
	t.Setenv("HEV_ORDERS_DIR", filepath.Join(home, "orders"))
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)
	t.Setenv("TMP", tmpDir)
	t.Setenv("TEMP", tmpDir)

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatal(err)
	}
	cfgText := "[hygiene]\nenabled = false\nevery = \"6h\"\nmin_free_gb = 0\ncompact_after = \"24h\"\nprune_tmp_after = \"24h\"\nthin_apfs_snapshots = false\n"
	if err := os.WriteFile(cfgPath, []byte(cfgText), 0o600); err != nil {
		t.Fatal(err)
	}

	exec := func(args ...string) string {
		t.Helper()
		cmd := newHygieneCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("hev hygiene %v failed: %v\n%s", args, err, out.String())
		}
		return out.String()
	}

	// 1. Dry-run JSON succeeds and creates zero state files.
	dryOut := exec("run", "--dry-run", "--json")
	var dryRep hygiene.Report
	if err := json.Unmarshal([]byte(dryOut), &dryRep); err != nil {
		t.Fatalf("decode dry-run json: %v\n%s", err, dryOut)
	}
	if !dryRep.DryRun {
		t.Fatal("expected dry_run=true")
	}
	if _, err := os.Stat(hygiene.StatePath(home)); !os.IsNotExist(err) {
		t.Fatalf("dry-run created state file: %v", err)
	}

	// 2. Real run JSON writes state.json.
	runOut := exec("run", "--json")
	var realRep hygiene.Report
	if err := json.Unmarshal([]byte(runOut), &realRep); err != nil {
		t.Fatalf("decode run json: %v\n%s", err, runOut)
	}
	if realRep.DryRun {
		t.Fatal("expected dry_run=false")
	}
	if _, err := os.Stat(hygiene.StatePath(home)); err != nil {
		t.Fatalf("expected state.json after real run: %v", err)
	}

	// 3. Status JSON reports the recorded run.
	statusOut := exec("status", "--json")
	var status hygiene.StatusReport
	if err := json.Unmarshal([]byte(statusOut), &status); err != nil {
		t.Fatalf("decode status json: %v\n%s", err, statusOut)
	}
	if status.State.LastRun == nil {
		t.Fatal("expected status.state.last_run to be populated")
	}

	// 4. Human-readable output smoke check.
	textStatus := exec("status")
	if !strings.Contains(textStatus, "Enabled: false") || !strings.Contains(textStatus, "Last run:") {
		t.Fatalf("unexpected text status:\n%s", textStatus)
	}
}
