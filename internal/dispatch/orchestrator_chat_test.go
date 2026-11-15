package dispatch

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	chat "github.com/hev/kit/internal/notify"
	"github.com/hev/kit/internal/notify/slackfake"
)

// Tests in this file run against slackfake only. Tokens are obviously fake
// and built by concatenation so no secret-shaped literal sits in the source.
var (
	chatToken    = "xo" + "xb-000000000000-000000000000-" + "FAKEFAKEFAKEFAKEFAKEFAKE"
	chatNewToken = "xo" + "xb-000000000000-000000000000-" + "ROTATEDROTATEDROTATEDROT"
)

const chatBuilds = "C0123456789"

const chatConfig = `[notifiers.team]
kind = "slack"
token = "op://example-vault/kit-slack-bot/credential"
channels = { builds = "C0123456789" }

[notifiers.client-a]
kind = "slack"
token = "op://client-a-vault/slack-bot/credential"
audience = "external"
repos = ["app"]
channels = { delivery = "C0AAAAAAAAA" }
`

// chatSpec is the RFC 0017 end-to-end example spec.
const chatSpec = `---
kind: orchestrator
name: release-train
goal: Every PR labelled release-0.9 is merged with green CI.
host: build-1
mode: loop
every: 20m
until: goal
max_lifetime: 7d
harness: claude
reply: my-session
notify:
  notifier: team
  channel: builds
manages:
  runs: {selector: "issue=REL-*"}
authority: [say, notify, rework, retry]
---
Keep the release moving.
`

// fakeVault stands in for hev secrets: one value, rotatable, and a count
// of fresh reads. It never touches a real provider.
type fakeVault struct {
	mu     sync.Mutex
	value  string
	fresh  int
	reads  int
	failer func() error
}

func (v *fakeVault) resolve(_ context.Context, n chat.Notifier, fresh bool) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.reads++
	if fresh {
		v.fresh++
	}
	if v.failer != nil {
		if e := v.failer(); e != nil {
			return "", e
		}
	}
	if !strings.HasPrefix(n.Token, "op://") {
		panic("resolver got a non-reference")
	}
	return v.value, nil
}

type chatClock struct {
	mu  sync.Mutex
	now time.Time
}

