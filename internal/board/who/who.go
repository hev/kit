package who

import (
	"os"
	"os/exec"
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
		name = roleName()
	}
	if name == "" {
		if insideAgent() {
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
	if inst := os.Getenv("FACTORY_INSTANCE"); inst != "" && role != "foreman" {
		return role + "-" + inst
	}
	return role
}
func insideAgent() bool {
	return os.Getenv("CLAUDE_SESSION_ID") != "" || os.Getenv("CLAUDECODE") != "" || os.Getenv("CODEX_THREAD_ID") != "" || os.Getenv("CODEX_CI") != ""
}

func Classify(name string) (role, instance string) { return classify(name) }
func classify(name string) (role, instance string) {
	switch {
	case strings.HasPrefix(name, "worker-"):
		rest := strings.TrimPrefix(name, "worker-")
		if i := strings.Index(rest, "-"); i > 0 {
			return "worker", rest[:i]
		}
		return "worker", rest
	case name == "agent":
		return "agent", ""
	case name == os.Getenv("USER"):
		return "human", ""
	}
	return "agent", ""
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
