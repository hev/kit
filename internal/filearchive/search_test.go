package filearchive

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

func TestDeletedAndOddSessionsInSearch(t *testing.T) {
	s := open(t)
	id := "ses_MixedCase/with:odd*chars"
	rows := []layer.Row{chunk(id, 0, "2026-11-01T10:00:00Z", "opencode", "odd identifiers work"), chunk("gone1234", 0, "2026-11-01T10:00:00Z", "codex", "odd but deleted")}
	if _, err := s.Write(rows); err != nil {
		t.Fatal(err)
	}
	hits, err := s.Search("identifiers", 5, []any{"session_id", "Eq", id})
	if err != nil || len(hits) != 1 {
		t.Fatalf("hits = %+v, %v", hits, err)
	}
	if err := s.DeleteSessionArchive("gone1234"); err != nil {
		t.Fatal(err)
	}
	if hits, _ := s.Search("odd", 5, nil); len(hits) != 1 || hits[0].SessionID != id {
		t.Fatalf("deleted session still searchable: %+v", hits)
	}
}

func TestLexicalSearchRanksFiltersAndFuses(t *testing.T) {
	s := open(t)
	now := time.Now().UTC()
	recent := now.Add(-time.Hour).Format(time.RFC3339)
	old := now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	var rows []layer.Row
	rows = append(rows,
		chunk("s1", 0, recent, "claude_code", "Port 5432 collides when two CI jobs run the database"),
		chunk("s2", 0, recent, "codex", "the deploy was flaky; retried twice"),
		chunk("s3", 0, old, "codex", "port collision in the old CI setup"),
		chunk("s4", 0, recent, "claude_code", "unrelated text about lunch"),
	)
	r := chunk("s5", 0, recent, "claude_code", "ran the tests")
	r.ToolName = "Bash"
	rows = append(rows, r)
	if _, err := s.Write(rows); err != nil {
		t.Fatal(err)
	}
	hits, err := s.SearchPhrasings([]string{"port collisions"}, 5, nil)
	if err != nil || len(hits) != 2 || hits[0].Dist <= 0 {
		t.Fatalf("hits = %+v, %v", hits, err)
	}
	since := []any{"ts", "Gte", now.Add(-24 * time.Hour).Format(time.RFC3339)}
	hits, err = s.SearchPhrasings([]string{"port collisions"}, 5, since)
	if err != nil || len(hits) != 1 || hits[0].SessionID != "s1" {
		t.Fatalf("since hits = %+v, %v", hits, err)
	}
	hits, err = s.SearchPhrasings([]string{"port"}, 5, []any{"harness", "Eq", "codex"})
	if err != nil || len(hits) != 1 || hits[0].SessionID != "s3" {
		t.Fatalf("harness hits = %+v, %v", hits, err)
	}
	// A second phrasing widens the search, as --also does on Layer.
	hits, err = s.SearchPhrasings([]string{"port collisions", "flaky deploy"}, 5, since)
	if err != nil || len(hits) != 2 {
		t.Fatalf("fused hits = %+v, %v", hits, err)
	}
	hits, err = s.Search("bash", 5, nil)
	if err != nil || len(hits) != 1 || hits[0].SessionID != "s5" {
		t.Fatalf("tool hits = %+v, %v", hits, err)
	}
}

func TestRewrittenSessionsReplaceTheirPostingsAndSegmentsMerge(t *testing.T) {
	s := open(t)
	// Many refreshes, each a segment: the index must merge and keep only the
	// latest copy of a session that keeps changing.
	for i := 0; i < 3*maxSegments; i++ {
		rows := []layer.Row{chunk("live", int64(i), "2026-11-01T10:00:00Z", "claude_code", fmt.Sprintf("growing session word%d", i))}
		rows = append(rows, chunk(fmt.Sprintf("other%02d", i), 0, "2026-11-01T10:00:00Z", "codex", "steady text"))
		if _, err := s.Write(rows); err != nil {
			t.Fatal(err)
		}
		if err := s.Refresh(); err != nil {
			t.Fatal(err)
		}
	}
	st, err := s.IndexStats()
	if err != nil {
		t.Fatal(err)
	}
	if st.Segments > maxSegments {
		t.Fatalf("segments not merged: %+v", st)
	}
	hits, err := s.Search("growing session", 100, nil)
	if err != nil || len(hits) != 3*maxSegments {
		t.Fatalf("live session chunks = %d, %v", len(hits), err)
	}
	seen := map[string]bool{}
	for _, h := range hits {
		if seen[h.ID] {
			t.Fatalf("duplicate hit %s", h.ID)
		}
		seen[h.ID] = true
	}
	hits, err = s.Search("steady", 100, nil)
	if err != nil || len(hits) != 3*maxSegments {
		t.Fatalf("steady = %d, %v", len(hits), err)
	}
	files, _ := filepath.Glob(filepath.Join(s.Dir(), "index", "seg-*.lx"))
	if len(files) != st.Segments {
		t.Fatalf("stale segment files: %v (manifest %d)", files, st.Segments)
	}
}

