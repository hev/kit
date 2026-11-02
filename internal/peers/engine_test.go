package peers

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"testing"
)

type recordingTransport struct {
	host string
	args []string
	body string
}

func (r *recordingTransport) Execute(_ context.Context, h string, args []string, in io.Reader, out, errout io.Writer) error {
	r.host = h
	r.args = args
	b, _ := io.ReadAll(in)
	r.body = string(b)
	return nil
}
func TestSSHBoundary(t *testing.T) {
	e, call := fixture(t)
	r := &recordingTransport{}
	e.Transport = r
	if _, err := call("message", "send", "remote/desk", "-", "--from", "local/sender", "--id", "stable"); err != nil {
		t.Fatal(err)
	}
	if r.host != "remote" || strings.Join(r.args, " ") != "send desk - --from local/sender --id stable" || r.body != "message" {
		t.Fatal(r)
	}
	if _, err := call("", "inbox", "--session", "remote/desk", "--peek", "--pending", "--json"); err != nil {
		t.Fatal(err)
	}
	if _, err := call("", "inbox", "ack", "one", "--session", "remote/desk"); err == nil {
		t.Fatal("remote mutation permitted")
	}
}
func TestUnknownHarnessInboxAndRead(t *testing.T) {
	e, call := fixture(t)
	old := e.Command
	e.Command = func(ctx context.Context, name string, in io.Reader, args ...string) (string, error) {
		if name == "tmux" && args[0] == "capture-pane" {
			return "sample screen", nil
		}
		return old(ctx, name, in, args...)
	}
	if s, err := call("", "read", "desk", "-n", "5"); err != nil || s != "sample screen" {
		t.Fatal(s, err)
	}
	s, err := call("", "send", "desk", "held", "--from", "sender")
	if err != nil || !strings.Contains(s, "queued in inbox") {
		t.Fatal(s, err)
	}
}
func TestScreenRefusals(t *testing.T) {
	for s, want := range map[string]string{"❯ ": "idle", "› human draft": "draft", "Do you trust this folder?": "prompt", "opaque": "idle"} {
		if got := screenState(s); got != want {
			t.Fatalf("%q: %s", s, got)
		}
	}
}

func TestInboxRegistrationOnUnregisteredShell(t *testing.T) {
	e, call := fixture(t)
	old := e.Command
	e.Command = func(ctx context.Context, name string, in io.Reader, args ...string) (string, error) {
		if name == "tmux" && args[0] == "list-panes" {
			return "desk\t%7\t700\tbash\t/work\t\t\n", nil
		}
		return old(ctx, name, in, args...)
	}
	if _, err := call("", "inbox", "on"); err != nil {
		t.Fatal(err)
	}
}

func TestInboxSharedByMultiplePanes(t *testing.T) {
	e, call := fixture(t)
	old := e.Command
	e.Command = func(ctx context.Context, name string, in io.Reader, args ...string) (string, error) {
		s, err := old(ctx, name, in, args...)
		if name == "tmux" && args[0] == "list-panes" {
			s += strings.ReplaceAll(s, "%7", "%8")
		}
		return s, err
	}
	if _, err := call("shared", "inbox", "hold", "--sender", "worker", "-", "--id", "shared"); err != nil {
		t.Fatal(err)
	}
	if _, err := call("", "inbox", "consume", "shared"); err != nil {
		t.Fatal(err)
	}
}

func TestAgentSelectionBreadthFirst(t *testing.T) {
	e, call := fixture(t)
	old := e.Command
	e.Command = func(ctx context.Context, name string, in io.Reader, args ...string) (string, error) {
		if name == "ps" && args[0] == "-axo" {
			// Deliberately put the deeper print process first in ps order.
			return "703 702 claude -p task\n702 701 claude\n701 700 bash\n700 1 bash\n", nil
		}
		return old(ctx, name, in, args...)
	}
	s, err := call("", "ls", "--json")
	if err != nil {
		t.Fatal(err)
	}
	p := decode(t, s)["panes"].([]any)[0].(map[string]any)
	if p["agent_pid"] != float64(702) || p["headless"] != false || p["state"] != "idle" {
		t.Fatal(p)
	}
}

func TestScreenActivityAndPlaceholder(t *testing.T) {
	for screen, want := range map[string]string{
		"❯ \x1b[2msuggested next prompt\x1b[0m": "idle",
		"✻ Thinking… (12s)\n❯ ":                 "working",
		"esc to interrupt\n› ":                  "working",
		"❯ typed\nfooter":                       "draft",
		"› 1. Yes\nEnter to confirm":            "prompt",
	} {
		if got := screenState(screen); got != want {
			t.Errorf("%q: %s, want %s", screen, got, want)
		}
	}
}

func TestDispatchOnlyWithoutTmux(t *testing.T) {
	e, call := fixture(t)
	old := e.Command
	e.Command = func(ctx context.Context, name string, in io.Reader, args ...string) (string, error) {
		if name == "tmux" {
			return "", exec.ErrNotFound
		}
		return old(ctx, name, in, args...)
	}
	if _, err := call("", "ls", "--json"); err != nil {
		t.Fatal(err)
	}
}

func TestEverywhereJSONNestsHosts(t *testing.T) {
	e, call := fixture(t)
	t.Setenv("PEERS_HOSTS", "remote")
	e.Transport = inventoryTransport{}
	s, err := call("", "ls", "--everywhere", "--json")
	if err != nil {
		t.Fatal(err)
	}
	hosts := decode(t, s)["hosts"].([]any)
	if len(hosts) != 1 || hosts[0].(map[string]any)["host"] != "remote" {
		t.Fatal(s)
	}
}

type inventoryTransport struct{}

func (inventoryTransport) Execute(_ context.Context, _ string, _ []string, _ io.Reader, out, _ io.Writer) error {
	_, err := io.WriteString(out, `{"host":"remote","panes":[]}`)
	return err
}
