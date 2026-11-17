package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/index"
)

// The steps run in order, each after the one it depends on: the config, the
// capture daemon, the skills, the dashboard. Nothing touches Docker.
func TestUpStepOrderOnFiles(t *testing.T) {
	home, log := upSandbox(t)
	var out bytes.Buffer
	if err := runUp(context.Background(), &out, false); err != nil {
		t.Fatalf("%v\n%s", err, &out)
	}
	domain := launchdDomain()
	agents := filepath.Join(home, "Library", "LaunchAgents")
	want := []string{
		"launchctl print " + domain + "/com.hev.test.hevd",
		"launchctl bootstrap " + domain + " " + filepath.Join(agents, "com.hev.test.hevd.plist"),
		"launchctl print " + domain + "/com.hev.test.serve",
		"launchctl bootstrap " + domain + " " + filepath.Join(agents, "com.hev.test.serve.plist"),
	}
	var got []string
	for _, line := range strings.Split(calls(t, log), "\n") {
		if len(got) == 0 || got[len(got)-1] != line {
			got = append(got, line)
		}
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// The config exists before the first job is installed.
	if first, _ := os.ReadFile(log + ".cfg"); strings.HasPrefix(string(first), "launchctl") && strings.TrimSpace(string(first)) != want[0] {
		t.Fatalf("first job call that saw a config file = %q, want %q", first, want[0])
	} else if len(first) == 0 {
		t.Fatal("no call saw the config file")
	}
	plist, _ := os.ReadFile(filepath.Join(agents, "com.hev.test.serve.plist"))
	for _, want := range []string{"<string>serve</string>", "<string>127.0.0.1</string>", "<string>" + os.Getenv("HEV_LOCAL_SERVE_PORT") + "</string>"} {
		if !strings.Contains(string(plist), want) {
			t.Fatalf("serve plist missing %s:\n%s", want, plist)
		}
	}
	for _, want := range []string{"traces are kept in files under", "hevd installed", "dashboard running at http://127.0.0.1:" + os.Getenv("HEV_LOCAL_SERVE_PORT")} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, &out)
		}
	}
	if strings.Contains(strings.ToLower(out.String()), "docker") || strings.Contains(strings.ToLower(out.String()), "postgres") {
		t.Fatalf("output mentions docker or postgres:\n%s", &out)
	}

	// --no-dashboard installs no dashboard job.
	os.Truncate(log, 0)
	os.Remove(filepath.Join(agents, "com.hev.test.serve.plist"))
	out.Reset()
	if err := runUp(context.Background(), &out, true); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(calls(t, log), "serve") {
		t.Fatalf("--no-dashboard installed the dashboard:\n%s", calls(t, log))
	}
}

// Bring-your-own Turbopuffer is hosted-only: choosing it, or `up` on a config
// its old local-gateway lane wrote, is one line saying so, before anything
// runs or is written. --store files moves such a config to files and drops
// the provider key.
func TestUpTurbopufferIsHostedOnly(t *testing.T) {
	home, log := upSandbox(t)
	withStore(t, "turbopuffer")
	t.Setenv("HEVLAYER_API_KEY", "tpuf_test")
	var out bytes.Buffer
	err := runUp(context.Background(), &out, false)
	if err == nil || strings.Contains(err.Error(), "\n") || !strings.Contains(err.Error(), "hosted-only") {
		t.Fatalf("--store turbopuffer: %v", err)
	}
	old := "[layer]\nendpoint = \"http://127.0.0.1:8080\"\napi_key = \"tpuf_test\"\nnamespace = \"hev-traces\"\nstore = \"turbopuffer\"\n\n[local]\nport = 8080\nproject = \"hev-test\"\n"
	path := writeConfig(t, home, old)
	withStore(t, "")
	t.Setenv("HEVLAYER_API_KEY", "")
	err = runUp(context.Background(), &out, false)
	if err == nil || !strings.Contains(err.Error(), "hosted-only") || !strings.Contains(err.Error(), "--store files") {
		t.Fatalf("up on a BYO config: %v", err)
	}
	if got := allCalls(t, log); got != "" || out.Len() != 0 {
		t.Fatalf("ran before refusing:\n%s\n%s", got, &out)
	}
	if got, _ := os.ReadFile(path); string(got) != old {
		t.Fatalf("config changed:\n%s", got)
	}
	withStore(t, "files")
	if err := runUp(context.Background(), &out, true); err != nil {
		t.Fatalf("--store files: %v\n%s", err, &out)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "tpuf_test") || strings.Contains(string(raw), "turbopuffer") {
		t.Fatalf("provider key or store kept:\n%s", raw)
	}
	if strings.Contains(calls(t, log), "docker") {
		t.Fatalf("docker called:\n%s", calls(t, log))
	}
}

