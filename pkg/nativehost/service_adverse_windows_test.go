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
)

// Deliberately leave a child in the original nonbreakaway service Job after its
// original host exits. No task manager, signal-by-PID or production hook is used.
func TestWindowsServiceAdverseFixture(t *testing.T) {
	role := os.Getenv("HEV_SERVICE_ADVERSE_ROLE")
	if role == "" {
		return
	}
	root := os.Getenv("HEV_SERVICE_ADVERSE_ROOT")
	switch role {
	case "leaf":
		serviceEvent(t, root, "leaf-started", true)
		serviceWaitFile(t, filepath.Join(root, "leaf-stop"))
	case "host":
		birth, e := n.ProcessBirth(os.Getpid())
		if e != nil {
			t.Fatal(e)
		}
		closeService, e := platformEnterService(filepath.Join(root, "config", "host.json"), birth)
		if e != nil {
			t.Fatal(e)
		}
		defer func() {
			if e := closeService(); e != nil {
				t.Error(e)
			}
		}()
		serviceEvent(t, root, "host-birth", birth)
		serviceWaitFile(t, filepath.Join(root, "spawn"))
		child := exec.Command(os.Args[0], "-test.run=^TestWindowsServiceAdverseFixture$")
		child.Env = []string{"SYSTEMROOT=" + os.Getenv("SYSTEMROOT"), "HEV_SERVICE_ADVERSE_ROLE=leaf", "HEV_SERVICE_ADVERSE_ROOT=" + root}
		if e := child.Start(); e != nil {
			t.Fatal(e)
		}
		childBirth, e := n.ProcessBirth(child.Process.Pid)
		if e != nil {
			t.Fatal(e)
		}
		serviceEvent(t, root, "leaf-birth", childBirth)
		serviceWaitFile(t, filepath.Join(root, "leaf-started"))
		// Release only this fixture's original child process handle, never kill it.
		if e := child.Process.Release(); e != nil {
			t.Fatal(e)
		}
		serviceWaitFile(t, filepath.Join(root, "host-exit"))
	default:
		t.Fatal("unknown adverse fixture role")
	}
}

func TestWindowsServiceWitnessHostDeadChildLive(t *testing.T) {
	session, e := windowsUserSession()
	if !supported() || e != nil || session != 2 {
		t.Skip("requires actual Windows26300 existing Session2; skip is not acceptance")
	}
	root := filepath.Join(t.TempDir(), "adverse-service")
	for _, path := range []string{root, filepath.Join(root, "bin"), filepath.Join(root, "config")} {
		if e := fs.CreatePrivateDir(path, false); e != nil {
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
	if e := errors.Join(copyErr, src.Close(), dst.Sync(), dst.Close(), fs.SyncDirectory(filepath.Join(root, "bin"))); e != nil {
		t.Fatal(e)
	}
	host := exec.Command(filepath.Join(root, "bin", "hev.exe"), "-test.run=^TestWindowsServiceAdverseFixture$")
	host.Env = []string{"SYSTEMROOT=" + os.Getenv("SYSTEMROOT"), "HEV_SERVICE_ADVERSE_ROLE=host", "HEV_SERVICE_ADVERSE_ROOT=" + root}
	if e := host.Start(); e != nil {
		t.Fatal(e)
	}
	hostDone := make(chan error, 1)
	go func() { hostDone <- host.Wait() }()
	hostJoined := false
	t.Cleanup(func() {
		// End only these owned fixtures, including on a failed assertion. Publish is
		// exclusive; already-existing stop events are left unchanged.
		for _, name := range []string{"leaf-stop", "host-exit", "spawn"} {
			if e := fs.PublishExclusive(filepath.Join(root, name), []byte("true")); e != nil && !errors.Is(e, os.ErrExist) {
				t.Error(e)
			}
		}
		if !hostJoined {
			select {
			case <-hostDone:
			case <-time.After(10 * time.Second):
				_ = host.Process.Kill()
				select {
				case <-hostDone:
				case <-time.After(time.Second):
					t.Error("owned host cleanup uncertain")
				}
			}
		}
	})
	readBirth := func(name string) n.Birth {
		serviceWaitFile(t, filepath.Join(root, name))
		raw, e := os.ReadFile(filepath.Join(root, name))
		if e != nil {
			t.Fatal(e)
		}
		var birth n.Birth
		if json.Unmarshal(raw, &birth) != nil {
			t.Fatal("malformed fixture birth")
		}
		return birth
	}
	birth := readBirth("host-birth")
	witness, e := OpenServiceWitness(birth, filepath.Join(root, "config", "host.json"))
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := witness.Close(); e != nil {
			t.Error(e)
		}
	}()
	serviceEvent(t, root, "spawn", true)
	leaf := readBirth("leaf-birth")
	serviceWaitFile(t, filepath.Join(root, "leaf-started"))
	serviceEvent(t, root, "host-exit", true)
	select {
	case e := <-hostDone:
		hostJoined = true
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("original fixture host did not exit")
	}
	if _, e := ObserveService(birth, filepath.Join(root, "config", "host.json")); e == nil {
		t.Fatal("dead root accepted as live service")
	}
	observed, e := witness.Observe()
	if e != nil || observed.HostBirth != birth || len(observed.ServiceMembers) != 1 || observed.ServiceMembers[0] != leaf {
		t.Fatal("retained Job lost live orphan", observed, e)
	}
	if actual, e := n.ProcessBirth(leaf.PID); e != nil || actual != leaf {
		t.Fatal("child was not actually alive", actual, e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	e = witness.WaitEmpty(ctx)
	cancel()
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("host exit released drain despite live child", e)
	}
	serviceEvent(t, root, "leaf-stop", true)
	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	e = witness.WaitEmpty(ctx)
	cancel()
	if e != nil {
		t.Fatal("original Job never emptied after owned child exit", e)
	}
	final, e := witness.Observe()
	if e != nil || final.HostBirth != birth || len(final.ServiceMembers) != 0 {
		t.Fatal(final, e)
	}
	if e := witness.Close(); e != nil {
		t.Fatal(e)
	}
	if e := witness.Close(); e != nil {
		t.Fatal("non-idempotent close", e)
	}
	if _, e := witness.Observe(); e == nil {
		t.Fatal("closed witness reacquired old host")
	}
}
