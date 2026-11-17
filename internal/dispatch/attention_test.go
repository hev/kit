package dispatch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hev/kit/internal/attention"
)

func TestRunAttentionStates(t *testing.T) {
	m := Meta{Order: Order{Name: "kit-x", Host: "oven"}, Start: time.Date(2026, 11, 17, 1, 0, 0, 0, time.UTC)}
	end := m.Start.Add(time.Hour)
	for _, c := range []struct {
		name   string
		r      Result
		kind   string
		source string
		text   string
	}{
		{"needs-decision", Result{End: end, Outcome: OutcomeNeedsDecision, Key: "db", Question: "Postgres or SQLite?", PR: "https://github.com/hev/kit/pull/9"}, attention.KindNeedsYou, attention.SourceRun, "needs-decision db: Postgres or SQLite?"},
		{"blocked", Result{End: end, Outcome: OutcomeBlocked, Reason: "no token"}, attention.KindNeedsYou, attention.SourceRun, "blocked: no token"},
		{"failed", Result{End: end, ExitCode: 2, Error: "harness crashed"}, attention.KindNeedsYou, attention.SourceRun, "failed (exit 2): harness crashed"},
		{"needs-review", Result{End: end, NeedsReview: true}, attention.KindNeedsYou, attention.SourceRun, "needs review: validation failed twice"},
		{"timeout", Result{End: end, ExitCode: 1, Timeout: true, Stopped: true}, attention.KindNeedsYou, attention.SourceRun, "timed out"},
		{"halt-escalated", Result{End: end, Halted: true, HaltEscalated: true, ExitCode: 1}, attention.KindNeedsYou, attention.SourceSteer, "halt escalated to a hard stop"},
		{"done", Result{End: end}, attention.KindDone, attention.SourceRun, "done"},
	} {
		t.Run(c.name, func(t *testing.T) {
			n, ok := runAttention(m, c.r)
			if !ok || n.Event != c.kind || n.Source != c.source || n.Text != c.text || n.Host != "oven" || n.Name != "kit-x" {
				t.Fatalf("%v %+v", ok, n)
			}
			if n.Links.Run != "hev dispatch show kit-x --host oven" || n.Links.PR != c.r.PR {
				t.Fatalf("links %+v", n.Links)
			}
			again, _ := runAttention(m, c.r)
			if again.ID != n.ID {
				t.Fatal("ID is not stable")
			}
		})
	}
	for _, quiet := range []Result{{End: end, Stopped: true, ExitCode: 1}, {End: end, Halted: true}} {
		if n, ok := runAttention(m, quiet); ok {
			t.Fatalf("a requested stop or soft halt is not a notice: %+v", n)
		}
	}
	// Validators and repeating rounds stay out.
	called := 0
	old := emitAttention
	emitAttention = func(attention.Notice, string) { called++ }
	t.Cleanup(func() { emitAttention = old })
	noticeRunEnd(t.TempDir(), Meta{Parent: "p", Order: m.Order}, Result{End: end, ExitCode: 1})
	noticeRunEnd(t.TempDir(), Meta{Order: Order{Name: "r", Every: "1h"}}, Result{End: end, ExitCode: 1})
	noticeRunEnd(t.TempDir(), m, Result{End: end, ExitCode: 1})
	if called != 1 {
		t.Fatal(called)
	}
}

// fakeSlack is a chat.postMessage endpoint that records posts.
type fakeSlack struct {
	mu    sync.Mutex
	posts []map[string]any
	fail  bool
}

