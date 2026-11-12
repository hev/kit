package daemon

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/internal/storagemode"
	"github.com/hev/kit/internal/trace"
)

// noArchive fails when anything exists at the archive root: a command that
// found no key must not have used or created a file archive.
func noArchive(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("archive root %s exists (err=%v): a file archive was created", root, err)
	}
}

// wantNoKey checks that both opens refuse with the machine-is-set-up-for-
// Layer message, name the reason and the fix, and leave no archive.
func wantNoKey(t *testing.T, cfg *Config, root string, reason string) *storagemode.NoKeyError {
	t.Helper()
	var last *storagemode.NoKeyError
	for name, open := range map[string]func(*Config, string) (Target, error){"OpenTarget": OpenTarget, "OpenTargetRead": OpenTargetRead} {
		target, err := open(cfg, "")
		var nk *storagemode.NoKeyError
		if !errors.As(err, &nk) {
			t.Fatalf("%s: target=%+v err=%v, want a NoKeyError", name, target, err)
		}
		if target.Files != nil {
			t.Fatalf("%s: returned a file archive", name)
		}
		msg := err.Error()
		for _, want := range []string{"set up for hev layer", reason, "HEVLAYER_API_KEY", "[layer].api_key", "remove endpoint and namespace"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("%s: message lacks %q:\n%s", name, want, msg)
			}
		}
		last = nk
	}
	noArchive(t, root)
	return last
}

func TestLayerEndpointWithNoKeyRefusesFiles(t *testing.T) {
	cfg, root := isolate(t, "[layer]\nendpoint = \"https://gateway.example\"\napi_key = \"\"\n")
	nk := wantNoKey(t, cfg, root, `[layer] endpoint = "https://gateway.example"`)
	if nk.Cause != nil || !strings.Contains(nk.Error(), "are not set in this environment") {
		t.Fatalf("why no key: %v", nk)
	}
}

func TestLayerNamespaceWithNoKeyRefusesFiles(t *testing.T) {
	cfg, root := isolate(t, "[layer]\nnamespace = \"team\"\n")
	wantNoKey(t, cfg, root, `[layer] namespace = "team"`)
}

// The stock config a 0.6.x `hev daemon install` wrote: hosted endpoint,
// default namespace, empty key. On such a machine the key lived in the
// daemon's service environment.
func TestStockLayerConfigFromOlderInstallRefusesFiles(t *testing.T) {
	cfg, root := isolate(t, "[layer]\nendpoint = \"https://aws-us-east-1.hevlayer.com\"\napi_key = \"\"\nnamespace = \"hev-traces\"\n\n[capture]\nscan_interval = \"5m\"\n")
	wantNoKey(t, cfg, root, "names a Layer endpoint")
}

func TestLayerEndpointInEnvironmentRefusesFiles(t *testing.T) {
	cfg, root := isolate(t, "")
	t.Setenv("HEVLAYER_ENDPOINT", "https://gateway.example")
	cfg, _ = LoadConfig()
	wantNoKey(t, cfg, root, "HEVLAYER_ENDPOINT is set")
}

func TestUnreadableKeyReferenceRefusesFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake op is a shell script")
	}
	cfg, root := isolate(t, "")
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "op"), []byte("#!/bin/sh\necho denied >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("HEVLAYER_API_KEY_OP_REF", "op://vault/item/field")
	nk := wantNoKey(t, cfg, root, "HEVLAYER_API_KEY_OP_REF is set")
	if nk.Cause == nil || !strings.Contains(nk.Error(), "could not be read") {
		t.Fatalf("cause not reported: %v", nk)
	}
	// With no op at all, the same.
	t.Setenv("PATH", t.TempDir())
	wantNoKey(t, cfg, root, "HEVLAYER_API_KEY_OP_REF is set")
}

