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
		// Only an open decision and a hard halt need someone; every other
		// end is a record on the opt-in done kind.
		{"blocked", Result{End: end, Outcome: OutcomeBlocked, Reason: "no token"}, attention.KindDone, attention.SourceRun, "blocked: no token"},
		{"failed", Result{End: end, ExitCode: 2, Error: "harness crashed"}, attention.KindDone, attention.SourceRun, "failed (exit 2): harness crashed"},
		{"needs-review", Result{End: end, NeedsReview: true}, attention.KindDone, attention.SourceRun, "needs review: validation failed twice"},
		{"timeout", Result{End: end, ExitCode: 1, Timeout: true, Stopped: true}, attention.KindDone, attention.SourceRun, "timed out"},
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
	// Validators stay out; a repeating round speaks only when it did not
	// end done, never for an uneventful round.
	called := 0
	old := emitAttention
	emitAttention = func(attention.Notice, string) { called++ }
	t.Cleanup(func() { emitAttention = old })
	noticeRunEnd(t.TempDir(), Meta{Parent: "p", Order: m.Order}, Result{End: end, ExitCode: 1})
	noticeRunEnd(t.TempDir(), Meta{Order: Order{Name: "r", Every: "1h"}}, Result{End: end})
	noticeRunEnd(t.TempDir(), Meta{Order: Order{Name: "r", Every: "1h"}}, Result{End: end, ExitCode: 1})
	noticeRunEnd(t.TempDir(), m, Result{End: end, ExitCode: 1})
	if called != 2 {
		t.Fatal(called)
	}
}

// needs-you is keyed by its subject: the run plus the decision key and
// question, or the halt. The same subject from another attempt (a rework, a
// later round) is the same ID; a changed key, question or halt is new.
func TestNeedsYouSubjectKey(t *testing.T) {
	m := Meta{Order: Order{Name: "kit-x", Host: "oven"}, Start: time.Date(2026, 11, 17, 1, 0, 0, 0, time.UTC)}
	ask := func(end time.Time, key, q string) string {
		n, ok := runAttention(m, Result{End: end, Outcome: OutcomeNeedsDecision, Key: key, Question: q})
		if !ok || n.Event != attention.KindNeedsYou {
			t.Fatalf("%v %+v", ok, n)
		}
		return n.ID
	}
	first := ask(m.Start.Add(time.Hour), "db", "Postgres or SQLite?")
	if again := ask(m.Start.Add(3*time.Hour), "db", "Postgres   or\nSQLite?"); again != first {
		t.Fatal("the same decision from a later attempt is a new ID")
	}
	if ask(m.Start.Add(time.Hour), "db", "Postgres or MySQL?") == first || ask(m.Start.Add(time.Hour), "store", "Postgres or SQLite?") == first {
		t.Fatal("a changed question or key kept the ID")
	}
	other := m
	other.Start = m.Start.Add(24 * time.Hour)
	if n, _ := runAttention(other, Result{End: other.Start, Outcome: OutcomeNeedsDecision, Key: "db", Question: "Postgres or SQLite?"}); n.ID == first {
		t.Fatal("another run of the same name shares the ID")
	}
	halt := func(end time.Time, id string) string {
		n, ok := runAttention(m, Result{End: end, HaltEscalated: true, Halted: true, HaltID: id, ExitCode: 1})
		if !ok || n.Event != attention.KindNeedsYou || n.Source != attention.SourceSteer {
			t.Fatalf("%v %+v", ok, n)
		}
		return n.ID
	}
	if halt(m.Start.Add(time.Hour), "h1") != halt(m.Start.Add(2*time.Hour), "h1") || halt(m.Start.Add(time.Hour), "h1") == halt(m.Start.Add(time.Hour), "h2") {
		t.Fatal("halt subject")
	}
}

