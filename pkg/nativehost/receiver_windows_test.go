//go:build windows

package nativehost

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	n "github.com/hev/kit/pkg/nativejob"
	"golang.org/x/sys/windows"
)

func TestWindowsReceiverFixture(t *testing.T) {
	role := os.Getenv("HEV_RECEIVER_PROOF_ROLE")
	if role == "" {
		return
	}
	root := os.Getenv("HEV_RECEIVER_PROOF_ROOT")
	if role == "leaf" {
		if err := os.WriteFile(filepath.Join(root, "leaf-started"), nil, 0600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Second)
		return
	}
	if role != "root" {
		t.Fatal("unknown fixture role")
	}
	child := exec.Command(os.Args[0], "-test.run=^TestWindowsReceiverFixture$")
	child.Env = []string{"SYSTEMROOT=" + os.Getenv("SYSTEMROOT"), "HEV_RECEIVER_PROOF_ROLE=leaf", "HEV_RECEIVER_PROOF_ROOT=" + root}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	birth, err := n.ProcessBirth(child.Process.Pid)
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
		if _, err := os.Stat(filepath.Join(root, "leaf-started")); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("own leaf startup timed out")
}

func TestWindowsReceiverOriginalHandlesAndTree(t *testing.T) {
	root := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	owned, err := startWindowsOwned(exe, []string{"-test.run=^TestWindowsReceiverFixture$"}, root, []string{"SYSTEMROOT=" + os.Getenv("SYSTEMROOT"), "HEV_RECEIVER_PROOF_ROLE=root", "HEV_RECEIVER_PROOF_ROOT=" + root})
	if err != nil {
		t.Fatal("kernel job-list/suspended original-handle start", err)
	}
	// If any assertion fails, close only this test's retained job and process.
	defer func() {
		if owned.job != 0 {
			windows.CloseHandle(owned.job)
		}
		if owned.process != 0 {
			windows.CloseHandle(owned.process)
		}
	}()
	deadline := time.Now().Add(10 * time.Second)
	var birth n.Birth
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(filepath.Join(root, "child.json"))
		if err == nil && json.Unmarshal(data, &birth) == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if birth.PID == 0 {
		t.Fatal("own child metadata missing")
	}
	active, err := windowsJobActive(owned.job)
	if err != nil || active < 1 {
		t.Fatal("job membership absent", active, err)
	}
	actual, err := n.ProcessBirth(birth.PID)
	if err != nil || actual != birth {
		t.Fatal("original child birth mismatch", actual, birth, err)
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(birth.PID))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	if err = owned.wait(); err != nil {
		t.Fatal("owned Wait/tree proof", err)
	}
	// wait already closed these original handles only after kernel active==0.
	owned.process = 0
	owned.job = 0
	status, err := windows.WaitForSingleObject(handle, 1000)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatal("descendant survived root completion", status, err)
	}
}

func TestWindowsReceiverLastJobHandleClosesOwnedRoot(t *testing.T) {
	root := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	owned, err := startWindowsOwned(exe, []string{"-test.run=^TestWindowsReceiverFixture$"}, root, []string{"SYSTEMROOT=" + os.Getenv("SYSTEMROOT"), "HEV_RECEIVER_PROOF_ROLE=leaf", "HEV_RECEIVER_PROOF_ROOT=" + root})
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(owned.process)
	if err = windows.CloseHandle(owned.job); err != nil {
		t.Fatal(err)
	}
	status, err := windows.WaitForSingleObject(owned.process, 10000)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatal("kill-on-last-job-handle failed", status, err)
	}
}

func TestWindowsReceiverEnvironmentRejectsAliases(t *testing.T) {
	for _, env := range [][]string{{"PATH=a", "Path=b"}, {"invalid"}, {"A=bad\x00value"}, {"=hidden"}} {
		if _, err := windowsEnvironment(env); err == nil {
			t.Fatal("unsafe environment", env)
		}
	}
}