// TestConcurrentWritersAndReaders runs capture and search at the same time,
// in goroutines and in separate processes, and checks nothing is lost or
// corrupted.
func TestConcurrentWritersAndReaders(t *testing.T) {
	if dir := os.Getenv("ARCHIVE_WRITER_DIR"); dir != "" {
		archiveWriter(t, dir)
		return
	}
	root := t.TempDir()
	s, err := Open(root, "hev-traces")
	if err != nil {
		t.Fatal(err)
	}
	const procs, goroutines, each = 3, 3, 20
	var cmds []*exec.Cmd
	for p := 0; p < procs; p++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestConcurrentWritersAndReaders$")
		cmd.Env = append(os.Environ(), "ARCHIVE_WRITER_DIR="+root, "ARCHIVE_WRITER_ID=p"+strconv.Itoa(p), "ARCHIVE_WRITER_N="+strconv.Itoa(each))
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		cmds = append(cmds, cmd)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 100)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			if err := writeSome(s, "g"+strconv.Itoa(g), each); err != nil {
				errs <- err
			}
		}(g)
	}
	stop := make(chan struct{})
	var readers sync.WaitGroup
	for r := 0; r < 3; r++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := s.Search("concurrent capture", 50, nil); err != nil {
					errs <- fmt.Errorf("search: %w", err)
					return
				}
				if _, err := s.ListSessionRows(-1, nil); err != nil {
					errs <- fmt.Errorf("list: %w", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	for _, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("writer process: %v", err)
		}
	}
	close(stop)
	readers.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	want := (procs + goroutines) * each
	rows, err := s.ListSessionRows(-1, nil)
	if err != nil || len(rows) != want {
		t.Fatalf("sessions = %d, want %d, %v", len(rows), want, err)
	}
	hits, err := s.Search("concurrent capture", 10*want, nil)
	if err != nil || len(hits) != 2*want {
		t.Fatalf("hits = %d, want %d, %v", len(hits), 2*want, err)
	}
	for _, r := range rows {
		blocks, err := s.ListBlockRows(r.ID)
		if err != nil || len(blocks) != 1 {
			t.Fatalf("blocks for %s = %d, %v", r.ID, len(blocks), err)
		}
	}
}

func writeSome(s *Store, who string, n int) error {
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("%s-%03d", who, i)
		rows := []layer.Row{
			chunk(id, 0, "2026-11-01T10:00:00Z", "claude_code", "concurrent capture from "+who),
			chunk(id, 1, "2026-11-01T10:00:01Z", "claude_code", "more concurrent capture text"),
		}
		if _, err := s.Write(rows); err != nil {
			return err
		}
		if _, err := s.WriteBlocks([]trace.BlockRow{{ID: id + "-b", SessionID: id, Text: "block"}}); err != nil {
			return err
		}
		if _, err := s.WriteSessions([]trace.SessionRow{{ID: id, SessionID: id, Start: int64(i), End: int64(i)}}); err != nil {
			return err
		}
		if i%5 == 0 {
			if err := s.Refresh(); err != nil {
				return err
			}
		}
	}
	return nil
}

func archiveWriter(t *testing.T, root string) {
	s, err := Open(root, "hev-traces")
	if err != nil {
		t.Fatal(err)
	}
	n, _ := strconv.Atoi(os.Getenv("ARCHIVE_WRITER_N"))
	if err := writeSome(s, os.Getenv("ARCHIVE_WRITER_ID"), n); err != nil {
		t.Fatal(err)
	}
}

// Deleting the index directory is safe: the next search rebuilds it from the
// chunk files.
func TestDeletedIndexIsRebuilt(t *testing.T) {
	s := open(t)
	if _, err := s.Write([]layer.Row{chunk("s1", 0, "2026-11-01T10:00:00Z", "codex", "rebuild me from chunks")}); err != nil {
		t.Fatal(err)
	}
	if hits, err := s.Search("rebuild", 5, nil); err != nil || len(hits) != 1 {
		t.Fatalf("hits = %+v, %v", hits, err)
	}
	if err := os.RemoveAll(filepath.Join(s.Dir(), "index")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(s.Dir(), "index"), 0o700); err != nil {
		t.Fatal(err)
	}
	if hits, err := s.Search("rebuild", 5, nil); err != nil || len(hits) != 1 {
		t.Fatalf("after deleting the index: %+v, %v", hits, err)
	}
}
