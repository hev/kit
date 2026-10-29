package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSkillPreviewNoStore(t *testing.T) {
	p := filepath.Join(t.TempDir(), "dot.jsonl")
	err := os.WriteFile(p, []byte(`{"session_id":"s","id":"a","ts":"2026-10-29T10:00:00Z","role":"assistant","tool_calls":[{"name":"read_file","args":{"path":"/repo/skills/review/SKILL.md"}}]}`+"\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}
	cmd, _, err := rootCmd.Find([]string{"skills-preview"})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	defer cmd.SetOut(nil)
	if err := cmd.RunE(cmd, []string{"dot", p}); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Scanned int `json:"sessions_scanned"`
		Gain    int `json:"sessions_with_skills"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Scanned != 1 || got.Gain != 1 {
		t.Fatalf("preview %s", buf.String())
	}
}
