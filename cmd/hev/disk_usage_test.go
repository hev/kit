package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fill(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), n), 0600); err != nil {
		t.Fatal(err)
	}
}

// Rule: every byte under ~/.hev is counted once, under the surface that
// owns its top-level folder, and the rest is "other".
func TestDiskUsagePerSurface(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	h := filepath.Join(home, ".hev")
	fill(t, filepath.Join(h, "archive", "hev-traces", "blocks", "01", "a.jsonl"), 1000)
	fill(t, filepath.Join(h, "traces", "x"), 24)
	fill(t, filepath.Join(h, "board", "threads", "1.json"), 300)
	fill(t, filepath.Join(h, "canvas", "c.json"), 7)
	fill(t, filepath.Join(h, "orders", "o.json"), 50)
	fill(t, filepath.Join(h, "peers", "p"), 60)
	fill(t, filepath.Join(h, "hooks", "events.jsonl"), 70)
	fill(t, filepath.Join(h, "evidence", "ev_x", "001-a.png"), 4096)
	fill(t, filepath.Join(h, "reviews", "rev_x", "review.json"), 21)
	fill(t, filepath.Join(h, "config.toml"), 11)
	fill(t, filepath.Join(h, "secrets", "cache"), 9)
	// A file named like a surface at the top level is not that surface.
	fill(t, filepath.Join(h, "hooks.bak"), 5)
	// Symlinks are not followed.
	outside := filepath.Join(t.TempDir(), "big")
	fill(t, outside, 1<<20)
	os.Symlink(outside, filepath.Join(h, "board", "link"))

	u := measureDiskUsage(home, filepath.Join(h, "evidence"), filepath.Join(h, "reviews"))
	want := map[string]int64{"traces": 1024, "board": 307, "orders": 50, "peers": 60, "hooks": 70, "evidence": 4096, "reviews": 21}
	for k, v := range want {
		if u.Surfaces[k] != v {
			t.Errorf("%s = %d, want %d", k, u.Surfaces[k], v)
		}
	}
	if u.Other != 25 || u.Total != 1024+307+50+60+70+4096+21+25 || u.Partial {
		t.Fatalf("other %d total %d partial %v", u.Other, u.Total, u.Partial)
	}
	var out bytes.Buffer
	writeDiskUsage(&out, u)
	for _, line := range []string{"Disk usage (" + filepath.Join("~", ".hev") + "): 5.5 KiB", "  traces    1.0 KiB", "  evidence  4.0 KiB", "  reviews   21 B", "  hooks     70 B", "  other     25 B"} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("missing %q in\n%s", line, out.String())
		}
	}

	// An evidence folder configured outside ~/.hev is counted and added to
	// the total; ~/.hev/evidence then is just other.
	ext := filepath.Join(t.TempDir(), "proof")
	fill(t, filepath.Join(ext, "ev_y", "manifest.json"), 500)
	u = measureDiskUsage(home, ext, "")
	if u.Surfaces["evidence"] != 500 || u.Other != 25+4096 || u.Total != 1024+307+50+60+70+4096+21+25+500 {
		t.Fatalf("external evidence %+v", u)
	}
	// A configured review folder is attributed to reviews even when nested
	// under another surface. The default review directory becomes other.
	override := filepath.Join(h, "orders", "code-reviews")
	fill(t, filepath.Join(override, "rev_y", "review.json"), 42)
	u = measureDiskUsage(home, "", override)
	if u.Surfaces["reviews"] != 42 || u.Surfaces["orders"] != 50 || u.Other != 25+21 {
		t.Fatalf("configured reviews %+v", u)
	}
	// An empty ~/.hev reports zeros, not an error.
	u = measureDiskUsage(t.TempDir(), "", "")
	if u.Total != 0 || len(u.Surfaces) != 7 {
		t.Fatalf("empty %+v", u)
	}
}

// Rule: the walk is bounded; past the bound counts are marked partial.
func TestDiskUsageBounded(t *testing.T) {
	home := t.TempDir()
	for i := range 50 {
		fill(t, filepath.Join(home, ".hev", "archive", strings.Repeat("d", 1+i%3), string(rune('a'+i%26))+string(rune('a'+i/26))), 10)
	}
	defer func(n int, d time.Duration) { diskUsageMaxEntries, diskUsageBudget = n, d }(diskUsageMaxEntries, diskUsageBudget)
	diskUsageMaxEntries = 10
	u := measureDiskUsage(home, "", "")
	if !u.Partial || u.Total >= 500 {
		t.Fatalf("bounded walk %+v", u)
	}
	var out bytes.Buffer
	writeDiskUsage(&out, u)
	if !strings.Contains(out.String(), "≥") || !strings.Contains(out.String(), "lower bounds") {
		t.Fatalf("partial output %s", out.String())
	}
	diskUsageMaxEntries = 1_000_000
	if u := measureDiskUsage(home, "", ""); u.Partial || u.Surfaces["traces"] != 500 {
		t.Fatalf("full walk %+v", u)
	}
}
