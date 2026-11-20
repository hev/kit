package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"github.com/hev/kit/internal/trace"
	"os"
	"path/filepath"
	"testing"
)

func TestSkillsPreviewIsLocalAndCountsHarnesses(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	args := []string{}
	for _, h := range []string{"claude_code", "codex", "pi", "dot"} {
		dir := filepath.Join(root, h)
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile("../../internal/trace/testdata/skills/" + h + ".jsonl")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "fixture.jsonl"), b, 0600); err != nil {
			t.Fatal(err)
		}
		flag := h
		if h == "claude_code" {
			flag = "claude"
		}
		args = append(args, "--"+flag+"-root", dir)
	}
	// OpenCode keeps sessions in SQLite: one real fixture session with a skill load.
	fixture, err := os.ReadFile("../../internal/trace/testdata/opencode/opencode-1.19.5.sql")
	if err != nil {
		t.Fatal(err)
	}
	opencode := filepath.Join(root, "opencode")
	db, err := sql.Open("sqlite", filepath.Join(root, "scratch.db"))
	if err != nil {
		t.Fatal(err)
	}
	os.Mkdir(opencode, 0700)
	for _, s := range []string{string(fixture), "DELETE FROM part WHERE session_id <> 'ses_e48a2f02bffeHbDXxurkNl3k8y'", "DELETE FROM message WHERE session_id <> 'ses_e48a2f02bffeHbDXxurkNl3k8y'", "DELETE FROM session WHERE id <> 'ses_e48a2f02bffeHbDXxurkNl3k8y'", "VACUUM INTO '" + filepath.Join(opencode, "opencode.db") + "'"} {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	args = append(args, "--opencode-root", opencode)
	// Antigravity (agy) keeps sessions under brain/<id>/.system_generated/logs/transcript_full.jsonl.
	agy := filepath.Join(root, "agy")
	agyLogs := filepath.Join(agy, "brain", "agy-fixture-1", ".system_generated", "logs")
	if err := os.MkdirAll(agyLogs, 0700); err != nil {
		t.Fatal(err)
	}
	agyJSONL := `{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-11-19T12:00:00Z","content":"check traces"}` + "\n" +
		`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-19T12:00:01Z","model":"gemini-4-neon-medium","tool_calls":[{"name":"read_file","args":{"Path":"/repo/skills/hev-traces/SKILL.md"}}]}` + "\n"
	if err := os.WriteFile(filepath.Join(agyLogs, "transcript_full.jsonl"), []byte(agyJSONL), 0600); err != nil {
		t.Fatal(err)
	}
	args = append(args, "--agy-root", agy)
	droidProject := filepath.Join(root, ".factory", "sessions", "-example-project")
	if err := os.MkdirAll(droidProject, 0700); err != nil {
		t.Fatal(err)
	}
	droidFixture, err := os.ReadFile("../../internal/trace/testdata/droid.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(droidProject, "session.jsonl"), droidFixture, 0600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "plan.json")
	args = append(args, "--out", output)
	cmd := newSkillsPreviewCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	t.Logf("preview: %s", buf.String())
	var counts map[string]struct {
		Sessions   int `json:"sessions"`
		WithSkills int `json:"with_skills"`
		WouldGain  int `json:"would_gain"`
	}
	if err := json.Unmarshal(buf.Bytes(), &counts); err != nil {
		t.Fatal(err)
	}
	if len(counts) != 7 {
		t.Fatalf("counts %s", buf.String())
	}
	for h, n := range counts {
		wantSkills := 1
		if h == "droid" {
			wantSkills = 0
		}
		if n.Sessions != 1 || n.WithSkills != wantSkills || n.WouldGain != wantSkills {
			t.Fatalf("%s: %+v", h, n)
		}
	}
	b, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var plan struct {
		Sessions []trace.SessionRow `json:"sessions"`
		Blocks   []trace.BlockRow   `json:"blocks"`
	}
	if err = json.Unmarshal(b, &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Sessions) != 7 || len(plan.Blocks) == 0 {
		t.Fatal("empty plan")
	}
	baseline := filepath.Join(root, "existing.json")
	b, _ = json.Marshal(plan.Sessions)
	if err = os.WriteFile(baseline, b, 0600); err != nil {
		t.Fatal(err)
	}
	cmd = newSkillsPreviewCmd()
	buf.Reset()
	cmd.SetOut(&buf)
	cmd.SetArgs(append(args[:len(args)-2], "--existing-sessions", baseline))
	if err = cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(buf.Bytes(), &counts); err != nil {
		t.Fatal(err)
	}
	for h, n := range counts {
		if n.WouldGain != 0 {
			t.Fatalf("%s already has skills: %+v", h, n)
		}
	}
}
