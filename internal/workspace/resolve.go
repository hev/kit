package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hev/kit/internal/hostlabel"
)

type Selection struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Workspace
}

// HostHomeEnv carries the host home into a harness with a private HOME.
const HostHomeEnv = "HEV_WORKSPACE_HOST_HOME"

func HostHome() string {
	if home := os.Getenv(HostHomeEnv); filepath.IsAbs(home) {
		return home
	}
	home, _ := os.UserHomeDir()
	return home
}

func ConfigPath() string {
	if p := os.Getenv("HEV_CONFIG"); p != "" {
		return p
	}
	return Expand("~/.hev/config.toml")
}

func Resolve(flagValue, cwd string) (Selection, error) {
	cfg, err := Read(ConfigPath())
	if err != nil {
		return Selection{}, err
	}
	return cfg.Resolve(flagValue, cwd)
}

func (c Config) Select(name, source string) (Selection, error) {
	w, ok := c.Workspaces[name]
	if !ok || !ValidName(name) {
		return Selection{}, fmt.Errorf("unknown workspace %q (%s)", name, source)
	}
	if w.Archived {
		return Selection{}, fmt.Errorf("workspace %q is archived", name)
	}
	return Selection{Name: name, Source: source, Workspace: w}, nil
}

func (c Config) Resolve(flagValue, cwd string) (Selection, error) {
	if flagValue != "" {
		return c.Select(flagValue, "flag")
	}
	if name := os.Getenv("HEV_WORKSPACE"); name != "" {
		return c.Select(name, "env")
	}
	if best := c.DirectoryOwner(cwd, false); best != "" {
		return c.Select(best, "pwd")
	}
	if c.ActiveWorkspace != "" {
		return c.Select(c.ActiveWorkspace, "active")
	}
	return c.Select(Default, "default")
}

// DirectoryOwner finds the most specific workspace containing path.
func (c Config) DirectoryOwner(path string, includeArchived bool) string {
	cwd := canonical(path)
	names := make([]string, 0, len(c.Workspaces))
	for name := range c.Workspaces {
		names = append(names, name)
	}
	sort.Strings(names)
	best, length := "", 0
	for _, name := range names {
		w := c.Workspaces[name]
		dir := canonical(w.Dir)
		if (includeArchived || !w.Archived) && len(dir) > length && containsCanonical(dir, cwd) {
			best, length = name, len(dir)
		}
	}
	return best
}

func canonical(path string) string {
	if path == "" {
		return ""
	}
	path, err := filepath.Abs(Expand(path))
	if err != nil {
		return ""
	}
	current := path
	for {
		if real, err := filepath.EvalSymlinks(current); err == nil {
			rest, _ := filepath.Rel(current, path)
			return filepath.Join(real, rest)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return path
		}
		current = parent
	}
}

func Contains(dir, path string) bool {
	return containsCanonical(canonical(dir), canonical(path))
}

func containsCanonical(dir, path string) bool {
	if dir == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (s Selection) AllowsHost(host string) bool {
	if len(s.Hosts) == 0 {
		return true
	}
	for _, allowed := range s.Hosts {
		if hostlabel.Same(allowed, host) {
			return true
		}
	}
	return false
}

func (s Selection) RequireHost(host string) error {
	if s.AllowsHost(host) {
		return nil
	}
	return fmt.Errorf("workspace boundary: host %q is outside workspace %q", host, s.Name)
}

func (s Selection) Root() string {
	return filepath.Join(Expand("~/.hev/workspaces"), s.Name)
}

// Named reads a target's local definition of a workspace carried on the wire.
func Named(name string) (Selection, error) {
	cfg, err := Read(ConfigPath())
	if err != nil {
		return Selection{}, err
	}
	return cfg.Select(name, "run")
}

func (s Selection) IsolatedStorage() bool {
	return s.Name != "" && (s.Name != Default || s.Layer != (Layer{}))
}

func (s Selection) Profile(harness string) string {
	var configured string
	switch harness {
	case "claude":
		configured = s.Subscriptions.ClaudeConfigDir
	case "codex":
		configured = s.Subscriptions.CodexHome
	case "agy":
		configured = s.Subscriptions.AgyDataDir
	default:
		return ""
	}
	if configured != "" {
		return Expand(configured)
	}
	if s.Login == "own" {
		return filepath.Join(s.Root(), "harnesses", harness)
	}
	return ""
}

func (s Selection) AgyTranscriptDir() string {
	if dir := s.Profile("agy"); dir != "" {
		return filepath.Join(dir, ".gemini", "antigravity-cli")
	}
	return ""
}
