package store

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/minio/minio-go/v7"
)

// CodexRolloutEvent is one decoded line from a Codex CLI rollout JSONL payload.
type CodexRolloutEvent struct {
	Timestamp   time.Time
	Type        string
	PayloadType string
	Role        string
	Payload     map[string]any
	Raw         json.RawMessage
}

// ListCodexRolloutSessions scans raw Codex rollout envelopes and aggregates
// each object into one CLI-visible session row.
func (s *Store) ListCodexRolloutSessions(ctx context.Context, date string) ([]OTLPSession, error) {
	prefix := fmt.Sprintf("raw/dt=%s/source=codex-rollouts/", date)
	var sessions []OTLPSession

	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("list codex rollouts: %w", obj.Err)
		}
		if !strings.HasSuffix(obj.Key, ".json") {
			continue
		}
		data, err := s.fetchObject(ctx, obj.Key)
		if err != nil {
			continue
		}
		sess, _, err := decodeCodexRolloutEnvelope(data, date)
		if err != nil || sess.ID == "" {
			continue
		}
		sessions = append(sessions, sess)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].FirstTS.Before(sessions[j].FirstTS)
	})
	return sessions, nil
}

// GetCodexRolloutSession returns the matched Codex rollout session and its
// decoded events for any session whose ID has the given prefix on the given date.
func (s *Store) GetCodexRolloutSession(ctx context.Context, date, idPrefix string) (OTLPSession, []CodexRolloutEvent, error) {
	prefix := fmt.Sprintf("raw/dt=%s/source=codex-rollouts/", date)
	var (
		matched string
		sessOut OTLPSession
		events  []CodexRolloutEvent
	)

	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return OTLPSession{}, nil, fmt.Errorf("list codex rollouts: %w", obj.Err)
		}
		if !strings.HasSuffix(obj.Key, ".json") {
			continue
		}
		data, err := s.fetchObject(ctx, obj.Key)
		if err != nil {
			continue
		}
		sess, evts, err := decodeCodexRolloutEnvelope(data, date)
		if err != nil || sess.ID == "" {
			continue
		}
		if !strings.HasPrefix(sess.ID, idPrefix) && !strings.HasPrefix(path.Base(sess.ID), idPrefix) {
			continue
		}
		if matched == "" {
			matched = sess.ID
			sessOut = sess
			events = evts
			continue
		}
		if matched != sess.ID {
			return OTLPSession{}, nil, fmt.Errorf("prefix %q matches multiple codex sessions (%s, %s)", idPrefix, matched, sess.ID)
		}
	}

	if matched == "" {
		return OTLPSession{}, nil, fmt.Errorf("no Codex rollout matching %q on %s", idPrefix, date)
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
	return sessOut, events, nil
}

func decodeCodexRolloutEnvelope(data []byte, date string) (OTLPSession, []CodexRolloutEvent, error) {
	var env daemon.TraceEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return OTLPSession{}, nil, err
	}

	events, err := decodeCodexRolloutEvents([]byte(env.PayloadRaw))
	if err != nil {
		return OTLPSession{}, nil, err
	}

	sess := OTLPSession{
		ID:         env.SessionID,
		Date:       date,
		Service:    codexService(env),
		Harness:    "codex_cli",
		GHUser:     env.GHUser,
		EventCount: len(events),
	}
	if ts, err := time.Parse(time.RFC3339Nano, env.SourceTS); err == nil {
		sess.FirstTS = ts
		sess.LastTS = ts
	}

	for _, evt := range events {
		if !evt.Timestamp.IsZero() {
			if sess.FirstTS.IsZero() || evt.Timestamp.Before(sess.FirstTS) {
				sess.FirstTS = evt.Timestamp
			}
			if evt.Timestamp.After(sess.LastTS) {
				sess.LastTS = evt.Timestamp
			}
		}
		switch {
		case evt.Type == "session_meta":
			if sess.ID == "" {
				sess.ID = mapString(evt.Payload, "id")
			}
			if sess.Service == "codex" {
				if version := mapString(evt.Payload, "cli_version"); version != "" {
					sess.Service = "codex " + version
				}
			}
		case evt.Type == "turn_context":
			if sess.Model == "" {
				sess.Model = mapString(evt.Payload, "model")
			}
		case evt.Type == "event_msg" && evt.PayloadType == "user_message":
			sess.PromptCount++
			if sess.FirstPrompt == "" {
				sess.FirstPrompt = truncate(mapString(evt.Payload, "message"), 60)
			}
		case evt.Type == "event_msg" && evt.PayloadType == "token_count":
			if usage := codexTokenUsage(evt.Payload); usage != nil {
				sess.InputTokens = attrInt(usage, "input_tokens")
				sess.OutputTokens = attrInt(usage, "output_tokens") + attrInt(usage, "reasoning_output_tokens")
			}
		}
	}

	return sess, events, nil
}

func decodeCodexRolloutEvents(raw []byte) ([]CodexRolloutEvent, error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)

	var events []CodexRolloutEvent
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var rec struct {
			Timestamp string          `json:"timestamp"`
			Type      string          `json:"type"`
			Payload   json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}

		payload := map[string]any{}
		if len(rec.Payload) > 0 {
			_ = json.Unmarshal(rec.Payload, &payload)
		}
		evt := CodexRolloutEvent{
			Type:        rec.Type,
			PayloadType: mapString(payload, "type"),
			Role:        mapString(payload, "role"),
			Payload:     payload,
			Raw:         append(json.RawMessage(nil), line...),
		}
		if ts, err := time.Parse(time.RFC3339Nano, rec.Timestamp); err == nil {
			evt.Timestamp = ts
		}
		events = append(events, evt)
	}

	return events, scanner.Err()
}

func codexService(env daemon.TraceEnvelope) string {
	if env.HarnessVersion == nil || *env.HarnessVersion == "" {
		return "codex"
	}
	return strings.Replace(*env.HarnessVersion, "codex-", "codex ", 1)
}

func codexTokenUsage(payload map[string]any) map[string]any {
	info := mapAny(payload["info"])
	if info == nil {
		return nil
	}
	if usage := mapAny(info["total_token_usage"]); usage != nil {
		return usage
	}
	return mapAny(info["last_token_usage"])
}

func mapAny(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func mapString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
