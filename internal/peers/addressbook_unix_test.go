//go:build !windows

package peers

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

// A send to the owner's session reaches ssh with the address book's
// destination, not the bare label that "Could not resolve hostname".
func TestSSHSendUsesAddressBookDestination(t *testing.T) {
	fakeAddressBook(t)
	bin := t.TempDir()
	argsLog := filepath.Join(bin, "args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + argsLog + "'\ncat >/dev/null\nprintf '{\"message\":\"delivered\"}'\n"
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var out, errout bytes.Buffer
	if err := (SSHTransport{Quiet: true}).Execute(context.Background(), "macbook", []string{"send", "foh-bcc", "hello", "--from", "oven/d-run"}, strings.NewReader(""), &out, &errout); err != nil {
		t.Fatal(err, errout.String())
	}
	b, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(b)), "\n")
	dest := ""
	for i, a := range args {
		if a == "--" && i+1 < len(args) {
			dest = args[i+1]
		}
	}
	if dest != "192.0.2.10" {
		t.Fatalf("ssh destination %q, args %q", dest, args)
	}
}

// An ssh config block with no HostName (User plus ProxyCommand tailscale nc)
// is still an alias: ssh -G prints hostname equal to the label, but the
// block's settings apply only when ssh gets the label as written, so it is
// never rewritten to a cluster address (review finding 5). A catch-all Host *
// block alone does not make a label an alias.
func TestSSHConfigAliasWithoutHostName(t *testing.T) {
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("no ssh")
	}
	cfg := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(cfg, []byte("Host macbook\n  User adam\n  ProxyCommand tailscale nc %h %p\nHost tsbox\n  HostName tsbox.example.ts.net\nHost *\n  ServerAliveInterval 10\n  ControlPath ~/.ssh/cm-%C\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for host, want := range map[string]bool{"macbook": true, "tsbox": true, "nowhere": false} {
		if got := sshConfigMatches(ctx, cfg, host); got != want {
			t.Errorf("sshConfigMatches(%q) = %v, want %v", host, got, want)
		}
	}
	// A label that is only a substring of a default setting (the local
	// user, the default ~/.ssh/id_* identity files) is not an alias
	// (re-review note 3).
	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	labels := []string{"ssh", "id", "nowhere", "macbook"}
	if u, err := user.Current(); err == nil && u.Username != "" && !strings.ContainsAny(u.Username, "@.:\\") {
		labels = append(labels, u.Username)
	}
	for _, host := range labels {
		if sshConfigMatches(ctx, empty, host) {
			t.Errorf("empty config: sshConfigMatches(%q) = true", host)
		}
	}
	fakeAddressBook(t)
	sshConfigAlias = func(ctx context.Context, host string) bool { return sshConfigMatches(ctx, cfg, host) }
	if got := SSHDestination(ctx, "macbook"); got != "macbook" {
		t.Fatalf("alias rewritten to %q", got)
	}
	if got := SSHDestination(ctx, "Adams-MacBook-Air"); got != "192.0.2.10" {
		t.Fatalf("label without a block: %q", got)
	}
}
