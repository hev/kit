package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/trace"
)

func TestSummaryModelNeverReceivesArchivedSecret(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HEV_CONFIG", filepath.Join(dir, "config.toml"))
	prompt := filepath.Join(dir, "prompt")
	response := filepath.Join(dir, "response")
	t.Setenv("TEST_PROMPT", prompt)
	t.Setenv("TEST_RESPONSE", response)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\ncat > \"$TEST_PROMPT\"\ncat \"$TEST_RESPONSE\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	secret := "sk-ABCDEFGHIJKLMNOP0123456789"
	os.WriteFile(response, []byte(`{"structured_output":{"titles":["`+secret+`"]}}`), 0600)
	titles, err := claudeSummaryBatch([]trace.SessionRow{{FirstPrompt: strings.Repeat("a", 989) + " " + secret}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "sk-ABCDEFGHIJKLMNOP") || !strings.Contains(string(b), "[REDACTED:") {
		t.Fatalf("model input was not scrubbed before truncation: %s", b)
	}
	if len(titles) != 1 || strings.Contains(titles[0], secret) || !strings.Contains(titles[0], "[REDACTED:") {
		t.Fatalf("model output not scrubbed: %v", titles)
	}
}
