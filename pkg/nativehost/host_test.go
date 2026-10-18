//go:build linux || darwin

package nativehost

import (
	"context"
	"encoding/json"
	n "github.com/hev/kit/pkg/nativejob"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fixture struct {
	p                              Profile
	config, desc, effects, release string
	call                           Dispatch
}

func setup(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	if e := os.Chmod(root, 0700); e != nil {
		t.Fatal(e)
	}
	state := filepath.Join(root, "state")
	if e := os.Mkdir(state, 0700); e != nil {
		t.Fatal(e)
	}
	cfg := filepath.Join(root, "receiver.json")
	_ = os.WriteFile(cfg, []byte("{}"), 0600)
	effects := filepath.Join(root, "effects")
	release := filepath.Join(root, "release")
	binary := filepath.Join(root, "receiver")
	reg := Registration{RegistrationVersion, n.Enrollment{HostID: "host", ExecutorID: "executor"}, "physical-node", "inc", 3, time.Now().UTC()}
	rb, _ := json.Marshal(reg)
	script := "#!/bin/sh\nif [ \"$3\" = '--register-host' ]; then cat >/dev/null; printf '%s\\n' '" + string(rb) + "'; exit 0; fi\nprintf effect\\n >> '" + effects + "'\nwhile [ ! -f '" + release + "' ]; do sleep 0.02; done\n"
	if e := os.WriteFile(binary, []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	p := Profile{ID: "one", ConfigRef: cfg, ConfigDigest: hash([]byte("{}")), SupervisorRef: binary, SupervisorDigest: hash([]byte(script)), StateRoot: state, Enrollment: reg.Enrollment, NodeUID: reg.NodeUID, Incarnation: reg.Incarnation, AdmissionGeneration: 3, Runtime: Runtime{Directory: root, Environment: []string{"HOME=" + root, "PATH=/usr/bin:/bin"}, Files: []FilePin{}}}
	config := filepath.Join(root, "host.json")
	b, _ := json.Marshal(Config{Version, filepath.Join(root, "control.sock"), []Profile{p}})
	_ = os.WriteFile(config, b, 0600)
	callJSON := json.RawMessage(`{"version":"native-receiver/v1","operation":"start","job_id":"original","claim":{},"binding":{}}`)
	d := descriptor{"native-supervisor/v1", cfg, p.ConfigDigest, binary, p.SupervisorDigest, callJSON, 120, ""}
	d.DescriptorDigest = n.Digest(d)
	dir := filepath.Join(state, strings.TrimPrefix(n.Digest("original"), "sha256:"))
	_ = os.Mkdir(dir, 0700)
	desc := filepath.Join(dir, "descriptor.json")
	if e := exclusive(desc, d); e != nil {
		t.Fatal(e)
	}
	f := fixture{p, config, desc, effects, release, Dispatch{DispatchVersion, p.ID, desc, d.DescriptorDigest}}
	t.Cleanup(func() { _ = os.WriteFile(release, []byte("release"), 0600) })
	return f
}
func TestVerifyReadOnlyAndStrictPins(t *testing.T) {
	f := setup(t)
	if _, e := VerifyConfig(f.config); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(f.effects); !os.IsNotExist(e) {
		t.Fatal("verify launched worker")
	}
	b, _ := os.ReadFile(f.config)
	for _, bad := range []string{strings.Replace(string(b), `"version"`, `"VERSION"`, 1), strings.Replace(string(b), `"id":"one"`, `"id":"one","id":"other"`, 1), string(b) + ` {}`, strings.Replace(string(b), `"runtime":{`, `"runtime":{"argv":[],`, 1)} {
		_ = os.WriteFile(f.config, []byte(bad), 0600)
		if _, e := VerifyConfig(f.config); e == nil {
			t.Fatal("ambiguous config accepted")
		}
	}
	_ = os.WriteFile(f.config, b, 0600)
	_ = os.Chmod(f.p.SupervisorRef, 0775)
	if _, e := VerifyConfig(f.config); e == nil {
		t.Fatal("writable receiver accepted")
	}
	_ = os.Chmod(f.p.SupervisorRef, 0700)
	_ = os.WriteFile(f.p.ConfigRef, []byte("changed"), 0600)
	if _, e := VerifyConfig(f.config); e == nil {
		t.Fatal("changed config accepted")
	}
}
func TestDescriptorBindsCanonicalBytesAndOriginalPath(t *testing.T) {
	f := setup(t)
	if e := validateDescriptor(f.p, f.call); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(f.desc)
	for _, bad := range []string{strings.Replace(string(b), `"operation":"start"`, `"operation":"inspect"`, 1), strings.Replace(string(b), `"job_id":"original"`, `"job_id":"other"`, 1), strings.Replace(string(b), `"start_call"`, `"START_CALL"`, 1)} {
		_ = os.WriteFile(f.desc, []byte(bad), 0600)
		if validateDescriptor(f.p, f.call) == nil {
			t.Fatal("changed descriptor accepted")
		}
	}
}
func TestOutputBudgetCannotUsePromotedReadFrom(t *testing.T) {
	var b limitedOutput
	count, e := writerOnly{&b}.Write([]byte(strings.Repeat("a", maxFrame+1)))
	if e == nil || count != 0 || !b.overflow || b.Len() != 0 {
		t.Fatal("output limit bypass")
	}
}
func TestRunRejectsNonUnixListener(t *testing.T) {
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	if Run(context.Background(), nil, l) == nil {
		t.Fatal("non Unix listener accepted")
	}
}
func TestHostDispatchRetainsOwnedReceiverAndNeverReplays(t *testing.T) {
	if !supported() {
		t.Skip("Linux IPC only")
	}
	f := setup(t)
	cfg, e := VerifyConfig(f.config)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- RunConfig(ctx, f.config) }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, e := os.Stat(cfg.Socket); e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("socket not ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if e := DispatchLocal(context.Background(), cfg.Socket, f.call); e != nil {
		t.Fatal(e)
	}
	if e := DispatchLocal(context.Background(), cfg.Socket, f.call); e == nil {
		t.Fatal("duplicate dispatch accepted")
	}
	deadline = time.Now().Add(3 * time.Second)
	for {
		if b, e := os.ReadFile(f.effects); e == nil && len(b) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	// Closing IPC/host context does not signal an owned receiver. Managed daemon
	// death/control-group cleanup is a separate integration proof, not asserted.
	b, _ := os.ReadFile(f.effects)
	if strings.Count(string(b), "effect") != 1 {
		t.Fatal("effect count", string(b))
	}
	if _, e := os.Stat(filepath.Join(filepath.Dir(f.desc), "host-registration.json")); e != nil {
		t.Fatal(e)
	}
	// Restart keeps old intent; no adoption or replay.
	ctx2, cancel2 := context.WithCancel(context.Background())
	done2 := make(chan error, 1)
	go func() { done2 <- RunConfig(ctx2, f.config) }()
	defer cancel2()
	deadline = time.Now().Add(2 * time.Second)
	for {
		if _, e := os.Stat(cfg.Socket); e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("restart socket")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if DispatchLocal(context.Background(), cfg.Socket, f.call) == nil {
		t.Fatal("restart replayed")
	}
	cancel2()
	if e := <-done2; e != nil {
		t.Fatal(e)
	}
}

func TestRegistrationSubprocessBudgetAndEnvironmentBeforeIntent(t *testing.T) {
	f := setup(t)
	script := "#!/bin/sh\nhead -c 70000 /dev/zero\n"
	if e := os.WriteFile(f.p.SupervisorRef, []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	f.p.SupervisorDigest = hash([]byte(script))
	if _, e := inspectRegistration(context.Background(), f.p, f.call); e == nil {
		t.Fatal("oversized real subprocess registration accepted")
	}
	for _, env := range [][]string{{"HOME=/tmp", "PATH=/usr/bin:/bin", "OP_SERVICE_ACCOUNT_TOKEN=forbidden"}, {"HOME=/tmp", "PATH=relative"}, {"HOME=/tmp", "PATH=/usr/bin:/bin", "HOME=/other"}, {"HOME=/tmp", "PATH=/usr/bin:/bin", "SSL_CERT_FILE=/not-pinned"}} {
		p := f.p
		p.Runtime.Environment = env
		if p.verify() == nil {
			t.Fatal("unsafe runtime env accepted")
		}
	}
}
