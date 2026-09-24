package store

import (
	"encoding/json"
	"testing"

	"github.com/hev/kit/internal/daemon"
)

func TestDecodeCodexRolloutEnvelope(t *testing.T) {
	payload := "" +
		`{"timestamp":"2026-05-13T13:45:55.110Z","type":"session_meta","payload":{"id":"sess-123","cli_version":"0.130.0"}}` + "\n" +
		`{"timestamp":"2026-05-13T13:45:56.110Z","type":"event_msg","payload":{"type":"user_message","message":"build the thing"}}` + "\n" +
		`{"timestamp":"2026-05-13T13:45:57.110Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1000,"output_tokens":200,"reasoning_output_tokens":50}}}}` + "\n"

	version := "codex-0.130.0"
	env := daemon.TraceEnvelope{
		SourceTS:       "2026-05-13T13:45:55.110Z",
		Harness:        "codex_cli",
		HarnessVersion: &version,
		SessionID:      "sess-123",
		PayloadRaw:     payload,
	}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}

	sess, events, err := decodeCodexRolloutEnvelope(data, "2026-05-13")
	if err != nil {
		t.Fatalf("decodeCodexRolloutEnvelope: %v", err)
	}
	if sess.ID != "sess-123" {
		t.Fatalf("ID = %q", sess.ID)
	}
	if sess.Harness != "codex_cli" || sess.Service != "codex 0.130.0" {
		t.Fatalf("harness/service = %q/%q", sess.Harness, sess.Service)
	}
	if sess.FirstPrompt != "build the thing" || sess.PromptCount != 1 {
		t.Fatalf("prompt = %q count=%d", sess.FirstPrompt, sess.PromptCount)
	}
	if sess.InputTokens != 1000 || sess.OutputTokens != 250 {
		t.Fatalf("tokens = %d/%d", sess.InputTokens, sess.OutputTokens)
	}
	if len(events) != 3 || sess.EventCount != 3 {
		t.Fatalf("events = %d/%d", len(events), sess.EventCount)
	}
}
