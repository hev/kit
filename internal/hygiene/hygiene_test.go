package hygiene

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/platformops"
)

func TestConfigLoadAndValidation(t *testing.T) {
	dir := t.TempDir()

	// Missing file returns opt-in defaults.
	cfg, err := Load(filepath.Join(dir, "missing.toml"))
	if err != nil {
		t.Fatalf("Load missing: %v", err)
	}
	if cfg.Enabled {
		t.Fatal("default enabled must be false")
	}
	if cfg.EveryDuration() != 6*time.Hour || cfg.MinFreeGBValue() != 40 ||
		cfg.CompactAfterDuration() != 24*time.Hour || cfg.PruneTmpAfterDuration() != 24*time.Hour {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if want := (runtime.GOOS == "darwin"); cfg.ThinAPFSSnapshots == nil || *cfg.ThinAPFSSnapshots != want {
		t.Fatalf("default thin_apfs_snapshots = %v, want %v", cfg.ThinAPFSSnapshots, want)
	}

	// Valid custom config (including explicit min_free_gb = 0).
	validPath := filepath.Join(dir, "valid.toml")
	validTOML := `
[hygiene]
enabled = true
every = "30m"
min_free_gb = 0
compact_after = "12h"
prune_tmp_after = "48h"
thin_apfs_snapshots = false
`
	if err := os.WriteFile(validPath, []byte(validTOML), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(validPath)
	if err != nil {
		t.Fatalf("Load valid: %v", err)
	}
	if !cfg.Enabled || cfg.EveryDuration() != 30*time.Minute || cfg.MinFreeGBValue() != 0 ||
		cfg.CompactAfterDuration() != 12*time.Hour || cfg.PruneTmpAfterDuration() != 48*time.Hour ||
		*cfg.ThinAPFSSnapshots {
		t.Fatalf("unexpected parsed config: %+v", cfg)
	}

	// Refusal cases (must reject invalid bounds and unknown keys).
	badCases := []struct {
		name string
		toml string
		want string
	}{
		{"unknown-key", "[hygiene]\nbogus = 1\n", "unknown field"},
		{"every-too-short", "[hygiene]\nevery = \"5m\"\n", "at least 15m"},
		{"negative-min-free", "[hygiene]\nmin_free_gb = -1\n", "nonnegative"},
		{"zero-compact-after", "[hygiene]\ncompact_after = \"0s\"\n", "positive"},
		{"bad-prune-after", "[hygiene]\nprune_tmp_after = \"-1h\"\n", "positive"},
		{"syntax-error", "[hygiene\nenabled = true\n", "invalid hygiene configuration"},
	}
	for _, tc := range badCases {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(dir, tc.name+".toml")
			if err := os.WriteFile(p, []byte(tc.toml), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(p)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
			if strings.Contains(err.Error(), dir) {
				t.Fatalf("error leaked local path: %v", err)
			}
		})
	}
}

func TestPruneTmpDirsSafetyBothSidesAndDryRun(t *testing.T) {
	tmpRoot := t.TempDir()
	now := time.Date(2026, 11, 21, 18, 0, 0, 0, time.UTC)
	oldTime := now.Add(-48 * time.Hour)
	recentTime := now.Add(-2 * time.Hour)

	mkDirWithFile := func(name, file string, dirMod, fileMod time.Time) string {
		t.Helper()
		d := filepath.Join(tmpRoot, name)
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
		f := filepath.Join(d, file)
		if err := os.WriteFile(f, []byte("payload-bytes-12345"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(f, fileMod, fileMod); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(d, dirMod, dirMod); err != nil {
			t.Fatal(err)
		}
		return d
	}

	// Eligible old matching dirs (must be pruned).
	oldTmp := mkDirWithFile("tmp.stale123", "data.txt", oldTime, oldTime)
	oldHev := mkDirWithFile("hev-oldwork", "cache.bin", oldTime, oldTime)
	oldGoBuild := mkDirWithFile("go-build98765", "obj.a", oldTime, oldTime)

	// Protected dirs (must be kept).
	nonMatching := mkDirWithFile("user-notes-old", "note.txt", oldTime, oldTime)
	recentHev := mkDirWithFile("hev-recent", "data.txt", recentTime, recentTime)
	oldDirRecentChild := mkDirWithFile("tmp.recentchild", "fresh.txt", oldTime, recentTime)
	otherUserDir := mkDirWithFile("hev-otheruser", "data.txt", oldTime, oldTime)

	// Active lock inside an otherwise old matching dir (must be kept).
	lockedDir := mkDirWithFile("hev-locked", "run.lock", oldTime, oldTime)
	releaseLock, err := platformops.LockPath(filepath.Join(lockedDir, "run.lock"), true)
	if err != nil {
		t.Fatalf("acquire test lock: %v", err)
	}
	defer releaseLock()
	// Re-age lock file and dir after opening lock.
	_ = os.Chtimes(filepath.Join(lockedDir, "run.lock"), oldTime, oldTime)
	_ = os.Chtimes(lockedDir, oldTime, oldTime)

	// Symlink matching prefix (must be kept; never followed or removed).
	symlinkTarget := mkDirWithFile("keep-target", "secret.txt", oldTime, oldTime)
	symlinkPath := filepath.Join(tmpRoot, "hev-symlink")
	if err := os.Symlink(symlinkTarget, symlinkPath); err != nil && runtime.GOOS != "windows" {
		t.Fatal(err)
	}

	// Simulate UID ownership: everything except hev-otheruser belongs to current user.
	ownedFn := func(info os.FileInfo) bool {
		return info.Name() != "hev-otheruser"
	}

	// 1. Dry-run: reports exact 3 planned prunes, mutates zero files.
	dryRep, err := PruneTmpDirs(tmpRoot, 24*time.Hour, now, true, ownedFn)
	if err != nil {
		t.Fatalf("dry-run PruneTmpDirs: %v", err)
	}
	wantPruned := []string{"go-build98765", "hev-oldwork", "tmp.stale123"}
	slices.Sort(dryRep.Pruned)
	if !slices.Equal(dryRep.Pruned, wantPruned) {
		t.Fatalf("dry-run pruned = %v, want %v", dryRep.Pruned, wantPruned)
	}
	if dryRep.BytesFreed <= 0 {
		t.Fatalf("expected positive estimated bytes freed in dry-run, got %d", dryRep.BytesFreed)
	}
	// Verify zero files were deleted during dry-run.
	for _, p := range []string{oldTmp, oldHev, oldGoBuild, nonMatching, recentHev, oldDirRecentChild, otherUserDir, lockedDir, symlinkTarget} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("dry-run mutated %s: %v", filepath.Base(p), err)
		}
	}

	// 2. Real run: prunes only the 3 stale matching dirs, keeps all protected dirs.
	realRep, err := PruneTmpDirs(tmpRoot, 24*time.Hour, now, false, ownedFn)
	if err != nil {
		t.Fatalf("real PruneTmpDirs: %v", err)
	}
	slices.Sort(realRep.Pruned)
	if !slices.Equal(realRep.Pruned, wantPruned) {
		t.Fatalf("real pruned = %v, want %v", realRep.Pruned, wantPruned)
	}
	for _, p := range []string{oldTmp, oldHev, oldGoBuild} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be pruned, stat err = %v", filepath.Base(p), err)
		}
	}
	for _, p := range []string{nonMatching, recentHev, oldDirRecentChild, otherUserDir, lockedDir, symlinkTarget} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("protected path %s was unexpectedly removed: %v", filepath.Base(p), err)
		}
	}
}

