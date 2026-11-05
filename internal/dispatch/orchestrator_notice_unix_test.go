//go:build !windows

package dispatch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// lockHeld reports whether dir's lockDirectory lock is held, by this process
// or another: flock conflicts across open file descriptions.
func lockHeld(dir string) bool {
	f, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return false
	}
	defer f.Close()
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return true
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return false
}

func TestOrchestratorFakeHarnessAndSenderEndToEnd(t *testing.T) {
	orchHome(t)
	bin := t.TempDir()
	log := filepath.Join(bin, "sent")
	sender := filepath.Join(bin, "fake-sender")
	// Model the native peers entry point, checking identity, stable IDs and the
	// backend isolation used for detached dispatch completion notices.
	script := `#!/bin/sh
[ "$HEV_PEERS_BACKEND" = "" ] || exit 1
[ "$PEERS_FROM" = "" ] || exit 2
[ "$1" = peers ] && [ "$2" = send ] && [ "$3" = owner ] || exit 3
[ "$5" = --from ] && [ "$6" = build-1/o-release-train ] || exit 4
[ "$7" = --id ] && [ "$9" = --inbox-only ] || exit 5
printf '%s\n' "$4" >> "$NOTICE_TEST_LOG"
`
	if err := os.WriteFile(sender, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte("#!/bin/sh\nprintf 'Goal pending. Reply notes: decision needed.\\n' > checkpoint.md\nprintf 'Decision needed: approve the next step.\\n' > reply.md\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("NOTICE_TEST_LOG", log)
	t.Setenv("PEERS_FROM", "stale-harness")
	t.Setenv("HEV_PEERS_BACKEND", "absent-backend")
	old := notificationExecutable
	notificationExecutable = func() (string, error) { return sender, nil }
	t.Cleanup(func() { notificationExecutable = old })
	orchestratorNoticeSender = sendOrchestratorNotice
	raw := strings.Replace(orchestratorExample, "harness: claude", "harness: codex\nhost: build-1", 1)
	raw = strings.Replace(raw, "say, rework", "notify, say, rework", 1)
	s, err := ApplyOrchestrator(raw, false)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := orchestratorState(s.Name)
	d := DefaultOrchestratorDriver()
	d.Gather = noticeDriver(t, nil).Gather
	if err := RunOrchestratorRound(context.Background(), s, &st, d); err != nil {
		t.Fatal(err)
	}
	if err := OrchestratorControl(s.Name, "stop", ""); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range []string{"started/applied", "Decision needed:", "stopped"} {
		if strings.Count(string(b), message) != 1 {
			t.Fatal(string(b))
		}
	}
	rows, err := ListOrchestrators()
	if err != nil || rows[0].LastNotice.Status != "delivered" || !strings.Contains(rows[0].LastNotice.Message, "stopped") {
		t.Fatal(rows, err)
	}
}

// m1: a sender slower than the old 5s deadline succeeds.
func TestNoticeSenderSlow(t *testing.T) {
	t.Parallel()
	exe := filepath.Join(t.TempDir(), "hev")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nsleep 8\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), orchestratorNoticeTimeout)
	defer cancel()
	if err := executeNotification(ctx, []string{exe, "peers", "send", "owner", "m"}, true); err != nil {
		t.Fatal(err)
	}
}

// m1: a grandchild holding the output pipe neither hangs delivery nor
// survives it, whether the sender exits zero or runs past its deadline.
func TestNoticeSenderReapsHungGrandchild(t *testing.T) {
	for name, tc := range map[string]struct {
		tail    string
		timeout time.Duration
		fails   bool
	}{
		"success": {"exit 0", orchestratorNoticeTimeout, false},
		"timeout": {"sleep 600", 2 * time.Second, true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			bin := t.TempDir()
			exe := filepath.Join(bin, "hev")
			pidFile := filepath.Join(bin, "grandchild.pid")
			script := "#!/bin/sh\nsleep 600 &\necho $! > " + pidFile + "\n" + tc.tail + "\n"
			if err := os.WriteFile(exe, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), tc.timeout)
				defer cancel()
				done <- executeNotification(ctx, []string{exe, "peers", "send", "owner", "m"}, true)
			}()
			select {
			case err := <-done:
				if (err != nil) != tc.fails {
					t.Fatalf("error %v, want failure %v", err, tc.fails)
				}
			case <-time.After(15 * time.Second):
				t.Fatal("notice send hung")
			}
			b, err := os.ReadFile(pidFile)
			if err != nil {
				t.Fatal(err)
			}
			pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
			if err != nil || pid <= 0 {
				t.Fatalf("grandchild pid %q", b)
			}
			// Allow init a moment to reap the killed, reparented zombie.
			for i := 0; processAlive(pid); i++ {
				if i == 40 {
					_ = syscall.Kill(pid, syscall.SIGKILL)
					t.Fatalf("sender returned but grandchild %d remains alive", pid)
				}
				time.Sleep(50 * time.Millisecond)
			}
		})
	}
}

// processAlive reports whether pid exists and is not a zombie.
func processAlive(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	return err == nil && !strings.HasPrefix(strings.TrimSpace(string(out)), "Z")
}

// Every group signal must name a group whose leader this process has not yet
// reaped. A reaped leader's number is free for reuse, so signalling it could
// kill an unrelated group.
func TestProcessGroupSignalledOnlyWhileOwned(t *testing.T) {
	var mu sync.Mutex
	var signals int
	var unowned []string
	old := signalProcessGroup
	signalProcessGroup = func(pgid int) error {
		out, err := exec.Command("ps", "-o", "ppid=", "-p", strconv.Itoa(pgid)).Output()
		mu.Lock()
		signals++
		if err != nil || strings.TrimSpace(string(out)) != strconv.Itoa(os.Getpid()) {
			unowned = append(unowned, strconv.Itoa(pgid))
		}
		mu.Unlock()
		return old(pgid)
	}
	t.Cleanup(func() { signalProcessGroup = old })
	sender := func(t *testing.T, body string) string {
		exe := filepath.Join(t.TempDir(), "hev")
		if err := os.WriteFile(exe, []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
		return exe
	}
	for name, run := range map[string]func(t *testing.T){
		"notice success": func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), orchestratorNoticeTimeout)
			defer cancel()
			if err := executeNotification(ctx, []string{sender(t, "exit 0"), "peers"}, true); err != nil {
				t.Fatal(err)
			}
		},
		"notice timeout": func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := executeNotification(ctx, []string{sender(t, "sleep 600"), "peers"}, true); err == nil {
				t.Fatal("timed-out sender succeeded")
			}
		},
		"source plugin": func(t *testing.T) {
			c := exec.CommandContext(context.Background(), sender(t, "exit 0"))
			cleanup, err := startSourceChild(c, "")
			if err != nil {
				t.Fatal(err)
			}
			if err = c.Wait(); err != nil {
				t.Fatal(err)
			}
			cleanup()
		},
	} {
		t.Run(name, func(t *testing.T) {
			mu.Lock()
			signals, unowned = 0, nil
			mu.Unlock()
			run(t)
			mu.Lock()
			defer mu.Unlock()
			if signals == 0 {
				t.Fatal("no group kill")
			}
			if len(unowned) > 0 {
				t.Fatalf("signalled group(s) %v after their leader was reaped", unowned)
			}
		})
	}
}
