package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/notify"
	"github.com/hev/kit/internal/notify/slackfake"
)

// An obviously fake bot token; it must never appear in output.
const fakeNotifyToken = "xo" + "xb-000000000000-000000000000-" + "FAKEFAKEFAKEFAKEFAKEFAKE"

func TestNotifyCheckCommand(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*slackfake.Server)
		want  string
		ok    bool
	}{
		{"healthy", func(*slackfake.Server) {}, "team: slack, auth ok (workspace Example), builds: member\n", true},
		{"bad token", func(f *slackfake.Server) { f.Revoke(fakeNotifyToken) }, "team: auth failed (token_revoked)\n", false},
		{"app not in channel", func(f *slackfake.Server) { f.SetChannel("C0123456789", slackfake.Channel{}) }, "team: slack, auth ok (workspace Example), builds: not a member (invite the app to the channel)\n", false},
		{"missing read scopes", func(f *slackfake.Server) { f.SetReadScope(false) }, "team: slack, auth ok (workspace Example), builds: membership unknown (add channels:read and groups:read to check)\n", true},
	}
	for _, tc := range cases {
		for _, asJSON := range []bool{false, true} {
			t.Run(tc.name, func(t *testing.T) {
				dir := t.TempDir()
				t.Setenv("HEV_DISPATCH_CONFIG", filepath.Join(dir, "dispatch.toml"))
				if err := os.WriteFile(filepath.Join(dir, "notify.toml"), []byte("[notifiers.team]\nkind = \"slack\"\ntoken = \"op://example-vault/kit-slack-bot/credential\"\nchannels = { builds = \"C0123456789\" }\n"), 0600); err != nil {
					t.Fatal(err)
				}
				f := slackfake.New(fakeNotifyToken, "C0123456789")
				defer f.Close()
				tc.setup(f)
				oldR, oldB := notifyCheckResolver, notifyCheckBase
				defer func() { notifyCheckResolver, notifyCheckBase = oldR, oldB }()
				notifyCheckResolver = func(context.Context, notify.Notifier, bool) (string, error) { return fakeNotifyToken, nil }
				notifyCheckBase = f.BaseURL()
				var out bytes.Buffer
				ok, err := runNotifyCheck(context.Background(), &out, "", asJSON)
				if err != nil || ok != tc.ok {
					t.Fatalf("ok=%v err=%v", ok, err)
				}
				if !asJSON && out.String() != tc.want {
					t.Fatalf("got %q", out.String())
				}
				for _, leak := range []string{"xoxb", "FAKE", "op://", "example-vault", "C0123456789"} {
					if strings.Contains(out.String(), leak) {
						t.Fatalf("output leaks %s: %s", leak, out.String())
					}
				}
				for _, r := range f.Requests() {
					if r.Method == "chat.postMessage" {
						t.Fatal("check posted")
					}
				}
			})
		}
	}
}