func TestAPFSSnapshotThinningThresholdAndTimeout(t *testing.T) {
	home := t.TempDir()
	tmpDir := t.TempDir()
	cfg := DefaultConfig()
	thin := true
	cfg.ThinAPFSSnapshots = &thin
	minFree := 40
	cfg.MinFreeGB = &minFree

	noopCompact := func(dispatch.CompactOptions) (dispatch.CompactReport, error) {
		return dispatch.CompactReport{Compacted: []string{}}, nil
	}

	// Case 1: Above threshold (50 GiB >= 40 GiB) -> tmutil MUST NOT be called.
	called := false
	rAbove := Runner{
		Home:        home,
		TempDir:     tmpDir,
		DispatchDir: home,
		OS:          "darwin",
		DiskFree:    func(string) (uint64, error) { return 50 * BytesPerGiB, nil },
		CompactRuns: noopCompact,
		ExecCommand: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			called = true
			return nil, nil
		},
	}
	rep, err := rAbove.Run(context.Background(), cfg, false)
	if err != nil {
		t.Fatalf("run above threshold: %v", err)
	}
	if called {
		t.Fatal("tmutil was called when free space was above min_free_gb")
	}
	if rep.UnderPressure || rep.APFS.Action != "skipped" {
		t.Fatalf("unexpected above-threshold report: pressure=%t apfs=%+v", rep.UnderPressure, rep.APFS)
	}

	// Case 2: Below threshold (15 GiB < 40 GiB) on darwin -> dry-run plans without calling tmutil.
	called = false
	rBelow := Runner{
		Home:        home,
		TempDir:     tmpDir,
		DispatchDir: home,
		OS:          "darwin",
		Volume:      "/",
		APFSTimeout: 5 * time.Second,
		DiskFree:    func(string) (uint64, error) { return 15 * BytesPerGiB, nil },
		CompactRuns: noopCompact,
		ExecCommand: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			called = true
			return nil, nil
		},
	}
	dryRep, err := rBelow.Run(context.Background(), cfg, true)
	if err != nil {
		t.Fatalf("dry-run below threshold: %v", err)
	}
	if called {
		t.Fatal("tmutil was called during dry-run")
	}
	if !dryRep.UnderPressure || dryRep.APFS.Action != "would_thin" || dryRep.APFS.TargetBytes != 25*BytesPerGiB {
		t.Fatalf("unexpected dry-run APFS result: %+v", dryRep.APFS)
	}

	// Case 3: Below threshold real run -> calls tmutil with strict deadline and expected args.
	var gotName string
	var gotArgs []string
	var hadDeadline bool
	rBelow.ExecCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		gotName = name
		gotArgs = append([]string(nil), args...)
		_, hadDeadline = ctx.Deadline()
		return []byte("Thinned local snapshots"), nil
	}
	realRep, err := rBelow.Run(context.Background(), cfg, false)
	if err != nil {
		t.Fatalf("real run below threshold: %v", err)
	}
	if !hadDeadline {
		t.Fatal("tmutil invocation did not set a strict timeout deadline")
	}
	wantArgs := []string{"thinlocalsnapshots", "/", "26843545600", "4"} // 25 GiB
	if gotName != "tmutil" || !slices.Equal(gotArgs, wantArgs) {
		t.Fatalf("got cmd %q %v, want tmutil %v", gotName, gotArgs, wantArgs)
	}
	if realRep.APFS.Action != "thinned" {
		t.Fatalf("expected APFS action=thinned, got %+v", realRep.APFS)
	}

	// Case 4: Timeout enforcement.
	rTimeout := rBelow
	rTimeout.APFSTimeout = 50 * time.Millisecond
	rTimeout.ExecCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	timeoutRep, err := rTimeout.Run(context.Background(), cfg, false)
	if err != nil {
		t.Fatalf("timeout run returned fatal error: %v", err)
	}
	if timeoutRep.APFS.Action != "failed" || !strings.Contains(timeoutRep.APFS.Error, "timed out") {
		t.Fatalf("expected timeout failure in APFS result, got %+v", timeoutRep.APFS)
	}

	// Case 5: Non-darwin OS skips tmutil even below threshold.
	called = false
	rLinux := rBelow
	rLinux.OS = "linux"
	rLinux.ExecCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		called = true
		return nil, nil
	}
	linuxRep, err := rLinux.Run(context.Background(), cfg, false)
	if err != nil {
		t.Fatalf("linux run: %v", err)
	}
	if called || linuxRep.APFS.Action != "skipped" {
		t.Fatalf("linux should skip APFS thinning: called=%t apfs=%+v", called, linuxRep.APFS)
	}
}

