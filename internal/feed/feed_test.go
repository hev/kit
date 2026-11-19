package feed_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/feed"
	"github.com/hev/kit/internal/hooks"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadDefaults(t *testing.T) {
	c, err := feed.Load(writeConfig(t, `
[feed]
channel = "C0A6L0UFU6R"
token = "op://layer-factory/SLACK_BOT_TOKEN/credential"
model_key = "op://layer-factory/hevmind-claude/credential"
skip = ["board-post"]
`))
	if err != nil {
		t.Fatal(err)
	}
	if !c.Enabled || c.Model != "claude-haiku-5-5" || c.MaxUploadBytes != 50<<20 {
		t.Fatalf("config = %+v", c)
	}
	if strings.Join(c.Boards, ",") != "feedback" || !c.WantsBoard("Feedback") || c.WantsBoard("research") {
		t.Fatalf("boards = %v", c.Boards)
	}
	if !c.Wants("stop") || !c.Wants("session-start") || c.Wants("pre-tool") || c.Wants("board-post") {
		t.Fatalf("events = %v skip %v", c.Events, c.Skip)
	}
	home, _ := os.UserHomeDir()
	if !c.IsClientPath(filepath.Join(home, "workspace", "clients", "acme")) || c.IsClientPath(filepath.Join(home, "workspace", "clientsx")) {
		t.Fatal("client roots")
	}
}

func TestLoadRefusesTokenValues(t *testing.T) {
	for _, body := range []string{
		"[feed]\nchannel = \"C1\"\ntoken = \"xoxb-123\"\n",
		"[feed]\nchannel = \"C1\"\ntoken = \"secret://slack\"\nmodel_key = \"sk-ant-123\"\n",
		"[feed]\nchannel = \"C1\"\ntoken = \"secret://slack\"\nevents = [\"stopp\"]\n",
		"[feed]\ntoken = \"secret://slack\"\n",
	} {
		c, err := feed.Load(writeConfig(t, body))
		if err == nil || c.Enabled {
			t.Fatalf("%q: accepted", body)
		}
		if strings.Contains(err.Error(), "xoxb-123") || strings.Contains(err.Error(), "sk-ant-123") {
			t.Fatalf("error echoes the value: %v", err)
		}
	}
	c, err := feed.Load(writeConfig(t, "[feed]\nenabled = false\n"))
	if err != nil || c.Enabled {
		t.Fatalf("disabled table: %+v %v", c, err)
	}
	c, err = feed.Load(writeConfig(t, "[notify]\nclient = true\n"))
	if err != nil || c.Enabled || !c.Client {
		t.Fatalf("no table: %+v %v", c, err)
	}
}

func TestEvidenceDir(t *testing.T) {
	for cmd, want := range map[string]string{
		"node ~/workspace/shell/hosts/mini/acceptance-recorder/record.cjs ~/jobs/x/flow.cjs ~/jobs/x/evidence-001 http://100.126.12.95:8800": "HOME/jobs/x/evidence-001",
		"cd /w && node record.cjs flow.cjs out-2":      "/w/job/out-2",
		"node studio/record.cjs '/tmp/f.cjs' '/tmp/e'": "/tmp/e",
		"git commit -m record.cjs":                     "",
		"cat record.cjs":                               "",
	} {
		got, ok := feed.EvidenceDir(cmd, "/w/job")
		home, _ := os.UserHomeDir()
		want = strings.Replace(want, "HOME", home, 1)
		if (want == "") == ok || got != want {
			t.Errorf("%q: got %q %v, want %q", cmd, got, ok, want)
		}
	}
}

func TestNormalizeKeyMatchesNotices(t *testing.T) {
	if got := feed.NormalizeKey("run:Adams-Mac-mini.local/x"); got != "run:mini/x" {
		t.Fatal(got)
	}
	if feed.RunKey("oven", "x") != "run:oven/x" || feed.OrchestratorKey("oven", "o") != "orchestrator:oven/o" {
		t.Fatal("keys")
	}
}

// The built-in observers spool through hev hook's lifecycle path without
// running a handler or touching the network.
func TestRunLifecycleSpoolsForFeed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(feed.DirEnv, dir)
	t.Setenv("HEV_CONFIG", writeConfig(t, "[feed]\nchannel = \"C1\"\ntoken = \"secret://slack\"\n"))
	t.Setenv("PATH", t.TempDir())
	spawned := 0
	old := feed.Spawn
	feed.Spawn = func() error { spawned++; return nil }
	t.Cleanup(func() { feed.Spawn = old })
	notice := `{"schema":"hev.attention.event/v1","id":"att_1","event":"needs-you","source":"run","name":"r1","host":"oven","text":"needs-decision db: A or B?","thread":"run:oven/r1"}`
	hooks.RunLifecycle(context.Background(), "needs-you", []byte(notice), os.Getenv("PATH"), hooks.Config{Enabled: true, Timeout: hooks.DefaultTimeout}, hooks.Log{})
	files := feed.Pending(dir)
	if len(files) != 1 || spawned != 1 {
		t.Fatalf("spool = %v, spawned %d", files, spawned)
	}
	var rec feed.Record
	b, _ := os.ReadFile(files[0])
	if err := json.Unmarshal(b, &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Kind != "needs-you" || rec.Thread.Key != "run:oven/r1" || rec.Thread.Kind != feed.ThreadRun {
		t.Fatalf("record = %+v", rec)
	}
	// HEV_FEED=off silences it.
	t.Setenv(feed.OffEnv, "off")
	hooks.RunLifecycle(context.Background(), "done", []byte(notice), os.Getenv("PATH"), hooks.Config{Enabled: true, Timeout: hooks.DefaultTimeout}, hooks.Log{})
	if len(feed.Pending(dir)) != 1 {
		t.Fatal("HEV_FEED=off still spooled")
	}
}

func TestObserveHookSkipsToolsAndActiveStops(t *testing.T) {
	dir := t.TempDir()
	cfg := feed.Config{Enabled: true, Channel: "C1", Token: "secret://s", Events: feed.DefaultEvents, Boards: []string{"feedback"}}
	o := feed.Observer{Config: &cfg, Dir: dir, Host: "oven", Kick: func(string) {}}
	for _, ev := range []map[string]any{
		{"event": "pre-tool", "harness": "claude", "tool": map[string]any{"name": "Bash", "command": "ls"}},
		{"event": "stop", "harness": "claude", "stop_hook_active": true},
		{"event": "prompt-submit", "harness": "claude", "prompt": "hi"},
		{"event": "session-start", "harness": "claude", "target": "orch:o", "cwd": "/d/orch-o/rounds/003"},
	} {
		b, _ := json.Marshal(ev)
		o.ObserveHook(b)
	}
	if n := len(feed.Pending(dir)); n != 0 {
		t.Fatalf("%d records spooled", n)
	}
}
