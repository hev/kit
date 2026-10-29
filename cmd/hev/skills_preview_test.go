package main

import (
	"bytes"
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
	if len(counts) != 4 {
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
	if len(plan.Sessions) != 4 || len(plan.Blocks) == 0 {
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