// fakeDaemon holds daemon.lock as hevd does and writes its storage record.
func fakeDaemon(t *testing.T, r StorageRecord) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(DaemonLockPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	release, err := platformops.LockPath(DaemonLockPath(), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	if err := writeStorageRecord(r); err != nil {
		t.Fatal(err)
	}
}

func TestRunningLayerDaemonRefusesFiles(t *testing.T) {
	cfg, root := isolate(t, "")
	fakeDaemon(t, StorageRecord{PID: 4242, Mode: storagemode.Layer, Namespace: "team", Endpoint: "https://gateway.example", KeySource: "HEVLAYER_API_KEY"})
	nk := wantNoKey(t, cfg, root, "capture daemon on this machine (pid 4242) is writing to hev layer namespace team")
	if !strings.Contains(nk.Error(), "service unit") {
		t.Fatalf("message does not explain the daemon's key: %v", nk)
	}
	st, err := RunningStorage()
	if err != nil || !st.Running || st.Record == nil || st.Record.Mode != storagemode.Layer || st.Record.PID != 4242 {
		t.Fatalf("RunningStorage = %+v, %v", st, err)
	}
}

func TestStaleDaemonRecordIsIgnored(t *testing.T) {
	cfg, root := isolate(t, "")
	os.MkdirAll(filepath.Dir(DaemonLockPath()), 0o700)
	os.WriteFile(DaemonLockPath(), nil, 0o600)
	if err := writeStorageRecord(StorageRecord{PID: 4242, Mode: storagemode.Layer, Namespace: "team"}); err != nil {
		t.Fatal(err)
	}
	// Nobody holds the lock: the daemon that wrote this record is gone.
	st, err := RunningStorage()
	if err != nil || st.Running || st.Record != nil {
		t.Fatalf("RunningStorage = %+v, %v", st, err)
	}
	target, err := OpenTargetRead(cfg, "")
	if err != nil || target.Mode != storagemode.Filesystem {
		t.Fatalf("target = %+v, %v", target, err)
	}
	noArchive(t, root)
}

func TestRunningFilesDaemonLeavesFilesAlone(t *testing.T) {
	cfg, _ := isolate(t, "")
	fakeDaemon(t, StorageRecord{PID: 4242, Mode: storagemode.Filesystem, Namespace: "hev-traces", Archive: "/x"})
	if target, err := OpenTargetRead(cfg, ""); err != nil || target.Mode != storagemode.Filesystem {
		t.Fatalf("target = %+v, %v", target, err)
	}
}

func TestNoDaemonLockMeansNoDaemonAndCreatesNothing(t *testing.T) {
	isolate(t, "")
	st, err := RunningStorage()
	if err != nil || st.Running {
		t.Fatalf("RunningStorage = %+v, %v", st, err)
	}
	if _, err := os.Stat(DaemonLockPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("asking created the lock file: %v", err)
	}
}

// Nothing says Layer: files work as before, and a read creates nothing.
func TestNothingPointingAtLayerIsFilesAndReadsCreateNothing(t *testing.T) {
	cfg, root := isolate(t, "[capture]\nscan_interval = \"5m\"\n")
	target, err := OpenTargetRead(cfg, "")
	if err != nil || target.Mode != storagemode.Filesystem || target.Files == nil {
		t.Fatalf("target = %+v, %v", target, err)
	}
	files := target.Files
	if rows, err := files.ListSessionRows(10, nil); err != nil || len(rows) != 0 {
		t.Fatalf("list = %v, %v", rows, err)
	}
	if hits, err := files.SearchHits("anything", 10, nil); err != nil || len(hits) != 0 {
		t.Fatalf("search = %v, %v", hits, err)
	}
	if rows, err := files.SessionRows("abc"); err != nil || len(rows) != 0 {
		t.Fatalf("show = %v, %v", rows, err)
	}
	if blocks, err := files.ListBlockRows("abc"); err != nil || len(blocks) != 0 {
		t.Fatalf("blocks = %v, %v", blocks, err)
	}
	if _, err := files.IndexStats(); err != nil {
		t.Fatal(err)
	}
	noArchive(t, root)

	// The first write creates the archive, as before.
	if _, err := files.WriteSessions([]trace.SessionRow{{ID: "s1", Host: "h"}}); err != nil {
		t.Fatal(err)
	}
	if rows, err := files.ListSessionRows(10, nil); err != nil || len(rows) != 1 {
		t.Fatalf("after write: %v, %v", rows, err)
	}
	if _, err := os.Stat(filepath.Join(root, "hev-traces", "meta.json")); err != nil {
		t.Fatalf("write did not create the archive: %v", err)
	}

	// OpenTarget (the daemon, hev up) creates it at once, as on v0.7.
	cfg2, root2 := isolate(t, "")
	if target, err := OpenTarget(cfg2, ""); err != nil || target.Files == nil {
		t.Fatalf("target = %+v, %v", target, err)
	}
	if _, err := os.Stat(filepath.Join(root2, "hev-traces", "sessions")); err != nil {
		t.Fatalf("OpenTarget no longer creates the archive: %v", err)
	}
}

// A read that finds an archive the daemon created after the store was
// opened sees its rows.
func TestLazyStoreSeesAnArchiveCreatedLater(t *testing.T) {
	cfg, _ := isolate(t, "")
	reader, err := OpenTargetRead(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	writer, err := OpenTarget(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Files.WriteSessions([]trace.SessionRow{{ID: "s1", Host: "h"}}); err != nil {
		t.Fatal(err)
	}
	if rows, err := reader.Files.ListSessionRows(10, nil); err != nil || len(rows) != 1 {
		t.Fatalf("rows = %v, %v", rows, err)
	}
}

// `hev daemon install` on a machine with no key writes a config that is
// files, not one that says Layer.
func TestDefaultConfigIsFiles(t *testing.T) {
	_, root := isolate(t, "")
	path, err := WriteDefaultConfig(true)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil || cfg.ConfigPath != path {
		t.Fatal(err)
	}
	if target, err := OpenTargetRead(cfg, ""); err != nil || target.Mode != storagemode.Filesystem {
		t.Fatalf("target = %+v, %v", target, err)
	}
	noArchive(t, root)
	if cfg.LayerEndpoint != "" && cfg.LayerEndpoint != layer.DefaultEndpoint {
		t.Fatalf("endpoint = %q", cfg.LayerEndpoint)
	}
}

func TestDaemonWritesItsStorageRecord(t *testing.T) {
	cfg, _ := isolate(t, "[layer]\napi_key = \"saved\"\nnamespace = \"team\"\nendpoint = \"https://gateway.example\"\n")
	target, err := OpenTarget(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeStorageRecord(recordFor(target)); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(StorageRecordPath())
	if strings.Contains(string(raw), "saved") {
		t.Fatalf("record carries the key: %s", raw)
	}
	var r StorageRecord
	if err := json.Unmarshal(raw, &r); err != nil || r.Mode != storagemode.Layer || r.Namespace != "team" || r.Endpoint != "https://gateway.example" || r.KeySource != "config [layer].api_key" || r.PID != os.Getpid() {
		t.Fatalf("record = %+v, %v", r, err)
	}
}
