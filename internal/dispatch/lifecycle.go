package dispatch

import (
	"os"
	"path/filepath"
	"strings"
)

// Environment values are handed directly to the launcher, never persisted.
func launchEnvironment(dir string) []string {
	keys := []string{"PATH", "HOME", "USERPROFILE", "SHELL", "LANG", "LC_ALL", "TMPDIR", "SSH_AUTH_SOCK", "GH_TOKEN", "GITHUB_TOKEN", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "LAYER_API_KEY", "LAYER_NAMESPACE"}
	values := map[string]string{}
	for _, key := range keys {
		if v, ok := os.LookupEnv(key); ok {
			values[key] = v
		}
	}
	var m Meta
	if readJSON(filepath.Join(dir, "meta.json"), &m) == nil {
		if cfg, err := Load(m.ConfigPath); err == nil {
			for k, v := range cfg.Hosts[m.Host].Env {
				values[k] = v
			}
		}
	}
	out := []string{}
	for k, v := range values {
		if k != "" && !strings.ContainsAny(k, "=\x00") {
			out = append(out, k+"="+v)
		}
	}
	return out
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
