package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
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
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture; native Windows coverage is in platform/service tests")
	}
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
	fake("docker", "[ \"$1\" = info ] && exit "+dockerInfoExit+"\n[ \"$1\" = ps ] && [ -n \"$FAKE_DOCKER_PS\" ] && printf \"$FAKE_DOCKER_PS\\n\"\nexit 0\n")
	// `launchctl print` succeeding means the job is loaded; FAKE_UNLOADED
	// makes it fail, as it does for a job that was never installed.
	fake("launchctl", "[ \"$1\" = print ] && [ -n \"$FAKE_UNLOADED\" ] && exit 113\nexit 0\n")
	for _, env := range []string{"HEVLAYER_API_KEY_REF", "HEVLAYER_API_KEY_OP_REF", "LAYER_API_KEY_OP_REF", "LAYER_API_KEY", "TPUF_API_KEY", "HEVLAYER_ENDPOINT", "HEVLAYER_API_KEY", "HEVLAYER_NAMESPACE", "HEVLAYER_STORE", "HEV_LOCAL_IMAGE", "HEV_LOCAL_PORT", "HEV_LOCAL_SERVE_PORT", "HEV_LOCAL_KIT_IMAGE", "FAKE_UNLOADED", "FAKE_DOCKER_PS"} {
		t.Setenv(env, "")
	}
	// Config loading shells out to `gh`, and a current gh writes its own
	// state under HOME (~/.local/state/gh/device-id). It is faked and logged
	// like the others, and PATH holds the fakes and nothing else, so what a
	// test sees under its HOME is what kit put there on any machine.
	fake("gh", "exit 1\n")
	t.Setenv("PATH", bin)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
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
	if _, err := daemon.WriteFilesConfig(cfg.Local); err != nil {
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

// calls is what kit ran to do its work: launchctl (and docker, which only a
// retired config's migration may call). allCalls also
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

// No key is the files lane: the config names no key and no store, hevd is
// installed, and nothing at all runs in Docker.
func TestUpWithoutAKeyKeepsTracesInFiles(t *testing.T) {
	home, log := upSandbox(t)
	t.Setenv("HEVLAYER_API_KEY", "")
	withStore(t, "")
	var out bytes.Buffer
	if err := runUp(context.Background(), &out, false); err != nil {
		t.Fatalf("%v\n%s", err, &out)
	}
	got := calls(t, log)
	if strings.Contains(got, "docker") {
		t.Fatalf("docker called with no key:\n%s", got)
	}
	if !strings.Contains(got, "launchctl bootstrap") {
		t.Fatalf("hevd not installed:\n%s", got)
	}
	archive := filepath.Join(home, ".hev", "traces", "hev-traces")
	if !strings.Contains(out.String(), "traces are kept in files under "+archive) || strings.Contains(out.String(), "turbopuffer") {
		t.Fatalf("output:\n%s", &out)
	}
	if _, err := os.Stat(archive); err != nil {
		t.Fatalf("archive not created: %v", err)
	}
	cfg, _ := daemon.LoadConfig()
	if cfg.LayerStore != "" || cfg.LayerAPIKey != "" || !cfg.Local.Managed {
		t.Fatalf("config = key %q store %q managed %v", cfg.LayerAPIKey, cfg.LayerStore, cfg.Local.Managed)
	}
	// A second up changes nothing.
	out.Reset()
	before, _ := os.ReadFile(cfg.ConfigPath)
	if err := runUp(context.Background(), &out, false); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(cfg.ConfigPath); string(after) != string(before) {
		t.Fatalf("second up rewrote the config:\n%s", after)
	}
	// down unloads the job and touches no Docker.
	out.Reset()
	if err := runDown(context.Background(), &out); err != nil {
		t.Fatalf("%v\n%s", err, &out)
	}
	if got := calls(t, log); strings.Contains(got, "docker") {
		t.Fatalf("down called docker with no key:\n%s", got)
	}
}

func withStore(t *testing.T, store string) {
	t.Helper()
	was := upStore
	upStore = store
	t.Cleanup(func() { upStore = was })
}

func TestLaunchdLabelsAndCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("launchd fixture uses POSIX paths for macOS jobs")
	}
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
	// This config names no key, so it is a files host: there are no
	// containers to stop either.
	if got := out.String(); !strings.Contains(got, "no [local] block") || strings.Count(strings.TrimSpace(got), "\n") != 0 || strings.Contains(calls(t, log), "docker") {
		t.Fatalf("output = %q", got)
	}
}

// stub listens on an ephemeral loopback port and returns it.
func stub(t *testing.T, body string) int {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	n, _ := strconv.Atoi(port)
	return n
}

// upSandbox is a sandbox `up` can succeed in: no key, launchd jobs that are
// not loaded, and a stub answering on the dashboard port.
func upSandbox(t *testing.T) (home, log string) {
	t.Helper()
	onOS(t, "darwin")
	home, log = sandbox(t, "0")
	t.Setenv("FAKE_UNLOADED", "1")
	t.Setenv("HEVLAYER_API_KEY", "")
	withStore(t, "")
	t.Setenv("HEV_LOCAL_SERVE_PORT", strconv.Itoa(stub(t, "ok")))
	// The stub holds the dashboard port; it stands in for the `hev serve`
	// the fake launchctl does not start.
	was := portFree
	portFree = func(int) bool { return true }
	t.Cleanup(func() { portFree = was })
	return home, log
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
