package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func fakeScript(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX script fixture")
	}
	path := filepath.Join(t.TempDir(), "provider")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+body), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestCommandProviderScripts(t *testing.T) {
	t.Setenv("SYNTHETIC_CLOUD_TOKEN", "synthetic")
	t.Setenv("UNLISTED_SECRET", "must-not-reach-script")
	script := fakeScript(t, `input=$(cat)
 test "$SYNTHETIC_CLOUD_TOKEN" = synthetic
 test -z "${UNLISTED_SECRET:-}"
 test -n "$input"
 printf '%s' '{"id":"vm","ssh":"user@example.invalid","host_key":"example.invalid ssh-ed25519 synthetic","state":"ready"}'
`)
	list := fakeScript(t, `cat >/dev/null
 printf '%s' '[{"id":"vm","ssh":"user@example.invalid","host_key":"example.invalid ssh-ed25519 synthetic","state":"ready","labels":{"hev.dispatch":"cloud","hev.order":"one"}}]'
`)
	mutation := fakeScript(t, "cat >/dev/null\n")
	p, _ := NewCommandProvider(Host{Name: "cloud", Env: []string{"SYNTHETIC_CLOUD_TOKEN"}, Commands: CommandScripts{Create: script, List: list, Delete: mutation, Stop: mutation, Resume: mutation}})
	m, err := p.Create(context.Background(), MachineSpec{Name: "d-one", AutoStop: time.Minute})
	if err != nil || m.ID != "vm" {
		t.Fatal(m, err)
	}
	tr, err := p.Transport(context.Background(), "vm")
	if err != nil || tr.Kind != TransportSSH || tr.SSH != m.SSH {
		t.Fatal(tr, err)
	}
	for _, fn := range []func() error{func() error { return p.Stop(context.Background(), "vm") }, func() error { return p.Delete(context.Background(), "vm") }, func() error { return p.Resume(context.Background(), "vm", time.Minute) }} {
		if err = fn(); err != nil {
			t.Fatal(err)
		}
	}
}
func TestCommandProviderWithholdsOutput(t *testing.T) {
	p, _ := NewCommandProvider(Host{Commands: CommandScripts{Create: fakeScript(t, "echo secret >&2\necho secret\nexit 1\n")}})
	_, err := p.Create(context.Background(), MachineSpec{})
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
}
func TestCommandConfigEnvironmentNamesOnly(t *testing.T) {
	for _, env := range []string{`["TOKEN"]`, `["TOKEN=value"]`} {
		text := "[hosts.cloud]\ntype='command'\ncap=1\nenv=" + env + "\n[hosts.cloud.commands]\ncreate='/tmp/create'\nlist='/tmp/list'\ndelete='/tmp/delete'\n"
		path := filepath.Join(t.TempDir(), "dispatch.toml")
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := Load(path)
		if (err == nil) != (env == `["TOKEN"]`) {
			t.Fatal(env, err)
		}
	}
}
func TestCloudClassQueues(t *testing.T) {
	f := &fakeEphemeral{machines: []Machine{{ID: "vm", State: "ready", Labels: map[string]string{"hev.order": "other"}}}}
	withFakeBoat(t, f)
	h := Host{Name: "cloud", Cap: 3, Classes: map[string]int{"build": 1}}
	dir := boatRecordDir(h.Name, "other")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Class: "build"}}); err != nil {
		t.Fatal(err)
	}
	old := cloudQueueLauncher
	cloudQueueLauncher = func(Host) error { return nil }
	defer func() { cloudQueueLauncher = old }()
	q := Request{Action: "start", Name: "one", Order: "---\nname: one\nrepo: example/repo\nhost: cloud\nharness: codex\nvalidation: none\nclass: build\n---\nSmoke.\n## Acceptance\n- Finishes.", Meta: Meta{Order: Order{Name: "one", Repo: "example/repo", Validation: "none", Class: "build"}}}
	r, err := boatRPC(context.Background(), h, q)
	if err != nil || !strings.Contains(r.Message, "queued") || f.created != 0 {
		t.Fatal(r, err, f.created)
	}
	q.Dequeue = true
	if _, err = boatRPC(context.Background(), h, q); err != errClassFull {
		t.Fatal(err)
	}
}

func TestSSHTransportPinsKeyAndExcludesProviderCredentials(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture")
	}
	dir := t.TempDir()
	ssh := filepath.Join(dir, "ssh")
	script := `#!/bin/sh
set -eu
test -z "${SYNTHETIC_PROVIDER_SECRET:-}"
found=
for arg in "$@"; do
 case "$arg" in UserKnownHostsFile=*) found=${arg#UserKnownHostsFile=};; esac
done
test -n "$found"
test "$(cat "$found")" = "vm.example.invalid ssh-ed25519 synthetic-public"
printf 'raw-file-bytes'
`
	if err := os.WriteFile(ssh, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SYNTHETIC_PROVIDER_SECRET", "never-forward")
	got, err := sshMachineExec(context.Background(), MachineTransport{Kind: TransportSSH, SSH: "user@vm.example.invalid", HostKey: "vm.example.invalid ssh-ed25519 synthetic-public"}, "cat /tmp/file")
	if err != nil || got != "raw-file-bytes" {
		t.Fatal(got, err)
	}
	if _, err = sshMachineExec(context.Background(), MachineTransport{SSH: "-oProxyCommand=bad", HostKey: "key"}, "true"); err == nil {
		t.Fatal("SSH option injection accepted")
	}
}
