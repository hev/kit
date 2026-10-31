package dispatch

import (
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
