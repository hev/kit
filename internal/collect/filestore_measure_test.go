package collect

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/grep"
)

// TestFileTurnsMeasure prints rough size and speed of the no-key turn store
// for https://hevkit.com/docs/app-traces. It only runs with HEV_COLLECT_MEASURE set (to the
// owners and turns per owner as "100x200"; "1" means 100x200).
func TestFileTurnsMeasure(t *testing.T) {
	v := os.Getenv("HEV_COLLECT_MEASURE")
	if v == "" {
		t.Skip("set HEV_COLLECT_MEASURE=1 (or a number of owners) to measure")
	}
	owners, perOwner := 100, 200
	if a, b, ok := strings.Cut(v, "x"); ok {
		owners, _ = strconv.Atoi(a)
		perOwner, _ = strconv.Atoi(b)
	}
	if owners < 1 || perOwner < 1 {
		t.Fatalf("HEV_COLLECT_MEASURE=%q: want OWNERSxTURNS", v)
	}
	owner := fmt.Sprint(1000 + owners/2)
	chat := fmt.Sprintf("chat-%d-%d", owners/2, 0)
	root := t.TempDir()
	st, err := filearchive.Open(root, "hev-app-traces-story-prod")
	if err != nil {
		t.Fatal(err)
	}
	ft := &FileTurns{Store: st, Namespace: "story-prod"}
	ctx := context.Background()
	words := []string{"heist", "Lisbon", "tram", "noir", "Chicago", "rain", "dragon", "harbor", "letter", "garden", "station", "winter"}
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	start := time.Now()
	var upserts []time.Duration
	for i := 0; i < perOwner; i++ {
		// One export per turn per owner, as a live app would send them.
		for o := 0; o < owners; o++ {
			turn := Turn{
				Namespace: "story-prod", Owner: fmt.Sprint(1000 + o), Tenant: "story", ChatID: fmt.Sprintf("chat-%d-%d", o, i/10), TurnID: fmt.Sprintf("turn-%d-%d", o, i),
				TS:     base.Add(time.Duration(i*owners+o) * time.Second),
				Prompt: fmt.Sprintf("Write scene %d with a %s in %s near the %s", i, words[i%12], words[(i+o)%12], words[(i*7+o)%12]),
				Answer: fmt.Sprintf("The %s waited by the %s while the %s rolled past. %s", words[(i+3)%12], words[(o+5)%12], words[(i+o+7)%12], longText),
				Tools:  []Tool{{Name: "searchStories", OK: true}},
			}
			s := time.Now()
			if err := ft.Upsert(ctx, []Turn{turn}); err != nil {
				t.Fatal(err)
			}
			upserts = append(upserts, time.Since(s))
		}
	}
	total := time.Since(start)
	var bytes int64
	files := 0
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			info, _ := d.Info()
			bytes += info.Size()
			files++
		}
		return nil
	})
	sort.Slice(upserts, func(i, j int) bool { return upserts[i] < upserts[j] })
	t.Logf("rg on PATH: %v; %d owners x %d turns = %d turns; %d files, %.1f MB on disk (%.0f B/turn)", grep.RgPath() != "", owners, perOwner, owners*perOwner, files, float64(bytes)/1e6, float64(bytes)/float64(owners*perOwner))
	t.Logf("write: %s total, upsert of one turn median %s, p99 %s (last turns rewrite a %d-turn owner file)", total.Round(time.Millisecond), upserts[len(upserts)/2].Round(time.Microsecond), upserts[len(upserts)*99/100].Round(time.Microsecond), perOwner)
	timeIt := func(name string, fn func() int) {
		var ds []time.Duration
		n := 0
		for i := 0; i < 20; i++ {
			s := time.Now()
			n = fn()
			ds = append(ds, time.Since(s))
		}
		sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
		t.Logf("%s: %d results, median %s, max %s", name, n, ds[10].Round(time.Microsecond), ds[19].Round(time.Microsecond))
	}
	timeIt("search one owner, 1 word", func() int {
		h, err := ft.Search(ctx, owner, "story", "Lisbon", time.Time{}, 10)
		if err != nil {
			t.Fatal(err)
		}
		return len(h)
	})
	timeIt("search one owner, 2 words", func() int {
		h, err := ft.Search(ctx, owner, "story", "heist "+words[(owners/2)%12], time.Time{}, 10)
		if err != nil {
			t.Fatal(err)
		}
		return len(h)
	})
	timeIt("search one owner, no match", func() int {
		h, err := ft.Search(ctx, owner, "story", "zeppelin", time.Time{}, 10)
		if err != nil {
			t.Fatal(err)
		}
		return len(h)
	})
	timeIt("conversation fetch", func() int {
		c, err := ft.Conversation(ctx, owner, "story", chat)
		if err != nil {
			t.Fatal(err)
		}
		return len(c)
	})
}

// longText makes a turn's answer the size of a short model reply (~1 KB).
var longText = func() string {
	s := ""
	for len(s) < 900 {
		s += "She counted the steps to the door and listened for the second bell before she moved. "
	}
	return s
}()
