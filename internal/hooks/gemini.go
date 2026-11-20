package hooks

import (
	"encoding/json"

	"github.com/hev/kit/internal/harness"
)

func isGemini(name string) bool { return name == harness.Gemini }

func renderGemini(event string, r Response) []byte {
	out := map[string]any{}
	if r.SystemMessage != "" {
		out["systemMessage"] = r.SystemMessage
	}
	if r.Context != "" && contextEvents[event] {
		out["hookSpecificOutput"] = map[string]string{"additionalContext": r.Context}
	}
	switch event {
	case "prompt-submit", "pre-tool", "post-tool":
		if r.Decision == Block || r.Decision == Halt {
			reason := r.Reason
			if reason == "" {
				reason = "Blocked by kit"
			}
			out["decision"], out["reason"] = "deny", reason
		}
		if r.Decision == Halt {
			out["continue"], out["stopReason"] = false, out["reason"]
		}
	case "stop":
		// Denying AfterAgent asks Gemini to retry the turn.
		if r.Decision == Halt {
			out["continue"], out["stopReason"] = false, r.Reason
		}
	}
	b, _ := json.Marshal(out)
	return append(b, '\n')
}
