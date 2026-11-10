package filearchive

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir(), "hev-traces")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func chunk(session string, seq int64, ts, harness, text string) layer.Row {
	return layer.Row{ID: fmt.Sprintf("%s-%d", session, seq), SessionID: session, Seq: seq, TS: ts, Harness: harness, Role: "assistant", BlockType: "text", Tier: "text", Text: text, Workdir: "/w/" + session}
}

func TestRoundTripListShowBlocks(t *testing.T) {
	s := open(t)
	rows := []layer.Row{
		chunk("aaaa1111", 0, "2026-11-01T10:00:00Z", "claude_code", "the preflight failed on HEVLAYER_API_KEY 401"),
		chunk("aaaa1111", 1, "2026-11-01T10:01:00Z", "claude_code", "fixed by exporting the key"),
		chunk("bbbb2222", 0, "2026-11-02T10:00:00Z", "codex", "flaky deploy on friday"),
	}
	if _, err := s.Write(rows); err != nil {
		t.Fatal(err)
	}
	// Upserting the same content-addressed rows again is a no-op.
	if _, err := s.Write(rows[:1]); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteBlocks([]trace.BlockRow{{ID: "b2", SessionID: "aaaa1111", Seq: 1, Text: "two"}, {ID: "b1", SessionID: "aaaa1111", Seq: 0, Text: "one"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteSessions([]trace.SessionRow{
		{ID: "aaaa1111", SessionID: "aaaa1111", Harness: "claude_code", Start: 1000, End: 2000, Cost: 0.42, Commits: trace.StringList{"c2"}},
		{ID: "bbbb2222", SessionID: "bbbb2222", Harness: "codex", Start: 3000, End: 4000},
	}); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListSessionRows(10, nil)
	if err != nil || len(list) != 2 || list[0].ID != "bbbb2222" || list[1].Cost != 0.42 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	filtered, err := s.ListSessionRows(10, []any{"And", []any{[]any{"start", "Gte", int64(500)}, []any{"harness", "In", []string{"claude_code"}}}})
	if err != nil || len(filtered) != 1 || filtered[0].ID != "aaaa1111" {
		t.Fatalf("filtered = %+v, %v", filtered, err)
	}
	show, err := s.SessionRows("aaaa")
	if err != nil || len(show) != 2 || show[0].Seq != 0 || show[1].Seq != 1 {
		t.Fatalf("show = %+v, %v", show, err)
	}
	blocks, err := s.ListBlockRows("aaaa1111")
	if err != nil || len(blocks) != 2 || blocks[0].Text != "one" {
		t.Fatalf("blocks = %+v, %v", blocks, err)
	}

	// A later capture keeps the summary, outcomes and commits other writers set.
	if _, err := s.PatchSessionSummaries([]trace.SessionRow{{ID: "aaaa1111", Summary: "fix the preflight"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.PatchOutcomes(context.Background(), trace.SessionRow{ID: "aaaa1111", PR: "https://example.com/pr/1", SessionOutcomes: trace.SessionOutcomes{PRState: "merged", OutcomeChecked: 7}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteSessions([]trace.SessionRow{{ID: "aaaa1111", SessionID: "aaaa1111", Harness: "claude_code", Start: 1000, End: 2500, Commits: trace.StringList{"c1"}}}); err != nil {
		t.Fatal(err)
	}
	// An older source never rolls a newer row back.
	if _, err := s.WriteSessions([]trace.SessionRow{{ID: "aaaa1111", SessionID: "aaaa1111", Start: 1000, End: 1500}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListSessionRows(1, []any{"id", "Eq", "aaaa1111"})
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	r := got[0]
	if r.Summary != "fix the preflight" || r.PRState != "merged" || r.PR == "" || r.End != 2500 || strings.Join(r.Commits, ",") != "c1,c2" {
		t.Fatalf("merged row = %+v", r)
	}
	page, err := s.OutcomePage(context.Background(), "", 0, 10)
	if err != nil || len(page) != 2 || page[0].ID != "aaaa1111" {
		t.Fatalf("outcome page = %+v, %v", page, err)
	}

	if err := s.DeleteSessionArchive("aaaa1111"); err != nil {
		t.Fatal(err)
	}
	if rows, _ := s.SessionRows("aaaa"); len(rows) != 0 {
		t.Fatalf("deleted session still has chunks: %+v", rows)
	}
}

func TestOddSessionIDsAreHashedAndStillFound(t *testing.T) {
	s := open(t)
	id := "ses_MixedCase/with:odd*chars"
	if _, err := s.Write([]layer.Row{chunk(id, 0, "2026-11-01T10:00:00Z", "opencode", "odd identifiers work")}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.SessionRows("ses_Mixed")
	if err != nil || len(rows) != 1 || rows[0].SessionID != id {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
}

func TestFilterGrammar(t *testing.T) {
	row, _ := decodeRow([]byte(`{"id":"x","n":5,"s":"abc","list":["a","b"],"slist":"[\"q\"]","flag":true}`))
	for _, c := range []struct {
		f    any
		want bool
	}{
		{nil, true},
		{[]any{"n", "Eq", 5}, true},
		{[]any{"n", "Gte", int64(6)}, false},
		{[]any{"n", "Lt", 5.5}, true},
		{[]any{"s", "In", []string{"abc", "d"}}, true},
		{[]any{"s", "NotIn", []string{"abc"}}, false},
		{[]any{"list", "ContainsAny", []string{"z", "b"}}, true},
		{[]any{"slist", "Contains", "q"}, true},
		{[]any{"missing", "Eq", nil}, true},
		{[]any{"s", "Glob", "a*"}, true},
		{[]any{"Not", []any{"flag", "Eq", true}}, false},
		{[]any{"Or", []any{[]any{"n", "Eq", 1}, []any{"s", "Eq", "abc"}}}, true},
		{[]any{"And", []any{[]any{"n", "Eq", 5}, []any{"s", "Eq", "no"}}}, false},
	} {
		m, err := compileFilter(c.f)
		if err != nil {
			t.Fatalf("%v: %v", c.f, err)
		}
		if got := m(row); got != c.want {
			t.Errorf("%v = %v, want %v", c.f, got, c.want)
		}
	}
	if _, err := compileFilter([]any{"n", "Approx", 1}); err == nil {
		t.Fatal("unknown operator accepted")
	}
}
