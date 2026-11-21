package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

const Default = "hev"

type Project struct {
	Dir   string `toml:"dir" json:"dir"`
	Icon  string `toml:"icon,omitempty" json:"icon"`
	Color string `toml:"color,omitempty" json:"color"`
}

type Subscriptions struct {
	ClaudeConfigDir string            `toml:"claude_config_dir,omitempty" json:"claude_config_dir,omitempty"`
	CodexHome       string            `toml:"codex_home,omitempty" json:"codex_home,omitempty"`
	AgyDataDir      string            `toml:"agy_data_dir,omitempty" json:"agy_data_dir,omitempty"`
	EnvRefs         map[string]string `toml:"env_refs,omitempty" json:"-"`
}

type Layer struct {
	Endpoint        string `toml:"endpoint,omitempty" json:"endpoint,omitempty"`
	TracesNamespace string `toml:"traces_namespace,omitempty" json:"traces_namespace,omitempty"`
	BoardNamespace  string `toml:"board_namespace,omitempty" json:"board_namespace,omitempty"`
	BoardURL        string `toml:"board_url,omitempty" json:"board_url,omitempty"`
	KeyRef          string `toml:"api_key_op_ref,omitempty" json:"-"`
}

type Cluster struct {
	Seeds   []string `toml:"seeds,omitempty" json:"seeds,omitempty"`
	KeyRef  string   `toml:"key_op_ref,omitempty" json:"-"`
	Network string   `toml:"network,omitempty" json:"network,omitempty"`
}

type Workspace struct {
	Subscriptions Subscriptions `toml:"subscriptions,omitempty" json:"subscriptions,omitempty"`
	Layer         Layer         `toml:"layer,omitempty" json:"layer,omitempty"`
	Cluster       Cluster       `toml:"cluster,omitempty" json:"cluster,omitempty"`

	Project
	Projects map[string]Project `toml:"projects,omitempty" json:"projects,omitempty"`
	Login    string             `toml:"login,omitempty" json:"login,omitempty"`
	Archived bool               `toml:"archived,omitempty" json:"archived,omitempty"`
	Hosts    []string           `toml:"hosts,omitempty" json:"hosts,omitempty"`
}

type Picker struct {
	Frequents     map[string]string `toml:"frequents"`
	CheckIn       []string          `toml:"check_in"`
	CloseRoutes   map[string]string `toml:"close_routes"`
	ProjectTables bool              `toml:"project_tables"`
	HideBOH       bool              `toml:"hide_boh"`
	ClaudePlugin  string            `toml:"claude_plugin"`
	PiExtension   string            `toml:"pi_extension"`
}

type Config struct {
	ActiveWorkspace string               `toml:"active_workspace"`
	Workspaces      map[string]Workspace `toml:"workspaces"`
	Pick            Picker               `toml:"pick"`
}

var namePattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]*$`)
var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func ValidName(name string) bool { return namePattern.MatchString(name) }
func Color(color string) string {
	if colorPattern.MatchString(color) {
		return color
	}
	return "#B6A0FF"
}

func Read(path string) (Config, error) {
	cfg := Config{}
	if _, err := toml.DecodeFile(path, &cfg); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return cfg, fmt.Errorf("invalid workspace configuration")
	}
	if cfg.Workspaces == nil {
		cfg.Workspaces = make(map[string]Workspace)
	}
	if _, ok := cfg.Workspaces[Default]; !ok {
		cfg.Workspaces[Default] = Workspace{Project: Project{Dir: "~/workspace", Color: Color("")}}
	}
	if cfg.Workspaces[Default].Dir == "" {
		w := cfg.Workspaces[Default]
		w.Dir = "~/workspace"
		cfg.Workspaces[Default] = w
	}
	return cfg, validateStorage(cfg.Workspaces)
}

// Update preserves the values of unrelated kit settings, including unknown keys.
func Update(path string, change func(map[string]Workspace) error) error {
	return edit(path, func(cfg *Config) error { return change(cfg.Workspaces) })
}

func Use(path, name string) error {
	return edit(path, func(cfg *Config) error {
		if _, err := cfg.Select(name, "active"); err != nil {
			return err
		}
		cfg.ActiveWorkspace = name
		return nil
	})
}

func edit(path string, change func(*Config) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	lock := path + ".workspace-lock"
	if err := os.Mkdir(lock, 0700); err != nil {
		return fmt.Errorf("lock workspace configuration: %w", err)
	}
	defer os.Remove(lock)

	var document map[string]any
	_, err := toml.DecodeFile(path, &document)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if document == nil {
		document = make(map[string]any)
	}
	cfg, err := Read(path)
	if err != nil {
		return err
	}
	if err := change(&cfg); err != nil {
		return err
	}
	if err := validateStorage(cfg.Workspaces); err != nil {
		return err
	}
	if cfg.Workspaces[cfg.ActiveWorkspace].Archived {
		cfg.ActiveWorkspace = ""
	}
	document["active_workspace"] = cfg.ActiveWorkspace
	document["workspaces"] = cfg.Workspaces

	file, err := os.CreateTemp(filepath.Dir(path), ".workspace-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	encodeErr := toml.NewEncoder(file).Encode(document)
	closeErr := file.Close()
	if encodeErr != nil {
		return encodeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), path)
}

func Expand(dir string) string {
	if dir == "~" || (strings.HasPrefix(dir, "~/") || (filepath.Separator == '\\' && strings.HasPrefix(dir, "~\\"))) {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimLeft(strings.TrimPrefix(dir, "~"), "/\\"))
	}
	return dir
}

// Import only reads literal list assignments. The input is never sourced.
func Import(themesPath, listsPath string) (map[string]Workspace, error) {
	type theme struct {
		Project
		Client bool   `json:"client"`
		Host   string `json:"execution_host"`
	}
	var themes map[string]theme
	raw, err := os.ReadFile(themesPath)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &themes); err != nil {
		return nil, err
	}
	raw, err = os.ReadFile(listsPath)
	if err != nil {
		return nil, err
	}
	literal := func(key string) (string, bool) {
		match := regexp.MustCompile("(?m)^" + key + `="([^"]*)"[ \t]*(?:#.*)?$`).FindStringSubmatch(string(raw))
		if match == nil {
			return "", false
		}
		return match[1], true
	}
	main := Workspace{Project: Project{Dir: "~/workspace"}, Projects: make(map[string]Project)}
	main.Color, _ = literal("TMUX_HEV_COLOR")
	main.Color = Color(main.Color)
	for name, t := range themes {
		if !t.Client && ValidName(name) {
			main.Projects[name] = t.Project
		}
	}
	out := map[string]Workspace{Default: main}
	for _, key := range []string{"TMUX_CLIENTS", "TMUX_ARCHIVE"} {
		list, ok := literal(key)
		if !ok {
			return nil, fmt.Errorf("missing literal %s assignment", key)
		}
		for _, line := range strings.Split(list, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			name, dir, ok := strings.Cut(strings.ReplaceAll(line, "\t", " "), " ")
			if !ok || !ValidName(name) || name == Default || strings.TrimSpace(dir) == "" {
				return nil, fmt.Errorf("invalid %s row %q", key, line)
			}
			if _, exists := out[name]; exists {
				return nil, fmt.Errorf("duplicate workspace %q", name)
			}
			t := themes[name]
			w := Workspace{Project: t.Project, Archived: key == "TMUX_ARCHIVE"}
			w.Dir = strings.TrimSpace(dir)
			if !w.Archived {
				w.Login = "own"
			}
			if t.Host != "" {
				w.Hosts = []string{t.Host}
			}
			out[name] = w
		}
	}
	return out, nil
}

func (s Selection) Namespaces() (traces, board string) {
	traces, board = s.Layer.TracesNamespace, s.Layer.BoardNamespace
	if traces == "" {
		traces = s.Name + "-traces"
	}
	if board == "" {
		board = s.Name + "-board"
		if s.Name == Default {
			board = "factory-board"
		}
	}
	return traces, board
}

func validateStorage(entries map[string]Workspace) error {
	spaces := map[string]string{"hev-traces": Default, "factory-board": Default}
	urls := make(map[string]string)
	for name, w := range entries {
		if !ValidName(name) {
			return fmt.Errorf("invalid configured workspace name %q", name)
		}
		if w.Login != "" && w.Login != "own" {
			return fmt.Errorf("invalid login mode for workspace %q", name)
		}
		tr, bo := (Selection{Name: name, Workspace: w}).Namespaces()
		for _, ns := range []string{tr, bo} {
			if !ValidName(ns) {
				return fmt.Errorf("invalid Layer namespace for workspace %q", name)
			}
			for previous, owner := range spaces {
				if owner != name && (namespaceWithin(ns, previous) || namespaceWithin(previous, ns)) {
					return fmt.Errorf("workspace boundary: %q and %q share a storage target", owner, name)
				}
			}
			spaces[ns] = name
		}
		if url := strings.TrimRight(w.Layer.BoardURL, "/"); url != "" {
			if owner, ok := urls[url]; ok && owner != name {
				return fmt.Errorf("workspace boundary: %q and %q share a board URL", owner, name)
			}
			urls[url] = name
		}
	}
	return nil
}

func namespaceWithin(namespace, base string) bool {
	return namespace == base || strings.HasPrefix(namespace, base+"-")
}

// CheckNamespace protects configured workspace targets even when the legacy
// default workspace supplies a namespace override.
func (c Config) CheckNamespace(selected, namespace string) error {
	for name, w := range c.Workspaces {
		if name == selected || name == Default {
			continue
		}
		traces, board := (Selection{Name: name, Workspace: w}).Namespaces()
		for _, reserved := range []string{traces, board} {
			if namespaceWithin(namespace, reserved) {
				return fmt.Errorf("workspace boundary: namespace belongs to workspace %q", name)
			}
		}
	}
	return nil
}
