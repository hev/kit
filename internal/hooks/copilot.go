package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/hev/kit/internal/harness"
	"github.com/hev/kit/internal/peers/compat"
)

type copilotInput struct {
	SessionID      string          `json:"sessionId"`
	TranscriptPath string          `json:"transcriptPath"`
	Cwd            string          `json:"cwd"`
	Prompt         string          `json:"prompt"`
	Source         string          `json:"source"`
	Trigger        string          `json:"trigger"`
	StopHookActive bool            `json:"stop_hook_active"`
	ToolName       string          `json:"toolName"`
	ToolArgs       json.RawMessage `json:"toolArgs"`
	ToolResult     json.RawMessage `json:"toolResult"`
}

func copilotNative(event string, raw []byte) (native, error) {
	var c copilotInput
	if err := json.Unmarshal(raw, &c); err != nil {
		return native{}, err
	}
	// Older releases serialized tool arguments as a JSON string.
	var args string
	if json.Unmarshal(c.ToolArgs, &args) == nil && json.Valid([]byte(args)) {
		c.ToolArgs = json.RawMessage(args)
	}
	return native{
		HookEventName: nativeName(harness.Copilot, event), SessionID: c.SessionID,
		TranscriptPath: c.TranscriptPath, Cwd: c.Cwd, Prompt: c.Prompt,
		Source: c.Source, Trigger: c.Trigger, StopHookActive: c.StopHookActive,
		ToolName: c.ToolName, ToolInput: c.ToolArgs, ToolResponse: c.ToolResult,
	}, nil
}

func copilotToolKind(name, command string) string {
	switch name {
	case "bash", "powershell":
		return ClassifyShell(command)
	case "view", "grep", "rg", "glob", "web_fetch", "web_search":
		return KindRead
	case "create", "edit", "str_replace_editor":
		return KindMutate
	default:
		return Classify(name, command)
	}
}

func copilotResponse(event string, r Response) []byte {
	var out map[string]string
	switch event {
	case "pre-tool":
		if r.Decision == Block || r.Decision == Halt {
			out = map[string]string{"permissionDecision": "deny", "permissionDecisionReason": r.Reason}
		}
	case "session-start", "post-tool":
		if r.Context != "" {
			out = map[string]string{"additionalContext": r.Context}
		}
	}
	if out == nil {
		return nil
	}
	b, _ := json.Marshal(out)
	return append(b, '\n')
}

func copilotHookArgs(event string) []any {
	return []any{"hook", event, "--harness", harness.Copilot, "--installed-by", "kit"}
}

// copilotHooks manages the flat command arrays in a dedicated user-level file.
// Other hook files are left alone, as are foreign entries in this file.
func copilotHooks(home, exe, action string) (Status, int, error) {
	path := SettingsPath(home, harness.Copilot)
	status := Status{Path: path}
	if info, err := os.Stat(filepath.Dir(filepath.Dir(path))); err == nil && info.IsDir() {
		status.Present = true
	}
	settings, mode, err := load(path)
	if err != nil {
		return status, 0, err
	}
	if settings.Len() > 0 && fmt.Sprint(settings.Get("version")) != "1" {
		return status, 0, fmt.Errorf("unsupported Copilot hook configuration version")
	}
	install := action == "install"
	h, err := hooksObj(settings, install)
	if err != nil {
		return status, 0, err
	}
	if h == nil {
		for _, event := range Events {
			status.Missing = append(status.Missing, event)
		}
		return status, 0, nil
	}
	if install {
		settings.Set("version", 1)
	}
	changes := 0
	for _, event := range Events {
		name := nativeName(harness.Copilot, event)
		list, ok := h.Get(name).([]any)
		if !ok && h.Has(name) {
			return status, 0, fmt.Errorf("Copilot hooks.%s must be an array", name)
		}
		args := copilotHookArgs(event)
		want := compat.NewObj("type", "command", "exec", exe, "args", args, "timeoutSec", EntryTimeout)
		var kept []any
		found, current := 0, false
		for _, item := range list {
			obj, ok := item.(*compat.Obj)
			if !ok || !reflect.DeepEqual(obj.Get("args"), args) {
				status.Foreign++
				kept = append(kept, item)
				continue
			}
			found++
			if compat.Dumps(obj) == compat.Dumps(want) {
				current = true
			}
			if install && found == 1 {
				kept = append(kept, want)
			}
		}
		switch {
		case current:
			status.Kit = append(status.Kit, event)
		case found > 0:
			status.Stale = append(status.Stale, event)
		default:
			status.Missing = append(status.Missing, event)
		}
		switch {
		case install && (found != 1 || !current):
			if found == 0 {
				kept = append(kept, want)
			}
			h.Set(name, kept)
			changes++
		case action == "uninstall" && found > 0:
			if len(kept) == 0 {
				h.Del(name)
			} else {
				h.Set(name, kept)
			}
			changes += found
		}
	}
	if changes == 0 {
		return status, 0, nil
	}
	return status, changes, write(path, compat.DumpsIndentUnicode(settings, 2), mode)
}
