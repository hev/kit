package steer

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Environment a supervisor sets so a hook finds its target without guessing.
const (
	EnvDir    = "HEV_STEER_DIR"
	EnvTarget = "HEV_STEER_TARGET"
)

// RunStore is a dispatch run's store.
func RunStore(dispatchRoot, name string) Store {
	return Store{Dir: filepath.Join(dispatchRoot, name, "steer"), Target: "dispatch:" + name}
}

// OrchestratorStore is an orchestrator's store.
func OrchestratorStore(dispatchRoot, name string) Store {
	return Store{Dir: filepath.Join(dispatchRoot, "orch-"+name, "steer"), Target: "orch:" + name}
}

var sessionName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// PeersStore is a local peers session's store.
func PeersStore(home, session string) (Store, bool) {
	if !sessionName.MatchString(session) || session == "." || session == ".." {
		return Store{}, false
	}
	return Store{Dir: filepath.Join(home, ".hev", "steer", "peers", session), Target: "peers:" + session}, true
}

// Resolve finds the store for a hook process: the supervisor's
// HEV_STEER_DIR, else the dispatch run whose directory holds cwd, else the
// tmux session the process runs in.
func Resolve(getenv func(string) string, cwd, home, dispatchRoot string, tmuxSession func() string) (Store, bool) {
	if dir := getenv(EnvDir); dir != "" {
		return Store{Dir: dir, Target: getenv(EnvTarget)}, true
	}
	if cwd != "" && dispatchRoot != "" {
		if rel, err := filepath.Rel(dispatchRoot, cwd); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			first := strings.Split(filepath.ToSlash(rel), "/")[0]
			if name, ok := strings.CutPrefix(first, "orch-"); ok {
				return OrchestratorStore(dispatchRoot, name), true
			}
			if _, err := os.Stat(filepath.Join(dispatchRoot, first, "meta.json")); err == nil {
				return RunStore(dispatchRoot, first), true
			}
		}
	}
	if tmuxSession != nil && getenv("TMUX_PANE") != "" {
		if s := tmuxSession(); s != "" {
			if strings.HasPrefix(s, "d-") && dispatchRoot != "" {
				if _, err := os.Stat(filepath.Join(dispatchRoot, s[2:], "meta.json")); err == nil {
					return RunStore(dispatchRoot, s[2:]), true
				}
			}
			return PeersStore(home, s)
		}
	}
	return Store{}, false
}

// Find locates a message by ID: in the given store first, then in every
// dispatch run, orchestrator and peers session store on this host.
func Find(id string, first *Store, home, dispatchRoot string) (Store, Message, error) {
	if first != nil && first.Dir != "" {
		if m, err := first.Get(id); err == nil {
			return *first, m, nil
		}
	}
	var candidates []Store
	if entries, err := os.ReadDir(dispatchRoot); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if name, ok := strings.CutPrefix(e.Name(), "orch-"); ok {
				candidates = append(candidates, OrchestratorStore(dispatchRoot, name))
			} else {
				candidates = append(candidates, RunStore(dispatchRoot, e.Name()))
			}
		}
	}
	if entries, err := os.ReadDir(filepath.Join(home, ".hev", "steer", "peers")); err == nil {
		for _, e := range entries {
			if s, ok := PeersStore(home, e.Name()); ok && e.IsDir() {
				candidates = append(candidates, s)
			}
		}
	}
	for _, s := range candidates {
		if m, err := s.Get(id); err == nil {
			return s, m, nil
		}
	}
	return Store{}, Message{}, os.ErrNotExist
}
