package main

import (
	"bufio"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/spf13/cobra"
)

var dCmd = &cobra.Command{
	Use:   "d",
	Short: "Start the daemon or show status if already running",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check for the local indexing process.
		if len(findDaemonPIDs()) > 0 {
			fmt.Println("Daemon is running.")
			return daemonStatusCmd.RunE(cmd, args)
		}

		// Not running — start it.
		fmt.Println("Starting daemon...")
		return daemon.Run(cmd.Context())
	},
}

var sCmd = &cobra.Command{
	Use:   "s",
	Short: "Daemon status",
	RunE: func(cmd *cobra.Command, args []string) error {
		return daemonStatusCmd.RunE(cmd, args)
	},
}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run Capture, the trace archival daemon",
	Long:  "Continuously index local agent transcripts into the configured Layer namespace.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return daemon.Run(cmd.Context())
	},
}

var daemonInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the daemon as a launchd service",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		destBin, _, err := installSelf(home)
		if err != nil {
			return err
		}

		// Ensure ~/.hev exists for logs.
		if err := os.MkdirAll(filepath.Join(home, ".hev"), 0o755); err != nil {
			return err
		}
		configPath, err := daemon.WriteDefaultConfig(false)
		if err != nil {
			return fmt.Errorf("write default config: %w", err)
		}
		fmt.Printf("Config: %s\n", configPath)

		configPath, err = filepath.Abs(configPath)
		if err != nil {
			return fmt.Errorf("resolve config path: %w", err)
		}
		job := daemonJob(destBin, home, configPath)
		if _, err := job.ensure(home, true); err != nil {
			return err
		}
		fmt.Printf("Wrote %s\n", job.plistPath(home))
		fmt.Println("Daemon installed and started.")
		return nil
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check daemon health",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(findDaemonPIDs()) > 0 {
			fmt.Println("Daemon: running")
		} else {
			fmt.Println("Daemon: stopped")
		}
		status, err := daemon.ReadStatus()
		if err == nil {
			fmt.Printf("Last run: %s\n", status.LastRun.Format(time.RFC3339))
			fmt.Printf("Units indexed: %d\n", status.UnitsIndexed)
			if status.LastError == "" {
				fmt.Println("Last error: none")
			} else {
				fmt.Printf("Last error: %s\n", status.LastError)
			}
		}

		if procs := findDaemonProcesses(); len(procs) > 0 {
			fmt.Println("Processes:")
			for _, proc := range procs {
				fmt.Printf("  %s\n", proc)
			}
		}

		out, err := exec.Command("launchctl", "list", launchdLabel()).CombinedOutput()
		if err != nil {
			fmt.Println("LaunchAgent: not loaded")
			return nil
		}
		fmt.Println("LaunchAgent: loaded")
		fmt.Println(strings.TrimSpace(string(out)))
		return nil
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		return daemonStopCmd.RunE(cmd, args)
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Unload our KeepAlive job before signalling, so launchd cannot restart it.
		if _, err := (launchdJob{Label: launchdLabel()}).bootout(); err != nil {
			return err
		}
		pids := findDaemonPIDs()
		if len(pids) == 0 {
			fmt.Println("Daemon: stopped")
			return nil
		}

		for _, pid := range pids {
			if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
				fmt.Printf("SIGTERM pid %d: %v\n", pid, err)
			} else {
				fmt.Printf("Sent SIGTERM to pid %d\n", pid)
			}
		}
		if waitForDaemonExit(3 * time.Second) {
			fmt.Println("daemon stopped.")
			return nil
		}

		for _, pid := range pids {
			if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
				fmt.Printf("SIGKILL pid %d: %v\n", pid, err)
			} else {
				fmt.Printf("Sent SIGKILL to pid %d\n", pid)
			}
		}
		if waitForDaemonExit(2 * time.Second) {
			fmt.Println("daemon stopped.")
			return nil
		}
		return fmt.Errorf("daemon still alive after SIGKILL: pids=%v", pids)
	},
}

// waitForDaemonExit waits for local indexing processes to exit.
func waitForDaemonExit(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(findDaemonPIDs()) == 0 {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

var daemonUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the daemon launchd service",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		plistPath := launchdJob{Label: launchdLabel()}.plistPath(home)

		// Stop and unload only the index job before removing its plist.
		if err := daemonStopCmd.RunE(cmd, args); err != nil {
			return err
		}

		// Remove plist.
		if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove plist: %w", err)
		}

		fmt.Println("Daemon uninstalled.")
		return nil
	},
}

func init() {
	daemonCmd.AddCommand(daemonInstallCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonUninstallCmd)
}

func generatePlist(binPath, homeDir, configPath string) string {
	return daemonJob(binPath, homeDir, configPath).plist()
}

func plistEscape(s string) string {
	return html.EscapeString(s)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// Replace the inode: truncating an installed Mach-O can leave macOS's
	// code-signature cache attached to the old binary and kill new launches.
	out, err := os.CreateTemp(filepath.Dir(dst), ".hev-install-*")
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Chmod(0o755); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(out.Name(), dst)
}

func findDaemonProcesses() []string {
	out, err := exec.Command("ps", "-axo", "pid=,command=").Output()
	if err != nil {
		return nil
	}

	var procs []string
	self := os.Getpid()
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err == nil && pid == self {
			continue
		}
		if isDaemonProcess(line) {
			procs = append(procs, line)
		}
	}
	return procs
}

// findDaemonPIDs returns PIDs of running `hev d`/`hev daemon` processes,
// excluding the current process.
func findDaemonPIDs() []int {
	out, err := exec.Command("ps", "-axo", "pid=,command=").Output()
	if err != nil {
		return nil
	}

	var pids []int
	self := os.Getpid()
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid == self {
			continue
		}
		if isDaemonProcess(line) {
			pids = append(pids, pid)
		}
	}
	return pids
}

func isDaemonProcess(line string) bool {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return false
	}

	cmd := fields[1:]
	exe := filepath.Base(cmd[0])
	if exe != "hev" {
		return false
	}
	return len(cmd) == 2 && (cmd[1] == "d" || cmd[1] == "daemon")
}
