package who

import (
	"github.com/hev/kit/internal/board/settings"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type Identity struct {
	Author   string `json:"author"`
	Role     string `json:"role"`
	Instance string `json:"instance,omitempty"`
	Host     string `json:"host,omitempty"`
	Session  string `json:"session,omitempty"`
}

func Detect(explicit string) Identity {
	id := Identity{Host: shortHost(), Session: os.Getenv("FACTORY_SESSION")}
	if s := os.Getenv("CLAUDE_SESSION_ID"); s != "" && id.Session == "" {
		id.Session = s
	}
	name := explicit
	if name == "" {
		name = os.Getenv("FACTORY_BOARD_AS")
	}
	if name == "" && os.Getenv("FACTORY_SESSION") != "" {
		id.Author, id.Role, id.Instance = "factory-"+os.Getenv("FACTORY_SESSION"), "worker", line()
		return id
	}
	if name == "" {
		name = tmuxSession()
	}
	if name == "" {
		name = settings.Current.Identity
	}
	if name == "" {
		name = roleName()
	}
	if name == "" {
		if insideAgent() || headless() {
			id.Author, id.Role = "agent", "agent"
			return id
		}
		name = os.Getenv("USER")
		if name == "" {
			name = "anon"
		}
		id.Author, id.Role = name, "human"
		return id
	}
	id.Author = name
	id.Role, id.Instance = classify(name)
	return id
}
func line() string {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	u := strings.TrimSuffix(strings.TrimSpace(string(out)), ".git")
	return u[strings.LastIndexAny(u, "/:")+1:]
}
func roleName() string {
	role := os.Getenv("FACTORY_ROLE")
	if role == "" {
		return ""
	}
	if inst := os.Getenv("FACTORY_INSTANCE"); inst != "" {
		return role + "-" + inst
	}
	return role
}
func insideAgent() bool {
	return os.Getenv("CLAUDE_SESSION_ID") != "" || os.Getenv("CLAUDECODE") != "" || os.Getenv("CODEX_THREAD_ID") != "" || os.Getenv("CODEX_CI") != ""
}

func Classify(name string) (role, instance string) { return classify(name) }
func Match(name string) (role, instance string, matched bool) {
	rules := settings.Current.Roles
	if len(rules) == 0 {
		rules = []settings.Role{{Pattern: "^worker-([^-]+).*$", Role: "worker", Instance: "$1"}, {Pattern: "^agent$", Role: "agent"}}
	}
	for _, rule := range rules {
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			continue
		}
		if m := re.FindStringSubmatchIndex(name); m != nil {
			return rule.Role, string(re.ExpandString(nil, rule.Instance, name, m)), true
		}
	}
	return "", "", false
}
func classify(name string) (role, instance string) {
	if r, i, ok := Match(name); ok {
		return r, i
	}
	if name == os.Getenv("USER") {
		return "human", ""
	}
	return "agent", ""
}
func headless() bool {
	if settings.Current.Headless {
		return true
	}
	home, _ := os.UserHomeDir()
	for _, dir := range []string{".hev", ".factory"} {
		if _, err := os.Stat(filepath.Join(home, dir, "no-desk")); err == nil {
			return true
		}
	}
	return false
}

func tmuxSession() string {
	if os.Getenv("TMUX") == "" {
		return ""
	}
	out, err := exec.Command("tmux", "display-message", "-p", "#S").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
func shortHost() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	if i := strings.Index(h, "."); i > 0 {
		h = h[:i]
	}
	return strings.ToLower(h)
}
