package dispatch

import (
	"context"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWindowsContainmentFlags(t *testing.T) {
	flags := windowsSupervisorFlags()
	for _, flag := range []uint32{windows.CREATE_BREAKAWAY_FROM_JOB, windows.DETACHED_PROCESS, windows.CREATE_NEW_PROCESS_GROUP} {
		if flags&flag == 0 {
			t.Fatalf("supervisor missing creation flag %#x", flag)
		}
	}
	if windowsHarnessFlags()&windows.CREATE_SUSPENDED == 0 {
		t.Fatal("harness must not execute before job assignment")
	}
}

func TestWindowsWrappedHarnessCommandLine(t *testing.T) {
	c := harnessCommand([]string{`C:\Program Files\launcher.exe`, "-File", `C:\tools\with creds.ps1`, "-FilePath"}, `C:\Program Files\codex.exe`, []string{"exec", `say "hello"`, `C:\path with spaces\`})
	configureHarnessCommandLine(c)
	got := c.SysProcAttr.CmdLine
	want := `"C:\Program Files\launcher.exe" -File "C:\tools\with creds.ps1" -FilePath -- "C:\Program Files\codex.exe" exec "say \"hello\"" "C:\path with spaces\\"`
	if got != want {
		t.Fatalf("command line:\n%s\nwant:\n%s", got, want)
	}
}

func TestPowerShellFileSentinel(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh.exe")
	if err != nil {
		t.Skip("PowerShell 7 not installed")
	}
	script := filepath.Join(t.TempDir(), "args.ps1")
	os.WriteFile(script, []byte(`if ($args.Count -ne 3 -or $args[0] -cne '--' -or $args[1] -cne 'codex' -or $args[2] -cne 'two words') { exit 3 }`), 0600)
	c := exec.Command(pwsh, "-NoProfile", "-File", script, "--", "codex", "two words")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("sentinel lost: %v %s", err, out)
	}
}

func TestWindowsAmendStopTerminatesHarnessTree(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(Root(), "example")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	child := exec.Command("cmd.exe", "/c", "ping -n 60 127.0.0.1 >NUL")
	startTestChild(t, child)
	// Admitting this child must not permit probes of other live processes.
	if _, err := created(os.Getppid()); err == nil {
		t.Fatal("foreign process probe accepted while child is owned")
	}
	if err := recordHarness(dir, child); err != nil {
		t.Fatal(err)
	}
	if err := stopAttempt(dir, "example"); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err == nil {
		t.Fatal("harness exited successfully instead of being terminated")
	}
	if _, err := os.Stat(filepath.Join(dir, "stop")); err != nil {
		t.Fatal(err)
	}
}

// m1: closing the notification job terminates a descendant the sender left
// behind, whether the sender exits zero or runs past its deadline.
func TestWindowsNotificationReapsDescendant(t *testing.T) {
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Skip("Windows PowerShell not installed")
	}
	for name, tc := range map[string]struct {
		tail    string
		timeout time.Duration
		fails   bool
	}{
		"success": {"exit 0", orchestratorNoticeTimeout, false},
		"timeout": {"Start-Sleep 600", 10 * time.Second, true},
	} {
		t.Run(name, func(t *testing.T) {
			pidFile := filepath.Join(t.TempDir(), "descendant.pid")
			script := "$p = Start-Process -FilePath ping.exe -ArgumentList '-n','600','127.0.0.1' -NoNewWindow -PassThru; " +
				"Set-Content -Path '" + pidFile + "' -Value $p.Id; " + tc.tail
			done := make(chan error, 1)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), tc.timeout)
				defer cancel()
				done <- executeNotification(ctx, []string{"powershell.exe", "-NoProfile", "-Command", script}, true)
			}()
			select {
			case err := <-done:
				if (err != nil) != tc.fails {
					t.Fatalf("error %v, want failure %v", err, tc.fails)
				}
			case <-time.After(40 * time.Second):
				t.Fatal("notice send hung")
			}
			b, err := os.ReadFile(pidFile)
			if err != nil {
				t.Fatal(err)
			}
			pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
			if err != nil || pid <= 0 {
				t.Fatalf("descendant pid %q", b)
			}
			for i := 0; ; i++ {
				if _, err := processCreated(pid); err != nil {
					return
				}
				if i == 40 {
					_ = exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/F").Run()
					t.Fatalf("sender returned but descendant %d remains alive", pid)
				}
				time.Sleep(50 * time.Millisecond)
			}
		})
	}
}