func TestUpStoreChoices(t *testing.T) {
	upSandbox(t)
	var out bytes.Buffer
	withStore(t, "sqlite")
	if err := runUp(context.Background(), &out, true); err == nil || !strings.Contains(err.Error(), "expected hosted or files") {
		t.Fatalf("--store sqlite: %v", err)
	}
	withStore(t, daemon.RetiredStore)
	if err := runUp(context.Background(), &out, true); err == nil || !strings.Contains(err.Error(), "retired") {
		t.Fatalf("--store %s: %v", daemon.RetiredStore, err)
	}
	withStore(t, "hosted")
	if err := runUp(context.Background(), &out, true); err == nil || !strings.Contains(err.Error(), "requires HEVLAYER_API_KEY") {
		t.Fatalf("--store hosted with no key: %v", err)
	}
}

// down unloads the daemon and the dashboard (KeepAlive jobs are unloaded,
// not signalled), removes their plists, keeps the index state, and touches no
// Docker.
func TestDownUnloadsTheDaemonAndTheDashboard(t *testing.T) {
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
	domain := launchdDomain()
	got := calls(t, log)
	for _, want := range []string{"launchctl bootout " + domain + "/com.hev.test.hevd", "launchctl bootout " + domain + "/com.hev.test.serve"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s:\n%s", want, got)
		}
	}
	if strings.Contains(got, "docker") {
		t.Fatalf("down called docker:\n%s", got)
	}
	if b, _ := os.ReadFile(state); string(b) != stateBytes {
		t.Fatal("index state reset by down")
	}
	if left, _ := os.ReadDir(plists); len(left) != 0 {
		t.Fatalf("plists left behind: %v", left)
	}
}

func TestDownWithoutLaunchd(t *testing.T) {
	onOS(t, "linux")
	home, log := sandbox(t, "0")
	managed(t, home)
	var out bytes.Buffer
	if err := runDown(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	if got := calls(t, log); strings.Contains(got, "launchctl") || strings.Contains(got, "docker") {
		t.Fatalf("calls on linux:\n%s", got)
	}
	if !strings.Contains(out.String(), "no launchd on linux") {
		t.Fatalf("output = %q", out.String())
	}
}

const retiredConfig = "[layer]\nendpoint = \"http://127.0.0.1:8080\"\napi_key = \"local\"\nnamespace = \"hev-traces\"\nstore = \"" + daemon.RetiredStore + "\"\n\n[local]\nimage = \"hevlayer/layer-gateway:edge\"\nport = 8080\nproject = \"hev-test\"\nserve_port = 8099\n"

// With no container to start, a retired config is re-indexed from the
// transcripts still on disk: the index state is reset and up says how many
// sessions could not be recovered. kit starts, stops and removes nothing.
func TestRetiredLaneReindexesWhenItsContainersDoNotStart(t *testing.T) {
	for _, tc := range []struct{ name, info, ps string }{
		{"no docker daemon", "1", ""},
		{"no containers", "0", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, log := upSandbox(t)
			t.Setenv("FAKE_DOCKER_PS", tc.ps)
			if tc.info == "1" {
				// sandbox's docker answers info with the exit it was given.
				os.WriteFile(filepath.Join(filepath.Dir(log), "docker"), []byte("#!/bin/sh\necho \"docker $*\" >> \""+log+"\"\n[ \"$1\" = info ] && exit 1\nexit 0\n"), 0o755)
			}
			writeConfig(t, home, retiredConfig)
			kept := filepath.Join(home, ".claude", "projects", "-w", "kept.jsonl")
			os.MkdirAll(filepath.Dir(kept), 0o755)
			os.WriteFile(kept, []byte("{}\n"), 0o600)
			state := filepath.Join(home, ".hev", "index-state.json")
			units := map[string]map[string]string{"units": {
				"aaaa:" + kept: "1",
				"bbbb:" + kept: "1", // the same transcript in another archive
				"aaaa:" + filepath.Join(home, ".claude", "projects", "-w", "gone.jsonl"):         "1",
				"aaaa:" + filepath.Join(home, ".local", "share", "opencode", "db") + "#ses_gone": "1",
				"span:0123456789abcdef": "1",
			}}
			b, _ := json.Marshal(units)
			os.WriteFile(state, b, 0o600)
			var out bytes.Buffer
			if err := runUp(context.Background(), &out, true); err != nil {
				t.Fatalf("%v\n%s", err, &out)
			}
			for _, want := range []string{"retired local Docker lane", "re-indexing the transcripts still on disk", "1 sessions will be re-indexed", "2 sessions could not be recovered", "traces are kept in files"} {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("output missing %q:\n%s", want, &out)
				}
			}
			if _, err := os.Stat(state); !os.IsNotExist(err) {
				t.Fatalf("index state not reset: %v", err)
			}
			for _, verb := range []string{"docker start", "docker stop", "docker rm", "docker volume", "docker compose"} {
				if strings.Contains(calls(t, log), verb) {
					t.Fatalf("%s called:\n%s", verb, calls(t, log))
				}
			}
			if cfg, _ := daemon.LoadConfig(); cfg.LayerStore != "" || cfg.LayerAPIKey != "" {
				t.Fatalf("config still on the retired lane: %+v", cfg)
			}
		})
	}
}

