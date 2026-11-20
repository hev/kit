package dispatch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	chat "github.com/hev/kit/internal/notify"
	"github.com/hev/kit/internal/notify/slackfake"
)

func TestNotifyApplyChecks(t *testing.T) {
	cases := []struct {
		name, config, from, to, want string
	}{
		{"unknown notifier", chatConfig, "notifier: team", "notifier: nobody", "notifier nobody not defined on this node"},
		{"unlisted channel", chatConfig, "channel: builds", "channel: ops", "channel ops not allowed for notifier team"},
		{"out-of-scope repo", chatConfig, "notifier: team\n  channel: builds", "notifier: client-a\n  channel: delivery\nmanages:\n  repos: [other]\n  runs: {selector: \"issue=REL-*\"}\n@@CUT@@", "notifier client-a does not allow repo other"},
		{"no notify.toml", "", "", "", "no notify.toml on this node"},
		{"no default", chatConfig, "  notifier: team\n", "", "no notifier named and no default"},
		{"unknown mapping field", chatConfig, "channel: builds", "channel: builds\n  webhook: x", "field webhook not found"},
		{"reply is a chat route", chatConfig, "reply: my-session", "reply: notify:team/builds", "reply must be a peers address"},
		{"bad event", chatConfig, "channel: builds", "channel: builds\n  events: [everything]", "unknown notify event"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := chatHome(t, tc.config)
			spec := strings.Replace(chatSpec, tc.from, tc.to, 1)
			// The out-of-scope case replaces the manages block wholesale.
			spec = strings.Replace(spec, "@@CUT@@\nmanages:\n  runs: {selector: \"issue=REL-*\"}\n", "", 1)
			for _, dry := range []bool{true, false} {
				_, err := ApplyOrchestrator(spec, dry)
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("dry=%v: %v", dry, err)
				}
			}
			if _, err := os.Stat(filepath.Join(OrchestratorConfigDir(), "release-train.md")); !os.IsNotExist(err) {
				t.Fatal("a refused spec must not be applied")
			}
			if len(env.fake.Requests()) != 0 {
				t.Fatal("apply must not call the chat app")
			}
		})
	}
}

func TestNotifyApplyAcceptsDefaultAndScopedRepos(t *testing.T) {
	chatHome(t, "default = \"team\"\n"+chatConfig)
	spec := strings.Replace(chatSpec, "  notifier: team\n", "", 1)
	if _, err := ApplyOrchestrator(spec, false); err != nil {
		t.Fatal(err)
	}
	r := chatReceipts(t, "release-train")
	if len(r) != 1 || r[0].To != "notify:team/builds" || r[0].Event != "started" {
		t.Fatalf("%+v", r)
	}
	scoped := strings.Replace(chatSpec, "notifier: team\n  channel: builds", "notifier: client-a\n  channel: delivery", 1)
	scoped = strings.Replace(scoped, "manages:\n", "manages:\n  repos: [app]\n", 1)
	if _, err := ApplyOrchestrator(scoped, true); err != nil {
		t.Fatal(err)
	}
}

// chatOne applies the example, delivers the root and queues one more event.
func chatOne(t *testing.T, env *chatEnv) OrchestratorSpec {
	t.Helper()
	s, err := ApplyOrchestrator(chatSpec, false)
	if err != nil {
		t.Fatal(err)
	}
	flushOrchestratorNotices(s, func(context.Context, OrchestratorNotice) error { return nil })
	if len(env.fake.Messages()) != 1 {
		t.Fatalf("root not posted: %v", env.fake.Requests())
	}
	st, _ := orchestratorState(s.Name)
	chatLifecycle(s, st, "paused:x", chat.Event{Kind: chat.EventBlocked, Run: "rel-42", Reason: "waiting on CI"})
	return s
}

func lastReceipt(t *testing.T) OrchestratorNotice {
	t.Helper()
	r := chatReceipts(t, "release-train")
	return r[len(r)-1]
}