// tick returns a strictly increasing time, so receipts order deterministically.
func (c *chatClock) tick() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(time.Millisecond)
	return c.now
}
func (c *chatClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

type chatEnv struct {
	fake  *slackfake.Server
	vault *fakeVault
	clock *chatClock
	dir   string
}

// chatHome isolates HOME, writes notify.toml (unless config is empty) beside
// a test dispatch.toml, and points every seam at fakes.
func chatHome(t *testing.T, config string) *chatEnv {
	t.Helper()
	orchHome(t)
	dir := t.TempDir()
	t.Setenv("HEV_DISPATCH_CONFIG", filepath.Join(dir, "dispatch.toml"))
	t.Setenv("HEV_CONFIG", filepath.Join(dir, "config.toml"))
	if config != "" {
		if err := os.WriteFile(filepath.Join(dir, "notify.toml"), []byte(config), 0600); err != nil {
			t.Fatal(err)
		}
	}
	env := &chatEnv{fake: slackfake.New(chatToken, chatBuilds, "C0AAAAAAAAA"), vault: &fakeVault{value: chatToken}, clock: &chatClock{now: time.Now().UTC()}, dir: dir}
	t.Cleanup(env.fake.Close)
	oldBase, oldResolve, oldApplier := notifySlackBase, notifyResolveSecret, notifyApplier
	t.Cleanup(func() { notifySlackBase, notifyResolveSecret, notifyApplier = oldBase, oldResolve, oldApplier })
	notifySlackBase, notifyResolveSecret = env.fake.BaseURL(), env.vault.resolve
	notifyApplier = func() string { return "local:owner" }
	noticeNow = env.clock.tick
	freshReadsMu.Lock()
	freshReads = map[string]time.Time{}
	freshReadsMu.Unlock()
	return env
}

func chatReceipts(t *testing.T, name string) []OrchestratorNotice {
	return noticesOf(t, name, "notify")
}

func (env *chatEnv) texts() []string {
	var out []string
	for _, m := range env.fake.Messages() {
		out = append(out, m.Text)
	}
	return out
}

// assertNoLeak checks every request body and every receipt for a string.
func assertNoLeak(t *testing.T, env *chatEnv, name string, needles ...string) {
	t.Helper()
	all, _ := loadOrchestratorNotices(name)
	for _, r := range env.fake.Requests() {
		for _, n := range needles {
			if strings.Contains(r.Body, n) {
				t.Fatalf("%q reached the chat server in %s", n, r.Method)
			}
		}
	}
	for _, f := range all {
		if !isChatRecipient(f.n.To) {
			continue
		}
		b, _ := json.Marshal(f.n)
		for _, n := range needles {
			if strings.Contains(string(b), n) {
				t.Fatalf("%q stored on chat receipt %s", n, f.n.ID)
			}
		}
	}
}

// e2eRun is the managed run rel-42, written as a real run directory so the
// decision is answered through the same ledger code as dispatch say --resolve.
type e2eRun struct {
	t     *testing.T
	dir   string
	start time.Time
	phase string
}

func newE2ERun(t *testing.T) *e2eRun {
	r := &e2eRun{t: t, dir: filepath.Join(Root(), "rel-42"), start: time.Now().UTC().Truncate(time.Second), phase: "running"}
	if err := os.MkdirAll(r.dir, 0700); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *e2eRun) setPhase(phase string) {
	r.t.Helper()
	r.phase = phase
	res := Result{Start: r.start, End: time.Now().UTC(), ExitCode: 0}
	switch phase {
	case "needs-decision":
		if err := os.WriteFile(filepath.Join(r.dir, "status.md"), []byte("working\nneeds-decision db: Postgres or SQLite for the cache?\n"), 0600); err != nil {
			r.t.Fatal(err)
		}
		recordOutcome(r.dir, &res)
	case "answered":
		// hev dispatch say rel-42 --resolve db 'SQLite': the ledger write.
		_, commit, err := resolveDecision(r.dir, "db", "SQLite", "local:owner")
		if err != nil {
			r.t.Fatal(err)
		}
		if err = commit(); err != nil {
			r.t.Fatal(err)
		}
		r.start = r.start.Add(time.Minute) // The rework is a new attempt.
		_ = os.Remove(filepath.Join(r.dir, "result.json"))
		return
	case "done":
		_ = os.WriteFile(filepath.Join(r.dir, "status.md"), []byte("done\n"), 0600)
		recordOutcome(r.dir, &res)
		res.PR, res.Validation = "https://github.com/example/app/pull/42", "pass"
	}
	if err := WriteJSON(filepath.Join(r.dir, "result.json"), res); err != nil {
		r.t.Fatal(err)
	}
}

// gather builds the evidence the inventory would: the row and the ledger.
func (r *e2eRun) gather(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) {
	m := Meta{Order: Order{Name: "rel-42", Issue: "REL-42", Repo: "app"}, Start: r.start}
	row := Row{Kind: "run", Meta: m, State: "running"}
	files := map[string]string{}
	var res Result
	if b, err := os.ReadFile(filepath.Join(r.dir, "result.json")); err == nil {
		files["result.json"] = string(b)
		_ = json.Unmarshal(b, &res)
		row.State, row.PR, row.Validation = State(&res, false), res.PR, res.Validation
		row.Outcome, row.Key, row.Question, row.Reason = res.Outcome, res.Key, res.Question, res.Reason
	}
	return []OrchestratorEvidence{{Host: "local", Row: row, Files: files, Decisions: loadDecisions(r.dir)}}, nil
}

// runE2E drives the example through the real supervisor: start, a
// needs-decision, an answer from the shell, a finished rework, goal met.
// between runs after each round and may change the world (revoke a token).
func runE2E(t *testing.T, env *chatEnv, spec string, between func(round int)) (OrchestratorState, *noticeLog) {
	t.Helper()
	s, err := ApplyOrchestrator(spec, false)
	if err != nil {
		t.Fatal(err)
	}
	run := newE2ERun(t)
	peers := &noticeLog{}
	waits := 0
	d := OrchestratorDriver{
		Gather: run.gather,
		Session: func(_ context.Context, _ OrchestratorSpec, dir string, _ OrchestratorPacket) error {
			// Agent prose with a secret: reply.md may reach peers, never chat.
			if err := os.WriteFile(filepath.Join(dir, "reply.md"), []byte("REPLY-SENTINEL owner note "+chatToken), 0600); err != nil {
				return err
			}
			if run.phase == "done" {
				if err := os.WriteFile(filepath.Join(dir, "goal-met.json"), []byte(`{"met":true}`), 0600); err != nil {
					return err
				}
			}
			return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("phase "+run.phase), 0600)
		},
		Now: func() time.Time { env.clock.mu.Lock(); defer env.clock.mu.Unlock(); return env.clock.now },
		Wait: func(context.Context) error {
			waits++
			if between != nil {
				between(waits)
			}
			switch waits {
			case 1:
				run.setPhase("needs-decision")
			case 2:
				run.setPhase("answered")
			case 3:
				run.setPhase("done")
			default:
				t.Fatalf("unexpected wait %d", waits)
			}
			env.clock.advance(21 * time.Minute)
			return nil
		},
		Notify: peers.send,
	}
	if err := SuperviseOrchestrator(context.Background(), s.Name, d); err != nil {
		t.Fatal(err)
	}
	st, err := orchestratorState(s.Name)
	if err != nil {
		t.Fatal(err)
	}
	return st, peers
}

