package filearchive

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// TestScaleSearch measures a large archive. It is opt-in:
//
//	HEV_ARCHIVE_SCALE_SOURCE  a real archive's namespace directory
//	HEV_ARCHIVE_SCALE_COPIES  how many copies of it to make (default 10)
//	HEV_ARCHIVE_SCALE_DIR     where to build the copy (default a temp dir)
//	HEV_ARCHIVE_SCALE_REUSE   time the copy already in HEV_ARCHIVE_SCALE_DIR
//
// Every chunk and session of the source is written again under a new
// session id per copy, through the ordinary write path; then the index is
// built and list and search are timed. docs/trace-archive.md records a run.
func TestScaleSearch(t *testing.T) {
	source := os.Getenv("HEV_ARCHIVE_SCALE_SOURCE")
	if source == "" {
		t.Skip("set HEV_ARCHIVE_SCALE_SOURCE to measure a large archive")
	}
	copies, _ := strconv.Atoi(os.Getenv("HEV_ARCHIVE_SCALE_COPIES"))
	if copies <= 0 {
		copies = 10
	}
	dir := os.Getenv("HEV_ARCHIVE_SCALE_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	src := &Store{dir: source}
	dst, err := Open(dir, "scale")
	if err != nil {
		t.Fatal(err)
	}
	keys, err := src.keys("chunks")
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := src.ListSessionRows(-1, nil)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	chunks := 0
	if os.Getenv("HEV_ARCHIVE_SCALE_REUSE") != "" {
		copies = 0 // time the archive already built in HEV_ARCHIVE_SCALE_DIR
	}
	for c := 0; c < copies; c++ {
		for _, k := range keys {
			rows, err := src.chunkRows(k)
			if err != nil {
				t.Fatal(err)
			}
			for i := range rows {
				if rows[i].SessionID != "" {
					rows[i].SessionID = fmt.Sprintf("%s-c%02d", rows[i].SessionID, c)
				}
				rows[i].ID = fmt.Sprintf("%s-c%02d", rows[i].ID, c)
			}
			for from := 0; from < len(rows); from += 200 {
				if _, err := dst.Write(rows[from:min(from+200, len(rows))]); err != nil {
					t.Fatal(err)
				}
			}
			chunks += len(rows)
		}
		batch := make([]trace.SessionRow, 0, 200)
		for _, s := range sessions {
			s.ID = fmt.Sprintf("%s-c%02d", s.ID, c)
			s.SessionID = s.ID
			batch = append(batch, s)
			if len(batch) == 200 {
				if _, err := dst.WriteSessions(batch); err != nil {
					t.Fatal(err)
				}
				batch = batch[:0]
			}
		}
		if _, err := dst.WriteSessions(batch); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("wrote %d chunks and %d sessions in %s", chunks, copies*len(sessions), time.Since(start).Round(time.Second))
	start = time.Now()
	if err := dst.Refresh(); err != nil {
		t.Fatal(err)
	}
	st, _ := dst.IndexStats()
	t.Logf("index built in %s: %+v", time.Since(start).Round(time.Second), st)

	timeIt := func(name string, fn func() (int, error)) {
		t.Helper()
		for run := 0; run < 2; run++ {
			start := time.Now()
			n, err := fn()
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%-44s run %d: %6d results in %s", name, run+1, n, time.Since(start).Round(time.Millisecond))
		}
	}
	since := []any{"ts", "Gte", time.Now().AddDate(0, 0, -7).UTC().Format(time.RFC3339)}
	hits := func(phrasings []string, top int, filter any) func() (int, error) {
		return func() (int, error) {
			h, err := dst.SearchPhrasings(phrasings, top, filter)
			return len(h), err
		}
	}
	timeIt("list sessions (all)", func() (int, error) { r, err := dst.ListSessionRows(-1, nil); return len(r), err })
	timeIt("list sessions (last 7 days)", func() (int, error) {
		r, err := dst.ListSessionRows(1000, []any{"start", "Gte", time.Now().AddDate(0, 0, -7).UnixMilli()})
		return len(r), err
	})
	timeIt("search: one word", hits([]string{"migration"}, 8, nil))
	timeIt("search: a question", hits([]string{"why did the preflight fail with a 401"}, 8, nil))
	timeIt("search: three phrasings (--also)", hits([]string{"flaky deploy", "retry the deploy", "deploy failed twice"}, 8, nil))
	timeIt("search: --since 7d", hits([]string{"why did the build fail"}, 8, since))
	timeIt("search: --harness codex", hits([]string{"why did the build fail"}, 8, []any{"harness", "Eq", "codex"}))
	timeIt("search: dashboard (200 candidates)", hits([]string{"redaction policy"}, 200, nil))
	var one layer.Hit
	if h, _ := dst.Search("migration", 1, nil); len(h) == 1 {
		one = h[0]
	}
	b, _ := json.Marshal(one)
	t.Logf("sample hit is %d bytes", len(b))
	var size int64
	filepath.Walk(dst.Dir(), func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	t.Logf("archive apparent size %d MB", size>>20)
}
