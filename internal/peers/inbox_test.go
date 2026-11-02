package peers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func fixture(t *testing.T) (*Engine, func(string, ...string) (string, error)) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "%7")
	e := New()
	e.State = t.TempDir()
	e.Host = "local"
	e.Command = func(_ context.Context, name string, _ io.Reader, args ...string) (string, error) {
		a := strings.Join(args, " ")
		if name == "ps" {
			if strings.Contains(a, "lstart=") {
				return "Mon Nov 2 12:00:00 2026", nil
			}
			return "700 1 claude\n", nil
		}
		if name == "tmux" {
			switch args[0] {
			case "list-panes":
				return "desk\t%7\t700\tclaude\t/work\tdesk\ton\n", nil
			case "display-message":
				return "100:$2:1234", nil
			case "set-option":
				return "", nil
			}
		}
		return "", fmt.Errorf("unexpected command %s %v", name, args)
	}
	call := func(input string, args ...string) (string, error) {
		var out bytes.Buffer
		err := e.Run(context.Background(), args, strings.NewReader(input), &out, io.Discard)
		return out.String(), err
	}
	return e, call
}
func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("%s: %v", s, err)
	}
	return m
}
func TestInboxModCompatibility(t *testing.T) {
	_, call := fixture(t)
	run := func(input string, args ...string) string {
		t.Helper()
		s, err := call(input, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return s
	}
	run("hello", "inbox", "hold", "--sender", "worker", "-", "--id", "first")
	run("world", "inbox", "hold", "--sender", "other", "-", "--id", "second")
	// These are the consumer's exact observation, consumption, and ACK shapes.
	s := run("", "inbox", "--peek", "--pending", "--json")
	m := decode(t, s)
	if m["on"] != true || m["unread"] != float64(2) || m["unacknowledged"] != float64(2) || m["recipient_incarnation"] == "" {
		t.Fatal(m)
	}
	ms := m["messages"].([]any)
	if len(ms) != 2 {
		t.Fatal(m)
	}
	first := ms[0].(map[string]any)
	for _, k := range []string{"id", "ts", "from", "header", "message", "recipient_session", "recipient_incarnation", "current", "consumed_at", "acknowledged_at", "transport_state"} {
		if _, ok := first[k]; !ok {
			t.Fatalf("missing %s", k)
		}
	}
	if first["from"] != "worker" || first["message"] != "hello" || first["id"] != "first" {
		t.Fatal(first)
	}
	if s := run("", "inbox", "--count"); s != "2\n" {
		t.Fatal(s)
	}
	if _, err := call("", "inbox", "consume", "first", "missing"); err == nil {
		t.Fatal("unknown consume accepted")
	}
	if s := run("", "inbox", "--count"); s != "2\n" {
		t.Fatal("partial consume", s)
	}
	if _, err := call("", "inbox", "ack", "first"); err == nil {
		t.Fatal("unconsumed ACK accepted")
	}
	run("", "inbox", "consume", "first")
	run("", "inbox", "consume", "first")
	if s := run("", "inbox", "--count"); s != "1\n" {
		t.Fatal(s)
	}
	if _, err := call("", "inbox", "ack", "first", "second"); err == nil {
		t.Fatal("batch ACK accepted unread item")
	}
	m = decode(t, run("", "inbox", "--peek", "--pending", "--json"))
	if m["unacknowledged"] != float64(2) {
		t.Fatal("partial ACK", m)
	}
	run("", "inbox", "consume", "second")
	if s := run("", "inbox", "--count"); s != "0\n" {
		t.Fatal(s)
	}
	m = decode(t, run("", "inbox", "--pending", "--limit", "8", "--json"))
	if len(m["messages"].([]any)) != 2 {
		t.Fatal("pending did not replay", m)
	}
	run("", "inbox", "ack", "first", "second")
	run("", "inbox", "ack", "first", "second")
	m = decode(t, run("", "inbox", "--peek", "--pending", "--json"))
	if len(m["messages"].([]any)) != 0 || m["unacknowledged"] != float64(0) {
		t.Fatal(m)
	}
	m = decode(t, run("", "inbox", "--all", "--json"))
	if len(m["messages"].([]any)) != 2 {
		t.Fatal("history lost", m)
	}
}
func TestInboxDefaultConsumesButPeekNeverDoes(t *testing.T) {
	_, call := fixture(t)
	call("hello", "inbox", "hold", "--sender", "worker", "-")
	for i := 0; i < 2; i++ {
		s, err := call("", "inbox", "--peek", "--json")
		if err != nil || decode(t, s)["unread"] != float64(1) {
			t.Fatal(s, err)
		}
	}
	if _, err := call("", "inbox", "--json"); err != nil {
		t.Fatal(err)
	}
	s, err := call("", "inbox", "--count")
	if err != nil || s != "0\n" {
		t.Fatal(s, err)
	}
	s, _ = call("", "inbox", "--peek", "--pending", "--json")
	if len(decode(t, s)["messages"].([]any)) != 1 {
		t.Fatal(s)
	}
}
func TestInboxLifetimeAndLegacyRetention(t *testing.T) {
	e, call := fixture(t)
	path := e.path("desk")
	os.MkdirAll(filepath.Dir(path), 0700)
	legacy := []byte("{\"ts\":1,\"from\":\"old\",\"message\":\"legacy\"}\n{torn")
	os.WriteFile(path, legacy, 0600)
	os.WriteFile(strings.TrimSuffix(path, ".jsonl")+".cursor", []byte("19"), 0600)
	call("hello", "inbox", "hold", "--sender", "worker", "-", "--id", "bound")
	s, _ := call("", "inbox", "--all", "--json")
	if len(decode(t, s)["messages"].([]any)) != 2 {
		t.Fatal(s)
	}
	old := e.Command
	e.Command = func(ctx context.Context, name string, in io.Reader, args ...string) (string, error) {
		if name == "tmux" && args[0] == "display-message" {
			return "100:$3:1235", nil
		}
		return old(ctx, name, in, args...)
	}
	s, _ = call("", "inbox", "--peek", "--pending", "--json")
	m := decode(t, s)
	if m["historical"] != float64(2) || m["unread"] != float64(0) {
		t.Fatal(m)
	}
	if _, err := call("", "inbox", "consume", "bound"); err == nil {
		t.Fatal("consumed earlier lifetime")
	}
	b, _ := os.ReadFile(path)
	if !bytes.HasPrefix(b, legacy) {
		t.Fatal("legacy bytes rewritten")
	}
}
func TestInboxConcurrentRetryAndConflict(t *testing.T) {
	e, _ := fixture(t)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.hold("desk", "life", Message{"id": "retry", "from": "sender", "message": "one"})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	ms, err := rows(e.path("desk"))
	if err != nil || len(ms) != 1 {
		t.Fatal(ms, err)
	}
	if _, err := e.hold("desk", "life", Message{"id": "retry", "from": "sender", "message": "different"}); err == nil {
		t.Fatal("conflicting retry accepted")
	}
}
func TestInboxOwnershipIgnoresSenderOverride(t *testing.T) {
	_, call := fixture(t)
	call("hello", "inbox", "hold", "--sender", "worker", "-", "--id", "one")
	t.Setenv("TMUX_PANE", "")
	if _, err := call("", "inbox", "consume", "one", "--from", "desk", "--session", "desk"); err == nil {
		t.Fatal("sender override impersonated recipient")
	}
	if _, err := call("", "inbox", "--peek", "--pending", "--json", "--session", "desk"); err != nil {
		t.Fatal("inspection refused", err)
	}
}