// A repeating run raises needs-you once per subject: the same open decision
// or halt every round is one post, a changed one posts again, and the
// repeat's end does not repost. Failed, blocked, timed-out and needs-review
// rounds are done-kind records: never posted by default, posted once per
// round with kinds = ["done"]. An uneventful round is silent.
func TestRepeatingRunNeedsYou(t *testing.T) {
	for _, c := range []struct {
		name    string
		r       Result
		changed Result
	}{
		{"needs-decision", Result{Outcome: OutcomeNeedsDecision, Key: "db", Question: "choose"}, Result{Outcome: OutcomeNeedsDecision, Key: "db", Question: "choose again"}},
		{"hard-halt", Result{Halted: true, HaltEscalated: true, HaltID: "h1", ExitCode: 1, Stopped: true}, Result{Halted: true, HaltEscalated: true, HaltID: "h2", ExitCode: 1, Stopped: true}},
	} {
		t.Run(c.name, func(t *testing.T) {
			orchHome(t)
			slack := &fakeSlack{}
			slackHost(t, slack, "")
			dir := t.TempDir()
			m := Meta{Order: Order{Name: "repeat-run", Host: "oven", Every: "1h"}, Start: time.Now().UTC()}
			round := func(n int, r Result) {
				t.Helper()
				if err := WriteJSON(filepath.Join(dir, "schedule.json"), Schedule{Rounds: n}); err != nil {
					t.Fatal(err)
				}
				r.End = m.Start.Add(time.Duration(n) * time.Hour)
				noticeRunEnd(dir, m, r)
			}
			round(3, c.r)
			if slack.count() != 1 || !strings.Contains(slack.posts[0]["text"].(string), "[needs you] repeat-run on oven") {
				t.Fatalf("round posts %+v", slack.posts)
			}
			// The same subject on later rounds is a no-op.
			round(4, c.r)
			round(5, c.r)
			if slack.count() != 1 {
				t.Fatalf("an unchanged subject posted again: posts=%d", slack.count())
			}
			end := c.r
			end.End = m.Start.Add(9 * time.Hour)
			noticeRepeatEnd(dir, m, end, "max_lifetime")
			if slack.count() != 1 {
				t.Fatalf("final transition reposted: posts=%d", slack.count())
			}
			// A changed subject raises again.
			round(6, c.changed)
			if slack.count() != 2 {
				t.Fatalf("a changed subject posts=%d, want 2", slack.count())
			}
			if n := len(attention.ReadStatus(dir)); n != 2 {
				t.Fatalf("records %d, want 2", n)
			}
		})
	}
	for _, c := range []struct {
		name string
		r    Result
	}{
		{"blocked", Result{Outcome: OutcomeBlocked, Reason: "access"}},
		{"failed", Result{ExitCode: 2}},
		{"timeout", Result{ExitCode: 1, Timeout: true, Stopped: true}},
		{"needs-review", Result{NeedsReview: true}},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, kinds := range []string{"", "kinds = [\"needs-you\", \"done\"]\n"} {
				orchHome(t)
				slack := &fakeSlack{}
				slackHost(t, slack, kinds)
				dir := t.TempDir()
				m := Meta{Order: Order{Name: "repeat-run", Host: "oven", Every: "1h"}, Start: time.Now().UTC()}
				for n := 3; n <= 4; n++ {
					if err := WriteJSON(filepath.Join(dir, "schedule.json"), Schedule{Rounds: n}); err != nil {
						t.Fatal(err)
					}
					r := c.r
					r.End = m.Start.Add(time.Duration(n) * time.Hour)
					noticeRunEnd(dir, m, r)
				}
				last := c.r
				last.End = m.Start.Add(9 * time.Hour)
				noticeRepeatEnd(dir, m, last, "max_lifetime")
				want := 0
				if kinds != "" {
					want = 2
				}
				if slack.count() != want {
					t.Fatalf("kinds %q: posts=%d, want %d", kinds, slack.count(), want)
				}
				for _, p := range slack.posts {
					if !strings.HasPrefix(p["text"].(string), "[done] repeat-run on oven: ") {
						t.Fatalf("text %q", p["text"])
					}
				}
				// Either way, each round is on the record.
				entries := attention.ReadStatus(dir)
				if len(entries) != 2 || entries[0].Event != attention.KindDone {
					t.Fatalf("records %+v", entries)
				}
			}
		})
	}
	t.Run("uneventful", func(t *testing.T) {
		orchHome(t)
		slack := &fakeSlack{}
		slackHost(t, slack, "kinds = [\"needs-you\", \"done\"]\n")
		dir := t.TempDir()
		m := Meta{Order: Order{Name: "repeat-ok", Host: "oven", Every: "1h"}, Start: time.Now().UTC()}
		r := Result{End: m.Start.Add(time.Minute)}
		noticeRunEnd(dir, m, r)
		if slack.count() != 0 {
			t.Fatalf("an uneventful round posted %d", slack.count())
		}
		// A requested stop is quiet; a repeat that ends is the opt-in done.
		stopped := r
		stopped.Stopped, stopped.ExitCode = true, 143
		noticeRepeatEnd(dir, m, stopped, "stopped")
		if slack.count() != 0 {
			t.Fatalf("a requested stop posted %d", slack.count())
		}
		noticeRepeatEnd(dir, m, r, "until")
		noticeRepeatEnd(dir, m, r, "until")
		if slack.count() != 1 || !strings.Contains(slack.posts[0]["text"].(string), "[done] repeat-ok on oven: repeat ended: until") {
			t.Fatalf("posts %+v", slack.posts)
		}
	})
}

