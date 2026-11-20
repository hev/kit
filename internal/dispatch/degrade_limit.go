package dispatch

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// LimitHit is positive evidence that a harness stopped because its plan is
// out of usage. Nothing else is a limit: a crash, a failed test, a timeout or
// a 529 overload are ordinary failures.
type LimitHit struct {
	Harness string `json:"harness"`
	// Kind names the evidence: usage_limit_reached (Claude apiError),
	// rate_limit_rejected (a Claude 429 rate_limit_error whose
	// rate_limit_info.status is rejected), limit_message (Claude's "You've
	// hit your … limit" error message), or usage_limit_exceeded (Codex).
	Kind string `json:"kind"`
	// LimitType is Claude's rateLimitType (seven_day_overage_included, …).
	LimitType string    `json:"limit_type,omitempty"`
	ResetsAt  time.Time `json:"resets_at,omitempty"`
	Message   string    `json:"message,omitempty"`
}

// Claude's plan-limit message: "You've hit your Fable limit · resets …",
// "You've hit your session limit · …". Only error records are read.
var claudeLimitMessage = regexp.MustCompile(`^You(?:'|’)ve hit your [^.\n]*limit`)

// Codex's usage-limit message prefix (codex-rs UsageLimitReachedError;
// the strings are in the 0.170.0 binary).
var codexLimitMessage = regexp.MustCompile(`^You(?:'|’)ve hit your usage limit`)

// "try again at Sep 19th, 2026 2:12 AM" (no zone: the host's local time).
var codexRetryAt = regexp.MustCompile(`try again at ([A-Z][a-z]{2}) (\d{1,2})(?:st|nd|rd|th), (\d{4}) (\d{1,2}:\d{2} [AP]M)`)

// ClassifyLimit reads a harness's JSON lines (Claude stream-json or
// transcript, Codex exec --json or a rollout) and returns the last plan-limit
// record, or nil when there is none. It reads only top-level error records,
// so a limit message quoted in tool output never counts.
func ClassifyLimit(harness string, data []byte) *LimitHit {
	var hit *LimitHit
	var rejected *rateLimitInfo
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64<<10), 16<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var rec map[string]json.RawMessage
		if json.Unmarshal(line, &rec) != nil {
			continue
		}
		var h *LimitHit
		if harness == "codex" {
			h = codexLimit(rec)
		} else {
			h, rejected = claudeLimit(rec, rejected)
		}
		if h != nil {
			hit = h
		}
	}
	return hit
}

type rateLimitInfo struct {
	Status        string `json:"status"`
	ResetsAt      int64  `json:"resetsAt"`
	RateLimitType string `json:"rateLimitType"`
}

func str(rec map[string]json.RawMessage, keys ...string) string {
	for _, k := range keys {
		var s string
		if json.Unmarshal(rec[k], &s) == nil && s != "" {
			return s
		}
	}
	return ""
}

func num(rec map[string]json.RawMessage, keys ...string) int {
	for _, k := range keys {
		var n int
		if json.Unmarshal(rec[k], &n) == nil && n != 0 {
			return n
		}
	}
	return 0
}

func boolean(rec map[string]json.RawMessage, keys ...string) bool {
	for _, k := range keys {
		var b bool
		if json.Unmarshal(rec[k], &b) == nil && b {
			return true
		}
	}
	return false
}

// claudeLimit reads one Claude Code record. Transcripts spell the error
// fields apiError/apiErrorParams; stream-json spells them api_error/
// api_error_params. A rejected rate_limit_event carries the window that a
// following error record may omit.
func claudeLimit(rec map[string]json.RawMessage, rejected *rateLimitInfo) (*LimitHit, *rateLimitInfo) {
	kind := str(rec, "type")
	if kind == "rate_limit_event" {
		var info rateLimitInfo
		if json.Unmarshal(rec["rate_limit_info"], &info) == nil && info.Status == "rejected" {
			return nil, &info
		}
		return nil, rejected
	}
	var params struct {
		Info *rateLimitInfo `json:"rate_limit_info"`
	}
	if raw, ok := rec["apiErrorParams"]; ok {
		_ = json.Unmarshal(raw, &params)
	} else if raw, ok := rec["api_error_params"]; ok {
		_ = json.Unmarshal(raw, &params)
	}
	info := params.Info
	if info == nil {
		info = rejected
	}
	apiErr := str(rec, "apiError", "api_error")
	status := num(rec, "apiErrorStatus", "api_error_status")
	errType := str(rec, "apiErrorType", "api_error_type")
	text := claudeErrorText(rec, kind)
	hit := &LimitHit{Harness: "claude", Message: text}
	switch {
	case apiErr == "usage_limit_reached":
		hit.Kind = "usage_limit_reached"
	case status == 429 && errType == "rate_limit_error" && params.Info != nil && params.Info.Status == "rejected":
		hit.Kind = "rate_limit_rejected"
	case text != "" && claudeLimitMessage.MatchString(text):
		hit.Kind = "limit_message"
	default:
		return nil, rejected
	}
	if info != nil {
		hit.LimitType = info.RateLimitType
		if info.ResetsAt > 0 {
			hit.ResetsAt = time.Unix(info.ResetsAt, 0).UTC()
		}
	}
	return hit, rejected
}