func TestNotifyFailureMapping(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(*chatEnv)
		status string
		class  string
		errSub string
		check  func(*testing.T, *chatEnv, OrchestratorNotice)
	}{
		{name: "server error", setup: func(e *chatEnv) { e.fake.ServerErrors(1) }, status: "queued", class: "transient", errSub: "slack: server error (HTTP 503)",
			check: func(t *testing.T, e *chatEnv, n OrchestratorNotice) {
				if n.Attempts != 1 || n.NextAttempt.Sub(e.clock.now) > time.Minute+time.Second {
					t.Fatalf("normal backoff: %+v", n)
				}
			}},
		{name: "network error", setup: func(e *chatEnv) { notifySlackBase = "http://127.0.0.1:1/api/" }, status: "queued", class: "transient", errSub: "slack: network error"},
		{name: "rate limit honours retry hint", setup: func(e *chatEnv) { e.fake.RateLimit(600) }, status: "queued", class: "rate-limited", errSub: "slack: ratelimited (HTTP 429)",
			check: func(t *testing.T, e *chatEnv, n OrchestratorNotice) {
				if d := n.NextAttempt.Sub(e.clock.now); d < 9*time.Minute || d > 10*time.Minute+time.Second {
					t.Fatalf("next attempt should follow Retry-After (600s), got %v", d)
				}
			}},
		{name: "rate limit shorter than backoff", setup: func(e *chatEnv) { e.fake.RateLimit(1) }, status: "queued", class: "rate-limited", errSub: "ratelimited",
			check: func(t *testing.T, e *chatEnv, n OrchestratorNotice) {
				if d := n.NextAttempt.Sub(e.clock.now); d < 59*time.Second {
					t.Fatalf("the later of backoff and retry: %v", d)
				}
			}},
		{name: "channel gone", setup: func(e *chatEnv) { e.fake.RemoveChannel(chatBuilds) }, status: "failed", class: "channel", errSub: "slack: channel_not_found"},
		{name: "app not in channel", setup: func(e *chatEnv) { e.fake.SetChannel(chatBuilds, slackfake.Channel{}) }, status: "failed", class: "channel", errSub: "slack: not_in_channel"},
		{name: "archived", setup: func(e *chatEnv) { e.fake.SetChannel(chatBuilds, slackfake.Channel{Member: true, Archived: true}) }, status: "failed", class: "channel", errSub: "slack: is_archived"},
		{name: "payload refused", setup: func(e *chatEnv) { e.fake.FailWith("no_text") }, status: "failed", class: "payload", errSub: "slack: no_text"},
		{name: "credential refused", setup: func(e *chatEnv) { e.fake.Revoke(chatToken) }, status: "queued", class: "credential", errSub: "slack: token_revoked"},
		{name: "missing scope", setup: func(e *chatEnv) { e.fake.FailWith("missing_scope") }, status: "queued", class: "credential", errSub: "slack: missing_scope"},
		{name: "secrets cooldown", setup: func(e *chatEnv) {
			e.vault.failer = func() error {
				return &chat.Error{Class: chat.ClassRateLimited, Code: "secrets cooldown for notifier team", RetryAt: e.clock.now.Add(30 * time.Minute), Local: true}
			}
		}, status: "queued", class: "rate-limited", errSub: "secrets cooldown for notifier team",
			check: func(t *testing.T, e *chatEnv, n OrchestratorNotice) {
				if d := n.NextAttempt.Sub(e.clock.now); d < 29*time.Minute {
					t.Fatalf("cooldown not honoured: %v", d)
				}
			}},
		{name: "no secrets provider", setup: func(e *chatEnv) {
			e.vault.failer = func() error { return chat.LocalError(chat.ClassConfig, "no secrets provider for notifier team") }
		}, status: "failed", class: "config", errSub: "no secrets provider for notifier team"},
		{name: "notifier removed after apply", setup: func(e *chatEnv) {
			_ = os.WriteFile(filepath.Join(e.dir, "notify.toml"), []byte(strings.Replace(chatConfig, "[notifiers.team]", "[notifiers.other]", 1)), 0600)
		}, status: "failed", class: "config", errSub: "notifier team not defined on this node"},
		{name: "label removed after apply", setup: func(e *chatEnv) {
			_ = os.WriteFile(filepath.Join(e.dir, "notify.toml"), []byte(strings.Replace(chatConfig, "builds = ", "ops = ", 1)), 0600)
		}, status: "failed", class: "config", errSub: "channel builds not allowed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := chatHome(t, chatConfig)
			s := chatOne(t, env)
			before := len(env.fake.Messages())
			tc.setup(env)
			requests := len(env.fake.Requests())
			flushOrchestratorNotices(s, func(context.Context, OrchestratorNotice) error { return nil })
			n := lastReceipt(t)
			if n.Status != tc.status || n.ErrorClass != tc.class || !strings.Contains(n.Error, tc.errSub) {
				t.Fatalf("receipt %+v", n)
			}
			if strings.Contains(n.Error, "xo"+"xb") || strings.Contains(n.Error, "127.0.0.1") || strings.Contains(n.Error, "op://") {
				t.Fatalf("error leaks: %s", n.Error)
			}
			if len(env.fake.Messages()) != before {
				t.Fatal("nothing should have been posted")
			}
			if tc.class == "config" && tc.name != "no secrets provider" && len(env.fake.Requests()) != requests {
				t.Fatal("a not-configured receipt must make no request")
			}
			if tc.check != nil {
				tc.check(t, env, n)
			}
			st, _ := orchestratorState(s.Name)
			if st.State != "ready" || st.Error != "" {
				t.Fatalf("a failed post changed orchestrator state: %+v", st)
			}
		})
	}
}

