package peers

import (
	"context"
	"io"
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
	for s, want := range map[string]string{"❯ ": "idle", "› human draft": "draft", "Do you trust this folder?": "prompt", "opaque": "?"} {
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