// When the retired lane's containers start, every row is exported from its
// gateway into the file archive, the containers kit started are stopped
// again, and nothing is removed.
func TestRetiredLaneExportsWhenItsContainersStart(t *testing.T) {
	home, log := upSandbox(t)
	t.Setenv("FAKE_DOCKER_PS", "c0ffee exited gateway\\nbeef running postgres")
	now := time.Now().UTC().Format(time.RFC3339)
	pages := map[string]string{
		"hev-traces":          `{"rows":[{"id":"chunk-1","session_id":"s1old","seq":0,"ts":"` + now + `","harness":"claude_code","role":"assistant","text":"the exported chunk about flaky deploys","vector":[0.1]}]}`,
		"hev-traces-blocks":   `{"rows":[{"id":"block-1","session_id":"s1old","seq":0,"text":"block text"}]}`,
		"hev-traces-sessions": `{"rows":[{"id":"s1old","session_id":"s1old","harness":"claude_code","start":1,"end":2,"summary":"an old session"}]}`,
		"hev-traces-evals":    `{"rows":[{"id":"e1","session_id":"s1old","ts":"2026-11-01T10:00:00Z","role":"validator","instance":"","host":"h","poor":false,"summary":"ok","marks":"{\"quality\":2}","evidence":"{}","findings":"[]","text":"ok"}]}`,
	}
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.Write([]byte(`{"status":"ok","version":"0.7.3"}`))
			return
		}
		ns := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v2/namespaces/"), "/query")
		page, ok := pages[ns]
		if !ok {
			http.Error(w, `{"error":"namespace not found"}`, http.StatusNotFound)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["filters"] != nil {
			page = `{"rows":[]}`
		}
		w.Write([]byte(page))
	}))
	defer gw.Close()
	writeConfig(t, home, strings.Replace(retiredConfig, "http://127.0.0.1:8080", gw.URL, 1))
	var out bytes.Buffer
	if err := runUp(context.Background(), &out, true); err != nil {
		t.Fatalf("%v\n%s", err, &out)
	}
	for _, want := range []string{"started 1 stopped container(s) of hev-test", "exported 1 sessions, 1 chunks, 1 blocks, 1 evals, 0 instruction versions", "stopped them again", "Docker volume is left"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, &out)
		}
	}
	got := calls(t, log)
	if !strings.Contains(got, "docker start c0ffee") || !strings.Contains(got, "docker stop c0ffee") || strings.Contains(got, "beef") || strings.Contains(got, "docker rm") || strings.Contains(got, "volume rm") {
		t.Fatalf("docker calls:\n%s", got)
	}
	store, err := filearchive.Open(filepath.Join(home, ".hev", "archive"), "hev-traces")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := store.ListSessionRows(10, nil)
	if err != nil || len(rows) != 1 || rows[0].Summary != "an old session" {
		t.Fatalf("sessions = %+v, %v", rows, err)
	}
	hits, err := store.SearchHits("flaky deploy", 5, nil)
	if err != nil || len(hits) != 1 {
		t.Fatalf("hits = %+v, %v", hits, err)
	}
	if evals, _ := store.ListEvalRows(nil); len(evals) != 1 {
		t.Fatalf("evals = %+v", evals)
	}
	if st := index.LoadState(); len(st.Units) != 0 {
		t.Fatalf("index state = %+v", st)
	}
}