func TestNotifyRateLimitRetriesAfterHint(t *testing.T) {
	env := chatHome(t, chatConfig)
	s := chatOne(t, env)
	env.fake.RateLimit(120)
	flushOrchestratorNotices(s, noPeers)
	if lastReceipt(t).Status != "queued" {
		t.Fatal("rate limited receipt should stay queued")
	}
	env.clock.advance(90 * time.Second)
	flushOrchestratorNotices(s, noPeers)
	if got := len(env.fake.Requests()); got != 2 { // root, 429, nothing more
		t.Fatalf("retried before the server's retry time: %d requests", got)
	}
	env.clock.advance(31 * time.Second)
	flushOrchestratorNotices(s, noPeers)
	if n := lastReceipt(t); n.Status != "delivered" {
		t.Fatalf("%+v", n)
	}
}

func TestNotifyCredentialFreshReadOncePerHour(t *testing.T) {
	env := chatHome(t, chatConfig)
	s := chatOne(t, env)
	env.fake.Revoke(chatToken)
	for i := 0; i < 4; i++ {
		flushOrchestratorNotices(s, noPeers)
		env.clock.advance(10 * time.Minute)
	}
	if env.vault.fresh != 1 {
		t.Fatalf("fresh reads %d, want 1 per hour", env.vault.fresh)
	}
	env.clock.advance(time.Hour)
	flushOrchestratorNotices(s, noPeers)
	if env.vault.fresh != 2 {
		t.Fatalf("fresh reads %d", env.vault.fresh)
	}
}

