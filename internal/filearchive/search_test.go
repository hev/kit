package filearchive

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

// engines runs fn once with ripgrep (when it is on PATH) and once with the
// Go walk: the two must agree.
func engines(t *testing.T, fn func(t *testing.T)) {
	t.Helper()
	for _, engine := range []string{"rg", "go"} {
		t.Run(engine, func(t *testing.T) {
			if engine == "rg" {
				if _, err := exec.LookPath("rg"); err != nil {
					t.Skip("rg is not on PATH")
				}
				t.Setenv("HEV_GREP", "")
			} else {
				t.Setenv("HEV_GREP", "go")
			}
			fn(t)
		})
	}
}

func TestGrepSearchANDsTermsRanksAndFilters(t *testing.T) {
	engines(t, func(t *testing.T) {
		s := open(t)
		now := time.Now().UTC()
		recent := now.Add(-time.Hour).Format(time.RFC3339)
		newest := now.Add(-time.Minute).Format(time.RFC3339)
		old := now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)
		var rows []layer.Row
		rows = append(rows,
			chunk("s1", 0, recent, "claude_code", "Port 5432 collides when two CI jobs run the database"),
			chunk("s2", 0, recent, "codex", "the deploy was flaky; retried twice"),
			chunk("s3", 0, old, "codex", "port collisions in the old CI setup"),
			chunk("s4", 0, newest, "claude_code", "the port is fine, lunch is not"),
			chunk("s6", 0, recent, "claude_code", `quoted "<html>" & backslash \\ text`),
		)
		r := chunk("s5", 0, recent, "claude_code", "ran the tests")
		r.ToolName = "Bash"
		rows = append(rows, r)
		if _, err := s.Write(rows); err != nil {
			t.Fatal(err)
		}
		// Every word must be there; plurals fold.
		hits, err := s.SearchPhrasings([]string{"ports collides"}, 5, nil)
		if err != nil || len(hits) != 1 || hits[0].SessionID != "s1" || hits[0].Dist != 2 {
			t.Fatalf("AND hits = %+v, %v", hits, err)
		}
		// One word: newest first.
		hits, err = s.SearchPhrasings([]string{"port"}, 5, nil)
		if err != nil || len(hits) != 3 || hits[0].SessionID != "s4" || hits[2].SessionID != "s3" {
			t.Fatalf("recency = %+v, %v", hits, err)
		}
		since := []any{"ts", "Gte", now.Add(-24 * time.Hour).Format(time.RFC3339)}
		hits, err = s.SearchPhrasings([]string{"port"}, 5, since)
		if err != nil || len(hits) != 2 {
			t.Fatalf("since hits = %+v, %v", hits, err)
		}
		hits, err = s.SearchPhrasings([]string{"port"}, 5, []any{"harness", "Eq", "codex"})
		if err != nil || len(hits) != 1 || hits[0].SessionID != "s3" {
			t.Fatalf("harness hits = %+v, %v", hits, err)
		}
		hits, err = s.SearchPhrasings([]string{"port"}, 5, []any{"session_id", "In", []string{"s1", "s2"}})
		if err != nil || len(hits) != 1 || hits[0].SessionID != "s1" {
			t.Fatalf("session-scoped hits = %+v, %v", hits, err)
		}
		// --also is OR; a row hitting more distinct words ranks first.
		hits, err = s.SearchPhrasings([]string{"flaky deploy", "port collides database"}, 5, nil)
		if err != nil || len(hits) != 2 || hits[0].SessionID != "s1" || hits[1].SessionID != "s2" {
			t.Fatalf("OR hits = %+v, %v", hits, err)
		}
		hits, err = s.Search("bash", 5, nil)
		if err != nil || len(hits) != 1 || hits[0].SessionID != "s5" {
			t.Fatalf("tool hits = %+v, %v", hits, err)
		}
		// Words JSON escapes in the file are still found.
		hits, err = s.Search(`"<html>"`, 5, nil)
		if err != nil || len(hits) != 1 || hits[0].SessionID != "s6" {
			t.Fatalf("escaped hits = %+v, %v", hits, err)
		}
		hits, err = s.SearchRegex(`(?i)port \d{4}`, 5, nil)
		if err != nil || len(hits) != 1 || hits[0].SessionID != "s1" {
			t.Fatalf("regex hits = %+v, %v", hits, err)
		}
		if _, err := s.SearchRegex(`(`, 5, nil); err == nil {
			t.Fatal("bad regex accepted")
		}
		// A query that keys on JSON field names finds nothing: only text is
		// searched.
		if hits, _ := s.Search("harness", 5, nil); len(hits) != 0 {
			t.Fatalf("matched an attribute name: %+v", hits)
		}
	})
}

