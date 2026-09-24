package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"

	"bytes"
	"context"
	"github.com/hev/kit/internal/daemon"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sandbox gives a test its own home, config, labels and a PATH holding fake
// `docker` and `launchctl` binaries that append their arguments to one log.
// Nothing here can reach the real jobs or the real Docker.
func sandbox(t *testing.T, dockerInfoExit string) (home, log string) {
	t.Helper()
	home = t.TempDir()
	bin := t.TempDir()
	log = filepath.Join(bin, "calls.log")
	fake := func(name, body string) {
		script := "#!/bin/sh\necho \"" + name + " $*\" >> \"" + log + "\"\n" +
			"[ -f \"$HEV_CONFIG\" ] && [ ! -f \"" + log + ".cfg\" ] && echo \"" + name + " $*\" > \"" + log + ".cfg\"\n" + body
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fake("docker", "[ \"$1\" = info ] && exit "+dockerInfoExit+"\nexit 0\n")
	// `launchctl print` succeeding means the job is loaded; FAKE_UNLOADED
	// makes it fail, as it does for a job that was never installed.
	fake("launchctl", "[ \"$1\" = print ] && [ -n \"$FAKE_UNLOADED\" ] && exit 113\nexit 0\n")
	for _, env := range []string{"LAYER_ENDPOINT", "LAYER_API_KEY", "LAYER_NAMESPACE", "LAYER_STORE", "HEV_LOCAL_IMAGE", "HEV_LOCAL_PORT", "HEV_LOCAL_SERVE_PORT", "HEV_LOCAL_KIT_IMAGE", "TURBOPUFFER_API_KEY", "FAKE_UNLOADED"} {
		t.Setenv(env, "")
	}
	// Config loading shells out to `gh`, and a current gh writes its own
	// state under HOME (~/.local/state/gh/device-id). It is faked and logged
	// like the others, and PATH holds the fakes and nothing else, so what a
	// test sees under its HOME is what kit put there on any machine.
	fake("gh", "exit 1\n")
	t.Setenv("PATH", bin)
	t.Setenv("HOME", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, ".hev", "config.toml"))
	t.Setenv("HEV_LAUNCHD_LABEL", "com.hev.test.hevd")
	t.Setenv("HEV_SERVE_LAUNCHD_LABEL", "com.hev.test.serve")
	t.Setenv("HEV_LOCAL_PROJECT", "hev-test")
	return home, log
}

// managed writes the config exactly as `hev up` would, plus an index state
// describing its archive, and returns the state's path.
func managed(t *testing.T, home string) (state string) {
	t.Helper()
	cfg, err := daemon.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := daemon.WriteLocalConfig(cfg.Local, "tpuf_test"); err != nil {
		t.Fatal(err)
	}
	return indexState(t, home)
}

const stateBytes = `{"units":{"a":"1"}}`

func indexState(t *testing.T, home string) string {
	t.Helper()
	state := filepath.Join(home, ".hev", "index-state.json")
	os.MkdirAll(filepath.Dir(state), 0o755)
	if err := os.WriteFile(state, []byte(stateBytes), 0o600); err != nil {
		t.Fatal(err)
	}
	return state
}

