package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSkillsPreviewLocalOnly(t *testing.T) {
	// Point every source at its own fixture root; no namespace or credentials.
	cmd := newSkillsPreviewCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	for _, h := range []string{"claude_code", "codex", "pi", "dot"} {
		dir := t.TempDir()
		raw, err := os.ReadFile("../../internal/trace/testdata/skills/" + h + ".jsonl")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "fixture.jsonl"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		if err = cmd.Flags().Set(h, dir); err != nil {
			t.Fatal(err)
		}
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	var reports []struct {
		Harness  string `json:"harness"`
		Sessions int    `json:"sessions"`
		Gain     int    `json:"would_gain_skills"`
	}
	if err := json.Unmarshal(out.Bytes(), &reports); err != nil {
		t.Fatal(err)
	}
	if len(reports) != 4 {
		t.Fatalf("reports=%s", out.String())
	}
	for _, r := range reports {
		if r.Sessions != 1 || r.Gain != 1 {
			t.Fatalf("report=%+v", r)
		}
	}
	if cmd.Flags().Lookup("apply") != nil {
		t.Fatal("preview must have no apply mode")
	}
}