func (f *fakeSlack) serve(t *testing.T) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.fail {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.posts = append(f.posts, body)
		w.Write([]byte(`{"ok":true,"channel":"D0DM","ts":"1700000000.0000` + string(rune('0'+len(f.posts))) + `"}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func (f *fakeSlack) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.posts) }

// slackHost points this process's default emitter at a fake Slack with
// [notify.slack] configured, and an outbox of its own.
func slackHost(t *testing.T, f *fakeSlack, extra string) string {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfg, []byte("[notify.slack]\nchannel = \"U0A4L3ENZ39\"\ntoken = \"secret://SLACK_BOT_TOKEN\"\n"+extra), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_CONFIG", cfg)
	t.Setenv(attention.RootEnv, filepath.Join(dir, "outbox"))
	t.Setenv(attention.SlackAPIEnv, f.serve(t))
	old := attention.ResolveToken
	attention.ResolveToken = func(context.Context, string) (string, error) { return "xoxb-fake", nil }
	t.Cleanup(func() { attention.ResolveToken = old })
	return dir
}

func needsYouDriver(t *testing.T) OrchestratorDriver {
	d := noticeDriver(t, nil)
	d.Session = func(_ context.Context, _ OrchestratorSpec, dir string, _ OrchestratorPacket) error {
		if err := os.WriteFile(filepath.Join(dir, "reply.md"), []byte("[needs you] pick the database for kit-x\nDetails follow."), 0600); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("Waiting on the owner."), 0600)
	}
	return d
}

// A pull-route orchestrator host with [notify.slack] posts the needs-you
// reply itself; the owner receipt carries the notice for hosts without it,
// and a pulling host posts it once however often it is redelivered.
func TestOrchestratorNeedsYouPostsOnPullRouteAndPulledHost(t *testing.T) {
	orchHome(t)
	slack := &fakeSlack{}
	slackHost(t, slack, "")
	raw := strings.Replace(orchestratorExample, "reply: owner", "reply: owner\nnotify_route: pull", 1)
	raw = strings.Replace(raw, "authority: [say,", "authority: [notify, say,", 1)
	s, err := ApplyOrchestrator(raw, false)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := orchestratorState(s.Name)
	if err = RunOrchestratorRound(context.Background(), s, &st, needsYouDriver(t)); err != nil {
		t.Fatal(err)
	}
	if slack.count() != 1 {
		t.Fatalf("posts %d", slack.count())
	}
	first := slack.posts[0]
	if text := first["text"].(string); !strings.HasPrefix(text, "[needs you] "+s.Name+" on test-host: pick the database for kit-x\nRun: hev orchestrate show "+s.Name) {
		t.Fatalf("text %q", text)
	}
	if first["channel"] != "U0A4L3ENZ39" || first["unfurl_links"] != false {
		t.Fatalf("post %+v", first)
	}
	// Thread state sits beside the orchestrator's.
	if _, err := os.Stat(filepath.Join(OrchestratorDir(s.Name), attention.ThreadFile)); err != nil {
		t.Fatal(err)
	}
	if entries := attention.ReadStatus(OrchestratorDir(s.Name)); len(entries) != 1 || entries[0].Slack != attention.StatusDelivered {
		t.Fatalf("status %+v", entries)
	}
	r, err := Handle(Request{Action: "orchestrator-notices", Name: s.Name})
	if err != nil {
		t.Fatal(err)
	}
	var carried *attention.Notice
	for _, n := range r.Notices {
		if n.Kind == "reply" {
			carried = n.Attention
		}
	}
	if carried == nil || carried.Event != attention.KindNeedsYou {
		t.Fatalf("receipt lacks its notice: %+v", r.Notices)
	}

	// The laptop pulls it: its own outbox, same Slack config.
	laptop := &fakeSlack{}
	slackHost(t, laptop, "")
	for i := 0; i < 2; i++ {
		if err := PulledAttention(context.Background(), "win-oven", carried); err != nil {
			t.Fatal(err)
		}
	}
	if laptop.count() != 1 || !strings.Contains(laptop.posts[0]["text"].(string), " on win-oven: ") {
		t.Fatalf("pulled posts %+v", laptop.posts)
	}
}

// A down Slack never fails a round; the post stays queued with backoff and
// ls reports it.
func TestSlackFailureDoesNotFailRound(t *testing.T) {
	orchHome(t)
	slack := &fakeSlack{fail: true}
	slackHost(t, slack, "")
	s := applyOrch(t)
	st, _ := orchestratorState(s.Name)
	if err := RunOrchestratorRound(context.Background(), s, &st, needsYouDriver(t)); err != nil {
		t.Fatalf("round failed on a Slack outage: %v", err)
	}
	if st.State == "failed" || st.Rounds != 1 {
		t.Fatalf("state %+v", st)
	}
	line := attentionStatus(OrchestratorDir(s.Name))
	if !strings.Contains(line, "slack notice retrying (attempt 1") || !strings.Contains(line, "HTTP 502") {
		t.Fatalf("status %q", line)
	}
	var r Response
	if err := addOrchestratorRows(&r); err != nil {
		t.Fatal(err)
	}
	if len(r.Rows) != 1 || r.Rows[0].Attention != line {
		t.Fatalf("rows %+v", r.Rows)
	}
}

func TestStalledExpediterEmitsNeedsYou(t *testing.T) {
	orchHome(t)
	var got []attention.Notice
	old := queueAttention
	queueAttention = func(n attention.Notice, _ string) { got = append(got, n) }
	t.Cleanup(func() { queueAttention = old })
	s := OrchestratorSpec{Name: "expedite-owner", Host: "oven", Reply: "owner"}
	n := orchestratorAttention(s, attention.KindNeedsYou, "k", "stalled", "stalled: [needs you] no PR")
	noticeOrchestrator(s, n)
	if len(got) != 1 || got[0].Source != attention.SourceExpedite || got[0].Name != "expedite-owner" {
		t.Fatalf("%+v", got)
	}
	if text, ok := replyNeedsYou("  [needs you] decide\nmore"); !ok || text != "decide" {
		t.Fatal(text, ok)
	}
	if _, ok := replyNeedsYou("Ready to merge\n[needs you] later line"); ok {
		t.Fatal("only the first line counts")
	}
}

func TestMemberNoticeCarriesAttention(t *testing.T) {
	orchHome(t)
	m := Meta{Sender: "teammate", Order: Order{Name: "kit-x", Host: "oven", Reply: "lap"}, Start: time.Now().UTC()}
	r := Result{End: time.Now().UTC(), Outcome: OutcomeBlocked, Reason: "no access"}
	if _, err := queueMemberNotice([]string{"peers", "send", "{reply}", "x"}, m, r); err != nil {
		t.Fatal(err)
	}
	pending, err := PendingMemberNotices("teammate")
	if err != nil || len(pending) != 1 || pending[0].Attention == nil {
		t.Fatal(pending, err)
	}
	local, _, err := LocalMemberNotice("shared", pending[0])
	if err != nil || local.Attention == nil || local.Attention.Name != "kit-x" || local.Attention.Text != "blocked: no access" {
		t.Fatalf("%+v %v", local.Attention, err)
	}
}

func TestQueuedAdmissionFailureEmits(t *testing.T) {
	var got []attention.Notice
	old := emitAttention
	emitAttention = func(n attention.Notice, _ string) { got = append(got, n) }
	t.Cleanup(func() { emitAttention = old })
	dir := t.TempDir()
	m := Meta{Order: Order{Name: "kit-q", Host: "oven"}, Start: time.Now().UTC()}
	_ = WriteJSON(filepath.Join(dir, "meta.json"), m)
	_ = WriteJSON(filepath.Join(dir, "result.json"), Result{AdmissionFailed: true, ExitCode: 1, End: time.Now().UTC(), Error: "git: no origin"})
	noticeAdmissionFailure(dir)
	if len(got) != 1 || got[0].Event != attention.KindNeedsYou || got[0].Text != "failed (exit 1): git: no origin" {
		t.Fatalf("%+v", got)
	}
	_ = WriteJSON(filepath.Join(dir, "result.json"), Result{ExitCode: 1, End: time.Now().UTC()})
	noticeAdmissionFailure(dir)
	if len(got) != 1 {
		t.Fatal("a run that did not fail admission is left to its supervisor")
	}
}
