package notify_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/notify"
	"github.com/hev/kit/internal/notify/slackfake"
)

const token = "xo" + "xb-000000000000-000000000000-" + "FAKEFAKEFAKEFAKEFAKEFAKE"

func client(f *slackfake.Server, now time.Time) notify.Slack {
	return notify.Slack{BaseURL: f.BaseURL(), Token: token, Now: func() time.Time { return now }}
}

func TestSlackPostAndThread(t *testing.T) {
	f := slackfake.New(token, "C0123456789")
	defer f.Close()
	c := client(f, time.Now())
	ref, anchor, err := c.Post(context.Background(), "C0123456789", "root <!channel> & @everyone", "")
	if err != nil || ref != "C0123456789/1700000000.000100" || anchor != "1700000000.000100" {
		t.Fatal(ref, anchor, err)
	}
	ref2, anchor2, err := c.Post(context.Background(), "C0123456789", "reply", anchor)
	if err != nil || anchor2 != anchor || ref2 == ref {
		t.Fatal(ref2, anchor2, err)
	}
	msgs := f.Messages()
	if msgs[0].Text != "root &lt;!channel&gt; &amp; @everyone" || msgs[1].ThreadTS != anchor {
		t.Fatalf("%+v", msgs)
	}
	for _, r := range f.Requests() {
		if strings.Contains(r.Body, token) || r.Auth != "Bearer "+token {
			t.Fatalf("token must travel only in the header: %+v", r)
		}
		var body map[string]any
		if json.Unmarshal([]byte(r.Body), &body) == nil {
			if _, ok := body["link_names"]; ok {
				t.Fatal("link_names must not be set")
			}
		}
	}
}

func TestSlackFailureClasses(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		setup func(*slackfake.Server)
		class notify.Class
		code  string
	}{
		{"rate limit", func(f *slackfake.Server) { f.RateLimit(30) }, notify.ClassRateLimited, "ratelimited"},
		{"server error", func(f *slackfake.Server) { f.ServerErrors(1) }, notify.ClassTransient, "server error"},
		{"revoked", func(f *slackfake.Server) { f.Revoke(token) }, notify.ClassCredential, "token_revoked"},
		{"invalid auth", func(f *slackfake.Server) { f.FailWith("invalid_auth") }, notify.ClassCredential, "invalid_auth"},
		{"account inactive", func(f *slackfake.Server) { f.FailWith("account_inactive") }, notify.ClassCredential, "account_inactive"},
		{"missing scope", func(f *slackfake.Server) { f.FailWith("missing_scope") }, notify.ClassCredential, "missing_scope"},
		{"channel gone", func(f *slackfake.Server) { f.RemoveChannel("C0123456789") }, notify.ClassChannel, "channel_not_found"},
		{"not in channel", func(f *slackfake.Server) { f.SetChannel("C0123456789", slackfake.Channel{}) }, notify.ClassChannel, "not_in_channel"},
		{"archived", func(f *slackfake.Server) {
			f.SetChannel("C0123456789", slackfake.Channel{Member: true, Archived: true})
		}, notify.ClassChannel, "is_archived"},
		{"no text", func(f *slackfake.Server) { f.FailWith("no_text") }, notify.ClassPayload, "no_text"},
		{"blocks too long", func(f *slackfake.Server) { f.FailWith("msg_blocks_too_long") }, notify.ClassPayload, "msg_blocks_too_long"},
		{"hostile code", func(f *slackfake.Server) { f.FailWith("see https://evil.example/" + token) }, notify.ClassTransient, "unknown error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := slackfake.New(token, "C0123456789")
			defer f.Close()
			tc.setup(f)
			_, _, err := client(f, now).Post(context.Background(), "C0123456789", "hello", "")
			var e *notify.Error
			if !errors.As(err, &e) || e.Class != tc.class || e.Code != tc.code {
				t.Fatalf("got %#v", err)
			}
			if strings.Contains(err.Error(), "xoxb") || strings.Contains(err.Error(), "http") {
				t.Fatalf("error text leaks: %v", err)
			}
			if tc.class == notify.ClassRateLimited && !e.RetryAt.Equal(now.Add(30*time.Second)) {
				t.Fatalf("retry hint not honoured: %v", e.RetryAt)
			}
		})
	}
}

func TestSlackNetworkErrorHidesURL(t *testing.T) {
	f := slackfake.New(token)
	base := f.BaseURL()
	f.Close()
	_, _, err := notify.Slack{BaseURL: base, Token: token}.Post(context.Background(), "C0123456789", "x", "")
	if notify.Classify(err) != notify.ClassTransient || strings.Contains(err.Error(), "127.0.0.1") || strings.Contains(err.Error(), "xoxb") {
		t.Fatalf("%v", err)
	}
}

func TestSlackThreadRefused(t *testing.T) {
	f := slackfake.New(token, "C0123456789")
	defer f.Close()
	_, _, err := client(f, time.Now()).Post(context.Background(), "C0123456789", "x", "1600000000.000100")
	if notify.Classify(err) != notify.ClassThread {
		t.Fatal(err)
	}
}
