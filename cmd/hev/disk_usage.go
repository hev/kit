package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hev/kit/internal/evidence"
)

// Disk usage bounds: `hev s` must stay fast on a host with a large archive.
var (
	diskUsageMaxEntries = 1_000_000
	diskUsageBudget     = 3 * time.Second
)

// diskSurfaces maps each kit surface to its folders under ~/.hev, in the
// order `hev s` prints them. traces counts the file archive (~/.hev/archive)
// beside ~/.hev/traces; orders counts ~/.hev/dispatch beside ~/.hev/orders.
var diskSurfaces = []struct {
	name string
	dirs []string
}{
	{"traces", []string{"traces", "archive"}},
	{"board", []string{"board", "canvas"}},
	{"orders", []string{"orders", "dispatch"}},
	{"peers", []string{"peers"}},
	{"hooks", []string{"hooks"}},
	{"evidence", []string{"evidence"}},
}

// diskUsage is the per-surface byte count of ~/.hev.
type diskUsage struct {
	Root     string           `json:"root"`
	Surfaces map[string]int64 `json:"surfaces"`
	Other    int64            `json:"other"`
	Total    int64            `json:"total"`
	// Partial is set when the walk hit its entry or time bound; the counts
	// are then lower bounds.
	Partial bool `json:"partial,omitempty"`
}

// measureDiskUsage walks home/.hev once (and the evidence folder, when it
// is configured outside ~/.hev), attributing each regular file to the
// surface of its top-level folder. Symlinks are not followed. The walk stops
// at diskUsageMaxEntries entries or diskUsageBudget.
func measureDiskUsage(home, evidenceRoot string) diskUsage {
	root := filepath.Join(home, ".hev")
	u := diskUsage{Root: root, Surfaces: map[string]int64{}}
	owner := map[string]string{}
	for _, s := range diskSurfaces {
		u.Surfaces[s.name] = 0
		for _, d := range s.dirs {
			owner[d] = s.name
		}
	}
	// A configured evidence folder inside ~/.hev but not at ~/.hev/evidence
	// still counts as evidence.
	evRel := ""
	if evidenceRoot != "" {
		if rel, err := filepath.Rel(root, evidenceRoot); err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
			evRel = filepath.ToSlash(rel)
		}
	}
	deadline := time.Now().Add(diskUsageBudget)
	entries := 0
	stop := errors.New("bound")
	walk := func(dir string, add func(rel string, n int64)) {
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			entries++
			if entries > diskUsageMaxEntries || entries%1024 == 0 && time.Now().After(deadline) {
				return stop
			}
			if !d.Type().IsRegular() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(dir, p)
			add(filepath.ToSlash(rel), info.Size())
			return nil
		})
		if errors.Is(err, stop) {
			u.Partial = true
		}
	}
	walk(root, func(rel string, n int64) {
		u.Total += n
		if evRel != "" && (rel == evRel || strings.HasPrefix(rel, evRel+"/")) {
			u.Surfaces["evidence"] += n
			return
		}
		top, _, _ := strings.Cut(rel, "/")
		if name, ok := owner[top]; ok && top != rel {
			if name == "evidence" && evidenceRoot != "" && evRel != "evidence" {
				// ~/.hev/evidence is not the configured folder.
				u.Other += n
				return
			}
			u.Surfaces[name] += n
			return
		}
		u.Other += n
	})
	if evidenceRoot != "" && evRel == "" && !u.Partial {
		if rel, err := filepath.Rel(root, evidenceRoot); err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			// Outside ~/.hev: count it, and add it to the total.
			walk(evidenceRoot, func(_ string, n int64) {
				u.Surfaces["evidence"] += n
				u.Total += n
			})
		}
	}
	return u
}

func writeDiskUsage(w io.Writer, u diskUsage) {
	bound := ""
	if u.Partial {
		bound = "≥"
	}
	fmt.Fprintf(w, "Disk usage (%s): %s%s\n", tildePath(u.Root), bound, evidenceBytes(u.Total))
	for _, s := range diskSurfaces {
		fmt.Fprintf(w, "  %-9s %s%s\n", s.name, bound, evidenceBytes(u.Surfaces[s.name]))
	}
	fmt.Fprintf(w, "  %-9s %s%s\n", "other", bound, evidenceBytes(u.Other))
	if u.Partial {
		fmt.Fprintln(w, "  (stopped early: counts are lower bounds)")
	}
}

func tildePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.Join("~", rel)
		}
	}
	return p
}

// printDiskUsage is the `hev s` section.
func printDiskUsage(w io.Writer) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	writeDiskUsage(w, measureDiskUsage(home, evidence.Root()))
}
