//go:build windows

package windowsfs

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func proofRoot(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "private")
	if err := CreatePrivateDir(p, false); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestWindowsLockChild(t *testing.T) {
	root := os.Getenv("NATIVE_FS_LOCK_ROOT")
	if root == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	lock, err := Acquire(ctx, filepath.Join(root, "owner.lock"))
	if os.Getenv("NATIVE_FS_LOCK_EXPECT_HELD") == "1" {
		if !errors.Is(err, context.DeadlineExceeded) {
			if lock != nil {
				lock.Close()
			}
			t.Fatal("crossprocess exclusion failed", err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err = lock.Publish(ctx, filepath.Join(root, "state.json"), []byte("original-owner"), func() error { return nil }); err != nil {
		t.Fatal(err)
	}
}
func TestWindowsLockCrossProcessAndPublish(t *testing.T) {
	root := proofRoot(t)
	lock, err := Acquire(context.Background(), filepath.Join(root, "owner.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	child := func(held bool) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWindowsLockChild$")
		c.Env = []string{"SYSTEMROOT=" + os.Getenv("SYSTEMROOT"), "NATIVE_FS_LOCK_ROOT=" + root}
		if held {
			c.Env = append(c.Env, "NATIVE_FS_LOCK_EXPECT_HELD=1")
		}
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("own child %v: %v %s", held, err, out)
		}
	}
	child(true)
	if err = lock.Close(); err != nil {
		t.Fatal(err)
	}
	child(false)
	data, err := os.ReadFile(filepath.Join(root, "state.json"))
	if err != nil || string(data) != "original-owner" {
		t.Fatal(string(data), err)
	}
	if err = VerifyPrivate(filepath.Join(root, "state.json"), true, false); err != nil {
		t.Fatal(err)
	}
}
func TestWindowsPublishFinalCheckAndClosedOwnership(t *testing.T) {
	root := proofRoot(t)
	target := filepath.Join(root, "state.json")
	lock, err := Acquire(context.Background(), filepath.Join(root, "owner.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err = lock.Publish(context.Background(), target, []byte("before"), func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	denied := errors.New("authority expired")
	calls := 0
	if err = lock.Publish(context.Background(), target, []byte("after"), func() error { calls++; return denied }); !errors.Is(err, denied) || calls != 1 {
		t.Fatal(calls, err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "before" {
		t.Fatal("denied mutation", string(data), err)
	}
	if err = lock.Publish(context.Background(), filepath.Join(t.TempDir(), "elsewhere"), nil, func() error { return nil }); err == nil {
		t.Fatal("crossdirectory publish")
	}
	if err = lock.Close(); err != nil {
		t.Fatal(err)
	}
	if err = lock.Publish(context.Background(), target, nil, func() error { return nil }); err == nil {
		t.Fatal("closed owner publish")
	}
}

func TestWindowsAcquireExistingNeverCreates(t *testing.T) {
	root := proofRoot(t)
	path := filepath.Join(root, "missing.lock")
	if lock, err := AcquireExisting(context.Background(), path); err == nil {
		lock.Close()
		t.Fatal("missing ownership lock created")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("existing-only lock mutated filesystem", err)
	}
	lock, err := Acquire(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if other, err := AcquireExisting(ctx, path); !errors.Is(err, context.DeadlineExceeded) {
		if other != nil {
			other.Close()
		}
		t.Fatal("existing handle exclusion", err)
	}
}

func TestWindowsTrustedReadUsesCheckedHandle(t *testing.T) {
	root := proofRoot(t)
	path := filepath.Join(root, "pinned.exe")
	f, err := CreatePrivateFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString("MZ-owned-proof"); err != nil {
		t.Fatal(err)
	}
	if err = errors.Join(f.Sync(), f.Close()); err != nil {
		t.Fatal(err)
	}
	file, err := OpenTrusted(path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil || string(data) != "MZ-owned-proof" {
		t.Fatal(string(data), err)
	}
}