func TestEvalsAreWriteOnceAndSearchable(t *testing.T) {
	engines(t, func(t *testing.T) {
		s := open(t)
		e := trace.Eval{Session: "s1", TS: "2026-11-01T10:00:00Z", Role: "validator", Marks: map[string]int{"quality": 2}, Summary: "flaky retries hid the failure"}
		if res, err := s.WriteEvals([]trace.Eval{e}); err != nil || res.RowsUpserted != 1 {
			t.Fatalf("write = %+v, %v", res, err)
		}
		e.Summary = "changed"
		if res, err := s.WriteEvals([]trace.Eval{e}); err != nil || res.RowsUpserted != 0 {
			t.Fatalf("rewrite = %+v, %v", res, err)
		}
		rows, err := s.ListEvalRows([]any{"session_id", "Eq", "s1"})
		if err != nil || len(rows) != 1 || rows[0].Summary != "flaky retries hid the failure" {
			t.Fatalf("rows = %+v, %v", rows, err)
		}
		marks, err := s.ListEvalMarks(nil)
		if err != nil || len(marks) != 1 || marks[0].Summary != "" || marks[0].Marks == "" {
			t.Fatalf("marks = %+v, %v", marks, err)
		}
		hits, err := s.SearchEvals("retry failure", 5, nil)
		if err != nil || len(hits) != 1 || hits[0].SessionID != "s1" {
			t.Fatalf("eval hits = %+v, %v", hits, err)
		}
		if err := s.DeleteSessionArchive("s1"); err != nil {
			t.Fatal(err)
		}
		if rows, _ := s.ListEvalRows(nil); len(rows) != 0 {
			t.Fatalf("evals survive redaction delete: %+v", rows)
		}
	})
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

// The listing is derived: removed, or naming the wrong sessions, it is
// rebuilt from the session files.
func TestListingIsRebuilt(t *testing.T) {
	s := open(t)
	for i, id := range []string{"aaaa", "bbbb", "cccc"} {
		if _, err := s.WriteSessions([]trace.SessionRow{{ID: id, SessionID: id, Start: int64(i), End: int64(i)}}); err != nil {
			t.Fatal(err)
		}
	}
	newest := func() []string {
		rows, err := s.ListSessionRows(2, nil)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, r := range rows {
			ids = append(ids, r.ID)
		}
		return ids
	}
	if got := fmt.Sprint(newest()); got != "[cccc bbbb]" {
		t.Fatalf("newest = %s", got)
	}
	if err := os.Remove(filepath.Join(s.Dir(), "index.jsonl")); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(newest()); got != "[cccc bbbb]" {
		t.Fatalf("after removing the listing: %s", got)
	}
	// A session file removed by hand drops out.
	if err := os.Remove(s.path("sessions", "cccc")); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(newest()); got != "[bbbb aaaa]" {
		t.Fatalf("after removing a session: %s", got)
	}
	if err := s.DeleteSessionArchive("bbbb"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(s.Dir(), "index.jsonl"))
	if strings.Contains(string(b), "bbbb") || !strings.Contains(string(b), "aaaa") {
		t.Fatalf("listing after delete: %s", b)
	}
}

// A file changed outside the archive lock between read and write is not
// overwritten.
func TestConditionalWriteRefusesAChangedFile(t *testing.T) {
	s := open(t)
	path := filepath.Join(s.Dir(), "probe.json")
	if err := s.casWrite(path, nil, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := s.casWrite(path, nil, []byte("two")); !errors.Is(err, errConflict) {
		t.Fatalf("create over an existing file: %v", err)
	}
	if err := s.casWrite(path, []byte("stale"), []byte("two")); !errors.Is(err, errConflict) {
		t.Fatalf("write over changed bytes: %v", err)
	}
	if err := s.casWrite(path, []byte("one"), []byte("two")); err != nil {
		t.Fatal(err)
	}
}
