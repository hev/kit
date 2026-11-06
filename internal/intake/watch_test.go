package intake

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWatchConfigNormalizesClauses(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "watch.toml")
	text := fmt.Sprintf(`state_dir = %q
[linear]
api_key_env = "TEST_KEY"
team = "TEST"
assignee = "bot"
allowed_actor_team = "TEST"
[[watch]]
name = "delivery"
source = "linear"
[[watch.when]]
event = "status-becomes"
statuses = ["Ready", "Review"]
action = "fix"
effect = "dispatch"
template = "order.md"
host = "worker"
concurrency = "code"
[[watch.when]]
event = "status-becomes"
statuses = ["Shipped"]
action = "announce"
effect = "comment"
template = "comment.md"
`, root)
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Status) != 3 || c.Status[2].Effect != "comment" || c.Status[2].Concurrency != "read" {
		t.Fatalf("%+v", c.Status)
	}
}
func TestLegacyConfigMapsToWatch(t *testing.T) {
	path, _, _ := previewFixture(t, previewTemplate)
	c, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Watches) != 1 || c.Watches[0].When[0].Event != "status-becomes" {
		t.Fatal(c.Watches)
	}
}
func TestWatchCommentDoesNotBypassHumanGuard(t *testing.T) {
	f := setup(t)
	f.guest = true
	f.p.cfg.Status[0].Effect = "comment"
	f.poll(t)
	if strings.Contains(f.out.String(), "would-comment") {
		t.Fatal("guest authorized effect")
	}
}
func TestWatchCommentOnceAndUncertain(t *testing.T) {
	for _, reject := range []bool{false, true} {
		t.Run(fmt.Sprint(reject), func(t *testing.T) {
			f := setup(t)
			f.live(t)
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if reject {
					w.WriteHeader(503)
					return
				}
				fmt.Fprint(w, `{"data":{"commentCreate":{"success":true}}}`)
			}))
			defer server.Close()
			f.p.endpoint = server.URL
			rule := f.p.cfg.Status[0]
			rule.Effect = "comment"
			decision, _, err := f.p.applyEffect(context.Background(), f.issue(), rule, "test-comment")
			if err != nil {
				t.Fatal(err)
			}
			if reject && decision != "failed" || !reject && decision != "commented" {
				t.Fatal(decision)
			}
			_, _, err = f.p.applyEffect(context.Background(), f.issue(), rule, "test-comment")
			if err != nil || calls != 1 {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
			if reject && f.p.state.Effects["test-comment"] != "needs-reconcile" {
				t.Fatal("uncertainty lost")
			}
		})
	}
}

func TestWatchNotifyOnce(t *testing.T) {
	f := setup(t)
	f.live(t)
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.Header.Get("Idempotency-Key") != "test-notify" {
			t.Error("invalid notification")
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	f.p.client = server.Client()
	rule := f.p.cfg.Status[0]
	rule.Effect = "notify"
	rule.NotifyURL = server.URL
	decision, _, err := f.p.applyEffect(context.Background(), f.issue(), rule, "test-notify")
	if err != nil || decision != "notified" {
		t.Fatalf("%s %v", decision, err)
	}
	_, _, err = f.p.applyEffect(context.Background(), f.issue(), rule, "test-notify")
	if err != nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
