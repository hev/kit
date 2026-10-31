package dispatch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Only environment names are stored; values are read from the admitting process.
func launchEnvironment(dir string) ([]string, error) {
	var m Meta
	if err := readJSON(filepath.Join(dir, "meta.json"), &m); err != nil {
		return nil, err
	}
	if !m.LaunchResolved {
		return nil, fmt.Errorf("launch settings not resolved at admission; rework required")
	}
	out := []string{}
	for _, key := range m.LaunchEnv {
		if strings.ContainsAny(key, "=\x00") {
			return nil, fmt.Errorf("invalid launch environment name")
		}
		if value, ok := os.LookupEnv(key); ok {
			out = append(out, key+"="+value)
		}
	}
	return out, nil
}

func resolveLaunch(m *Meta, cfg Config) {
	keys := []string{"PATH", "HOME", "USERPROFILE", "SHELL", "LANG", "LC_ALL", "TMPDIR", "SSH_AUTH_SOCK", "GH_TOKEN", "GITHUB_TOKEN", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "LAYER_API_KEY", "LAYER_NAMESPACE"}
	names := map[string]bool{}
	for _, key := range keys {
		if _, ok := os.LookupEnv(key); ok {
			names[key] = true
		}
	}
	for key := range cfg.Hosts[m.Host].Env {
		names[key] = true
	}
	if len(cfg.Hosts[m.Host].HarnessWrapper) > 0 {
		m.HarnessWrapper = cfg.Hosts[m.Host].HarnessWrapper
	}
	out := []string{}
	for key := range names {
		if key != "" && !strings.ContainsAny(key, "=\x00") {
			out = append(out, key)
		}
	}
	sort.Strings(out)
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

func recordUnconfirmed(dir string, m Meta, err error) {
	var completed Result
	if readJSON(filepath.Join(dir, "result.json"), &completed) == nil && !completed.End.IsZero() {
		return
	}
	_ = WriteJSON(filepath.Join(dir, "result.json"), Result{LaunchUnconfirmed: true, Start: m.Start, ExitCode: 1, Error: err.Error()})
}
