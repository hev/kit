package peers

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"testing"
)

type recordingTransport struct {
	host    string
	args    []string
	body    string
	receipt string
}

func (r *recordingTransport) Execute(_ context.Context, h string, args []string, in io.Reader, out, errout io.Writer) error {
	r.host = h
	r.args = args
	b, _ := io.ReadAll(in)
	r.body = string(b)
	_, err := io.WriteString(out, r.receipt)
	return err
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
		if got := screenState("claude", s); got != want {
			t.Fatalf("%q: %s", s, got)
		}
	}
}

func TestInboxRegistrationOnUnregisteredShell(t *testing.T) {
	e, call := fixture(t)
	old := e.Command
	e.Command = func(ctx context.Context, name string, in io.Reader, args ...string) (string, error) {
		if name == "tmux" && args[0] == "list-panes" {
			return "desk\x1f%7\x1f700\x1fbash\x1f/work\x1f\x1f\n", nil
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
		if got := screenState("claude", screen); got != want {
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

func TestInboxOnlyStableDelivery(t *testing.T) {
	e, call := fixture(t)
	for i := 0; i < 2; i++ {
		if _, err := call("", "send", "desk", "durable body", "--from", "local/o-release", "--id", "event-1", "--inbox-only"); err != nil {
			t.Fatal(err)
		}
	}
	ms, err := rows(e.path("desk"))
	if err != nil || len(ms) != 1 {
		t.Fatal(ms, err)
	}
	old := e.Command
	e.Command = func(ctx context.Context, name string, in io.Reader, args ...string) (string, error) {
		result, err := old(ctx, name, in, args...)
		if name == "tmux" && args[0] == "list-panes" {
			result = strings.ReplaceAll(result, "\ton\n", "\t\n")
		}
		return result, err
	}
	if _, err := call("", "send", "desk", "no paste", "--from", "local/o-release", "--id", "event-2", "--inbox-only"); err == nil {
		t.Fatal("durable send allowed terminal paste")
	}
	transport := &recordingTransport{}
	e.Transport = transport
	if _, err := call("", "send", "remote/desk", "body", "--from", "local/o-release", "--id", "event-3", "--inbox-only"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(transport.args, " "), "--inbox-only") {
		t.Fatal(transport.args)
	}
}

func TestAgyScreenStateMatchesCompat(t *testing.T) {
	for _, tt := range agyScreens {
		if got := screenState(tt.agent, tt.screen); got != tt.want {
			t.Errorf("%s: state = %q, want %q", tt.name, got, tt.want)
		}
	}
	// The native engine reads the whole capture: agy's view is its last 20 lines.
	long := strings.Repeat("transcript\n", 40) + agyEditor + agyCancelFooter
	if got := screenState("agy", long); got != "working" {
		t.Errorf("long screen: %q", got)
	}
}

const (
	agyRule         = "────────────────────────"
	agyIdle         = "? for shortcuts          Gemini 4 Neon · medium"
	agyCancelFooter = "esc to cancel · 12s          Gemini 4 Neon · medium"
	agyEditor       = agyRule + "\n>  \n" + agyRule + "\n"
)

// agyScreens: agy's running footer reads "esc to cancel" and is working; a
// real dialog or the feedback survey is a prompt; Claude and Codex keep
// "Esc to cancel" as a dialog. This is a copy of the compat CLI's table: the two must agree.
var agyScreens = []struct{ name, agent, screen, want string }{
	{"empty editor", "agy", agyEditor + agyIdle, "idle"},
	{"typed editor", "agy", agyRule + "\n> unsent text\n" + agyRule + "\n" + agyIdle, "draft"},
	{"numbered draft", "agy", agyRule + "\n> 1. unsent text\n" + agyRule + "\n" + agyIdle, "draft"},
	{"multiline draft", "agy", agyRule + "\n>  \nsecond line\n" + agyRule + "\n" + agyIdle, "draft"},
	{"blockquote above editor", "agy", "> some quote\n" + agyEditor + agyIdle, "idle"},
	{"blockquote alone", "agy", "> some quote", "?"},
	{"esc to interrupt", "agy", "Thinking… (esc to interrupt)\n" + agyEditor + agyIdle, "working"},
	{"finished thought is not a turn", "agy", "○ Thought for 3s\n" + agyEditor + agyIdle, "idle"},
	{"cancel footer under editor", "agy", agyEditor + agyCancelFooter, "working"},
	{"cancel footer with spinner", "agy", "⣯ Running command...\n" + agyEditor + agyCancelFooter, "working"},
	{"cancel footer after thought", "agy", "○ Thought for 3s\n⣽ Reading files...\n" + agyEditor + agyCancelFooter, "working"},
	{"cancel footer, spinner, no editor", "agy", "⣯ Running command...\n" + agyCancelFooter, "working"},
	{"spinner alone", "agy", "⣾ Generating...", "working"},
	{"draft while running", "agy", "⣯ Running command...\n" + agyRule + "\n> next idea\n" + agyRule + "\n" + agyCancelFooter, "draft"},
	{"typed cancel words are a draft", "agy", agyRule + "\n> esc to cancel\n" + agyRule + "\n" + agyIdle, "draft"},
	{"cancel footer alone is a dialog", "agy", "Allow this command?\n  1. Yes\n  2. No\nEsc to cancel", "prompt"},
	{"dialog", "agy", "Do you want to proceed?\n> 1. Yes\n  2. No", "prompt"},
	{"dialog during a turn", "agy", "⣯ Running command...\nDo you want to proceed?\n> 1. Yes\n  2. No\nEsc to cancel", "prompt"},
	{"feedback survey", "agy", "○ Thought for 3s\nHow's the CLI experience so far? Help us improve:\n[1] Good  [2] Fine  [3] Bad  [0] Skip", "prompt"},
	{"survey choices alone", "agy", "[1] Good  [2] Fine  [3] Bad  [0] Skip", "prompt"},
	{"survey above editor", "agy", "How's the CLI experience so far? Help us improve:\n" + agyEditor + agyIdle, "prompt"},
	{"claude dialog", "claude", "Do you want to make this edit?\n❯ 1. Yes\nEsc to cancel", "prompt"},
	{"claude cancel footer", "claude", "Esc to cancel\n❯ ", "prompt"},
	{"codex cancel footer", "codex", "Esc to cancel\n› ", "prompt"},
	{"claude esc to interrupt", "claude", "✻ Thinking… (12s · esc to interrupt)\n❯ ", "working"},
	{"codex esc to interrupt", "codex", "• Working (3s • esc to interrupt)\n› ", "working"},
	{"claude idle", "claude", "❯ ", "idle"},
}
