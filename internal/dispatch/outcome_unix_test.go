//go:build !windows

package dispatch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// outcomeFixture is an ended-run host: a fake claude that ends with the marker
// in $FINAL_MARKER (or, once a prompt carries the db answer, with done), and a
// notify command that records the rendered text.
func outcomeFixture(t *testing.T, name string) (dir, prompts, notices string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	prompts, notices = filepath.Join(bin, "prompts"), filepath.Join(bin, "notices")
	t.Setenv("PROMPT_LOG", prompts)
	t.Setenv("NOTIFY_LOG", notices)
	scripts := map[string]string{
		"claude": `#!/bin/sh
for a; do p="$a"; done
printf '%s\n---\n' "$p" >> "$PROMPT_LOG"
case "$p" in
*"Decision db (Postgres or SQLite?) was resolved: SQLite."*) printf 'chose SQLite\ndone\n' > ../status.md ;;
*) printf 'working\n%s\n' "$FINAL_MARKER" > ../status.md ;;
esac
`,
		"notifier": "#!/bin/sh\nprintf '%s\\n' \"$1\" >> \"$NOTIFY_LOG\"\n",
		"gh":       "#!/bin/sh\nexit 1\n",
		"git":      "#!/bin/sh\necho test-head\n",
		"tmux":     "#!/bin/sh\nexit 0\n",
	}
	for file, script := range scripts {
		if err := os.WriteFile(filepath.Join(bin, file), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	cfg := filepath.Join(bin, "dispatch.toml")
	if err := os.WriteFile(cfg, []byte(`notify = ["notifier", "dispatch {name}: {state} {summary}"]`), 0600); err != nil {
		t.Fatal(err)
	}
	dir = filepath.Join(Root(), name)
	if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
		t.Fatal(err)
	}
	m := Meta{Start: time.Now().UTC(), ConfigPath: cfg, LaunchResolved: true, Branch: "d/" + name, Order: Order{Name: name, Repo: "app", Harness: "claude", Timeout: 1}}
	if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "order.md"), []byte("---\nname: "+name+"\n---\nPick a database.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return dir, prompts, notices
}

func readResult(t *testing.T, dir string) Result {
	t.Helper()
	var r Result
	if err := readJSON(filepath.Join(dir, "result.json"), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func lastLine(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	return lines[len(lines)-1]
}

func TestNeedsDecisionResolvedByKey(t *testing.T) {
	dir, prompts, notices := outcomeFixture(t, "pick-db")
	t.Setenv("FINAL_MARKER", "needs-decision db: Postgres or SQLite?")
	launched := 0
	original := runLauncher
	t.Cleanup(func() { runLauncher = original })
	runLauncher = func(string, string) error { launched++; return nil }

	if err := superviseRound("pick-db"); err != nil {
		t.Fatal(err)
	}
	r := readResult(t, dir)
	if r.Outcome != OutcomeNeedsDecision || r.Key != "db" || r.Question != "Postgres or SQLite?" || State(&r, false) != "needs-review" {
		t.Fatalf("%+v", r)
	}
	// show --json carries the result file verbatim.
	show, err := Handle(Request{Action: "show", Name: "pick-db"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(show)
	for _, want := range []string{`\"outcome\": \"needs-decision\"`, `\"key\": \"db\"`, `\"question\": \"Postgres or SQLite?\"`} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("show --json missing %s: %s", want, show.Files["result.json"])
		}
	}
	if got := lastLine(t, notices); got != "dispatch pick-db: needs-review needs-decision db: Postgres or SQLite?; exit=0" {
		t.Fatalf("notify text %q", got)
	}
	list, err := Handle(Request{Action: "list"})
	if err != nil || len(list.Rows) != 1 || list.Rows[0].Key != "db" || list.Rows[0].Question != "Postgres or SQLite?" || !VisibleByDefault(list.Rows[0]) {
		t.Fatalf("ls row %+v %v", list.Rows, err)
	}

	// A bad key is refused and leaves the run as it was.
	if _, err := Handle(Request{Action: "say", Name: "pick-db", Note: "SQLite", Resolve: "cache"}); err == nil || !strings.Contains(err.Error(), "unknown decision key cache; open decision is db") {
		t.Fatalf("bad key: %v", err)
	}
	if launched != 0 || readResult(t, dir).Key != "db" {
		t.Fatal("bad key started a rework")
	}

	r2, err := Handle(Request{Action: "say", Name: "pick-db", Note: "SQLite", Resolve: "db", Meta: Meta{Sender: "local:hev"}})
	if err != nil || r2.Message != "started pick-db" || launched != 1 {
		t.Fatalf("%+v %v", r2, err)
	}
	order, _ := os.ReadFile(filepath.Join(dir, "order.md"))
	if !strings.Contains(string(order), "## Rework\n\nDecision db resolved.\n\nQuestion: Postgres or SQLite?\n\nAnswer: SQLite") {
		t.Fatalf("rework order %s", order)
	}
	// The same branch is reworked; the order is not restarted elsewhere.
	var m Meta
	if err := readJSON(filepath.Join(dir, "meta.json"), &m); err != nil || m.Branch != "d/pick-db" {
		t.Fatalf("%+v %v", m, err)
	}
	if _, err := Handle(Request{Action: "say", Name: "pick-db", Note: "Postgres", Resolve: "db"}); err == nil || !strings.Contains(err.Error(), "decision db already resolved: SQLite") {
		t.Fatalf("resolved key: %v", err)
	}

	// The relaunched worker sees the answer in its prompt and ends done.
	if err := superviseRound("pick-db"); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(prompts)
	attempts := strings.Split(string(b), "\n---\n")
	if len(attempts) != 3 || !strings.Contains(attempts[1], "Decision db (Postgres or SQLite?) was resolved: SQLite.") {
		t.Fatalf("rework prompt %q", b)
	}
	r = readResult(t, dir)
	if r.Outcome != OutcomeDone || State(&r, false) != "done" || len(r.Decisions) != 1 {
		t.Fatalf("%+v", r)
	}
	if d := r.Decisions[0]; d.Key != "db" || d.Question != "Postgres or SQLite?" || d.Answer != "SQLite" || d.ResolvedBy != "local:hev" || d.Resolved.IsZero() {
		t.Fatalf("resolution %+v", d)
	}
	if got := lastLine(t, notices); got != "dispatch pick-db: done exit=0" {
		t.Fatalf("notify text %q", got)
	}
	show, _ = Handle(Request{Action: "show", Name: "pick-db"})
	if !strings.Contains(show.Files["decisions.json"], `"answer": "SQLite"`) {
		t.Fatalf("decisions %s", show.Files["decisions.json"])
	}
	if _, err := Handle(Request{Action: "say", Name: "pick-db", Note: "Postgres", Resolve: "db"}); err == nil || !strings.Contains(err.Error(), "already resolved") {
		t.Fatalf("resolved key after done: %v", err)
	}
}

func TestBlockedOutcome(t *testing.T) {
	dir, _, notices := outcomeFixture(t, "stuck")
	t.Setenv("FINAL_MARKER", "blocked: staging credentials missing")
	if err := superviseRound("stuck"); err != nil {
		t.Fatal(err)
	}
	r := readResult(t, dir)
	if r.Outcome != OutcomeBlocked || r.Reason != "staging credentials missing" || State(&r, false) != "blocked" {
		t.Fatalf("%+v", r)
	}
	if got := lastLine(t, notices); got != "dispatch stuck: blocked blocked: staging credentials missing; exit=0" {
		t.Fatalf("notify text %q", got)
	}
	if _, err := Handle(Request{Action: "say", Name: "stuck", Note: "x", Resolve: "db"}); err == nil || !strings.Contains(err.Error(), "no open decision") {
		t.Fatalf("resolve on blocked run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "decisions.json")); !os.IsNotExist(err) {
		t.Fatal("blocked run opened a decision")
	}
}

func TestNoMarkerKeepsExitState(t *testing.T) {
	dir, _, notices := outcomeFixture(t, "plain")
	t.Setenv("FINAL_MARKER", "all finished, see report")
	if err := superviseRound("plain"); err != nil {
		t.Fatal(err)
	}
	r := readResult(t, dir)
	if r.Outcome != "" || r.Key != "" || State(&r, false) != "done" {
		t.Fatalf("%+v", r)
	}
	if got := lastLine(t, notices); got != "dispatch plain: done exit=0" {
		t.Fatalf("notify text %q", got)
	}
	// A result written by an older hev has no outcome and no open decision.
	if _, err := Handle(Request{Action: "say", Name: "plain", Note: "SQLite", Resolve: "db"}); err == nil || !strings.Contains(err.Error(), "unknown decision key db; no open decision") {
		t.Fatalf("resolve without marker: %v", err)
	}
}

func TestResolveRefusedForTeammate(t *testing.T) {
	q := Request{Action: "say", Name: "own", Note: "SQLite", Resolve: "db"}
	if err := authorizeSender(&q, "teammate"); err == nil || !strings.Contains(err.Error(), "cannot resolve") {
		t.Fatalf("%v", err)
	}
}
