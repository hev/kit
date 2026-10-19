//go:build windows

package nativehost

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	fs "github.com/hev/kit/pkg/nativefs/windowsfs"
	n "github.com/hev/kit/pkg/nativejob"
	"golang.org/x/sys/windows"
)

func serviceWaitFile(t *testing.T, path string) {
	t.Helper()
	until := time.Now().Add(8 * time.Second)
	for time.Now().Before(until) {
		if _, e := os.Stat(path); e == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("owned fixture event timeout", path)
}
func serviceEvent(t *testing.T, root, name string, v any) {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	if e = fs.PublishExclusive(filepath.Join(root, name), b); e != nil {
		t.Fatal(e)
	}
}
func TestWindowsServiceFixture(t *testing.T) {
	role := os.Getenv("HEV_SERVICE_PROOF_ROLE")
	if role == "" {
		return
	}
	root := os.Getenv("HEV_SERVICE_PROOF_ROOT")
	switch role {
	case "leaf":
		serviceEvent(t, root, "leaf-started", true)
		serviceWaitFile(t, filepath.Join(root, "leaf-stop"))
		return
	case "intermediate":
		c := exec.Command(os.Args[0], "-test.run=^TestWindowsServiceFixture$")
		c.Env = []string{"SYSTEMROOT=" + os.Getenv("SYSTEMROOT"), "HEV_SERVICE_PROOF_ROLE=leaf", "HEV_SERVICE_PROOF_ROOT=" + root}
		if e := c.Start(); e != nil {
			t.Fatal(e)
		}
		birth, e := n.ProcessBirth(c.Process.Pid)
		if e != nil {
			t.Fatal(e)
		}
		serviceEvent(t, root, "leaf-birth", birth)
		serviceWaitFile(t, filepath.Join(root, "leaf-started"))
		return // deliberate intermediate exit, leaf remains in the kernel Job
	case "service":
		birth, e := n.ProcessBirth(os.Getpid())
		if e != nil {
			t.Fatal(e)
		}
		release, e := platformEnterService(filepath.Join(root, "config", "host.json"), birth)
		if e != nil {
			t.Fatal(e)
		}
		defer func() {
			if e := release(); e != nil {
				t.Error(e)
			}
		}()
		serviceEvent(t, root, "service-birth", birth)
		serviceWaitFile(t, filepath.Join(root, "spawn"))
		owned, e := startWindowsOwned(os.Args[0], []string{"-test.run=^TestWindowsServiceFixture$"}, root, []string{"SYSTEMROOT=" + os.Getenv("SYSTEMROOT"), "HEV_SERVICE_PROOF_ROLE=intermediate", "HEV_SERVICE_PROOF_ROOT=" + root})
		if e != nil {
			t.Fatal(e)
		}
		serviceWaitFile(t, filepath.Join(root, "leaf-started"))
		until := time.Now().Add(5 * time.Second)
		exited := false
		for time.Now().Before(until) {
			state, e := windows.WaitForSingleObject(owned.process, 0)
			if e != nil {
				t.Fatal(e)
			}
			if state == uint32(windows.WAIT_OBJECT_0) {
				exited = true
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if !exited {
			t.Fatal("owned intermediate did not exit")
		}
		serviceEvent(t, root, "intermediate-exited", true)
		serviceWaitFile(t, filepath.Join(root, "leaf-stop"))
		if e = owned.wait(); e != nil {
			t.Fatal(e)
		}
		serviceEvent(t, root, "nested-empty", true)
		serviceWaitFile(t, filepath.Join(root, "service-stop"))
		return
	default:
		t.Fatal("unknown owned service fixture")
	}
}

func TestWindowsServiceKernelWholeTree(t *testing.T) {
	session, e := windowsUserSession()
	if !supported() || e != nil || session != 2 {
		// Unsupported build/session is a real refusal, not synthetic proof.
		if release, e := platformEnterService(`C:\not-created\host.json`, n.Birth{BootID: "x", PID: os.Getpid(), Start: "x"}); e == nil || release != nil {
			t.Fatal("unsupported service mutation")
		}
		t.Skip("whole-tree acceptance requires actual Windows26300 existingSession2")
	}
	root := filepath.Join(t.TempDir(), "service")
	for _, p := range []string{root, filepath.Join(root, "bin"), filepath.Join(root, "config")} {
		if e = fs.CreatePrivateDir(p, false); e != nil {
			t.Fatal(e)
		}
	}
	windowsWritePrivate(t, filepath.Join(root, "config", "host.json"), []byte("{}\n"))
	src, e := os.Open(os.Args[0])
	if e != nil {
		t.Fatal(e)
	}
	dst, e := fs.CreatePrivateFile(filepath.Join(root, "bin", "hev.exe"))
	if e != nil {
		src.Close()
		t.Fatal(e)
	}
	_, copyErr := io.Copy(dst, src)
	if e = errors.Join(copyErr, src.Close(), dst.Sync(), dst.Close(), fs.SyncDirectory(filepath.Join(root, "bin"))); e != nil {
		t.Fatal(e)
	}
	c := exec.Command(filepath.Join(root, "bin", "hev.exe"), "-test.run=^TestWindowsServiceFixture$")
	c.Env = []string{"SYSTEMROOT=" + os.Getenv("SYSTEMROOT"), "HEV_SERVICE_PROOF_ROLE=service", "HEV_SERVICE_PROOF_ROOT=" + root}
	if e = c.Start(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if c.ProcessState == nil {
			_ = c.Process.Kill()
			_ = c.Wait()
		}
	}) // original owned exec handle only; never a discovered PID
	serviceWaitFile(t, filepath.Join(root, "service-birth"))
	b, e := os.ReadFile(filepath.Join(root, "service-birth"))
	if e != nil {
		t.Fatal(e)
	}
	var birth n.Birth
	if json.Unmarshal(b, &birth) != nil {
		t.Fatal("birth")
	}
	path := filepath.Join(root, "config", "host.json")
	o, e := ObserveService(birth, path)
	if e != nil || len(o.ServiceMembers) != 1 || o.ServiceMembers[0] != birth {
		t.Fatal(o, e)
	}
	witness, e := OpenServiceWitness(birth, path)
	if e != nil {
		t.Fatal(e)
	}
	defer witness.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	if e := witness.WaitEmpty(ctx); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("live service considered drained", e)
	}
	cancel()
	wrong := birth
	wrong.Start += "1"
	if _, e = ObserveService(wrong, path); e == nil {
		t.Fatal("stale original birth accepted")
	}
	serviceEvent(t, root, "spawn", true)
	serviceWaitFile(t, filepath.Join(root, "leaf-started"))
	serviceWaitFile(t, filepath.Join(root, "intermediate-exited"))
	leafRaw, e := os.ReadFile(filepath.Join(root, "leaf-birth"))
	if e != nil {
		t.Fatal(e)
	}
	var leaf n.Birth
	if json.Unmarshal(leafRaw, &leaf) != nil {
		t.Fatal("leaf birth")
	}
	deadline := time.Now().Add(5 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		o, e = ObserveService(birth, path)
		if e == nil && len(o.ServiceMembers) >= 2 {
			for _, member := range o.ServiceMembers {
				if member == leaf {
					found = true
				}
			}
		}
		if found {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !found {
		t.Fatal("kernel observer lost nested/orphan child", o, e)
	}
	serviceEvent(t, root, "leaf-stop", true)
	serviceWaitFile(t, filepath.Join(root, "nested-empty"))
	o, e = ObserveService(birth, path)
	if e != nil || len(o.ServiceMembers) != 1 || o.ServiceMembers[0] != birth {
		t.Fatal("nested tree not empty", o, e)
	}
	serviceEvent(t, root, "service-stop", true)
	if e = c.Wait(); e != nil {
		t.Fatal(e)
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	if e := witness.WaitEmpty(ctx); e != nil {
		t.Fatal("retained original Job did not drain", e)
	}
	cancel()
	retained, e := witness.Observe()
	if e != nil || len(retained.ServiceMembers) != 0 || retained.HostBirth != birth {
		t.Fatal(retained, e)
	}
	if e := witness.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e := witness.Observe(); e == nil {
		t.Fatal("closed witness adopted historical PID")
	}
	if _, e = ObserveService(birth, path); e == nil {
		t.Fatal("root exit became a live service proof")
	}
}
