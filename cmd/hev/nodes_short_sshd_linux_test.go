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
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestShortInviteOverRealSSHD runs the short-code flow against a real sshd:
// invite opens the enrollment key, join exchanges the code over it, the owner
// sees the pending fingerprint and accepts by name, and dispatch then works
// through the forced-command key. A second invitation locks after wrong codes.
func TestShortInviteOverRealSSHD(t *testing.T) {
	if testing.Short() {
		t.Skip("starts sshd")
	}
	sshd, err := exec.LookPath("sshd")
	if err != nil {
		if sshd, err = exec.LookPath("/usr/sbin/sshd"); err != nil {
			t.Skip("sshd unavailable")
		}
	}
	me, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
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
	ownerHome, teamHome, otherHome := dir("owner"), dir("team"), dir("other")

	// Host: a workspace repository and a fake tmux so a run starts without a
	// real session.
	hostBin := dir("hostbin")
	if err := os.WriteFile(filepath.Join(hostBin, "tmux"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	workspace := dir("owner", "workspace")
	repo := dir("owner", "workspace", "widget")
	origin := dir("origin.git")
	for _, args := range [][]string{
		{"-C", origin, "init", "-q", "--bare", "-b", "main"},
		{"-C", repo, "init", "-q", "-b", "main"},
		{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@example.test", "commit", "-q", "--allow-empty", "-m", "init"},
		{"-C", repo, "remote", "add", "origin", origin},
		{"-C", repo, "push", "-q", "origin", "main"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	hostConfig := filepath.Join(dir("owner", ".config", "hev"), "dispatch.toml")
	if err := os.WriteFile(hostConfig, []byte("cap = 2\nworkspace = \""+workspace+"\"\n"), 0600); err != nil {
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
	authorizedKeys := filepath.Join(ownerHome, ".ssh", "authorized_keys")
	conf := strings.Join([]string{
		"Port " + port,
		"ListenAddress 127.0.0.1",
		"HostKey " + hostKey,
		"PidFile none",
		"AuthorizedKeysFile " + authorizedKeys,
		"StrictModes no",
		"UsePAM no",
		"PasswordAuthentication no",
		"KbdInteractiveAuthentication no",
		"PubkeyAuthentication yes",
		"AllowTcpForwarding no",
		"SetEnv HOME=" + ownerHome + " PATH=" + hostBin + ":/usr/local/bin:/usr/bin:/bin",
		"LogLevel ERROR",
	}, "\n") + "\n"
	if err := os.WriteFile(sshdConfig, []byte(conf), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	daemon := exec.CommandContext(ctx, sshd, "-D", "-e", "-f", sshdConfig)
	var sshdLog bytes.Buffer
	daemon.Stdout, daemon.Stderr = &sshdLog, &sshdLog
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
			t.Fatalf("sshd did not start: %s", sshdLog.String())
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Members' ssh reads the config join wrote in their sandbox home and uses
	// the test port; the enrollment connection goes through the same wrapper.
	teamBin := dir("teambin")
	wrapper := "#!/bin/sh\nexec /usr/bin/ssh -F \"$HOME/.ssh/config\" -p " + port + " -o HostKeyAlias=127.0.0.1 \"$@\"\n"
	if err := os.WriteFile(filepath.Join(teamBin, "ssh"), []byte(wrapper), 0700); err != nil {
		t.Fatal(err)
	}
	run := func(home string, args ...string) (string, error) {
		t.Helper()
		c := exec.Command(hev, args...)
		path := hostBin + ":" + os.Getenv("PATH")
		if home != ownerHome {
			path = teamBin + ":" + path
			_ = os.MkdirAll(filepath.Join(home, ".ssh"), 0700)
			if _, err := os.Stat(filepath.Join(home, ".ssh", "config")); err != nil {
				_ = os.WriteFile(filepath.Join(home, ".ssh", "config"), nil, 0600)
			}
		}
		c.Env = []string{"HOME=" + home, "PATH=" + path, "USER=" + me.Username, "LANG=C"}
		var out bytes.Buffer
		c.Stdout, c.Stderr = &out, &out
		err := c.Run()
		return out.String(), err
	}
	must := func(home string, args ...string) string {
		t.Helper()
		out, err := run(home, args...)
		if err != nil {
			t.Fatalf("hev %v: %v\n%s\nsshd: %s", args, err, out, sshdLog.String())
		}
		return out
	}
	joinLine := regexp.MustCompile(`hev nodes join (\S+@127\.0\.0\.1) ([0-9A-Z]{2}-[0-9A-Z]{4}-[0-9A-Z]{4})`)
	invite := func(name string) (string, string) {
		out := must(ownerHome, "nodes", "invite", name, "--address", "127.0.0.1", "--user", me.Username, "--host-key", hostKey+".pub", "--expires", "30m")
		t.Logf("$ hev nodes invite %s ...\n%s", name, out)
		m := joinLine.FindStringSubmatch(out)
		if m == nil || len(out) > 600 {
			t.Fatalf("invite output: %s", out)
		}
		return m[1], m[2]
	}
	enrollOpen := func() bool {
		b, _ := os.ReadFile(authorizedKeys)
		return strings.Contains(string(b), "nodes enroll")
	}

	target, code := invite("alex")
	if !enrollOpen() {
		t.Fatal("invite did not open the enrollment key")
	}
	out := must(teamHome, "nodes", "join", target, code)
	t.Logf("$ hev nodes join %s %s\n%s", target, code, out)
	if !strings.Contains(out, "Waiting for the owner to approve") {
		t.Fatalf("join: %s", out)
	}
	memberFP := regexp.MustCompile(`Your key: (SHA256:\S+)`).FindStringSubmatch(out)
	if memberFP == nil {
		t.Fatalf("join printed no key fingerprint: %s", out)
	}
	if enrollOpen() {
		t.Fatal("enrollment key still open with no open invitation")
	}
	out = must(ownerHome, "nodes", "pending")
	t.Logf("$ hev nodes pending\n%s", out)
	if !strings.Contains(out, "alex invite=pending") || !strings.Contains(out, "key="+memberFP[1]) {
		t.Fatalf("pending: %s", out)
	}
	if out, err := run(otherHome, "nodes", "join", target, code); err == nil || !strings.Contains(out, "Permission denied") {
		t.Fatalf("reused code after enrollment closed: %v %s", err, out)
	}
	must(ownerHome, "nodes", "accept", "alex")
	authorized, _ := os.ReadFile(authorizedKeys)
	if !strings.HasPrefix(string(authorized), "restrict,command=") || !strings.Contains(string(authorized), "dispatch rpc --node") || strings.Contains(string(authorized), "nodes enroll") {
		t.Fatalf("authorized_keys after accept: %s", authorized)
	}
	var joined struct {
		Joined []struct{ Name string } `json:"joined"`
	}
	if err := json.Unmarshal([]byte(must(teamHome, "nodes", "ls", "--json")), &joined); err != nil || len(joined.Joined) != 1 {
		t.Fatalf("joined: %+v %v", joined, err)
	}
	alias := joined.Joined[0].Name
	order := filepath.Join(base, "short-run.md")
	if err := os.WriteFile(order, []byte("---\nname: short-run\nrepo: widget\nbase: main\n---\nTask\n## Acceptance\n- Done\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if out := must(teamHome, "dispatch", order, "--host", alias); !strings.Contains(out, "started short-run") {
		t.Fatalf("dispatch: %s", out)
	}
	b, err := os.ReadFile(filepath.Join(ownerHome, "dispatch", "short-run", "meta.json"))
	if err != nil || !strings.Contains(string(b), `"sender": "alex"`) && !strings.Contains(string(b), `"sender":"alex"`) {
		t.Fatalf("meta %s %v", b, err)
	}

	// Wrong codes lock the invitation; the right code is then refused.
	target, code = invite("sam")
	wrong := code[:3] + "ZZZZ-ZZZZ"
	if code[3:] == "ZZZZ-ZZZZ" {
		wrong = code[:3] + "YYYY-YYYY"
	}
	for n := 0; n < 5; n++ {
		if out, err := run(otherHome, "nodes", "join", target, wrong); err == nil || !strings.Contains(out, "could not prove") {
			t.Fatalf("wrong code %d: %v %s", n, err, out)
		}
	}
	if out, err := run(otherHome, "nodes", "join", target, code); err == nil || (!strings.Contains(out, "locked") && !strings.Contains(out, "Permission denied")) {
		t.Fatalf("right code after lockout: %v %s", err, out)
	}
	if out := must(ownerHome, "nodes", "pending"); !strings.Contains(out, "sam invite=locked") {
		t.Fatalf("pending after lockout: %s", out)
	}
	if enrollOpen() {
		t.Fatal("enrollment key open after the only open invitation locked")
	}
	if entries, _ := os.ReadDir(filepath.Join(otherHome, ".ssh")); len(entries) != 1 {
		t.Fatalf("failed joins wrote %v", entries)
	}
	if _, err := run(ownerHome, "nodes", "accept", "sam"); err == nil {
		t.Fatal("accepted a locked invitation")
	}
}
