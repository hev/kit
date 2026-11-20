// Package hooks is kit's single harness hook layer (RFC 0013): it normalizes a
// harness's hook JSON to one versioned event, runs handlers under a budget
// that always fails open, and answers in the harness's native shape.
package hooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hev/kit/internal/harness"
	"github.com/oklog/ulid/v2"
)

// Schema names the normalized event version (schemas/hook_event.schema.json).
const Schema = "hev.hook.event/v1"

// Events are kit's lifecycle names, in the order a session meets them.
var Events = []string{"session-start", "prompt-submit", "pre-tool", "post-tool", "stop", "pre-compact"}

// Harnesses with a verified hook mechanism.
var Harnesses = hookHarnesses()

func hookHarnesses() []string {
	var names []string
	for _, d := range harness.All() {
		if d.Hooks != nil {
			names = append(names, d.Name)
		}
	}
	return names
}

// NativeName is the harness's name for a kit event.
func NativeName(event string) string { return harness.NativeHookEvent(event) }

// ValidEvent reports whether event is a kit hook event.
func ValidEvent(event string) bool { return NativeName(event) != "" }

// ValidHarness reports whether h has a verified hook mechanism.
func ValidHarness(h string) bool { return harness.HooksFor(h) != nil }

// Tool kinds. Only mutate is ever held by an urgent steer.
const (
	KindRead   = "read"
	KindMutate = "mutate"
	KindSteer  = "steer"
	KindOther  = "other"
)

// Tool is the call a pre-tool or post-tool event is about.
type Tool struct {
	Name          string          `json:"name"`
	UseID         string          `json:"use_id,omitempty"`
	Kind          string          `json:"kind"`
	Command       string          `json:"command,omitempty"`
	Input         json.RawMessage `json:"input,omitempty"`
	Response      json.RawMessage `json:"response,omitempty"`
	InputBytes    int             `json:"input_bytes,omitempty"`
	ResponseBytes int             `json:"response_bytes,omitempty"`
}

// Event is one normalized hook event. Handlers receive it whole; the event
// log keeps a redacted copy (see Redacted).
type Event struct {
	Schema         string    `json:"schema"`
	ID             string    `json:"id"`
	Time           time.Time `json:"time"`
	Event          string    `json:"event"`
	Harness        string    `json:"harness"`
	NativeEvent    string    `json:"native_event,omitempty"`
	SessionID      string    `json:"session_id,omitempty"`
	TurnID         string    `json:"turn_id,omitempty"`
	TranscriptPath string    `json:"transcript_path,omitempty"`
	Cwd            string    `json:"cwd,omitempty"`
	Model          string    `json:"model,omitempty"`
	Prompt         string    `json:"prompt,omitempty"`
	PromptBytes    int       `json:"prompt_bytes,omitempty"`
	// Source is session-start's source or pre-compact's trigger.
	Source         string `json:"source,omitempty"`
	StopHookActive bool   `json:"stop_hook_active,omitempty"`
	Tool           *Tool  `json:"tool,omitempty"`
	// Target is the steering target the hook resolved, when it found one.
	Target string `json:"target,omitempty"`
}

// native is the union of the hook input fields kit reads.
type native struct {
	SessionID      string          `json:"session_id"`
	TurnID         string          `json:"turn_id"`
	TranscriptPath string          `json:"transcript_path"`
	Cwd            string          `json:"cwd"`
	HookEventName  string          `json:"hook_event_name"`
	Model          string          `json:"model"`
	Prompt         string          `json:"prompt"`
	Source         string          `json:"source"`
	Trigger        string          `json:"trigger"`
	StopHookActive bool            `json:"stop_hook_active"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
	ToolResponse   json.RawMessage `json:"tool_response"`
	ToolUseID      string          `json:"tool_use_id"`
}

// Normalize turns a harness's hook JSON into an Event. The event argument is
// authoritative: a harness that names it differently is still answered in
// the shape the installed hook asked for.
func Normalize(harness, event string, raw []byte, now time.Time) (Event, error) {
	if !ValidHarness(harness) {
		return Event{}, fmt.Errorf("unknown harness %q", harness)
	}
	if nativeName(harness, event) == "" {
		return Event{}, fmt.Errorf("unknown event %q", event)
	}
	ev := Event{Schema: Schema, ID: strings.ToLower(ulid.Make().String()), Time: now.UTC(), Event: event, Harness: harness}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ev, fmt.Errorf("empty hook input")
	}
	var n native
	if err := json.Unmarshal(raw, &n); err != nil {
		return ev, fmt.Errorf("parse %s hook input: %w", harness, err)
	}
	ev.NativeEvent = n.HookEventName
	ev.SessionID, ev.TurnID, ev.TranscriptPath, ev.Cwd, ev.Model = n.SessionID, n.TurnID, n.TranscriptPath, n.Cwd, n.Model
	switch event {
	case "prompt-submit":
		ev.Prompt, ev.PromptBytes = n.Prompt, len(n.Prompt)
	case "session-start":
		ev.Source = n.Source
	case "pre-compact":
		ev.Source = n.Trigger
	case "stop":
		ev.StopHookActive = n.StopHookActive
	case "pre-tool", "post-tool":
		t := &Tool{Name: n.ToolName, UseID: n.ToolUseID, InputBytes: len(n.ToolInput)}
		if len(n.ToolInput) > 0 && string(n.ToolInput) != "null" {
			t.Input = n.ToolInput
		}
		if event == "post-tool" && len(n.ToolResponse) > 0 && string(n.ToolResponse) != "null" {
			t.Response, t.ResponseBytes = n.ToolResponse, len(n.ToolResponse)
		}
		t.Command = toolCommand(t.Input)
		t.Kind = Classify(t.Name, t.Command)
		if harness == "droid" {
			switch t.Name {
			case "Execute":
				t.Kind = ClassifyShell(t.Command)
			case "Create", "ApplyPatch":
				t.Kind = KindMutate
			case "FetchUrl":
				t.Kind = KindRead
			}
		}
		ev.Tool = t
	}
	return ev, nil
}

// toolCommand is a shell tool's command line: a string, or Codex's argv list.
func toolCommand(input json.RawMessage) string {
	var v struct {
		Command json.RawMessage `json:"command"`
		Cmd     json.RawMessage `json:"cmd"`
	}
	if json.Unmarshal(input, &v) != nil {
		return ""
	}
	for _, raw := range []json.RawMessage{v.Command, v.Cmd} {
		var s string
		if json.Unmarshal(raw, &s) == nil && s != "" {
			return s
		}
		var argv []string
		if json.Unmarshal(raw, &argv) == nil && len(argv) > 0 {
			// ["bash","-lc","git push"] runs its last word.
			if len(argv) >= 3 && (argv[1] == "-lc" || argv[1] == "-c") {
				return argv[len(argv)-1]
			}
			return strings.Join(argv, " ")
		}
	}
	return ""
}

// Redacted is the copy kept in the event log: sizes, not content. Prompts,
// tool inputs, commands and tool output can carry secrets.
func (e Event) Redacted() Event {
	r := e
	r.Prompt = ""
	if e.Tool != nil {
		t := *e.Tool
		t.Input, t.Response, t.Command = nil, nil, ""
		r.Tool = &t
	}
	return r
}

func nativeName(name, event string) string {
	if h := harness.HooksFor(name); h != nil {
		return h.NativeEvent(event)
	}
	return ""
}
