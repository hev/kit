//go:build windows

package nativejob

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsWorkloadFixture(t *testing.T) {
	role := os.Getenv("HEV_WORKLOAD_PROOF_ROLE")
	if role == "" {
		return
	}
	root := os.Getenv("HEV_WORKLOAD_PROOF_ROOT")
	if role == "leaf" {
		if err := os.WriteFile(filepath.Join(root, "leaf-started"), nil, 0600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Second)
		return
	}
	if role != "root" {
		t.Fatal("unknown owned fixture role")
	}
	child := exec.Command(os.Args[0], "-test.run=^TestWindowsWorkloadFixture$")
	child.Env = []string{"SYSTEMROOT=" + os.Getenv("SYSTEMROOT"), "HEV_WORKLOAD_PROOF_ROLE=leaf", "HEV_WORKLOAD_PROOF_ROOT=" + root}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	birth, err := processBirth(child.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(birth)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "child.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err = os.Stat(filepath.Join(root, "leaf-started")); err == nil {
			fmt.Println("original-owned-root")
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("own leaf startup timed out")
}
func windowsWorkloadProof(t *testing.T, role string) (*windowsWorkload, string, *os.File) {
	t.Helper()
	root := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.CreateTemp(root, "stdout-")
	if err != nil {
		t.Fatal(err)
	}
	errout, err := os.CreateTemp(root, "stderr-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { out.Close(); errout.Close() })
	cmd := exec.Command(exe, "-test.run=^TestWindowsWorkloadFixture$")
	cmd.Dir = root
	cmd.Env = []string{"SYSTEMROOT=" + os.Getenv("SYSTEMROOT"), "HEV_WORKLOAD_PROOF_ROLE=" + role, "HEV_WORKLOAD_PROOF_ROOT=" + root}
	cmd.Stdout = out
	cmd.Stderr = errout
	owned, err := startWindowsWorkload(cmd)
	if err != nil {
		t.Fatal("original suspended job start", err)
	}
	t.Cleanup(func() {
		if owned.job != 0 {
			windows.CloseHandle(owned.job)
		}
		if owned.process != 0 {
			windows.CloseHandle(owned.process)
		}
	})
	return owned, root, out
}
func TestWindowsWorkloadWaitProvesTreeBeforeResult(t *testing.T) {
	owned, root, out := windowsWorkloadProof(t, "root")
	var leaf Birth
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(filepath.Join(root, "child.json"))
		if err == nil && json.Unmarshal(data, &leaf) == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if leaf.PID == 0 {
		t.Fatal("owned descendant metadata missing")
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(leaf.PID))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	code, err := owned.Wait()
	if err != nil || code != 0 {
		t.Fatal("known root and tree terminal", code, err)
	}
	status, err := windows.WaitForSingleObject(handle, 1000)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatal("descendant survived result boundary", status, err)
	}
	if err = out.Sync(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out.Name())
	if err != nil || !strings.Contains(string(data), "original-owned-root") {
		t.Fatal("explicit artifact output absent", err)
	}
	if code2, err := owned.Wait(); err != nil || code2 != code {
		t.Fatal("owned wait not stable", code2, err)
	}
}
func TestWindowsWorkloadKillUsesOriginalJob(t *testing.T) {
	owned, _, _ := windowsWorkloadProof(t, "leaf")
	birth, err := owned.Birth()
	if err != nil {
		t.Fatal(err)
	}
	got, err := processBirth(birth.PID)
	if err != nil || got != birth {
		t.Fatal("original handle birth differs", got, birth, err)
	}
	if err = owned.Kill(); err != nil {
		t.Fatal(err)
	}
	code, err := owned.Wait()
	if err != nil || code == 0 {
		t.Fatal("owned kill/terminal", code, err)
	}
}

func TestWindowsWorkloadLostJobCannotReturnKnownResult(t *testing.T) {
	owned, _, _ := windowsWorkloadProof(t, "leaf")
	if err := windows.CloseHandle(owned.job); err != nil {
		t.Fatal(err)
	}
	owned.job = 0 // simulate losing the last owned job handle, as on owner death
	status, err := windows.WaitForSingleObject(owned.process, 10000)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatal("last job handle did not stop owned root", status, err)
	}
	if _, err = owned.Wait(); err == nil {
		t.Fatal("lost tree authority returned a known result")
	}
}