var e2eThread = []string{
	"Orchestrator release-train started on build-1. Goal: Every PR labelled release-0.9 is merged with green CI. loop every 20m until goal. Details: hev orchestrate show release-train",
	"rel-42 needs a decision (db): Postgres or SQLite for the cache? Answer from a shell: hev dispatch say rel-42 --resolve db 'ANSWER'",
	"rel-42: decision db answered by local:owner; rework started.",
	"rel-42 done. PR: https://github.com/example/app/pull/42",
	"rel-42 validation pass. PR: https://github.com/example/app/pull/42",
	"release-train met its goal after 4 rounds.",
}

func TestNotifyEndToEndExample(t *testing.T) {
	env := chatHome(t, chatConfig)
	st, peers := runE2E(t, env, chatSpec, nil)
	if st.State != "complete" || st.Rounds != 4 {
		t.Fatalf("state %+v", st)
	}
	msgs := env.fake.Messages()
	if got := env.texts(); !reflect.DeepEqual(got, e2eThread) {
		t.Fatalf("thread:\n%s", strings.Join(got, "\n"))
	}
	root := msgs[0]
	if root.ThreadTS != "" || root.Channel != chatBuilds {
		t.Fatalf("root %+v", root)
	}
	for _, m := range msgs[1:] {
		if m.ThreadTS != root.TS {
			t.Fatalf("not in the thread: %+v", m)
		}
	}
	receipts := chatReceipts(t, "release-train")
	for _, r := range receipts {
		if r.Status != "delivered" || r.Notifier != "team" || r.Channel != "builds" || r.NotifierKind != "slack" || r.RequestedBy != "local:owner" || r.Event == "" || r.Thread != root.TS || !strings.HasPrefix(r.Ref, chatBuilds+"/") {
			t.Fatalf("receipt audit fields: %+v", r)
		}
		if !strings.HasPrefix(r.ID, "build-1/o-release-train:notify:") || r.To != "notify:team/builds" {
			t.Fatalf("receipt route: %+v", r)
		}
	}
	if receipts[0].Ref != chatBuilds+"/"+root.TS {
		t.Fatalf("root ref %s", receipts[0].Ref)
	}
	threads, err := readThreads("release-train")
	if err != nil || len(threads) != 1 {
		t.Fatalf("threads.json %v %v", threads, err)
	}
	// reply.md went to peers (notify authority), never to chat.
	if peers.count("REPLY-SENTINEL") == 0 {
		t.Fatal("reply.md should still reach the peers owner")
	}
	assertNoLeak(t, env, "release-train", "REPLY-SENTINEL", chatToken)
	// One audit line per final receipt, without message text.
	audit, err := os.ReadFile(filepath.Join(Root(), ".notify", "audit.jsonl"))
	if err != nil || strings.Count(string(audit), "\n") != len(receipts) || strings.Contains(string(audit), "Postgres") {
		t.Fatalf("audit %q %v", audit, err)
	}
}

// stateSnapshot is everything a post must never change.
func stateSnapshot(t *testing.T, st OrchestratorState) map[string]any {
	t.Helper()
	dir := OrchestratorDir("release-train")
	checkpoint, _ := os.ReadFile(filepath.Join(dir, "checkpoint.md"))
	run := filepath.Join(Root(), "rel-42")
	var res Result
	_ = readJSON(filepath.Join(run, "result.json"), &res)
	var decisions []Decision
	_ = readJSON(filepath.Join(run, "decisions.json"), &decisions)
	for i := range decisions {
		decisions[i].Asked, decisions[i].Resolved = time.Time{}, time.Time{}
	}
	return map[string]any{
		"state": st.State, "rounds": st.Rounds, "lineage": st.Lineage, "error": st.Error, "failures": st.ConsecutiveFailures, "managed": st.ManagedRuns,
		"checkpoint": string(checkpoint), "run-state": State(&res, false), "outcome": res.Outcome, "pr": res.PR, "validation": res.Validation, "decisions": decisions,
	}
}

