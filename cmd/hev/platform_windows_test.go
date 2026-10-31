//go:build windows

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestWindowsServiceRegistration(t *testing.T) {
	if os.Getenv("HEV_WINDOWS_SERVICE_TEST") != "1" {
		t.Skip("set HEV_WINDOWS_SERVICE_TEST=1 on a disposable native host")
	}
	exists, _, err := queryWindowsTask()
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Skip("existing hev task: refusing to overwrite it in a test")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "hev.exe")
	buildCtx, buildCancel := context.WithTimeout(context.Background(), time.Minute)
	defer buildCancel()
	if out, err := exec.CommandContext(buildCtx, "go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build native CLI: %v\n%s", err, out)
	}
	config := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(config, []byte("[layer]\nendpoint='http://127.0.0.1:1'\napi_key='test'\nnamespace='test'\n[capture]\nscan_interval='1m'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_CONFIG", config)
	run := func(action string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, binary, "daemon", action).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", action, err, out)
		}
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, binary, "daemon", "uninstall").CombinedOutput()
		if err != nil {
			t.Errorf("cleanup: %v\n%s", err, out)
			// Best-effort bounded fallback even when CLI cleanup fails.
			_, _ = scheduler("/Change", "/TN", windowsTaskName, "/DISABLE")
			_, _ = scheduler("/End", "/TN", windowsTaskName)
			_, _ = scheduler("/Delete", "/TN", windowsTaskName, "/F")
		}
	}()
	run("install")
	deadline := time.Now().Add(15 * time.Second)
	for {
		exists, running, err := queryWindowsTask()
		if err != nil {
			t.Fatal(err)
		}
		if exists && running && len(nativeDaemonPIDs()) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("task did not publish a running daemon")
		}
		time.Sleep(100 * time.Millisecond)
	}
	run("status")
	run("stop")
	exists, running, err := queryWindowsTask()
	if err != nil || !exists || running || len(nativeDaemonPIDs()) != 0 {
		t.Fatalf("stop not confirmed: exists=%v running=%v err=%v", exists, running, err)
	}
	run("uninstall")
	exists, _, err = queryWindowsTask()
	if err != nil || exists {
		t.Fatalf("unregistration not confirmed: %v %v", exists, err)
	}
}
