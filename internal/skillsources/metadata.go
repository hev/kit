package skillsources

import (
	"fmt"
	"github.com/hev/kit/skills"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
)

// Metadata is restricted to the YAML front matter; body examples never affect installs.
func metadata(path string) (map[string][]string, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	return Metadata(path, b)
}

// Metadata parses install metadata from SKILL.md front matter; name labels errors.
// Bundled skills use it for audience; sources do not honor audience yet.
func Metadata(name string, b []byte) (map[string][]string, error) {
	path := name
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	result := map[string][]string{}
	if len(lines) == 0 || lines[0] != "---" {
		return result, nil
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, fmt.Errorf("unclosed front matter in %s", path)
	}
	var doc map[string]any
	if e := yaml.Unmarshal([]byte(strings.Join(lines[1:end], "\n")), &doc); e != nil {
		return nil, fmt.Errorf("metadata %s: %w", path, e)
	}
	nested, _ := doc["metadata"].(map[string]any)
	for _, key := range []string{"harnesses", "machines", "roles", "audience"} {
		value, ok := nested[key]
		if !ok {
			value = doc[key]
		}
		switch v := value.(type) {
		case nil:
		case string:
			result[key] = []string{v}
		case []any:
			for _, item := range v {
				s, ok := item.(string)
				if !ok {
					return nil, fmt.Errorf("%s must contain strings", key)
				}
				result[key] = append(result[key], s)
			}
		default:
			return nil, fmt.Errorf("%s must be a string or list", key)
		}
	}
	return result, nil
}
func includes(values []string, s string) bool {
	for _, v := range values {
		if v == s || v == "all" {
			return true
		}
	}
	return false
}
func plan(home, root, pin string, c Config) (map[string]string, error) {
	entries, e := os.ReadDir(root)
	if e != nil {
		return nil, e
	}
	links := map[string]string{}
	for _, d := range entries {
		if !d.IsDir() {
			continue
		}
		name := d.Name()
		if _, err := skills.FS.ReadFile(name + "/SKILL.md"); err == nil {
			return nil, fmt.Errorf("source uses reserved bundled skill name %s", name)
		}
		src := filepath.Join(root, name)
		from := filepath.Join(pin, name)
		if _, e = os.Stat(filepath.Join(src, "SKILL.md")); os.IsNotExist(e) {
			continue
		}
		if c.Role == "host" {
			if _, e = os.Stat(filepath.Join(src, "boh", "SKILL.md")); e == nil {
				src = filepath.Join(src, "boh")
				from = filepath.Join(from, "boh")
			}
		}
		m, e := metadata(filepath.Join(src, "SKILL.md"))
		if e != nil {
			return nil, e
		}
		targets := m["harnesses"]
		if len(targets) == 0 {
			targets = []string{"claude", "codex"}
		}
		for _, h := range targets {
			if h != "all" && h != "claude" && h != "codex" && h != "agents" {
				return nil, fmt.Errorf("unknown harness %q", h)
			}
		}
		for _, role := range m["roles"] {
			if role != "all" && role != "sender" && role != "host" {
				return nil, fmt.Errorf("unknown role %q", role)
			}
		}
		if len(m["machines"]) > 0 && !includes(m["machines"], c.Machine) {
			continue
		}
		if len(m["roles"]) > 0 && !includes(m["roles"], c.Role) {
			continue
		}
		for _, h := range []string{"claude", "codex", "agents"} {
			if includes(targets, h) {
				links[filepath.Join(home, "."+h, "skills", name)] = from
			}
		}
		bins, e := os.ReadDir(filepath.Join(src, "bin"))
		if e != nil && !os.IsNotExist(e) {
			return nil, e
		}
		for _, bin := range bins {
			if bin.IsDir() {
				continue
			}
			info, e := bin.Info()
			if e != nil {
				return nil, e
			}
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("nonregular command %s", bin.Name())
			}
			dest := filepath.Join(home, ".local", "bin", bin.Name())
			if _, ok := links[dest]; ok {
				return nil, fmt.Errorf("command collision: %s", bin.Name())
			}
			links[dest] = filepath.Join(from, "bin", bin.Name())
		}
	}
	return links, nil
}
