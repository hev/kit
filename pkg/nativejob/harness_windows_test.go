//go:build windows

package nativejob

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	fs "github.com/hev/kit/pkg/nativefs/windowsfs"
	"golang.org/x/sys/windows"
)

func TestWindowsOwnedHarnessFixture(t *testing.T) {
	root := os.Getenv("HEV_HARNESS_PROOF_ROOT")
	if root == "" {
		return
	}
	prompt, err := io.ReadAll(os.Stdin)
	if err != nil {
		t.Fatal(err)
	}
	if string(prompt) != "owned-prompt" {
		t.Fatal("prompt stdin differs")
	}
	raw, err := strconv.ParseUint(os.Getenv("HEV_HARNESS_LEASE_HANDLE"), 10, 64)
	if err != nil || raw == 0 {
		t.Fatal("lease handle absent", err)
	}
	lease := os.NewFile(uintptr(raw), "own-harness-lease")
	defer lease.Close()
	if err = fs.VerifyPrivateHandle(windows.Handle(raw), true, false); err != nil {
		t.Fatal(err)
	}
	if _, err = lease.Write([]byte("forbidden")); err == nil {
		t.Fatal("inherited lease granted write")
	}
	fmt.Printf("stream:%s\n", prompt)
	fmt.Fprintln(os.Stderr, "stderr-record")
	if err = os.WriteFile(filepath.Join(root, "ready"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err = os.Stat(filepath.Join(root, "release")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owned harness release timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err = os.Stdout.Write(bytes.Repeat([]byte("x"), 1<<20)); err != nil {
		t.Fatal(err)
	}
}
func TestWindowsOwnedHarnessStreamsAndReadonlyLease(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if err := fs.CreatePrivateDir(root, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "use.lease")
	seed, err := fs.CreatePrivateFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = errors.Join(seed.Sync(), seed.Close(), fs.SyncDirectory(root)); err != nil {
		t.Fatal(err)
	}
	lease, err := fs.OpenFileLease(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	childFile, err := lease.DuplicateForChild()
	if err != nil {
		t.Fatal(err)
	}
	defer childFile.Close()
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer inR.Close()
	defer inW.Close()
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer outR.Close()
	defer outW.Close()
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer errR.Close()
	defer errW.Close()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestWindowsOwnedHarnessFixture$")
	cmd.Dir = root
	cmd.Env = []string{"SYSTEMROOT=" + os.Getenv("SYSTEMROOT"), "HEV_HARNESS_PROOF_ROOT=" + root, "HEV_HARNESS_LEASE_HANDLE=" + strconv.FormatUint(uint64(childFile.Fd()), 10)}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = inR, outW, errW
	cmd.ExtraFiles = []*os.File{childFile}
	child, err := StartOwnedHarness(cmd)
	if child != nil {
		t.Cleanup(func() { _ = child.Kill(); _, _ = child.Wait() })
	}
	if err != nil {
		t.Fatal("owned stream start", err)
	}
	if cmd.Process != nil || cmd.ProcessState != nil {
		t.Fatal("facade adopted a PID into exec.Cmd")
	}
	birth, err := child.Birth()
	if err != nil || birth.PID == 0 {
		t.Fatal(birth, err)
	}
	if _, err = readonlyHarnessFiles([]*os.File{childFile}); err != nil {
		t.Fatal("parent inheritance flag not restored", err)
	}
	if err = errors.Join(inR.Close(), outW.Close(), errW.Close(), childFile.Close(), lease.Close()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = child.Kill() })
	defer stop()
	var stdout, stderr bytes.Buffer
	copied := make(chan error, 2)
	go func() { _, e := io.Copy(&stdout, outR); copied <- e }()
	go func() { _, e := io.Copy(&stderr, errR); copied <- e }()
	if _, err = inW.Write([]byte("owned-prompt")); err != nil {
		t.Fatal(err)
	}
	if err = inW.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err = os.Stat(filepath.Join(root, "ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owned harness did not read stdin")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if publisher, e := fs.OpenFileLease(path, false); !errors.Is(e, fs.ErrLeaseBusy) {
		if publisher != nil {
			publisher.Close()
		}
		t.Fatal("parent close lost child lease", e)
	}
	if err = os.WriteFile(filepath.Join(root, "release"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	code, err := child.Wait()
	if err != nil || code != 0 {
		t.Fatal("owned harness tree terminal", code, err)
	}
	for i := 0; i < 2; i++ {
		if e := <-copied; e != nil {
			t.Fatal("stream drain", e)
		}
	}
	if !strings.HasPrefix(stdout.String(), "stream:owned-prompt\n") || bytes.Count(stdout.Bytes(), []byte("x")) != 1<<20 || stderr.String() != "stderr-record\n" {
		t.Fatal("streamed stdio differs", stdout.Len(), stderr.Len())
	}
	publisher, err := fs.OpenFileLease(path, false)
	if err != nil {
		t.Fatal("last child close retained stale lease", err)
	}
	if err = publisher.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestWindowsOwnedHarnessRejectsWritableInheritedFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if err := fs.CreatePrivateDir(root, false); err != nil {
		t.Fatal(err)
	}
	file, err := fs.CreatePrivateFile(filepath.Join(root, "writable"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestWindowsOwnedHarnessFixture$")
	cmd.Dir = root
	cmd.Env = []string{"SYSTEMROOT=" + os.Getenv("SYSTEMROOT")}
	cmd.ExtraFiles = []*os.File{file}
	child, err := StartOwnedHarness(cmd)
	if child != nil {
		child.Kill()
		child.Wait()
		t.Fatal("writable inheritance produced an owned effect")
	}
	if !errors.Is(err, ErrOwnedCommandUnknown) {
		t.Fatal("writable file not refused", err)
	}
}

func TestWindowsOwnedHarnessInsideOwnedWorkload(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if err := fs.CreatePrivateDir(root, false); err != nil {
		t.Fatal(err)
	}
	out, err := fs.CreatePrivateFile(filepath.Join(root, "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	errout, err := fs.CreatePrivateFile(filepath.Join(root, "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	defer errout.Close()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestWindowsOwnedHarnessStreamsAndReadonlyLease$")
	cmd.Dir = root
	cmd.Env = []string{"SYSTEMROOT=" + os.Getenv("SYSTEMROOT"), "TEMP=" + root, "TMP=" + root}
	cmd.Stdout = out
	cmd.Stderr = errout
	outer, err := startWindowsWorkload(cmd)
	if outer != nil {
		t.Cleanup(func() { outer.Kill(); outer.Wait() })
	}
	if err != nil {
		t.Fatal("outer original workload", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { outer.Kill() })
	defer stop()
	code, err := outer.Wait()
	if err != nil || code != 0 {
		data, _ := os.ReadFile(out.Name())
		diagnostic, _ := os.ReadFile(errout.Name())
		t.Fatalf("nested original-handle harness: exit=%d err=%v stdout=%s stderr=%s", code, err, data, diagnostic)
	}
}
