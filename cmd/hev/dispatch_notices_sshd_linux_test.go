//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestMemberNoticesOverForcedCommandSSH runs the teammate path against a real
// sshd and real tmux servers: invite, join, accept; the teammate starts a run
// with reply set through the forced-command key; the run really executes and
// ends on the host; the laptop pulls its notice into a registered peers inbox
// exactly once, and a second pull delivers nothing.
func TestMemberNoticesOverForcedCommandSSH(t *testing.T) {
	if testing.Short() {
		t.Skip("starts sshd")
	}
	sshd, err := exec.LookPath("sshd")
	if err != nil {
		if sshd, err = exec.LookPath("/usr/sbin/sshd"); err != nil {
			t.Skip("sshd unavailable")
		}
	}
	if _, err = exec.LookPath("tmux"); err != nil {
		t.Skip("tmux unavailable")
	}
	me, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	// tmux socket paths must stay short, so avoid the long test temp root.
	base, err := os.MkdirTemp("", "hev-mn-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	dir := func(parts ...string) string {
		p := filepath.Join(append([]string{base}, parts...)...)
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
		return p
	}
	hev := filepath.Join(dir("bin"), "hev")
	if out, err := exec.Command("go", "build", "-o", hev, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	ownerHome, teamHome := dir("owner"), dir("team")
	ownerTmux, teamTmux := dir("otmux"), dir("ttmux")
	// An inherited $TMUX names the caller's server and overrides TMUX_TMPDIR,
	// so every tmux call here drops it.
	tmuxEnv := func(d string) []string {
		var env []string
		for _, kv := range os.Environ() {
			if !strings.HasPrefix(kv, "TMUX=") && !strings.HasPrefix(kv, "TMUX_PANE=") && !strings.HasPrefix(kv, "TMUX_TMPDIR=") {
				env = append(env, kv)
			}
		}
		return append(env, "TMUX_TMPDIR="+d)
	}
	t.Cleanup(func() {
		for _, d := range []string{ownerTmux, teamTmux} {
			c := exec.Command("tmux", "kill-server")
			c.Env = tmuxEnv(d)
			_ = c.Run()
		}
	})

	// Host: a fake harness that finishes, a push recorder, and a repository.
	hostBin := dir("hostbin")
	pushed := filepath.Join(base, "pushed")
	for name, body := range map[string]string{
		"codex":  "#!/bin/sh\necho finished\nexit 0\n",
		"gh":     "#!/bin/sh\nexit 1\n",
		"notify": "#!/bin/sh\necho \"$@\" >> " + pushed + "\n",
	} {
		if err := os.WriteFile(filepath.Join(hostBin, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	workspace := dir("owner", "workspace")
	git := func(args ...string) {
		c := exec.Command("git", args...)
		c.Env = append(os.Environ(), "HOME="+ownerHome, "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.test", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.test")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	bare := filepath.Join(base, "widget.git")
	git("init", "-q", "--bare", "-b", "main", bare)
	repo := filepath.Join(workspace, "widget")
	git("init", "-q", "-b", "main", repo)
	git("-C", repo, "commit", "-q", "--allow-empty", "-m", "init")
	git("-C", repo, "remote", "add", "origin", bare)
	git("-C", repo, "push", "-q", "origin", "main")
	hostConfig := filepath.Join(dir("owner", ".config", "hev"), "dispatch.toml")
	policy := "cap = 2\nworkspace = \"" + workspace + "\"\nnotify = [\"" + filepath.Join(hostBin, "notify") + "\", \"{reply}\", \"dispatch {name}: {state} {summary}\"]\n"
	if err := os.WriteFile(hostConfig, []byte(policy), 0600); err != nil {
		t.Fatal(err)
	}

	keygen := func(path string) {
		if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", path).CombinedOutput(); err != nil {
			t.Fatalf("ssh-keygen: %v: %s", err, out)
		}
	}
	hostKey := filepath.Join(dir("sshd"), "host_key")
	keygen(hostKey)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	l.Close()
	sshdConfig := filepath.Join(base, "sshd", "sshd_config")
	conf := strings.Join([]string{
		"Port " + port,
		"ListenAddress 127.0.0.1",
		"HostKey " + hostKey,
		"PidFile none",
		"AuthorizedKeysFile " + filepath.Join(ownerHome, ".ssh", "authorized_keys"),
		"StrictModes no",
		"UsePAM no",
		"PasswordAuthentication no",
		"KbdInteractiveAuthentication no",
		"PubkeyAuthentication yes",
		"AllowTcpForwarding no",
		// The forced command sees the owner sandbox and a private tmux server.
		"SetEnv HOME=" + ownerHome + " PATH=" + hostBin + ":/usr/local/bin:/usr/bin:/bin TMUX_TMPDIR=" + ownerTmux + " PEERS_SELF=build",
		"LogLevel ERROR",
	}, "\n") + "\n"
	if err := os.WriteFile(sshdConfig, []byte(conf), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	daemon := exec.CommandContext(ctx, sshd, "-D", "-e", "-f", sshdConfig)
	// A file, not a buffer: sshd writes it while failures read it.
	logFile, err := os.Create(filepath.Join(base, "sshd", "log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	daemon.Stdout, daemon.Stderr = logFile, logFile
	sshdLog := func() string { b, _ := os.ReadFile(logFile.Name()); return string(b) }
	if err := daemon.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = daemon.Wait() })
	for deadline := time.Now().Add(10 * time.Second); ; {
		if c, err := net.Dial("tcp", "127.0.0.1:"+port); err == nil {
			c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("sshd did not start: %s", sshdLog())
		}
		time.Sleep(50 * time.Millisecond)
	}

	teamBin := dir("teambin")
	wrapper := "#!/bin/sh\nexec /usr/bin/ssh -F \"$HOME/.ssh/config\" -p " + port + " -o HostKeyAlias=127.0.0.1 \"$@\"\n"
	if err := os.WriteFile(filepath.Join(teamBin, "ssh"), []byte(wrapper), 0700); err != nil {
		t.Fatal(err)
	}
	run := func(home string, args ...string) (string, error) {
		t.Helper()
		c := exec.Command(hev, args...)
		// tmux -F prints tabs as "_" in the C locale, which hides panes from peers.
		env := []string{"HOME=" + home, "USER=" + me.Username, "LANG=C.UTF-8", "HEV_PEERS_BACKEND="}
		if home == teamHome {
			env = append(env, "PATH="+teamBin+":"+os.Getenv("PATH"), "TMUX_TMPDIR="+teamTmux, "PEERS_SELF=laptop")
		} else {
			env = append(env, "PATH="+hostBin+":"+os.Getenv("PATH"), "TMUX_TMPDIR="+ownerTmux, "PEERS_SELF=build")
		}
		c.Env = env
		var out bytes.Buffer
		c.Stdout, c.Stderr = &out, &out
		err := c.Run()
		return out.String(), err
	}
	must := func(home string, args ...string) string {
		t.Helper()
		out, err := run(home, args...)
		if err != nil {
			t.Fatalf("hev %v: %v\n%s\nsshd: %s", args, err, out, sshdLog())
		}
		return out
	}

	code := strings.TrimSpace(must(ownerHome, "nodes", "invite", "alex", "--long", "--address", "127.0.0.1", "--user", me.Username, "--host-key", hostKey+".pub"))
	request := strings.TrimSpace(strings.Split(must(teamHome, "nodes", "join", code), "\n")[0])
	must(ownerHome, "nodes", "accept", request)
	authorized, _ := os.ReadFile(filepath.Join(ownerHome, ".ssh", "authorized_keys"))
	if !strings.HasPrefix(string(authorized), "restrict,command=") || !strings.Contains(string(authorized), "dispatch rpc --node") {
		t.Fatalf("not a forced-command key: %s", authorized)
	}

	// The laptop's reply session: an agent session (peers lists only agents
	// and inboxes) with a registered inbox.
	// argv0 names the harness; sleep may be a multicall binary, so the
	// renamed process is a shell that waits.
	agent := filepath.Join(teamBin, "agent")
	if err := os.WriteFile(agent, []byte("#!/bin/bash\nexec -a claude /bin/sh -c 'sleep 600; :'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	tmux := exec.Command("tmux", "new-session", "-d", "-s", "laptop-session", agent)
	tmux.Env = tmuxEnv(teamTmux)
	if out, err := tmux.CombinedOutput(); err != nil {
		t.Fatalf("tmux: %v: %s", err, out)
	}
	must(teamHome, "peers", "inbox", "on", "laptop-session")
	inbox := func() []map[string]any {
		t.Helper()
		var rows []map[string]any
		out := must(teamHome, "peers", "inbox", "--session", "laptop-session", "--peek", "--all", "--json")
		if err := json.Unmarshal([]byte(out), &rows); err != nil {
			var wrapped struct {
				Messages []map[string]any `json:"messages"`
			}
			if err2 := json.Unmarshal([]byte(out), &wrapped); err2 != nil {
				t.Fatalf("inbox json: %v: %s", err, out)
			}
			rows = wrapped.Messages
		}
		return rows
	}
	if got := inbox(); len(got) != 0 {
		t.Fatalf("inbox not empty: %v", got)
	}

	// Nothing to pull yet.
	if out := must(teamHome, "dispatch", "notices", "pull"); strings.Contains(out, "delivered") {
		t.Fatalf("pulled before any run: %s", out)
	}
	order := filepath.Join(base, "noticed.md")
	if err := os.WriteFile(order, []byte("---\nname: noticed\nrepo: widget\nbase: main\nreply: laptop-session\n---\nTask\n## Acceptance\n- Done\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var alias string
	var joined struct {
		Joined []struct{ Name string } `json:"joined"`
	}
	if err := json.Unmarshal([]byte(must(teamHome, "nodes", "ls", "--json")), &joined); err != nil || len(joined.Joined) != 1 {
		t.Fatalf("joined: %+v %v", joined, err)
	}
	alias = joined.Joined[0].Name
	if out := must(teamHome, "dispatch", order, "--host", alias); !strings.Contains(out, "started noticed") {
		t.Fatalf("dispatch: %s", out)
	}
	result := filepath.Join(ownerHome, "dispatch", "noticed", "result.json")
	for deadline := time.Now().Add(60 * time.Second); ; {
		var r struct {
			End      time.Time `json:"end"`
			ExitCode int       `json:"exit_code"`
		}
		if b, err := os.ReadFile(result); err == nil && json.Unmarshal(b, &r) == nil && !r.End.IsZero() {
			if out := must(teamHome, "dispatch", "ls", "--all", "--host", alias); !strings.Contains(out, "noticed") {
				t.Fatalf("ls: %s", out)
			}
			break
		}
		if time.Now().After(deadline) {
			log, _ := os.ReadFile(filepath.Join(ownerHome, "dispatch", "noticed", "log"))
			t.Fatalf("run did not end: %s", log)
		}
		time.Sleep(200 * time.Millisecond)
	}
	// The supervisor queues before it returns; give it a moment to exit.
	for deadline := time.Now().Add(10 * time.Second); ; {
		if entries, _ := filepath.Glob(filepath.Join(ownerHome, "dispatch", ".member-notices", "*", "*.json")); len(entries) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no member notice queued on the host")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if b, err := os.ReadFile(pushed); err == nil {
		t.Fatalf("host pushed a {reply} notice for a teammate's run: %s", b)
	}

	first := must(teamHome, "dispatch", "notices", "pull")
	if !strings.Contains(first, alias+": delivered 1 notices") {
		t.Fatalf("first pull: %s", first)
	}
	got := inbox()
	if len(got) != 1 || !strings.Contains(got[0]["message"].(string), "dispatch noticed: done exit=0; sender=alex") || got[0]["from"] != alias+"/d-noticed" {
		t.Fatalf("inbox after first pull: %v", got)
	}
	if out := must(teamHome, "dispatch", "notices", "pull"); strings.Contains(out, "delivered") {
		t.Fatalf("second pull delivered again: %s", out)
	}
	if got = inbox(); len(got) != 1 {
		t.Fatalf("inbox after second pull: %v", got)
	}
	t.Logf("first pull: %s; inbox: %v", strings.TrimSpace(first), got[0]["message"])
}
