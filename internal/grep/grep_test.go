package grep

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

func TestTerms(t *testing.T) {
	for q, want := range map[string]string{
		"why did the preflight fail": "preflight fail",
		"Ports and RETRIES":          "port retry",
		"HEVLAYER_API_KEY 401":       "hevlayer_api_key 401",
		"cmd/hev/up.go, again.":      "cmd/hev/up.go again",
		"the of":                     "the of",
		"status class process":       "status class process",
		"!!!":                        "",
	} {
		if got := strings.Join(Terms(q), " "); got != want {
			t.Errorf("Terms(%q) = %q, want %q", q, got, want)
		}
	}
}

func TestScoreANDsPhrasingsAndCountsDistinctTerms(t *testing.T) {
	q, err := Compile([]string{"flaky deploy", "retries"})
	if err != nil {
		t.Fatal(err)
	}
	for text, want := range map[string]int{
		"a FLAKY deploy":             2,
		"flaky deploy, then retried": 3,
		"retry once":                 1,
		"flaky only":                 0,
	} {
		n, ok := q.Score(text)
		if (want == 0) == ok || (ok && n != want) {
			t.Errorf("Score(%q) = %d, %v; want %d", text, n, ok, want)
		}
	}
	if _, err := Compile([]string{"  ", "!!"}); err == nil {
		t.Fatal("a query with no words compiled")
	}
}

func TestSnippetCentersOnTheMatch(t *testing.T) {
	q, _ := Compile([]string{"needle"})
	text := strings.Repeat("hay ", 100) + "the NEEDLE is here " + strings.Repeat("straw ", 100)
	got := q.Snippet(text, 60)
	if !strings.Contains(got, "NEEDLE") || !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {
		t.Fatalf("snippet = %q", got)
	}
	if got := q.Snippet("short needle", 60); got != "short needle" {
		t.Fatalf("short = %q", got)
	}
}

// ripgrep and the Go walk hand the same lines to the caller.
func TestLinesRgAndGoAgree(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg is not on PATH")
	}
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("ab/one.jsonl", "{\"text\":\"Alpha beta\"}\n{\"text\":\"gamma\"}\n")
	write("~cd/two.jsonl", "{\"text\":\"BETA\"}\n")
	write("ab/.hev-state-123", "{\"text\":\"beta in a temp file\"}\n")
	write("ab/skip.json", "{\"text\":\"beta not jsonl\"}\n")
	write(".gitignore", "*\n")
	q, _ := Compile([]string{"beta"})
	collect := func() []string {
		var out []string
		var mu sync.Mutex
		err := Lines(context.Background(), q, []string{dir}, "*.jsonl", func(path string, line []byte) bool {
			rel, _ := filepath.Rel(dir, path)
			mu.Lock()
			out = append(out, rel+" "+string(line))
			mu.Unlock()
			return true
		})
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(out)
		return out
	}
	t.Setenv("HEV_GREP", "")
	rg := collect()
	t.Setenv("HEV_GREP", "go")
	gowalk := collect()
	want := `[ab/one.jsonl {"text":"Alpha beta"} ~cd/two.jsonl {"text":"BETA"}]`
	if got := strings.Join(rg, "|"); "["+strings.ReplaceAll(got, "|", " ")+"]" != want {
		t.Fatalf("rg = %v", rg)
	}
	if strings.Join(rg, "|") != strings.Join(gowalk, "|") {
		t.Fatalf("rg %v != go %v", rg, gowalk)
	}
}
