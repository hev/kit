package notify_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hev/kit/internal/notify"
	"github.com/hev/kit/internal/notify/slackfake"
)

const checkConfig = `[notifiers.team]
kind = "slack"
token = "op://example-vault/kit-slack-bot/credential"
channels = { builds = "C0123456789" }
`

func runCheck(t *testing.T, f *slackfake.Server, cfgText string, resolveErr error) []notify.CheckResult {
	t.Helper()
	cfg, err := notify.Decode([]byte(cfgText))
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(context.Context, notify.Notifier, bool) (string, error) { return token, resolveErr }
	res, err := notify.Check(context.Background(), cfg, "", resolve, func(tok string) notify.Slack {
		return notify.Slack{BaseURL: f.BaseURL(), Token: tok}
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		b, _ := json.Marshal(r)
		for _, s := range []string{r.String(), string(b)} {
			if strings.Contains(s, "xoxb") || strings.Contains(s, "FAKE") || strings.Contains(s, "op://") || strings.Contains(s, "C0123456789") {
				t.Fatalf("check output leaks: %s", s)
			}
		}
	}
	return res
}

func TestCheckOutputs(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*slackfake.Server)
		err   error
		cfg   string
		want  string
		ok    bool
	}{
		{"healthy", func(*slackfake.Server) {}, nil, checkConfig, "team: slack, auth ok (workspace Example), builds: member", true},
		{"bad token", func(f *slackfake.Server) { f.Revoke(token) }, nil, checkConfig, "team: auth failed (token_revoked)", false},
		{"not in channel", func(f *slackfake.Server) { f.SetChannel("C0123456789", slackfake.Channel{}) }, nil, checkConfig, "team: slack, auth ok (workspace Example), builds: not a member (invite the app to the channel)", false},
		{"missing read scopes", func(f *slackfake.Server) { f.SetReadScope(false) }, nil, checkConfig, "team: slack, auth ok (workspace Example), builds: membership unknown (add channels:read and groups:read to check)", true},
		{"channel gone", func(f *slackfake.Server) { f.RemoveChannel("C0123456789") }, nil, checkConfig, "team: slack, auth ok (workspace Example), builds: channel not found", false},
		{"no provider", func(*slackfake.Server) {}, errors.New("no secrets provider for notifier team"), checkConfig, "team: credential not resolved (no secrets provider for notifier team)", false},
		{"external without repos", func(*slackfake.Server) {}, nil, strings.Replace(checkConfig, `kind = "slack"`, "kind = \"slack\"\naudience = \"external\"", 1), "team: slack, auth ok (workspace Example), builds: member; warning: external notifier has no repos list", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := slackfake.New(token, "C0123456789")
			defer f.Close()
			tc.setup(f)
			res := runCheck(t, f, tc.cfg, tc.err)
			if len(res) != 1 || res[0].String() != tc.want || res[0].OK() != tc.ok {
				t.Fatalf("got %q ok=%v", res[0].String(), res[0].OK())
			}
			for _, r := range f.Requests() {
				if r.Method == "chat.postMessage" {
					t.Fatal("check must not post")
				}
			}
		})
	}
}