func TestNotifyEndToEndRevokedToken(t *testing.T) {
	// Baseline: the same run with no notify mapping.
	chatHome(t, chatConfig)
	plain := strings.Replace(chatSpec, "notify:\n  notifier: team\n  channel: builds\n", "", 1)
	baseState, _ := runE2E(t, &chatEnv{clock: &chatClock{now: time.Now().UTC()}}, plain, nil)
	want := stateSnapshot(t, baseState)

	env := chatHome(t, chatConfig)
	st, peers := runE2E(t, env, chatSpec, func(round int) {
		if round == 1 {
			// Revoked before step 2 (the needs-decision).
			env.fake.Revoke(chatToken)
		}
	})
	if got := stateSnapshot(t, st); !reflect.DeepEqual(got, want) {
		t.Fatalf("run state changed by a failed post:\n got %#v\nwant %#v", got, want)
	}
	if got := env.texts(); len(got) != 1 || got[0] != e2eThread[0] {
		t.Fatalf("only the root should be delivered: %q", got)
	}
	receipts := chatReceipts(t, "release-train")
	if len(receipts) != len(e2eThread) {
		t.Fatalf("receipts %d", len(receipts))
	}
	nd := receipts[1]
	if nd.Event != "needs-decision" || nd.Status != "queued" || nd.Attempts < 1 || nd.Error != "slack: token_revoked" || nd.ErrorClass != "credential" {
		t.Fatalf("needs-decision receipt %+v", nd)
	}
	for _, r := range receipts[1:] {
		if r.Status != "queued" {
			t.Fatalf("should stay queued: %+v", r)
		}
	}
	if env.vault.fresh == 0 {
		t.Fatal("a credential failure should ask for a fresh secret read")
	}
	// The peers notice to my-session is a separate receipt and unaffected.
	if peers.count("Orchestrator release-train on build-1: goal met") != 1 {
		t.Fatalf("peers lifecycle notices: %+v", peers.sent)
	}
	last, _ := orchestratorState("release-train")
	if last.LastNotice == nil {
		t.Fatal("last_notice missing")
	}
	// Nobody rotates the token: at 24 hours after creation each becomes failed.
	env.clock.mu.Lock()
	env.clock.now = nd.Created.Add(24*time.Hour - time.Minute)
	env.clock.mu.Unlock()
	flushOrchestratorNoticesByName("release-train", peers.send)
	if r := chatReceipts(t, "release-train")[1]; r.Status != "queued" {
		t.Fatalf("expired early: %+v", r)
	}
	env.clock.mu.Lock()
	env.clock.now = receipts[len(receipts)-1].Created.Add(24 * time.Hour)
	env.clock.mu.Unlock()
	flushOrchestratorNoticesByName("release-train", peers.send)
	for _, r := range chatReceipts(t, "release-train")[1:] {
		if r.Status != "failed" || r.Error != "expired after 24h: slack: token_revoked" {
			t.Fatalf("not expired: %+v", r)
		}
	}
	if got := stateSnapshot(t, st); !reflect.DeepEqual(got, want) {
		t.Fatal("expiry changed run state")
	}
}

func TestNotifyRevokedTokenRotatedDeliversInOrder(t *testing.T) {
	env := chatHome(t, chatConfig)
	_, peers := runE2E(t, env, chatSpec, func(round int) {
		if round == 1 {
			env.fake.Revoke(chatToken)
		}
	})
	// The owner rotates the token in the vault; the next attempt delivers
	// the queued receipts in order, in the same thread.
	env.fake.AddToken(chatNewToken)
	env.vault.mu.Lock()
	env.vault.value = chatNewToken
	env.vault.mu.Unlock()
	env.clock.advance(2 * time.Hour)
	flushOrchestratorNoticesByName("release-train", peers.send)
	msgs := env.fake.Messages()
	if got := env.texts(); !reflect.DeepEqual(got, e2eThread) {
		t.Fatalf("thread after rotation:\n%s", strings.Join(got, "\n"))
	}
	for _, m := range msgs[1:] {
		if m.ThreadTS != msgs[0].TS {
			t.Fatalf("not in thread: %+v", m)
		}
	}
	for _, r := range env.fake.Requests() {
		if strings.Contains(r.Body, "xo"+"xb") {
			t.Fatal("token in a request body")
		}
	}
}
