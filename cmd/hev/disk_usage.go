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
	"github.com/hev/kit/internal/reviews"
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
	{"reviews", []string{"reviews"}},
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

// measureDiskUsage walks home/.hev once (and configured evidence and review folders outside ~/.hev), attributing each regular file to the
// surface of its top-level folder. Symlinks are not followed. The walk stops
// at diskUsageMaxEntries entries or diskUsageBudget.
func measureDiskUsage(home, evidenceRoot, reviewsRoot string) diskUsage {
	root := filepath.Join(home, ".hev")
	u := diskUsage{Root: root, Surfaces: map[string]int64{}}
	owner := map[string]string{}
	for _, s := range diskSurfaces {
		u.Surfaces[s.name] = 0
		for _, d := range s.dirs {
			owner[d] = s.name
		}
	}
	type override struct {
		name, root, inside string
		outside            bool
	}
	overrides := []override{{name: "evidence", root: evidenceRoot}, {name: "reviews", root: reviewsRoot}}
	for i := range overrides {
		o := &overrides[i]
		if o.root == "" {
			continue
		}
		absolute, err := filepath.Abs(o.root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(root, absolute)
		if err != nil || !filepath.IsLocal(rel) {
			o.outside = true
		} else if rel != "." {
			o.inside = filepath.ToSlash(rel)
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
		var match override
		for _, o := range overrides {
			if o.inside != "" && strings.HasPrefix(rel, o.inside+"/") && len(o.inside) > len(match.inside) {
				match = o
			}
		}
		if match.name != "" {
			u.Surfaces[match.name] += n
			return
		}
		top, _, _ := strings.Cut(rel, "/")
		if name, ok := owner[top]; ok && top != rel {
			for _, o := range overrides {
				if o.name == name && o.root != "" && o.inside != top {
					u.Other += n
					return
				}
			}
			u.Surfaces[name] += n
			return
		}
		u.Other += n
	})
	for _, o := range overrides {
		if o.outside && !u.Partial {
			walk(o.root, func(_ string, n int64) {
				u.Surfaces[o.name] += n
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
	writeDiskUsage(w, measureDiskUsage(home, evidence.Root(), reviews.Root()))
}
