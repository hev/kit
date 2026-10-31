package dispatch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Launch environment is frozen at admission and redacted from RPC responses.
func launchEnvironment(dir string) ([]string, error) {
	var m Meta
	if err := readJSON(filepath.Join(dir, "meta.json"), &m); err != nil {
		return nil, err
	}
	if !m.LaunchResolved {
		return nil, fmt.Errorf("launch settings not resolved at admission; rework required")
	}
	return m.LaunchEnv, nil
}

func resolveLaunch(m *Meta, cfg Config) {
	keys := []string{"PATH", "HOME", "USERPROFILE", "SHELL", "LANG", "LC_ALL", "TMPDIR", "SSH_AUTH_SOCK", "GH_TOKEN", "GITHUB_TOKEN", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "LAYER_API_KEY", "LAYER_NAMESPACE"}
	values := map[string]string{}
	for _, key := range keys {
		if v, ok := os.LookupEnv(key); ok {
			values[key] = v
		}
	}
	for k, v := range cfg.Hosts[m.Host].Env {
		values[k] = v
	}
	if m.HarnessWrapper == nil {
		m.HarnessWrapper = cfg.Hosts[m.Host].HarnessWrapper
	}
	out := []string{}
	for k, v := range values {
		if k != "" && !strings.ContainsAny(k, "=\x00") {
			out = append(out, k+"="+v)
		}
	}
	m.LaunchEnv = out
	m.LaunchResolved = true
}

func addReservations(live map[string]bool) error {
	dirs, err := os.ReadDir(Root())
	if err != nil {
		return err
	}
	for _, d := range dirs {
		if d.IsDir() && (processRecordLive(filepath.Join(Root(), d.Name(), "admission.json")) || supervisorLive(filepath.Join(Root(), d.Name()))) {
			live[d.Name()] = true
		}
	}
	return nil
}
func reserve(dir string) error {
	return writeProcessRecord(filepath.Join(dir, "admission.json"), os.Getpid())
}
func supervisorLive(dir string) bool { return processRecordLive(filepath.Join(dir, "supervisor.json")) }

var errLaunchUnconfirmed = errors.New("supervisor launch unconfirmed")

const launchGrace = 30 * time.Second

func reconcileLaunch(dir string, r *Result) {
	if !r.LaunchUnconfirmed || time.Since(r.Start) < launchGrace || supervisorLive(dir) {
		return
	}
	r.LaunchUnconfirmed = false
	r.End = time.Now().UTC()
	r.Error = "supervisor did not confirm launch"
	_ = WriteJSON(filepath.Join(dir, "result.json"), r)
}
