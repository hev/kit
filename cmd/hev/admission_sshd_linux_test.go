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

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/gittest"
)

// TestAdmissionAddRemoveOverForcedCommandSSH runs the whole teammate path against a real
// sshd: invite, join, accept, then hev admission add/remove and hev dispatch through the
// forced-command key, with the host cloning over HTTPS with its own credential.
func TestAdmissionAddRemoveOverForcedCommandSSH(t *testing.T) {
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
	ownerHome, teamHome := dir("owner"), dir("team")

	// Host: its own git trust and credential, a policy and a fake tmux so an
	// admitted run starts without a real session.
	srv := gittest.Serve(t)
	srv.HostConfig(t, ownerHome)
	srv.Repo(t, "ExampleOrg/widget.git", map[string]string{"README.md": "widget\n"})
	srv.Repo(t, "Other/tool.git", map[string]string{"README.md": "tool\n"})
	hostBin := dir("hostbin")
	if err := os.WriteFile(filepath.Join(hostBin, "tmux"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	workspace := dir("owner", "workspace")
	hostConfig := filepath.Join(dir("owner", ".config", "hev"), "dispatch.toml")
	policy := "# shared build host\ncap = 2\nworkspace = \"" + workspace + "\"\n\n[admission]\nrepos = [] # owner-curated\nadd_from = [\"" + srv.URL + "/ExampleOrg/\"]\n"
	if err := os.WriteFile(hostConfig, []byte(policy), 0600); err != nil {
		t.Fatal(err)
	}

	// A real sshd running as this user, with the owner's authorized_keys.
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
		// The forced command must see the owner sandbox, never the real home.
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

	// The teammate's ssh reads the config join wrote in their (sandbox) home;
	// OpenSSH otherwise reads the passwd home and the default port.
	teamBin := dir("teambin")
	wrapper := "#!/bin/sh\nexec /usr/bin/ssh -F \"$HOME/.ssh/config\" -p " + port + " -o HostKeyAlias=127.0.0.1 \"$@\"\n"
	if err := os.WriteFile(filepath.Join(teamBin, "ssh"), []byte(wrapper), 0700); err != nil {
		t.Fatal(err)
	}
	run := func(home string, args ...string) (string, error) {
		t.Helper()
		c := exec.Command(hev, args...)
		path := os.Getenv("PATH")
		if home == teamHome {
			path = teamBin + ":" + path
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

	code := strings.TrimSpace(must(ownerHome, "nodes", "invite", "alex", "--address", "127.0.0.1", "--user", me.Username, "--host-key", hostKey+".pub"))
	request := strings.TrimSpace(strings.Split(must(teamHome, "nodes", "join", code), "\n")[0])
	must(ownerHome, "nodes", "accept", request)
	var joined struct {
		Joined []struct{ Name string } `json:"joined"`
	}
	if err := json.Unmarshal([]byte(must(teamHome, "nodes", "ls", "--json")), &joined); err != nil || len(joined.Joined) != 1 {
		t.Fatalf("joined: %+v %v", joined, err)
	}
	alias := joined.Joined[0].Name
	if out := must(ownerHome, "nodes", "ls"); !strings.Contains(out, "alex ") || !strings.Contains(out, "allow=-") {
		t.Fatalf("default grant: %s", out)
	}
	authorized, _ := os.ReadFile(filepath.Join(ownerHome, ".ssh", "authorized_keys"))
	if !strings.HasPrefix(string(authorized), "restrict,command=") || !strings.Contains(string(authorized), "dispatch rpc --node") {
		t.Fatalf("not a forced-command key: %s", authorized)
	}

	widget := srv.URL + "/ExampleOrg/widget.git"
	order := func(name string) string {
		p := filepath.Join(base, name+".md")
		body := "---\nname: " + name + "\nrepo: widget\nbase: main\n---\nTask\n## Acceptance\n- Done\n"
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	workspaceEntries := func() []string {
		entries, _ := os.ReadDir(workspace)
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return names
	}

	// Without the capability: listing works, adding is refused, nothing cloned,
	// and dispatch to the repo is not admitted.
	if out := must(teamHome, "admission", "ls", "--host", alias); !strings.Contains(out, "can-add=false") || !strings.Contains(out, "add-from "+srv.URL+"/ExampleOrg/") {
		t.Fatalf("ls: %s", out)
	}
	if out, err := run(teamHome, "admission", "add", widget, "--host", alias); err == nil || !strings.Contains(out, "your access does not include adding repos") || strings.Contains(out, "allowed organization") {
		t.Fatalf("add without capability: %v %s", err, out)
	}
	if out, err := run(teamHome, "dispatch", order("before-add"), "--host", alias); err == nil || !strings.Contains(out, "not admitted") {
		t.Fatalf("dispatch before add: %v %s", err, out)
	}
	if out, err := run(teamHome, "admission", "remove", widget, "--host", alias); err == nil || !strings.Contains(out, "your access does not include removing repos") || strings.Contains(out, "allowed organization") {
		t.Fatalf("remove without capability: %v %s", err, out)
	}

	must(ownerHome, "nodes", "allow", "alex", "repos")
	if out := must(ownerHome, "nodes", "ls"); !strings.Contains(out, "allow=repos") {
		t.Fatalf("grant not shown: %s", out)
	}

	// Outside add_from: refused and nothing cloned.
	before, _ := os.ReadFile(hostConfig)
	if out, err := run(teamHome, "admission", "add", srv.URL+"/Other/tool.git", "--host", alias); err == nil || !strings.Contains(out, "not in an allowed organization") || strings.Contains(out, "your access") {
		t.Fatalf("add outside add_from: %v %s", err, out)
	}
	if names := workspaceEntries(); len(names) != 0 {
		t.Fatalf("refused add left %v", names)
	}
	if after, _ := os.ReadFile(hostConfig); string(after) != string(before) {
		t.Fatalf("refused add edited config: %s", after)
	}

	// Allowed: cloned with the host's credential, admitted, attributed.
	if out := must(teamHome, "admission", "add", widget, "--host", alias); !strings.Contains(out, "added "+widget+" as widget") {
		t.Fatalf("add: %s", out)
	}
	if out := must(teamHome, "admission", "add", widget, "--host", alias); !strings.Contains(out, "already present") {
		t.Fatalf("repeat add: %s", out)
	}
	if names := workspaceEntries(); len(names) != 1 || names[0] != "widget" {
		t.Fatalf("workspace %v", names)
	}
	after, _ := os.ReadFile(hostConfig)
	if want := strings.Replace(policy, "repos = []", "repos = [\""+widget+"\"]", 1); string(after) != want {
		t.Fatalf("config not preserved:\n%s", after)
	}
	var decoded map[string]any
	if _, err := toml.Decode(string(after), &decoded); err != nil {
		t.Fatal(err)
	}
	if gitConfig, _ := os.ReadFile(filepath.Join(workspace, "widget", ".git", "config")); strings.Contains(string(gitConfig), gittest.Password) {
		t.Fatal("credential persisted into the clone")
	}
	if out := must(ownerHome, "admission", "ls"); !strings.Contains(out, "widget "+widget+" added-by=alex") {
		t.Fatalf("owner ls: %s", out)
	}

	// The added repo is now admitted for the teammate's dispatch.
	if out := must(teamHome, "dispatch", order("after-add"), "--host", alias); !strings.Contains(out, "started after-add") {
		t.Fatalf("dispatch after add: %s", out)
	}
	var meta struct {
		Sender      string `json:"sender"`
		RequestedBy string `json:"requested_by"`
	}
	b, err := os.ReadFile(filepath.Join(ownerHome, "dispatch", "after-add", "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &meta); err != nil || meta.Sender != "alex" || meta.RequestedBy != "alex" {
		t.Fatalf("meta %s %v", b, err)
	}
	if _, err = os.Stat(filepath.Join(ownerHome, "dispatch", "after-add", "wt", "README.md")); err != nil {
		t.Fatalf("worktree not created: %v", err)
	}

	// Removal: outside add_from is owner-only; inside it the entry goes, the
	// clone and the started run stay, and new dispatches are refused.
	if out, err := run(teamHome, "admission", "remove", srv.URL+"/Other/tool.git", "--host", alias); err == nil || !strings.Contains(out, "was not added from an allowed organization; ask the owner") || strings.Contains(out, "your access") {
		t.Fatalf("remove outside add_from: %v %s", err, out)
	}
	if out := must(teamHome, "admission", "remove", widget, "--host", alias); !strings.Contains(out, "removed "+widget) || !strings.Contains(out, "clone widget stays in the workspace") || !strings.Contains(out, "new dispatches to it are refused") {
		t.Fatalf("remove: %s", out)
	}
	if after, _ := os.ReadFile(hostConfig); string(after) != policy {
		t.Fatalf("remove did not restore config:\n%s", after)
	}
	if names := workspaceEntries(); len(names) != 1 || names[0] != "widget" {
		t.Fatalf("remove touched workspace %v", names)
	}
	if _, err = os.Stat(filepath.Join(ownerHome, "dispatch", "after-add", "wt", "README.md")); err != nil {
		t.Fatalf("remove touched the started run: %v", err)
	}
	if out, err := run(teamHome, "dispatch", order("after-remove"), "--host", alias); err == nil || !strings.Contains(out, "not admitted") {
		t.Fatalf("dispatch after remove: %v %s", err, out)
	}
	if out := must(teamHome, "admission", "remove", widget, "--host", alias); !strings.Contains(out, "not admitted; nothing to remove") {
		t.Fatalf("repeat remove: %s", out)
	}
	if log, _ := os.ReadFile(filepath.Join(filepath.Dir(hostConfig), "admission-log.jsonl")); strings.Count(string(log), "\n") != 2 || !strings.Contains(string(log), `"action":"remove"`) || !strings.Contains(string(log), `"removed_by":"alex"`) {
		t.Fatalf("admission log: %s", log)
	}
	if out := must(ownerHome, "admission", "ls"); strings.Contains(out, widget) {
		t.Fatalf("owner ls after remove: %s", out)
	}

	// Withdrawing the capability applies to the next RPC.
	must(ownerHome, "nodes", "deny", "alex", "repos")
	if out, err := run(teamHome, "admission", "add", srv.URL+"/ExampleOrg/other.git", "--host", alias); err == nil || !strings.Contains(out, "your access does not include adding repos") || strings.Contains(out, "allowed organization") {
		t.Fatalf("add after deny: %v %s", err, out)
	}
}
