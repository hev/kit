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
	if len(counts) != 5 {
		t.Fatalf("counts %s", buf.String())
	}
	for h, n := range counts {
		if n.Sessions != 1 || n.WithSkills != 1 || n.WouldGain != 1 {
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
	if len(plan.Sessions) != 5 || len(plan.Blocks) == 0 {
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
