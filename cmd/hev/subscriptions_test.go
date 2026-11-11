package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/subscriptions"
	"github.com/spf13/cobra"
)

var subsNow = time.Date(2030, 3, 6, 12, 0, 0, 0, time.UTC)

const subsFixtureToken = "fixture-cli-token-0000"

type subsFixture struct {
	claude string // "signed_in", "not_signed_in", "login_expired"
	codex  string // "signed_in", "not_signed_in"
}

// useSubscriptionsFixture points the command at a synthetic home and a local
// usage server, and returns the server's request count.
func useSubscriptionsFixture(t *testing.T, f subsFixture) *int {
	t.Helper()
	home := t.TempDir()
	hits := new(int)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits++
		if f.claude == "login_expired" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"limits":[{"kind":"session","percent":9,"resets_at":"2030-03-06T15:00:00Z"},{"kind":"weekly_all","percent":44,"resets_at":"2030-03-09T18:00:00Z"}]}`)
	}))
	t.Cleanup(srv.Close)
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if f.claude != "not_signed_in" {
		write(filepath.Join(home, ".claude", ".credentials.json"), fmt.Sprintf(`{"claudeAiOauth":{"accessToken":%q,"expiresAt":%d,"subscriptionType":"max"}}`, subsFixtureToken, subsNow.Add(time.Hour).UnixMilli()))
	}
	if f.codex == "signed_in" {
		write(filepath.Join(home, ".codex", "auth.json"), `{"auth_mode":"chatgpt"}`)
		write(filepath.Join(home, ".codex", "sessions", "2030", "03", "06", "rollout.jsonl"),
			fmt.Sprintf(`{"timestamp":"2030-03-06T11:00:00Z","type":"event_msg","payload":{"type":"token_count","rate_limits":{"limit_id":"codex","primary":{"used_percent":27,"window_minutes":10080,"resets_at":%d},"secondary":null,"plan_type":"plus"}}}`+"\n", subsNow.Add(30*time.Hour).Unix()))
	}
	old := subscriptionsEnv
	subscriptionsEnv = func(plans string, refresh bool) (subscriptions.Env, error) {
		return subscriptions.Env{
			Home: home, StateDir: filepath.Join(home, ".hev"),
			Now:            func() time.Time { return subsNow },
			Getenv:         func(string) string { return "" },
			HTTP:           srv.Client(),
			ClaudeUsageURL: srv.URL,
			Refresh:        refresh,
		}, nil
	}
	t.Cleanup(func() { subscriptionsEnv = old })
	return hits
}

func runSubs(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "hev", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(newSubscriptionsCommand())
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	if strings.Contains(out.String(), subsFixtureToken) {
		t.Fatalf("output leaks the credential:\n%s", out.String())
	}
	return out.String(), err
}

func TestSubscriptionsListGrammar(t *testing.T) {
	useSubscriptionsFixture(t, subsFixture{claude: "signed_in", codex: "signed_in"})
	want, err := runSubs(t, "subscriptions")
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"subscriptions", "ls"}, {"subscription"}, {"subs"}, {"subscriptions", "list"}} {
		got, err := runSubs(t, args...)
		if err != nil || got != want {
			t.Fatalf("%v: err %v\n%s\nwant\n%s", args, err, got, want)
		}
	}
	lines := strings.Split(strings.TrimSpace(want), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[1], "claude ") || !strings.HasPrefix(lines[2], "codex ") {
		t.Fatalf("rows:\n%s", want)
	}
	if !strings.Contains(lines[1], "max") || !strings.Contains(lines[1], "44%") || !strings.Contains(lines[2], "plus") || !strings.Contains(lines[2], "27%") {
		t.Fatalf("rows:\n%s", want)
	}
}

func TestSubscriptionsListStates(t *testing.T) {
	for _, tc := range []struct {
		f            subsFixture
		claude, codx string
	}{
		{subsFixture{"signed_in", "signed_in"}, "signed in", "signed in"},
		{subsFixture{"not_signed_in", "not_signed_in"}, "not signed in", "not signed in"},
		// Codex has no expiry signal in its local files; Claude reports 401.
		{subsFixture{"login_expired", "signed_in"}, "login expired", "signed in"},
	} {
		t.Run(tc.f.claude+"/"+tc.f.codex, func(t *testing.T) {
			useSubscriptionsFixture(t, tc.f)
			out, err := runSubs(t, "subscriptions")
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSpace(out), "\n")
			if !strings.Contains(lines[1], tc.claude) || !strings.Contains(lines[2], tc.codx) {
				t.Fatalf("want %q / %q:\n%s", tc.claude, tc.codx, out)
			}
			js, err := runSubs(t, "subscriptions", "--json")
			if err != nil {
				t.Fatal(err)
			}
			var l subscriptions.List
			if err := json.Unmarshal([]byte(js), &l); err != nil || l.Schema != subscriptions.Schema || len(l.Subscriptions) != 2 {
				t.Fatalf("json %v: %s", err, js)
			}
			if l.Subscriptions[0].State != tc.f.claude || l.Subscriptions[1].State != tc.f.codex {
				t.Fatalf("states %s/%s", l.Subscriptions[0].State, l.Subscriptions[1].State)
			}
		})
	}
}

func TestSubscriptionsShowGrammar(t *testing.T) {
	hits := useSubscriptionsFixture(t, subsFixture{claude: "signed_in", codex: "signed_in"})
	// The first read asks the endpoint; every later one is served from cache.
	if _, err := runSubs(t, "subscriptions"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"claude", "codex"} {
		want, err := runSubs(t, "subscriptions", "show", name)
		if err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"subscription", name}, {"subs", "show", name}, {"subscriptions", name}} {
			got, err := runSubs(t, args...)
			if err != nil || got != want {
				t.Fatalf("%v: err %v\n%s\nwant\n%s", args, err, got, want)
			}
		}
		if !strings.HasPrefix(want, name+"\n") || !strings.Contains(want, "weekly") || !strings.Contains(want, "source:") {
			t.Fatalf("detail:\n%s", want)
		}
		js, err := runSubs(t, "subscription", name, "--json")
		if err != nil {
			t.Fatal(err)
		}
		var d subscriptions.Detail
		if err := json.Unmarshal([]byte(js), &d); err != nil || d.Schema != subscriptions.Schema || d.Subscription.Name != name {
			t.Fatalf("json %v: %s", err, js)
		}
	}
	if *hits != 1 {
		t.Fatalf("nine Claude reads inside the interval made %d requests", *hits)
	}
}

func TestSubscriptionsUnknownName(t *testing.T) {
	useSubscriptionsFixture(t, subsFixture{claude: "signed_in", codex: "signed_in"})
	for _, args := range [][]string{{"subscriptions", "show", "nope"}, {"subscription", "nope"}, {"subs", "nope", "--json"}} {
		_, err := runSubs(t, args...)
		if err == nil || !strings.Contains(err.Error(), "claude, codex") {
			t.Fatalf("%v: err %v", args, err)
		}
	}
}

func TestSubscriptionsRPC(t *testing.T) {
	useSubscriptionsFixture(t, subsFixture{claude: "not_signed_in", codex: "signed_in"})
	b, err := subscriptionsRPC(context.Background(), dispatch.Request{Action: "subscriptions"})
	if err != nil {
		t.Fatal(err)
	}
	var l subscriptions.List
	if err := json.Unmarshal(b, &l); err != nil || len(l.Subscriptions) != 2 {
		t.Fatalf("%v %s", err, b)
	}
	b, err = subscriptionsRPC(context.Background(), dispatch.Request{Action: "subscriptions", Name: "codex"})
	var d subscriptions.Detail
	if err != nil || json.Unmarshal(b, &d) != nil || d.Subscription.Weekly == nil || *d.Subscription.Weekly.Percent != 27 {
		t.Fatalf("%v %s", err, b)
	}
	if _, err = subscriptionsRPC(context.Background(), dispatch.Request{Action: "subscriptions", Name: "nope"}); err == nil {
		t.Fatal("unknown name accepted")
	}
}

func TestSubscriptionsRPCRefusedToTeammates(t *testing.T) {
	_, err := dispatch.HandleGrant(dispatch.Request{Action: "subscriptions"}, "fixture-teammate", nil)
	if err == nil || !strings.Contains(err.Error(), "unavailable to teammates") {
		t.Fatalf("err = %v", err)
	}
}

func TestSubscriptionsInRootHelp(t *testing.T) {
	root := &cobra.Command{Use: "hev", Long: rootCmd.Long}
	root.AddCommand(newSubscriptionsCommand())
	groupFactoryCommands(root)
	var out bytes.Buffer
	root.SetOut(&out)
	if err := root.Help(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	factory, other := strings.Index(help, "Factory:"), strings.Index(help, "Other commands:")
	pos := strings.Index(help, "  subscriptions ")
	if pos < factory || (other >= 0 && pos > other) || !strings.Contains(help, "hev subs ") {
		t.Fatalf("subscriptions not listed under Factory:\n%s", help)
	}
}
