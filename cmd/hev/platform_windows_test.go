//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWindowsServiceRegistration(t *testing.T) {
	if os.Getenv("HEV_WINDOWS_SERVICE_TEST") != "1" {
		t.Skip("set HEV_WINDOWS_SERVICE_TEST=1 on a disposable native host")
	}
	if _, err := scheduler("/Query", "/TN", windowsTaskName); err == nil {
		t.Skip("existing hev task: refusing to overwrite it in a test")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "hev.exe")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build native CLI: %v\n%s", err, out)
	}
	config := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(config, []byte("[layer]\nendpoint='http://127.0.0.1:1'\napi_key='test'\nnamespace='test'\n[capture]\nscan_interval='1m'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_CONFIG", config)
	run := func(action string) {
		t.Helper()
		out, err := exec.Command(binary, "daemon", action).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", action, err, out)
		}
	}
	defer func() {
		out, err := exec.Command(binary, "daemon", "uninstall").CombinedOutput()
		if err != nil {
			t.Errorf("cleanup: %v\n%s", err, out)
		}
	}()
	run("install")
	run("status")
	run("stop")
	run("uninstall")
}
