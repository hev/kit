//go:build windows

package nativehost

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hev/kit/internal/winpipe"
	fs "github.com/hev/kit/pkg/nativefs/windowsfs"
	n "github.com/hev/kit/pkg/nativejob"
)

func windowsWritePrivate(t *testing.T, path string, data []byte) {
	t.Helper()
	f, e := fs.CreatePrivateFile(path)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.Write(data)
	if e = errors.Join(e, f.Sync(), f.Close()); e != nil {
		t.Fatal(e)
	}
}
func TestWindowsHostCheckedConfigAndRoleGuard(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if e := fs.CreatePrivateDir(root, false); e != nil {
		t.Fatal(e)
	}
	receiver := filepath.Join(root, "receiver.json")
	body := []byte("{}\n")
	windowsWritePrivate(t, receiver, body)
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	bytes, e := os.ReadFile(exe)
	if e != nil {
		t.Fatal(e)
	}
	home, e := os.UserHomeDir()
	if e != nil {
		t.Fatal(e)
	}
	profile := Profile{ID: "windows-owned-proof", ConfigRef: receiver, ConfigDigest: hash(body), SupervisorRef: exe, SupervisorDigest: hash(bytes), StateRoot: root, Enrollment: n.Enrollment{HostID: "proof", ExecutorID: "proof"}, NodeUID: "node", Incarnation: "incarnation", AdmissionGeneration: 1, Runtime: Runtime{Directory: root, Environment: []string{"HOME=" + home, "USERPROFILE=" + home, "PATH=" + filepath.Dir(exe), "SYSTEMROOT=" + os.Getenv("SYSTEMROOT")}}}
	if e = profile.verify(); e != nil {
		t.Fatal("checked native executable/runtime profile", e)
	}
	path := filepath.Join(root, "host.json")
	name, e := winpipe.Name(path)
	if e != nil {
		t.Fatal(e)
	}
	config := Config{Version: Version, Socket: name, Profiles: []Profile{profile}}
	data, e := json.Marshal(config)
	if e != nil {
		t.Fatal(e)
	}
	windowsWritePrivate(t, path, data)
	got, e := VerifyConfig(path)
	if e != nil || got.Socket != name {
		t.Fatal(got, e)
	}
	f, e := openPrivate(receiver)
	if e != nil {
		t.Fatal(e)
	}
	read, e := io.ReadAll(f)
	e = errors.Join(e, f.Close())
	if e != nil || string(read) != string(body) {
		t.Fatal(string(read), e)
	}
	if e = RunConfig(context.Background(), path); !errors.Is(e, ErrHeld) {
		t.Fatal("unaccepted Windows role enabled", e)
	}
	if _, e = os.Stat(filepath.Join(root, "host.lock")); !os.IsNotExist(e) {
		t.Fatal("disabled role mutated endpoint ownership", e)
	}
	// Test the actual isolated endpoint hook without granting the public role.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	listener, e := openEndpoint(ctx, path, config)
	if _, sessionErr := windowsUserSession(); sessionErr != nil {
		if !errors.Is(e, ErrHeld) || listener != nil {
			t.Fatal("session-zero server accepted", e)
		}
		if _, statErr := os.Stat(filepath.Join(root, "host.lock")); !os.IsNotExist(statErr) {
			t.Fatal("session-zero server mutated lock", statErr)
		}
		return
	}
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	if e = verifyListener(listener); e != nil {
		t.Fatal(e)
	}
}
func TestWindowsHostRuntimeAliasesAndScriptsHold(t *testing.T) {
	if e := platformRuntime(Runtime{Directory: t.TempDir(), Environment: []string{"PATH=x", "Path=y"}}); e == nil {
		t.Fatal("ambiguous Windows runtime accepted")
	}
	root := filepath.Join(t.TempDir(), "private")
	if e := fs.CreatePrivateDir(root, false); e != nil {
		t.Fatal(e)
	}
	script := filepath.Join(root, "supervisor.ps1")
	windowsWritePrivate(t, script, []byte("Write-Output forbidden"))
	if f, e := openPinned(script, true); e == nil {
		f.Close()
		t.Fatal("script silently selected as native supervisor")
	}
}
