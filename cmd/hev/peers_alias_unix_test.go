//go:build unix

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPeersHelperAliasesNeverRunBackend runs a compiled hev through real
// peers, peers-inbox-hook and peers-runtime symlinks with HEV_PEERS_BACKEND
// inherited. The helpers and every call they make back into hev (the runtime's
// bind-runtime, the hook's inbox read and the ACK it instructs) must stay in
// this implementation; the backend only records that it ran.
func TestPeersHelperAliasesNeverRunBackend(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		if os.Getenv("PEERS_INTEROP_REQUIRED") == "1" {
			t.Fatal("tmux is required")
		}
		t.Skip("tmux not installed")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	home := filepath.Join(root, "home")
	state := filepath.Join(home, ".local/state")
	for _, dir := range []string{bin, filepath.Join(root, "tmux"), filepath.Join(state, "peers-v2")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(root, "tmux"), 0o700); err != nil {
		t.Fatal(err)
	}
	// A completed reviewed cutover: regular-file seal plus readiness marker.
	writeFile(t, filepath.Join(state, "peers"), "sealed\n", 0o644)
	writeFile(t, filepath.Join(state, "peers-v2/cutover-ready.json"), `{"version": 2, "legacy_consumers_retired": true}`, 0o644)
	hev := filepath.Join(bin, "hev")
	if out, err := exec.Command("go", "build", "-o", hev, ".").CombinedOutput(); err != nil {
		t.Fatalf("build hev: %v\n%s", err, out)
	}
	for _, name := range []string{"peers", "peers-inbox-hook", "peers-runtime"} {
		if err := os.Symlink(hev, filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	marker := filepath.Join(root, "backend-ran")
	backend := filepath.Join(root, "backend")
	writeFile(t, backend, "#!/bin/sh\nprintf '%s\\n' \"$*\" >> "+marker+"\nexit 97\n", 0o755)
	base := []string{"HOME=" + home, "PATH=" + bin + ":/usr/bin:/bin", "TMUX_TMPDIR=" + filepath.Join(root, "tmux"),
		"PEERS_SELF=test", "PEERS_HOSTS=", "LANG=C.UTF-8", "HEV_PEERS_BACKEND=" + backend}
	tmux := func(args ...string) string {
		cmd := exec.Command("tmux", args...)
		cmd.Env = base
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	t.Cleanup(func() {
		cmd := exec.Command("tmux", "kill-server")
		cmd.Env = base
		_ = cmd.Run()
	})
	panes := map[string]string{}
	for _, name := range []string{"alpha", "bravo"} {
		tmux("-f", "/dev/null", "new-session", "-d", "-s", name, "-c", home, "bash -c 'exec -a claude sleep 900'")
		panes[name] = tmux("display-message", "-p", "-t", "="+name+":", "#{pane_id}")
	}
	tmux("set-option", "-t", "=alpha:", "@peers-inbox", "on")
	run := func(pane, stdin string, extra []string, argv ...string) string {
		t.Helper()
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Env = append(append(append([]string{}, base...), "TMUX_PANE="+panes[pane]), extra...)
		cmd.Dir = home
		cmd.Stdin = strings.NewReader(stdin)
		var out, errout bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errout
		if err := cmd.Run(); err != nil {
			t.Fatalf("%v: %v\nstdout: %s\nstderr: %s", argv, err, out.String(), errout.String())
		}
		return out.String()
	}
	peers := filepath.Join(bin, "peers")
	run("bravo", "", nil, peers, "send", "alpha", "hook item", "--id", "hook-1")
	run("bravo", "", nil, peers, "send", "alpha", "runtime item", "--id", "rt-1")

	// peers-inbox-hook re-enters hev for the inbox read and names the pinned
	// entry in its ACK instruction.
	event := `{"hook_event_name": "UserPromptSubmit", "prompt": "hi"}`
	hookOut := run("alpha", event, nil, filepath.Join(bin, "peers-inbox-hook"))
	ack := hev + " peers-native inbox ack ID"
	if !strings.Contains(hookOut, "additionalContext") || !strings.Contains(hookOut, "hook-1") || !strings.Contains(hookOut, ack) {
		t.Fatalf("hook output: %s", hookOut)
	}
	run("alpha", "", nil, hev, "peers-native", "inbox", "ack", "hook-1")

	// peers-runtime binds through hev, then the harness runs the hook it was
	// given with the delegated token.
	harness := filepath.Join(bin, "fake-harness")
	writeFile(t, harness, `#!/bin/sh
printf '%s\n' "$@" > "$HOME/args"
cmd=$(printf '%s' "$2" | sed -n 's/.*"command": "\([^"]*\)".*/\1/p')
printf '%s' "$cmd" > "$HOME/hook-command"
printf '%s' '`+event+`' | $cmd > "$HOME/hook-out"
`, 0o755)
	run("alpha", "", []string{"PEERS_RUNTIME_LAUNCHER=" + harness}, filepath.Join(bin, "peers-runtime"), "claude", "--model", "x")
	if got := readFile(t, filepath.Join(home, "hook-command")); got != hev+" peers-native inbox-hook" {
		t.Fatalf("runtime hook command %q", got)
	}
	if got := readFile(t, filepath.Join(home, "hook-out")); !strings.Contains(got, "rt-1") || strings.Contains(got, "hook-1") {
		t.Fatalf("runtime hook output: %s", got)
	}

	// hev peers inbox-hook and runtime exist only here, so they never forward either.
	run("alpha", event, nil, hev, "peers", "inbox-hook")
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("backend ran: %s", readFile(t, marker))
	}
	// Control: an unpinned `hev peers` call still honours the backend.
	cmd := exec.Command(hev, "peers", "ls")
	cmd.Env = base
	if err := cmd.Run(); err == nil {
		t.Fatal("backend control did not fail with the backend's exit code")
	}
	if got := readFile(t, marker); got != "ls\n" {
		t.Fatalf("backend control saw %q", got)
	}
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
