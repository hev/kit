package daemon

import (
	"strings"
	"testing"
	"time"
)

func TestWrapCodexRollout(t *testing.T) {
	raw := []byte(`{"timestamp":"2026-05-13T13:45:55.110Z","type":"session_meta","payload":{"id":"019e2195-5b7f-7f11-84b6-6ce48acdd6ab","timestamp":"2026-05-13T13:44:56.960Z","cwd":"/repo","originator":"codex-tui","cli_version":"0.130.0","source":"cli","thread_source":"user","model_provider":"openai"},"git":{"commit_hash":"abc","branch":"main","repository_url":"git@example.com:repo.git"}}` + "\n")

	env, err := WrapCodexRollout(RolloutFile{
		Path:    "/home/me/.codex/sessions/2026/05/13/rollout-test.jsonl",
		Size:    int64(len(raw)),
		SHA256:  strings.Repeat("a", 64),
		Raw:     raw,
		ModTime: time.Date(2026, 5, 13, 13, 45, 0, 0, time.UTC),
	}, "host-a", "octocat")
	if err != nil {
		t.Fatalf("WrapCodexRollout: %v", err)
	}

	if env.GHUser != "octocat" {
		t.Fatalf("GHUser = %q", env.GHUser)
	}

	if env.IngestSource != "codex_rollout_jsonl" {
		t.Fatalf("IngestSource = %q", env.IngestSource)
	}
	if env.IngestAgent != "hev-kit" {
		t.Fatalf("IngestAgent = %q", env.IngestAgent)
	}
	if env.Harness != "codex_cli" {
		t.Fatalf("Harness = %q", env.Harness)
	}
	if env.HarnessVersion == nil || *env.HarnessVersion != "codex-0.130.0" {
		t.Fatalf("HarnessVersion = %v", env.HarnessVersion)
	}
	if env.SessionID != "019e2195-5b7f-7f11-84b6-6ce48acdd6ab" {
		t.Fatalf("SessionID = %q", env.SessionID)
	}
	if env.PayloadSchemaRef != "codex.rollout_jsonl.v0.130.0" {
		t.Fatalf("PayloadSchemaRef = %q", env.PayloadSchemaRef)
	}
	if env.Repo != "git@example.com:repo.git" || env.Branch != "main" {
		t.Fatalf("git metadata = %q %q", env.Repo, env.Branch)
	}
}
