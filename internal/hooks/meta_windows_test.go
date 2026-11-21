//go:build windows

package hooks

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsMetaBatchHandler(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	bin := filepath.Join(home, "Program Files (x86)")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	cfg := filepath.Join(home, "config.toml")
	t.Setenv("HEV_CONFIG", cfg)
	if err := os.WriteFile(cfg, []byte("[hooks]\ntimeout = \"10s\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_HOOKS", "")
	script := "@echo off\r\nif not \"%~1\"==\"admission-check\" exit /b 2\r\nif not \"%HEV_HOOK_HARNESS%\"==\"kit\" exit /b 3\r\necho {\"decision\":\"block\",\"reason\":\"fixture denial\"}\r\n"
	if err := os.WriteFile(filepath.Join(bin, "hev-hook-policy.cmd"), []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	r := Meta(context.Background(), "admission-check", map[string]any{"name": "fixture"}, "")
	if r.Decision != Block || r.Reason != "fixture denial" {
		t.Fatalf("batch handler response = %+v", r)
	}
}

func TestWindowsHookChildProcess(t *testing.T) {
	ready := os.Getenv("HEV_WINDOWS_HOOK_CHILD_PID")
	if ready == "" {
		return
	}
	if err := os.WriteFile(ready, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(45 * time.Second)
}

func TestWindowsHookCancellationTerminatesBatchDescendants(t *testing.T) {
	dir := t.TempDir()
	ready := filepath.Join(dir, "child.pid")
	t.Setenv("HEV_WINDOWS_HOOK_CHILD_PID", ready)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_WINDOWS_HOOK_TEST_EXE", self)
	exe := filepath.Join(dir, "hev-hook-child.cmd")
	script := "@echo off\r\n\"%HEV_WINDOWS_HOOK_TEST_EXE%\" -test.run=TestWindowsHookChildProcess$\r\n"
	if err := os.WriteFile(exe, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd, err := handlerCommand(ctx, exe, "admission-check")
	if err != nil {
		t.Fatal(err)
	}
	// Keep the wait open through the descendant's inherited output pipes.
	cmd.Stdout = &bytes.Buffer{}
	cmd.WaitDelay = 100 * time.Millisecond
	done := make(chan error, 1)
	go func() { done <- runHandler(ctx, cmd) }()
	var proc windows.Handle
	for proc == 0 {
		select {
		case err := <-done:
			t.Fatalf("handler exited before cancellation: %v", err)
		case <-ctx.Done():
			t.Fatal("child did not start within fixture deadline")
		case <-time.After(15 * time.Millisecond):
			pidFile, _ := os.ReadFile(ready)
			if pid, err := strconv.Atoi(string(pidFile)); err == nil {
				proc, err = windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid))
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	defer windows.CloseHandle(proc)
	defer windows.TerminateProcess(proc, 1)
	cancel()
	if result, err := windows.WaitForSingleObject(proc, 3000); err != nil || result != windows.WAIT_OBJECT_0 {
		t.Fatalf("child survived handler cancellation: wait=%d, error=%v", result, err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("handler cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("handler did not return after cancellation")
	}
}
