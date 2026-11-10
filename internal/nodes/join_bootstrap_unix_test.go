//go:build !windows

package nodes

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeSSH writes a stand-in for ssh that behaves like a hostile, not yet
// authenticated server: it ignores its arguments and runs script.
func fakeSSH(t *testing.T, script string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ssh")
	if e := os.WriteFile(p, []byte("#!/bin/sh\n"+script+"\n"), 0700); e != nil {
		t.Fatal(e)
	}
	return p
}

// TestBootstrapDeadlineKillsStalledServer: a server that accepts the public
// enrollment key and then never answers cannot hold the member past the
// deadline, and its stderr flood is capped.
func TestBootstrapDeadlineKillsStalledServer(t *testing.T) {
	bin := fakeSSH(t, "head -c 1000000 /dev/zero | tr '\\0' x >&2; exec sleep 600")
	conn, e := sshDial("alex@example.test", bin, 700*time.Millisecond)()
	if e != nil {
		t.Fatal(e)
	}
	start := time.Now()
	_, e = io.ReadAll(conn)
	if e == nil || !strings.Contains(e.Error(), "timed out") {
		t.Fatalf("read error %v", e)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("read returned after %s", d)
	}
	if e = conn.Close(); e != nil {
		t.Fatal(e)
	}
	sc := conn.(*sshConn)
	if sc.cmd.ProcessState == nil {
		t.Fatal("ssh not reaped")
	}
	if n := len(sc.stderr.String()); n > stderrLimit {
		t.Fatalf("kept %d bytes of stderr, limit %d", n, stderrLimit)
	}
	if _, e = os.Stat(sc.dir); !os.IsNotExist(e) {
		t.Fatalf("temporary key directory left: %v", e)
	}
}

// TestBootstrapCloseDoesNotWaitForServer: after a malformed answer the
// member closes; a server that keeps the channel open is killed, not waited
// on.
func TestBootstrapCloseDoesNotWaitForServer(t *testing.T) {
	bin := fakeSSH(t, "echo '{not json'; exec sleep 600")
	conn, e := sshDial("alex@example.test", bin, time.Minute)()
	if e != nil {
		t.Fatal(e)
	}
	buf := make([]byte, 64)
	if _, e = conn.Read(buf); e != nil {
		t.Fatal(e)
	}
	start := time.Now()
	_ = conn.Close()
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("close took %s", d)
	}
	if conn.(*sshConn).cmd.ProcessState == nil {
		t.Fatal("ssh not reaped")
	}
}

// TestBootstrapSSHIgnoresMemberForwarding: the bootstrap gives the
// unauthenticated server nothing from the member's ssh config.
func TestBootstrapSSHIgnoresMemberForwarding(t *testing.T) {
	args := enrollSSHArgs("/tmp/k", "/tmp/known", "alex@example.test")
	for _, want := range []string{"ClearAllForwardings=yes", "ForwardX11=no", "ForwardAgent=no", "PermitLocalCommand=no", "IdentitiesOnly=yes", "IdentityAgent=none", "ControlMaster=no", "ControlPath=none"} {
		i := slices.Index(args, want)
		if i < 1 || args[i-1] != "-o" {
			t.Fatalf("missing -o %s in %q", want, args)
		}
	}
	if i := slices.Index(args, "--"); i < 0 || i > slices.Index(args, "alex@example.test") {
		t.Fatalf("options not ended before target: %q", args)
	}
}

// TestConcurrentAttemptsAdmitOnlyMax: concurrent exchanges cannot get more
// than MaxAttempts offers out of one invitation.
func TestConcurrentAttemptsAdmitOnlyMax(t *testing.T) {
	h := newShortHost(t)
	r := h.invite(t, "alex")
	var wg sync.WaitGroup
	var n atomic.Int32
	for range 3 * MaxAttempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := h.s.attempt(r.Slot); e == nil {
				n.Add(1)
			}
		}()
	}
	wg.Wait()
	if n.Load() != MaxAttempts {
		t.Fatalf("admitted %d attempts, want %d", n.Load(), MaxAttempts)
	}
}
