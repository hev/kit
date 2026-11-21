package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

const (
	subsFixtureToken    = "fixture-cli-token-0000"
	subsFixtureAgyToken = "ya29.fixture-cli-agy-token-0000"
)

type subsFixture struct {
	agy    string // "signed_in", "api_key", "not_signed_in", "login_expired"
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
	agyDir := filepath.Join(home, ".gemini", "antigravity-cli")
	var agyProbe func(context.Context) (subscriptions.AgyProbeResult, error)
	switch f.agy {
	case "signed_in":
		write(filepath.Join(agyDir, "antigravity-oauth-token"),
			fmt.Sprintf(`{"token":{"access_token":%q,"refresh_token":"1//fixture-refresh","expiry":%q},"auth_method":"consumer"}`, subsFixtureAgyToken, subsNow.Add(time.Hour).Format(time.RFC3339)))
		write(filepath.Join(agyDir, "brain", "conv-1", ".system_generated", "logs", "transcript_full.jsonl"),
			`{"step_index":0,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2030-03-06T11:30:00Z","model":"gemini-4-carbon-high","input_tokens":4000,"output_tokens":500,"cache_read_tokens":500}`+"\n")
		agyProbe = func(context.Context) (subscriptions.AgyProbeResult, error) {
			return subscriptions.AgyProbeResult{Installed: true, Models: []string{"gemini-4-carbon-high", "gemini-4-neon-medium"}}, nil
		}
	case "api_key":
		write(filepath.Join(agyDir, "settings.json"), `{"modelProvider":"gemini"}`)
	case "login_expired":
		write(filepath.Join(agyDir, "antigravity-oauth-token"),
			fmt.Sprintf(`{"token":{"access_token":%q,"expiry":%q},"auth_method":"consumer"}`, subsFixtureAgyToken, subsNow.Add(-time.Hour).Format(time.RFC3339)))
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
	subscriptionsEnv = func(plans string) (subscriptions.Env, error) {
		return subscriptions.Env{
			Home: home, StateDir: filepath.Join(home, ".hev"),
			Now:            func() time.Time { return subsNow },
			Getenv:         func(string) string { return "" },
			AgyProbe:       agyProbe,
			HTTP:           srv.Client(),
			ClaudeUsageURL: srv.URL,
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
	if strings.Contains(out.String(), subsFixtureToken) || strings.Contains(out.String(), subsFixtureAgyToken) {
		t.Fatalf("output leaks the credential:\n%s", out.String())
	}
	return out.String(), err
}

func TestSubscriptionsListGrammar(t *testing.T) {
	useSubscriptionsFixture(t, subsFixture{agy: "signed_in", claude: "signed_in", codex: "signed_in"})
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
	if len(lines) != 4 || !strings.HasPrefix(lines[1], "agy ") || !strings.HasPrefix(lines[2], "claude ") || !strings.HasPrefix(lines[3], "codex ") {
		t.Fatalf("rows:\n%s", want)
	}
	if !strings.Contains(lines[1], "ultra") || !strings.Contains(lines[2], "max") || !strings.Contains(lines[2], "44%") || !strings.Contains(lines[3], "plus") || !strings.Contains(lines[3], "27%") {
		t.Fatalf("rows:\n%s", want)
	}
}

func TestSubscriptionsListStates(t *testing.T) {
	for _, tc := range []struct {
		f                 subsFixture
		agy, claude, codx string
	}{
		{subsFixture{"signed_in", "signed_in", "signed_in"}, "signed in", "signed in", "signed in"},
		{subsFixture{"not_signed_in", "not_signed_in", "not_signed_in"}, "not signed in", "not signed in", "not signed in"},
		{subsFixture{"api_key", "login_expired", "signed_in"}, "not signed in", "login expired", "signed in"},
		{subsFixture{"login_expired", "signed_in", "not_signed_in"}, "login expired", "signed in", "not signed in"},
	} {
		t.Run(tc.f.agy+"/"+tc.f.claude+"/"+tc.f.codex, func(t *testing.T) {
			useSubscriptionsFixture(t, tc.f)
			out, err := runSubs(t, "subscriptions")
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSpace(out), "\n")
			if !strings.Contains(lines[1], tc.agy) || !strings.Contains(lines[2], tc.claude) || !strings.Contains(lines[3], tc.codx) {
				t.Fatalf("want %q / %q / %q:\n%s", tc.agy, tc.claude, tc.codx, out)
			}
			js, err := runSubs(t, "subscriptions", "--json")
			if err != nil {
				t.Fatal(err)
			}
			var l subscriptions.List
			if err := json.Unmarshal([]byte(js), &l); err != nil || l.Schema != subscriptions.Schema || len(l.Subscriptions) != 3 {
				t.Fatalf("json %v: %s", err, js)
			}
			wantAgyState := tc.f.agy
			if wantAgyState == "api_key" {
				wantAgyState = "not_signed_in"
			}
			if l.Subscriptions[0].State != wantAgyState || l.Subscriptions[1].State != tc.f.claude || l.Subscriptions[2].State != tc.f.codex {
				t.Fatalf("states %s/%s/%s", l.Subscriptions[0].State, l.Subscriptions[1].State, l.Subscriptions[2].State)
			}
		})
	}
}

func TestSubscriptionsShowGrammar(t *testing.T) {
	hits := useSubscriptionsFixture(t, subsFixture{agy: "signed_in", claude: "signed_in", codex: "signed_in"})
	// The first read asks the endpoint; every later one is served from cache.
	if _, err := runSubs(t, "subscriptions"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"agy", "claude", "codex"} {
		want, err := runSubs(t, "subscriptions", "show", name)
		if err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"subscription", name}, {"subs", "show", name, "--refresh"}, {"subscriptions", name}} {
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
	useSubscriptionsFixture(t, subsFixture{agy: "signed_in", claude: "signed_in", codex: "signed_in"})
	for _, args := range [][]string{{"subscriptions", "show", "nope"}, {"subscription", "nope"}, {"subs", "nope", "--json"}} {
		out, err := runSubs(t, args...)
		exit, ok := err.(*commandExit)
		if !ok || exit.code != 2 || !strings.Contains(out, "agy, claude, codex") || !strings.Contains(out, "--help") {
			t.Fatalf("%v: output %q, err %v", args, out, err)
		}
	}
}

func TestSubscriptionsRPC(t *testing.T) {
	useSubscriptionsFixture(t, subsFixture{agy: "signed_in", claude: "not_signed_in", codex: "signed_in"})
	b, err := subscriptionsRPC(context.Background(), dispatch.Request{Action: "subscriptions"})
	if err != nil {
		t.Fatal(err)
	}
	var l subscriptions.List
	if err := json.Unmarshal(b, &l); err != nil || len(l.Subscriptions) != 3 {
		t.Fatalf("%v %s", err, b)
	}
	b, err = subscriptionsRPC(context.Background(), dispatch.Request{Action: "subscriptions", Name: "agy"})
	var dAgy subscriptions.Detail
	if err != nil || json.Unmarshal(b, &dAgy) != nil || dAgy.Subscription.Plan == nil || *dAgy.Subscription.Plan != "ultra" {
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

func TestSubscriptionsHostAndAllClusterSupport(t *testing.T) {
	useSubscriptionsFixture(t, subsFixture{agy: "signed_in", claude: "signed_in", codex: "signed_in"})
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "dispatch.toml")
	cfgBody := `
cap = 2
workspace = "~/workspace"

[hosts.mini]
local = true

[hosts.oven]
ssh = "hev@oven.example.invalid"

[hosts.win-oven]
ssh = "hev@win-oven.example.invalid"
os = "windows"
`
	if err := os.WriteFile(cfgPath, []byte(cfgBody), 0o600); err != nil {
		t.Fatal(err)
	}

	oldRPC := subscriptionsDispatchRPC
	subscriptionsDispatchRPC = func(ctx context.Context, h dispatch.Host, q dispatch.Request) (dispatch.Response, error) {
		if h.Local {
			// Exercise the real local dispatch.Handle -> SubscriptionsHandler path!
			return dispatch.RPC(ctx, h, q)
		}
		if strings.Contains(h.SSH, "win-oven") {
			// Simulate one offline Windows host to verify --all degrades cleanly without failing the fleet table.
			return dispatch.Response{}, errors.New("ssh: connect to host win-oven.example.invalid: Operation timed out")
		}
		// Simulate oven returning agy in API-key mode.
		planAPI := "api-key"
		agyStatus := subscriptions.Status{
			Name: "agy", Harness: "agy", Plan: &planAPI, State: subscriptions.NotSignedIn,
			StateReason: "Antigravity is configured for Gemini API-key billing", Source: "Antigravity Gemini API key",
			Meters: []subscriptions.Meter{}, ConfiguredPlans: []string{},
		}
		if q.Name == "agy" {
			raw, _ := json.Marshal(subscriptions.Detail{Schema: subscriptions.Schema, GeneratedAt: subsNow, Subscription: agyStatus})
			return dispatch.Response{Subscriptions: raw}, nil
		}
		raw, _ := json.Marshal(subscriptions.List{
			Schema:      subscriptions.Schema,
			GeneratedAt: subsNow,
			Subscriptions: []subscriptions.Status{
				agyStatus,
				{Name: "claude", Harness: "claude", State: subscriptions.NotSignedIn, Source: "Claude OAuth usage", Meters: []subscriptions.Meter{}, ConfiguredPlans: []string{}},
				{Name: "codex", Harness: "codex", State: subscriptions.SignedIn, Source: "codex rollouts", Meters: []subscriptions.Meter{}, ConfiguredPlans: []string{}},
			},
		})
		return dispatch.Response{Subscriptions: raw}, nil
	}
	t.Cleanup(func() { subscriptionsDispatchRPC = oldRPC })

	// 1. Single --host on local=true (`mini`) includes agy Ultra row with HOST column.
	outMini, err := runSubs(t, "subscriptions", "--config", cfgPath, "--host", "mini")
	if err != nil {
		t.Fatalf("--host mini failed: %v\n%s", err, outMini)
	}
	if !strings.HasPrefix(outMini, "HOST ") || !strings.Contains(outMini, "mini") || !strings.Contains(outMini, "ultra") {
		t.Fatalf("unexpected --host mini output:\n%s", outMini)
	}

	// 2. --all fans out across mini, oven, and win-oven in sorted host order.
	outAll, err := runSubs(t, "subscriptions", "--config", cfgPath, "--all")
	if err != nil {
		t.Fatalf("--all failed: %v\n%s", err, outAll)
	}
	for _, want := range []string{"mini", "ultra", "oven", "api-key", "win-oven", "unknown"} {
		if !strings.Contains(outAll, want) {
			t.Fatalf("--all output missing %q:\n%s", want, outAll)
		}
	}
	// SSH internal hostnames/errors must not leak into unknown rows.
	if strings.Contains(outAll, "win-oven.example.invalid") {
		t.Fatalf("--all leaked internal SSH address:\n%s", outAll)
	}

	// 3. `hev subscriptions agy --all --json` returns versioned List for agy across all 3 hosts.
	jsAgyAll, err := runSubs(t, "subscriptions", "agy", "--config", cfgPath, "--all", "--json")
	if err != nil {
		t.Fatalf("agy --all --json failed: %v", err)
	}
	var agyList subscriptions.List
	if err := json.Unmarshal([]byte(jsAgyAll), &agyList); err != nil || len(agyList.Subscriptions) != 3 {
		t.Fatalf("agy --all json: %v\n%s", err, jsAgyAll)
	}
	if agyList.Subscriptions[0].Host != "mini" || agyList.Subscriptions[1].Host != "oven" || agyList.Subscriptions[2].Host != "win-oven" {
		t.Fatalf("hosts out of order: %+v", agyList.Subscriptions)
	}

	// 4. Combining --host and --all is refused as a usage error (exit 2).
	outBoth, err := runSubs(t, "subscriptions", "--config", cfgPath, "--host", "mini", "--all")
	exit, ok := err.(*commandExit)
	if !ok || exit.code != 2 || !strings.Contains(outBoth, "cannot combine --host and --all") {
		t.Fatalf("expected exit 2 for --host + --all, got %v: %s", err, outBoth)
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
