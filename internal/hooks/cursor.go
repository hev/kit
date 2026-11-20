package hooks

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/hev/kit/internal/harness"
	"github.com/hev/kit/internal/peers/compat"
)

func renderCursor(event string, r Response) []byte {
	out := map[string]any{}
	deny := r.Decision == Block || r.Decision == Halt
	switch event {
	case "prompt-submit":
		if deny {
			out["continue"] = false
			out["user_message"] = r.Reason
		}
	case "pre-tool":
		out["permission"] = "allow"
		if deny {
			out["permission"] = "deny"
			out["agent_message"] = r.Reason
			out["user_message"] = r.SystemMessage
		}
	case "pre-compact":
		if r.SystemMessage != "" {
			out["user_message"] = r.SystemMessage
		}
	case "session-start", "post-tool":
		context := r.Context
		if context == "" && deny && event == "post-tool" {
			context = r.Reason
		}
		if context != "" {
			out["additional_context"] = context
		}
	}
	b, _ := json.Marshal(out)
	return append(b, '\n')
}

func inspectCursor(s Status, h *compat.Obj, exe string) Status {
	for _, event := range Events {
		want := Command(exe, event, harness.Cursor, Stamp)
		found, current := false, false
		if h != nil {
			list, _ := h.Get(nativeName(harness.Cursor, event)).([]any)
			for _, entry := range list {
				if o, kit := isKit(entry); kit {
					found = true
					current = current || o.Get("command") == want
				} else {
					s.Foreign++
				}
			}
		}
		switch {
		case current:
			s.Kit = append(s.Kit, event)
		case found:
			s.Stale = append(s.Stale, event)
		default:
			s.Missing = append(s.Missing, event)
		}
	}
	return s
}

func installCursor(path, exe string, settings *compat.Obj, mode os.FileMode) (bool, error) {
	before := compat.DumpsIndentUnicode(settings, 2)
	if version, ok := settings.Num("version"); settings.Get("version") != nil && (!ok || version != 1) {
		return false, fmt.Errorf("unsupported Cursor hooks version")
	}
	if settings.Get("version") == nil {
		settings.Set("version", 1)
	}
	h, err := hooksObj(settings, true)
	if err != nil {
		return false, err
	}
	for _, event := range Events {
		name := nativeName(harness.Cursor, event)
		list, ok := h.Get(name).([]any)
		if !ok && h.Get(name) != nil {
			return false, fmt.Errorf("Cursor hooks.%s is not a list", name)
		}
		entry := compat.NewObj("command", Command(exe, event, harness.Cursor, Stamp), "timeout", EntryTimeout)
		out := make([]any, 0, len(list)+1)
		placed := false
		for _, existing := range list {
			if _, kit := isKit(existing); !kit {
				out = append(out, existing)
			} else if !placed {
				out = append(out, entry)
				placed = true
			}
		}
		if !placed {
			out = append(out, entry)
		}
		h.Set(name, out)
	}
	after := compat.DumpsIndentUnicode(settings, 2)
	if before == after {
		return false, nil
	}
	return true, write(path, after, mode)
}

func uninstallCursor(path string, settings, h *compat.Obj, mode os.FileMode) (int, error) {
	removed := 0
	for _, event := range Events {
		name := nativeName(harness.Cursor, event)
		list, _ := h.Get(name).([]any)
		kept := make([]any, 0, len(list))
		for _, entry := range list {
			if _, kit := isKit(entry); kit {
				removed++
			} else {
				kept = append(kept, entry)
			}
		}
		if len(kept) != len(list) {
			if len(kept) == 0 {
				h.Del(name)
			} else {
				h.Set(name, kept)
			}
		}
	}
	if removed == 0 {
		return 0, nil
	}
	return removed, write(path, compat.DumpsIndentUnicode(settings, 2), mode)
}