// calls is what kit ran to do its work: docker and launchctl. allCalls also
// has the `gh` identity lookup that loading a config makes.
func calls(t *testing.T, log string) string {
	t.Helper()
	var kept []string
	for _, line := range strings.Split(allCalls(t, log), "\n") {
		if line != "" && !strings.HasPrefix(line, "gh ") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

func allCalls(t *testing.T, log string) string {
	t.Helper()
	raw, _ := os.ReadFile(log)
	return strings.TrimSpace(string(raw))
}

// onOS pins the platform decision for one test, so the launchd half runs (or
// does not) wherever the suite does. launchctl itself is always the fake.
func onOS(t *testing.T, goos string) {
	t.Helper()
	was := hostOS
	hostOS = goos
	t.Cleanup(func() { hostOS = was })
}

func TestUpStopsAtDockerPreflight(t *testing.T) {
	home, log := sandbox(t, "1")
	t.Setenv("TURBOPUFFER_API_KEY", "tpuf_test")
	var out bytes.Buffer
	err := runUp(context.Background(), &out, false)
	if err == nil {
		t.Fatal("hev up succeeded without docker")
	}
	if msg := err.Error(); strings.Contains(msg, "\n") || !strings.Contains(msg, "docker is not running") {
		t.Fatalf("hint = %q", msg)
	}
	if out.Len() != 0 {
		t.Fatalf("reported progress before docker was up: %q", out.String())
	}
	raw, _ := os.ReadFile(log)
	if strings.Contains(string(raw), "compose") || strings.Contains(string(raw), "launchctl") {
		t.Fatalf("went past the preflight:\n%s", raw)
	}
	if _, err := os.Stat(filepath.Join(home, ".hev", "config.toml")); err == nil {
		t.Fatal("config written before docker was up")
	}
}

// The key is the one thing `up` cannot supply. Without it, up says where to get
// one and touches nothing: no docker, no config, no job.
func TestUpWithoutAKeyStopsBeforeAnything(t *testing.T) {
	onOS(t, "darwin")
	home, log := sandbox(t, "0")
	var out bytes.Buffer
	err := runUp(context.Background(), &out, false)
	if err == nil || strings.Contains(err.Error(), "\n") || !strings.Contains(err.Error(), "TURBOPUFFER_API_KEY") ||
		!strings.Contains(err.Error(), "turbopuffer.com") {
		t.Fatalf("err = %v", err)
	}
	if got := allCalls(t, log); got != "" {
		t.Fatalf("ran commands without a key:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".hev")); err == nil {
		t.Fatal("wrote under ~/.hev without a key")
	}
}

// KeepAlive restarts what was just stopped, so the containers go first and
// the jobs are unloaded — not signalled — second.
func TestDownStopsContainersThenUnloadsJobs(t *testing.T) {
	onOS(t, "darwin")
	home, log := sandbox(t, "0")
	plists := filepath.Join(home, "Library", "LaunchAgents")
	os.MkdirAll(plists, 0o755)
	for _, label := range []string{"com.hev.test.hevd", "com.hev.test.serve"} {
		os.WriteFile(filepath.Join(plists, label+".plist"), []byte("x"), 0o644)
	}
	state := managed(t, home)
	var out bytes.Buffer
	if err := runDown(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(log)
	var calls []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.HasPrefix(line, "docker compose -p") || strings.HasPrefix(line, "launchctl bootout") {
			calls = append(calls, line)
		}
	}
	domain := launchdDomain()
	want := []string{
		"docker compose -p hev-test -f " + filepath.Join(home, ".hev", "local", "docker-compose.yml") + " down --remove-orphans",
		"launchctl bootout " + domain + "/com.hev.test.hevd",
		"launchctl bootout " + domain + "/com.hev.test.serve",
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls:\n%s\nwant:\n%s", strings.Join(calls, "\n"), strings.Join(want, "\n"))
	}
	// The archive is in Turbopuffer and outlives the containers, so the index
	// state that describes it does too.
	if got, _ := os.ReadFile(state); string(got) != stateBytes {
		t.Fatal("index state reset by down")
	}
	if left, _ := os.ReadDir(plists); len(left) != 0 {
		t.Fatalf("plists left behind: %v", left)
	}
	if strings.Contains(string(raw), "com.hev.hevd") || strings.Contains(string(raw), "com.hev.serve") {
		t.Fatalf("addressed a default label from a sandbox:\n%s", raw)
	}
}

// Without launchd, down is compose down and a line saying what is left to the
// user. It must not reach for launchctl.
func TestDownWithoutLaunchd(t *testing.T) {
	onOS(t, "linux")
	home, log := sandbox(t, "0")
	managed(t, home)
	var out bytes.Buffer
	if err := runDown(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(log)
	if strings.Contains(string(raw), "launchctl") {
		t.Fatalf("called launchctl on linux:\n%s", raw)
	}
	if !strings.Contains(string(raw), "docker compose -p hev-test -f "+filepath.Join(home, ".hev", "local", "docker-compose.yml")+" down --remove-orphans") {
		t.Fatalf("no compose down:\n%s", raw)
	}
	if got := out.String(); !strings.Contains(got, "containers stopped") || !strings.Contains(got, "no launchd on linux") {
		t.Fatalf("output = %q", got)
	}
}

func TestLaunchdLabelsAndCommands(t *testing.T) {
	t.Setenv("HEV_LAUNCHD_LABEL", "")
	t.Setenv("HEV_SERVE_LAUNCHD_LABEL", "")
	if launchdLabel() != "com.hev.hevd" || serveLabel() != "com.hev.serve" {
		t.Fatalf("defaults = %s, %s", launchdLabel(), serveLabel())
	}
	t.Setenv("HEV_LAUNCHD_LABEL", "com.hev.test.hevd")
	job := daemonJob("/tmp/bin/hev", "/tmp/home", "/tmp/home/.hev/config.toml")
	if got := job.plistPath("/tmp/home"); got != "/tmp/home/Library/LaunchAgents/com.hev.test.hevd.plist" {
		t.Fatalf("plist path = %s", got)
	}
	if got := strings.Join(job.bootstrapArgs("/p.plist"), " "); got != "bootstrap "+launchdDomain()+" /p.plist" {
		t.Fatalf("bootstrap = %s", got)
	}
	if got := strings.Join(job.bootoutArgs(), " "); got != "bootout "+launchdDomain()+"/com.hev.test.hevd" {
		t.Fatalf("bootout = %s", got)
	}
	plist := job.plist()
	for _, want := range []string{
		"<string>com.hev.test.hevd</string>",
		"<key>HOME</key>\n        <string>/tmp/home</string>",
		"<key>HEV_CONFIG</key>\n        <string>/tmp/home/.hev/config.toml</string>",
		"<string>/tmp/home/.hev/hevd.err</string>",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("plist missing %q:\n%s", want, plist)
		}
	}
	if strings.Contains(plist, "com.hev.hevd") {
		t.Fatal("plist names the default label under an override")
	}
}

const hostedConfig = "# my hosted archive\n[layer]\nendpoint = \"https://gcp-us-central1.turbopuffer.com\"\napi_key = \"\"\nnamespace = \"hev-traces\"\n"

func writeConfig(t *testing.T, home, text string) string {
	t.Helper()
	path := filepath.Join(home, ".hev", "config.toml")
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A hosted host is refused before anything is started: no docker call — so no
// pull and no container — no launchctl call, and no file written.
func TestUpRefusesAHostedConfigBeforeStartingAnything(t *testing.T) {
	onOS(t, "darwin")
	home, log := sandbox(t, "0")
	path := writeConfig(t, home, hostedConfig)
	state := indexState(t, home)
	var out bytes.Buffer
	err := runUp(context.Background(), &out, false)
	if err == nil || !strings.Contains(err.Error(), "will not repoint a hosted config") {
		t.Fatalf("err = %v", err)
	}
	// Not one external command, the identity lookup included.
	if got := allCalls(t, log); got != "" {
		t.Fatalf("refusal came after:\n%s", got)
	}
	if out.Len() != 0 {
		t.Fatalf("output = %q", out.String())
	}
	if got, _ := os.ReadFile(path); string(got) != hostedConfig {
		t.Fatalf("config changed:\n%s", got)
	}
	if got, _ := os.ReadFile(state); string(got) != stateBytes {
		t.Fatal("index state changed")
	}
	left, _ := os.ReadDir(filepath.Join(home, ".hev"))
	if len(left) != 2 {
		t.Fatalf("files written under ~/.hev: %v", left)
	}
	for _, dir := range []string{"Library", ".local"} {
		if _, err := os.Stat(filepath.Join(home, dir)); err == nil {
			var under []string
			filepath.WalkDir(filepath.Join(home, dir), func(path string, _ os.DirEntry, _ error) error {
				under = append(under, strings.TrimPrefix(path, home+"/"))
				return nil
			})
			t.Fatalf("%s created: %v", dir, under)
		}
	}
}

// down on a config `up` never wrote — a hosted host — leaves the jobs, their
// plists and the index state exactly as they are.
func TestDownOnAnUnmanagedConfigTouchesNoJobsAndNoState(t *testing.T) {
	onOS(t, "darwin")
	home, log := sandbox(t, "0")
	path := writeConfig(t, home, hostedConfig)
	state := indexState(t, home)
	plist := filepath.Join(home, "Library", "LaunchAgents", "com.hev.test.hevd.plist")
	os.MkdirAll(filepath.Dir(plist), 0o755)
	os.WriteFile(plist, []byte("x"), 0o644)
	var out bytes.Buffer
	if err := runDown(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	if got := calls(t, log); strings.Contains(got, "launchctl") {
		t.Fatalf("launchctl called on an unmanaged config:\n%s", got)
	}
	if got, _ := os.ReadFile(state); string(got) != stateBytes {
		t.Fatal("index state changed")
	}
	if _, err := os.Stat(plist); err != nil {
		t.Fatal("plist removed")
	}
	if got, _ := os.ReadFile(path); string(got) != hostedConfig {
		t.Fatal("config changed")
	}
	if got := out.String(); !strings.Contains(got, "no [local] block") || strings.Count(strings.TrimSpace(got), "\n") != 1 {
		t.Fatalf("output = %q", got)
	}
}

// Without Docker a managed down still unloads the jobs it installed, then
// fails in one line naming what it could not stop.
func TestDownWithoutDockerStillUnloadsJobs(t *testing.T) {
	onOS(t, "darwin")
	home, log := sandbox(t, "1")
	state := managed(t, home)
	var out bytes.Buffer
	err := runDown(context.Background(), &out)
	if err == nil {
		t.Fatal("down reported success with the containers still up")
	}
	if msg := err.Error(); strings.Contains(msg, "\n") || !strings.Contains(msg, "hev-test containers were left") || !strings.Contains(msg, "`hev down`") {
		t.Fatalf("err = %q", msg)
	}
	got := calls(t, log)
	if strings.Contains(got, "compose") {
		t.Fatalf("compose called without docker:\n%s", got)
	}
	for _, label := range []string{"com.hev.test.hevd", "com.hev.test.serve"} {
		if !strings.Contains(got, "launchctl bootout "+launchdDomain()+"/"+label) {
			t.Fatalf("%s not unloaded:\n%s", label, got)
		}
	}
	if raw, _ := os.ReadFile(state); string(raw) != stateBytes {
		t.Fatal("index state reset by down")
	}
}

// stub listens on an ephemeral loopback port and returns it.
func stub(t *testing.T, body string) int {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
	t.Cleanup(srv.Close)
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	n, _ := strconv.Atoi(port)
	return n
}

// upSandbox is a sandbox `up` can succeed in: a key, launchd jobs that are not
// loaded, and stubs holding the gateway and dashboard ports.
func upSandbox(t *testing.T) (home, log string) {
	t.Helper()
	onOS(t, "darwin")
	home, log = sandbox(t, "0")
	t.Setenv("FAKE_UNLOADED", "1")
	t.Setenv("TURBOPUFFER_API_KEY", "tpuf_test")
	t.Setenv("HEV_LOCAL_PORT", strconv.Itoa(stub(t, `{"status":"ok","version":"0.6.0-dev"}`)))
	t.Setenv("HEV_LOCAL_SERVE_PORT", strconv.Itoa(stub(t, "ok")))
	// The stubs hold the ports `up` checks; they stand in for what it starts.
	was := portFree
	portFree = func(int) bool { return true }
	t.Cleanup(func() { portFree = was })
	return home, log
}

// The steps run in order, each after the one it depends on: docker, compose
// up, the config file, the daemon job. The dashboard is a container, so no
// read-side job is installed.
func TestUpStepOrder(t *testing.T) {
	home, log := upSandbox(t)
	var out bytes.Buffer
	if err := runUp(context.Background(), &out, false); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(home, ".hev", "local", "docker-compose.yml")
	domain := launchdDomain()
	agents := filepath.Join(home, "Library", "LaunchAgents")
	want := []string{
		"docker info --format {{.ServerVersion}}",
		"docker compose version",
		"docker compose -p hev-test ps -q gateway",
		"docker compose -p hev-test ps -q dashboard",
		"docker compose -p hev-test -f " + file + " up --detach --wait --remove-orphans gateway dashboard",
		"docker compose -p hev-test ps -q gateway",
		"docker compose -p hev-test ps -q dashboard",
		"launchctl print " + domain + "/com.hev.test.hevd",
		"launchctl bootstrap " + domain + " " + filepath.Join(agents, "com.hev.test.hevd.plist"),
	}
	// How often a job is probed is incidental; repeats of a call are folded.
	var got []string
	for _, line := range strings.Split(calls(t, log), "\n") {
		if len(got) == 0 || got[len(got)-1] != line {
			got = append(got, line)
		}
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// The config exists by the first launchctl call and not before it: it is
	// written after the key is checked and before the daemon is installed.
	first, _ := os.ReadFile(log + ".cfg")
	if got := strings.TrimSpace(string(first)); got != want[7] {
		t.Fatalf("first call that saw a config file = %q, want %q", got, want[7])
	}
	ticks := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(ticks) != 5 || !strings.Contains(ticks[0], "docker running") || !strings.Contains(ticks[1], "running on :") ||
		!strings.Contains(ticks[2], "turbopuffer key accepted, archiving to namespace hev-traces") ||
		!strings.Contains(ticks[3], "hevd installed, first scan started") || !strings.Contains(ticks[4], "dashboard running http://127.0.0.1:") {
		t.Fatalf("output:\n%s", out.String())
	}
	cfg, err := daemon.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LayerAPIKey != "tpuf_test" || cfg.LayerStore != "turbopuffer" || cfg.LayerNamespace != "hev-traces" {
		t.Fatalf("config = key %q store %q ns %q", cfg.LayerAPIKey, cfg.LayerStore, cfg.LayerNamespace)
	}
}

// A machine an older kit set up has a Postgres-era config, a launchd read side
// on the dashboard's port, and index state describing the Postgres archive.
// up retires the job, moves the config, and forgets the state with the daemon
// stopped.
func TestUpMovesAPostgresEraMachine(t *testing.T) {
	home, log := upSandbox(t)
	t.Setenv("FAKE_UNLOADED", "")
	writeConfig(t, home, "[layer]\nendpoint = \"http://127.0.0.1:8080\"\napi_key = \"local\"\nnamespace = \"hev-traces-local\"\nstore = \"pgvector\"\n\n[local]\nimage = \"hevlayer/layer-gateway:edge\"\nport = 8080\nproject = \"hev-kit\"\nserve_port = 8099\n")
	state := indexState(t, home)
	var out bytes.Buffer
	if err := runUp(context.Background(), &out, false); err != nil {
		t.Fatalf("%v\n%s", err, &out)
	}
	got := calls(t, log)
	retire := strings.Index(got, "launchctl bootout "+launchdDomain()+"/com.hev.test.serve")
	compose := strings.Index(got, "up --detach")
	if retire < 0 || compose < 0 || retire > compose {
		t.Fatalf("read-side job not retired before compose:\n%s", got)
	}
	if _, err := os.Stat(state); err == nil {
		t.Fatal("index state of the Postgres archive kept")
	}
	for _, want := range []string{"retired the launchd read side", "moved off the local Postgres store", "archiving to namespace hev-traces\n"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, &out)
		}
	}
	cfg, _ := daemon.LoadConfig()
	if cfg.LayerStore != "turbopuffer" || cfg.LayerNamespace != "hev-traces" || cfg.LayerAPIKey != "tpuf_test" {
		t.Fatalf("config not moved: %+v", cfg)
	}
}

// A second up needs nothing exported: the key comes from the config.
func TestUpReusesTheStoredKey(t *testing.T) {
	home, _ := upSandbox(t)
	managed(t, home)
	t.Setenv("TURBOPUFFER_API_KEY", "")
	var out bytes.Buffer
	if err := runUp(context.Background(), &out, true); err != nil {
		t.Fatalf("%v\n%s", err, &out)
	}
	if !strings.Contains(out.String(), "turbopuffer key accepted") {
		t.Fatalf("output:\n%s", &out)
	}
}

func TestEnsureLeavesAnIdenticalLoadedJobAlone(t *testing.T) {
	home, log := sandbox(t, "0")
	job := daemonJob("/tmp/bin/hev", home, filepath.Join(home, ".hev", "config.toml"))
	path := job.plistPath(home)
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte(job.plist()), 0o644)
	before, _ := os.Stat(path)
	changed, err := job.ensure(home, false)
	if err != nil || changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if got := calls(t, log); got != "launchctl print "+job.target() {
		t.Fatalf("calls:\n%s", got)
	}
	if after, _ := os.Stat(path); !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("plist rewritten")
	}
	// A changed binary restarts it even though the plist is the same.
	if changed, err := job.ensure(home, true); err != nil || !changed {
		t.Fatalf("restart: changed=%v err=%v", changed, err)
	}
	if got := calls(t, log); !strings.Contains(got, "bootout "+job.target()) || !strings.HasSuffix(got, "bootstrap "+launchdDomain()+" "+path) {
		t.Fatalf("calls:\n%s", got)
	}
}
