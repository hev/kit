//go:build !windows

package compat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Interop qualification against the Python reference (hev/skills
// peers/bin/peers). The reference stays outside this repository: set
// PEERS_PYTHON_REFERENCE to its bin/peers. Each simulated host gets a scratch
// HOME, its own tmux server and PEERS_SELF; a fake ssh on PATH runs remote
// commands as the target host. Nothing touches the caller's tmux or state.

func TestMain(m *testing.M) {
	if os.Getenv("PEERS_COMPAT_MAIN") == "1" {
		args := os.Args[1:]
		if len(args) > 0 && args[0] == "peers" { // invoked the way hev is: `hev peers ...`
			args = args[1:]
		}
		os.Exit(Main(args, os.Stdin, os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

type cluster struct {
	t         *testing.T
	root      string
	reference string
	python    string
	shared    string // bin dir with the fake ssh and python3
	hosts     map[string]*host
}

type host struct {
	c       *cluster
	name    string
	home    string
	bin     string
	peers   []string // PEERS_HOSTS
	panes   map[string]string
	install string // "go" or "python": what `peers` on this host runs
}

func newCluster(t *testing.T) *cluster {
	t.Helper()
	reference := os.Getenv("PEERS_PYTHON_REFERENCE")
	if reference == "" {
		t.Skip("set PEERS_PYTHON_REFERENCE to the reference bin/peers")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux required")
	}
	python, err := exec.Command("python3", "-c", "import sys;print(sys.executable)").Output()
	if err != nil {
		t.Skip("python3 required")
	}
	root, err := os.MkdirTemp("/tmp", "peers-interop-") // short: tmux socket paths are limited
	if err != nil {
		t.Fatal(err)
	}
	c := &cluster{t: t, root: root, reference: reference, python: strings.TrimSpace(string(python)), hosts: map[string]*host{}}
	c.shared = filepath.Join(root, "shared")
	must(t, os.MkdirAll(c.shared, 0o755))
	must(t, os.Symlink(c.python, filepath.Join(c.shared, "python3")))
	// ssh [-o OPT]... HOST CMD runs CMD as HOST; ssh -G prints no aliases.
	ssh := `#!/bin/sh
while [ $# -gt 0 ]; do
  case "$1" in
    -G) exit 0 ;;
    -o) shift 2 ;;
    --) shift; break ;;
    -*) shift ;;
    *) break ;;
  esac
done
host=$1; shift
dir="$FAKE_SSH_ROOT/$host"
if [ ! -f "$dir/reachable" ]; then echo "ssh: connect to host $host port 22: Connection refused" >&2; exit 255; fi
exec env -i HOME="$dir/home" PATH="$dir/home/.local/bin:$FAKE_SSH_SHARED:/usr/bin:/bin" PEERS_SELF="$host" \
  PEERS_HOSTS="$(cat "$dir/peers_hosts")" TMUX_TMPDIR="$dir/tmux" FAKE_SSH_ROOT="$FAKE_SSH_ROOT" \
  FAKE_SSH_SHARED="$FAKE_SSH_SHARED" LANG=C.UTF-8 /bin/sh -c "$*"
`
	must(t, os.WriteFile(filepath.Join(c.shared, "ssh"), []byte(ssh), 0o755))
	t.Cleanup(func() {
		for _, h := range c.hosts {
			cmd := exec.Command("tmux", "kill-server")
			cmd.Env = h.env("")
			cmd.Run()
		}
		os.RemoveAll(root)
	})
	return c
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func (c *cluster) host(name, install string, peers ...string) *host {
	t := c.t
	dir := filepath.Join(c.root, name)
	h := &host{c: c, name: name, home: filepath.Join(dir, "home"), bin: filepath.Join(dir, "home/.local/bin"), peers: peers,
		panes: map[string]string{}, install: install}
	must(t, os.MkdirAll(h.bin, 0o755))
	must(t, os.MkdirAll(filepath.Join(dir, "tmux"), 0o700))
	must(t, os.WriteFile(filepath.Join(dir, "peers_hosts"), []byte(strings.Join(peers, " ")), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "reachable"), nil, 0o644))
	state := filepath.Join(h.home, ".local/state")
	must(t, os.MkdirAll(filepath.Join(state, "peers-v2"), 0o755))
	// A completed reviewed cutover: regular-file seal plus readiness marker.
	must(t, os.WriteFile(filepath.Join(state, "peers"), []byte("sealed\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(state, "peers-v2/cutover-ready.json"), []byte(`{"version": 2, "legacy_consumers_retired": true}`), 0o644))
	exe, err := os.Executable()
	must(t, err)
	script := fmt.Sprintf("#!/bin/sh\nPEERS_COMPAT_MAIN=1 exec %s \"$@\"\n", shlexQuote(exe))
	if install == "python" {
		script = fmt.Sprintf("#!/bin/sh\nexec python3 %s \"$@\"\n", shlexQuote(c.reference))
	}
	must(t, os.WriteFile(filepath.Join(h.bin, "peers"), []byte(script), 0o755))
	c.hosts[name] = h
	return h
}

func (h *host) unreachable() { os.Remove(filepath.Join(h.c.root, h.name, "reachable")) }

func (h *host) env(pane string) []string {
	env := []string{"HOME=" + h.home, "PATH=" + h.bin + ":" + h.c.shared + ":/usr/bin:/bin", "PEERS_SELF=" + h.name,
		"PEERS_HOSTS=" + strings.Join(h.peers, " "), "TMUX_TMPDIR=" + filepath.Join(h.c.root, h.name, "tmux"),
		"FAKE_SSH_ROOT=" + h.c.root, "FAKE_SSH_SHARED=" + h.c.shared, "LANG=C.UTF-8", "TZ=UTC"}
	if pane != "" {
		env = append(env, "TMUX_PANE="+h.panes[pane])
	}
	return env
}

func (h *host) tmux(args ...string) string {
	cmd := exec.Command("tmux", args...)
	cmd.Env = h.env("")
	out, err := cmd.CombinedOutput()
	if err != nil {
		h.c.t.Fatalf("tmux %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// agent starts a session whose process tree reads as an interactive claude.
func (h *host) agent(name string, inbox bool) {
	h.tmux("-f", "/dev/null", "new-session", "-d", "-s", name, "-c", h.home, "bash -c 'exec -a claude sleep 900'")
	h.panes[name] = h.tmux("display-message", "-p", "-t", "="+name+":", "#{pane_id}")
	if inbox {
		h.tmux("set-option", "-t", "="+name+":", "@peers-inbox", "on")
	}
}

type impl string

const (
	goImpl impl = "go"
	pyImpl impl = "python"
)

// run runs peers as impl on h, acting from pane (empty: no pane).
func (h *host) run(as impl, pane, stdin string, args ...string) (string, string, int) {
	h.c.t.Helper()
	var cmd *exec.Cmd
	env := h.env(pane)
	if as == goImpl {
		exe, _ := os.Executable()
		cmd = exec.Command(exe, args...)
		env = append(env, "PEERS_COMPAT_MAIN=1")
	} else {
		cmd = exec.Command(h.c.python, append([]string{h.c.reference}, args...)...)
	}
	cmd.Env = env
	cmd.Dir = h.home
	cmd.Stdin = strings.NewReader(stdin)
	var out, errout bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errout
	err := cmd.Run()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		h.c.t.Fatal(err)
	}
	return out.String(), errout.String(), code
}

func (h *host) ok(as impl, pane, stdin string, args ...string) string {
	h.c.t.Helper()
	out, errout, code := h.run(as, pane, stdin, args...)
	if code != 0 {
		h.c.t.Fatalf("%s peers %v on %s/%s: exit %d\nstdout: %s\nstderr: %s", as, args, h.name, pane, code, out, errout)
	}
	return out
}

func decodeObj(t *testing.T, s string) *Obj {
	t.Helper()
	o, err := LoadObj([]byte(s))
	if err != nil {
		t.Fatalf("%v: %q", err, s)
	}
	return o
}

func ids(t *testing.T, packet string) []string {
	t.Helper()
	out := []string{}
	for _, m := range decodeObj(t, packet).List("messages") {
		out = append(out, m.(*Obj).Str("id"))
	}
	return out
}

func keysOf(o *Obj) string { return strings.Join(o.Keys(), ",") }

// same asserts both implementations print identical bytes for one command.
func (h *host) same(pane string, normalize func(string) string, args ...string) string {
	h.c.t.Helper()
	pyOut, pyErr, pyCode := h.run(pyImpl, pane, "", args...)
	goOut, goErr, goCode := h.run(goImpl, pane, "", args...)
	if normalize != nil {
		pyOut, goOut = normalize(pyOut), normalize(goOut)
	}
	if pyOut != goOut || pyCode != goCode || pyErr != goErr {
		h.c.t.Fatalf("peers %v differs on %s/%s\npython (%d): %s%s\ngo (%d): %s%s", args, h.name, pane, pyCode, pyOut, pyErr, goCode, goOut, goErr)
	}
	return goOut
}

func lines(path string) []*Obj {
	return inboxRows(path)
}

var agoRE = regexp.MustCompile(`\d+[smhd] ago`)

func normAgo(s string) string { return agoRE.ReplaceAllString(s, "N ago") }

func TestInteropInboxConsumeAck(t *testing.T) {
	c := newCluster(t)
	h := c.host("laptop", "go")
	for _, name := range []string{"alpha", "bravo"} {
		h.agent(name, true)
	}
	inbox := func(s string) string { return filepath.Join(h.home, ".local/state/peers-v2/inbox", s+".jsonl") }

	// Python writes, Go consumes, Python acknowledges.
	out := h.ok(pyImpl, "alpha", "", "send", "bravo", "from python", "--id", "py-1", "--order", "o-1")
	if !strings.HasPrefix(out, "queued for bravo (claude) [id py-1]; held unread in inbox") {
		t.Fatal(out)
	}
	if got := ids(t, h.ok(goImpl, "bravo", "", "inbox", "--peek", "--pending", "--json")); strings.Join(got, ",") != "py-1" {
		t.Fatal(got)
	}
	h.same("bravo", nil, "inbox", "--peek", "--pending", "--json")
	h.same("bravo", nil, "inbox", "--count")
	h.ok(goImpl, "bravo", "", "inbox", "consume", "py-1")
	pending := decodeObj(t, h.same("bravo", nil, "inbox", "--peek", "--pending", "--json"))
	if pending.List("messages")[0].(*Obj).Get("consumed_at") == nil || !Equal(pending.Get("unread"), 0) {
		t.Fatal(Dumps(pending))
	}
	if _, errout, code := h.run(pyImpl, "alpha", "", "inbox", "ack", "py-1", "--session", "bravo"); code == 0 || !strings.Contains(errout, "only the intended current recipient") {
		t.Fatalf("a sender acknowledged another recipient's message: %d %s", code, errout)
	}
	h.ok(pyImpl, "bravo", "", "inbox", "ack", "py-1")
	if got := decodeObj(t, h.same("bravo", nil, "inbox", "--peek", "--json")); !Equal(got.Get("unacknowledged"), 0) {
		t.Fatal(Dumps(got))
	}

	// Go writes, Python consumes (plain read), Go acknowledges.
	h.ok(goImpl, "bravo", "", "send", "alpha", "from go", "--id", "go-1", "--run", "r-1")
	h.same("alpha", nil, "inbox", "--peek", "--json")
	text := h.ok(pyImpl, "alpha", "", "inbox")
	if !strings.Contains(text, "--- go-1 [queued; current]") || !strings.Contains(text, "[peer message from bravo (claude, ~). Reply with: peers send bravo \"...\"]\nfrom go") {
		t.Fatal(text)
	}
	if got := ids(t, h.ok(goImpl, "alpha", "", "inbox", "--pending", "--json")); strings.Join(got, ",") != "go-1" {
		t.Fatalf("consumed-unacknowledged replay: %v", got)
	}
	h.ok(goImpl, "alpha", "", "inbox", "ack", "go-1")
	h.same("alpha", nil, "inbox", "--peek", "--all", "--json")

	// Rows and receipts from either writer have the same fields in the same order.
	py, gorow := lines(inbox("bravo"))[0], lines(inbox("alpha"))[0]
	if keysOf(py) != strings.Replace(keysOf(gorow), "run", "order", 1) {
		t.Fatalf("row field order differs\npython: %s\ngo:     %s", keysOf(py), keysOf(gorow))
	}
	pr, gr := lines(inbox("bravo")+".receipts.jsonl"), lines(inbox("alpha")+".receipts.jsonl")
	if len(pr) != 2 || len(gr) != 2 || keysOf(pr[0]) != keysOf(gr[0]) || keysOf(pr[1]) != keysOf(gr[1]) {
		t.Fatalf("receipts differ\npython: %v\ngo: %v", pr, gr)
	}
	// Retrying a stable ID from the other implementation dedupes, never duplicates.
	h.ok(pyImpl, "bravo", "", "send", "alpha", "from go", "--id", "go-1", "--run", "r-1")
	if n := len(lines(inbox("alpha"))); n != 1 {
		t.Fatalf("stable-ID retry appended: %d rows", n)
	}
	if _, errout, code := h.run(goImpl, "bravo", "", "send", "alpha", "changed", "--id", "go-1"); code != 1 || !strings.Contains(errout, "message ID reused") {
		t.Fatalf("%d %s", code, errout)
	}

	// hold from either side; --pending with --limit; owner.
	h.ok(pyImpl, "alpha", "held one", "inbox", "hold", "--sender", "SendMessage")
	h.ok(goImpl, "alpha", "held two", "inbox", "hold", "--sender", "SendMessage")
	h.same("alpha", nil, "inbox", "--peek", "--pending", "--limit", "1", "--json")
	h.same("alpha", nil, "inbox", "owner")
	h.same("alpha", nil, "inbox", "--peek", "--limit", "65")
	h.same("", nil, "inbox", "--session", "alpha", "--peek", "--current-all", "--json")
	h.same("alpha", normAgo, "inbox", "--peek", "--all")

	// Every listing field matches, apart from when it was taken.
	norm := func(s string) string { return regexp.MustCompile(`"at": [0-9.]+`).ReplaceAllString(s, `"at": T`) }
	h.same("alpha", norm, "ls", "--json")
	h.same("alpha", normAgo, "ls")
	h.same("alpha", normAgo, "ls", "-a")
	h.same("alpha", nil, "role")
	h.same("alpha", nil, "role", "bravo")
}

func TestInteropRoleAndOffline(t *testing.T) {
	c := newCluster(t)
	h := c.host("laptop", "go")
	h.agent("alpha", false)
	h.agent("bravo", true)
	h.ok(pyImpl, "alpha", "", "role", "bravo", "offline")
	if _, errout, code := h.run(goImpl, "alpha", "", "send", "bravo", "hi"); code != 1 || !strings.Contains(errout, "no pane matches 'bravo'") {
		t.Fatalf("%d %s", code, errout)
	}
	h.same("alpha", nil, "role", "bravo")
	h.ok(goImpl, "alpha", "", "role", "bravo", "foh")
	h.same("alpha", normAgo, "ls")
	h.same("alpha", nil, "inbox", "on", "alpha")
	h.same("alpha", nil, "inbox", "off", "alpha")
}

func TestInteropChat(t *testing.T) {
	c := newCluster(t)
	h := c.host("laptop", "go")
	for _, name := range []string{"alpha", "bravo", "charlie", "delta"} {
		h.agent(name, true)
	}
	out := h.ok(pyImpl, "alpha", "", "chat", "new", "bravo", "charlie", "--title", "interop", "-m", "hello from python")
	m := regexp.MustCompile(`chat #([0-9a-f]{6}) with bravo, charlie`).FindStringSubmatch(out)
	if m == nil {
		t.Fatal(out)
	}
	id := m[1]
	thread := filepath.Join(h.home, ".local/state/peers-v2/threads", id+".json")
	h.same("bravo", nil, "chat", "show", id)
	h.same("bravo", normAgo, "chat", "ls")
	// Go says to the chat Python started; Python reads it from its inbox.
	out = h.ok(goImpl, "bravo", "", "chat", "say", id, "reply from go")
	if !strings.Contains(out, "alpha: held unread in inbox") || !strings.Contains(out, "charlie: held unread in inbox") {
		t.Fatal(out)
	}
	got := decodeObj(t, h.ok(pyImpl, "charlie", "", "inbox", "--json"))
	msgs := got.List("messages")
	if len(msgs) != 2 || msgs[1].(*Obj).Str("message") != "reply from go" || msgs[1].(*Obj).Str("thread") != id {
		t.Fatal(Dumps(got))
	}
	if header := msgs[1].(*Obj).Str("header"); !strings.HasPrefix(header, "[peer chat #"+id+" “interop”: from bravo (claude, ~). In this chat: alpha, bravo, charlie.") {
		t.Fatal(header)
	}
	// Python adds a member with a backlog; Go adds another; Go and Python show the same thread.
	h.ok(pyImpl, "charlie", "", "chat", "add", id, "delta")
	h.same("alpha", nil, "chat", "show", id, "-n", "10")
	delta := decodeObj(t, h.ok(goImpl, "delta", "", "inbox", "--json")).List("messages")
	if len(delta) != 1 || !strings.Contains(delta[0].(*Obj).Str("message"), "charlie added you to this chat. The last messages:\n- alpha: hello from python\n- bravo: reply from go") {
		t.Fatal(delta)
	}
	// Go leaves; the /clear hook form (Python) leaves every chat quietly.
	h.ok(goImpl, "delta", "", "chat", "leave", id, "--reason", "is done")
	if out := h.ok(pyImpl, "charlie", "{}", "chat", "leave", "--all", "--hook", "claude", "-q", "--reason", "cleared its session and left the chat"); out != "" {
		t.Fatalf("quiet leave printed %q", out)
	}
	tj, err := readObj(thread)
	must(t, err)
	if keysOf(tj) != "id,title,created,created_by,members" {
		t.Fatal(keysOf(tj))
	}
	if a := strings.Join(active(tj), ","); a != "laptop/alpha,laptop/bravo" {
		t.Fatal(a)
	}
	h.same("alpha", nil, "chat", "show", id)
	h.same("alpha", normAgo, "chat", "ls")
	// A Go-started chat reads the same through Python.
	out = h.ok(goImpl, "alpha", "", "chat", "new", "delta", "-m", "-")
	if !strings.Contains(out, "nothing sent yet") && !strings.Contains(out, "delta:") {
		t.Fatal(out)
	}
	h.same("alpha", normAgo, "chat", "ls")
	h.same("", nil, "chat", "--help")
}

func TestInteropBeaconDelivery(t *testing.T) {
	for _, tc := range []struct{ beacon, sender impl }{{goImpl, pyImpl}, {pyImpl, goImpl}, {goImpl, goImpl}} {
		t.Run(fmt.Sprintf("%s-beacon-%s-sender", tc.beacon, tc.sender), func(t *testing.T) {
			c := newCluster(t)
			// The laptop can reach the mini; the mini cannot reach back.
			laptop := c.host("laptop", string(tc.beacon), "mini")
			mini := c.host("mini", string(tc.sender))
			laptop.unreachable()
			laptop.agent("alpha", true)
			laptop.agent("echo", false)
			mini.agent("worker", true)
			outbox := filepath.Join(mini.home, ".local/state/peers-v2/outbox")

			laptop.ok(tc.beacon, "", "", "beacon")
			if _, err := os.Stat(filepath.Join(mini.home, ".local/state/peers-v2/hosts/laptop.json")); err != nil {
				t.Fatal("beacon left no snapshot on the mini")
			}
			mini.same("worker", normAgo, "ls")
			// A bare name resolves through the snapshot; the reply is queued.
			out := mini.ok(tc.sender, "worker", "", "send", "alpha", "relayed hello", "--id", "relay-1")
			if !strings.Contains(out, "queued for laptop/alpha (claude, idle as of") || !strings.Contains(out, "[inbox ID mini:relay-1]") {
				t.Fatal(out)
			}
			mini.ok(tc.sender, "worker", "", "send", "laptop/alpha", "relayed hello", "--id", "relay-1") // idempotent retry
			if n := len(sortedGlob(filepath.Join(outbox, "*.json"))); n != 1 {
				t.Fatalf("outbox has %d messages", n)
			}
			mini.same("worker", nil, "read", "laptop/alpha", "-n", "5")

			laptop.ok(tc.beacon, "", "", "beacon")
			other := pyImpl
			if tc.beacon == pyImpl {
				other = goImpl
			}
			packet := decodeObj(t, laptop.ok(other, "alpha", "", "inbox", "--json"))
			msgs := packet.List("messages")
			if len(msgs) != 1 || msgs[0].(*Obj).Str("id") != "mini:relay-1" || msgs[0].(*Obj).Str("message") != "relayed hello" {
				t.Fatal(Dumps(packet))
			}
			header := msgs[0].(*Obj).Str("header")
			if !strings.HasPrefix(header, "[peer message from mini/worker (claude, ~), relayed from mini (queued ") {
				t.Fatal(header)
			}
			laptop.ok(other, "alpha", "", "inbox", "ack", "mini:relay-1")
			// The next beacon reports the durable transfer; the mini drops its outbox copy.
			laptop.ok(other, "", "", "beacon")
			if n := len(sortedGlob(filepath.Join(outbox, "*.json"))); n != 0 {
				t.Fatalf("outbox kept %d transferred messages", n)
			}
			transfers := lines(filepath.Join(laptop.home, ".local/state/peers-v2/relay-transfers.jsonl"))
			if len(transfers) != 2 || !Truthy(transfers[1].Get("completed")) || keysOf(transfers[0]) != "id,from_host,via,fingerprint,ts" {
				t.Fatalf("%v", transfers)
			}
			if Dumps(transfers[0].Get("fingerprint")) != Dumps(transfers[1].Get("fingerprint")) {
				t.Fatal("fingerprints differ between fetch and completion")
			}
			// Delivery outcome travels back in the next snapshot.
			mini.same("worker", normAgo, "ls")
			if out := mini.ok(tc.sender, "worker", "", "ls"); !strings.Contains(out, "relay mini/worker → laptop/alpha: held unread in inbox") {
				t.Fatal(out)
			}
			// Laptop's sent log ships to the mini by byte cursor, once.
			sent := filepath.Join(mini.home, ".local/state/peers-v2/hosts/laptop.sent.jsonl")
			before := len(lines(sent))
			laptop.ok(tc.beacon, "", "", "beacon")
			if after := len(lines(sent)); after != before {
				t.Fatalf("sent log reshipped: %d -> %d", before, after)
			}

			// A relayed chat: the mini posts to a chat that includes a laptop member.
			out = mini.ok(tc.sender, "worker", "", "chat", "new", "laptop/alpha", "--title", "relay", "-m", "chat across")
			if !strings.Contains(out, "alpha: queued; relayed on its next check-in") {
				t.Fatal(out)
			}
			laptop.ok(tc.beacon, "", "", "beacon")
			got := decodeObj(t, laptop.ok(other, "alpha", "", "inbox", "--json")).List("messages")
			if len(got) != 1 || got[0].(*Obj).Str("message") != "chat across" || !strings.Contains(got[0].(*Obj).Str("header"), "relayed from mini, queued") {
				t.Fatal(got)
			}
			laptop.same("alpha", normAgo, "chat", "ls")
			// A non-inbox recipient gets a bracketed paste, logged as submitted.
			mini.ok(tc.sender, "worker", "", "send", "laptop/echo", "pasted relay")
			laptop.ok(tc.beacon, "", "", "beacon")
			screen := laptop.tmux("capture-pane", "-p", "-t", laptop.panes["echo"])
			if !strings.Contains(screen, "pasted relay") {
				t.Fatal(screen)
			}
			if !strings.Contains(mustRead(t, filepath.Join(laptop.home, ".local/state/peers-v2/relayed.jsonl")), `"status": "submitted; not acknowledged"`) {
				t.Fatal("relay log lacks the submitted outcome")
			}
		})
	}
}

func mustRead(t *testing.T, path string) string {
	b, err := os.ReadFile(path)
	must(t, err)
	return string(b)
}

func TestInteropRemoteSSH(t *testing.T) {
	c := newCluster(t)
	laptop := c.host("laptop", "go", "mini")
	mini := c.host("mini", "python")
	laptop.agent("alpha", true)
	mini.agent("worker", true)
	// Go reaches a Python install over ssh, and Python ships itself to it.
	for _, as := range []impl{goImpl, pyImpl} {
		out := laptop.ok(as, "alpha", "", "send", "mini/worker", "over ssh from "+string(as))
		if !strings.HasPrefix(out, "queued for mini/worker (claude) [id ") {
			t.Fatal(out)
		}
	}
	packet := decodeObj(t, mini.ok(goImpl, "worker", "", "inbox", "--json"))
	headers := packet.List("messages")
	if len(headers) != 2 {
		t.Fatal(Dumps(packet))
	}
	for _, m := range headers {
		// The receiving side words every ssh-delivered message as cross-machine.
		if h := m.(*Obj).Str("header"); h != `[peer message from laptop/alpha (claude, ~), on another machine; this one can't ssh to laptop, so a reply is queued and delivered when it next checks in, within a minute while it's awake. Reply with: peers send laptop/alpha "..."]` {
			t.Fatal(h)
		}
	}
	laptop.same("alpha", func(s string) string {
		return regexp.MustCompile(`"at": [0-9.]+`).ReplaceAllString(s, `"at": T`)
	}, "ls", "--json", "--everywhere")
	laptop.same("alpha", normAgo, "ls", "--everywhere")
	laptop.same("alpha", nil, "inbox", "--session", "mini/worker", "--peek", "--json")
	laptop.same("alpha", nil, "read", "mini/worker", "-n", "3")
	// A chat spanning both hosts goes through _chat; the transcript merges _chatlog.
	out := laptop.ok(goImpl, "alpha", "", "chat", "new", "mini/worker", "-m", "cross-host")
	id := regexp.MustCompile(`chat #([0-9a-f]{6})`).FindStringSubmatch(out)[1]
	if !strings.Contains(out, "mini/worker: held unread in inbox") {
		t.Fatal(out)
	}
	laptop.same("alpha", nil, "chat", "show", id)
	if _, errout, code := laptop.run(goImpl, "alpha", "", "inbox", "--session", "mini/worker", "ack", "x"); code != 1 || !strings.Contains(errout, "inspection-only") {
		t.Fatal(errout)
	}
	mini.unreachable()
	_, goErr, goCode := laptop.run(goImpl, "alpha", "", "send", "mini/worker", "down")
	_, pyErr, pyCode := laptop.run(pyImpl, "alpha", "", "send", "mini/worker", "down")
	if goCode != pyCode || goErr != pyErr {
		t.Fatalf("unreachable host differs\npython %d %s\ngo %d %s", pyCode, pyErr, goCode, goErr)
	}
}

func TestInteropInboxHook(t *testing.T) {
	c := newCluster(t)
	h := c.host("laptop", "go")
	h.agent("alpha", true)
	h.agent("bravo", true)
	h.ok(pyImpl, "bravo", "", "send", "alpha", "hook me", "--id", "hook-1")
	hook := filepath.Join(filepath.Dir(c.reference), "peers-inbox-hook")
	event := `{"hook_event_name": "UserPromptSubmit", "prompt": "hi"}`
	// Both hooks consume through the same peers; compare their output with paths normalized.
	exe, _ := os.Executable()
	goEnv := append(h.env("alpha"), "PEERS_INBOX_BIN="+filepath.Join(h.bin, "peers"))
	run := func(argv []string, env []string) string {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Env, cmd.Stdin = env, strings.NewReader(event)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%v: %v", argv, err)
		}
		return string(out)
	}
	// The first hook consumes; the second replays the consumed, unacknowledged item.
	consumed := regexp.MustCompile(`\\"consumed_at\\": (null|[0-9.]+)`)
	pyOut := consumed.ReplaceAllString(run([]string{c.python, hook}, goEnv), `\"consumed_at\": T`)
	goOut := consumed.ReplaceAllString(run([]string{exe, "inbox-hook"}, append(goEnv, "PEERS_COMPAT_MAIN=1")), `\"consumed_at\": T`)
	if n := len(lines(filepath.Join(h.home, ".local/state/peers-v2/inbox/alpha.jsonl.receipts.jsonl"))); n != 1 {
		t.Fatalf("hooks wrote %d consumption receipts", n)
	}
	if pyOut != goOut {
		t.Fatalf("hook output differs\npython: %s\ngo:     %s", pyOut, goOut)
	}
	var doc struct {
		Out struct {
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	must(t, json.Unmarshal([]byte(goOut), &doc))
	if !strings.Contains(doc.Out.Context, "inbox ack ID with the full ID below") || !strings.Contains(doc.Out.Context, `"id": "hook-1"`) {
		t.Fatal(goOut)
	}
	// No receipt owner: both pass the prompt through with the same system message.
	other := h.env("")
	other = append(other, "PEERS_INBOX_BIN="+filepath.Join(h.bin, "peers"))
	if a, b := run([]string{c.python, hook}, other), run([]string{exe, "inbox-hook"}, append(other, "PEERS_COMPAT_MAIN=1")); a != b || !strings.Contains(a, "FOH inbox held") {
		t.Fatalf("%s\n%s", a, b)
	}
	// Other events produce nothing.
	cmd := exec.Command(exe, "inbox-hook")
	cmd.Env, cmd.Stdin = append(goEnv, "PEERS_COMPAT_MAIN=1"), strings.NewReader(`{"hook_event_name": "Stop"}`)
	if out, err := cmd.Output(); err != nil || len(out) != 0 {
		t.Fatalf("%v %q", err, out)
	}
}

func TestInteropWaitPasteAndErrors(t *testing.T) {
	c := newCluster(t)
	h := c.host("laptop", "go")
	h.agent("alpha", false)
	h.agent("bravo", false)
	h.same("alpha", nil, "wait", "bravo", "--timeout", "5")
	// The reference dies with a Refused traceback here; this port reports it as peers: MESSAGE.
	for _, as := range []impl{pyImpl, goImpl} {
		if _, errout, code := h.run(as, "alpha", "", "wait", "nobody", "--timeout", "1"); code != 1 || !strings.Contains(errout, "no pane matches 'nobody' (peers ls to see who's here)") {
			t.Fatalf("%s: %d %s", as, code, errout)
		}
	}
	norm := func(s string) string { return regexp.MustCompile(`\[id [0-9a-f]{32}\]`).ReplaceAllString(s, "[id X]") }
	h.same("alpha", norm, "send", "bravo", "pasted directly")
	if screen := h.tmux("capture-pane", "-p", "-t", h.panes["bravo"]); strings.Count(screen, "pasted directly") != 2 {
		t.Fatalf("expected one paste per implementation:\n%s", screen)
	}
	for _, args := range [][]string{
		{"send", "alpha", "to myself"}, {"send", "bravo", "   "}, {"send", "bravo"}, {"send", "bravo", "x", "--id", "bad id"},
		{"bogus"}, {"chef-notify"}, {"chat", "frob", "abc"}, {"chat", "show", "nope"}, {"read"}, {"role", "a", "b", "c"},
		{"inbox", "--session"}, {"inbox", "--session", "bravo", "ack"}, {"inbox", "consume", "missing"},
		{"managed", "register", "x"}, {"send", "--from"}, {"send", "elsewhere/x", "y"},
	} {
		h.same("alpha", nil, args...)
	}
	// Without a pane the sender comes from process ancestry (or is refused), identically.
	h.same("", norm, "send", "bravo", "who am I")
	// Help is the reference text plus the two folded-in helpers.
	py, goHelp := h.ok(pyImpl, "", "", "--help"), h.ok(goImpl, "", "", "--help")
	if strings.Replace(goHelp, "  peers inbox-hook                 UserPromptSubmit hook: inject the bound inbox (peers-inbox-hook)\n  peers runtime codex|claude|pi [ARGS...]\n                                   launch a harness owning a receipt capability (peers-runtime)\n", "", 1) != py {
		t.Fatalf("help drifted\npython: %s\ngo: %s", py, goHelp)
	}
}

func TestInteropManagedRegister(t *testing.T) {
	c := newCluster(t)
	h := c.host("laptop", "go")
	h.agent("alpha", false)
	root := filepath.Join(h.home, "runner")
	must(t, os.MkdirAll(root, 0o755))
	runner := filepath.Join(root, "runner.py")
	must(t, os.WriteFile(runner, []byte("import time\nwhile True: time.sleep(1)\n"), 0o644))
	proc := exec.Command(c.python, runner, root)
	must(t, proc.Start())
	t.Cleanup(func() { proc.Process.Kill(); proc.Wait() })
	must(t, os.WriteFile(filepath.Join(root, "state.json"), []byte(fmt.Sprintf(`{"pid": %d, "phase": "running", "session": "s-1", "turn": 2}`, proc.Process.Pid)), 0o644))
	time.Sleep(200 * time.Millisecond)
	rec := decodeObj(t, h.ok(goImpl, "alpha", "", "managed", "register", "runner-1", "--root", root))
	if keysOf(rec) != "version,name,root,pid,birth,command,uid,incarnation,registered_at" {
		t.Fatal(keysOf(rec))
	}
	// Re-registering the same lifetime is idempotent in either implementation.
	again := h.same("alpha", nil, "managed", "register", "runner-1", "--root", root)
	if decodeObj(t, again).Str("incarnation") != rec.Str("incarnation") {
		t.Fatal(again)
	}
	h.same("alpha", nil, "managed", "register", "runner-2", "--root", root)
	h.same("alpha", nil, "read", "runner-1")
	h.same("alpha", normAgo, "ls")
	out := h.ok(pyImpl, "alpha", "", "send", "runner-1", "to the runner")
	if !strings.HasPrefix(out, "queued for runner-1 (claude) [id ") {
		t.Fatal(out)
	}
	h.same("alpha", nil, "inbox", "--session", "runner-1", "--peek", "--json")
	h.same("alpha", nil, "inbox", "off", "runner-1")
}

func TestInteropRuntimeBinding(t *testing.T) {
	c := newCluster(t)
	h := c.host("laptop", "go")
	h.agent("alpha", true)
	h.agent("bravo", false)
	h.ok(goImpl, "bravo", "", "send", "alpha", "for the runtime", "--id", "rt-1")
	// The fake harness reports its hook wiring, then consumes and replays with
	// the delegated token through the other implementation.
	harness := filepath.Join(h.bin, "fake-harness")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$@" > %[1]s/args
printf '%%s' "$PEERS_INBOX_TOKEN" > %[1]s/token
%[2]s %[3]s inbox --json > %[1]s/consumed
PEERS_COMPAT_MAIN=1 %[4]s inbox --pending --json > %[1]s/pending
`, h.home, shlexQuote(c.python), shlexQuote(c.reference), shlexQuote(mustExe(t)))
	must(t, os.WriteFile(harness, []byte(script), 0o755))
	exe := mustExe(t)
	cmd := exec.Command(exe, "runtime", "claude", "--model", "x")
	cmd.Env = append(h.env("alpha"), "PEERS_COMPAT_MAIN=1", "PEERS_RUNTIME_LAUNCHER="+harness)
	cmd.Dir = h.home
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	args := strings.Split(strings.TrimSpace(mustRead(t, filepath.Join(h.home, "args"))), "\n")
	if len(args) != 4 || args[0] != "--settings" || !strings.Contains(args[1], `"UserPromptSubmit"`) || !strings.Contains(args[1], "inbox-hook") || args[2] != "--model" {
		t.Fatal(args)
	}
	if !tokenRE.MatchString(mustRead(t, filepath.Join(h.home, "token"))) {
		t.Fatal("no delegated token")
	}
	if got := ids(t, mustRead(t, filepath.Join(h.home, "consumed"))); strings.Join(got, ",") != "rt-1" {
		t.Fatal(got)
	}
	pending := decodeObj(t, mustRead(t, filepath.Join(h.home, "pending")))
	if m := pending.List("messages"); len(m) != 1 || m[0].(*Obj).Get("consumed_at") == nil {
		t.Fatal(Dumps(pending))
	}
	// A token cannot outlive its launcher.
	token := mustRead(t, filepath.Join(h.home, "token"))
	for _, as := range []impl{pyImpl, goImpl} {
		cmd := exec.Command(exe, "inbox", "ack", "rt-1")
		if as == pyImpl {
			cmd = exec.Command(c.python, c.reference, "inbox", "ack", "rt-1")
		}
		cmd.Env = append(h.env(""), "PEERS_COMPAT_MAIN=1", "PEERS_INBOX_TOKEN="+token)
		if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "peers: can't tell which session you are") {
			t.Fatalf("%s accepted a dead launcher's token: %s", as, out)
		}
	}
}

func mustExe(t *testing.T) string {
	exe, err := os.Executable()
	must(t, err)
	return exe
}