// claudeErrorText is an error record's message: the synthetic assistant
// message of an API error, or a result marked is_error.
func claudeErrorText(rec map[string]json.RawMessage, kind string) string {
	switch kind {
	case "assistant":
		if !boolean(rec, "isApiErrorMessage", "is_api_error_message") && str(rec, "error") == "" {
			return ""
		}
		var msg struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		_ = json.Unmarshal(rec["message"], &msg)
		for _, c := range msg.Content {
			if c.Type == "text" {
				return strings.TrimSpace(c.Text)
			}
		}
	case "result":
		if boolean(rec, "is_error") {
			return strings.TrimSpace(str(rec, "result"))
		}
	}
	return ""
}

// codexLimit reads one Codex record. codex exec --json reports a failed turn
// as {"type":"error","message":…} and {"type":"turn.failed","error":
// {"message":…}}; a rollout's event_msg task_complete or error carries
// codex_error_info "usage_limit_exceeded" beside the same message.
func codexLimit(rec map[string]json.RawMessage) *LimitHit {
	kind := str(rec, "type")
	var message, info string
	switch kind {
	case "error":
		message = str(rec, "message")
	case "turn.failed":
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(rec["error"], &e)
		message = e.Message
	case "event_msg":
		var p struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			Info    string `json:"codex_error_info"`
			Error   *struct {
				Message string `json:"message"`
				Info    string `json:"codex_error_info"`
			} `json:"error"`
		}
		if json.Unmarshal(rec["payload"], &p) != nil {
			return nil
		}
		switch {
		case p.Type == "error":
			message, info = p.Message, p.Info
		case p.Type == "task_complete" && p.Error != nil:
			message, info = p.Error.Message, p.Error.Info
		}
	default:
		return nil
	}
	message = strings.TrimSpace(message)
	if info != "usage_limit_exceeded" && info != "usage_limit" && !codexLimitMessage.MatchString(message) {
		return nil
	}
	hit := &LimitHit{Harness: "codex", Kind: "usage_limit_exceeded", Message: message}
	if m := codexRetryAt.FindStringSubmatch(message); m != nil {
		if t, err := time.ParseInLocation("Jan 2 2006 3:04 PM", m[1]+" "+m[2]+" "+m[3]+" "+m[4], time.Local); err == nil {
			hit.ResetsAt = t.UTC()
		}
	}
	return hit
}

// limitFromRun classifies one launch: its slice of the run log and, for
// Codex, the session's rollout file, which carries codex_error_info. A
// launch that timed out or was stopped or halted is never a limit. The
// harness's own error records are the evidence, so the exit code is not
// consulted except to read a Codex rollout only after a failed exit.
func limitFromRun(harness string, log []byte, exitCode int, timeout, stopped bool, session string) *LimitHit {
	if timeout || stopped {
		return nil
	}
	if hit := ClassifyLimit(harness, log); hit != nil {
		return hit
	}
	if harness == "codex" && session != "" && exitCode != 0 {
		if b := codexRollout(session); b != nil {
			return ClassifyLimit("codex", b)
		}
	}
	return nil
}

// codexRollout reads a Codex session's rollout from $CODEX_HOME/sessions
// (default ~/.codex), or nil.
func codexRollout(session string) []byte {
	if !regexp.MustCompile(`^[A-Za-z0-9-]+$`).MatchString(session) {
		return nil
	}
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		home = Expand("~/.codex")
	}
	paths, _ := filepath.Glob(filepath.Join(home, "sessions", "*", "*", "*", "rollout-*"+session+".jsonl"))
	if len(paths) == 0 {
		return nil
	}
	b, err := os.ReadFile(paths[len(paths)-1])
	if err != nil {
		return nil
	}
	return b
}
