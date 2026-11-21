package feed_test

import (
	"context"
	"encoding/json"
	"fmt"
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

// A test that never set up a feed cannot write one or start a sender, even
// with a [feed] table in its config: kit's feed is inert under go test
// unless HEV_FEED_DIR points somewhere the test owns. This is what keeps
// dispatch and peers tests from leaving a spool or a detached `hev feed
// send` in their temp HOME.
func TestFeedInertUnderTestWithoutDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(feed.DirEnv, "")
	t.Setenv(feed.OffEnv, "")
	t.Setenv("HEV_CONFIG", writeConfig(t, "[feed]\nchannel = \"C1\"\ntoken = \"secret://slack\"\n"))
	t.Setenv("PATH", t.TempDir())
	spawned := 0
	old := feed.Spawn
	feed.Spawn = func() error { spawned++; return nil }
	t.Cleanup(func() { feed.Spawn = old })
	notice := `{"schema":"hev.attention.event/v1","id":"att_1","event":"needs-you","source":"run","name":"r1","host":"oven","text":"q?","thread":"run:oven/r1"}`
	hooks.RunLifecycle(context.Background(), "needs-you", []byte(notice), os.Getenv("PATH"), hooks.Config{Enabled: true, Timeout: hooks.DefaultTimeout}, hooks.Log{})
	feed.ObserveLifecycle("done", []byte(notice))
	feed.ObserveHook([]byte(`{"event":"stop","harness":"claude","session_id":"s1","cwd":"/tmp"}`))
	if err := os.MkdirAll(feed.SpoolDir(feed.Dir()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := feed.Add(feed.Dir(), feed.Record{Kind: "stop"}); err != nil {
		t.Fatal(err)
	}
	feed.Wake() // hevd's cycle: records wait, but the feed is off.
	if spawned != 0 {
		t.Fatalf("sender started %d times under test", spawned)
	}
	if n := len(feed.Pending(feed.Dir())); n != 1 {
		t.Fatalf("observers spooled under test: %d records", n)
	}
}

func TestBoardReplyReplayUsesLiveBoardThread(t *testing.T) {
	dir := t.TempDir()
	cfg := feed.Config{Enabled: true, Events: []string{feed.KindBoardPost}, Boards: []string{"feedback"}}
	obs := feed.Observer{Config: &cfg, Dir: dir, Host: "test-host", Kick: func(string) {}}
	for _, mount := range []string{"", "/engineering", "/clients/t/42"} {
		url := "https://user:password@board.example" + mount + "/t/42?token=secret#fragment"
		for _, message := range []string{
			`{"board":"feedback","kind":"post","id":"42","url":%q}`,
			`{"board":"feedback","kind":"reply","thread_id":42,"id":"second","url":%q}`,
		} {
			obs.ObserveLifecycle(feed.KindBoardPost, []byte(fmt.Sprintf(message, url)))
		}
		obs.ObserveHook([]byte(fmt.Sprintf(`{"event":"board-reply","data":{"board":"feedback","thread_id":42,"board_origin":%q}}`, "https://board.example"+mount)))
	}
	obs.ObserveHook([]byte(`{"event":"board-reply","data":{"board":"hidden","thread_id":42,"board_origin":"https://board.example"}}`))
	counts := map[string]int{}
	for _, path := range feed.Pending(dir) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var rec feed.Record
		if err := json.Unmarshal(raw, &rec); err != nil {
			t.Fatal(err)
		}
		if rec.Kind != feed.KindBoardPost {
			t.Fatalf("unexpected board record: %s", raw)
		}
		counts[rec.Thread.Key]++
	}
	if len(counts) != 3 || counts["board:https://board.example/42"] != 3 ||
		counts["board:https://board.example/engineering/42"] != 3 ||
		counts["board:https://board.example/clients/t/42/42"] != 3 {
		t.Fatalf("unexpected board keys: %v", counts)
	}
}
