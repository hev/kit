package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/attention"
)

func subscriptionSpec(subscription string) string {
	return strings.Replace(orchestratorExample, "source: none\n", "source: none\n"+subscription, 1)
}

func TestOrchestratorSubscriptionValidation(t *testing.T) {
	orchHome(t)
	t.Setenv("HEV_DISPATCH_CONFIG", filepath.Join(t.TempDir(), "dispatch.toml"))
	for _, tc := range []struct{ name, yaml, err string }{
		{"absent", "", ""},
		{"subset", "subscribe: {events: [ready]}\n", ""},
		{"all filters", "subscribe: {events: [done, ready, needs-you], hosts: [mini, oven], sources: [run, orchestrator, follow, swarm, steer, cron, service]}\n", ""},
		{"empty events", "subscribe: {events: []}\n", "subscribe.events"},
		{"null subscription", "subscribe: null\n", "subscribe.events"},
		{"unknown event", "subscribe: {events: [ready, started]}\n", "subscribe.events"},
		{"invalid host", "subscribe: {events: [done], hosts: [mini/path]}\n", "subscribe.hosts"},
		{"unknown source", "subscribe: {events: [done], sources: [expedite]}\n", "subscribe.sources"},
		{"unknown field", "subscribe: {events: [done], repos: [app]}\n", "field repos not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec := subscriptionSpec(tc.yaml)
			for _, validate := range []func(string) (OrchestratorSpec, error){
				ParseOrchestrator,
				func(s string) (OrchestratorSpec, error) { return ApplyOrchestrator(s, true) },
			} {
				got, err := validate(spec)
				if tc.err == "" {
					if err != nil || (got.Subscribe == nil) != (tc.yaml == "") {
						t.Fatalf("subscription = %+v, err = %v", got.Subscribe, err)
					}
				} else if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("expected %q, got %v", tc.err, err)
				}
			}
		})
	}
}

func subscriptionEmitter(t *testing.T, config string) *attention.Emitter {
	t.Helper()
	orchHome(t)
	t.Setenv("HEV_ORDERS_DIR", t.TempDir())
	t.Setenv("HEV_HOOKS", "")
	t.Setenv("HEV_ATTENTION_DIR", t.TempDir())
	t.Setenv("HEV_DISPATCH_CONFIG", filepath.Join(t.TempDir(), "dispatch.toml"))
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("HEV_CONFIG", path)
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	e := attention.Default()
	e.Hooks = func(context.Context, string, []byte) {}
	if e.ConfigErr != nil {
		t.Fatal(e.ConfigErr)
	}
	return e
}

func TestOrchestratorSubscriptionMatchingAndRedaction(t *testing.T) {
	for _, tc := range []struct {
		name, kind, source, host, run, extraConfig string
		deliver                                    bool
	}{
		{name: "done", kind: "done", source: "run", host: "test-host", run: "job", deliver: true},
		{name: "ready", kind: "ready", source: "run", host: "mini", run: "job", deliver: true},
		{name: "needs-you", kind: "needs-you", source: "orchestrator", host: "test-host", run: "other", deliver: true},
		{name: "same name on another host", kind: "done", source: "orchestrator", host: "mini", run: "release-train", deliver: true},
		{name: "same name from run", kind: "done", source: "run", host: "test-host", run: "release-train", deliver: true},
		{name: "self", kind: "done", source: "orchestrator", host: "test-host", run: "release-train"},
		{name: "other source", kind: "done", source: "cron", host: "test-host", run: "job"},
		{name: "other host", kind: "done", source: "run", host: "oven", run: "job"},
		{name: "path in metadata", kind: "done", source: "run", host: "test-host", run: "/private/job"},
		{name: "disabled in config", kind: "done", source: "run", host: "test-host", run: "job", extraConfig: "\n[hooks]\nenabled = false\n"},
		{name: "disabled in environment", kind: "done", source: "run", host: "test-host", run: "job"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			emitter := subscriptionEmitter(t, "[notify]\nclient = true\n"+tc.extraConfig)
			if tc.name == "disabled in environment" {
				t.Setenv("HEV_HOOKS", "off")
			}
			spec, err := ApplyOrchestrator(subscriptionSpec("subscribe: {events: [done, ready, needs-you], sources: [run, orchestrator], hosts: [test-host, mini]}\n"), false)
			if err != nil {
				t.Fatal(err)
			}
			n := attention.New(tc.kind, tc.source, tc.host, tc.run, "key", "complete",
				"private-secret\n/private/customer/repo", attention.Links{PR: "https://private.example/pull/7", Run: "/private/logs/run"})
			for range 2 {
				if err := emitter.Emit(context.Background(), n, ""); err != nil {
					t.Fatal(err)
				}
			}
			dir := OrchestratorDir(spec.Name)
			if exists(filepath.Join(dir, "wake")) != tc.deliver {
				t.Fatal("unexpected wake state")
			}
			inbox := filepath.Join(dir, "inbox.md")
			got, err := os.ReadFile(inbox)
			if tc.deliver {
				want := "[event] " + tc.kind + " " + tc.source + " " + tc.host + "/" + tc.run + " complete\n"
				if err != nil || string(got) != want {
					t.Fatalf("inbox = %q, error = %v, want %q", got, err, want)
				}
			} else if (err != nil && !os.IsNotExist(err)) || len(got) != 0 {
				t.Fatalf("nonmatching event wrote inbox: %q, %v", got, err)
			}
		})
	}
}

func TestQueuedOrchestratorSubscriptionUsesUnlockedFlush(t *testing.T) {
	emitter := subscriptionEmitter(t, "")
	spec, err := ApplyOrchestrator(subscriptionSpec("subscribe: {events: [ready]}\n"), false)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	unlock, err := lockRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"done", "ready"} {
		n := attention.New(kind, attention.SourceRun, "test-host", "job", "key", "complete", "", attention.Links{})
		if err := emitter.Queue(ctx, n, ""); err != nil {
			unlock()
			t.Fatal(err)
		}
	}
	unlock()
	dir := OrchestratorDir(spec.Name)
	if exists(filepath.Join(dir, "wake")) {
		t.Fatal("queue woke orchestrator before flush")
	}
	emitter.Flush(ctx)
	emitter.Flush(ctx)
	b, err := os.ReadFile(filepath.Join(dir, "inbox.md"))
	if err != nil || string(b) != "[event] ready run test-host/job complete\n" || !exists(filepath.Join(dir, "wake")) {
		t.Fatalf("flush did not deliver and wake exactly once: inbox = %q, error = %v", b, err)
	}
}

func TestOrchestratorEventRPCEnforcesTargetSubscription(t *testing.T) {
	subscriptionEmitter(t, "")
	spec, err := ApplyOrchestrator(subscriptionSpec("subscribe: {events: [done], hosts: [mini]}\n"), false)
	if err != nil {
		t.Fatal(err)
	}
	event := attention.EventSummary{Event: "done", Source: "run", Host: "oven", Name: "job"}
	q := Request{Action: "orchestrator-event", Name: "o-" + spec.Name, Event: &event}
	wake := filepath.Join(OrchestratorDir(spec.Name), "wake")
	if _, err := Handle(q); err != nil || exists(wake) {
		t.Fatalf("nonmatching event accepted: %v", err)
	}
	event.Host = "mini"
	event.State = "complete\nprivate"
	if _, err := Handle(q); err == nil || exists(wake) {
		t.Fatalf("invalid metadata accepted: %v", err)
	}
	event.State = "complete"
	if _, err := Handle(q); err != nil || !exists(wake) {
		t.Fatalf("matching remote event did not wake target: %v", err)
	}
}