// The repeating supervisor's real round loop: failing rounds are done-kind
// records (posted only with kinds = ["done"]), and a round loop asking the
// same decision every round posts it once.
func TestRepeatRoundsEmitAttention(t *testing.T) {
	for _, c := range []struct {
		name  string
		kinds string
		r     Result
		want  int
	}{
		{"failing-default-kinds", "", Result{ExitCode: 2, Error: "harness crashed"}, 0},
		{"failing-done-kind", "kinds = [\"needs-you\", \"done\"]\n", Result{ExitCode: 2, Error: "harness crashed"}, 2},
		{"same-decision", "", Result{Outcome: OutcomeNeedsDecision, Key: "db", Question: "which store?"}, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			orchHome(t)
			slack := &fakeSlack{}
			slackHost(t, slack, c.kinds)
			dir := filepath.Join(Root(), "repeat-loop")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			start := time.Date(2026, 11, 17, 1, 0, 0, 0, time.UTC)
			m := Meta{Order: Order{Name: "repeat-loop", Host: "oven", Every: "1h", MaxLifetime: "150m"}, Start: start}
			if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
				t.Fatal(err)
			}
			clock := start
			round := func(string) error {
				// Mirrors superviseRound's tail for a repeating run.
				r := c.r
				r.End = clock
				if err := WriteJSON(filepath.Join(dir, "result.json"), r); err != nil {
					return err
				}
				noticeRunEnd(dir, m, r)
				clock = clock.Add(time.Hour)
				return nil
			}
			err := repeatRoundsNotify(dir, m, nil, func() time.Time { return clock }, func() { clock = clock.Add(time.Hour) }, round, func(Meta, Result) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			// Two rounds ran (0h, 2h); the repeat ended at max_lifetime after the second.
			if slack.count() != c.want {
				t.Fatalf("posts=%d, want %d", slack.count(), c.want)
			}
		})
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
		if err := os.WriteFile(filepath.Join(dir, "reply.md"), []byte("[needs you] needs-decision db: pick the database for kit-x\nDetails follow."), 0600); err != nil {
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
	if text := first["text"].(string); !strings.HasPrefix(text, "[needs you] "+s.Name+" on test-host: needs-decision db: pick the database for kit-x\nRun: hev orchestrate show "+s.Name) {
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

// A stall and a [needs you] reply that is not a decision are done-kind
// records; only "[needs you] needs-decision KEY: QUESTION" raises needs-you.
func TestOrchestratorNeedsYouOnlyForDecisions(t *testing.T) {
	orchHome(t)
	var got []attention.Notice
	old := queueAttention
	queueAttention = func(n attention.Notice, _ string) { got = append(got, n) }
	t.Cleanup(func() { queueAttention = old })
	// follow-* and the expedite-* names older builds wrote are both the
	// follow-through source.
	for _, name := range []string{"follow-owner", "expedite-owner"} {
		s := OrchestratorSpec{Name: name, Host: "oven", Reply: "owner"}
		orchestratorNeedsYouCases(t, s, &got)
	}
	if text, ok := replyNeedsYou("  [needs you] decide\nmore"); !ok || text != "decide" {
		t.Fatal(text, ok)
	}
	if _, ok := replyNeedsYou("Ready to merge\n[needs you] later line"); ok {
		t.Fatal("only the first line counts")
	}
	if src := orchestratorSource("release-train"); src != attention.SourceOrchestrator {
		t.Fatal(src)
	}
}

func orchestratorNeedsYouCases(t *testing.T, s OrchestratorSpec, got *[]attention.Notice) {
	t.Helper()
	st := OrchestratorState{Start: time.Date(2026, 11, 17, 1, 0, 0, 0, time.UTC), Rounds: 1, State: "waiting"}
	for _, c := range []struct{ text, kind string }{
		{"needs-decision merge: squash or rebase?", attention.KindNeedsYou},
		{"`needs-decision merge: squash or rebase?`", attention.KindNeedsYou},
		{"kit-x stalled: CI red for 3 rounds", attention.KindDone},
		{"kit-x failed (exit 1)", attention.KindDone},
		{"needs-decision: no key", attention.KindDone},
		{"needs-decision merge:", attention.KindDone},
	} {
		*got = nil
		noticeOrchestrator(s, orchestratorReplyAttention(s, st, c.text))
		if len(*got) != 1 || (*got)[0].Event != c.kind || (*got)[0].Source != attention.SourceFollow {
			t.Fatalf("%s %q: %+v", s.Name, c.text, *got)
		}
	}
}

// The real stall path raises a done-kind record, not needs-you.
func TestFollowStallIsRecordNotNeedsYou(t *testing.T) {
	orchHome(t)
	var got []attention.Notice
	old := queueAttention
	queueAttention = func(n attention.Notice, _ string) { got = append(got, n) }
	t.Cleanup(func() { queueAttention = old })
	s, e := ApplyOrchestrator(strings.Replace(orchestratorExample, "release-train", "follow-stall", 1), false)
	if e != nil {
		t.Fatal(e)
	}
	st, _ := orchestratorState(s.Name)
	d := OrchestratorDriver{
		Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) {
			return []OrchestratorEvidence{{Host: "worker", Row: Row{Head: "h", State: "done"}}}, nil
		},
		Session: func(_ context.Context, _ OrchestratorSpec, dir string, _ OrchestratorPacket) error {
			return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("[needs you] resolve CI failure\n"), 0600)
		},
		Notify: func(context.Context, OrchestratorNotice) error { return nil },
	}
	for i := 0; i < 5; i++ {
		if e = RunOrchestratorRound(context.Background(), s, &st, d); e != nil {
			t.Fatal(e)
		}
	}
	if st.State != "stalled" || len(got) != 1 || got[0].Event != attention.KindDone || got[0].State != "stalled" {
		t.Fatalf("%s %+v", st.State, got)
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
	// An admission failure is a record on the opt-in done kind.
	if len(got) != 1 || got[0].Event != attention.KindDone || got[0].State != "failed" || got[0].Text != "failed (exit 1): git: no origin" {
		t.Fatalf("%+v", got)
	}
	_ = WriteJSON(filepath.Join(dir, "result.json"), Result{ExitCode: 1, End: time.Now().UTC()})
	noticeAdmissionFailure(dir)
	if len(got) != 1 {
		t.Fatal("a run that did not fail admission is left to its supervisor")
	}
}

// A client island ([notify] client = true, no Slack) builds both receipt
// kinds with only the name and state, and the client marker survives the
// pull, so a pulling host with Slack posts nothing private.
func TestClientPrivacyAcrossPull(t *testing.T) {
	for _, source := range []string{"orchestrator", "member"} {
		t.Run(source, func(t *testing.T) {
			orchHome(t)
			cfg := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(cfg, []byte("[notify]\nclient = true\n"), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HEV_CONFIG", cfg)
			t.Setenv(attention.RootEnv, t.TempDir())
			var carried *attention.Notice
			if source == "orchestrator" {
				s := applyOrch(t)
				s.Authority = append(s.Authority, "notify")
				st, _ := orchestratorState(s.Name)
				d := noticeDriver(t, func(context.Context, OrchestratorNotice) error { return nil })
				d.Session = func(_ context.Context, _ OrchestratorSpec, dir string, _ OrchestratorPacket) error {
					if err := os.WriteFile(filepath.Join(dir, "reply.md"), []byte("[needs you] needs-decision terms: CLIENT_PRIVATE_BODY choose terms\n"), 0600); err != nil {
						return err
					}
					return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("waiting"), 0600)
				}
				if err := RunOrchestratorRound(context.Background(), s, &st, d); err != nil {
					t.Fatal(err)
				}
				for _, n := range noticesOf(t, s.Name, "reply") {
					carried = n.Attention
				}
			} else {
				m := Meta{Sender: "teammate", Order: Order{Name: "client-run", Host: "win-oven", Reply: "lap"}, Start: time.Now().UTC()}
				r := Result{End: time.Now().UTC(), Outcome: OutcomeBlocked, Reason: "CLIENT_PRIVATE_BODY choose terms", PR: "https://example.invalid/private-client/pull/1"}
				if _, err := queueMemberNotice(nil, m, r); err != nil {
					t.Fatal(err)
				}
				pending, err := PendingMemberNotices("teammate")
				if err != nil || len(pending) != 1 {
					t.Fatal(pending, err)
				}
				// The persisted receipt itself carries nothing private.
				if a := pending[0].Attention; a == nil || !a.Client || a.Text != "" || a.Links.PR != "" {
					t.Fatalf("persisted receipt %+v", a)
				}
				local, _, err := LocalMemberNotice("win-oven", pending[0])
				if err != nil {
					t.Fatal(err)
				}
				carried = local.Attention
			}
			if carried == nil || !carried.Client {
				t.Fatalf("receipt lost the client marker: %+v", carried)
			}
			b, _ := json.Marshal(carried)
			if strings.Contains(string(b), "CLIENT_PRIVATE_BODY") || strings.Contains(string(b), "private-client") {
				t.Fatalf("receipt carries client content: %s", b)
			}
			laptop := &fakeSlack{}
			// The member receipt is a blocked run: a done-kind record.
			slackHost(t, laptop, "kinds = [\"needs-you\", \"done\"]\n")
			if err := PulledAttention(context.Background(), "win-oven", carried); err != nil {
				t.Fatal(err)
			}
			if laptop.count() != 1 {
				t.Fatalf("posts=%d", laptop.count())
			}
			text := laptop.posts[0]["text"].(string)
			if strings.Contains(text, "CLIENT_PRIVATE_BODY") || strings.Contains(text, "private-client") {
				t.Fatalf("client privacy lost across pull: %q", text)
			}
		})
	}
}

// A source host whose config cannot be read fails closed for receipts.
func TestOutboundFailsClosed(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte("[notify\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_CONFIG", cfg)
	n := attention.Outbound(attention.New(attention.KindNeedsYou, attention.SourceRun, "h", "r", "k", "blocked", "secret plan", attention.Links{PR: "https://x/pull/1"}))
	if !n.Client || n.Text != "" || n.Links.PR != "" {
		t.Fatalf("%+v", n)
	}
}
