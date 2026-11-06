package intake

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func reviewCommentFixture(t *testing.T) (*fixture, *int) {
	f := setup(t)
	f.live(t)
	tpl := filepath.Join(f.p.cfg.StateDir, "comment.md")
	if err := os.WriteFile(tpl, []byte("Shipped {{identifier}}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r := &f.p.cfg.Status[0]
	r.Effect, r.Template = "comment", tpl
	posted := new(int)
	return f, posted
}

// A rate-limit reset observed on an earlier response in the same poll makes the
// comment effect fail locally (no request) yet it is recorded as uncertain.
func TestReviewBackoffMarksUnsentEffectUncertain(t *testing.T) {
	f, posted := reviewCommentFixture(t)
	f.p.cfg.Limits.MinRemaining = 0
	f.stoppedHandler = func(w http.ResponseWriter, q string) {
		*posted++
		fmt.Fprint(w, `{"data":{"commentCreate":{"success":true}}}`)
	}
	// First poll: warm the authorization cache while the clause is off.
	f.p.cfg.Status[0].Mode = "off"
	f.poll(t)
	f.p.cfg.Status[0].Mode = "dry-run"
	f.poll(t)
	f.p.cfg.Status[0].Mode = "live"
	reset := f.p.now().Add(10 * time.Minute).UnixMilli()
	f.headers = map[string]string{"X-RateLimit-Requests-Remaining": "0", "X-RateLimit-Requests-Limit": "100", "X-RateLimit-Requests-Reset": fmt.Sprint(reset)}
	f.out.Reset()
	f.poll(t)
	t.Logf("posted=%d effects=%v needs=%v\n%s", *posted, f.p.state.Effects, f.p.state.NeedsReconcile, f.out.String())
	if *posted != 0 || len(f.p.state.NeedsReconcile) > 0 || len(f.p.state.Effects) != 0 || f.p.day().Dispatches != 0 {
		t.Errorf("REPRODUCED: no comment request sent, but effect marked needs-reconcile")
	}
}

// A successful commentCreate whose response reports quota below the threshold
// is recorded as uncertain instead of completed.
func TestReviewQuotaHeaderOnSuccessfulComment(t *testing.T) {
	f, posted := reviewCommentFixture(t)
	second := f.issue()
	second.ID = "second"
	second.Identifier = "TEST-2"
	f.extraIssues = []Issue{second}
	f.stoppedHandler = func(w http.ResponseWriter, q string) {
		*posted++
		w.Header().Set("X-RateLimit-Requests-Remaining", "5")
		w.Header().Set("X-RateLimit-Requests-Limit", "100")
		fmt.Fprint(w, `{"data":{"commentCreate":{"success":true}}}`)
	}
	f.poll(t)
	t.Logf("posted=%d effects=%v\n%s", *posted, f.p.state.Effects, f.out.String())
	if *posted != 1 || len(f.p.state.Effects) != 1 {
		t.Fatalf("posted=%d effects=%v", *posted, f.p.state.Effects)
	}
	if f.p.state.Effects != nil {
		for _, v := range f.p.state.Effects {
			if v != "completed" {
				t.Errorf("REPRODUCED: comment created (success=true) but recorded needs-reconcile")
			}
		}
	}
}

// Legacy [[mention]] accepts effect/notify_url and silently dispatches instead.
func TestReviewMentionEffectIgnored(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "intake.toml")
	text := fmt.Sprintf(`state_dir = %q
[linear]
api_key_env = "TEST_KEY"
team = "TEST"
assignee = "bot"
allowed_actor_team = "TEST"
[[mention]]
action = "feedback"
effect = "notify"
notify_url = "https://hooks.example.invalid/x"
template = "f.md"
host = "worker"
concurrency = "code"
`, root)
	os.WriteFile(path, []byte(text), 0600)
	c, err := loadConfig(path)
	if err == nil {
		t.Errorf("REPRODUCED: mention effect=%q accepted; mention path dispatches regardless (watches=%+v)", c.Mention[0].Effect, c.Watches[0].When[0].Rule.Effect)
	}
}

func TestEffectPreSendDefersWithoutCharge(t *testing.T) {
	for _, kind := range []string{"backoff", "quota", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			f, _ := reviewCommentFixture(t)
			ctx := t.Context()
			switch kind {
			case "backoff":
				f.p.state.BackoffUntil = f.p.now().Add(time.Hour)
			case "quota":
				f.p.state.SkipNext = true
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			decision, _, err := f.p.applyEffect(ctx, Issue{}, f.p.cfg.Status[0], "test-effect")
			if err != nil || decision != "skipped" || len(f.p.state.Effects) != 0 || f.p.day().Dispatches != 0 {
				t.Fatalf("%s %v %+v", decision, err, f.p.state)
			}
		})
	}
}

func TestCompletedEffectsPrunedButUncertainRetained(t *testing.T) {
	f := setup(t)
	f.p.state.Effects = map[string]string{"old": "completed", "pending": "pending", "uncertain": "needs-reconcile", "record": "completed"}
	f.p.pruneLeases(nil, map[string]bool{"record": true})
	if len(f.p.state.Effects) != 3 || f.p.state.Effects["old"] != "" || f.p.state.Effects["record"] != "completed" {
		t.Fatal(f.p.state.Effects)
	}
	i := Issue{Identifier: "TEST-1", ID: "1", UpdatedAt: "new"}
	f.p.state.Effects["test-1-unchecked"] = "completed"
	f.p.pruneLeases([]Issue{i}, nil)
	if f.p.state.Effects["test-1-unchecked"] != "completed" {
		t.Fatal("unchecked candidate pruned")
	}
}

func TestNotifyURLRejectedForOtherEffects(t *testing.T) {
	for _, effect := range []string{"", "dispatch", "comment"} {
		if err := validateEffect(Rule{Effect: effect, NotifyURL: "https://example.invalid/hook"}); err == nil {
			t.Fatal(effect)
		}
	}
}