func TestNotifyThreadKeptAcrossRotationAndHandoffNewOnReapply(t *testing.T) {
	env := chatHome(t, chatConfig)
	spec := strings.Replace(chatSpec, "authority:", "handoff: {rotate_after: 1}\nauthority:", 1)
	spec = strings.Replace(spec, "  channel: builds\n", "  channel: builds\n  events: [all]\n", 1)
	s, err := ApplyOrchestrator(spec, false)
	if err != nil {
		t.Fatal(err)
	}
	rounds := 0
	d := OrchestratorDriver{
		Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) {
			rounds++
			return []OrchestratorEvidence{{Row: Row{Kind: "run", Meta: Meta{Order: Order{Name: "rel-42", Issue: "REL-42"}, Start: time.Unix(int64(rounds), 0)}, State: "running"}}}, nil
		},
		Session: func(_ context.Context, _ OrchestratorSpec, dir string, p OrchestratorPacket) error {
			return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("ok"), 0600)
		},
		Now: func() time.Time { return env.clock.now },
	}
	for i := 0; i < 3; i++ { // Every round rotates the lineage.
		st, _ := orchestratorState(s.Name)
		if err := RunOrchestratorRound(context.Background(), s, &st, d); err != nil {
			t.Fatal(err)
		}
		chatRound(s, st)
		if err := WriteJSON(filepath.Join(OrchestratorDir(s.Name), "state.json"), st); err != nil {
			t.Fatal(err)
		}
		flushOrchestratorNotices(s, func(context.Context, OrchestratorNotice) error { return nil })
	}
	if _, err := os.Stat(filepath.Join(OrchestratorDir(s.Name), "lineages")); err != nil {
		t.Fatal("rotation did not happen")
	}
	// Handoff ends the supervisor and posts in the same thread.
	if err := WriteJSON(filepath.Join(OrchestratorDir(s.Name), "handoff.json"), map[string]string{"to": "next-owner"}); err != nil {
		t.Fatal(err)
	}
	d.Notify = func(context.Context, OrchestratorNotice) error { return nil }
	if err := SuperviseOrchestrator(context.Background(), s.Name, d); err != nil {
		t.Fatal(err)
	}
	msgs := env.fake.Messages()
	if len(msgs) != 5 || !strings.Contains(msgs[4].Text, "handed off to next-owner") {
		t.Fatalf("%q", env.texts())
	}
	for _, m := range msgs[1:] {
		if m.ThreadTS != msgs[0].TS {
			t.Fatalf("rotation or handoff left the thread: %+v", m)
		}
	}
	// Reapply: a new lifetime, a new top-level root, then replies under it.
	if _, err := ApplyOrchestrator(spec, false); err != nil {
		t.Fatal(err)
	}
	st, _ := orchestratorState(s.Name)
	flushOrchestratorNotices(s, noPeers)
	chatLifecycle(s, st, "stopped", chat.Event{Kind: chat.EventStopped})
	flushOrchestratorNotices(s, noPeers)
	msgs = env.fake.Messages()
	root2, after := msgs[5], msgs[6]
	if root2.ThreadTS != "" || !strings.HasPrefix(root2.Text, "Orchestrator release-train started") || after.ThreadTS != root2.TS {
		t.Fatalf("reapply thread: %+v %+v", root2, after)
	}
	threads, _ := readThreads(s.Name)
	if len(threads) != 2 {
		t.Fatalf("one anchor per lifetime: %v", threads)
	}
}

func TestNotifyThreadLostAndRecovered(t *testing.T) {
	for _, mode := range []string{"file missing", "file unreadable", "anchor refused"} {
		t.Run(mode, func(t *testing.T) {
			env := chatHome(t, chatConfig)
			s := chatOne(t, env)
			switch mode {
			case "file missing":
				_ = os.Remove(threadsPath(s.Name))
			case "file unreadable":
				_ = os.WriteFile(threadsPath(s.Name), []byte("{not json"), 0600)
			case "anchor refused":
				env.fake.RefuseThreads(true)
			}
			flushOrchestratorNotices(s, noPeers)
			msgs := env.fake.Messages()
			lost := msgs[len(msgs)-1]
			if lost.ThreadTS != "" || !strings.HasPrefix(lost.Text, "(new thread) rel-42 is blocked") {
				t.Fatalf("lost anchor post: %+v", lost)
			}
			n := lastReceipt(t)
			if n.Status != "delivered" || !strings.HasPrefix(n.Message, "(new thread) ") || n.Thread != lost.TS {
				t.Fatalf("receipt %+v", n)
			}
			// Recovered: the new anchor is recorded and later posts reply to it.
			env.fake.RefuseThreads(false)
			st, _ := orchestratorState(s.Name)
			chatLifecycle(s, st, "stopped", chat.Event{Kind: chat.EventStopped})
			flushOrchestratorNotices(s, noPeers)
			msgs = env.fake.Messages()
			if next := msgs[len(msgs)-1]; next.ThreadTS != lost.TS || strings.HasPrefix(next.Text, "(new thread)") {
				t.Fatalf("not recovered: %+v", next)
			}
		})
	}
}

