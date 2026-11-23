package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hev/kit/internal/harness"
	"github.com/hev/kit/internal/peers/compat"
)

// Stamp ends every command kit installs. Check and uninstall touch only
// entries carrying it, so a person's own hooks are never rewritten.
const Stamp = "--installed-by kit"

// EntryTimeout is the harness-side limit (seconds) on one kit hook. Kit's own
// budget is far shorter; this only bounds a wedged process, and a harness
// treats a timed-out hook as a non-blocking error.
const EntryTimeout = 10

// SettingsPath is the file a harness reads global hooks from.
func SettingsPath(home, name string) string {
	if capability := harness.HooksFor(name); capability != nil {
		return capability.SettingsPath(home)
	}
	return ""
}

// HarnessHome is the directory whose presence means the harness is set up.
func HarnessHome(home, harness string) string {
	path := SettingsPath(home, harness)
	if path == "" {
		return ""
	}
	if harness == "grok" {
		return filepath.Dir(filepath.Dir(path))
	}
	return filepath.Dir(path)
}

// Command is the shim a harness runs for one event.
func Command(exe, event, harness string, extra ...string) string {
	words := append([]string{shellQuote(exe), "hook", event, "--harness", harness}, extra...)
	return strings.Join(words, " ")
}

func shellQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n'\"\\$`;&|<>()*?[]{}!#~") {
		return s
	}
	if runtime.GOOS == "windows" {
		return `"` + s + `"`
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// InvocationHooks is the hooks object a supervisor passes for one launch
// (Claude --settings, Codex -c hooks.EVENT=...). It carries no stamp: these
// entries are never written to a settings file.
func InvocationHooks(exe, harness string) *compat.Obj {
	hooks := compat.NewObj()
	for _, ev := range Events {
		name := nativeName(harness, ev)
		if name == "" {
			continue
		}
		hooks.Set(name, []any{compat.NewObj("hooks", []any{compat.NewObj("type", "command", "command", Command(exe, ev, harness), "timeout", EntryTimeout)})})
	}
	return hooks
}

// CodexInvocationArgs are the codex flags that wire kit's hooks for one run.
// Codex runs a configured hook only after a person trusts it or when the
// invocation bypasses trust; kit vouches for its own command here.
func CodexInvocationArgs(exe string) []string { return ConfigInvocationArgs(exe, "codex") }

func ConfigInvocationArgs(exe, harness string) []string {
	args := []string{"--dangerously-bypass-hook-trust"}
	for _, ev := range Events {
		name := nativeName(harness, ev)
		if name == "" {
			continue
		}
		entry := `[{ hooks = [{ type = "command", command = ` + compat.Dumps(Command(exe, ev, harness)) + `, timeout = ` + fmt.Sprint(EntryTimeout) + ` }] }]`
		args = append(args, "-c", "hooks."+name+"="+entry)
	}
	return args
}

// Status is what check finds in one settings file.
type Status struct {
	Path    string
	Present bool     // the harness home exists
	Kit     []string // events with a kit entry for this binary
	Missing []string // events without a kit entry
	Stale   []string // events whose kit entry runs another binary or an old shape
	Foreign int      // hooks that are not kit's, left alone
}

// Current reports whether every event has a current kit entry.
func (s Status) Current() bool { return len(s.Missing) == 0 && len(s.Stale) == 0 }

func isKit(h any) (*compat.Obj, bool) {
	o, ok := h.(*compat.Obj)
	if !ok {
		return nil, false
	}
	cmd, _ := o.Get("command").(string)
	return o, strings.Contains(cmd, " hook ") && strings.HasSuffix(strings.TrimSpace(cmd), Stamp)
}

func load(path string) (*compat.Obj, os.FileMode, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return compat.NewObj(), 0600, nil
	}
	if err != nil {
		return nil, 0, err
	}
	mode := os.FileMode(0600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if strings.TrimSpace(string(b)) == "" {
		return compat.NewObj(), mode, nil
	}
	o, err := compat.LoadObj(b)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w; not touching it", path, err)
	}
	return o, mode, nil
}

func hooksObj(settings *compat.Obj, create bool) (*compat.Obj, error) {
	v := settings.Get("hooks")
	if v == nil {
		if !create {
			return nil, nil
		}
		h := compat.NewObj()
		settings.Set("hooks", h)
		return h, nil
	}
	h, ok := v.(*compat.Obj)
	if !ok {
		return nil, fmt.Errorf("hooks is not an object; not touching it")
	}
	return h, nil
}

