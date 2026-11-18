package daemon

import (
	"encoding/json"
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

// App capture added turn_id and redaction_* to the envelope; coding-agent
// capture must still write exactly the fields it always has.
func TestCodingAgentEnvelopeWireUnchanged(t *testing.T) {
	raw := []byte(`{"timestamp":"2026-05-13T13:45:55.110Z","type":"session_meta","payload":{"id":"s","cli_version":"0.130.0"},"git":{"branch":"main","repository_url":"r"}}` + "\n")
	codex, err := WrapCodexRollout(RolloutFile{Path: "/x/rollout.jsonl", Raw: raw, ModTime: time.Now()}, "h", "u")
	if err != nil {
		t.Fatal(err)
	}
	claude, err := WrapClaudeRawBody(RawBodyFile{Path: "/x/body.json", Kind: "request", Raw: []byte(`{}`), ModTime: time.Now()}, "h", "u")
	if err != nil {
		t.Fatal(err)
	}
	for name, env := range map[string]*TraceEnvelope{"codex": codex, "claude": claude} {
		b, err := json.Marshal(env)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{"turn_id", "redaction_policy", "redaction_applied", "redaction_count"} {
			if _, ok := m[k]; ok {
				t.Errorf("%s envelope gained %s: %s", name, k, b)
			}
		}
		for _, k := range []string{"ingest_id", "ingest_ts", "ingest_source", "source_ts", "host", "harness", "harness_version", "session_id", "event_kind", "signal_type", "payload_raw", "payload_schema_ref", "resource"} {
			if _, ok := m[k]; !ok {
				t.Errorf("%s envelope lost %s: %s", name, k, b)
			}
		}
	}
}