func TestNotifyChatExcludedFromPullRoute(t *testing.T) {
	env := chatHome(t, chatConfig)
	spec := strings.Replace(chatSpec, "reply: my-session", "reply: my-session\nnotify_route: pull", 1)
	s, err := ApplyOrchestrator(spec, false)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := PendingOrchestratorNotices(s.Name)
	if err != nil || len(pending) != 1 || isChatRecipient(pending[0].To) {
		t.Fatalf("pull must list only peers receipts: %+v %v", pending, err)
	}
	chatID := chatReceipts(t, s.Name)[0].ID
	if err := AckOrchestratorNotices([]string{chatID}); err != nil {
		t.Fatal(err)
	}
	if n := chatReceipts(t, s.Name)[0]; n.Status != "queued" {
		t.Fatalf("a pull ack must not mark a chat receipt delivered unsent: %+v", n)
	}
	peers := &noticeLog{}
	flushOrchestratorNotices(s, peers.send)
	if len(peers.sent) != 0 {
		t.Fatal("pull route executed a peers transport")
	}
	if n := chatReceipts(t, s.Name)[0]; n.Status != "delivered" || len(env.fake.Messages()) != 1 {
		t.Fatalf("chat receipts are pushed by the supervisor host: %+v", n)
	}
	all, _ := PendingOrchestratorNotices("")
	for _, n := range all {
		if isChatRecipient(n.To) {
			t.Fatal("chat receipt in host-wide pull")
		}
	}
}

func TestNotifyChatSkipsReplyGateButNotScopeOnly(t *testing.T) {
	env := chatHome(t, chatConfig)
	spec := strings.Replace(chatSpec, "reply: my-session\n", "", 1)
	spec = strings.Replace(spec, "authority: [say, notify, rework, retry]", "authority: [say, rework, retry]", 1)
	s, err := ApplyOrchestrator(spec, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(noticesOf(t, s.Name, "lifecycle")) != 0 || len(chatReceipts(t, s.Name)) != 1 {
		t.Fatal("chat posts need a notify mapping, not reply")
	}
	// A scope-only registration never posts.
	if _, err := RegisterOrchestratorScope(strings.Replace(spec, "name: release-train", "name: scoped", 1), Host{}, false); err != nil {
		t.Fatal(err)
	}
	scoped, _ := LoadOrchestrator("scoped")
	st, _ := orchestratorState("scoped")
	chatLifecycle(scoped, st, "stopped", chat.Event{Kind: chat.EventStopped})
	if len(chatReceipts(t, "scoped")) != 0 {
		t.Fatal("scope-only target queued a chat post")
	}
	_ = env
}

func TestNotifyReplyMDNeverReachesChat(t *testing.T) {
	env := chatHome(t, chatConfig)
	spec := strings.Replace(chatSpec, "  channel: builds\n", "  channel: builds\n  events: [all]\n", 1)
	s, err := ApplyOrchestrator(spec, false)
	if err != nil {
		t.Fatal(err)
	}
	peers := &noticeLog{}
	d := noticeDriver(t, peers.send)
	d.Session = func(_ context.Context, _ OrchestratorSpec, dir string, _ OrchestratorPacket) error {
		_ = os.WriteFile(filepath.Join(dir, "reply.md"), []byte("REPLY-SENTINEL ready to merge"), 0600)
		_ = os.WriteFile(filepath.Join(dir, "report.md"), []byte("REPORT-SENTINEL"), 0600)
		return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("CHECKPOINT-SENTINEL"), 0600)
	}
	d.Gather = func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) {
		return []OrchestratorEvidence{{Row: Row{Kind: "run", Meta: Meta{Order: Order{Name: "rel-42"}}, State: "done", PR: "https://github.com/example/app/pull/42"},
			Files: map[string]string{"status.md": "done: STATUS-SENTINEL", "report.md": "REPORT-SENTINEL"}}}, nil
	}
	st, _ := orchestratorState(s.Name)
	if err := RunOrchestratorRound(context.Background(), s, &st, d); err != nil {
		t.Fatal(err)
	}
	chatRound(s, st)
	flushOrchestratorNotices(s, peers.send)
	if peers.count("REPLY-SENTINEL") != 1 {
		t.Fatal("reply.md should go to the peers owner")
	}
	if len(env.fake.Messages()) < 3 {
		t.Fatalf("expected started, done and round posts: %q", env.texts())
	}
	assertNoLeak(t, env, s.Name, "REPLY-SENTINEL", "REPORT-SENTINEL", "CHECKPOINT-SENTINEL", "STATUS-SENTINEL")
}

