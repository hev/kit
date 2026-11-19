package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/trace"
)

func TestAgySourceScope(t *testing.T) {
	root, harness := sourceScope(&trace.AgySource{Root: "/synthetic/agy"})
	if root != "/synthetic/agy" || harness != "agy" {
		t.Fatalf("sourceScope = (%q, %q), want (/synthetic/agy, agy)", root, harness)
	}
}

func TestAgyIndexEndToEndAndIncrementalGrowth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	agyRoot := filepath.Join(home, ".gemini", "antigravity-cli")
	cid := "agy-e2e-session-1"
	logsDir := filepath.Join(agyRoot, "brain", cid, ".system_generated", "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptPath := filepath.Join(logsDir, "transcript_full.jsonl")
	initial := `{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-11-19T12:00:00Z","content":"<USER_REQUEST>\nCheck agy indexing\n</USER_REQUEST>"}` + "\n" +
		`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-19T12:00:01Z","model":"gemini-4-neon-medium","input_tokens":100,"output_tokens":20,"content":"Checked and verified."}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}

	archiveDir := filepath.Join(t.TempDir(), "archive")
	store, err := filearchive.Open(archiveDir, "test")
	if err != nil {
		t.Fatal(err)
	}
	state := &State{Units: map[string]string{}}
	src := &trace.AgySource{Root: agyRoot}

	rep, err := Run(src, store, state, Options{Tiers: trace.AllTiers[:]})
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if rep.UnitsIndexed != 1 || rep.Sessions != 1 || len(rep.Errors) != 0 {
		t.Fatalf("unexpected first report: %+v", rep)
	}

	// Second scan with no changes skips the unit.
	rep2, err := Run(src, store, state, Options{Tiers: trace.AllTiers[:]})
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if rep2.UnitsIndexed != 0 || rep2.UnitsSkipped != 1 {
		t.Fatalf("expected skip on unchanged unit, got %+v", rep2)
	}

	// Append a new completed turn: third scan re-indexes cleanly.
	grown := initial +
		`{"step_index":2,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-11-19T12:01:00Z","content":"<USER_REQUEST>\nFollow up question\n</USER_REQUEST>"}` + "\n" +
		`{"step_index":3,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-19T12:01:01Z","model":"gemini-4-neon-medium","input_tokens":150,"output_tokens":30,"content":"Follow up answer."}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(grown), 0o600); err != nil {
		t.Fatal(err)
	}
	rep3, err := Run(src, store, state, Options{Tiers: trace.AllTiers[:]})
	if err != nil {
		t.Fatalf("third Run: %v", err)
	}
	if rep3.UnitsIndexed != 1 || rep3.Turns != 4 || len(rep3.Errors) != 0 {
		t.Fatalf("unexpected third report: %+v", rep3)
	}

	rows, err := store.ListSessionRows(10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Harness != "agy" || rows[0].SessionID != cid || rows[0].FirstPrompt != "Check agy indexing" {
		t.Fatalf("unexpected archived sessions: %+v", rows)
	}
}
