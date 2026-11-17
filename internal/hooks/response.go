package hooks

import (
	"encoding/json"
	"strings"
)

// Decisions, weakest first. Merge keeps the strongest.
const (
	Continue = "continue"
	Block    = "block"
	Halt     = "halt"
)

// Response is a handler's answer, and the merged answer of all handlers.
type Response struct {
	Decision string `json:"decision,omitempty"`
	Reason   string `json:"reason,omitempty"`
	// Context is injected into the model's context where the event allows.
	Context string `json:"context,omitempty"`
	// SystemMessage is shown to the person, not the model.
	SystemMessage string `json:"system_message,omitempty"`
}

func rank(d string) int {
	switch d {
	case Halt:
		return 2
	case Block:
		return 1
	}
	return 0
}

// Merge combines responses in handler order: the strongest decision wins and
// carries its reason; contexts and system messages concatenate.
func Merge(rs ...Response) Response {
	out := Response{Decision: Continue}
	var contexts, systems, reasons []string
	for _, r := range rs {
		if r.Decision == "" {
			r.Decision = Continue
		}
		if rank(r.Decision) > rank(out.Decision) {
			out.Decision, reasons = r.Decision, nil
		}
		if r.Decision == out.Decision && r.Decision != Continue && strings.TrimSpace(r.Reason) != "" {
			reasons = append(reasons, r.Reason)
		}
		if strings.TrimSpace(r.Context) != "" {
			contexts = append(contexts, r.Context)
		}
		if strings.TrimSpace(r.SystemMessage) != "" {
			systems = append(systems, r.SystemMessage)
		}
	}
	out.Reason = strings.Join(reasons, "\n\n")
	out.Context = strings.Join(contexts, "\n\n")
	out.SystemMessage = strings.Join(systems, "\n")
	return out
}

// contextEvents can carry additionalContext into the model.
var contextEvents = map[string]bool{"session-start": true, "prompt-submit": true, "post-tool": true}

// Render answers in the harness's native hook output. Claude Code and Codex
// share one shape: hookSpecificOutput.additionalContext, a PreToolUse
// permissionDecision of deny with its reason, and continue:false with a
// stopReason to end the turn. An empty result means continue. The exit code
// is always zero: exit 2 would block, and a broken kit must never block.
func Render(harness, event string, r Response) []byte {
	out := map[string]any{}
	specific := map[string]any{}
	switch r.Decision {
	case Halt:
		out["continue"] = false
		out["stopReason"] = r.Reason
		if event == "pre-tool" {
			specific["permissionDecision"] = "deny"
			specific["permissionDecisionReason"] = r.Reason
		}
	case Block:
		switch event {
		case "pre-tool":
			specific["permissionDecision"] = "deny"
			specific["permissionDecisionReason"] = r.Reason
		case "post-tool":
			// The call already ran; the reason reaches the model as feedback.
			out["decision"] = "block"
			out["reason"] = r.Reason
		}
	}
	if r.Context != "" && contextEvents[event] {
		specific["additionalContext"] = r.Context
	}
	if len(specific) > 0 {
		specific["hookEventName"] = NativeName(event)
		out["hookSpecificOutput"] = specific
	}
	if r.SystemMessage != "" {
		out["systemMessage"] = r.SystemMessage
	}
	if len(out) == 0 {
		return nil
	}
	b, _ := json.Marshal(out)
	return append(b, '\n')
}