func TestStateRedactionAndDryRunCreatesNoState(t *testing.T) {
	home := t.TempDir()
	tmpDir := t.TempDir()
	cfg := DefaultConfig()

	r := Runner{
		Home:        home,
		TempDir:     tmpDir,
		DispatchDir: home,
		OS:          "linux",
		DiskFree:    func(string) (uint64, error) { return 80 * BytesPerGiB, nil },
		CompactRuns: func(dispatch.CompactOptions) (dispatch.CompactReport, error) {
			return dispatch.CompactReport{Compacted: []string{}}, nil
		},
	}

	// Dry-run must not create ~/.hev/hygiene/state.json.
	if _, err := r.Run(context.Background(), cfg, true); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if _, err := os.Stat(StatePath(home)); !os.IsNotExist(err) {
		t.Fatalf("dry-run unexpectedly created state file: %v", err)
	}

	// Real run writes state.json without leaking local paths.
	if _, err := r.Run(context.Background(), cfg, false); err != nil {
		t.Fatalf("real run: %v", err)
	}
	raw, err := os.ReadFile(StatePath(home))
	if err != nil {
		t.Fatalf("read state.json: %v", err)
	}
	text := string(raw)
	if strings.Contains(text, home) || strings.Contains(text, tmpDir) {
		t.Fatalf("state.json leaked local path:\n%s", text)
	}

	st, err := r.Status(cfg)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.State.LastRun == nil || st.CurrentFreeBytes != 80*BytesPerGiB {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestStartStopsCleanly(t *testing.T) {
	home := t.TempDir()
	r := Runner{Home: home}

	disabled := DefaultConfig()
	stopDisabled := r.Start(context.Background(), disabled)
	stopDisabled()

	enabled := DefaultConfig()
	enabled.Enabled = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopEnabled := r.Start(ctx, enabled)
	stopEnabled()
}