// Inspect reports kit's entries in path without changing anything.
func Inspect(home, harness, exe string) (Status, error) {
	path := SettingsPath(home, harness)
	if path == "" {
		return Status{}, fmt.Errorf("unknown hook harness %q", harness)
	}
	s := Status{Path: path}
	if info, err := os.Stat(HarnessHome(home, harness)); err == nil && info.IsDir() {
		s.Present = true
	}
	settings, _, err := load(path)
	if err != nil {
		return s, err
	}
	h, err := hooksObj(settings, false)
	if err != nil {
		return s, err
	}
	for _, ev := range Events {
		name := nativeName(harness, ev)
		if name == "" {
			continue
		}
		want := Command(exe, ev, harness, Stamp)
		found, current := false, false
		if h != nil {
			groups, _ := h.Get(name).([]any)
			for _, g := range groups {
				gobj, ok := g.(*compat.Obj)
				if !ok {
					continue
				}
				list, _ := gobj.Get("hooks").([]any)
				for _, hk := range list {
					if o, kit := isKit(hk); kit {
						found = true
						current = current || o.Get("command") == want
					} else {
						s.Foreign++
					}
				}
			}
		}
		switch {
		case current:
			s.Kit = append(s.Kit, ev)
		case found:
			s.Stale = append(s.Stale, ev)
		default:
			s.Missing = append(s.Missing, ev)
		}
	}
	return s, nil
}

// Install merges kit's entries into the harness settings: a stale kit entry
// is replaced in place, a missing one appended, and everything else kept.
// It writes nothing when the file is already current.
func Install(home, harness, exe string) (bool, error) {
	path := SettingsPath(home, harness)
	if path == "" {
		return false, fmt.Errorf("unknown hook harness %q", harness)
	}
	settings, mode, err := load(path)
	if err != nil {
		return false, err
	}
	before := compat.DumpsIndentUnicode(settings, 2)
	h, err := hooksObj(settings, true)
	if err != nil {
		return false, err
	}
	for _, ev := range Events {
		name := nativeName(harness, ev)
		if name == "" {
			continue
		}
		entry := compat.NewObj("type", "command", "command", Command(exe, ev, harness, Stamp), "timeout", EntryTimeout)
		var groups []any
		if v := h.Get(name); v != nil {
			var ok bool
			if groups, ok = v.([]any); !ok {
				return false, fmt.Errorf("%s: hooks.%s is not a list; not touching it", path, name)
			}
		}
		placed := false
		var out []any
		for _, g := range groups {
			gobj, ok := g.(*compat.Obj)
			if !ok {
				out = append(out, g)
				continue
			}
			list, _ := gobj.Get("hooks").([]any)
			var kept []any
			kitHere := false
			for _, hk := range list {
				if _, kit := isKit(hk); kit {
					kitHere = true
					if !placed {
						kept = append(kept, entry)
						placed = true
					}
					continue
				}
				kept = append(kept, hk)
			}
			if kitHere && len(kept) == 0 {
				continue
			}
			if kitHere {
				gobj.Set("hooks", kept)
			}
			out = append(out, gobj)
		}
		if !placed {
			out = append(out, compat.NewObj("hooks", []any{entry}))
		}
		h.Set(name, out)
	}
	after := compat.DumpsIndentUnicode(settings, 2)
	if after == before {
		return false, nil
	}
	return true, write(path, after, mode)
}

// Uninstall removes only kit's entries, and the groups and event lists they
// leave empty. It reports how many entries it removed.
func Uninstall(home, harness string) (int, error) {
	path := SettingsPath(home, harness)
	if path == "" {
		return 0, fmt.Errorf("unknown hook harness %q", harness)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return 0, nil
	}
	settings, mode, err := load(path)
	if err != nil {
		return 0, err
	}
	h, err := hooksObj(settings, false)
	if err != nil || h == nil {
		return 0, err
	}
	removed := 0
	for _, name := range h.Keys() {
		groups, ok := h.Get(name).([]any)
		if !ok {
			continue
		}
		var out []any
		for _, g := range groups {
			gobj, ok := g.(*compat.Obj)
			if !ok {
				out = append(out, g)
				continue
			}
			list, _ := gobj.Get("hooks").([]any)
			var kept []any
			for _, hk := range list {
				if _, kit := isKit(hk); kit {
					removed++
					continue
				}
				kept = append(kept, hk)
			}
			if len(kept) == 0 && len(list) > 0 {
				continue
			}
			if len(kept) != len(list) {
				gobj.Set("hooks", kept)
			}
			out = append(out, gobj)
		}
		if len(out) == 0 && len(groups) > 0 {
			h.Del(name)
		} else if len(out) != len(groups) {
			h.Set(name, out)
		}
	}
	if removed == 0 {
		return 0, nil
	}
	if h.Len() == 0 {
		settings.Del("hooks")
	}
	return removed, write(path, compat.DumpsIndentUnicode(settings, 2), mode)
}

func write(path, body string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if old, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".before-hev-hooks", old, mode); err != nil {
			return err
		}
	}
	tmp := path + ".hev-tmp"
	if err := os.WriteFile(tmp, []byte(body+"\n"), mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// CodexTrusted reports whether Codex has a persisted trust record for kit's
// global hooks in path. Codex skips untrusted global hooks until a person
// reviews them (/hooks); kit never writes that record itself.
func CodexTrusted(home, hooksPath string) bool {
	dir := filepath.Dir(hooksPath)
	b, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	if err != nil {
		return false
	}
	return strings.Contains(string(b), hooksPath+":pre_tool_use:")
}