// plantedSecrets are secret-shaped strings the scrubber recognises, built at
// run time so none sits in the source.
func plantedSecrets() []string {
	return []string{
		"xo" + "xb-123456789012-1234567890123-abcdefghijklmnopqrstuvwx",
		"gh" + "p_aB3dE5gH7jK9mN1pQ3sT5vW7yZ9bC1dF3hJ5",
		"ey" + "JhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
	}
}

func TestNotifyScrubsAgentFieldsWithCaptureRedactionOnAndOff(t *testing.T) {
	for _, capture := range []string{"on", "off"} {
		t.Run("capture redaction "+capture, func(t *testing.T) {
			env := chatHome(t, chatConfig)
			if capture == "off" {
				if err := os.WriteFile(filepath.Join(env.dir, "config.toml"), []byte("[capture]\nredact = false\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			secrets := plantedSecrets()
			planted := "use " + strings.Join(secrets, " and ")
			spec := strings.Replace(chatSpec, "goal: Every PR labelled release-0.9 is merged with green CI.", "goal: ship it "+secrets[1], 1)
			spec = strings.Replace(spec, "  channel: builds\n", "  channel: builds\n  events: [all]\n", 1)
			s, err := ApplyOrchestrator(spec, false)
			if err != nil {
				t.Fatal(err)
			}
			st, _ := orchestratorState(s.Name)
			start := time.Unix(1700000000, 0)
			evidence := []OrchestratorEvidence{
				{Row: Row{Kind: "run", Meta: Meta{Order: Order{Name: "rel-42"}, Start: start}, State: "needs-review", Outcome: OutcomeNeedsDecision, Key: "db", Question: planted}},
				{Row: Row{Kind: "run", Meta: Meta{Order: Order{Name: "rel-43"}, Start: start}, State: "blocked", Outcome: OutcomeBlocked, Reason: planted}},
				{Row: Row{Kind: "run", Meta: Meta{Order: Order{Name: "rel-44"}, Start: start}, State: "running"},
					Decisions: []Decision{{Key: "db", Question: planted, Answer: planted, Resolved: start, ResolvedBy: "local:owner " + secrets[0]}}},
			}
			chatRunEvents(s, st, evidence)
			chatLifecycle(s, st, "handoff", chat.Event{Kind: chat.EventHandoff, To: "next " + secrets[2]})
			_ = queueIntakeNotice(s, IntakeSignal{ID: "sig-1", Kind: "fetch-failed", Event: "evt " + secrets[0], Error: planted})
			flushOrchestratorNotices(s, func(context.Context, OrchestratorNotice) error { return nil })
			if got := len(env.fake.Messages()); got != 6 {
				t.Fatalf("posts %d: %q", got, env.texts())
			}
			assertNoLeak(t, env, s.Name, secrets...)
			redacted := 0
			for _, n := range chatReceipts(t, s.Name) {
				if n.Redactions != "" {
					redacted++
				}
			}
			if redacted < 5 {
				t.Fatalf("redaction counts should be recorded on receipts: %d", redacted)
			}
		})
	}
}

func TestNotifyExternalAudienceWithholdsAgentText(t *testing.T) {
	env := chatHome(t, chatConfig)
	spec := strings.Replace(chatSpec, "notifier: team\n  channel: builds", "notifier: client-a\n  channel: delivery", 1)
	spec = strings.Replace(spec, "manages:\n", "manages:\n  repos: [app]\n", 1)
	s, err := ApplyOrchestrator(spec, false)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := orchestratorState(s.Name)
	start := time.Unix(1700000000, 0)
	chatRunEvents(s, st, []OrchestratorEvidence{
		{Row: Row{Kind: "run", Meta: Meta{Order: Order{Name: "rel-42", Repo: "app"}, Start: start}, State: "needs-review", Outcome: OutcomeNeedsDecision, Key: "db", Question: "QUESTION-SENTINEL"}},
		{Row: Row{Kind: "run", Meta: Meta{Order: Order{Name: "rel-43", Repo: "app"}, Start: start}, State: "blocked", Outcome: OutcomeBlocked, Reason: "REASON-SENTINEL"}},
		{Row: Row{Kind: "run", Meta: Meta{Order: Order{Name: "rel-45", Repo: "elsewhere"}, Start: start}, State: "done"}},
	})
	flushOrchestratorNotices(s, noPeers)
	assertNoLeak(t, env, s.Name, "QUESTION-SENTINEL", "REASON-SENTINEL")
	for _, r := range env.fake.Requests() {
		if strings.Contains(r.Body, "build-1") {
			t.Fatal("host label posted to an external notifier")
		}
	}
	want := []string{
		"Orchestrator release-train started. Goal: Every PR labelled release-0.9 is merged with green CI. loop every 20m until goal. Details: hev orchestrate show release-train",
		"rel-42 needs a decision (db). The question is withheld from this channel; see hev dispatch show rel-42.",
		"rel-43 is blocked. The reason is withheld from this channel; see hev dispatch show rel-43.",
	}
	if got := env.texts(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("%q", got)
	}
	var withheld []string
	for _, n := range chatReceipts(t, s.Name) {
		if n.Withheld != "" {
			withheld = append(withheld, n.Withheld)
		}
		if strings.HasPrefix(n.Message, "rel-45") && (n.Status != "failed" || n.Error != "repo not allowed") {
			t.Fatalf("out-of-scope run: %+v", n)
		}
	}
	if strings.Join(withheld, " ") != "host question reason" {
		t.Fatalf("withheld audit: %v", withheld)
	}
}

// TestNotifyUnchangedHost proves a node with no notify.toml, a spec without
// notify: and a dispatch.toml with today's notify argv behave as before.
func TestNotifyUnchangedHost(t *testing.T) {
	env := chatHome(t, "")
	argv := `notify = ["peers", "send", "{reply}", "{name} {state}: {summary}"]` + "\n"
	cfgPath := filepath.Join(env.dir, "dispatch.toml")
	if err := os.WriteFile(cfgPath, []byte(argv+"[hosts.local]\nlocal = true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(cfgPath)
	if err != nil || strings.Join(c.Notify, " ") != "peers send {reply} {name} {state}: {summary}" {
		t.Fatalf("dispatch.toml notify argv: %v %v", c.Notify, err)
	}
	if _, err := loadTarget(cfgPath, ""); err != nil {
		t.Fatalf("target config: %v", err)
	}
	s, err := ApplyOrchestrator(orchestratorExample, false)
	if err != nil {
		t.Fatal(err)
	}
	peers := &noticeLog{}
	st, _ := orchestratorState(s.Name)
	if err := RunOrchestratorRound(context.Background(), s, &st, noticeDriver(t, peers.send)); err != nil {
		t.Fatal(err)
	}
	if peers.count("started/applied") != 1 {
		t.Fatalf("peers notices: %+v", peers.sent)
	}
	raw, _ := os.ReadFile(filepath.Join(OrchestratorDir(s.Name), "state.json"))
	if strings.Contains(string(raw), "applied_by") {
		t.Fatal("state.json gained a field on an unchanged host")
	}
	all, _ := loadOrchestratorNotices(s.Name)
	for _, f := range all {
		b, _ := os.ReadFile(f.path)
		for _, key := range []string{"notifier", "event", "requested_by", "lifetime", "thread", "error_class"} {
			if strings.Contains(string(b), `"`+key+`"`) {
				t.Fatalf("peers receipt gained %s: %s", key, b)
			}
		}
		if f.n.Kind == "notify" {
			t.Fatal("chat receipt on a node with no notify mapping")
		}
	}
	for _, p := range []string{threadsPath(s.Name), filepath.Join(Root(), ".notify")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("%s created on an unchanged host", p)
		}
	}
	if len(env.fake.Requests()) != 0 {
		t.Fatal("unchanged host contacted a chat app")
	}
	// A spec that asks for notify on a node without notify.toml is refused,
	// never silently dropped.
	if _, err := ApplyOrchestrator(chatSpec, true); err == nil || !errors.Is(err, chat.ErrNoConfig) {
		t.Fatalf("got %v", err)
	}
}

func noPeers(context.Context, OrchestratorNotice) error { return nil }
