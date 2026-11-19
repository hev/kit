package dispatch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hev/kit/internal/update"
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
		if key == "HEV_ORCHESTRATOR" {
			continue
		}
		if strings.ContainsAny(key, "=\x00") {
			return nil, fmt.Errorf("invalid launch environment name")
		}
		if value, ok := os.LookupEnv(key); ok {
			out = append(out, key+"="+value)
		}
	}
	// Host env cannot forward a plugin-only name to a worker.
	return withoutPluginOnlyEnv(out), nil
}

func resolveLaunch(m *Meta, cfg Config) {
	keys := []string{"PATH", "HOME", "USERPROFILE", "SHELL", "LANG", "LC_ALL", "TMPDIR", "SSH_AUTH_SOCK", "GH_TOKEN", "GITHUB_TOKEN", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "HEVLAYER_API_KEY", "HEVLAYER_API_KEY_OP_REF", "HEVLAYER_API_KEY_REF", "LAYER_API_KEY_OP_REF", "HEVLAYER_NAMESPACE", "HEVLAYER_ENDPOINT", "HEVLAYER_STORE", "FACTORY_BOARD_URL", "HEV_CONFIG", "HEV_PEERS_BACKEND", "HEV_PEERS_STATE", "PEERS_SELF", "HEV_DISPATCH_CONFIG"}
	for _, name := range []string{"HEVLAYER_API_KEY", "HEVLAYER_NAMESPACE", "HEVLAYER_ENDPOINT", "HEVLAYER_STORE"} {
		keys = append(keys, "LAYER_"+strings.TrimPrefix(name, "HEVLAYER_"))
	}
	names := map[string]bool{}
	for _, key := range keys {
		if _, ok := os.LookupEnv(key); ok {
			names[key] = true
		}
	}
	for _, key := range cfg.Hosts[m.Host].Env {
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

// addReservations adds runs whose admission or supervisor process is live.
// Orchestrator state directories share the root but hold no worker slot:
// their work takes slots through the runs they dispatch.
func addReservations(live map[string]bool) error {
	dirs, err := os.ReadDir(Root())
	if err != nil {
		return err
	}
	for _, d := range dirs {
		if !d.IsDir() || orchestratorStateDir(d.Name()) {
			continue
		}
		if processRecordLive(filepath.Join(Root(), d.Name(), "admission.json")) || supervisorLive(filepath.Join(Root(), d.Name())) {
			live[d.Name()] = true
		}
	}
	return nil
}

// orchestratorStateDir is true for an orch-NAME directory under Root() that
// is not a dispatch run. A run writes meta.json before its admission or
// supervisor record, the same file hev dispatch ls and the cluster activity
// count read to call a directory a run; an orchestrator never writes one.
func orchestratorStateDir(name string) bool {
	return strings.HasPrefix(name, "orch-") && !exists(filepath.Join(Root(), name, "meta.json"))
}

// LiveOrchestrators counts orchestrators (Expediters included) whose
// supervisor process is live on this host. They hold no worker slot; hosts
// reports them beside Running so status bars can show them.
func LiveOrchestrators() (int, error) {
	dirs, err := os.ReadDir(Root())
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n := 0
	for _, d := range dirs {
		if d.IsDir() && orchestratorStateDir(d.Name()) && supervisorLive(filepath.Join(Root(), d.Name())) {
			n++
		}
	}
	return n, nil
}

// updateSwitchWait bounds how long admission waits for an update's switch.
var updateSwitchWait = time.Minute

// awaitUpdateSwitch lets an automatic update that is switching this host's
// build finish before the work just registered starts. Called after the
// admission record or round state is written: the update's last check then
// either sees the work and defers, or the work starts on the new build. An
// error means a switch was still under way when the wait ran out: the work
// must not start (admission fails; it can be retried).
func awaitUpdateSwitch() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	if _, err := update.AwaitSwitchWithin(home, updateSwitchWait); err != nil {
		return fmt.Errorf("admission: %w; try again once it finishes (hev update status)", err)
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
